// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"strconv"
	"strings"
)

// scope is where a program's expressions are being emitted. It decides what
// value() and country_code() lower to.
type scope int

const (
	// scopeRule is a format or checksum program: value() is the canonical
	// value under validation and never changes.
	scopeRule scope = iota
	// scopeCanonicalization is a canonicalizer: value() is the value current
	// at the moment the enclosing step runs, read straight from the workspace.
	scopeCanonicalization
)

// emitter renders one program into Go expressions.
type emitter struct {
	bundle *Bundle
	names  *naming
	prog   *Program
	scope  scope

	// target is the dispatch target a canonicalizer runs under, which lets
	// country_code() and PREPEND_COUNTRY_IF_MISSING fold to constants.
	target *DispatchTarget

	// local names a node that was hoisted into a variable.
	local map[uint32]string
}

// stringExpr renders a node producing a string view.
//
//nolint:gocyclo // One case per string constructor keeps every lowering next to the operation it implements.
func (e *emitter) stringExpr(idx uint32) string {
	if name, ok := e.local[idx]; ok {
		return name
	}
	n := e.prog.Nodes[idx]
	arg := func(k int) string { return e.stringExpr(n.InputNodes[k]) }

	switch n.Op {
	case OpConstant:
		return fmt.Sprintf("rt.Value(%s)", quote(n.Text))
	case OpValue:
		if e.scope == scopeCanonicalization {
			return "rt.Value(w.Str())"
		}
		return "rt.Value(c.value)"
	case OpSubject:
		return "subject"
	case OpCountryCode:
		// The selected target is known at generation time, so the country
		// folds to a constant, or to the absent view for a GLOBAL target.
		if e.target == nil || !e.target.HasCountryCode {
			return "rt.Absent"
		}
		return fmt.Sprintf("rt.Value(%s)", quote(e.target.CountryCode))
	case OpSlice:
		return fmt.Sprintf("%s.Slice(%d, %d)", arg(0), n.Start, n.End)
	case OpSliceFrom:
		return fmt.Sprintf("%s.SliceFrom(%d)", arg(0), n.Start)
	case OpSliceTo:
		return fmt.Sprintf("%s.SliceTo(%d)", arg(0), n.End)
	case OpBeforeFirst:
		return fmt.Sprintf("%s.BeforeFirst(%s)", arg(0), quote(n.Text))
	case OpAfterFirst:
		return fmt.Sprintf("%s.AfterFirst(%s)", arg(0), quote(n.Text))
	case OpStripPrefix:
		return fmt.Sprintf("%s.StripPrefix(%s)", arg(0), quote(n.Text))
	case OpConcat:
		parts := make([]string, len(n.InputNodes))
		for k := range n.InputNodes {
			parts[k] = arg(k)
		}
		return fmt.Sprintf("rt.Concat(%s)", strings.Join(parts, ", "))
	}
	panic(fmt.Sprintf("%s does not produce a string", n.Op))
}

// predExpr renders a node producing a boolean.
//
//nolint:gocyclo // The predicate table is flat by design.
func (e *emitter) predExpr(idx uint32) string {
	n := e.prog.Nodes[idx]
	str := func(k int) string { return e.stringExpr(n.InputNodes[k]) }

	switch n.Op {
	case OpIsEmpty:
		return fmt.Sprintf("%s.IsEmpty()", str(0))
	case OpIsAbsent:
		return fmt.Sprintf("%s.IsAbsent()", str(0))
	case OpEquals:
		return fmt.Sprintf("%s.Equals(%s)", str(0), str(1))
	case OpLengthEq:
		return fmt.Sprintf("%s.LengthEq(%d)", str(0), n.Length)
	case OpLengthIn:
		return fmt.Sprintf("%s.LengthIn(%s)", str(0), joinUint32(n.Lengths))
	case OpLengthBetween:
		return fmt.Sprintf("%s.LengthBetween(%d, %d)", str(0), n.MinLength, n.MaxLength)
	case OpASCIIDigits:
		return fmt.Sprintf("%s.ASCIIDigits()", str(0))
	case OpASCIIUpperLetters:
		return fmt.Sprintf("%s.ASCIIUpperLetters()", str(0))
	case OpASCIIAlphanumeric:
		return fmt.Sprintf("%s.ASCIIAlphanumeric()", str(0))
	case OpASCIICharset:
		return fmt.Sprintf("%s.ASCIICharset(%s)", str(0), quote(n.Text))
	case OpStartsWith:
		return fmt.Sprintf("%s.HasPrefix(%s)", str(0), quote(n.Text))
	case OpEndsWith:
		return fmt.Sprintf("%s.HasSuffix(%s)", str(0), quote(n.Text))
	case OpPrefixIn:
		// The values are emitted once, as a package level table grouped by
		// length, and searched rather than scanned: section 14 of engine.md
		// requires a membership test not to be linear in the size of the list,
		// and writing the list at the call site would rebuild it per call.
		return fmt.Sprintf("%s.PrefixInSorted(%s)", str(0), e.prefixSetName(idx))
	case OpCharAtIn:
		return fmt.Sprintf("%s.CharAtIn(%d, %s)", str(0), n.Index, quote(n.Text))
	case OpContains:
		return fmt.Sprintf("%s.Contains(%s)", str(0), quote(n.Text))
	case OpAll:
		return e.joinPredicates(n.InputNodes, " && ")
	case OpAny:
		return e.joinPredicates(n.InputNodes, " || ")
	case OpNot:
		return negate(e.predExpr(n.InputNodes[0]))
	case OpProfileIs:
		return fmt.Sprintf("c.profile == %s", profileConst(n.Text))
	case OpIntegerIs:
		return fmt.Sprintf("%s.Is(%d)", e.intExpr(n.InputNodes[0]), n.Constant)
	}
	panic(fmt.Sprintf("%s does not produce a boolean", n.Op))
}

func (e *emitter) joinPredicates(inputs []uint32, sep string) string {
	parts := make([]string, len(inputs))
	for k, in := range inputs {
		parts[k] = e.predExpr(in)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, sep) + ")"
}

// intExpr renders a node producing a checked integer.
func (e *emitter) intExpr(idx uint32) string {
	if name, ok := e.local[idx]; ok {
		return name
	}
	n := e.prog.Nodes[idx]
	switch n.Op {
	case OpDigitsToInteger:
		return fmt.Sprintf("rt.DigitsToInteger(%s)", e.stringExpr(n.InputNodes[0]))
	case OpModDigits:
		return fmt.Sprintf("rt.ModDigits(%s, %d)", e.stringExpr(n.InputNodes[0]), n.Modulus)
	case OpWeightedSum:
		if n.Mapping == MappingCustomAlphabet {
			return fmt.Sprintf("rt.WeightedSumAlphabet(%s, %s[:], %s, %q)",
				e.stringExpr(n.InputNodes[0]), e.weightsName(idx),
				alignConst(n.Alignment), n.Alphabet)
		}
		return fmt.Sprintf("rt.WeightedSum(%s, %s[:], %s, %s)",
			e.stringExpr(n.InputNodes[0]), e.weightsName(idx),
			alignConst(n.Alignment), mappingConst(n.Mapping))
	case OpModulo:
		return fmt.Sprintf("%s.Modulo(%d)", e.intExpr(n.InputNodes[0]), n.Modulus)
	case OpComplement:
		return fmt.Sprintf("%s.Complement(%d)", e.intExpr(n.InputNodes[0]), n.Modulus)
	case OpRemainderMap:
		return fmt.Sprintf("%s.RemainderMap(%s[:])", e.intExpr(n.InputNodes[0]), e.remainderName(idx))
	}
	panic(fmt.Sprintf("%s does not produce an integer", n.Op))
}

// weightsName and remainderName address the static tables emitted beside the
// program, one per node that needs one.
// prefixSetName names the table of accepted prefixes of one prefix_in node.
func (e *emitter) prefixSetName(idx uint32) string {
	return fmt.Sprintf("prefixes%s%d", capitalize(e.names.Program(e.prog.ID)), idx)
}

func (e *emitter) weightsName(idx uint32) string {
	return fmt.Sprintf("weights%s%d", capitalize(e.names.Program(e.prog.ID)), idx)
}

func (e *emitter) remainderName(idx uint32) string {
	return fmt.Sprintf("remainders%s%d", capitalize(e.names.Program(e.prog.ID)), idx)
}

// negate flips a rendered predicate, folding the double negation that
// require(not(...)) produces so the generated code reads directly.
func negate(expr string) string {
	if rest, ok := strings.CutPrefix(expr, "!"); ok {
		return rest
	}
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return "!" + expr
	}
	if strings.ContainsAny(expr, " ") && !strings.HasSuffix(expr, ")") {
		return "!(" + expr + ")"
	}
	return "!" + expr
}

func quote(s string) string { return strconv.Quote(s) }

func joinUint32(vs []uint32) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = strconv.FormatUint(uint64(v), 10)
	}
	return strings.Join(parts, ", ")
}

func joinQuoted(vs []string) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = strconv.Quote(v)
	}
	return strings.Join(parts, ", ")
}

func profileConst(name string) string {
	if name == ProfileStrictCurrent {
		return "StrictCurrent"
	}
	return "Compatible"
}

func tierConst(t SourceTier) string {
	switch t {
	case TierPrimary:
		return "TierPrimary"
	case TierSecondary:
		return "TierSecondary"
	}
	return "TierUnspecified"
}

func alignConst(a WeightAlignment) string {
	switch a {
	case AlignRight:
		return "rt.AlignRight"
	case AlignCycle:
		return "rt.AlignCycle"
	}
	return "rt.AlignLeft"
}

// mappingConst lowers the two mappings whose domain is fixed. CUSTOM_ALPHABET
// carries its own domain and is lowered to its own call instead.
func mappingConst(m CharMapping) string {
	if m == MappingAlnumBase36 {
		return "rt.MapAlnumBase36"
	}
	return "rt.MapDigitValue"
}
