// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package runtime

import "unicode/utf8"

// Int is a checked signed 64 bit integer that may be indeterminate.
//
// An integer is indeterminate when it cannot be evaluated: an absent or empty
// operand, a code point outside the mapping domain, an index outside a
// remainder table, or a complement operand outside [0, modulus]. It propagates
// through every integer operation and makes the enclosing checksum node
// unsupported. It never produces invalid: refusing a valid identifier is the
// most serious defect of the project.
type Int struct {
	v  int64
	ok bool
}

// Indeterminate is the integer no operation could evaluate.
var Indeterminate = Int{}

// Number returns a determinate integer.
func Number(v int64) Int { return Int{v: v, ok: true} }

// Determinate reports whether the integer holds a value.
func (i Int) Determinate() bool { return i.ok }

// Value returns the integer, which is meaningless unless Determinate is true.
func (i Int) Value() int64 { return i.v }

// DigitsToInteger reads a view as a non negative decimal integer. The generator
// only emits it once it has proven the view holds at most 18 code points, so no
// overflow is reachable.
func DigitsToInteger(v View) Int {
	if !v.present || len(v.s) == 0 {
		return Indeterminate
	}
	var acc int64
	for i := 0; i < len(v.s); i++ {
		b := v.s[i]
		if !isASCIIDigit(b) {
			return Indeterminate
		}
		acc = acc*10 + int64(b-'0')
	}
	return Number(acc)
}

// ModDigits computes the remainder of a view modulo m, digit by digit and
// without any big integer conversion, so an identifier of any length stays
// exact.
func ModDigits(v View, m int64) Int {
	if !v.present || len(v.s) == 0 {
		return Indeterminate
	}
	var acc int64
	for i := 0; i < len(v.s); i++ {
		b := v.s[i]
		if !isASCIIDigit(b) {
			return Indeterminate
		}
		acc = (acc*10 + int64(b-'0')) % m
	}
	return Number(acc)
}

// Modulo is the euclidean remainder, always in [0, m).
func (i Int) Modulo(m int64) Int {
	if !i.ok {
		return Indeterminate
	}
	return Number(((i.v % m) + m) % m)
}

// Complement yields m - i. It is indeterminate when i lies outside [0, m].
func (i Int) Complement(m int64) Int {
	if !i.ok || i.v < 0 || i.v > m {
		return Indeterminate
	}
	return Number(m - i.v)
}

// RemainderMap yields table[i]. It is indeterminate when i lies outside the
// table.
func (i Int) RemainderMap(table []int64) Int {
	if !i.ok || i.v < 0 || i.v >= int64(len(table)) {
		return Indeterminate
	}
	return Number(table[i.v])
}

// Is reports whether the integer equals a literal constant.
//
// An indeterminate operand yields false, so a branch guarded by it does not
// apply and the enclosing CHOOSE falls through, as section 3.3 of ir.md
// requires.
func (i Int) Is(constant int64) bool { return i.ok && i.v == constant }

// Alignment describes how weights are paired with input code points.
type Alignment uint8

// The three frozen alignments.
const (
	AlignLeft Alignment = iota
	AlignRight
	AlignCycle
)

// Mapping maps a code point to its numeric contribution.
type Mapping uint8

// The two frozen mappings.
const (
	MapDigitValue Mapping = iota
	MapAlnumBase36
)

// WeightedSum sums mapping(expr[i]) * weight(i) over the paired positions.
//
// LEFT pairs position i with weights[i], RIGHT pairs the last position with the
// last weight, and CYCLE pairs position i with weights[i mod len(weights)].
// LEFT and RIGHT only pair min(len(expr), len(weights)) positions; the
// remaining positions contribute nothing.
//
// A single code point outside the mapping domain makes the whole sum
// indeterminate, hence the enclosing checksum unsupported.
func WeightedSum(v View, weights []int64, align Alignment, mapping Mapping) Int {
	if !v.present || len(v.s) == 0 {
		return Indeterminate
	}
	// The V1 mapping domains are ASCII only, so a byte wise walk is exact: a
	// multi byte code point starts above 0x7F and is rejected.
	n := len(v.s)
	value := func(b byte) int64 {
		switch {
		case isASCIIDigit(b):
			return int64(b - '0')
		case mapping == MapAlnumBase36 && isASCIIUpper(b):
			return int64(b-'A') + 10
		}
		return -1
	}
	for i := 0; i < n; i++ {
		if value(v.s[i]) < 0 {
			return Indeterminate
		}
	}

	var acc int64
	switch align {
	case AlignLeft:
		for i := 0; i < n && i < len(weights); i++ {
			acc += value(v.s[i]) * weights[i]
		}
	case AlignRight:
		pairs := min(n, len(weights))
		for j := range pairs {
			acc += value(v.s[n-1-j]) * weights[len(weights)-1-j]
		}
	default: // AlignCycle
		for i := 0; i < n; i++ {
			acc += value(v.s[i]) * weights[i%len(weights)]
		}
	}
	return Number(acc)
}

// WeightedSumAlphabet is WeightedSum over an issuer's own alphabet: the value of
// a code point is its index in alphabet, and a code point absent from it makes
// the whole sum indeterminate.
//
// Issuers routinely drop the letters that are misread as digits, and every
// letter after the gap shifts: the Chinese unified social credit code omits I,
// O, S, V and Z, so its J is 18 where ALNUM_BASE36 makes it 19.
func WeightedSumAlphabet(v View, weights []int64, align Alignment, alphabet string) Int {
	if !v.present || len(v.s) == 0 {
		return Indeterminate
	}
	// A single code point outside the alphabet makes the whole sum
	// indeterminate, including one at a position no weight pairs with, so the
	// value is walked once before anything is summed.
	n := 0
	for i := 0; i < len(v.s); {
		r, size := utf8.DecodeRuneInString(v.s[i:])
		if r == utf8.RuneError && size <= 1 {
			return Indeterminate
		}
		if alphabetIndex(alphabet, r) < 0 {
			return Indeterminate
		}
		i += size
		n++
	}

	// RIGHT pairs the last code point with the last weight, so it starts
	// contributing only once the walk reaches that suffix.
	first := 0
	if align == AlignRight {
		first = n - min(n, len(weights))
	}
	var acc int64
	pos := 0
	for i := 0; i < len(v.s); pos++ {
		r, size := utf8.DecodeRuneInString(v.s[i:])
		i += size
		var w int64
		switch align {
		case AlignLeft:
			if pos >= len(weights) {
				return Number(acc)
			}
			w = weights[pos]
		case AlignRight:
			if pos < first {
				continue
			}
			w = weights[len(weights)-(n-pos)]
		default: // AlignCycle
			w = weights[pos%len(weights)]
		}
		acc += int64(alphabetIndex(alphabet, r)) * w
	}
	return Number(acc)
}

// alphabetIndex is the value a custom alphabet gives a code point, or -1 when
// the alphabet does not carry it. The generator has already refused an alphabet
// that repeats a code point, so the first match is the only one.
func alphabetIndex(alphabet string, r rune) int {
	if r < utf8.RuneSelf {
		// An ASCII code point can only match an ASCII byte of the alphabet,
		// and every byte before it is then one code point of its own. The
		// conversion is exact: r was just bounded below RuneSelf.
		want := byte(r) //nolint:gosec // r < utf8.RuneSelf, so it fits a byte
		for i := 0; i < len(alphabet); i++ {
			if alphabet[i] >= utf8.RuneSelf {
				break
			}
			if alphabet[i] == want {
				return i
			}
		}
	}
	idx := 0
	for _, a := range alphabet {
		if a == r {
			return idx
		}
		idx++
	}
	return -1
}
