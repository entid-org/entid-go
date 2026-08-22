// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid_test

import (
	"errors"
	"fmt"
	"slices"

	businessid "github.com/libbusinessid/businessid-go"
)

func ExampleEngine_Validate() {
	engine := businessid.New()

	report, err := engine.Validate(businessid.Input{
		Kind:  "vat",
		Value: "BE 0123.456.749",
	})
	if err != nil {
		panic(err)
	}

	fmt.Println("canonical:", report.CanonicalValue)
	fmt.Println("country:  ", report.CountryCode)
	fmt.Println("format:   ", report.Format.Status, report.Format.Reason)
	fmt.Println("checksum: ", report.Checksum.Status, report.Checksum.Reason)
	// Output:
	// canonical: BE0123456749
	// country:   BE
	// format:    valid ok
	// checksum:  valid ok
}

// An identifier whose format holds but whose checksum algorithm is not
// published is Unsupported, never Invalid. Treating Unsupported as a rejection
// would turn incomplete coverage into a false negative.
func ExampleEngine_Validate_unpublishedChecksum() {
	report, _ := businessid.New().Validate(businessid.Input{
		Kind:  "vat",
		Value: "DE123456789",
	})

	fmt.Println("format:  ", report.Format.Status)
	fmt.Println("checksum:", report.Checksum.Status, report.Checksum.Reason)
	fmt.Println("OK:      ", report.OK())
	// Output:
	// format:   valid
	// checksum: unsupported unsupported_checksum
	// OK:       false
}

// A country hint disambiguates a value that carries no country prefix, and
// contradicts one that carries a different country.
func ExampleEngine_Validate_countryHint() {
	engine := businessid.New()

	hinted, _ := engine.Validate(businessid.Input{
		Kind: "vat", Value: "0123456749", CountryCode: "BE",
	})
	fmt.Println("hinted:  ", hinted.CanonicalValue, hinted.Format.Status)

	clashing, _ := engine.Validate(businessid.Input{
		Kind: "vat", Value: "BE0123456749", CountryCode: "FR",
	})
	fmt.Println("clashing:", clashing.Format.Status, clashing.Format.Reason)
	// Output:
	// hinted:   BE0123456749 valid
	// clashing: invalid country_mismatch
}

// The strict_current profile refuses variants that are no longer issued. It is
// always opt-in: compatible is the normative default, and neither profile
// changes the canonical form.
func ExampleEngine_Validate_profiles() {
	engine := businessid.New()
	in := businessid.Input{Kind: "vat", Value: "FRK7012345674"}

	compatible, _ := engine.Validate(in)
	fmt.Println("compatible:   ", compatible.Format.Status)

	in.Profile = businessid.StrictCurrent
	strict, _ := engine.Validate(in)
	fmt.Println("strict:       ", strict.Format.Status, strict.Format.Reason)
	fmt.Println("same canonical:", compatible.CanonicalValue == strict.CanonicalValue)
	// Output:
	// compatible:    valid
	// strict:        invalid invalid_characters
	// same canonical: true
}

// Canonicalize answers "what is this value, normalized" without running any
// rule. It is what a caller stores, or uses as a key.
func ExampleEngine_Canonicalize() {
	got, _ := businessid.New().Canonicalize(businessid.Input{
		Kind:  "lei",
		Value: "0000-0000-0000-0000-0098",
	})

	fmt.Println(got.CanonicalValue, got.Status)
	// Output: 00000000000000000098 valid
}

// Coverage answers "what does this engine actually know", with the source
// behind every rule. It grows with every rules release, so a caller filters it
// rather than assuming a fixed list.
func ExampleEngine_Coverage() {
	for _, c := range businessid.New().Coverage() {
		if c.Kind != "vat" || c.CountryCode != "DE" {
			continue
		}
		fmt.Println("kind:      ", c.Kind, c.CountryCode)
		fmt.Println("checksum:  ", c.HasChecksum, c.AbsentChecksumReason)
		fmt.Println("authority: ", c.Sources[0].Authority)
		fmt.Println("tier:      ", c.Sources[0].Tier)
	}
	// Output:
	// kind:       vat DE
	// checksum:   false unsupported_checksum
	// authority:  Bundeszentralamt fuer Steuern (BZSt)
	// tier:       primary
}

// ValidateFormat stops after the shape. It suits a form that validates while
// the user types, where computing a check digit on a half-entered value says
// nothing useful.
func ExampleEngine_ValidateFormat() {
	report, _ := businessid.New().ValidateFormat(businessid.Input{
		Kind:  "siren",
		Value: "012345674",
	})

	fmt.Println("format:  ", report.Format.Status)
	fmt.Println("checksum:", report.Checksum.Status, report.Checksum.Reason)
	// Output:
	// format:   valid
	// checksum: not_run not_requested
}

// A report says why it concluded, and the reason is what a caller branches on.
// Rejecting on Unsupported would turn incomplete coverage into a false
// negative, so only Invalid justifies refusing a value.
func ExampleReport() {
	engine := businessid.New()

	for _, value := range []string{"012345674", "012345675", "0123"} {
		report, _ := engine.Validate(businessid.Input{Kind: "siren", Value: value})

		switch {
		case report.OK():
			fmt.Printf("%s: accepted\n", value)
		case report.Format.Status == businessid.Invalid,
			report.Checksum.Status == businessid.Invalid:
			fmt.Printf("%s: rejected, %s\n", value, worstReason(report))
		default:
			fmt.Printf("%s: no verdict, %s\n", value, worstReason(report))
		}
	}
	// Output:
	// 012345674: accepted
	// 012345675: rejected, invalid_checksum
	// 0123: rejected, invalid_length
}

// worstReason picks the step that decided the outcome.
func worstReason(r businessid.Report) businessid.Reason {
	if r.Format.Status != businessid.Valid {
		return r.Format.Reason
	}
	return r.Checksum.Reason
}

// A message key names an entry in the caller's own catalogue, so that a user
// facing message can be translated without parsing anything.
func ExampleStep_messageKey() {
	report, _ := businessid.New().Validate(businessid.Input{
		Kind:  "vat",
		Value: "BE01234567",
	})

	fmt.Println(report.Format.Reason, report.Format.MessageKey)
	// Output: invalid_length vat.be.length
}

// A value that is not valid UTF-8, or one above 1024 bytes, is refused before
// any rule runs and reported verbatim. Both are safety bounds, not verdicts.
func ExampleEngine_Validate_safetyBounds() {
	report, _ := businessid.New().Validate(businessid.Input{
		Kind:  "siren",
		Value: "0123456\xff",
	})

	fmt.Println(report.Format.Status, report.Format.Reason)
	fmt.Println("value returned unchanged:", report.CanonicalValue == "0123456\xff")
	// Output:
	// unsupported invalid_encoding
	// value returned unchanged: true
}

// Kinds lists what the engine can route, which is what a caller offers in a
// dropdown rather than hard-coding. The list grows with every rules release.
func ExampleEngine_Kinds() {
	kinds := businessid.New().Kinds()

	fmt.Println(slices.Contains(kinds, "vat"), slices.Contains(kinds, "lei"))
	// Output: true true
}

// An unknown profile is the only error the API raises; every value, however
// malformed, is answered with a verdict instead.
func ExampleEngine_Validate_errors() {
	_, err := businessid.New().Validate(businessid.Input{
		Kind:    "siren",
		Value:   "012345674",
		Profile: "lenient",
	})

	fmt.Println(errors.Is(err, businessid.ErrUnknownProfile))
	// Output: true
}
