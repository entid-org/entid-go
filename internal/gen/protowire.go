// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"math"
	"unicode/utf8"
)

// Protobuf wire types. Groups (3 and 4) are deprecated and never emitted by the
// compiler, so the reader refuses them outright.
const (
	wireVarint = 0
	wireI64    = 1
	wireBytes  = 2
	wireI32    = 5
)

// maxDecodeDepth bounds message nesting. The deepest V1 message chain is
// RuleBundle > Program > Node > StringOperation, so 16 leaves ample headroom
// while stopping a nesting bomb long before the Go stack is at risk.
const maxDecodeDepth = 16

// reader walks one Protobuf message body. It refuses every construct the V1
// schema cannot contain rather than skipping it, so a forged bundle can neither
// read past its slice nor recurse without bound.
type reader struct {
	buf   []byte
	pos   int
	depth int

	// lenient skips a field the schema does not declare instead of refusing
	// it. Section 10 orders the format version and the capability ids before
	// the unknown field scan, and a lenient pass is what lets those two
	// answer first on a bundle built against a later version.
	lenient bool
}

func newReader(buf []byte) *reader { return &reader{buf: buf} }

func (r *reader) done() bool { return r.pos >= len(r.buf) }

// next reads the next tag and returns its field number and wire type.
func (r *reader) next() (field, wire int, err error) {
	tag, err := r.varint()
	if err != nil {
		return 0, 0, err
	}
	wire = int(tag & 7)
	num := tag >> 3
	if num == 0 || num > math.MaxInt32 {
		return 0, 0, malformed("field number %d out of range", num)
	}
	switch wire {
	case wireVarint, wireI64, wireBytes, wireI32:
	default:
		return 0, 0, malformed("wire type %d", wire)
	}
	return int(num), wire, nil
}

func (r *reader) varint() (uint64, error) {
	var v uint64
	for shift := uint(0); ; shift += 7 {
		if r.pos >= len(r.buf) {
			return 0, malformed("truncated varint")
		}
		if shift >= 64 {
			return 0, malformed("varint overflows 64 bits")
		}
		b := r.buf[r.pos]
		r.pos++
		if shift == 63 && b > 1 {
			return 0, malformed("varint overflows 64 bits")
		}
		v |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return v, nil
		}
	}
}

// bytes reads a length delimited field body.
func (r *reader) bytes() ([]byte, error) {
	n, err := r.varint()
	if err != nil {
		return nil, err
	}
	remaining := len(r.buf) - r.pos
	if remaining < 0 || n > uint64(remaining) {
		return nil, malformed("length %d exceeds the %d remaining bytes", n, remaining)
	}
	start := r.pos
	r.pos += int(n) //nolint:gosec // n was just bounded by remaining, an int
	return r.buf[start:r.pos], nil
}

// str reads a length delimited field and requires valid UTF-8, as proto3 does
// for the string type.
func (r *reader) str() (string, error) {
	b, err := r.bytes()
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", malformed("string field is not valid UTF-8")
	}
	return string(b), nil
}

// u32 reads a varint that must fit in 32 unsigned bits.
func (r *reader) u32() (uint32, error) {
	v, err := r.varint()
	if err != nil {
		return 0, err
	}
	if v > math.MaxUint32 {
		return 0, malformed("value %d overflows uint32", v)
	}
	return uint32(v), nil
}

// i64 reads a varint holding a signed 64 bit value.
//
// proto3 encodes int64 as an unsigned varint of the two's complement bits, so
// reinterpreting the full 64 bits is the decoding, not a lossy conversion.
func (r *reader) i64() (int64, error) {
	v, err := r.varint()
	if err != nil {
		return 0, err
	}
	return int64(v), nil //nolint:gosec // proto3 int64 is the two's complement of the varint
}

// enum reads an enum value, which proto3 transports as a varint.
func (r *reader) enum() (int32, error) {
	v, err := r.varint()
	if err != nil {
		return 0, err
	}
	// Comparing the unsigned value is what makes this correct: converting
	// first would wrap a value above 2^63 into a negative int64 and let it
	// through as zero.
	if v > math.MaxInt32 {
		return 0, malformed("enum value %d out of range", v)
	}
	return int32(v), nil
}

// boolean reads a proto3 bool, which may only carry 0 or 1.
func (r *reader) boolean() (bool, error) {
	v, err := r.varint()
	if err != nil {
		return false, err
	}
	if v > 1 {
		return false, malformed("bool field carries %d", v)
	}
	return v == 1, nil
}

// message reads a nested message body and hands it to decode.
func (r *reader) message(decode func(*reader) error) error {
	body, err := r.bytes()
	if err != nil {
		return err
	}
	if r.depth+1 > maxDecodeDepth {
		return malformed("message nesting deeper than %d", maxDecodeDepth)
	}
	sub := &reader{buf: body, depth: r.depth + 1, lenient: r.lenient}
	if err := decode(sub); err != nil {
		return err
	}
	if !sub.done() {
		return malformed("trailing bytes in a nested message")
	}
	return nil
}

// packedU32 reads a repeated uint32 field. proto3 packs repeated scalars by
// default, but a conforming decoder must also accept the unpacked encoding, so
// both wire types are handled.
func (r *reader) packedU32(dst []uint32, wire int) ([]uint32, error) {
	switch wire {
	case wireVarint:
		v, err := r.u32()
		if err != nil {
			return dst, err
		}
		return append(dst, v), nil
	case wireBytes:
		body, err := r.bytes()
		if err != nil {
			return dst, err
		}
		sub := newReader(body)
		for !sub.done() {
			v, err := sub.u32()
			if err != nil {
				return dst, err
			}
			dst = append(dst, v)
		}
		return dst, nil
	}
	return dst, malformed("repeated uint32 with wire type %d", wire)
}

// packedI64 mirrors packedU32 for repeated int64 fields.
func (r *reader) packedI64(dst []int64, wire int) ([]int64, error) {
	switch wire {
	case wireVarint:
		v, err := r.i64()
		if err != nil {
			return dst, err
		}
		return append(dst, v), nil
	case wireBytes:
		body, err := r.bytes()
		if err != nil {
			return dst, err
		}
		sub := newReader(body)
		for !sub.done() {
			v, err := sub.i64()
			if err != nil {
				return dst, err
			}
			dst = append(dst, v)
		}
		return dst, nil
	}
	return dst, malformed("repeated int64 with wire type %d", wire)
}

// seen tracks which singular fields a message body already carried. Protobuf
// lets a singular field repeat and keeps the last occurrence, but two engines
// disagreeing on which one wins would break the observable determinism of
// section 2.4, so a duplicate is refused instead.
type seen uint64

func (s *seen) mark(msg string, field int) error {
	bit := seen(1) << uint(field)
	if *s&bit != 0 {
		return malformed("field %d of %s appears twice", field, msg)
	}
	*s |= bit
	return nil
}

// unknown handles a field number outside the schema of the message being
// decoded. Section 7.1 of the specification requires an unknown field to be
// refused at any depth rather than preserved or ignored, which is what the
// strict pass does; the lenient pass consumes it so that the version checks
// section 10 places first can run.
func (r *reader) unknown(msg string, field, wire int) error {
	if !r.lenient {
		return malformed("unknown field %d in %s", field, msg)
	}
	return r.skip(wire)
}

// skip consumes the body of a field the decoder does not read.
func (r *reader) skip(wire int) error {
	switch wire {
	case wireVarint:
		_, err := r.varint()
		return err
	case wireI64:
		return r.fixed(8)
	case wireBytes:
		_, err := r.bytes()
		return err
	case wireI32:
		return r.fixed(4)
	}
	return malformed("wire type %d", wire)
}

func (r *reader) fixed(n int) error {
	if len(r.buf)-r.pos < n {
		return malformed("truncated %d byte field", n)
	}
	r.pos += n
	return nil
}

func wrongWire(msg string, field, wire int) error {
	return malformed("field %d of %s has wire type %d", field, msg, wire)
}

func malformed(format string, args ...any) error {
	return invalidf("malformed protobuf: %s", fmt.Sprintf(format, args...))
}
