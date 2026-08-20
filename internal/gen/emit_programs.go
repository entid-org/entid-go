// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"sort"
	"strings"
)

// emitPreCanonicalizers writes the dispatcher pre-canonicalizers and the switch
// that selects one.
func (g *generator) emitPreCanonicalizers() {
	seen := map[uint32]bool{}
	for _, d := range g.bundle.Dispatchers {
		id := d.PreCanonicalizationProgram
		if seen[id] {
			continue
		}
		seen[id] = true
		users := []string{}
		for _, other := range g.bundle.Dispatchers {
			if other.PreCanonicalizationProgram == id {
				users = append(users, other.Kind)
			}
		}
		p := g.bundle.Program(id)
		g.p("// %s is the routing pre-canonicalizer of %s.", g.names.Program(id), strings.Join(users, ", "))
		g.p("//")
		g.p("// It performs only the normalization routing needs, and can neither add,")
		g.p("// replace nor interpret a prefix.")
		g.p("func %s(w *rt.Buf) {", g.names.Program(id))
		e := &emitter{bundle: g.bundle, names: g.names, prog: p, scope: scopeCanonicalization}
		g.emitSteps(e, p.RootNode, 1)
		g.p("}")
		g.p("")
	}

	g.p("// preCanonicalize runs the pre-canonicalizer of a dispatcher exactly once")
	g.p("// on the raw value.")
	g.p("func preCanonicalize(d dispatcher, w *rt.Buf) {")
	g.p("\tswitch d {")
	for _, d := range g.bundle.Dispatchers {
		g.p("\tcase %s:", g.names.DispatcherConst(d.Kind))
		g.p("\t\t%s(w)", g.names.Program(d.PreCanonicalizationProgram))
	}
	g.p("\t}")
	g.p("}")
	g.p("")
}

// emitCanonicalizers writes one canonicalizer per definition, with the dispatch
// target folded in so that the country prefix is a constant.
func (g *generator) emitCanonicalizers() error {
	sizing, err := g.sizeScratch()
	if err != nil {
		return err
	}
	g.p("// The canonicalization workspace, sized from the growth the rules")
	g.p("// above can add. It is an array on the stack of the caller, so a")
	g.p("// canonicalization never allocates.")
	g.p("const (")
	g.p("\tcanonicalizationLeftMargin = %d", sizing.left)
	g.p("\tcanonicalizationScratch    = %d", sizing.size())
	g.p(")")
	g.p("")

	for _, def := range g.bundle.Identifiers {
		p := g.bundle.Program(def.CanonicalizationProgram)
		target := g.targetOf(def.ID)
		name := "canonicalize" + g.names.Definition(def.ID)
		g.p("// %s canonicalizes a %s value.", name, describeDefinition(def))
		g.p("func %s(w *rt.Buf, c ruleCtx) {", name)
		e := &emitter{
			bundle: g.bundle, names: g.names, prog: p,
			scope: scopeCanonicalization, target: target,
		}
		g.emitSteps(e, p.RootNode, 1)
		g.p("}")
		g.p("")
	}

	g.p("// canonicalize runs the canonicalizer of the selected definition exactly")
	g.p("// once on the pre-canonical value.")
	g.p("func canonicalize(d definition, w *rt.Buf, c ruleCtx) {")
	g.p("\tswitch d {")
	for _, def := range g.bundle.Identifiers {
		g.p("\tcase %s:", g.names.DefConst(def.ID))
		g.p("\t\tcanonicalize%s(w, c)", g.names.Definition(def.ID))
	}
	g.p("\t}")
	g.p("}")
	g.p("")
	return nil
}

// emitSteps writes the canonicalization steps a node stands for.
//
//nolint:gocyclo // One case per step keeps each lowering beside the rule it implements.
func (g *generator) emitSteps(e *emitter, idx uint32, depth int) {
	n := e.prog.Nodes[idx]
	pad := strings.Repeat("\t", depth)

	switch n.Op {
	case OpCanonSequence:
		for _, in := range n.InputNodes {
			g.emitSteps(e, in, depth)
		}
	case OpCanonWhen:
		g.p("%sif %s {", pad, e.predExpr(n.InputNodes[0]))
		for _, in := range n.InputNodes[1:] {
			g.emitSteps(e, in, depth+1)
		}
		g.p("%s}", pad)
	case OpTrimWhitespace:
		g.p("%sw.TrimWhitespace()", pad)
	case OpRemoveWhitespace:
		g.p("%sw.RemoveWhitespace()", pad)
	case OpUppercaseASCII:
		g.p("%sw.UppercaseASCII()", pad)
	case OpRemoveChars:
		g.p("%sw.RemoveChars(%s)", pad, quote(n.Text))
	case OpReplacePrefix:
		g.p("%sw.ReplacePrefix(%s, %s)", pad, quote(n.Text), quote(n.Replacement))
	case OpPrepend:
		g.p("%sw.Prepend(%s)", pad, quote(n.Text))
	case OpAppend:
		g.p("%sw.Append(%s)", pad, quote(n.Text))
	case OpInsert:
		g.p("%sw.Insert(%d, %s)", pad, n.Index, quote(n.Text))
	case OpLeftPad:
		g.p("%sw.LeftPad(%d, %s)", pad, n.Length, quote(n.Text))
	case OpPrependCountryIfMissing:
		g.emitPrependCountry(e, pad)
	default:
		panic(fmt.Sprintf("%s is not a canonicalization step", n.Op))
	}
}

// emitPrependCountry folds PREPEND_COUNTRY_IF_MISSING against the target the
// definition is reached through, which the generator knows statically.
func (g *generator) emitPrependCountry(e *emitter, pad string) {
	t := e.target
	if t == nil {
		panic("PREPEND_COUNTRY_IF_MISSING outside a definition canonicalizer")
	}
	lead := t.CanonicalPrefix
	if !t.HasCanonicalPrefix {
		lead = t.CountryCode
	}
	if lead == "" {
		return
	}
	if len(t.AcceptedPrefixes) == 0 {
		g.p("%sw.Prepend(%s)", pad, quote(lead))
		return
	}
	conds := make([]string, len(t.AcceptedPrefixes))
	for i, p := range t.AcceptedPrefixes {
		conds[i] = fmt.Sprintf("!w.HasPrefix(%s)", quote(p))
	}
	g.p("%sif %s {", pad, strings.Join(conds, " && "))
	g.p("%s\tw.Prepend(%s)", pad, quote(lead))
	g.p("%s}", pad)
}

// MaxScratchMargin is the largest growth this generator will size a stack
// workspace for, on either side of the value.
//
// It is derived from the specification rather than chosen: 4096 is the bound
// ir.md places on a slice bound and on a constant, and a code point is at most
// four UTF-8 bytes, so no single conforming step can add more. A bundle whose
// steps compose past it is refused, because a workspace beyond this size would
// stop fitting a goroutine stack — a limit of this implementation, not a
// reinterpretation of the rules.
const MaxScratchMargin = MaxSliceBound * 4

// scratchSizing is the workspace the compiled canonicalizers need.
type scratchSizing struct {
	left, right int
}

// size is the total workspace: the left margin, the largest input, and the
// growth the steps can add on the right.
func (s scratchSizing) size() int { return s.left + MaxInputBytes + s.right }

// sizeScratch computes the workspace from every canonicalizer of the bundle,
// routing ones included, and refuses one that would not fit a stack.
func (g *generator) sizeScratch() (scratchSizing, error) {
	var out scratchSizing
	seen := map[uint32]bool{}

	consider := func(p *Program, t *DispatchTarget) error {
		if p == nil || seen[p.ID] {
			return nil
		}
		seen[p.ID] = true
		left, right := growth(p, p.RootNode, t)
		if left > MaxScratchMargin || right > MaxScratchMargin {
			return invalidf(
				"canonicalization program %d may grow the value by %d bytes on the left and %d on the right, "+
					"beyond the %d bytes a single conforming step can add",
				p.ID, left, right, MaxScratchMargin)
		}
		out.left = max(out.left, left)
		out.right = max(out.right, right)
		return nil
	}

	for _, d := range g.bundle.Dispatchers {
		if err := consider(g.bundle.Program(d.PreCanonicalizationProgram), nil); err != nil {
			return out, err
		}
	}
	for _, def := range g.bundle.Identifiers {
		if err := consider(g.bundle.Program(def.CanonicalizationProgram), g.targetOf(def.ID)); err != nil {
			return out, err
		}
	}
	return out, nil
}

// targetOf resolves the dispatch target a definition is reached through.
func (g *generator) targetOf(id uint32) *DispatchTarget {
	for _, d := range g.bundle.Dispatchers {
		for _, t := range d.Targets {
			if t.IdentifierDefinitionID == id {
				return t
			}
		}
	}
	return nil
}

// growth bounds how many bytes a canonicalization node can add on either side.
func growth(p *Program, idx uint32, target *DispatchTarget) (left, right int) {
	n := p.Nodes[idx]
	switch n.Op {
	case OpCanonSequence:
		for _, in := range n.InputNodes {
			l, r := growth(p, in, target)
			left, right = left+l, right+r
		}
	case OpCanonWhen:
		for _, in := range n.InputNodes[1:] {
			l, r := growth(p, in, target)
			left, right = left+l, right+r
		}
	case OpReplacePrefix:
		left = max(0, len(n.Replacement)-len(n.Text))
	case OpPrepend:
		left = len(n.Text)
	case OpLeftPad:
		left = int(n.Length) * len(n.Text)
	case OpAppend, OpInsert:
		right = len(n.Text)
	case OpPrependCountryIfMissing:
		if target != nil {
			lead := target.CanonicalPrefix
			if !target.HasCanonicalPrefix {
				lead = target.CountryCode
			}
			left = len(lead)
		}
	}
	return left, right
}

// emitFormats writes one function per format program, plus the switch that
// selects one.
func (g *generator) emitFormats() {
	for _, p := range g.formatPrograms() {
		e := g.newRuleEmitter(p)
		g.p("// %s implements %s.", g.names.Program(p.ID), g.describeProgram(p))
		g.p("func %s(c ruleCtx, subject rt.View) stepResult {", g.names.Program(p.ID))
		g.emitLocals(e, 1)
		g.emitAssertions(e, p.RootNode, 1)
		g.p("\treturn validStep()")
		g.p("}")
		g.p("")
	}

	g.p("// formatRule runs the format rule of a definition on its canonical value.")
	g.p("func formatRule(d definition, c ruleCtx) stepResult {")
	g.p("\tswitch d {")
	for _, def := range g.bundle.Identifiers {
		g.p("\tcase %s:", g.names.DefConst(def.ID))
		g.p("\t\treturn %s(c, rt.Value(c.value))", g.names.Program(def.FormatProgram))
	}
	g.p("\t}")
	g.p("\treturn unsupportedStep(ReasonUnsupportedFormat, \"\")")
	g.p("}")
	g.p("")
}

// emitAssertions writes the ordered checks of a format program, each returning
// as soon as it fails.
func (g *generator) emitAssertions(e *emitter, idx uint32, depth int) {
	n := e.prog.Nodes[idx]
	pad := strings.Repeat("\t", depth)

	switch n.Op {
	case OpAssertSequence:
		for _, in := range n.InputNodes {
			g.emitAssertions(e, in, depth)
		}
	case OpRequire:
		key := ""
		if n.HasMessageKey {
			key = n.MessageKey
		}
		g.p("%sif %s {", pad, negate(e.predExpr(n.InputNodes[0])))
		g.p("%s\treturn invalidStep(%s, %s)", pad, n.ReasonCode.GoName(), quote(key))
		g.p("%s}", pad)
	case OpCallFormat:
		callee := g.names.Program(n.ProgramID)
		g.p("%s// The callee reason code and message key propagate unchanged.", pad)
		g.p("%sif r := %s(c, %s); r.status != Valid {", pad, callee, e.stringExpr(n.InputNodes[0]))
		g.p("%s\treturn r", pad)
		g.p("%s}", pad)
	default:
		panic(fmt.Sprintf("%s is not an assertion", n.Op))
	}
}

// emitChecksums writes one function per checksum program, plus the switch that
// selects one and the combinators the rules actually use.
func (g *generator) emitChecksums() {
	g.emitChecksumCombinators()
	for _, p := range g.checksumPrograms() {
		e := g.newRuleEmitter(p)
		g.p("// %s implements %s.", g.names.Program(p.ID), g.describeProgram(p))
		g.p("func %s(c ruleCtx, subject rt.View) stepResult {", g.names.Program(p.ID))
		g.emitLocals(e, 1)
		g.emitChecksumNodes(e, 1)
		g.p("\treturn %s", e.local[p.RootNode])
		g.p("}")
		g.p("")
	}

	g.p("// checksumRule runs the checksum rule of a definition on its canonical")
	g.p("// value. It is only reached once the format holds.")
	g.p("func checksumRule(d definition, c ruleCtx) stepResult {")
	g.p("\tswitch d {")
	for _, def := range g.bundle.Identifiers {
		if !def.HasChecksumProgram {
			continue
		}
		g.p("\tcase %s:", g.names.DefConst(def.ID))
		g.p("\t\treturn %s(c, rt.Value(c.value))", g.names.Program(def.ChecksumProgram))
	}
	g.p("\t}")
	g.p("\treturn unsupportedStep(absentChecksumReason(d), \"\")")
	g.p("}")
	g.p("")
}

// emitChecksumCombinators writes ALL_CHECKS and ANY_CHECK only when a rule uses
// them, so the engine carries no combinator the compiled rules never reach.
func (g *generator) emitChecksumCombinators() {
	uses := map[Opcode]bool{}
	for _, p := range g.bundle.Programs {
		for _, n := range p.Nodes {
			uses[n.Op] = true
		}
	}
	if uses[OpAllChecks] {
		g.p("// allChecks returns the first invalid outcome, otherwise the first")
		g.p("// unsupported one, otherwise valid. A later invalid outweighs an earlier")
		g.p("// unsupported, so every operand is considered.")
		g.p("func allChecks(results ...stepResult) stepResult {")
		g.p("	unsupported := -1")
		g.p("	for i, r := range results {")
		g.p("		if r.status == Invalid {")
		g.p("			return r")
		g.p("		}")
		g.p("		if r.status == Unsupported && unsupported < 0 {")
		g.p("			unsupported = i")
		g.p("		}")
		g.p("	}")
		g.p("	if unsupported >= 0 {")
		g.p("		return results[unsupported]")
		g.p("	}")
		g.p("	return validStep()")
		g.p("}")
		g.p("")
	}
	if uses[OpAnyCheck] {
		g.p("// anyCheck returns valid as soon as one operand is valid, otherwise the")
		g.p("// first unsupported one, otherwise the first invalid one.")
		g.p("func anyCheck(results ...stepResult) stepResult {")
		g.p("	unsupported, invalid := -1, -1")
		g.p("	for i, r := range results {")
		g.p("		switch r.status {")
		g.p("		case Valid:")
		g.p("			return r")
		g.p("		case Unsupported:")
		g.p("			if unsupported < 0 {")
		g.p("				unsupported = i")
		g.p("			}")
		g.p("		case Invalid:")
		g.p("			if invalid < 0 {")
		g.p("				invalid = i")
		g.p("			}")
		g.p("		}")
		g.p("	}")
		g.p("	switch {")
		g.p("	case unsupported >= 0:")
		g.p("		return results[unsupported]")
		g.p("	case invalid >= 0:")
		g.p("		return results[invalid]")
		g.p("	}")
		g.p("	return unsupportedStep(ReasonUnsupportedChecksum, \"\")")
		g.p("}")
		g.p("")
	}
}

// emitChecksumNodes writes every checksum outcome of a program as a local, in
// topological order. A WHEN branch is not written on its own: only the CHOOSE
// that owns it can observe whether it applies.
func (g *generator) emitChecksumNodes(e *emitter, depth int) {
	pad := strings.Repeat("\t", depth)
	for i, n := range e.prog.Nodes {
		idx := uint32(i)
		if n.OutputType != ValueChecksumOutcome || n.Op == OpChecksumWhen {
			continue
		}
		name := e.local[idx]
		switch n.Op {
		case OpChoose:
			g.emitChoose(e, idx, name, pad)
		default:
			g.p("%s%s := %s", pad, name, g.checksumExpr(e, idx))
		}
	}
}

// emitChoose writes a CHOOSE as a switch over its branch predicates. A branch
// that is not a WHEN always applies, so it becomes the default and every branch
// after it is dead and dropped.
func (g *generator) emitChoose(e *emitter, idx uint32, name, pad string) {
	n := e.prog.Nodes[idx]
	g.p("%svar %s stepResult", pad, name)
	g.p("%sswitch {", pad)
	for _, in := range n.InputNodes {
		branch := e.prog.Nodes[in]
		if branch.Op != OpChecksumWhen {
			g.p("%sdefault:", pad)
			g.p("%s\t%s = %s", pad, name, e.local[in])
			g.p("%s}", pad)
			return
		}
		g.p("%scase %s:", pad, e.predExpr(branch.InputNodes[0]))
		g.p("%s\t%s = %s", pad, name, e.local[branch.InputNodes[1]])
	}
	g.p("%sdefault:", pad)
	g.p("%s\t%s = unsupportedStep(ReasonUnsupportedChecksum, \"\")", pad, name)
	g.p("%s}", pad)
}

// checksumExpr renders a checksum node that is not a CHOOSE.
func (g *generator) checksumExpr(e *emitter, idx uint32) string {
	n := e.prog.Nodes[idx]
	key := quote(n.MessageKey)
	switch n.Op {
	case OpLuhn:
		return fmt.Sprintf("fromOutcome(rt.Luhn(%s), %s)", e.stringExpr(n.InputNodes[0]), key)
	case OpISO7064Mod97_10:
		return fmt.Sprintf("fromOutcome(rt.ISO7064Mod97(%s), %s)", e.stringExpr(n.InputNodes[0]), key)
	case OpCompareDigit:
		return fmt.Sprintf("fromOutcome(rt.CompareDigit(%s, %s, %d), %s)",
			e.intExpr(n.InputNodes[0]), e.stringExpr(n.InputNodes[1]), n.Index, key)
	case OpCompareSlice:
		return fmt.Sprintf("fromOutcome(rt.CompareSlice(%s, %s, %d, %d), %s)",
			e.intExpr(n.InputNodes[0]), e.stringExpr(n.InputNodes[1]), n.Start, n.End, key)
	case OpCompareConstant:
		return fmt.Sprintf("fromOutcome(rt.CompareConstant(%s, %d), %s)",
			e.intExpr(n.InputNodes[0]), n.Constant, key)
	case OpUnsupportedChecksum:
		return fmt.Sprintf("unsupportedStep(%s, %s)", n.ReasonCode.GoName(), key)
	case OpAllChecks:
		return fmt.Sprintf("allChecks(%s)", g.joinLocals(e, n.InputNodes))
	case OpAnyCheck:
		return fmt.Sprintf("anyCheck(%s)", g.joinLocals(e, n.InputNodes))
	case OpCallChecksum:
		return fmt.Sprintf("%s(c, %s)", g.names.Program(n.ProgramID), e.stringExpr(n.InputNodes[0]))
	}
	panic(fmt.Sprintf("%s is not a checksum operation", n.Op))
}

func (g *generator) joinLocals(e *emitter, inputs []uint32) string {
	parts := make([]string, len(inputs))
	for i, in := range inputs {
		parts[i] = e.local[in]
	}
	return strings.Join(parts, ", ")
}

// newRuleEmitter prepares a format or checksum program: every checksum outcome
// gets a local, and so does every string or integer node used more than once,
// which keeps the emitted expressions readable and each subexpression computed
// once.
func (g *generator) newRuleEmitter(p *Program) *emitter {
	e := &emitter{
		bundle: g.bundle, names: g.names, prog: p,
		scope: scopeRule, local: map[uint32]string{},
	}
	uses := make([]int, len(p.Nodes))
	for _, n := range p.Nodes {
		for _, in := range n.InputNodes {
			uses[in]++
		}
	}
	for i, n := range p.Nodes {
		idx := uint32(i)
		switch {
		case n.OutputType == ValueChecksumOutcome && n.Op != OpChecksumWhen:
			e.local[idx] = fmt.Sprintf("check%d", i)
		case (n.OutputType == ValueString || n.OutputType == ValueInteger) && uses[i] > 1 && !trivial(n.Op):
			e.local[idx] = fmt.Sprintf("n%d", i)
		}
	}
	return e
}

// trivial reports whether an operation already renders as a bare identifier or
// a constant, in which case hoisting it into a local would only add noise.
func trivial(op Opcode) bool {
	switch op {
	case OpSubject, OpValue, OpConstant, OpCountryCode:
		return true
	}
	return false
}

// emitLocals writes the hoisted string and integer nodes, in topological order.
func (g *generator) emitLocals(e *emitter, depth int) {
	pad := strings.Repeat("\t", depth)
	for i, n := range e.prog.Nodes {
		idx := uint32(i)
		name, hoisted := e.local[idx]
		if !hoisted || n.OutputType == ValueChecksumOutcome {
			continue
		}
		// The node must render without consulting its own local name.
		delete(e.local, idx)
		var expr string
		if n.OutputType == ValueString {
			expr = e.stringExpr(idx)
		} else {
			expr = e.intExpr(idx)
		}
		e.local[idx] = name
		g.p("%s%s := %s", pad, name, expr)
	}
}

func (g *generator) formatPrograms() []*Program   { return g.programsOfKind(ProgramFormat) }
func (g *generator) checksumPrograms() []*Program { return g.programsOfKind(ProgramChecksum) }

func (g *generator) programsOfKind(kind ProgramKind) []*Program {
	var out []*Program
	for _, p := range g.bundle.Programs {
		if p.Kind == kind {
			out = append(out, p)
		}
	}
	// Emitting callees before callers keeps the file readable top to bottom.
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// describeProgram names the rule a program implements, for the doc comment.
func (g *generator) describeProgram(p *Program) string {
	for _, d := range g.bundle.Identifiers {
		switch p.ID {
		case d.FormatProgram:
			return "the format rule of " + describeDefinition(d)
		case d.ChecksumProgram:
			if d.HasChecksumProgram {
				return "the checksum rule of " + describeDefinition(d)
			}
		case d.CanonicalizationProgram:
			return "the canonicalizer of " + describeDefinition(d)
		}
	}
	return fmt.Sprintf("program %d", p.ID)
}

func describeDefinition(d *IdentifierDefinition) string {
	if !d.HasCountryCode {
		return d.Kind + " (global)"
	}
	return d.Kind + " " + d.CountryCode
}

// emitTables writes the static weight and remainder tables. Arrays, not slices:
// an array of constants lives in read-only data, while a slice literal is built
// when the program starts.
func (g *generator) emitTables() {
	type table struct {
		name    string
		values  []int64
		comment string
	}
	var tables []table
	for _, p := range g.bundle.Programs {
		e := &emitter{bundle: g.bundle, names: g.names, prog: p}
		for i, n := range p.Nodes {
			switch n.Op {
			case OpWeightedSum:
				tables = append(tables, table{
					name: e.weightsName(uint32(i)), values: n.Weights,
					comment: fmt.Sprintf("weights of %s node %d", g.names.Program(p.ID), i),
				})
			case OpRemainderMap:
				tables = append(tables, table{
					name: e.remainderName(uint32(i)), values: n.RemainderValues,
					comment: fmt.Sprintf("remainder table of %s node %d", g.names.Program(p.ID), i),
				})
			}
		}
	}
	if len(tables) == 0 {
		return
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].name < tables[j].name })
	for _, t := range tables {
		g.p("// %s is the %s.", t.name, t.comment)
		g.p("var %s = [%d]int64{%s}", t.name, len(t.values), joinInt64(t.values))
		g.p("")
	}
}

func joinInt64(vs []int64) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ", ")
}
