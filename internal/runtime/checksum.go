// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package runtime

// Outcome is the tri-state result of a checksum primitive.
//
// Unsupported is the zero value on purpose: an algorithm that cannot be applied
// says nothing about the identifier, and saying nothing is the safe default.
type Outcome uint8

// The three checksum outcomes.
const (
	Unsupported Outcome = iota
	Valid
	Invalid
)

// Luhn applies the Luhn algorithm, whose rightmost code point is the check
// digit. It is unsupported, never invalid, when the view cannot carry a Luhn
// check at all: absent, shorter than two code points, or holding a code point
// that is not an ASCII digit.
func Luhn(v View) Outcome {
	if !v.present || len(v.s) < 2 {
		return Unsupported
	}
	sum := 0
	double := false
	for i := len(v.s) - 1; i >= 0; i-- {
		b := v.s[i]
		if !isASCIIDigit(b) {
			return Unsupported
		}
		d := int(b - '0')
		if double {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	if sum%10 == 0 {
		return Valid
	}
	return Invalid
}

// ISO7064Mod97 expands every ASCII letter to its base 36 decimal value, every
// ASCII digit to itself, and requires the resulting decimal string to be
// congruent to one modulo 97.
//
// The remainder is accumulated digit by digit, so no big integer is ever built
// and an identifier of any length stays exact.
func ISO7064Mod97(v View) Outcome {
	if !v.present || len(v.s) < 3 {
		return Unsupported
	}
	rem := 0
	for i := 0; i < len(v.s); i++ {
		b := v.s[i]
		switch {
		case isASCIIDigit(b):
			rem = (rem*10 + int(b-'0')) % 97
		case isASCIIUpper(b):
			// A letter expands to two decimal digits, 10 through 35.
			rem = (rem*100 + int(b-'A') + 10) % 97
		default:
			return Unsupported
		}
	}
	if rem == 1 {
		return Valid
	}
	return Invalid
}

// CompareDigit compares an integer with the ASCII digit of a view at a code
// point position.
func CompareDigit(i Int, v View, index int) Outcome {
	if !i.ok || !v.present {
		return Unsupported
	}
	off, ok := runeOffset(v.s, index)
	if !ok || off >= len(v.s) {
		return Unsupported
	}
	b := v.s[off]
	if !isASCIIDigit(b) {
		return Unsupported
	}
	return equals(i.v, int64(b-'0'))
}

// CompareSlice compares an integer with the decimal value of a view slice.
func CompareSlice(i Int, v View, start, end int) Outcome {
	if !i.ok {
		return Unsupported
	}
	got := DigitsToInteger(v.Slice(start, end))
	if !got.ok {
		return Unsupported
	}
	return equals(i.v, got.v)
}

// CompareConstant compares an integer with a literal constant.
//
// COMPARE_DIGIT and COMPARE_SLICE can only compare against part of the value
// being checked, so a rule stating that a remainder must equal a fixed number
// had nothing to compare with.
func CompareConstant(i Int, constant int64) Outcome {
	if !i.ok {
		return Unsupported
	}
	return equals(i.v, constant)
}

func equals(got, want int64) Outcome {
	if got == want {
		return Valid
	}
	return Invalid
}
