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
