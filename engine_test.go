// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid_test

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	businessid "github.com/libbusinessid/businessid-go"
)

// lockValue reads one entry of rules.lock. The tests below assert against the
// lock rather than against numbers typed here, so that a rules synchronization
// that nobody regenerated fails instead of quietly passing.
func lockValue(t *testing.T, key string) string {
	t.Helper()
	raw, err := os.ReadFile("rules.lock")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^` + key + `\s*=\s*"([^"]+)"`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatalf("rules.lock declares no %s", key)
	}
	return m[1]
}

// Conformance is not checked here. Section 8.7 of the specification gives that
// job to the runner of the spec repository, driving cmd/businessid-testee: an
// engine that graded itself could declare conformance by comparing too weakly.
// What follows checks the contract of the public API instead.

func TestEngineIsUsableWithoutSetup(t *testing.T) {
	engine := businessid.New()
	if want := lockValue(t, "rules_version"); engine.RulesVersion() != want {
		t.Fatalf("rules version %q, but rules.lock names %q; run go generate ./...",
			engine.RulesVersion(), want)
	}
	if engine.FormatVersion() != 1 {
		t.Fatalf("format version %d", engine.FormatVersion())
	}
	// The zero value works too: the engine holds no state.
	var zero businessid.Engine
	if _, err := zero.Validate(businessid.Input{Kind: "siren", Value: "012345674"}); err != nil {
		t.Fatalf("the zero Engine must be usable: %v", err)
	}
}

func TestProfileDefaultsToCompatible(t *testing.T) {
	engine := businessid.New()
	got, err := engine.Validate(businessid.Input{Kind: "siren", Value: "012345674"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != businessid.Compatible {
		t.Fatalf("profile %q, want compatible", got.Profile)
	}
}

func TestUnknownProfileIsRefused(t *testing.T) {
	engine := businessid.New()
	in := businessid.Input{Kind: "siren", Value: "012345674", Profile: "lenient"}

	for _, call := range []struct {
		name string
		run  func() error
	}{
		{"Validate", func() error { _, err := engine.Validate(in); return err }},
		{"ValidateFormat", func() error { _, err := engine.ValidateFormat(in); return err }},
		{"ValidateChecksum", func() error { _, err := engine.ValidateChecksum(in); return err }},
		{"Canonicalize", func() error { _, err := engine.Canonicalize(in); return err }},
	} {
		if err := call.run(); !errors.Is(err, businessid.ErrUnknownProfile) {
			t.Errorf("%s: got %v, want ErrUnknownProfile", call.name, err)
		}
	}
}

func TestReportOK(t *testing.T) {
	engine := businessid.New()
	tests := []struct {
		name string
		in   businessid.Input
		want bool
	}{
		{"a valid identifier", businessid.Input{Kind: "siren", Value: "012345674"}, true},
		{"a wrong check digit", businessid.Input{Kind: "siren", Value: "012345675"}, false},
		// A definition with no published checksum never reports OK, because an
		// unsupported checksum is not a verdict.
		{"no published checksum", businessid.Input{Kind: "vat", Value: "DE123456789"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := engine.Validate(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got.OK() != tc.want {
				t.Fatalf("OK() = %v, want %v (%s/%s)", got.OK(), tc.want, got.Format.Status, got.Checksum.Status)
			}
		})
	}
}

func TestValidateChecksumMatchesValidate(t *testing.T) {
	engine := businessid.New()
	for _, value := range []string{"012345674", "012345675", "0123", ""} {
		in := businessid.Input{Kind: "siren", Value: value}
		a, err := engine.Validate(in)
		if err != nil {
			t.Fatal(err)
		}
		b, err := engine.ValidateChecksum(in)
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Errorf("%q: ValidateChecksum and Validate disagree", value)
		}
	}
}

func TestInputIsReportedUnchanged(t *testing.T) {
	engine := businessid.New()
	for _, value := range []string{" be 0123.456.749 ", "", strings.Repeat("1", 2000), "é"} {
		got, err := engine.Validate(businessid.Input{Kind: "vat", Value: value})
		if err != nil {
			t.Fatal(err)
		}
		if got.Input != value {
			t.Errorf("the raw input must come back unchanged, got %q for %q", got.Input, value)
		}
	}
}

// TestMalformedUTF8IsRefusedBeforeAnyRule checks the safety bound of section 5
// of ir.md. An identifier is a sequence of code points, and bytes that do not
// form one have none to evaluate, so the value is refused verbatim rather than
// canonicalized: a step filtering by code point would substitute U+FFFD for
// every malformed byte and change the value it reports.
//
// Step 1 of that section requires each engine to pin invalid_encoding with a
// native test, because no conformance case can carry it: a proto3 string is
// valid UTF-8 by definition, on the wire and in the corpus, so the corpus has
// no way to spell an ill formed value. This is that test, and the malformed
// form it names is the one a Go string admits: a Go string is an arbitrary
// byte sequence, so any byte that starts no valid UTF-8 sequence reaches the
// public API unchanged. The values below are the three shapes of that: a
// lone 0xff, which no sequence may contain; a truncated multi byte sequence;
// and a surrogate, whose bytes are well formed but whose code point UTF-8
// forbids.
func TestMalformedUTF8IsRefusedBeforeAnyRule(t *testing.T) {
	engine := businessid.New()

	for _, value := range []string{
		"01234567\xff",
		"\xff",
		"BE\xef\xbb\xef\xbb\xbf\xbf",
		"\xed\xa0\x80", // a surrogate, which UTF-8 forbids
	} {
		report, err := engine.Validate(businessid.Input{Kind: "siren", Value: value})
		if err != nil {
			t.Fatal(err)
		}
		if report.Format.Status != businessid.Unsupported ||
			report.Format.Reason != businessid.ReasonInvalidEncoding {
			t.Errorf("%q: got %s/%s, want unsupported/invalid_encoding",
				value, report.Format.Status, report.Format.Reason)
		}
		if report.Checksum.Status != businessid.NotRun {
			t.Errorf("%q: the checksum must not run behind a refused encoding", value)
		}
		if report.CanonicalValue != value {
			t.Errorf("%q: the value must be reported verbatim, got %q", value, report.CanonicalValue)
		}
		canonical, err := engine.Canonicalize(businessid.Input{Kind: "siren", Value: value})
		if err != nil {
			t.Fatal(err)
		}
		if canonical.Status != businessid.Unsupported ||
			canonical.Reason != businessid.ReasonInvalidEncoding {
			t.Errorf("%q: Canonicalize got %s/%s, want unsupported/invalid_encoding",
				value, canonical.Status, canonical.Reason)
		}
		if canonical.CanonicalValue != value {
			t.Errorf("%q: Canonicalize must report the value verbatim, got %q", value, canonical.CanonicalValue)
		}
	}

	// A well formed value is untouched by the check.
	report, err := engine.Validate(businessid.Input{Kind: "siren", Value: "012345674"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK() {
		t.Fatal("a well formed value must still pass")
	}
}

func TestCoverageDescribesEveryDefinition(t *testing.T) {
	engine := businessid.New()
	coverage := engine.Coverage()
	if len(coverage) == 0 {
		t.Fatal("the engine claims to implement nothing")
	}
	seen := map[string]bool{}
	for _, c := range coverage {
		seen[c.Kind+"/"+c.CountryCode] = true
		if c.DefaultProfile != businessid.Compatible && c.DefaultProfile != businessid.StrictCurrent {
			t.Errorf("%s: unknown default profile %q", c.Kind, c.DefaultProfile)
		}
		if c.HasChecksum && c.AbsentChecksumReason != businessid.ReasonUnspecified {
			t.Errorf("%s: a definition with a checksum must carry no absence reason", c.Kind)
		}
		if !c.HasChecksum && c.AbsentChecksumReason == businessid.ReasonUnspecified {
			t.Errorf("%s: a definition without a checksum must explain why", c.Kind)
		}
		if len(c.Sources) == 0 {
			t.Errorf("%s: a rule that can reject an input must carry a source", c.Kind)
		}
		for _, src := range c.Sources {
			if src.ID == "" || src.Authority == "" {
				t.Errorf("%s: a source must name itself and its authority", c.Kind)
			}
			if src.Tier != businessid.TierPrimary && src.Tier != businessid.TierSecondary {
				t.Errorf("%s: source %s carries no tier", c.Kind, src.ID)
			}
		}
	}
	// A sample the rules have carried since the first release. It is not an
	// exhaustive list: coverage grows, and a test that enumerated it would
	// have to be rewritten on every synchronization.
	for _, want := range []string{"siren/FR", "lei/", "euid/FR", "vat/BE", "vat/DE", "vat/FR", "vat/GR"} {
		if !seen[want] {
			t.Errorf("coverage does not mention %s", want)
		}
	}
}

func TestKindsAndCapabilities(t *testing.T) {
	engine := businessid.New()
	kinds := engine.Kinds()
	if len(kinds) == 0 {
		t.Fatal("the engine claims to route nothing")
	}
	// Kinds and Coverage must describe the same engine.
	fromCoverage := map[string]bool{}
	for _, c := range engine.Coverage() {
		fromCoverage[c.Kind] = true
	}
	for _, k := range kinds {
		if !fromCoverage[k] {
			t.Errorf("Kinds lists %q, which Coverage does not describe", k)
		}
		delete(fromCoverage, k)
	}
	for k := range fromCoverage {
		t.Errorf("Coverage describes %q, which Kinds does not list", k)
	}
	// The accessors must hand out copies, never the engine's own tables.
	kinds[0] = "mutated"
	if engine.Kinds()[0] == "mutated" {
		t.Fatal("Kinds exposes its backing array")
	}
	caps := engine.RequiredCapabilities()
	caps[0] = 999
	if engine.RequiredCapabilities()[0] == 999 {
		t.Fatal("RequiredCapabilities exposes its backing array")
	}
	// The generator refuses a bundle needing a capability it does not
	// implement, so every required id must be supported.
	supported := map[uint32]bool{}
	for _, id := range businessid.SupportedCapabilities() {
		supported[id] = true
	}
	for _, id := range engine.RequiredCapabilities() {
		if !supported[id] {
			t.Errorf("capability %d is required but not supported", id)
		}
	}
}

func TestStringers(t *testing.T) {
	if businessid.Valid.String() != "valid" || businessid.NotRun.String() != "not_run" {
		t.Error("Status.String is wrong")
	}
	if businessid.LevelFormat.String() != "format" || businessid.LevelChecksum.String() != "checksum" {
		t.Error("Level.String is wrong")
	}
	if businessid.ReasonInvalidChecksum.String() != "invalid_checksum" {
		t.Error("Reason.String is wrong")
	}
	if businessid.Reason(200).String() != "unspecified" {
		t.Error("an out of range Reason must render as unspecified")
	}
	if businessid.Compatible.String() != "compatible" {
		t.Error("Profile.String is wrong")
	}
}

// TestValidateAllocations pins the allocation budget, which is the measure that
// justifies compiling the rules instead of interpreting them.
func TestValidateAllocations(t *testing.T) {
	engine := businessid.New()
	var sink businessid.Report

	tests := []struct {
		name string
		in   businessid.Input
		want float64
	}{
		{
			// Already canonical: nothing is copied, nothing is allocated.
			name: "a canonical value",
			in:   businessid.Input{Kind: "vat", Value: "BE0123456749"},
			want: 0,
		},
		{
			name: "a canonical value with an explicit country",
			in:   businessid.Input{Kind: "vat", Value: "BE0123456749", CountryCode: "BE"},
			want: 0,
		},
		{
			name: "a canonical SIREN",
			in:   businessid.Input{Kind: "siren", Value: "012345674"},
			want: 0,
		},
		{
			name: "an unknown kind",
			in:   businessid.Input{Kind: "nope", Value: "x"},
			want: 0,
		},
		{
			// Canonicalization rewrites the value, so the result is built once.
			name: "a value needing canonicalization",
			in:   businessid.Input{Kind: "vat", Value: "be 0123.456.749"},
			want: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := testing.AllocsPerRun(200, func() {
				sink, _ = engine.Validate(tc.in)
			})
			if got > tc.want {
				t.Errorf("%.0f allocations, want at most %.0f", got, tc.want)
			}
			t.Logf("%s: %.0f allocations", tc.name, got)
		})
	}
	_ = sink
}

// TestSiretComposesOnSiren pins the restriction the SIRET rule carries: the
// fourteen digit Luhn is not enough, the nine digit head must hold its own
// SIREN check. Nine SIRET shaped strings out of ten pass the outer Luhn while
// failing that, so an engine that skipped the composition would look correct on
// a handful of valid samples and accept a large population of invalid ones.
//
// The values are synthetic: they are digit strings that satisfy an arithmetic
// property, not identifiers taken from any register.
func TestSiretComposesOnSiren(t *testing.T) {
	engine := businessid.New()
	composed := 0
	for seed := 0; seed < 200000 && composed < 50; seed++ {
		value := fmt.Sprintf("%014d", seed)
		if !luhnHolds(value) || luhnHolds(value[:9]) {
			continue
		}
		composed++
		report, err := engine.Validate(businessid.Input{Kind: "siret", Value: value})
		if err != nil {
			t.Fatal(err)
		}
		if report.Format.Status != businessid.Valid {
			t.Fatalf("%s: the shape must hold, got %s %s", value, report.Format.Status, report.Format.Reason)
		}
		if report.Checksum.Status != businessid.Invalid {
			t.Fatalf("%s passes the fourteen digit Luhn but its head %s does not, so the checksum must be invalid, got %s",
				value, value[:9], report.Checksum.Status)
		}
		// The head is what the composition refuses, and it must be refused on
		// its own too.
		head, err := engine.Validate(businessid.Input{Kind: "siren", Value: value[:9]})
		if err != nil {
			t.Fatal(err)
		}
		if head.Checksum.Status != businessid.Invalid {
			t.Fatalf("%s: the head must be invalid on its own, got %s", value[:9], head.Checksum.Status)
		}
	}
	if composed < 50 {
		t.Fatalf("only %d values exercised the composition", composed)
	}
}

// luhnHolds is the test's own oracle, deliberately independent of the engine.
func luhnHolds(digits string) bool {
	sum, double := 0, false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// TestPreCanonicalizationPrecedesTheCountryDecision covers step 4 of section 5
// of ir.md: the pre-canonicalizer runs as soon as the dispatcher resolves, so a
// result that stops at the country decision still reports the pre-canonical
// value. engine.md and spec.md stated the opposite order until 2026.08.14, and
// an engine that followed either reports the raw value here.
//
// Step 3 is the boundary: an unresolved kind returns before any program runs,
// so that one alone reports the value verbatim.
func TestPreCanonicalizationPrecedesTheCountryDecision(t *testing.T) {
	engine := businessid.New()
	// Dirty enough that the pre-canonicalizer must have run for the reported
	// value to differ: leading and inner whitespace, lower case, punctuation.
	const dirty = "  be 0123.456-749  "
	const pre = "BE0123456749"

	for _, tc := range []struct {
		name    string
		in      businessid.Input
		want    string
		status  businessid.Status
		reason  businessid.Reason
		country string
	}{
		{
			// Section 5.1: with no definition selected the report carries the
			// normalized country when one exists, and this token normalizes to
			// nothing, so the raw context stands.
			name:    "a country token that is not two letters",
			in:      businessid.Input{Kind: "vat", Value: dirty, CountryCode: "belgium"},
			want:    pre,
			status:  businessid.Unsupported,
			reason:  businessid.ReasonUnsupportedCountry,
			country: "belgium",
		},
		{
			name:    "a well formed country the dispatcher has no target for",
			in:      businessid.Input{Kind: "vat", Value: dirty, CountryCode: "ZZ"},
			want:    pre,
			status:  businessid.Unsupported,
			reason:  businessid.ReasonUnsupportedCountry,
			country: "ZZ",
		},
		{
			name:    "a country contradicting the prefix",
			in:      businessid.Input{Kind: "vat", Value: dirty, CountryCode: "fr"},
			want:    pre,
			status:  businessid.Invalid,
			reason:  businessid.ReasonCountryMismatch,
			country: "FR",
		},
		{
			name:   "no country and no prefix to select on",
			in:     businessid.Input{Kind: "vat", Value: "  0123.456-749  "},
			want:   "0123456749",
			status: businessid.Unsupported,
			reason: businessid.ReasonMissingCountryCode,
		},
		{
			// Step 3 precedes step 4: no dispatcher, so no program ran and
			// the value is reported exactly as it was submitted.
			name:   "an unresolved kind stops before any program",
			in:     businessid.Input{Kind: "not-an-identifier", Value: dirty},
			want:   dirty,
			status: businessid.Unsupported,
			reason: businessid.ReasonUnsupportedKind,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := engine.Validate(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if report.CanonicalValue != tc.want {
				t.Errorf("value %q, want %q", report.CanonicalValue, tc.want)
			}
			if report.Format.Status != tc.status || report.Format.Reason != tc.reason {
				t.Errorf("format %s %s, want %s %s",
					report.Format.Status, report.Format.Reason, tc.status, tc.reason)
			}
			if report.CountryCode != tc.country {
				t.Errorf("country %q, want %q", report.CountryCode, tc.country)
			}
			// Canonicalize answers the same question through another entry
			// point, and must not disagree about the value.
			canonical, err := engine.Canonicalize(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if canonical.CanonicalValue != tc.want {
				t.Errorf("Canonicalize returned %q, want %q", canonical.CanonicalValue, tc.want)
			}
		})
	}
}

// TestInputLengthIsCountedInBytesHeld pins the choice step 1 of section 6 of
// ir.md requires an engine to make and to state. The bound is expressed in
// UTF-8 bytes and is measured before the step that refuses ill formed text, so
// a language whose strings admit such text has to decide what to count.
//
// A Go string is an arbitrary byte sequence rather than code units awaiting an
// encoder, so this engine counts the bytes the string already holds. Nothing is
// encoded and nothing is invented. What that decides is the order between the
// two refusals, which is what this test measures.
func TestInputLengthIsCountedInBytesHeld(t *testing.T) {
	engine := businessid.New()
	const bound = 1024

	// One invalid byte, padded to sit either side of the bound. The byte is
	// counted exactly like any other, so the length alone decides.
	for _, tc := range []struct {
		name   string
		value  string
		reason businessid.Reason
	}{
		{"ill formed, one byte above the bound", strings.Repeat("0", bound) + "\xff", businessid.ReasonInputTooLong},
		{"ill formed, exactly at the bound", strings.Repeat("0", bound-1) + "\xff", businessid.ReasonInvalidEncoding},
		{"well formed, one byte above the bound", strings.Repeat("0", bound+1), businessid.ReasonInputTooLong},
		// Past the bound, an ordinary rule answers: 1024 digits is not a SIREN.
		{"well formed, exactly at the bound", strings.Repeat("0", bound), businessid.ReasonInvalidLength},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := engine.Validate(businessid.Input{Kind: "siren", Value: tc.value})
			if err != nil {
				t.Fatalf("neither case is an engine error: %v", err)
			}
			if report.Format.Reason != tc.reason {
				t.Fatalf("got %s, want %s", report.Format.Reason, tc.reason)
			}
			if report.CanonicalValue != tc.value {
				t.Error("the value must be reported verbatim")
			}
		})
	}

	// A multi byte code point is counted in bytes, not in code points: three
	// hundred and forty five U+00E9 are 690 bytes and pass the bound, while
	// five hundred and thirteen are 1026 bytes and do not.
	for _, tc := range []struct {
		count  int
		reason businessid.Reason
	}{{345, businessid.ReasonInvalidLength}, {513, businessid.ReasonInputTooLong}} {
		report, err := engine.Validate(businessid.Input{
			Kind: "siren", Value: strings.Repeat("é", tc.count),
		})
		if err != nil {
			t.Fatal(err)
		}
		if report.Format.Reason != tc.reason {
			t.Errorf("%d code points, %d bytes: got %s, want %s",
				tc.count, tc.count*2, report.Format.Reason, tc.reason)
		}
	}
}
