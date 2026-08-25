// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"math"
	"unicode/utf8"
)

// This file encodes rule bundles byte by byte so that the tests can exercise
// operations the shipped rules never use. It is deliberately unconstrained: it
// will emit a graph violating any invariant, because that is what the refusal
// paths need.

// Wire enum values mirrored from spec/rules.proto.
const (
	tString    = 1
	tInteger   = 2
	tBoolean   = 3
	tCanonStep = 4
	tAssertion = 5
	tChecksum  = 6

	kCanon  = 1
	kFormat = 2
	kCheck  = 3
)

// String, integer, predicate, canonicalization, assertion, checksum and call
// operation kinds.
const (
	sConstant, sValue, sSubject, sCountry    = 1, 2, 3, 4
	sSlice, sSliceFrom, sSliceTo             = 5, 6, 7
	sBefore, sAfter, sStrip, sConcat         = 8, 9, 10, 11
	iDigits, iModDigits, iWeighted           = 1, 2, 3
	iModulo, iComplement, iRemainder         = 4, 5, 6
	pIsEmpty, pIsAbsent, pEquals             = 1, 2, 3
	pLenEq, pLenIn, pLenBetween              = 4, 5, 6
	pDigits, pUpper, pAlnum, pCharset        = 7, 8, 9, 10
	pStarts, pEnds, pPrefixIn                = 11, 12, 13
	pCharAt, pContains, pAll, pAny, pNot     = 14, 15, 16, 17, 18
	pProfile, pIntegerIs                     = 19, 20
	cSeq, cTrim, cRemoveWS, cUpper           = 1, 2, 3, 4
	cRemoveChars, cReplacePrefix             = 5, 6
	cPrepend, cAppend, cInsert, cLeftPad     = 7, 8, 9, 10
	cCountry, cWhen                          = 11, 12
	aSeq, aRequire                           = 1, 2
	xLuhn, xMod97, xCmpDigit, xCmpSlice      = 1, 2, 3, 4
	xChoose, xWhen, xAll, xAny, xUnsupported = 5, 6, 7, 8, 9
	xCmpConstant                             = 10
	callFormat, callChecksum                 = 1, 2
	alignLeft, alignRight, alignCycle        = 1, 2, 3
	mapDigit, mapBase36, mapCustom           = 1, 2, 3
	rcInvalidLength, rcInvalidChars          = 3, 4
	rcInvalidFormat, rcNotPublished          = 5, 13
)

// Oneof field numbers of Node.
const (
	oneofString = 10
	oneofInt    = 11
	oneofPred   = 12
	oneofCanon  = 13
	oneofAssert = 14
	oneofCheck  = 15
	oneofCall   = 16
)

// field is one operation parameter, resolved against the message that carries it.
type field struct {
	name string
	emit func(num int) []byte
}

var paramNumbers = map[int]map[string]int{
	oneofString: {"text": 2, "start": 3, "end": 4},
	oneofInt: {
		"modulus": 2, "weights": 3, "alignment": 4, "mapping": 5,
		"remainders": 6, "alphabet": 7,
	},
	oneofPred: {
		"text": 2, "values": 3, "lengths": 4, "length": 5,
		"min": 6, "max": 7, "index": 8, "constant": 9,
	},
	oneofCanon:  {"text": 2, "replacement": 3, "index": 4, "length": 5},
	oneofAssert: {"reason": 2, "key": 3},
	oneofCheck:  {"index": 2, "start": 3, "end": 4, "reason": 5, "key": 6, "constant": 7},
	oneofCall:   {"program": 2},
}

func text(s string) field { return field{"text", func(n int) []byte { return str(n, s) }} }
func replacement(s string) field {
	return field{"replacement", func(n int) []byte { return str(n, s) }}
}
func key(s string) field   { return field{"key", func(n int) []byte { return str(n, s) }} }
func start(v uint32) field { return field{"start", func(n int) []byte { return vfield(n, uint64(v)) }} }
func end(v uint32) field   { return field{"end", func(n int) []byte { return vfield(n, uint64(v)) }} }
func index(v uint32) field { return field{"index", func(n int) []byte { return vfield(n, uint64(v)) }} }
func length(v uint32) field {
	return field{"length", func(n int) []byte { return vfield(n, uint64(v)) }}
}
func minLen(v uint32) field { return field{"min", func(n int) []byte { return vfield(n, uint64(v)) }} }
func maxLen(v uint32) field { return field{"max", func(n int) []byte { return vfield(n, uint64(v)) }} }
func reason(v int) field    { return field{"reason", func(n int) []byte { return vfield(n, uint64(v)) }} }
func constant(v int64) field {
	return field{"constant", func(n int) []byte { return vfield(n, uint64(v)) }}
}
func program(v uint32) field {
	return field{"program", func(n int) []byte { return vfield(n, uint64(v)) }}
}
func modulus(v int64) field {
	return field{"modulus", func(n int) []byte { return vfield(n, uint64(v)) }}
}
func alignment(v int) field {
	return field{"alignment", func(n int) []byte { return vfield(n, uint64(v)) }}
}
func mapping(v int) field {
	return field{"mapping", func(n int) []byte { return vfield(n, uint64(v)) }}
}

func values(vs ...string) field {
	return field{"values", func(n int) []byte {
		var out []byte
		for _, v := range vs {
			out = append(out, str(n, v)...)
		}
		return out
	}}
}

func lengths(vs ...uint32) field {
	return field{"lengths", func(n int) []byte {
		var packed []byte
		for _, v := range vs {
			packed = uvarint(packed, uint64(v))
		}
		return bfield(n, packed)
	}}
}

func weights(vs ...int64) field    { return packedInts("weights", vs) }
func remainders(vs ...int64) field { return packedInts("remainders", vs) }

func packedInts(name string, vs []int64) field {
	return field{name, func(n int) []byte {
		var packed []byte
		for _, v := range vs {
			packed = uvarint(packed, uint64(v))
		}
		return bfield(n, packed)
	}}
}

// node describes one node of a program.
type node struct {
	out    int
	in     []uint32
	oneof  int
	kind   int
	fields []field
}

func nodeOf(oneof, kind, out int, in []uint32, f []field) node {
	return node{out: out, in: in, oneof: oneof, kind: kind, fields: f}
}

func sn(kind int, in []uint32, f ...field) node { return nodeOf(oneofString, kind, tString, in, f) }
func in64(kind int, in []uint32, f ...field) node {
	return nodeOf(oneofInt, kind, tInteger, in, f)
}
func pr(kind int, in []uint32, f ...field) node { return nodeOf(oneofPred, kind, tBoolean, in, f) }
func cn(kind int, in []uint32, f ...field) node {
	return nodeOf(oneofCanon, kind, tCanonStep, in, f)
}
func as(kind int, in []uint32, f ...field) node {
	return nodeOf(oneofAssert, kind, tAssertion, in, f)
}
func ck(kind int, in []uint32, f ...field) node {
	return nodeOf(oneofCheck, kind, tChecksum, in, f)
}
func call(kind, out int, in []uint32, f ...field) node {
	return nodeOf(oneofCall, kind, out, in, f)
}

func (n node) encode() []byte {
	body := vfield(1, uint64(n.out))
	for _, i := range n.in {
		body = append(body, vfield(2, uint64(i))...)
	}
	op := vfield(1, uint64(n.kind))
	numbers := paramNumbers[n.oneof]
	for _, f := range n.fields {
		op = append(op, f.emit(numbers[f.name])...)
	}
	return bfield(3, append(body, bfield(n.oneof, op)...))
}

// prog describes one program.
type prog struct {
	id    uint32
	kind  int
	nodes []node
	root  uint32
	// subject is only written when hasSubject is set, so that a test can tell
	// an absent subject_node from one naming node 0.
	subject    uint32
	hasSubject bool
	captures   []capture
}

// capture names a node of a format program.
type capture struct {
	name string
	node uint32
}

func (p prog) encode() []byte {
	body := append(vfield(1, uint64(p.id)), vfield(2, uint64(p.kind))...)
	for _, n := range p.nodes {
		body = append(body, n.encode()...)
	}
	body = append(body, vfield(4, uint64(p.root))...)
	for _, c := range p.captures {
		body = append(body, bfield(5, append(str(1, c.name), vfield(2, uint64(c.node))...))...)
	}
	if p.hasSubject {
		body = append(body, vfield(6, uint64(p.subject))...)
	}
	return bfield(7, body)
}

// def describes one identifier definition.
type def struct {
	id           uint32
	kind         string
	country      string // empty means GLOBAL
	canon        uint32
	format       uint32
	checksum     uint32
	hasChecksum  bool
	absentReason int
	tier         int
}

func (d def) encode() []byte {
	body := append(vfield(1, uint64(d.id)), str(2, d.kind)...)
	if d.country != "" {
		body = append(body, str(3, d.country)...)
	}
	body = append(body, vfield(4, uint64(d.canon))...)
	body = append(body, vfield(5, uint64(d.format))...)
	if d.hasChecksum {
		body = append(body, vfield(6, uint64(d.checksum))...)
	}
	body = append(body, str(7, "compatible")...)
	source := append(str(1, "test-source"), str(2, "https://example.invalid")...)
	if d.tier != 0 {
		source = append(source, vfield(11, uint64(d.tier))...)
	}
	body = append(body, bfield(8, source)...)
	if d.absentReason != 0 {
		body = append(body, vfield(9, uint64(d.absentReason))...)
	}
	return bfield(6, body)
}

// target is one routing entry.
type target struct {
	country         string
	prefixes        []string
	canonicalPrefix string
	definition      uint32
	unprefixed      bool
}

func (t target) encode() []byte {
	var body []byte
	if t.country != "" {
		body = append(body, str(1, t.country)...)
	}
	for _, p := range t.prefixes {
		body = append(body, str(2, p)...)
	}
	if t.canonicalPrefix != "" {
		body = append(body, str(3, t.canonicalPrefix)...)
	}
	body = append(body, vfield(4, uint64(t.definition))...)
	if t.unprefixed {
		body = append(body, vfield(5, 1)...)
	}
	return bfield(5, body)
}

// dispatch describes one dispatcher.
type dispatch struct {
	kind    string
	aliases []string
	pre     uint32
	targets []target
}

func (d dispatch) encode() []byte {
	body := str(1, d.kind)
	for _, a := range d.aliases {
		body = append(body, str(2, a)...)
	}
	body = append(body, vfield(3, uint64(d.pre))...)
	for _, t := range d.targets {
		body = append(body, t.encode()...)
	}
	return bfield(8, body)
}

// bundle describes a whole rule bundle.
type bundle struct {
	formatVersion uint32
	rulesVersion  string
	features      []uint32
	digest        []byte
	defs          []def
	programs      []prog
	dispatchers   []dispatch
	extra         []byte
}

// allFeatures is the full capability registry. Declaring more than a bundle
// uses is allowed; using one that is not declared is not.
var allFeatures = []uint32{1, 2, 3, 4, 5, 10, 11, 20, 21, 30, 31, 32, 33, 34, 35, 40, 41, 42}

func (b bundle) encode() []byte {
	out := vfield(1, uint64(b.formatVersion))
	out = append(out, str(2, b.rulesVersion)...)
	for _, id := range b.features {
		out = append(out, vfield(3, uint64(id))...)
	}
	digest := b.digest
	if digest == nil {
		digest = make([]byte, 32)
	}
	out = append(out, bfield(4, digest)...)
	for _, d := range b.defs {
		out = append(out, d.encode()...)
	}
	for _, p := range b.programs {
		out = append(out, p.encode()...)
	}
	for _, d := range b.dispatchers {
		out = append(out, d.encode()...)
	}
	return append(out, b.extra...)
}

// Wire encoding helpers.

func uvarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

func tag(num, wire int) uint64 {
	if num <= 0 || num > math.MaxInt32 {
		panic("field number out of range")
	}
	return uint64(num)<<3 | uint64(wire)
}

func vfield(num int, v uint64) []byte { return uvarint(uvarint(nil, tag(num, 0)), v) }

func bfield(num int, payload []byte) []byte {
	out := uvarint(nil, tag(num, 2))
	out = uvarint(out, uint64(len(payload)))
	return append(out, payload...)
}

func str(num int, s string) []byte { return bfield(num, []byte(s)) }

func alphabet(s string) field { return field{"alphabet", func(n int) []byte { return str(n, s) }} }

// longAlphabet builds an alphabet of n distinct code points, starting at a
// printable ASCII character and continuing past it into the BMP.
func longAlphabet(n int) string {
	var b []rune
	for r := rune('!'); len(b) < n; r++ {
		if r == utf8.RuneError {
			continue
		}
		b = append(b, r)
	}
	return string(b)
}

// withoutFeature drops one capability id from a declaration list.
func withoutFeature(ids []uint32, drop uint32) []uint32 {
	out := make([]uint32, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}
