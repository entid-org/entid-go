// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

// Package runtime holds the support primitives the generated engine calls.
//
// It implements the parts of the IR semantics that are worth writing once —
// the frozen whitespace table, the ASCII classes, code point indexing, the
// checksum algorithms and the canonicalization workspace — while control flow
// and every constant live in the generated code.
//
// Nothing here is constructed at program start-up, and no operation on a
// well formed input allocates.
package runtime

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// View is a possibly absent string view over Unicode code points.
//
// Absence propagates: every string constructor applied to an absent operand
// yields an absent result, and every predicate applied to one yields false,
// except IsAbsent. Absence is never an error.
//
// Views share the memory of the value they are cut from, so slicing one never
// allocates.
type View struct {
	s       string
	present bool
}

// Absent is the view that holds no value at all, as opposed to a present view
// of zero code points.
var Absent = View{}

// Value returns a present view over s.
func Value(s string) View { return View{s: s, present: true} }

// IsAbsent reports whether the view holds no value.
func (v View) IsAbsent() bool { return !v.present }

// IsEmpty reports whether the view is present and holds zero code points.
func (v View) IsEmpty() bool { return v.present && len(v.s) == 0 }

// String returns the underlying value. It is empty when the view is absent,
// which only a caller that already checked presence should rely on.
func (v View) String() string { return v.s }

// RuneLen is the number of code points, or zero when the view is absent.
func (v View) RuneLen() int {
	if !v.present {
		return 0
	}
	return utf8.RuneCountInString(v.s)
}

// LengthEq reports whether the view is present and holds exactly n code points.
func (v View) LengthEq(n int) bool { return v.present && countIs(v.s, n) }

// LengthBetween reports whether the view is present and its code point length
// lies in [min, max].
func (v View) LengthBetween(minLen, maxLen int) bool {
	if !v.present {
		return false
	}
	n := utf8.RuneCountInString(v.s)
	return n >= minLen && n <= maxLen
}

// Equals reports whether both views are present and hold the same code points.
func (v View) Equals(other View) bool {
	return v.present && other.present && v.s == other.s
}

// HasPrefix reports whether the view is present and starts with prefix.
func (v View) HasPrefix(prefix string) bool { return v.present && strings.HasPrefix(v.s, prefix) }

// HasSuffix reports whether the view is present and ends with suffix.
func (v View) HasSuffix(suffix string) bool { return v.present && strings.HasSuffix(v.s, suffix) }

// Contains reports whether the view is present and holds literal.
func (v View) Contains(literal string) bool { return v.present && strings.Contains(v.s, literal) }

// ASCIIDigits reports whether the view is present, non empty and made only of
// U+0030..U+0039.
func (v View) ASCIIDigits() bool { return v.classOf(isASCIIDigit) }

// ASCIIUpperLetters reports whether the view is present, non empty and made
// only of U+0041..U+005A.
func (v View) ASCIIUpperLetters() bool { return v.classOf(isASCIIUpper) }

// ASCIIAlphanumeric reports whether the view is present, non empty and made
// only of ASCII digits and upper case letters.
func (v View) ASCIIAlphanumeric() bool {
	return v.classOf(func(b byte) bool { return isASCIIDigit(b) || isASCIIUpper(b) })
}

// ASCIICharset reports whether the view is present, non empty and every code
// point belongs to the non empty ASCII set.
func (v View) ASCIICharset(set string) bool {
	return v.classOf(func(b byte) bool { return strings.IndexByte(set, b) >= 0 })
}

// classOf applies an ASCII class to every byte. A multi byte code point always
// has its leading byte above 0x7F, so a byte wise test rejects it, which is
// exactly what an ASCII only class must do.
func (v View) classOf(in func(byte) bool) bool {
	if !v.present || len(v.s) == 0 {
		return false
	}
	for i := 0; i < len(v.s); i++ {
		if !in(v.s[i]) {
			return false
		}
	}
	return true
}

// CharAtIn reports whether the code point at position index belongs to the non
// empty ASCII set.
func (v View) CharAtIn(index int, set string) bool {
	if !v.present {
		return false
	}
	off, ok := runeOffset(v.s, index)
	if !ok || off == len(v.s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(v.s[off:])
	if r >= utf8.RuneSelf {
		// The set is ASCII, so no wide code point can belong to it.
		return false
	}
	return strings.IndexByte(set, byte(r)) >= 0 //nolint:gosec // guarded above
}

// Slice yields the code points of the view in [start, end). It is absent when
// the view is absent, when start is above end, or when end is past the end of
// the view.
func (v View) Slice(start, end int) View {
	if !v.present || start > end {
		return Absent
	}
	from, ok := runeOffset(v.s, start)
	if !ok {
		return Absent
	}
	to, ok := runeOffsetFrom(v.s, from, end-start)
	if !ok {
		return Absent
	}
	return View{s: v.s[from:to], present: true}
}

// SliceFrom yields the code points from start to the end of the view. It is
// absent when the view is absent or when start is past its end.
func (v View) SliceFrom(start int) View {
	if !v.present {
		return Absent
	}
	from, ok := runeOffset(v.s, start)
	if !ok {
		return Absent
	}
	return View{s: v.s[from:], present: true}
}

// SliceTo yields the code points before end. It is absent when the view is
// absent or when end is past its end.
func (v View) SliceTo(end int) View {
	if !v.present {
		return Absent
	}
	to, ok := runeOffset(v.s, end)
	if !ok {
		return Absent
	}
	return View{s: v.s[:to], present: true}
}

// BeforeFirst yields the part of the view before the first occurrence of the
// non empty constant sep. It is absent when sep does not occur.
func (v View) BeforeFirst(sep string) View {
	if !v.present {
		return Absent
	}
	i := strings.Index(v.s, sep)
	if i < 0 {
		return Absent
	}
	return View{s: v.s[:i], present: true}
}

// AfterFirst yields the part of the view after the first occurrence of the non
// empty constant sep. It is absent when sep does not occur.
func (v View) AfterFirst(sep string) View {
	if !v.present {
		return Absent
	}
	i := strings.Index(v.s, sep)
	if i < 0 {
		return Absent
	}
	return View{s: v.s[i+len(sep):], present: true}
}

// StripPrefix yields the view without its exact leading prefix. It is absent
// when the view does not start with prefix.
func (v View) StripPrefix(prefix string) View {
	if !v.present || !strings.HasPrefix(v.s, prefix) {
		return Absent
	}
	return View{s: v.s[len(prefix):], present: true}
}

// runeOffset returns the byte offset of the i-th code point of s, and whether
// s holds at least i code points. Offsets are counted in code points because
// every index in the IR is, while Go slices strings by bytes.
func runeOffset(s string, i int) (int, bool) { return runeOffsetFrom(s, 0, i) }

// runeOffsetFrom advances i code points from a byte offset already known to sit
// on a code point boundary.
func runeOffsetFrom(s string, off, i int) (int, bool) {
	if i < 0 {
		return 0, false
	}
	// An all ASCII prefix is the common case once a value is canonical: the
	// byte offset and the code point index then coincide.
	if off+i <= len(s) && isASCII(s[off:off+i]) {
		return off + i, true
	}
	for ; i > 0; i-- {
		if off >= len(s) {
			return 0, false
		}
		_, size := utf8.DecodeRuneInString(s[off:])
		off += size
	}
	return off, true
}

// countIs reports whether s holds exactly n code points, without counting past
// what the answer needs.
func countIs(s string, n int) bool {
	off, ok := runeOffset(s, n)
	return ok && off == len(s)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }
func isASCIIUpper(b byte) bool { return b >= 'A' && b <= 'Z' }
func isASCIILower(b byte) bool { return b >= 'a' && b <= 'z' }

// LengthIn reports whether the view is present and its code point length
// belongs to the set.
func (v View) LengthIn(lengths ...int) bool {
	if !v.present {
		return false
	}
	n := utf8.RuneCountInString(v.s)
	for _, want := range lengths {
		if n == want {
			return true
		}
	}
	return false
}

// PrefixGroup is the set of accepted prefixes sharing one byte length, sorted
// ascending. The generator builds these, one per distinct length, from the
// values of a single prefix_in node.
type PrefixGroup struct {
	// Length is the byte length every value in the group has.
	Length int
	// Values is sorted ascending and holds no duplicate. Section 10 of ir.md
	// requires a bundle's prefix_in values to be strictly ascending, and the
	// loader refuses one where they are not, so filtering that list by length
	// yields a sorted list without reordering anything.
	Values []string
}

// PrefixInSorted reports whether the view is present and starts with at least
// one of the prefixes of the groups. The name states the precondition: the
// groups must be sorted, and the answer is wrong rather than slow if they are
// not.
//
// Section 14 of engine.md requires a membership test not to be linear in the
// size of the list, which matters because the cost falls on the refused input:
// a scan stops early on a value it finds and reads everything on a value it
// does not. Grouping by length is what makes a search exact. Within one group
// every value has the same length, so "starts with one of these" is "equals
// the first Length bytes", which a binary search answers in log time on keys
// of constant size. Across groups the work is one search per distinct length,
// and a rule declares one or two.
//
// The groups are read, never written, so one slice is shared by every call and
// nothing is built per validation.
func (v View) PrefixInSorted(groups []PrefixGroup) bool {
	if !v.present {
		return false
	}
	for _, g := range groups {
		if len(v.s) < g.Length {
			continue
		}
		if _, found := slices.BinarySearch(g.Values, v.s[:g.Length]); found {
			return true
		}
	}
	return false
}

// Concat joins its operands in order. It is absent when any operand is absent.
func Concat(parts ...View) View {
	total := 0
	for _, p := range parts {
		if !p.present {
			return Absent
		}
		total += len(p.s)
	}
	var b strings.Builder
	b.Grow(total)
	for _, p := range parts {
		b.WriteString(p.s)
	}
	return Value(b.String())
}

// HasPrefix reports whether s starts with prefix. Dispatch uses it on the
// pre-canonical value, where no view wrapper is needed.
func HasPrefix(s, prefix string) bool { return strings.HasPrefix(s, prefix) }
