// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

// decodeBundle decodes a RuleBundle body. Every field outside the V1 schema is
// refused at any depth, as section 7.1 of the specification requires.
func decodeBundle(r *reader, b *Bundle) error {
	const msg = "RuleBundle"
	var s seen
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		switch field {
		case 1: // format_version
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			if b.FormatVersion, err = r.u32(); err != nil {
				return err
			}
		case 2: // rules_version
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			if b.RulesVersion, err = r.str(); err != nil {
				return err
			}
		case 3: // required_feature_ids
			if b.RequiredFeatureIDs, err = r.packedU32(b.RequiredFeatureIDs, wire); err != nil {
				return err
			}
		case 4: // source_digest
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			raw, err := r.bytes()
			if err != nil {
				return err
			}
			b.SourceDigest = append([]byte(nil), raw...)
		case 6: // identifiers
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			def := &IdentifierDefinition{}
			if err := r.message(func(sub *reader) error { return decodeIdentifier(sub, def) }); err != nil {
				return err
			}
			b.Identifiers = append(b.Identifiers, def)
		case 7: // programs
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			p := &Program{}
			if err := r.message(func(sub *reader) error { return decodeProgram(sub, p) }); err != nil {
				return err
			}
			b.Programs = append(b.Programs, p)
		case 8: // dispatchers
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			d := &IdentifierDispatcher{}
			if err := r.message(func(sub *reader) error { return decodeDispatcher(sub, d) }); err != nil {
				return err
			}
			b.Dispatchers = append(b.Dispatchers, d)
		default:
			// Field 5 is reserved for the removed generated_at and must never
			// come back; every other number is simply unknown.
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeIdentifier(r *reader, d *IdentifierDefinition) error {
	const msg = "IdentifierDefinition"
	var s seen
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		switch field {
		case 1:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.ID, err = r.u32()
		case 2:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.Kind, err = r.str()
		case 3:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.CountryCode, err = r.str()
			d.HasCountryCode = true
		case 4:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.CanonicalizationProgram, err = r.u32()
		case 5:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.FormatProgram, err = r.u32()
		case 6:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.ChecksumProgram, err = r.u32()
			d.HasChecksumProgram = true
		case 7:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.DefaultProfile, err = r.str()
		case 8:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			var src Source
			if err := r.message(func(sub *reader) error { return decodeSource(sub, &src) }); err != nil {
				return err
			}
			d.Sources = append(d.Sources, src)
		case 9:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			var code int32
			if code, err = r.enum(); err == nil {
				d.AbsentChecksumReason = ReasonCode(code)
				d.HasAbsentChecksumReason = true
			}
		default:
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeSource(r *reader, src *Source) error {
	const msg = "Source"
	var s seen
	targets := map[int]*string{
		1: &src.ID, 2: &src.URL, 3: &src.Authority, 4: &src.Title,
		5: &src.AccessedAt, 6: &src.Jurisdiction, 7: &src.Language,
		8: &src.Notes, 9: &src.LicenseOrTerms, 10: &src.ArchiveURL,
	}
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		if field == 11 { // tier, capability 41
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			tier, err := r.enum()
			if err != nil {
				return err
			}
			src.Tier = SourceTier(tier)
			continue
		}
		dst, ok := targets[field]
		if !ok {
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
			continue
		}
		if wire != wireBytes {
			return wrongWire(msg, field, wire)
		}
		if err := s.mark(msg, field); err != nil {
			return err
		}
		if *dst, err = r.str(); err != nil {
			return err
		}
		if field == 10 {
			src.HasArchiveURL = true
		}
	}
	return nil
}

func decodeDispatcher(r *reader, d *IdentifierDispatcher) error {
	const msg = "IdentifierDispatcher"
	var s seen
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		switch field {
		case 1:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.Kind, err = r.str()
		case 2:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			var alias string
			if alias, err = r.str(); err == nil {
				d.KindAliases = append(d.KindAliases, alias)
			}
		case 3:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			d.PreCanonicalizationProgram, err = r.u32()
		case 4:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			var ca CountryAlias
			if err := r.message(func(sub *reader) error { return decodeCountryAlias(sub, &ca) }); err != nil {
				return err
			}
			d.CountryAliases = append(d.CountryAliases, ca)
		case 5:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			t := &DispatchTarget{}
			if err := r.message(func(sub *reader) error { return decodeTarget(sub, t) }); err != nil {
				return err
			}
			d.Targets = append(d.Targets, t)
		default:
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeCountryAlias(r *reader, ca *CountryAlias) error {
	const msg = "CountryAlias"
	var s seen
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		if wire != wireBytes {
			return wrongWire(msg, field, wire)
		}
		if err := s.mark(msg, field); err != nil {
			return err
		}
		switch field {
		case 1:
			ca.Alias, err = r.str()
		case 2:
			ca.CountryCode, err = r.str()
		default:
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeTarget(r *reader, t *DispatchTarget) error {
	const msg = "DispatchTarget"
	var s seen
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		switch field {
		case 1:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			t.CountryCode, err = r.str()
			t.HasCountryCode = true
		case 2:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			var prefix string
			if prefix, err = r.str(); err == nil {
				t.AcceptedPrefixes = append(t.AcceptedPrefixes, prefix)
			}
		case 3:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			t.CanonicalPrefix, err = r.str()
			t.HasCanonicalPrefix = true
		case 4:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			t.IdentifierDefinitionID, err = r.u32()
		case 5:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			t.AllowUnprefixedWithoutCountry, err = r.boolean()
		default:
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeProgram(r *reader, p *Program) error {
	const msg = "Program"
	var s seen
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		switch field {
		case 1:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			p.ID, err = r.u32()
		case 2:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			var kind int32
			if kind, err = r.enum(); err == nil {
				p.Kind = ProgramKind(kind)
			}
		case 3:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if len(p.Nodes) >= MaxNodesPerProgram {
				return malformed("more than %d nodes in a program", MaxNodesPerProgram)
			}
			n := &Node{}
			if err := r.message(func(sub *reader) error { return decodeNode(sub, n) }); err != nil {
				return err
			}
			p.Nodes = append(p.Nodes, n)
		case 4:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			p.RootNode, err = r.u32()
		case 5:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			var c Capture
			if err := r.message(func(sub *reader) error { return decodeCapture(sub, &c) }); err != nil {
				return err
			}
			p.Captures = append(p.Captures, c)
		case 6:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			p.SubjectNode, err = r.u32()
			p.HasSubject = true
		default:
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeCapture(r *reader, c *Capture) error {
	const msg = "Capture"
	var s seen
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		if err := s.mark(msg, field); err != nil {
			return err
		}
		switch field {
		case 1:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			c.Name, err = r.str()
		case 2:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			c.Node, err = r.u32()
		default:
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeNode(r *reader, n *Node) error {
	const msg = "Node"
	var s seen
	// operations counts how many branches of the oneof the body carried.
	operations := 0
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		switch field {
		case 1:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			var t int32
			if t, err = r.enum(); err == nil {
				n.OutputType = ValueType(t)
			}
		case 2:
			if n.InputNodes, err = r.packedU32(n.InputNodes, wire); err != nil {
				return err
			}
			if len(n.InputNodes) > MaxNodesPerProgram {
				return malformed("more than %d operands on a node", MaxNodesPerProgram)
			}
		case 10, 11, 12, 13, 14, 15, 16:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			operations++
			if err := r.message(func(sub *reader) error { return decodeOperation(sub, field, n) }); err != nil {
				return err
			}
		default:
			if err := r.unknown(msg, field, wire); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	// A oneof carrying two branches is ambiguous: which one an engine keeps
	// would depend on its Protobuf runtime, so the node is refused outright.
	if operations != 1 {
		return malformed("node carries %d operations, exactly one is required", operations)
	}
	return nil
}

// operationField maps the oneof field number to its per category enum table.
var operationField = map[int]struct {
	msg   string
	table map[int32]Opcode
}{
	10: {"StringOperation", stringOps},
	11: {"IntegerOperation", integerOps},
	12: {"PredicateOperation", predicateOps},
	13: {"CanonicalizationOperation", canonicalizationOps},
	14: {"AssertionOperation", assertionOps},
	15: {"ChecksumOperation", checksumOps},
	16: {"CallOperation", callOps},
}

// decodeOperation decodes one operation message into the flat node parameters.
// The kind enum is field 1 of every operation message, but it may appear after
// the parameters on the wire, so the opcode is resolved once the body is read.
func decodeOperation(r *reader, oneof int, n *Node) error {
	meta := operationField[oneof]
	var s seen
	var kind int32
	haveKind := false
	for !r.done() {
		field, wire, err := r.next()
		if err != nil {
			return err
		}
		if field == 1 {
			if wire != wireVarint {
				return wrongWire(meta.msg, field, wire)
			}
			if err := s.mark(meta.msg, field); err != nil {
				return err
			}
			if kind, err = r.enum(); err != nil {
				return err
			}
			haveKind = true
			continue
		}
		if err := decodeOperationParam(r, meta.msg, oneof, field, wire, &s, n); err != nil {
			return err
		}
	}
	if !haveKind {
		return malformed("%s has no kind", meta.msg)
	}
	op, ok := meta.table[kind]
	if !ok {
		if r.lenient {
			// Check 10 comes after the version checks, so the lenient pass
			// leaves the operation unresolved and lets the strict pass name it.
			return nil
		}
		// Check 10 of ir.md section 10 is structural, so an unknown operation is
		// invalid_ruleset rather than incompatible_ruleset; only an unsupported
		// format version and an unknown capability id carry the latter.
		return invalidf("%s kind %d is unknown to this generator", meta.msg, kind)
	}
	n.Op = op
	return nil
}

//nolint:gocyclo // One flat switch per operation message keeps the field to parameter mapping auditable against rules.proto.
func decodeOperationParam(r *reader, msg string, oneof, field, wire int, s *seen, n *Node) error {
	str := func(dst *string, has *bool) error {
		if wire != wireBytes {
			return wrongWire(msg, field, wire)
		}
		if err := s.mark(msg, field); err != nil {
			return err
		}
		v, err := r.str()
		if err != nil {
			return err
		}
		*dst, *has = v, true
		return nil
	}
	u32 := func(dst *uint32, has *bool) error {
		if wire != wireVarint {
			return wrongWire(msg, field, wire)
		}
		if err := s.mark(msg, field); err != nil {
			return err
		}
		v, err := r.u32()
		if err != nil {
			return err
		}
		*dst, *has = v, true
		return nil
	}
	i64 := func(dst *int64, has *bool) error {
		if wire != wireVarint {
			return wrongWire(msg, field, wire)
		}
		if err := s.mark(msg, field); err != nil {
			return err
		}
		v, err := r.i64()
		if err != nil {
			return err
		}
		*dst, *has = v, true
		return nil
	}
	enum := func(apply func(int32)) error {
		if wire != wireVarint {
			return wrongWire(msg, field, wire)
		}
		if err := s.mark(msg, field); err != nil {
			return err
		}
		v, err := r.enum()
		if err != nil {
			return err
		}
		apply(v)
		return nil
	}

	switch oneof {
	case 10: // StringOperation
		switch field {
		case 2:
			return str(&n.Text, &n.HasText)
		case 3:
			return u32(&n.Start, &n.HasStart)
		case 4:
			return u32(&n.End, &n.HasEnd)
		}
	case 11: // IntegerOperation
		switch field {
		case 2:
			if wire != wireVarint {
				return wrongWire(msg, field, wire)
			}
			if err := s.mark(msg, field); err != nil {
				return err
			}
			v, err := r.i64()
			if err != nil {
				return err
			}
			n.Modulus, n.HasModulus = v, true
			return nil
		case 3:
			var err error
			if n.Weights, err = r.packedI64(n.Weights, wire); err != nil {
				return err
			}
			if len(n.Weights) > MaxWeights {
				return malformed("more than %d weights", MaxWeights)
			}
			return nil
		case 4:
			return enum(func(v int32) { n.Alignment, n.HasAlignment = WeightAlignment(v), true })
		case 5:
			return enum(func(v int32) { n.Mapping, n.HasMapping = CharMapping(v), true })
		case 6:
			var err error
			if n.RemainderValues, err = r.packedI64(n.RemainderValues, wire); err != nil {
				return err
			}
			if len(n.RemainderValues) > MaxRemainderValues {
				return malformed("more than %d remainder values", MaxRemainderValues)
			}
			return nil
		case 7:
			return str(&n.Alphabet, &n.HasAlphabet)
		}
	case 12: // PredicateOperation
		switch field {
		case 2:
			return str(&n.Text, &n.HasText)
		case 3:
			if wire != wireBytes {
				return wrongWire(msg, field, wire)
			}
			v, err := r.str()
			if err != nil {
				return err
			}
			n.Values = append(n.Values, v)
			if len(n.Values) > MaxNodesPerProgram {
				return malformed("more than %d prefix values", MaxNodesPerProgram)
			}
			return nil
		case 4:
			var err error
			if n.Lengths, err = r.packedU32(n.Lengths, wire); err != nil {
				return err
			}
			if len(n.Lengths) > MaxNodesPerProgram {
				return malformed("more than %d lengths", MaxNodesPerProgram)
			}
			return nil
		case 5:
			return u32(&n.Length, &n.HasLength)
		case 6:
			return u32(&n.MinLength, &n.HasMinLength)
		case 7:
			return u32(&n.MaxLength, &n.HasMaxLength)
		case 8:
			return u32(&n.Index, &n.HasIndex)
		case 9:
			return i64(&n.Constant, &n.HasConstant)
		}
	case 13: // CanonicalizationOperation
		switch field {
		case 2:
			return str(&n.Text, &n.HasText)
		case 3:
			return str(&n.Replacement, &n.HasReplacement)
		case 4:
			return u32(&n.Index, &n.HasIndex)
		case 5:
			return u32(&n.Length, &n.HasLength)
		}
	case 14: // AssertionOperation
		switch field {
		case 2:
			return enum(func(v int32) { n.ReasonCode, n.HasReasonCode = ReasonCode(v), true })
		case 3:
			return str(&n.MessageKey, &n.HasMessageKey)
		}
	case 15: // ChecksumOperation
		switch field {
		case 2:
			return u32(&n.Index, &n.HasIndex)
		case 3:
			return u32(&n.Start, &n.HasStart)
		case 4:
			return u32(&n.End, &n.HasEnd)
		case 5:
			return enum(func(v int32) { n.ReasonCode, n.HasReasonCode = ReasonCode(v), true })
		case 6:
			return str(&n.MessageKey, &n.HasMessageKey)
		case 7:
			return i64(&n.Constant, &n.HasConstant)
		}
	case 16: // CallOperation
		if field == 2 {
			var has bool
			return u32(&n.ProgramID, &has)
		}
	}
	return r.unknown(msg, field, wire)
}

// present returns the set of parameters the node actually carries.
func (n *Node) present() param {
	var p param
	set := func(cond bool, bit param) {
		if cond {
			p |= bit
		}
	}
	set(n.HasText, pText)
	set(n.HasReplacement, pReplacement)
	set(n.HasStart, pStart)
	set(n.HasEnd, pEnd)
	set(n.HasIndex, pIndex)
	set(n.HasLength, pLength)
	set(n.HasMinLength, pMinLength)
	set(n.HasMaxLength, pMaxLength)
	set(len(n.Values) > 0, pValues)
	set(len(n.Lengths) > 0, pLengths)
	set(n.HasModulus, pModulus)
	set(len(n.Weights) > 0, pWeights)
	set(n.HasAlignment, pAlignment)
	set(n.HasMapping, pMapping)
	set(n.HasAlphabet, pAlphabet)
	set(len(n.RemainderValues) > 0, pRemainderValues)
	set(n.HasReasonCode, pReasonCode)
	set(n.HasMessageKey, pMessageKey)
	set(n.ProgramID != 0, pProgramID)
	set(n.HasConstant, pConstant)
	return p
}
