// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid

// Profile selects how strict a validation is about historical variants.
//
// Compatible is the normative default: it accepts current variants and the
// documented historical ones that can still legitimately appear in data being
// processed today. StrictCurrent is opt-in and accepts only variants that are
// currently issued. Neither profile changes the shared canonicalization.
type Profile string

// The two frozen profiles. The zero value of Profile is neither: it means the
// caller supplied none, which lets the selected definition apply its own
// default, per section 5.2 of ir.md.
const (
	// Compatible is the normative default when nothing else applies.
	Compatible Profile = "compatible"

	// StrictCurrent refuses historical variants. It is always opt-in.
	StrictCurrent Profile = "strict_current"
)

func (p Profile) String() string { return string(p) }

// Status is the outcome of one validation step.
type Status uint8

// Statuses of the result contract.
const (
	// Unspecified is the zero value and never appears in a result.
	Unspecified Status = iota

	// Valid means a documented rule ran and the value satisfied it.
	Valid

	// Invalid means a documented, applicable rule proved the value wrong.
	// This is the only status that justifies rejecting an identifier.
	Invalid

	// Unsupported means no verdict: the rule is unknown, the algorithm is
	// unpublished, the variant is ambiguous, or the value never reached a
	// rule at all. Treating it as a rejection turns incomplete coverage into
	// a false negative.
	Unsupported

	// NotRun means the step was skipped, either because the caller did not
	// ask for it or because the format that guards it did not hold.
	NotRun
)

func (s Status) String() string {
	switch s {
	case Valid:
		return "valid"
	case Invalid:
		return "invalid"
	case Unsupported:
		return "unsupported"
	case NotRun:
		return "not_run"
	}
	return "unspecified"
}

// Level names the step a result belongs to.
type Level uint8

// Validation levels.
const (
	LevelUnspecified Level = iota
	LevelFormat
	LevelChecksum
	LevelRegistry
)

func (l Level) String() string {
	switch l {
	case LevelFormat:
		return "format"
	case LevelChecksum:
		return "checksum"
	case LevelRegistry:
		return "registry"
	}
	return "unspecified"
}

// Reason is the machine readable explanation of a step result. The registry is
// frozen for V1: engines agree on these codes, while human readable messages
// are not part of the contract.
type Reason uint8

// The frozen V1 reason registry.
//
// The codes group into four families, and which family a code belongs to is
// what a caller should branch on. ReasonOK accompanies Valid. The five codes
// that prove an invalidity accompany Invalid. The not_run codes say why a step
// was skipped. Everything else accompanies Unsupported and means "no verdict".
const (
	// ReasonUnspecified is the zero value and never appears in a result.
	ReasonUnspecified Reason = iota

	// ReasonOK accompanies every Valid step.
	ReasonOK

	// ReasonEmpty: the value held nothing where the rule requires something.
	ReasonEmpty

	// ReasonInvalidLength: the value has a length no documented variant uses.
	ReasonInvalidLength

	// ReasonInvalidCharacters: a character sits outside what the variant
	// allows at that position.
	ReasonInvalidCharacters

	// ReasonInvalidFormat: the shape is wrong in a way the more specific codes
	// do not describe, such as a missing prefix or separator.
	ReasonInvalidFormat

	// ReasonInvalidChecksum: the format held and the published check digit
	// did not. This is the strongest rejection the engine can produce.
	ReasonInvalidChecksum

	// ReasonMissingCountryCode: the value carries no country prefix and the
	// caller supplied no hint, so no definition could be selected. Supplying
	// Input.CountryCode usually resolves it.
	ReasonMissingCountryCode

	// ReasonCountryMismatch: the caller's country hint and the prefix the
	// value carries name different countries. The only dispatch failure that
	// proves an invalidity.
	ReasonCountryMismatch

	// ReasonUnsupportedKind: Input.Kind names no identifier family this
	// engine knows. See Engine.Kinds.
	ReasonUnsupportedKind

	// ReasonUnsupportedCountry: the country is malformed, or this engine
	// carries no rule for it. See Engine.Coverage.
	ReasonUnsupportedCountry

	// ReasonUnsupportedFormat: no format rule applies.
	ReasonUnsupportedFormat

	// ReasonUnsupportedChecksum: no checksum could be computed, either
	// because none is published or because the value did not give the
	// algorithm what it needs.
	ReasonUnsupportedChecksum

	// ReasonChecksumNotPublished: the authority publishes no check digit
	// algorithm for this variant, so none can be verified. The format result
	// is all this engine can offer.
	ReasonChecksumNotPublished

	// ReasonNotRequested: the checksum step was skipped because
	// Engine.ValidateFormat was called rather than Engine.Validate.
	ReasonNotRequested

	// ReasonNotRunFormatInvalid: the checksum was skipped because the format
	// was invalid. Verifying a check digit on a shape it was not designed for
	// would be meaningless.
	ReasonNotRunFormatInvalid

	// ReasonNotRunFormatUnsupported: the checksum was skipped because the
	// format could not be established.
	ReasonNotRunFormatUnsupported

	// ReasonRegistryNotConfigured is reserved: this engine never contacts a
	// register, and V1 defines no registry step.
	ReasonRegistryNotConfigured

	// ReasonIncompatibleRuleset and ReasonInvalidRuleset are reported by the
	// generator when it refuses a rule bundle. A built engine never produces
	// them, since its rules were accepted before the binary existed.
	ReasonIncompatibleRuleset
	ReasonInvalidRuleset

	// ReasonInputTooLong: the value exceeds 1024 UTF-8 bytes. A safety bound
	// raised before any rule runs; the value is reported verbatim, never
	// truncated.
	ReasonInputTooLong

	// ReasonInvalidEncoding: the value is not valid UTF-8. Like
	// ReasonInputTooLong this is a safety bound raised before any rule runs,
	// and the value is reported verbatim. An identifier is a sequence of code
	// points, and bytes that do not form one have none to evaluate.
	ReasonInvalidEncoding
)

func (r Reason) String() string {
	if int(r) >= len(reasonNames) {
		return "unspecified"
	}
	return reasonNames[r]
}

// reasonNames is a constant lookup table: it lives in read-only data and is not
// built at start-up.
var reasonNames = [...]string{
	"unspecified", "ok", "empty", "invalid_length", "invalid_characters",
	"invalid_format", "invalid_checksum", "missing_country_code",
	"country_mismatch", "unsupported_kind", "unsupported_country",
	"unsupported_format", "unsupported_checksum", "checksum_not_published",
	"not_requested", "not_run_format_invalid", "not_run_format_unsupported",
	"registry_not_configured", "incompatible_ruleset", "invalid_ruleset",
	"input_too_long", "invalid_encoding",
}

// Step is one line of a validation report.
type Step struct {
	// Level names the check this step reports on.
	Level Level

	// Status is the verdict of the step.
	Status Status

	// Reason explains the verdict in machine readable form.
	Reason Reason

	// MessageKey identifies a human readable message in the caller's own
	// catalogue. It is empty when the rule that produced the step declares
	// none, which is always the case for results produced before a rule ran.
	MessageKey string
}

// Report is the result of ValidateFormat, ValidateChecksum and Validate.
//
// Format acts as a guard: the checksum only runs on a value whose format is
// valid, so a checksum is never computed on a shape it was not designed for.
type Report struct {
	// Kind is the canonical kind once a dispatcher resolved, and otherwise the
	// requested token after trim and lower casing.
	Kind string

	// Input is the caller's value, unchanged.
	Input string

	// CanonicalValue is the value the rules canonicalized, as far as routing
	// got. It is the raw input when no dispatcher resolved.
	CanonicalValue string

	// CountryCode is the ISO 3166-1 alpha-2 code of the selected definition,
	// or the normalized country context when no definition was selected. It is
	// empty when no country applies.
	CountryCode string

	// Profile is the profile the validation ran under.
	Profile Profile

	// RulesVersion and FormatVersion identify the bundle that produced the
	// result.
	RulesVersion  string
	FormatVersion uint32

	Format   Step
	Checksum Step
}

// OK reports whether both steps concluded positively. A caller that treats
// Unsupported as a rejection would turn incomplete coverage into a false
// negative, which is why this is a deliberate, narrow helper rather than the
// only way to read a report.
func (r Report) OK() bool {
	return r.Format.Status == Valid && r.Checksum.Status == Valid
}

// Canonical is the result of Canonicalize. It reports the canonical form and
// how far routing got, without running any format or checksum rule.
type Canonical struct {
	// Kind is the canonical kind once a dispatcher resolved, and otherwise
	// the requested token after trim and lower casing.
	Kind string

	// Input is the caller's value, unchanged.
	Input string

	// CanonicalValue is the normalized value, as far as routing got. It is
	// the raw input when no dispatcher resolved, or when a safety bound
	// refused the value before any rule ran.
	CanonicalValue string

	// CountryCode is the ISO 3166-1 alpha-2 code of the selected definition,
	// or the normalized country context when none was selected. It is empty
	// when no country applies.
	CountryCode string

	// Profile is the profile the canonicalization ran under: the one the
	// caller passed, or the default the selected definition documents.
	Profile Profile

	// RulesVersion and FormatVersion identify the rules that produced the
	// result.
	RulesVersion  string
	FormatVersion uint32

	// Status is Valid when a definition was selected, Invalid only for a
	// country mismatch, and Unsupported when routing could not conclude.
	Status Status

	// Reason explains the status.
	Reason Reason

	// MessageKey is always empty: canonicalization stops before any rule
	// assertion runs.
	MessageKey string
}
