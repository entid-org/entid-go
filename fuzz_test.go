// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	businessid "github.com/libbusinessid/businessid-go"
)

// FuzzValidate drives arbitrary caller input through the public API.
//
// The value comes from whatever a caller received — a form, an invoice, a
// third party feed — so it is untrusted in shape if not in intent. No input may
// cause a panic, and the invariants below are the ones a caller relies on
// whatever happens.
func FuzzValidate(f *testing.F) {
	seeds := []struct{ kind, value, country, profile string }{
		{"vat", "BE 0123.456.749", "", "compatible"},
		{"vat", "FR09012345674", "FR", "strict_current"},
		{"siren", "012 345-674", "", ""},
		{"lei", "0000-0000-0000-0000-0098", "fr", "compatible"},
		{"euid", "FRTVX.012345674", "FR", "compatible"},
		{"  VAT  ", "GR012345670", "EL", "compatible"},
		{"nope", "", "belgium", "compatible"},
		{"vat", strings.Repeat("1", 2000), "", ""},
		{"vat", "\xff\xfe", "\xff", ""},
		{"vat", "\u00e9  \ufeff0123456749", "BE", ""},
	}
	for _, s := range seeds {
		f.Add(s.kind, s.value, s.country, s.profile)
	}

	engine := businessid.New()
	f.Fuzz(func(t *testing.T, kind, value, country, profile string) {
		in := businessid.Input{
			Kind: kind, Value: value, CountryCode: country,
			Profile: businessid.Profile(profile),
		}

		report, err := engine.Validate(in)
		if err != nil {
			// The only error the engine can raise is a profile outside the
			// frozen set; every other input is answered with a verdict.
			if profile != "" && profile != "compatible" && profile != "strict_current" {
				return
			}
			t.Fatalf("Validate refused a well formed request: %v", err)
		}

		if report.Input != value {
			t.Fatalf("the raw input must come back unchanged")
		}
		if report.Format.Level != businessid.LevelFormat {
			t.Fatalf("the format step must report the format level")
		}
		if report.Checksum.Level != businessid.LevelChecksum {
			t.Fatalf("the checksum step must report the checksum level")
		}
		if report.Format.Status == businessid.Unspecified || report.Checksum.Status == businessid.Unspecified {
			t.Fatalf("a step must always carry a status")
		}
		// A checksum only runs behind a valid format, so it is never computed
		// on a shape the rule was not designed for.
		if report.Format.Status != businessid.Valid && report.Checksum.Status != businessid.NotRun {
			t.Fatalf("checksum %s ran behind format %s", report.Checksum.Status, report.Format.Status)
		}
		// Only a rule assertion carries a message key.
		if report.Format.Status == businessid.Valid && report.Format.MessageKey != "" {
			t.Fatalf("a valid format must carry no message key")
		}
		assertReason(t, "format", report.Format)
		assertReason(t, "checksum", report.Checksum)

		// The other entry points must agree with Validate on everything the
		// pipeline decides before the checksum.
		canonical, err := engine.Canonicalize(in)
		if err != nil {
			t.Fatalf("Canonicalize refused what Validate accepted: %v", err)
		}
		if canonical.CanonicalValue != report.CanonicalValue ||
			canonical.Kind != report.Kind ||
			canonical.CountryCode != report.CountryCode {
			t.Fatalf("Canonicalize and Validate disagree on the identity")
		}
		if canonical.MessageKey != "" {
			t.Fatalf("canonicalization stops before any assertion and carries no message key")
		}

		format, err := engine.ValidateFormat(in)
		if err != nil {
			t.Fatalf("ValidateFormat refused what Validate accepted: %v", err)
		}
		if format.Format != report.Format {
			t.Fatalf("ValidateFormat and Validate disagree on the format step")
		}
		if format.Format.Status == businessid.Valid &&
			(format.Checksum.Status != businessid.NotRun || format.Checksum.Reason != businessid.ReasonNotRequested) {
			t.Fatalf("ValidateFormat must report the checksum as not requested")
		}

		// A value that is not valid UTF-8 is refused before any rule runs, and
		// reported verbatim. That is the bound section 6.6 of the
		// specification relies on to state idempotence over well formed input
		// alone: removing a code point can otherwise leave two malformed
		// fragments adjacent, and the pair may then decode as one code point
		// the next pass removes.
		// The byte limit is step 1 of the pipeline and the encoding check is
		// step 1 of the dispatch it then runs, so a value that is both too long
		// and malformed is refused for its length.
		if !utf8.ValidString(value) && len(value) <= 1024 {
			if report.Format.Reason != businessid.ReasonInvalidEncoding {
				t.Fatalf("malformed UTF-8 must be refused with invalid_encoding, got %s",
					report.Format.Reason)
			}
			if report.CanonicalValue != value {
				t.Fatalf("a refused encoding must be reported verbatim")
			}
			return
		}

		// Canonicalization is idempotent over well formed input: running it on
		// its own output changes nothing.
		if canonical.Status == businessid.Valid {
			again, err := engine.Canonicalize(businessid.Input{
				Kind: kind, Value: canonical.CanonicalValue,
				CountryCode: country, Profile: businessid.Profile(profile),
			})
			if err != nil {
				t.Fatal(err)
			}
			if again.Status == businessid.Valid && again.CanonicalValue != canonical.CanonicalValue {
				t.Fatalf("canonicalization is not idempotent: %q then %q",
					canonical.CanonicalValue, again.CanonicalValue)
			}
		}
	})
}

// assertReason checks that a status only ever carries a reason the registry
// allows it to carry.
func assertReason(t *testing.T, name string, step businessid.Step) {
	t.Helper()
	switch step.Status {
	case businessid.Valid:
		if step.Reason != businessid.ReasonOK {
			t.Fatalf("%s: valid must carry ok, got %s", name, step.Reason)
		}
	case businessid.NotRun:
		switch step.Reason {
		case businessid.ReasonNotRequested,
			businessid.ReasonNotRunFormatInvalid,
			businessid.ReasonNotRunFormatUnsupported:
		default:
			t.Fatalf("%s: not_run cannot carry %s", name, step.Reason)
		}
	case businessid.Invalid:
		// Invalid demands proof: only a documented, applicable rule produces it.
		switch step.Reason {
		case businessid.ReasonEmpty,
			businessid.ReasonInvalidLength,
			businessid.ReasonInvalidCharacters,
			businessid.ReasonInvalidFormat,
			businessid.ReasonInvalidChecksum,
			businessid.ReasonCountryMismatch:
		default:
			t.Fatalf("%s: invalid cannot carry %s", name, step.Reason)
		}
	case businessid.Unsupported:
		if step.Reason == businessid.ReasonOK {
			t.Fatalf("%s: unsupported cannot carry ok", name)
		}
	}
}
