// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

// ErrInvalidRuleset reports a bundle whose size, structure, arithmetic or graph
// violates the V1 contract. Section 10 of ir.md maps every such violation to
// invalid_ruleset.
var ErrInvalidRuleset = errors.New("invalid_ruleset")

// ErrIncompatibleRuleset reports a bundle this generator understands the shape
// of but refuses to compile: an unsupported format version or an unknown
// capability id. Section 2.3 requires closed generation rather than partial
// output.
var ErrIncompatibleRuleset = errors.New("incompatible_ruleset")

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRuleset, fmt.Sprintf(format, args...))
}

func incompatiblef(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrIncompatibleRuleset, fmt.Sprintf(format, args...))
}

// Load decodes and fully validates a rule bundle. It performs the twenty five
// ordered checks of section 10 of ir.md and never returns a partially validated
// graph: the bundle is either ready to generate from, or refused.
//
// The bundle is untrusted input: it is an artifact fetched from a release, and
// the generator is the only place where its structure is ever checked.
func Load(raw []byte) (*Bundle, error) {
	// 1. binary size at most 16 MiB
	if len(raw) > MaxBundleBytes {
		return nil, invalidf("bundle of %d bytes exceeds the %d byte limit", len(raw), MaxBundleBytes)
	}

	// 2. complete Protobuf decoding
	//
	// This first pass skips what it does not know instead of refusing it, so
	// that checks 3 and 4 can answer before check 5. A bundle built against a
	// later version carries fields and operations this generator has never
	// heard of, and calling that a forged bundle rather than a version gap
	// sends an operator looking for a tampered file instead of an upgrade.
	scout := &Bundle{}
	sr := newReader(raw)
	sr.lenient = true
	if err := decodeBundle(sr, scout); err != nil {
		return nil, err
	}
	if !sr.done() {
		return nil, invalidf("trailing bytes after the root message")
	}

	// 3. supported format_version
	// 4. every required_feature_ids entry known, strictly ascending
	declared, err := checkVersions(scout)
	if err != nil {
		return nil, err
	}

	// 5. absence of any unknown field at any depth
	b := &Bundle{}
	r := newReader(raw)
	if err := decodeBundle(r, b); err != nil {
		return nil, err
	}
	if !r.done() {
		return nil, invalidf("trailing bytes after the root message")
	}

	v := &validator{
		bundle:       b,
		declared:     declared,
		usedFeatures: map[uint32]bool{FeatureCoreGraph: true},
	}
	if err := v.run(); err != nil {
		return nil, err
	}
	return b, nil
}

// checkVersions runs checks 3 and 4 on the leniently decoded bundle and returns
// the set of declared capabilities.
func checkVersions(b *Bundle) (map[uint32]bool, error) {
	// 3. supported format_version
	if b.FormatVersion != SupportedFormatVersion {
		return nil, incompatiblef("format version %d is not supported, this generator implements version %d",
			b.FormatVersion, SupportedFormatVersion)
	}

	// 4. every required_feature_ids entry known, strictly ascending
	declared := make(map[uint32]bool, len(b.RequiredFeatureIDs))
	for i, id := range b.RequiredFeatureIDs {
		if i > 0 && id <= b.RequiredFeatureIDs[i-1] {
			return nil, invalidf("required_feature_ids is not strictly ascending at index %d", i)
		}
		if !supportedFeatureSet[id] {
			return nil, incompatiblef("capability %d is unknown to this generator", id)
		}
		declared[id] = true
	}
	return declared, nil
}

// validator carries the state shared by the ordered load time checks.
type validator struct {
	bundle       *Bundle
	usedFeatures map[uint32]bool
	declared     map[uint32]bool

	// preCanonicalizers holds the programs a dispatcher uses for routing.
	// Check 15 restricts their operations, so they are identified before the
	// programs are walked.
	preCanonicalizers map[uint32]bool
}

func (v *validator) run() error {
	b := v.bundle

	// 6. rules_version non empty, at most 64 bytes, and made only of ASCII
	// letters, digits, dot, dash and underscore
	if err := checkRulesVersion(b.RulesVersion); err != nil {
		return err
	}

	// 7. source_digest of exactly 32 bytes
	if len(b.SourceDigest) != 32 {
		return invalidf("source_digest holds %d bytes, 32 are required", len(b.SourceDigest))
	}

	// 8..16 operate on programs. Which ones route is declared by the
	// dispatchers, so that is collected first.
	v.preCanonicalizers = make(map[uint32]bool, len(b.Dispatchers))
	for _, d := range b.Dispatchers {
		v.preCanonicalizers[d.PreCanonicalizationProgram] = true
	}
	if err := v.checkPrograms(); err != nil {
		return err
	}
	// 17..18 operate on identifier definitions.
	if err := v.checkIdentifiers(); err != nil {
		return err
	}
	// 19..23 operate on dispatchers.
	if err := v.checkDispatchers(); err != nil {
		return err
	}
	// 24. call graph acyclic, typed and of static depth at most 32
	if err := v.checkCallGraph(); err != nil {
		return err
	}

	// 25. no capability used without being declared
	for _, id := range SupportedFeatures {
		if v.usedFeatures[id] && !v.declared[id] {
			return invalidf("capability %d is used but not declared in required_feature_ids", id)
		}
	}
	return nil
}

func (v *validator) use(ids ...uint32) {
	for _, id := range ids {
		v.usedFeatures[id] = true
	}
}

// checkPrograms runs checks 8 to 16.
func (v *validator) checkPrograms() error {
	b := v.bundle
	b.programByID = make(map[uint32]*Program, len(b.Programs))

	// 8. program ids unique and non zero, program kinds specified
	total := 0
	for i, p := range b.Programs {
		if p.ID == 0 {
			return invalidf("program at index %d has id 0", i)
		}
		if i > 0 && p.ID <= b.Programs[i-1].ID {
			return invalidf("programs are not sorted by ascending id at index %d", i)
		}
		if _, dup := b.programByID[p.ID]; dup {
			return invalidf("program id %d appears twice", p.ID)
		}
		switch p.Kind {
		case ProgramCanonicalization, ProgramFormat, ProgramChecksum:
		default:
			return invalidf("program %d has an unspecified or unknown kind", p.ID)
		}
		b.programByID[p.ID] = p

		// 9. node count within the per program and total limits
		if len(p.Nodes) == 0 {
			return invalidf("program %d holds no node", p.ID)
		}
		if len(p.Nodes) > MaxNodesPerProgram {
			return invalidf("program %d holds %d nodes, the limit is %d", p.ID, len(p.Nodes), MaxNodesPerProgram)
		}
		total += len(p.Nodes)
		if total > MaxTotalNodes {
			return invalidf("the bundle holds more than %d nodes", MaxTotalNodes)
		}
	}

	for _, p := range b.Programs {
		if err := v.checkProgram(p); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // Checks 10 to 15 are one ordered pass over a program's nodes; splitting them would hide the order the specification mandates.
func (v *validator) checkProgram(p *Program) error {
	// bounds[i] is the provable upper bound in code points of node i when it
	// produces a string, or unboundedLen when no bound can be proven.
	bounds := make([]int, len(p.Nodes))

	for i, n := range p.Nodes {
		// 10. every operation known, with its declared output type
		if n.Op <= OpInvalid || n.Op >= opCount {
			return invalidf("program %d node %d carries an unknown operation", p.ID, i)
		}
		spec := n.Op.spec()
		if n.OutputType != spec.output {
			return invalidf("program %d node %d declares output type %s but %s produces %s",
				p.ID, i, n.OutputType, n.Op, spec.output)
		}

		// 11. operand count, operand types and strictly lower operand indices
		if err := v.checkOperands(p, i, n, spec); err != nil {
			return err
		}

		// 12. only the parameters the operation declares, and every required one
		got := n.present()
		if extra := got &^ spec.allowed(); extra != 0 {
			return invalidf("program %d node %d passes %s to %s, which does not declare it",
				p.ID, i, paramList(extra), n.Op)
		}
		if missing := spec.required &^ got; missing != 0 {
			return invalidf("program %d node %d omits %s, required by %s",
				p.ID, i, paramList(missing), n.Op)
		}

		// 13. arithmetic bounds
		if err := v.checkBounds(p, i, n, bounds); err != nil {
			return err
		}

		// 16. program shape, node level: accepted categories per program kind
		if err := v.checkCategory(p, i, n); err != nil {
			return err
		}

		v.use(spec.features...)
		bounds[i] = stringBound(n, bounds)
	}

	// 14. expansion within the evaluation budget once repeated operands are
	// inlined. Check 15 is what proves the root is inside the program, so a
	// root out of range is left for it to name.
	if int(p.RootNode) < len(p.Nodes) {
		if n := expansionOf(p); n > MaxSteps {
			return invalidf("program %d expands to %s operation instances once repeated operands are inlined, the budget is %d",
				p.ID, describeExpansion(n), MaxSteps)
		}
	}

	// 15. root, subject and capture nodes inside the program and correctly typed
	if int(p.RootNode) >= len(p.Nodes) {
		return invalidf("program %d has root node %d outside its %d nodes", p.ID, p.RootNode, len(p.Nodes))
	}
	if p.HasSubject {
		if p.Kind == ProgramCanonicalization {
			return invalidf("canonicalization program %d declares a subject", p.ID)
		}
		if int(p.SubjectNode) >= len(p.Nodes) {
			return invalidf("program %d has subject node %d outside its %d nodes", p.ID, p.SubjectNode, len(p.Nodes))
		}
		if p.Nodes[p.SubjectNode].OutputType != ValueString {
			return invalidf("program %d subject node %d does not produce a string", p.ID, p.SubjectNode)
		}
		v.use(FeatureCapturesAndCalls)
	}
	if len(p.Captures) > 0 {
		if p.Kind != ProgramFormat {
			return invalidf("program %d is not a format program but declares captures", p.ID)
		}
		if len(p.Captures) > MaxCapturesPerFormat {
			return invalidf("program %d declares %d captures, the limit is %d",
				p.ID, len(p.Captures), MaxCapturesPerFormat)
		}
		names := make(map[string]bool, len(p.Captures))
		for _, c := range p.Captures {
			if c.Name == "" {
				return invalidf("program %d declares an unnamed capture", p.ID)
			}
			if names[c.Name] {
				return invalidf("program %d declares capture %q twice", p.ID, c.Name)
			}
			names[c.Name] = true
			if int(c.Node) >= len(p.Nodes) {
				return invalidf("program %d capture %q points at node %d outside its %d nodes",
					p.ID, c.Name, c.Node, len(p.Nodes))
			}
			if p.Nodes[c.Node].OutputType != ValueString {
				return invalidf("program %d capture %q does not name a string node", p.ID, c.Name)
			}
		}
		v.use(FeatureCapturesAndCalls)
	}

	// 16. program shape, root level.
	return v.checkRoot(p)
}

func (v *validator) checkOperands(p *Program, i int, n *Node, spec *opSpec) error {
	for _, in := range n.InputNodes {
		// i is a slice index, so it is never negative; comparing as int is
		// exact for every operand a bundle can carry.
		if in > math.MaxInt32 || int(in) >= i {
			return invalidf("program %d node %d references node %d, which is not strictly lower", p.ID, i, in)
		}
	}
	fixed := len(spec.fixed)
	if len(n.InputNodes) < fixed {
		return invalidf("program %d node %d gives %d operands to %s, which needs at least %d",
			p.ID, i, len(n.InputNodes), n.Op, fixed)
	}
	for k, want := range spec.fixed {
		if got := p.Nodes[n.InputNodes[k]].OutputType; got != want {
			return invalidf("program %d node %d passes a %s as operand %d of %s, which needs a %s",
				p.ID, i, got, k, n.Op, want)
		}
	}
	rest := n.InputNodes[fixed:]
	if spec.repeated == ValueUnspecified {
		if len(rest) > 0 {
			return invalidf("program %d node %d gives %d operands to %s, which takes exactly %d",
				p.ID, i, len(n.InputNodes), n.Op, fixed)
		}
		return nil
	}
	if len(rest) < spec.minRep {
		return invalidf("program %d node %d gives %d repeated operands to %s, which needs at least %d",
			p.ID, i, len(rest), n.Op, spec.minRep)
	}
	if spec.maxRep != unbounded && len(rest) > spec.maxRep {
		return invalidf("program %d node %d gives %d repeated operands to %s, the limit is %d",
			p.ID, i, len(rest), n.Op, spec.maxRep)
	}
	for k, in := range rest {
		if got := p.Nodes[in].OutputType; got != spec.repeated {
			return invalidf("program %d node %d passes a %s as repeated operand %d of %s, which needs a %s",
				p.ID, i, got, k, n.Op, spec.repeated)
		}
	}
	return nil
}

//nolint:gocyclo // The arithmetic bounds of section 8 of ir.md are a flat list; a switch keeps each one next to the operation it constrains.
func (v *validator) checkBounds(p *Program, i int, n *Node, bounds []int) error {
	where := func(what string) error {
		return invalidf("program %d node %d: %s", p.ID, i, what)
	}

	if n.HasMessageKey && n.MessageKey == "" {
		return where("a declared message key must not be empty")
	}
	if n.HasText && len(n.Text) > MaxConstantBytes {
		return where(fmt.Sprintf("constant of %d bytes exceeds the %d byte limit", len(n.Text), MaxConstantBytes))
	}
	if n.HasReplacement && len(n.Replacement) > MaxConstantBytes {
		return where(fmt.Sprintf("replacement of %d bytes exceeds the %d byte limit",
			len(n.Replacement), MaxConstantBytes))
	}
	for _, bound := range []struct {
		has  bool
		val  uint32
		name string
	}{
		{n.HasStart, n.Start, "start"},
		{n.HasEnd, n.End, "end"},
		{n.HasIndex, n.Index, "index"},
	} {
		if bound.has && bound.val > MaxSliceBound {
			return where(fmt.Sprintf("%s is %d, the limit is %d", bound.name, bound.val, MaxSliceBound))
		}
	}
	if n.HasModulus && (n.Modulus < MinModulus || n.Modulus > MaxModulus) {
		return where(fmt.Sprintf("modulus %d is outside [%d, %d]", n.Modulus, MinModulus, MaxModulus))
	}
	if len(n.Weights) > 0 {
		if len(n.Weights) < MinWeights || len(n.Weights) > MaxWeights {
			return where(fmt.Sprintf("%d weights, the accepted range is [%d, %d]",
				len(n.Weights), MinWeights, MaxWeights))
		}
		for _, w := range n.Weights {
			if w < -MaxWeightMagnitude || w > MaxWeightMagnitude {
				return where(fmt.Sprintf("weight %d exceeds the magnitude limit %d", w, MaxWeightMagnitude))
			}
		}
	}
	if len(n.RemainderValues) > 0 &&
		(len(n.RemainderValues) < MinRemainderValues || len(n.RemainderValues) > MaxRemainderValues) {
		return where(fmt.Sprintf("%d remainder values, the accepted range is [%d, %d]",
			len(n.RemainderValues), MinRemainderValues, MaxRemainderValues))
	}

	// Section 8 bounds the comparison constant like every other integer of the
	// IR; the operations that carry one are the only ones that declare it.
	if n.HasConstant && (n.Constant < MinConstant || n.Constant > MaxConstant) {
		return where(fmt.Sprintf("constant %d is outside [%d, %d]", n.Constant, MinConstant, MaxConstant))
	}

	switch n.Op {
	case OpSlice:
		if n.Start > n.End {
			return where(fmt.Sprintf("slice start %d is above end %d", n.Start, n.End))
		}
	case OpLengthBetween:
		if n.MinLength > n.MaxLength {
			return where(fmt.Sprintf("min_length %d is above max_length %d", n.MinLength, n.MaxLength))
		}
	case OpLengthIn:
		for k, l := range n.Lengths {
			if k > 0 && l <= n.Lengths[k-1] {
				return where("lengths are not strictly ascending")
			}
		}
	case OpPrefixIn:
		for k, val := range n.Values {
			if val == "" {
				return where("prefix_in carries an empty value")
			}
			if k > 0 && val <= n.Values[k-1] {
				return where("prefix_in values are not strictly ascending")
			}
		}
	case OpWeightedSum:
		switch n.Alignment {
		case AlignLeft, AlignRight, AlignCycle:
		default:
			return where("weighted_sum has an unspecified alignment")
		}
		switch n.Mapping {
		case MappingDigitValue, MappingAlnumBase36:
			// The alphabet belongs to CUSTOM_ALPHABET alone, so a bundle
			// cannot state one that nothing reads.
			if n.HasAlphabet {
				return where("weighted_sum carries an alphabet under a mapping that does not read one")
			}
		case MappingCustomAlphabet:
			if err := checkAlphabet(n.Alphabet, n.HasAlphabet); err != nil {
				return where(err.Error())
			}
			// Capability 42 belongs to the variant, not to the operation: a
			// weighted sum over digits must not oblige an engine to implement
			// an alphabet it never reads.
			v.use(FeatureChecksumCustomAlphabet)
		default:
			return where("weighted_sum has an unspecified mapping")
		}
		if err := checkWeightedSumOverflow(n, bounds[n.InputNodes[0]]); err != nil {
			return where(err.Error())
		}
	case OpDigitsToInteger:
		got := bounds[n.InputNodes[0]]
		if got == unboundedLen || got < MinProvableDigits || got > MaxProvableDigits {
			return where(fmt.Sprintf("digits_to_integer needs a provable operand length in [%d, %d], got %s",
				MinProvableDigits, MaxProvableDigits, describeBound(got)))
		}
	case OpCompareSlice:
		width := int64(n.End) - int64(n.Start)
		if width < MinProvableDigits || width > MaxProvableDigits {
			return where(fmt.Sprintf("compare_slice width %d is outside [%d, %d]",
				width, MinProvableDigits, MaxProvableDigits))
		}
	case OpRequire:
		if !n.ReasonCode.provesInvalidity() {
			return where(fmt.Sprintf("reason code %s cannot prove an invalidity", n.ReasonCode))
		}
	case OpUnsupportedChecksum:
		if !n.ReasonCode.absentChecksumReason() {
			return where(fmt.Sprintf("reason code %s cannot explain an unsupported checksum", n.ReasonCode))
		}
	case OpProfileIs:
		if !validProfile(n.Text) {
			return where(fmt.Sprintf("profile_is names the unknown profile %q", n.Text))
		}
	case OpBeforeFirst, OpAfterFirst, OpASCIICharset, OpStartsWith, OpEndsWith,
		OpContains, OpCharAtIn, OpRemoveChars, OpPrepend, OpAppend, OpInsert,
		OpReplacePrefix:
		// ir.md calls the constant of each of these operations non empty.
		// CONSTANT and STRIP_PREFIX carry no such requirement.
		if n.Text == "" {
			return where(fmt.Sprintf("%s needs a non empty text parameter", n.Op))
		}
	case OpLeftPad:
		if utf8.RuneCountInString(n.Text) != 1 {
			return where("left_pad needs exactly one padding code point")
		}
		// Since ir.md bounds it like every other slice bound, an engine that
		// sizes a buffer from it has a static maximum.
		if n.Length < 1 || n.Length > MaxSliceBound {
			return where(fmt.Sprintf("left_pad length %d is outside [1, %d]", n.Length, MaxSliceBound))
		}
	}
	if n.Op == OpReplacePrefix && n.Text == n.Replacement {
		return where("replace_prefix maps a prefix to itself")
	}
	return nil
}

// checkWeightedSumOverflow proves that no weighted sum can overflow int64
// within the V1 limits. LEFT and RIGHT pair at most one position per weight;
// CYCLE pairs one per code point of the operand, which the input bound and the
// canonicalization growth keep well inside the range.
func checkWeightedSumOverflow(n *Node, operandBound int) error {
	maxContribution := int64(9)
	switch n.Mapping {
	case MappingAlnumBase36:
		maxContribution = 35
	case MappingCustomAlphabet:
		// The largest value a custom alphabet can give a code point is the
		// index of its last one.
		maxContribution = int64(utf8.RuneCountInString(n.Alphabet)) - 1
	}
	var pairs int64
	switch n.Alignment {
	case AlignLeft, AlignRight:
		pairs = int64(len(n.Weights))
	default:
		if operandBound == unboundedLen {
			// An unbounded operand can still only hold as many code points as
			// the canonical value, itself bounded by the 1024 byte input limit
			// plus what canonicalization may prepend.
			pairs = int64(MaxInputBytes + MaxConstantBytes)
		} else {
			pairs = int64(operandBound)
		}
	}
	var magnitude int64
	for _, w := range n.Weights {
		if w < 0 {
			w = -w
		}
		magnitude = max(magnitude, w)
	}
	if magnitude == 0 || pairs == 0 {
		return nil
	}
	if magnitude > math.MaxInt64/pairs {
		return errors.New("weighted_sum may overflow a signed 64 bit integer")
	}
	if bound := magnitude * pairs; maxContribution > math.MaxInt64/bound {
		return errors.New("weighted_sum may overflow a signed 64 bit integer")
	}
	return nil
}

// unboundedLen marks a string expression whose maximum length cannot be proven.
const unboundedLen = -1

func describeBound(b int) string {
	if b == unboundedLen {
		return "no provable bound"
	}
	return fmt.Sprintf("%d", b)
}

// stringBound computes the provable maximum length in code points of the string
// a node produces. Nodes are visited in topological order, so every operand
// bound is already known.
func stringBound(n *Node, bounds []int) int {
	if n.OutputType != ValueString {
		return 0
	}
	operand := func(k int) int { return bounds[n.InputNodes[k]] }
	switch n.Op {
	case OpConstant:
		return utf8.RuneCountInString(n.Text)
	case OpCountryCode:
		// A canonical country code is exactly two ASCII letters.
		return 2
	case OpSlice:
		return int(n.End - n.Start)
	case OpSliceFrom:
		if b := operand(0); b != unboundedLen {
			return max(0, b-int(n.Start))
		}
		return unboundedLen
	case OpSliceTo:
		if b := operand(0); b != unboundedLen {
			return min(b, int(n.End))
		}
		return int(n.End)
	case OpBeforeFirst, OpAfterFirst, OpStripPrefix:
		return operand(0)
	case OpConcat:
		total := 0
		for k := range n.InputNodes {
			b := operand(k)
			if b == unboundedLen {
				return unboundedLen
			}
			total += b
			if total > MaxSliceBound {
				return unboundedLen
			}
		}
		return total
	}
	// VALUE and SUBJECT depend on the input and on the canonicalizer that ran
	// before the program, so no static bound exists.
	return unboundedLen
}

// checkCategory enforces the per program kind operation categories of section 2
// of ir.md, plus the restrictions on SUBJECT and on WHEN.
func (v *validator) checkCategory(p *Program, i int, n *Node) error {
	cat := n.Op.Category()
	allowed := false
	for _, c := range categoriesByProgramKind[p.Kind] {
		if c == cat {
			allowed = true
			break
		}
	}
	if !allowed {
		return invalidf("program %d node %d uses %s, which a %s program may not contain", p.ID, i, n.Op, p.Kind)
	}
	if cat == CatCall && n.Op != callOpForProgram[p.Kind] {
		return invalidf("program %d node %d uses %s inside a %s program", p.ID, i, n.Op, p.Kind)
	}
	if n.Op == OpSubject && p.Kind == ProgramCanonicalization {
		return invalidf("program %d node %d uses subject() inside a canonicalization program", p.ID, i)
	}
	if v.preCanonicalizers[p.ID] && !preCanonicalizationOps[n.Op] {
		// A routing pre-canonicalizer can never add, replace or interpret a
		// prefix, so it is restricted to five operations.
		return invalidf("program %d node %d uses %s in a pre-canonicalization program", p.ID, i, n.Op)
	}
	if n.Op == OpChecksumWhen {
		// A WHEN branch is only observable as a direct operand of CHOOSE.
		for k := i + 1; k < len(p.Nodes); k++ {
			consumer := p.Nodes[k]
			for _, in := range consumer.InputNodes {
				if int(in) == i && consumer.Op != OpChoose {
					return invalidf("program %d node %d is a WHEN branch consumed by %s instead of CHOOSE",
						p.ID, i, consumer.Op)
				}
			}
		}
	}
	return nil
}

// checkRoot enforces the accepted root of each program kind.
func (v *validator) checkRoot(p *Program) error {
	root := p.Nodes[p.RootNode]
	switch p.Kind {
	case ProgramCanonicalization:
		if root.Op != OpCanonSequence {
			return invalidf("canonicalization program %d has root %s instead of a step sequence", p.ID, root.Op)
		}
	case ProgramFormat:
		if root.Op != OpAssertSequence {
			return invalidf("format program %d has root %s instead of an assertion sequence", p.ID, root.Op)
		}
	case ProgramChecksum:
		if root.OutputType != ValueChecksumOutcome {
			return invalidf("checksum program %d has a root that does not produce a checksum outcome", p.ID)
		}
		if root.Op == OpChecksumWhen {
			return invalidf("checksum program %d has a WHEN branch as its root", p.ID)
		}
	}
	return nil
}

// checkIdentifiers runs checks 17 and 18.
//
//nolint:gocyclo // One ordered pass over the definitions, mirroring checks 17 and 18.
func (v *validator) checkIdentifiers() error {
	b := v.bundle
	if len(b.Identifiers) > MaxIdentifiers {
		return invalidf("the bundle declares %d identifiers, the limit is %d", len(b.Identifiers), MaxIdentifiers)
	}
	b.identifierByID = make(map[uint32]*IdentifierDefinition, len(b.Identifiers))
	for i, d := range b.Identifiers {
		// 17. identifier ids unique, kinds and countries well formed,
		// serialization order respected
		if d.ID == 0 {
			return invalidf("identifier at index %d has id 0", i)
		}
		if _, dup := b.identifierByID[d.ID]; dup {
			return invalidf("identifier id %d appears twice", d.ID)
		}
		b.identifierByID[d.ID] = d

		if !validKindToken(d.Kind) {
			return invalidf("identifier %d has the malformed kind %q", d.ID, d.Kind)
		}
		if d.HasCountryCode && !validCountryToken(d.CountryCode) {
			return invalidf("identifier %d has the malformed country code %q", d.ID, d.CountryCode)
		}
		if i > 0 && !identifierOrderBefore(b.Identifiers[i-1], d) {
			return invalidf("identifiers are not in the normative order at index %d", i)
		}
		if !validProfile(d.DefaultProfile) {
			return invalidf("identifier %d declares the unknown profile %q", d.ID, d.DefaultProfile)
		}
		v.use(FeatureProfiles)

		canon := b.programByID[d.CanonicalizationProgram]
		if canon == nil || canon.Kind != ProgramCanonicalization {
			return invalidf("identifier %d references %d as a canonicalization program",
				d.ID, d.CanonicalizationProgram)
		}
		if !d.HasCountryCode && programUses(b, canon, OpPrependCountryIfMissing) {
			return invalidf("identifier %d is GLOBAL but its canonicalizer prepends a country", d.ID)
		}
		format := b.programByID[d.FormatProgram]
		if format == nil || format.Kind != ProgramFormat {
			return invalidf("identifier %d references %d as a format program", d.ID, d.FormatProgram)
		}

		// 18. exactly one checksum program or one absence reason per definition
		switch {
		case d.HasChecksumProgram && d.HasAbsentChecksumReason:
			return invalidf("identifier %d declares both a checksum program and an absence reason", d.ID)
		case d.HasChecksumProgram:
			checksum := b.programByID[d.ChecksumProgram]
			if checksum == nil || checksum.Kind != ProgramChecksum {
				return invalidf("identifier %d references %d as a checksum program", d.ID, d.ChecksumProgram)
			}
		case d.HasAbsentChecksumReason:
			if !d.AbsentChecksumReason.absentChecksumReason() {
				return invalidf("identifier %d declares the absence reason %s", d.ID, d.AbsentChecksumReason)
			}
			v.use(FeatureChecksumTristate)
		default:
			return invalidf("identifier %d declares neither a checksum program nor an absence reason", d.ID)
		}

		for k, src := range d.Sources {
			if src.ID == "" {
				return invalidf("identifier %d has an unnamed source at index %d", d.ID, k)
			}
			if k > 0 && src.ID <= d.Sources[k-1].ID {
				return invalidf("identifier %d does not sort its sources by id at index %d", d.ID, k)
			}
			// Capability 41. tier is not optional in the schema, so a source
			// that omits the field and one that sets UNSPECIFIED are the same
			// bytes: UNSPECIFIED means the source states no tier, and only a
			// stated one requires the capability. Refusing it would make 41
			// mandatory the moment 40 is, which is the opposite of the
			// independence a separate id exists to give.
			switch src.Tier {
			case TierUnspecified:
			case TierPrimary, TierSecondary:
				v.use(FeatureProvenanceTier)
			default:
				return invalidf("source %q of identifier %d states the tier %d, which is outside the enumeration",
					src.ID, d.ID, src.Tier)
			}
		}
		if len(d.Sources) > 0 {
			v.use(FeatureProvenance)
		}
	}
	return nil
}

// identifierOrderBefore reports whether prev sorts strictly before next in the
// normative order: kind, then GLOBAL first, then country code.
func identifierOrderBefore(prev, next *IdentifierDefinition) bool {
	if prev.Kind != next.Kind {
		return prev.Kind < next.Kind
	}
	if prev.HasCountryCode != next.HasCountryCode {
		return !prev.HasCountryCode
	}
	return prev.CountryCode < next.CountryCode
}

// programUses reports whether the program, or anything it calls, contains the
// given opcode.
func programUses(b *Bundle, p *Program, op Opcode) bool {
	for _, n := range p.Nodes {
		if n.Op == op {
			return true
		}
		if n.Op == OpCallFormat || n.Op == OpCallChecksum {
			if callee := b.programByID[n.ProgramID]; callee != nil && callee != p && programUses(b, callee, op) {
				return true
			}
		}
	}
	return false
}

// checkDispatchers runs checks 19 to 23.
//
//nolint:gocyclo // The dispatcher invariants are five ordered checks over the same structure; keeping them together mirrors ir.md section 10.
func (v *validator) checkDispatchers() error {
	b := v.bundle
	if len(b.Dispatchers) > 0 {
		v.use(FeatureIdentifierDispatch)
	}

	// 19. dispatcher kinds and aliases globally unique, sorted, never ambiguous
	kindSpace := make(map[string]string, len(b.Dispatchers))
	claim := func(token, owner string) error {
		if prev, dup := kindSpace[token]; dup {
			return invalidf("kind token %q is claimed by both %q and %q", token, prev, owner)
		}
		kindSpace[token] = owner
		return nil
	}
	for i, d := range b.Dispatchers {
		if !validKindToken(d.Kind) {
			return invalidf("dispatcher %q has a malformed kind", d.Kind)
		}
		if i > 0 && d.Kind <= b.Dispatchers[i-1].Kind {
			return invalidf("dispatchers are not sorted by kind at index %d", i)
		}
		if err := claim(d.Kind, d.Kind); err != nil {
			return err
		}
		for k, alias := range d.KindAliases {
			if !validKindToken(alias) {
				return invalidf("dispatcher %q declares the malformed kind alias %q", d.Kind, alias)
			}
			if k > 0 && alias <= d.KindAliases[k-1] {
				return invalidf("dispatcher %q does not sort its kind aliases at index %d", d.Kind, k)
			}
			if err := claim(alias, d.Kind); err != nil {
				return err
			}
		}

		pre := b.programByID[d.PreCanonicalizationProgram]
		if pre == nil || pre.Kind != ProgramCanonicalization {
			return invalidf("dispatcher %q references %d as a pre-canonicalization program",
				d.Kind, d.PreCanonicalizationProgram)
		}

		if err := v.checkTargets(d); err != nil {
			return err
		}
	}

	// 23. every definition referenced by exactly one dispatch target
	owner := make(map[uint32]string, len(b.Identifiers))
	for _, d := range b.Dispatchers {
		for _, t := range d.Targets {
			def := b.identifierByID[t.IdentifierDefinitionID]
			if def == nil {
				return invalidf("dispatcher %q targets the unknown definition %d", d.Kind, t.IdentifierDefinitionID)
			}
			if prev, dup := owner[def.ID]; dup {
				return invalidf("definition %d is targeted by both %q and %q", def.ID, prev, d.Kind)
			}
			owner[def.ID] = d.Kind
			if def.Kind != d.Kind {
				return invalidf("dispatcher %q targets definition %d of kind %q", d.Kind, def.ID, def.Kind)
			}
			if def.HasCountryCode != t.HasCountryCode || def.CountryCode != t.CountryCode {
				return invalidf("dispatcher %q target and definition %d disagree on the country", d.Kind, def.ID)
			}
			t.Definition = def
		}
	}
	for _, def := range b.Identifiers {
		if _, ok := owner[def.ID]; !ok {
			return invalidf("definition %d is referenced by no dispatch target", def.ID)
		}
	}
	return nil
}

//nolint:gocyclo // Checks 19 to 21 all constrain one dispatcher's targets.
func (v *validator) checkTargets(d *IdentifierDispatcher) error {
	if len(d.Targets) == 0 {
		return invalidf("dispatcher %q declares no target", d.Kind)
	}
	byCountry := make(map[string]bool, len(d.Targets))
	byPrefix := make(map[string]string, len(d.Targets))

	// 21. targets sorted, unique per country, prefixes claimed by at most one
	// 22. GLOBAL targets alone, without prefix and without country alias
	for i, t := range d.Targets {
		if i > 0 && !targetOrderBefore(d.Targets[i-1], t) {
			return invalidf("dispatcher %q does not sort its targets at index %d", d.Kind, i)
		}
		if !t.HasCountryCode {
			if len(d.Targets) != 1 {
				return invalidf("dispatcher %q mixes a GLOBAL target with country targets", d.Kind)
			}
			if len(t.AcceptedPrefixes) > 0 || t.HasCanonicalPrefix {
				return invalidf("dispatcher %q declares a prefix on its GLOBAL target", d.Kind)
			}
			if len(d.CountryAliases) > 0 {
				return invalidf("dispatcher %q declares country aliases beside a GLOBAL target", d.Kind)
			}
			d.Global = t
			continue
		}
		if !validCountryToken(t.CountryCode) {
			return invalidf("dispatcher %q declares the malformed country %q", d.Kind, t.CountryCode)
		}
		if byCountry[t.CountryCode] {
			return invalidf("dispatcher %q declares two targets for country %q", d.Kind, t.CountryCode)
		}
		byCountry[t.CountryCode] = true

		for k, prefix := range t.AcceptedPrefixes {
			if !validPrefixToken(prefix) {
				return invalidf("dispatcher %q declares the malformed prefix %q", d.Kind, prefix)
			}
			if k > 0 && prefix <= t.AcceptedPrefixes[k-1] {
				return invalidf("dispatcher %q does not sort the prefixes of target %q", d.Kind, t.CountryCode)
			}
			if prev, dup := byPrefix[prefix]; dup {
				return invalidf("dispatcher %q lets prefix %q designate both %q and %q",
					d.Kind, prefix, prev, t.CountryCode)
			}
			byPrefix[prefix] = t.CountryCode
		}
		if t.HasCanonicalPrefix && !validPrefixToken(t.CanonicalPrefix) {
			return invalidf("dispatcher %q declares the malformed canonical prefix %q", d.Kind, t.CanonicalPrefix)
		}
		if t.AllowUnprefixedWithoutCountry {
			if d.Unprefixed != nil {
				return invalidf("dispatcher %q has two targets selectable without a country", d.Kind)
			}
			d.Unprefixed = t
		}
	}

	// 20. country aliases sorted, unique, never self mapping, never shadowing
	for i, alias := range d.CountryAliases {
		if !validCountryToken(alias.Alias) {
			return invalidf("dispatcher %q declares the malformed country alias %q", d.Kind, alias.Alias)
		}
		if !validCountryToken(alias.CountryCode) {
			return invalidf("dispatcher %q maps %q onto the malformed country %q",
				d.Kind, alias.Alias, alias.CountryCode)
		}
		if i > 0 && alias.Alias <= d.CountryAliases[i-1].Alias {
			return invalidf("dispatcher %q does not sort its country aliases at index %d", d.Kind, i)
		}
		if alias.Alias == alias.CountryCode {
			return invalidf("dispatcher %q maps country %q onto itself", d.Kind, alias.Alias)
		}
		if byCountry[alias.Alias] {
			return invalidf("dispatcher %q aliases %q, which is already a target country", d.Kind, alias.Alias)
		}
	}
	return nil
}

// targetOrderBefore reports whether prev sorts strictly before next: GLOBAL
// first, then country code.
func targetOrderBefore(prev, next *DispatchTarget) bool {
	if prev.HasCountryCode != next.HasCountryCode {
		return !prev.HasCountryCode
	}
	return prev.CountryCode < next.CountryCode
}

// checkCallGraph runs check 24.
func (v *validator) checkCallGraph() error {
	b := v.bundle
	const (
		white = 0 // not visited
		grey  = 1 // on the current path
		black = 2 // fully explored
	)
	colour := make(map[uint32]int, len(b.Programs))
	depth := make(map[uint32]int, len(b.Programs))

	var visit func(p *Program) (int, error)
	visit = func(p *Program) (int, error) {
		switch colour[p.ID] {
		case grey:
			return 0, invalidf("the call graph contains a cycle through program %d", p.ID)
		case black:
			return depth[p.ID], nil
		}
		colour[p.ID] = grey
		best := 1
		for i, n := range p.Nodes {
			if n.Op != OpCallFormat && n.Op != OpCallChecksum {
				continue
			}
			callee := b.programByID[n.ProgramID]
			if callee == nil {
				return 0, invalidf("program %d node %d calls the unknown program %d", p.ID, i, n.ProgramID)
			}
			if callee.Kind != p.Kind {
				return 0, invalidf("program %d node %d calls program %d of kind %s", p.ID, i, callee.ID, callee.Kind)
			}
			d, err := visit(callee)
			if err != nil {
				return 0, err
			}
			best = max(best, d+1)
		}
		colour[p.ID] = black
		depth[p.ID] = best
		if best > MaxCallDepth {
			return 0, invalidf("program %d reaches a static call depth of %d, the limit is %d",
				p.ID, best, MaxCallDepth)
		}
		return best, nil
	}

	for _, p := range b.Programs {
		if _, err := visit(p); err != nil {
			return err
		}
	}
	return nil
}

// validKindToken matches [a-z][a-z0-9_-]{0,63}.
func validKindToken(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// validCountryToken matches exactly [A-Z]{2}.
func validCountryToken(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}

// validPrefixToken matches 1 to 8 ASCII alphanumeric characters.
func validPrefixToken(s string) bool {
	if len(s) == 0 || len(s) > 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

// paramList renders a parameter set for a diagnostic.
func paramList(set param) string {
	out := ""
	for bit := param(1); bit != 0; bit <<= 1 {
		name, ok := paramNames[bit]
		if set&bit == 0 || !ok {
			continue
		}
		if out != "" {
			out += ", "
		}
		out += name
	}
	if out == "" {
		return "an unknown parameter"
	}
	return out
}

// checkRulesVersion is check 6 of section 10. The shape matters beyond the
// bundle: the version reaches generated sources, manifests and logs in every
// engine, so anything an identifier cannot carry is refused here.
func checkRulesVersion(s string) error {
	if s == "" {
		return invalidf("rules_version is empty")
	}
	if len(s) > MaxRulesVersionBytes {
		return invalidf("rules_version holds %d bytes, at most %d are allowed", len(s), MaxRulesVersionBytes)
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '.', c == '-', c == '_':
		default:
			return invalidf("rules_version carries %q, which is outside the allowed set", c)
		}
	}
	return nil
}

// checkAlphabet validates the ordered code points of CHAR_MAPPING_CUSTOM_ALPHABET.
//
// A repeated code point is the reason this is a load time refusal rather than a
// runtime tolerance: it would have two values at once, and which one wins would
// depend on how an implementation happens to search. Two conforming engines
// would then disagree on a checksum without either being wrong, which is the one
// divergence a conformance corpus cannot catch.
func checkAlphabet(alphabet string, has bool) error {
	if !has {
		return errors.New("weighted_sum reads a custom alphabet but carries none")
	}
	if alphabet == "" {
		return errors.New("the custom alphabet is empty")
	}
	// The decoder already proved the string is valid UTF-8, as proto3 requires.
	seen := make(map[rune]bool, len(alphabet))
	count := 0
	for _, r := range alphabet {
		if seen[r] {
			return fmt.Errorf("the custom alphabet repeats %q", r)
		}
		seen[r] = true
		count++
	}
	if count > MaxAlphabetPoints {
		return fmt.Errorf("the custom alphabet holds %d code points, the limit is %d", count, MaxAlphabetPoints)
	}
	return nil
}

// expansionOf counts the operation instances a generated program holds once
// every repeated operand is inlined: one per path from the root to a node,
// which is 1 + the sum over the operands.
//
// The node count of a program is bounded and its graph is acyclic, but a DAG
// whose every node reads the previous one twice doubles at each level, so the
// result is counted with saturating arithmetic. An int64 would overflow into a
// small number that passes, which is the shape of the attack rather than an
// edge case.
//
// A call counts as one instance: the callee is a program of its own, emitted
// once and reached through a function call, so it is counted under its own id.
//
// The emission roots are the root node, the subject node when the program
// declares one, and every capture no other root already reaches. A node no
// root reaches is emitted by nobody and counts for nothing, and a capture the
// root already reaches is not a second emission: it is the same subtree under
// a name, so counting it twice would report instances the generator never
// writes.
//
// Their costs are summed, because a generator emits all of them. Checking each
// root on its own would let a program carry any number of roots just below the
// ceiling.
func expansionOf(p *Program) int64 {
	instances := make([]int64, len(p.Nodes))
	for i, n := range p.Nodes {
		// Operand indices are strictly lower, which check 11 proved, so every
		// operand already holds its count.
		total := int64(1)
		for _, in := range n.InputNodes {
			total = saturatingAdd(total, instances[in])
		}
		instances[i] = total
	}

	covered := make([]bool, len(p.Nodes))
	total := int64(0)
	reach := func(from uint32) {
		if covered[from] {
			return
		}
		total = saturatingAdd(total, instances[from])
		// Operands are strictly lower, so one descending sweep marks the whole
		// subtree without recursion.
		covered[from] = true
		for i := int(from); i >= 0; i-- {
			if !covered[i] {
				continue
			}
			for _, in := range p.Nodes[i].InputNodes {
				covered[in] = true
			}
		}
	}
	reach(p.RootNode)
	if p.HasSubject && int(p.SubjectNode) < len(p.Nodes) {
		reach(p.SubjectNode)
	}
	for _, c := range p.Captures {
		if int(c.Node) < len(p.Nodes) {
			reach(c.Node)
		}
	}
	return total
}

// saturatingAdd stops at one past the budget rather than wrapping. Anything
// above the budget is refused, so the exact value beyond it carries no meaning.
func saturatingAdd(a, b int64) int64 {
	const ceiling = int64(MaxSteps) + 1
	if a >= ceiling || b >= ceiling || a > ceiling-b {
		return ceiling
	}
	return a + b
}

// describeExpansion renders a count the saturating walk may have capped.
func describeExpansion(n int64) string {
	if n > MaxSteps {
		return fmt.Sprintf("more than %d", MaxSteps)
	}
	return fmt.Sprintf("%d", n)
}
