// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package runtime_test

import (
	"strings"
	"testing"
	"unsafe"

	rt "github.com/entid-org/entid-go/internal/runtime"
)

// The workspace a test uses. Real ones are sized by the generator from the
// growth each canonicalizer can add.
const (
	testMargin  = 64
	testScratch = testMargin + rt.MaxInput + testMargin
)

func TestBufferSteps(t *testing.T) {
	tests := []struct {
		name  string
		steps func(*rt.Buf)
		in    string
		want  string
	}{
		{
			name:  "trim_whitespace uses the frozen table",
			steps: func(w *rt.Buf) { w.TrimWhitespace() },
			// U+00A0, U+2028 and U+FEFF all belong to whitespace_v1.
			in: "\u00a0\u2028 AB\ufeff\t", want: "AB",
		},
		{
			name:  "trim_whitespace leaves interior whitespace alone",
			steps: func(w *rt.Buf) { w.TrimWhitespace() },
			in:    " A B ", want: "A B",
		},
		{
			name:  "trim_whitespace on an all whitespace value",
			steps: func(w *rt.Buf) { w.TrimWhitespace() },
			in:    " \t  ", want: "",
		},
		{
			name:  "remove_whitespace removes every occurrence",
			steps: func(w *rt.Buf) { w.RemoveWhitespace() },
			in:    " A B\u3000C ", want: "ABC",
		},
		{
			name:  "uppercase_ascii maps only a to z",
			steps: func(w *rt.Buf) { w.UppercaseASCII() },
			// The accented letters and the Turkish dotless i stay untouched.
			in: "abzéıü", want: "ABZéıü",
		},
		{
			name:  "remove_chars",
			steps: func(w *rt.Buf) { w.RemoveChars("-./") },
			in:    "0-1.2/3", want: "0123",
		},
		{
			name:  "remove_chars keeps wide code points",
			steps: func(w *rt.Buf) { w.RemoveChars("-") },
			in:    "é-è", want: "éè",
		},
		{
			name:  "replace_prefix when present",
			steps: func(w *rt.Buf) { w.ReplacePrefix("GR", "EL") },
			in:    "GR123", want: "EL123",
		},
		{
			name:  "replace_prefix leaves a non matching value alone",
			steps: func(w *rt.Buf) { w.ReplacePrefix("GR", "EL") },
			in:    "FRGR1", want: "FRGR1",
		},
		{
			name:  "replace_prefix with a shorter replacement",
			steps: func(w *rt.Buf) { w.ReplacePrefix("ABC", "X") },
			in:    "ABC123", want: "X123",
		},
		{
			name:  "replace_prefix with a longer replacement",
			steps: func(w *rt.Buf) { w.ReplacePrefix("A", "XYZ") },
			in:    "A123", want: "XYZ123",
		},
		{
			name:  "prepend",
			steps: func(w *rt.Buf) { w.Prepend("XX") },
			in:    "123", want: "XX123",
		},
		{
			name:  "append",
			steps: func(w *rt.Buf) { w.Append("ZZ") },
			in:    "123", want: "123ZZ",
		},
		{
			name:  "insert",
			steps: func(w *rt.Buf) { w.Insert(2, "0") },
			in:    "AB123", want: "AB0123",
		},
		{
			name:  "insert at the exact length appends",
			steps: func(w *rt.Buf) { w.Insert(3, "Z") },
			in:    "ABC", want: "ABCZ",
		},
		{
			name:  "insert past the end leaves the value unchanged",
			steps: func(w *rt.Buf) { w.Insert(9, "Z") },
			in:    "ABC", want: "ABC",
		},
		{
			name:  "insert counts code points",
			steps: func(w *rt.Buf) { w.Insert(2, "-") },
			in:    "éèê", want: "éè-ê",
		},
		{
			name:  "left_pad",
			steps: func(w *rt.Buf) { w.LeftPad(6, "0") },
			in:    "123", want: "000123",
		},
		{
			name:  "left_pad never truncates",
			steps: func(w *rt.Buf) { w.LeftPad(2, "0") },
			in:    "12345", want: "12345",
		},
		{
			name:  "left_pad counts code points",
			steps: func(w *rt.Buf) { w.LeftPad(4, "0") },
			in:    "éè", want: "00éè",
		},
		{
			name: "steps run in order",
			steps: func(w *rt.Buf) {
				w.TrimWhitespace()
				w.RemoveWhitespace()
				w.UppercaseASCII()
				w.RemoveChars("-./")
				w.Prepend("X")
			},
			in: " a-b.c ", want: "XABC",
		},
		{
			name: "a removal after a prepend stays consistent",
			steps: func(w *rt.Buf) {
				w.Prepend("--")
				w.RemoveChars("-")
			},
			in: "a-b", want: "ab",
		},
		{
			name: "an insert after a removal stays consistent",
			steps: func(w *rt.Buf) {
				w.RemoveChars("-")
				w.Insert(1, "!")
			},
			in: "a-b", want: "a!b",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var scratch [testScratch]byte
			w := rt.New(tc.in, scratch[:], testMargin)
			tc.steps(&w)
			got := tc.in
			if w.Modified() {
				got = w.String()
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBufferReportsWhetherAnythingChanged checks the property that keeps a
// clean input allocation free: a workspace no step touched says so, and the
// caller hands back its own string rather than a copy.
func TestBufferReportsWhetherAnythingChanged(t *testing.T) {
	var scratch [testScratch]byte
	w := rt.New("BE0123456749", scratch[:], testMargin)
	w.TrimWhitespace()
	w.RemoveWhitespace()
	w.UppercaseASCII()
	w.RemoveChars("-./")
	if w.Modified() {
		t.Fatal("no step changed the value, yet the workspace reports a change")
	}

	w = rt.New("  BE1  ", scratch[:], testMargin)
	w.TrimWhitespace()
	if !w.Modified() {
		t.Fatal("trimming changed the value, yet the workspace reports none")
	}
	if got := w.String(); got != "BE1" {
		t.Fatalf("got %q, want BE1", got)
	}
}

// TestBufferAllocations pins the allocation budget of a canonicalization: none
// at all when nothing changes, and a single one for the result otherwise.
func TestBufferAllocations(t *testing.T) {
	clean := testing.AllocsPerRun(100, func() {
		var scratch [testScratch]byte
		w := rt.New("BE0123456749", scratch[:], testMargin)
		w.TrimWhitespace()
		w.RemoveWhitespace()
		w.UppercaseASCII()
		w.RemoveChars("-./")
		if w.Modified() {
			_ = w.String()
		}
	})
	if clean != 0 {
		t.Errorf("an unchanged canonicalization allocated %.0f times, want 0", clean)
	}

	dirty := testing.AllocsPerRun(100, func() {
		var scratch [testScratch]byte
		w := rt.New("be 0123.456.749", scratch[:], testMargin)
		w.TrimWhitespace()
		w.RemoveWhitespace()
		w.UppercaseASCII()
		w.RemoveChars("-./")
		if w.Modified() {
			_ = w.String()
		}
	})
	if dirty > 1 {
		t.Errorf("a modified canonicalization allocated %.0f times, want at most 1", dirty)
	}
}

// TestBufferHandlesTheLargestInput checks that the workspace covers the
// specified input bound plus the growth the margin promises.
func TestBufferHandlesTheLargestInput(t *testing.T) {
	// The generator sizes a scratch from the growth its steps can add. This
	// stands in for one that prepends and appends the largest margin used here.
	const margin = 512
	var scratch [margin + rt.MaxInput + margin]byte

	in := strings.Repeat("a", rt.MaxInput)
	w := rt.New(in, scratch[:], margin)
	w.UppercaseASCII()
	w.Prepend(strings.Repeat("X", margin))
	w.Append(strings.Repeat("Z", margin))

	got := w.String()
	want := strings.Repeat("X", margin) + strings.Repeat("A", rt.MaxInput) + strings.Repeat("Z", margin)
	if got != want {
		t.Fatalf("got %d bytes, want %d", len(got), len(want))
	}
}

func TestTokenHelpers(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"  vat  ", "vat"}, {"\tVAT\r\n", "VAT"}, {"vat", "vat"}, {"   ", ""},
		// whitespace_v1 code points outside the ASCII class are not trimmed
		// here: dispatch trims ASCII only.
		{"\u00a0vat", "\u00a0vat"},
	} {
		if got := rt.TrimASCII(tc.in); got != tc.want {
			t.Errorf("TrimASCII(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"VAT", "vat"}, {"vat", "vat"}, {"VaT_1", "vat_1"}, {"ÉA", "Éa"},
	} {
		if got := rt.LowerASCII(tc.in); got != tc.want {
			t.Errorf("LowerASCII(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"fr", "FR"}, {"FR", "FR"}, {"fR", "FR"}, {"éa", "éA"},
	} {
		if got := rt.UpperASCII(tc.in); got != tc.want {
			t.Errorf("UpperASCII(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// A token that needs no mapping is returned without copying.
	in := "vat"
	if got := rt.LowerASCII(in); unsafe.StringData(got) != unsafe.StringData(in) {
		t.Error("LowerASCII copied a token that needed no change")
	}
}

func TestWhitespaceV1Table(t *testing.T) {
	inTable := []rune{
		0x0009, 0x000A, 0x000B, 0x000C, 0x000D, 0x0020, 0x0085, 0x00A0, 0x1680,
		0x2000, 0x2005, 0x200A, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF,
	}
	for _, r := range inTable {
		if !rt.IsWhitespaceV1(r) {
			t.Errorf("U+%04X must belong to whitespace_v1", r)
		}
	}
	// U+200B and U+180E are whitespace in some Unicode versions but are
	// deliberately outside the frozen table.
	for _, r := range []rune{'a', '0', 0x200B, 0x180E, 0x2060, 0x00B7} {
		if rt.IsWhitespaceV1(r) {
			t.Errorf("U+%04X must not belong to whitespace_v1", r)
		}
	}
}

// TestFilterNeverGrowsTheValue guards a defect the fuzzer found: ranging over a
// string yields U+FFFD for every byte that is not valid UTF-8, and re-encoding
// that replacement writes three bytes where one stood. A filter that grew its
// input would run past the workspace on a long enough value.
func TestFilterNeverGrowsTheValue(t *testing.T) {
	// A workspace with no room to grow at all: any growth would panic.
	const size = 64
	var scratch [size]byte

	for _, in := range []string{
		"\xff\xfe\xfd",
		strings.Repeat("\xbf", size),
		"a\xffb-c",
		"\xed\xa0\x80", // a surrogate, which UTF-8 forbids
		"\xf4\x90\x80\x80",
	} {
		w := rt.New(in, scratch[:], 0)
		w.RemoveChars("-")
		if w.Modified() && len(w.String()) > len(in) {
			t.Errorf("%q: filtering grew the value to %d bytes", in, len(w.String()))
		}

		w = rt.New(in, scratch[:], 0)
		w.RemoveWhitespace()
		if w.Modified() && len(w.String()) > len(in) {
			t.Errorf("%q: filtering grew the value to %d bytes", in, len(w.String()))
		}
	}
}

// TestFilterKeepsMalformedBytes checks that a byte UTF-8 rejects survives a
// filter unchanged: dropping it would silently rewrite a caller's value.
func TestFilterKeepsMalformedBytes(t *testing.T) {
	var scratch [testScratch]byte
	w := rt.New("a-\xffb", scratch[:], testMargin)
	w.RemoveChars("-")
	if got := w.String(); got != "a\xffb" {
		t.Fatalf("got %q, want %q", got, "a\xffb")
	}
}
