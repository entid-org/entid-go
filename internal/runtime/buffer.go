// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"strings"
	"unicode/utf8"
	"unsafe"
)

// MaxInput is the largest user input any operation accepts, in UTF-8 bytes.
// A longer value is refused without being processed.
const MaxInput = 1024

// Buf is a canonicalization workspace over a caller supplied scratch buffer.
// It never allocates, and it never copies the input until a step actually
// changes it: on a value that is already canonical — the common case — the
// original string is handed straight back.
//
// The generator sizes the scratch from the growth each canonicalizer can add,
// so the buffer is a fixed-size array on the caller's stack. Its zero value is
// ready after Reset.
type Buf struct {
	// b is the workspace. It is never resized and never escapes.
	b []byte

	// left is where a value is centred, leaving room for the steps that
	// prepend.
	left int

	// lo and hi delimit the value inside b once it has been loaded.
	lo, hi int

	// src holds the value while it still shares memory with the input.
	src string

	// loaded reports whether the value lives in b rather than in src.
	loaded bool
}

// New points a workspace at an input without copying it.
//
// scratch must hold at least leftMargin + len(in) + the growth the steps can
// add on the right; the generator computes both from the rules it compiles.
//
// The workspace is returned by value rather than filled through a pointer, so
// that Go can keep the caller's scratch array on the stack.
func New(in string, scratch []byte, leftMargin int) Buf {
	return Buf{b: scratch, left: leftMargin, src: in}
}

// Str returns the current value without allocating.
//
// The result borrows the workspace: it stays valid only until the next step
// mutates it, so it must never outlive the canonicalization. Callers inside
// this package and the generated canonicalizers respect that; String is the
// only way out.
func (w *Buf) Str() string {
	if !w.loaded {
		return w.src
	}
	if w.hi == w.lo {
		return ""
	}
	return unsafe.String(&w.b[w.lo], w.hi-w.lo)
}

// Modified reports whether any step changed the input.
//
// When it is false the value is exactly the string Reset was given, and the
// caller already holds it: returning it from here instead would tie the
// workspace to the result and force the scratch array onto the heap.
func (w *Buf) Modified() bool { return w.loaded }

// String copies the canonical value out of the workspace.
//
// It is the single allocation of a canonicalization, and it only happens when
// Modified reports true. Calling it otherwise yields an empty string.
func (w *Buf) String() string { return string(w.b[w.lo:w.hi]) }

// load copies the value into the workspace, centred so that later steps can
// grow it on either side.
func (w *Buf) load() {
	if w.loaded {
		return
	}
	w.lo = w.left
	w.hi = w.lo + copy(w.b[w.lo:], w.src)
	w.loaded = true
}

// TrimWhitespace removes every leading and trailing code point of the frozen
// whitespace_v1 table. It only ever narrows the value, so it never needs the
// workspace.
func (w *Buf) TrimWhitespace() {
	s := w.Str()
	start := 0
	for start < len(s) {
		r, size := utf8.DecodeRuneInString(s[start:])
		if !IsWhitespaceV1(r) {
			break
		}
		start += size
	}
	end := len(s)
	for end > start {
		r, size := utf8.DecodeLastRuneInString(s[start:end])
		if !IsWhitespaceV1(r) {
			break
		}
		end -= size
	}
	if start == 0 && end == len(s) {
		return
	}
	// Narrowing could stay a sub-slice of the caller's string, but then an
	// unmodified workspace could no longer be told from a narrowed one without
	// handing the caller a string that borrows it. Loading keeps Modified
	// exact, and a trim that changes anything has to be materialized anyway.
	w.load()
	w.lo, w.hi = w.lo+start, w.lo+end
}

// RemoveWhitespace removes every code point of the frozen whitespace_v1 table.
func (w *Buf) RemoveWhitespace() {
	w.filter(func(r rune) bool { return !IsWhitespaceV1(r) })
}

// RemoveChars removes every code point belonging to the non empty ASCII set.
func (w *Buf) RemoveChars(set string) {
	w.filter(func(r rune) bool {
		if r >= utf8.RuneSelf {
			// The set is ASCII, so no wide code point can belong to it.
			return true
		}
		return strings.IndexByte(set, byte(r)) < 0 //nolint:gosec // guarded above
	})
}

// filter keeps the code points the predicate accepts, in place. It is a no-op
// when nothing is removed, which keeps a clean input allocation free.
func (w *Buf) filter(keep func(rune) bool) {
	cut := -1
	for i, r := range w.Str() {
		if !keep(r) {
			cut = i
			break
		}
	}
	if cut < 0 {
		return
	}
	w.load()

	// The bytes of a kept code point are copied, never re-encoded. A byte that
	// is not valid UTF-8 decodes to U+FFFD, which re-encodes to three bytes
	// where it occupied one: writing that back would grow the value while
	// removing from it, and run past the workspace on a long enough input.
	src := w.b[w.lo:w.hi]
	dst := cut
	for i := cut; i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if keep(r) {
			// dst never overtakes i, since filtering only ever removes.
			copy(src[dst:dst+size], src[i:i+size])
			dst += size
		}
		i += size
	}
	w.hi = w.lo + dst
}

// UppercaseASCII maps only a..z to A..Z. Every other code point is preserved,
// and no locale is ever consulted.
func (w *Buf) UppercaseASCII() {
	s := w.Str()
	first := -1
	for i := 0; i < len(s); i++ {
		if isASCIILower(s[i]) {
			first = i
			break
		}
	}
	if first < 0 {
		return
	}
	w.load()
	for i := w.lo + first; i < w.hi; i++ {
		if isASCIILower(w.b[i]) {
			w.b[i] -= 32
		}
	}
}

// ReplacePrefix replaces the exact leading from by to when present.
func (w *Buf) ReplacePrefix(from, to string) {
	if !strings.HasPrefix(w.Str(), from) {
		return
	}
	w.load()
	switch {
	case len(to) <= len(from):
		// Shrinking or keeping the width: write over the old prefix and move
		// the left edge forward.
		w.lo += len(from) - len(to)
		copy(w.b[w.lo:], to)
	default:
		w.shiftLeftEdge(len(to) - len(from))
		copy(w.b[w.lo:], to)
	}
}

// Prepend inserts the non empty constant before the current value.
func (w *Buf) Prepend(text string) {
	w.load()
	w.shiftLeftEdge(len(text))
	copy(w.b[w.lo:], text)
}

// Append adds the non empty constant after the current value.
func (w *Buf) Append(text string) {
	w.load()
	w.hi += copy(w.b[w.hi:], text)
}

// Insert places the non empty constant at code point position index. An index
// past the end leaves the value unchanged: canonicalization never fails on user
// input.
func (w *Buf) Insert(index int, text string) {
	off, ok := runeOffset(w.Str(), index)
	if !ok {
		return
	}
	w.load()
	at := w.lo + off
	copy(w.b[at+len(text):w.hi+len(text)], w.b[at:w.hi])
	copy(w.b[at:], text)
	w.hi += len(text)
}

// LeftPad prepends copies of the single code point pad until the value holds
// length code points. A longer value is never truncated.
func (w *Buf) LeftPad(length int, pad string) {
	have := utf8.RuneCountInString(w.Str())
	if have >= length {
		return
	}
	w.load()
	n := (length - have) * len(pad)
	w.shiftLeftEdge(n)
	for at := w.lo; at < w.lo+n; at += len(pad) {
		copy(w.b[at:], pad)
	}
}

// shiftLeftEdge makes n bytes room before the value, moving it right when the
// left margin is exhausted.
func (w *Buf) shiftLeftEdge(n int) {
	if w.lo >= n {
		w.lo -= n
		return
	}
	// The generator sizes the left margin so this never happens for a bundle
	// it accepted; the fallback keeps a hand written caller correct anyway.
	shift := n - w.lo
	copy(w.b[w.lo+shift:], w.b[w.lo:w.hi])
	w.hi += shift
	w.lo = 0
}

// IsWhitespaceV1 reports whether r belongs to the frozen whitespace_v1 table of
// section 7 of ir.md.
//
// A runtime must never delegate this definition to its own Unicode tables,
// whose versions differ between languages and releases.
func IsWhitespaceV1(r rune) bool {
	switch {
	case r >= 0x0009 && r <= 0x000D:
		return true
	case r >= 0x2000 && r <= 0x200A:
		return true
	}
	switch r {
	case 0x0020, 0x0085, 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return false
}

// TrimASCII removes only U+0009..U+000D and U+0020 at both ends. Dispatch uses
// it on the kind and country tokens supplied by the caller, a narrower class
// than whitespace_v1.
func TrimASCII(s string) string {
	start := 0
	for start < len(s) && isASCIISpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isASCIISpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isASCIISpace(b byte) bool { return b == 0x20 || (b >= 0x09 && b <= 0x0D) }

// LowerASCII maps only A..Z to a..z. It returns s unchanged, without
// allocating, when there is nothing to map.
func LowerASCII(s string) string {
	if !hasByteIn(s, isASCIIUpper) {
		return s
	}
	out := []byte(s)
	for i, b := range out {
		if isASCIIUpper(b) {
			out[i] = b + 32
		}
	}
	return string(out)
}

// UpperASCII maps only a..z to A..Z, and is allocation free when there is
// nothing to map. Country tokens are two bytes, so the copy is never hot.
func UpperASCII(s string) string {
	if !hasByteIn(s, isASCIILower) {
		return s
	}
	out := []byte(s)
	for i, b := range out {
		if isASCIILower(b) {
			out[i] = b - 32
		}
	}
	return string(out)
}

func hasByteIn(s string, in func(byte) bool) bool {
	for i := 0; i < len(s); i++ {
		if in(s[i]) {
			return true
		}
	}
	return false
}

// HasPrefix reports whether the current value starts with prefix.
func (w *Buf) HasPrefix(prefix string) bool { return strings.HasPrefix(w.Str(), prefix) }
