// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package runtime_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	rt "github.com/entid-org/entid-go/internal/runtime"
)

func TestViewSlicing(t *testing.T) {
	tests := []struct {
		name   string
		got    rt.View
		want   string
		absent bool
	}{
		{name: "slice", got: rt.Value("ABCDEF").Slice(1, 4), want: "BCD"},
		{name: "slice with equal bounds is empty", got: rt.Value("ABCDEF").Slice(2, 2), want: ""},
		{name: "slice past the end is absent", got: rt.Value("ABC").Slice(0, 9), absent: true},
		{name: "slice with start above end is absent", got: rt.Value("ABC").Slice(2, 1), absent: true},
		{name: "slice_from", got: rt.Value("ABCDEF").SliceFrom(2), want: "CDEF"},
		{name: "slice_from at the exact length is empty", got: rt.Value("ABC").SliceFrom(3), want: ""},
		{name: "slice_from past the end is absent", got: rt.Value("ABC").SliceFrom(9), absent: true},
		{name: "slice_to", got: rt.Value("ABCDEF").SliceTo(2), want: "AB"},
		{name: "slice_to past the end is absent", got: rt.Value("ABC").SliceTo(9), absent: true},
		{name: "before_first", got: rt.Value("FR.123").BeforeFirst("."), want: "FR"},
		{name: "before_first stops at the first", got: rt.Value("A.B.C").BeforeFirst("."), want: "A"},
		{name: "before_first without occurrence is absent", got: rt.Value("ABC").BeforeFirst("."), absent: true},
		{name: "after_first", got: rt.Value("A.B.C").AfterFirst("."), want: "B.C"},
		{name: "after_first with a wide delimiter", got: rt.Value("AB::CD").AfterFirst("::"), want: "CD"},
		{name: "after_first without occurrence is absent", got: rt.Value("ABC").AfterFirst("."), absent: true},
		{name: "strip_prefix", got: rt.Value("FR123").StripPrefix("FR"), want: "123"},
		{name: "strip_prefix without the prefix is absent", got: rt.Value("BE1").StripPrefix("FR"), absent: true},

		// Indices are counted in code points, not bytes.
		{name: "slice counts code points", got: rt.Value("éèêë").Slice(1, 3), want: "èê"},
		{name: "slice_from counts code points", got: rt.Value("éèêë").SliceFrom(2), want: "êë"},
		{name: "slice_to counts code points", got: rt.Value("éèêë").SliceTo(1), want: "é"},

		// Absence propagates through every constructor.
		{name: "absence propagates through slice", got: rt.Absent.Slice(0, 1), absent: true},
		{name: "absence propagates through slice_from", got: rt.Absent.SliceFrom(0), absent: true},
		{name: "absence propagates through before_first", got: rt.Absent.BeforeFirst("."), absent: true},
		{name: "absence propagates through strip_prefix", got: rt.Absent.StripPrefix(""), absent: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.absent {
				if !tc.got.IsAbsent() {
					t.Fatalf("expected an absent view, got %q", tc.got.String())
				}
				return
			}
			if tc.got.IsAbsent() {
				t.Fatalf("expected %q, got an absent view", tc.want)
			}
			if tc.got.String() != tc.want {
				t.Fatalf("got %q, want %q", tc.got.String(), tc.want)
			}
		})
	}
}

func TestViewPredicates(t *testing.T) {
	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{"is_empty on empty", rt.Value("").IsEmpty(), true},
		{"is_empty on non empty", rt.Value("A").IsEmpty(), false},
		{"is_empty on absent", rt.Absent.IsEmpty(), false},
		{"is_absent on absent", rt.Absent.IsAbsent(), true},
		{"is_absent on empty", rt.Value("").IsAbsent(), false},

		{"equals matching", rt.Value("AB").Equals(rt.Value("AB")), true},
		{"equals differing", rt.Value("AB").Equals(rt.Value("AC")), false},
		{"equals with an absent side", rt.Value("AB").Equals(rt.Absent), false},
		{"equals with two absent sides", rt.Absent.Equals(rt.Absent), false}, //nolint:gocritic // two absent sides is the case

		{"length_eq matching", rt.Value("ABC").LengthEq(3), true},
		{"length_eq differing", rt.Value("AB").LengthEq(3), false},
		{"length_eq on absent", rt.Absent.LengthEq(0), false},
		{"length_eq counts code points", rt.Value("éèê").LengthEq(3), true},
		{"length_eq is not a byte count", rt.Value("éèê").LengthEq(6), false},
		{"length_between lower bound", rt.Value("AB").LengthBetween(2, 4), true},
		{"length_between upper bound", rt.Value("ABCD").LengthBetween(2, 4), true},
		{"length_between below", rt.Value("A").LengthBetween(2, 4), false},
		{"length_between above", rt.Value("ABCDE").LengthBetween(2, 4), false},

		{"ascii_digits", rt.Value("0123456789").ASCIIDigits(), true},
		{"ascii_digits rejects a letter", rt.Value("01A").ASCIIDigits(), false},
		{"ascii_digits on empty", rt.Value("").ASCIIDigits(), false},
		{"ascii_digits on absent", rt.Absent.ASCIIDigits(), false},
		{"ascii_digits rejects an Arabic-Indic digit", rt.Value("٠١").ASCIIDigits(), false},
		{"ascii_upper_letters", rt.Value("ABZ").ASCIIUpperLetters(), true},
		{"ascii_upper_letters rejects lower case", rt.Value("ABz").ASCIIUpperLetters(), false},
		{"ascii_alphanumeric", rt.Value("AB12").ASCIIAlphanumeric(), true},
		{"ascii_alphanumeric rejects lower case", rt.Value("ab12").ASCIIAlphanumeric(), false},
		{"ascii_alphanumeric rejects a separator", rt.Value("AB-12").ASCIIAlphanumeric(), false},
		{"ascii_charset", rt.Value("0110").ASCIICharset("01"), true},
		{"ascii_charset outside", rt.Value("012").ASCIICharset("01"), false},
		{"ascii_charset rejects a wide code point", rt.Value("0é").ASCIICharset("01"), false},

		{"starts_with", rt.Value("FR123").HasPrefix("FR"), true},
		{"starts_with mismatch", rt.Value("BE123").HasPrefix("FR"), false},
		{"starts_with on absent", rt.Absent.HasPrefix("FR"), false},
		{"ends_with", rt.Value("12399").HasSuffix("99"), true},
		{"ends_with mismatch", rt.Value("12398").HasSuffix("99"), false},
		{"ends_with on a shorter value", rt.Value("99").HasSuffix("999"), false},
		{"contains", rt.Value("A.B").Contains("."), true},
		{"contains missing", rt.Value("AB").Contains("."), false},

		{"char_at_in", rt.Value("AB0C").CharAtIn(2, "01"), true},
		{"char_at_in outside the set", rt.Value("AB2C").CharAtIn(2, "01"), false},
		{"char_at_in past the end", rt.Value("AB").CharAtIn(9, "01"), false},
		{"char_at_in on a wide code point", rt.Value("ABé").CharAtIn(2, "01"), false},
		{"char_at_in counts code points", rt.Value("éé0").CharAtIn(2, "01"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

func TestLuhn(t *testing.T) {
	tests := []struct {
		value string
		want  rt.Outcome
	}{
		// A published INSEE example: nine digits closed by a Luhn check digit.
		{"012345674", rt.Valid},
		{"012345675", rt.Invalid},
		{"00", rt.Valid},
		{"18", rt.Valid},
		{"19", rt.Invalid},
		// Shorter than two code points cannot carry a check digit.
		{"7", rt.Unsupported},
		{"", rt.Unsupported},
		// A non ASCII digit makes the algorithm inapplicable, never invalid.
		{"01234567A", rt.Unsupported},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			if got := rt.Luhn(rt.Value(tc.value)); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	if got := rt.Luhn(rt.Absent); got != rt.Unsupported {
		t.Fatalf("an absent view must be unsupported, got %v", got)
	}
}

func TestISO7064Mod97(t *testing.T) {
	tests := []struct {
		value string
		want  rt.Outcome
	}{
		{"00000000000000000098", rt.Valid},
		{"00000000000000000099", rt.Invalid},
		{"AB00", rt.Invalid},
		// Shorter than three code points is inapplicable.
		{"98", rt.Unsupported},
		// The domain is ASCII digits and upper case letters only.
		{"ab00", rt.Unsupported},
		{"AB.0", rt.Unsupported},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			if got := rt.ISO7064Mod97(rt.Value(tc.value)); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestISO7064Mod97MatchesTheDefinition cross-checks the digit by digit
// remainder against a direct base 36 expansion, so an optimization cannot
// drift from the published algorithm.
func TestISO7064Mod97MatchesTheDefinition(t *testing.T) {
	expand := func(s string) string {
		var b strings.Builder
		for _, c := range s {
			if c >= '0' && c <= '9' {
				b.WriteRune(c)
				continue
			}
			v := int(c-'A') + 10
			b.WriteByte(byte('0' + v/10))
			b.WriteByte(byte('0' + v%10))
		}
		return b.String()
	}
	remainder := func(decimal string) int {
		rem := 0
		for _, c := range decimal {
			rem = (rem*10 + int(c-'0')) % 97
		}
		return rem
	}
	for _, value := range []string{
		"5493001KJTIIGC8Y1R12", "ABCDEFGHIJ0123456789", "ZZ99", "100",
		"0000000000000000AB00", "969500TJ5KRTCJQWXH05", "00000000000000000098",
	} {
		want := rt.Invalid
		if remainder(expand(value)) == 1 {
			want = rt.Valid
		}
		if got := rt.ISO7064Mod97(rt.Value(value)); got != want {
			t.Errorf("%s: got %v, want %v", value, got, want)
		}
	}
}

func TestIntegerOperations(t *testing.T) {
	// 10^25 mod 97 is 71, and no big integer is ever built.
	big := "1" + strings.Repeat("0", 25)
	if got := rt.ModDigits(rt.Value(big), 97); !got.Determinate() || got.Value() != 71 {
		t.Fatalf("mod_digits over 26 digits: got %v/%d, want 71", got.Determinate(), got.Value())
	}
	if got := rt.ModDigits(rt.Value("12A"), 97); got.Determinate() {
		t.Fatal("a non digit must be indeterminate")
	}
	if got := rt.ModDigits(rt.Value(""), 97); got.Determinate() {
		t.Fatal("an empty view must be indeterminate")
	}
	if got := rt.DigitsToInteger(rt.Value("1234")); !got.Determinate() || got.Value() != 1234 {
		t.Fatalf("digits_to_integer: got %d", got.Value())
	}
	// 123456789 mod 97 is 39.
	base := rt.ModDigits(rt.Value("123456789"), 97)
	if got := base.Modulo(10); got.Value() != 9 {
		t.Fatalf("modulo: got %d, want 9", got.Value())
	}
	if got := base.Complement(97); got.Value() != 58 {
		t.Fatalf("complement: got %d, want 58", got.Value())
	}
	if got := rt.Number(98).Complement(97); got.Determinate() {
		t.Fatal("a complement operand above the modulus must be indeterminate")
	}
	if got := rt.Number(-1).Complement(97); got.Determinate() {
		t.Fatal("a negative complement operand must be indeterminate")
	}
	table := [3]int64{7, 8, 9}
	if got := rt.Number(1).RemainderMap(table[:]); got.Value() != 8 {
		t.Fatalf("remainder_map: got %d, want 8", got.Value())
	}
	if got := rt.Number(3).RemainderMap(table[:]); got.Determinate() {
		t.Fatal("an index outside the table must be indeterminate")
	}
	if got := rt.Indeterminate.Modulo(10); got.Determinate() {
		t.Fatal("indeterminacy must propagate through modulo")
	}
}

func TestWeightedSum(t *testing.T) {
	weights := [2]int64{1, 10}
	tests := []struct {
		name  string
		got   rt.Int
		want  int64
		indet bool
	}{
		// "123" with weights [1,10]: LEFT pairs 1*1 + 2*10, and "3" is unpaired.
		{name: "left", got: rt.WeightedSum(rt.Value("123"), weights[:], rt.AlignLeft, rt.MapDigitValue), want: 21},
		// RIGHT pairs 3*10 + 2*1, and "1" is unpaired.
		{name: "right", got: rt.WeightedSum(rt.Value("123"), weights[:], rt.AlignRight, rt.MapDigitValue), want: 32},
		// CYCLE pairs 1*1 + 2*10 + 3*1.
		{name: "cycle", got: rt.WeightedSum(rt.Value("123"), weights[:], rt.AlignCycle, rt.MapDigitValue), want: 24},
		{
			name: "base 36 mapping",
			got:  rt.WeightedSum(rt.Value("AB"), weights[:], rt.AlignLeft, rt.MapAlnumBase36),
			want: 10*1 + 11*10,
		},
		{
			name:  "outside the digit domain",
			got:   rt.WeightedSum(rt.Value("AB"), weights[:], rt.AlignLeft, rt.MapDigitValue),
			indet: true,
		},
		{
			name:  "an unpaired code point outside the domain still counts",
			got:   rt.WeightedSum(rt.Value("12A"), weights[:], rt.AlignLeft, rt.MapDigitValue),
			indet: true,
		},
		{name: "absent", got: rt.WeightedSum(rt.Absent, weights[:], rt.AlignLeft, rt.MapDigitValue), indet: true},
		{name: "empty", got: rt.WeightedSum(rt.Value(""), weights[:], rt.AlignLeft, rt.MapDigitValue), indet: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.indet {
				if tc.got.Determinate() {
					t.Fatalf("expected an indeterminate sum, got %d", tc.got.Value())
				}
				return
			}
			if !tc.got.Determinate() || tc.got.Value() != tc.want {
				t.Fatalf("got %d (determinate %v), want %d", tc.got.Value(), tc.got.Determinate(), tc.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	v := rt.Value("123")
	if got := rt.CompareDigit(rt.Number(3), v, 2); got != rt.Valid {
		t.Fatalf("compare_digit: got %v", got)
	}
	if got := rt.CompareDigit(rt.Number(9), v, 2); got != rt.Invalid {
		t.Fatalf("compare_digit mismatch: got %v", got)
	}
	if got := rt.CompareDigit(rt.Number(3), v, 9); got != rt.Unsupported {
		t.Fatalf("compare_digit past the end: got %v", got)
	}
	if got := rt.CompareDigit(rt.Indeterminate, v, 2); got != rt.Unsupported {
		t.Fatalf("an indeterminate integer must be unsupported: got %v", got)
	}
	if got := rt.CompareDigit(rt.Number(1), rt.Value("A"), 0); got != rt.Unsupported {
		t.Fatalf("a non digit must be unsupported: got %v", got)
	}
	if got := rt.CompareSlice(rt.Number(12), v, 0, 2); got != rt.Valid {
		t.Fatalf("compare_slice: got %v", got)
	}
	if got := rt.CompareSlice(rt.Number(13), v, 0, 2); got != rt.Invalid {
		t.Fatalf("compare_slice mismatch: got %v", got)
	}
	if got := rt.CompareSlice(rt.Number(1), v, 0, 9); got != rt.Unsupported {
		t.Fatalf("compare_slice past the end: got %v", got)
	}
	if got := rt.CompareSlice(rt.Number(1), rt.Value("AB"), 0, 2); got != rt.Unsupported {
		t.Fatalf("compare_slice over non digits: got %v", got)
	}
}

// TestWeightedSumAlphabet mirrors TestWeightedSum over an issuer's own
// alphabet. The three alignments must pair exactly as they do for the fixed
// mappings, and a code point outside the alphabet must be indeterminate wherever
// it sits.
func TestWeightedSumAlphabet(t *testing.T) {
	weights := [2]int64{1, 10}
	// The alphabet of the Chinese unified social credit code: the ten digits,
	// then the letters that cannot be misread as one. J is 18, not 19.
	const uscc = "0123456789ABCDEFGHJKLMNPQRTUWXY"
	tests := []struct {
		name  string
		got   rt.Int
		want  int64
		indet bool
	}{
		{name: "left", got: rt.WeightedSumAlphabet(rt.Value("123"), weights[:], rt.AlignLeft, uscc), want: 1*1 + 2*10},
		{name: "right", got: rt.WeightedSumAlphabet(rt.Value("123"), weights[:], rt.AlignRight, uscc), want: 3*10 + 2*1},
		{name: "cycle", got: rt.WeightedSumAlphabet(rt.Value("123"), weights[:], rt.AlignCycle, uscc), want: 1*1 + 2*10 + 3*1},
		{
			name: "a letter takes its index, not its base 36 value",
			got:  rt.WeightedSumAlphabet(rt.Value("J"), weights[:], rt.AlignLeft, uscc),
			want: 18,
		},
		{
			name:  "a letter the alphabet drops",
			got:   rt.WeightedSumAlphabet(rt.Value("I"), weights[:], rt.AlignLeft, uscc),
			indet: true,
		},
		{
			name:  "an unpaired code point outside the alphabet still counts",
			got:   rt.WeightedSumAlphabet(rt.Value("12I"), weights[:], rt.AlignLeft, uscc),
			indet: true,
		},
		{
			name: "a non ASCII alphabet pairs by code point, not by byte",
			// "éb" has three bytes: pairing by byte would read the second one
			// as a weight index of its own.
			got:  rt.WeightedSumAlphabet(rt.Value("éb"), weights[:], rt.AlignLeft, "aéb"),
			want: 1*1 + 2*10,
		},
		{
			name: "a non ASCII alphabet under RIGHT",
			got:  rt.WeightedSumAlphabet(rt.Value("aéb"), weights[:], rt.AlignRight, "aéb"),
			want: 1*1 + 2*10,
		},
		{
			name:  "a code point outside a non ASCII alphabet",
			got:   rt.WeightedSumAlphabet(rt.Value("z"), weights[:], rt.AlignLeft, "aéb"),
			indet: true,
		},
		{name: "absent", got: rt.WeightedSumAlphabet(rt.Absent, weights[:], rt.AlignLeft, uscc), indet: true},
		{name: "empty", got: rt.WeightedSumAlphabet(rt.Value(""), weights[:], rt.AlignLeft, uscc), indet: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.indet {
				if tc.got.Determinate() {
					t.Fatalf("expected an indeterminate sum, got %d", tc.got.Value())
				}
				return
			}
			if !tc.got.Determinate() || tc.got.Value() != tc.want {
				t.Fatalf("got %d (determinate %v), want %d", tc.got.Value(), tc.got.Determinate(), tc.want)
			}
		})
	}
}

// TestWeightedSumAlphabetMatchesFixedMappings states the property that keeps the
// two implementations honest: over the base 36 alphabet, the custom mapping must
// agree with ALNUM_BASE36 on every alignment.
func TestWeightedSumAlphabetMatchesFixedMappings(t *testing.T) {
	const base36 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	weights := [3]int64{7, 3, 1}
	for _, in := range []string{"1", "12", "123", "1234", "A1B2", "ZZZZZ", "0"} {
		for _, align := range []rt.Alignment{rt.AlignLeft, rt.AlignRight, rt.AlignCycle} {
			fixed := rt.WeightedSum(rt.Value(in), weights[:], align, rt.MapAlnumBase36)
			custom := rt.WeightedSumAlphabet(rt.Value(in), weights[:], align, base36)
			if fixed != custom {
				t.Errorf("%q under alignment %d: fixed %v, custom %v", in, align, fixed, custom)
			}
		}
	}
}

// TestViewPrimitivesTheShippedRulesDoNotReach covers the primitives no current
// rule uses. The emitted code calls into this package, so a primitive the
// bundle happens not to exercise today is untested code that a rule change
// would put on the hot path tomorrow. Absence is checked on each of them:
// section 1.1 of ir.md makes absence a value, never an error.
func TestViewPrimitivesTheShippedRulesDoNotReach(t *testing.T) {
	t.Run("rune length counts code points, not bytes", func(t *testing.T) {
		for _, tc := range []struct {
			in   rt.View
			want int
		}{
			{rt.Value("ABC"), 3},
			{rt.Value(""), 0},
			// Four code points, eight bytes.
			{rt.Value("éèêë"), 4},
			{rt.Absent, 0},
		} {
			if got := tc.in.RuneLen(); got != tc.want {
				t.Errorf("RuneLen(%q) = %d, want %d", tc.in.String(), got, tc.want)
			}
		}
	})

	t.Run("length_in", func(t *testing.T) {
		for _, tc := range []struct {
			in      rt.View
			lengths []int
			want    bool
		}{
			{rt.Value("ABC"), []int{1, 3, 5}, true},
			{rt.Value("ABC"), []int{1, 5}, false},
			{rt.Value("ABC"), nil, false},
			{rt.Value("éèê"), []int{3}, true},
			{rt.Absent, []int{0, 3}, false},
		} {
			if got := tc.in.LengthIn(tc.lengths...); got != tc.want {
				t.Errorf("LengthIn(%q, %v) = %v", tc.in.String(), tc.lengths, got)
			}
		}
	})

	t.Run("prefix_in", func(t *testing.T) {
		two := []rt.PrefixGroup{{Length: 2, Values: []string{"BE", "FR"}}}
		for _, tc := range []struct {
			in     rt.View
			groups []rt.PrefixGroup
			want   bool
		}{
			{rt.Value("FR123"), two, true},
			{rt.Value("BE123"), two, true},
			{rt.Value("DE123"), two, false},
			{rt.Value("FR123"), nil, false},
			{rt.Value("F"), two, false}, // shorter than the group
			{rt.Value("FR"), two, true}, // exactly the group length
			{rt.Absent, two, false},
			// Two lengths, which is the shape the German register rule takes.
			{rt.Value("B1000X"), []rt.PrefixGroup{
				{Length: 5, Values: []string{"A1000", "B1000"}},
				{Length: 6, Values: []string{"B1000Y", "B1000Z"}},
			}, true},
		} {
			if got := tc.in.PrefixInSorted(tc.groups); got != tc.want {
				t.Errorf("PrefixInSorted(%q, %v) = %v", tc.in.String(), tc.groups, got)
			}
		}
	})

	t.Run("concat", func(t *testing.T) {
		if got := rt.Concat(rt.Value("AB"), rt.Value(""), rt.Value("CD")); got.String() != "ABCD" {
			t.Errorf("Concat = %q", got.String())
		}
		if got := rt.Concat(); !got.IsEmpty() {
			t.Errorf("Concat of nothing = %q, want the empty value", got.String())
		}
		// One absent part makes the whole absent: a concatenation with a hole
		// has no value, and inventing one would fabricate an identifier.
		if got := rt.Concat(rt.Value("AB"), rt.Absent, rt.Value("CD")); !got.IsAbsent() {
			t.Errorf("Concat with an absent part = %q, want absent", got.String())
		}
	})

	t.Run("char_at_in on an index outside the value", func(t *testing.T) {
		if rt.Value("ABC").CharAtIn(9, "A") {
			t.Error("an index past the end matches nothing")
		}
		if rt.Value("ABC").CharAtIn(-1, "A") {
			t.Error("a negative index matches nothing")
		}
		if rt.Absent.CharAtIn(0, "A") {
			t.Error("absence matches nothing")
		}
	})

	t.Run("absence propagates through every view constructor", func(t *testing.T) {
		for name, got := range map[string]rt.View{
			"slice":         rt.Absent.Slice(0, 1),
			"slice_from":    rt.Absent.SliceFrom(1),
			"slice_to":      rt.Absent.SliceTo(1),
			"before_first":  rt.Absent.BeforeFirst("."),
			"after_first":   rt.Absent.AfterFirst("."),
			"strip_prefix":  rt.Absent.StripPrefix("A"),
			"concat":        rt.Concat(rt.Absent),
			"slice_from_ok": rt.Value("AB").SliceFrom(9),
		} {
			if !got.IsAbsent() {
				t.Errorf("%s produced %q, want absent", name, got.String())
			}
		}
	})
}

// TestPrefixInSortedAgreesWithAScan is the guard on replacing a scan with a
// search. Section 14 of engine.md wants the membership test not to be linear,
// and a binary search answers a different question from a scan the moment its
// precondition slips, so the two are compared exhaustively over a space small
// enough to enumerate and varied enough to hold every edge: prefixes of several
// lengths, subjects shorter than a group, subjects exactly a group's length,
// and subjects over an alphabet the prefixes do not use.
func TestPrefixInSortedAgreesWithAScan(t *testing.T) {
	// Every string over ABC of length 1 to 3, kept when its bytes sum odd, so
	// the set is neither everything nor an interval.
	var flat []string
	var build func(prefix string)
	build = func(prefix string) {
		if len(prefix) > 0 {
			sum := 0
			for i := 0; i < len(prefix); i++ {
				sum += int(prefix[i])
			}
			if sum%2 == 1 {
				flat = append(flat, prefix)
			}
		}
		if len(prefix) == 3 {
			return
		}
		for _, c := range "ABC" {
			build(prefix + string(c))
		}
	}
	build("")
	slices.Sort(flat)
	if len(flat) < 10 {
		t.Fatalf("only %d prefixes; the space is too small to prove anything", len(flat))
	}

	// Grouped by length, which is what the generator emits. Each group keeps
	// the order of the sorted list, so each group is sorted.
	var groups []rt.PrefixGroup
	for length := 1; length <= 3; length++ {
		var values []string
		for _, p := range flat {
			if len(p) == length {
				values = append(values, p)
			}
		}
		if !slices.IsSorted(values) {
			t.Fatalf("group of length %d is not sorted, so the search is unsound", length)
		}
		groups = append(groups, rt.PrefixGroup{Length: length, Values: values})
	}

	scan := func(s string) bool {
		for _, p := range flat {
			if strings.HasPrefix(s, p) {
				return true
			}
		}
		return false
	}

	// Every subject over ABCD of length 0 to 4. D appears in no prefix, so the
	// space covers misses that sort past the end of a group as well as inside.
	subjects := []string{""}
	for range 4 {
		var next []string
		for _, s := range subjects {
			for _, c := range "ABCD" {
				next = append(next, s+string(c))
			}
		}
		subjects = append(subjects, next...)
	}

	checked, matched := 0, 0
	for _, s := range subjects {
		want := scan(s)
		if got := rt.Value(s).PrefixInSorted(groups); got != want {
			t.Fatalf("PrefixInSorted(%q) = %v, a scan says %v", s, got, want)
		}
		checked++
		if want {
			matched++
		}
	}
	// A space where nothing matches, or everything does, would agree trivially.
	if matched == 0 || matched == checked {
		t.Fatalf("%d of %d subjects matched; the comparison is degenerate", matched, checked)
	}
	t.Logf("%d prefixes over %d groups, %d subjects, %d matching", len(flat), len(groups), checked, matched)
}

// TestPrefixInSortedDecidesMixedLengths is the case the corpus cannot carry.
// Every prefix_in of the shipped bundle holds one length, so no conformance
// case separates an engine that searches one whole sorted table from one that
// searches per length, and the first is wrong rather than slow. Section 9 of
// ir.md now refuses the mixed shape at load, so this table can no longer reach
// a generated engine; the search is checked against it anyway, because a rule
// that cannot arrive today is not a rule that answers correctly tomorrow.
func TestPrefixInSortedDecidesMixedLengths(t *testing.T) {
	// The example section 9 gives. "AB" is a prefix of "ABCD" and "ABA" is not,
	// and "ABA" is the greatest element not after "ABCD".
	flat := []string{"AB", "ABA"}
	subject := "ABCD"

	// What a single search over the whole table answers: it lands on "ABA",
	// which is not a prefix, and reports absence. This is the wrong answer,
	// asserted so that the trap is written down rather than described.
	i, _ := slices.BinarySearch(flat, subject)
	nearest := flat[i-1]
	if nearest != "ABA" {
		t.Fatalf("the nearest element is %q, so the example no longer illustrates the trap", nearest)
	}
	if strings.HasPrefix(subject, nearest) {
		t.Fatal("the nearest element is a prefix, so the example no longer illustrates the trap")
	}

	// What this engine answers, one group per length, which is what the
	// generator emits.
	groups := []rt.PrefixGroup{
		{Length: 2, Values: []string{"AB"}},
		{Length: 3, Values: []string{"ABA"}},
	}
	if !rt.Value(subject).PrefixInSorted(groups) {
		t.Fatal("a per length search must find AB in ABCD")
	}

	// And the general case, against a scan taken as reference: every subject
	// over ABC up to length 4, against a table of every string over AB of
	// length 1 to 3, grouped. A whole table search disagrees with the scan
	// somewhere in this space; a per length search must not, anywhere.
	var table []string
	var build func(string)
	build = func(p string) {
		if p != "" {
			table = append(table, p)
		}
		if len(p) == 3 {
			return
		}
		for _, c := range "AB" {
			build(p + string(c))
		}
	}
	build("")
	slices.Sort(table)

	var byLength []rt.PrefixGroup
	for length := 1; length <= 3; length++ {
		var values []string
		for _, v := range table {
			if len(v) == length {
				values = append(values, v)
			}
		}
		byLength = append(byLength, rt.PrefixGroup{Length: length, Values: values})
	}

	subjects := []string{""}
	for range 4 {
		var next []string
		for _, s := range subjects {
			for _, c := range "ABC" {
				next = append(next, s+string(c))
			}
		}
		subjects = append(subjects, next...)
	}

	scan := func(s string) bool {
		for _, p := range table {
			if strings.HasPrefix(s, p) {
				return true
			}
		}
		return false
	}
	wholeTable := func(s string) bool {
		j, _ := slices.BinarySearch(table, s)
		if j == 0 {
			return false
		}
		return strings.HasPrefix(s, table[j-1])
	}

	wrong := 0
	for _, s := range subjects {
		want := scan(s)
		if got := rt.Value(s).PrefixInSorted(byLength); got != want {
			t.Errorf("per length search on %q = %v, a scan says %v", s, got, want)
		}
		if wholeTable(s) != want {
			wrong++
		}
	}
	// If a whole table search happened to agree everywhere, this space would
	// prove nothing about the difference between the two designs.
	if wrong == 0 {
		t.Fatal("a whole table search agrees everywhere here, so the space does not separate the designs")
	}
	t.Logf("%d subjects, %d of them answered wrongly by a whole table search, none by a per length one",
		len(subjects), wrong)
}

// TestPrefixInSortedGroupsByBytes pins the unit. Section 9 of ir.md states the
// single element length of a prefix_in in UTF-8 bytes, because the search is
// over bytes, and notes that two elements of one byte length may differ in code
// points: PZ and é are both two bytes and are not both two code points. No
// conformance case separates the two readings, since every element of the
// published bundle is ASCII, where they agree.
//
// This search slices bytes and compares bytes, so the byte reading is the one
// it implements. What that has to be worth is agreement with the definition
// taken literally, which is what the reference below is.
func TestPrefixInSortedGroupsByBytes(t *testing.T) {
	const pz, eacute = "PZ", "é"
	if len(pz) != 2 || len(eacute) != 2 {
		t.Fatalf("the example needs two elements of two bytes, got %d and %d", len(pz), len(eacute))
	}
	if utf8.RuneCountInString(pz) == utf8.RuneCountInString(eacute) {
		t.Fatal("the example needs the two to differ in code points, or it pins nothing")
	}

	// Sorted by bytes, which is the order the loader requires and the order a
	// search needs: 0x50 before 0xC3.
	values := []string{pz, eacute}
	if !slices.IsSorted(values) {
		t.Fatal("the group is not sorted, so the search is unsound")
	}
	groups := []rt.PrefixGroup{{Length: 2, Values: values}}

	scan := func(s string) bool {
		for _, p := range values {
			if strings.HasPrefix(s, p) {
				return true
			}
		}
		return false
	}

	for _, subject := range []string{
		"PZAB", "PZ", "éX", eacute, "P", "", "PA", "Pé", "ée", "\xc3", "\xc3\xa9\xc3",
	} {
		want := scan(subject)
		if got := rt.Value(subject).PrefixInSorted(groups); got != want {
			t.Errorf("PrefixInSorted(%q) = %v, the definition says %v", subject, got, want)
		}
	}

	// The case the two readings would disagree on if the search grouped by code
	// points while calling the count a byte length: "Pé" opens with one byte of
	// PZ and one of é and is a prefix of neither.
	if rt.Value("Pé").PrefixInSorted(groups) {
		t.Error("a subject sharing no whole element must not match")
	}
}
