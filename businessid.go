// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

// Package businessid canonicalizes and validates business identifiers offline:
// VAT numbers, EUID, the national company number of every EU member state,
// SIREN, SIRET, LEI, USCC, CNPJ, DUNS, EORI and EIN.
//
//	engine := businessid.New()
//
//	report, err := engine.Validate(businessid.Input{
//		Kind:  "vat",
//		Value: "BE 0123.456.749",
//	})
//	if err != nil {
//		return err
//	}
//	fmt.Println(report.CanonicalValue)   // BE0123456749
//	fmt.Println(report.Format.Status)    // valid
//	fmt.Println(report.Checksum.Status)  // valid
//
// The rules come from a signed bundle compiled by the LibBusinessID spec
// repository, and are compiled into this package as Go code when the engine is
// built. Nothing is interpreted at run time, nothing is downloaded, and no
// register is ever contacted.
//
// # What it answers, and what it does not
//
// Two questions, and no others: does this value have the shape of a documented
// variant, and does its internal check digit hold.
//
// A valid format means the shape matches a documented variant. A valid checksum
// means the published internal check is satisfied. Neither is evidence that a
// company exists, is active, or belongs to anyone — that would require a
// register, which this package never contacts.
//
// Nor does a valid result say whose identifier it is. A sole trader often
// invoices under a personal number — a Spanish DNI, a Bulgarian EGN, a Czech
// number built on a birth number — and those forms are accepted, because
// nothing in the format separates a sole trader from a private individual and
// refusing them would reject millions of real businesses. The caller knows that
// context; the format does not. "euid" is the exception, since a company
// register rather than a tax role issues it.
//
// # Reliability before coverage
//
// Refusing a valid identifier is the most serious defect this project
// recognizes. A value is reported [Invalid] only when a documented, applicable
// rule proves it. An unknown rule, an unpublished algorithm, an ambiguous
// variant or incomplete coverage all yield [Unsupported].
//
// Treat [Unsupported] as "no verdict", never as a rejection. A caller that
// rejects on Unsupported turns the engine's honesty about its own coverage into
// a false negative. German VAT numbers, for instance, close on an algorithm the
// V1 rule language cannot express: their checksum step is always Unsupported,
// and rejecting on it would reject every German VAT number in existence.
//
//	switch report.Checksum.Status {
//	case businessid.Valid:
//		// The check digit holds.
//	case businessid.Invalid:
//		// A documented rule proved this value wrong. Safe to reject.
//	default:
//		// No verdict. Fall back to the format result, or to a register.
//	}
//
// # Choosing an entry point
//
// [Engine.Validate] is the usual one: it checks the format and, when the format
// holds, the check digit. [Engine.ValidateChecksum] is the same call under a
// name that reads better at some call sites.
//
// [Engine.ValidateFormat] stops after the shape, which suits a form that
// validates as the user types. [Engine.Canonicalize] runs no rule at all and
// answers "what is this value, normalized" — what a caller stores, or uses as a
// database key.
//
// # Reading a report
//
// A [Report] carries two steps, format and checksum, each with a [Status] and a
// [Reason]. Format guards checksum: a check digit is never computed on a shape
// it was not designed for, so a format that did not hold leaves the checksum
// [NotRun].
//
// [Report.OK] is a deliberately narrow helper for "both steps concluded
// positively". Anything more nuanced should read the two steps.
//
// [Step.MessageKey] names a message in the caller's own catalogue, for the
// rules that declare one. It is empty otherwise, and always empty for results
// produced before a rule ran.
//
// # Cost
//
// An [Engine] holds no mutable state and is safe for concurrent use. Validating
// a value that is already canonical performs no allocation; one that needs
// rewriting costs a single allocation, for the canonical form itself. Nothing
// is built when the program starts.
//
// # Where the rules come from
//
// [Engine.Coverage] lists every definition this engine implements, with the
// authority, source URL, access date and tier behind each rule.
// [Engine.RulesVersion] identifies the bundle they were compiled from.
package businessid

import (
	"errors"
	"fmt"
)

// ErrUnknownProfile reports an Input asking for a profile outside the frozen
// set of section 2.6 of the specification.
var ErrUnknownProfile = errors.New("unknown validation profile")

// Input is one identifier submitted for canonicalization or validation.
type Input struct {
	// Kind names the identifier family, for example "vat", "siren", "lei" or
	// "euid". Matching trims ASCII whitespace, lower cases and resolves the
	// aliases the rules declare. It is required.
	Kind string

	// Value is the identifier as it was received. It is passed through
	// unchanged in the result. A value above 1024 UTF-8 bytes is reported
	// Unsupported with ReasonInputTooLong rather than truncated.
	//
	// Step 1 of section 6 of ir.md bounds the input in UTF-8 bytes and runs
	// before the step that refuses ill formed text, so an engine whose string
	// type admits such text must choose how to count it and say which choice
	// it made. This one counts the bytes the string already holds. A Go string
	// is an arbitrary byte sequence, not a sequence of code units awaiting an
	// encoder, so there is nothing to encode and nothing to invent: len(Value)
	// is the count, exactly, whether or not the bytes form valid UTF-8.
	//
	// The observable consequence is the order. A value both ill formed and
	// above the bound is ReasonInputTooLong, not ReasonInvalidEncoding, since
	// the bound is measured first. Below the bound, ill formed text is
	// ReasonInvalidEncoding. Both are Unsupported, and no conformance case can
	// carry either, because a proto3 string is valid UTF-8 by definition.
	Value string

	// CountryCode is an optional ISO 3166-1 alpha-2 hint. It disambiguates a
	// value carrying no country prefix, and contradicts one that carries a
	// different country. An empty string means no hint.
	CountryCode string

	// Profile selects the validation profile. The zero value means the caller
	// expresses no preference, which lets the definition the value routes to
	// apply its own documented default. Passing a profile always overrides it.
	//
	// Every definition of the current rules documents Compatible, so the two
	// are indistinguishable today.
	Profile Profile
}

// check refuses a profile outside the frozen set. An empty profile is not a
// value but the absence of one: section 5.2 of ir.md then lets the selected
// definition supply its own default.
func (in Input) check() error {
	switch in.Profile {
	case "", Compatible, StrictCurrent:
		return nil
	}
	return fmt.Errorf("%w: %q", ErrUnknownProfile, string(in.Profile))
}

// Engine answers queries against the rules compiled into this package.
//
// It holds no state: the zero value works, and New exists so that future
// options have somewhere to go.
type Engine struct{}

// New returns an Engine.
//
// It cannot fail. The rules are Go code in this package, checked by the
// compiler rather than parsed at run time; a bundle that did not satisfy the IR
// contract would have stopped the generator long before this binary existed.
func New() *Engine { return &Engine{} }

// Canonicalize normalizes a value and reports which definition it routes to,
// without running any format or checksum rule.
func (e *Engine) Canonicalize(in Input) (Canonical, error) {
	if err := in.check(); err != nil {
		return Canonical{}, err
	}
	r, _, _ := evaluate(in, modeCanonicalize)
	return Canonical{
		Kind:           r.kind,
		Input:          in.Value,
		CanonicalValue: r.canonical,
		CountryCode:    r.country,
		Profile:        r.profile,
		RulesVersion:   rulesVersion,
		FormatVersion:  formatVersion,
		Status:         r.status,
		Reason:         r.reason,
	}, nil
}

// ValidateFormat checks only that the value has the shape of a documented
// variant. The returned report always carries a checksum step; after a valid
// format it is NotRun with ReasonNotRequested.
func (e *Engine) ValidateFormat(in Input) (Report, error) {
	return e.run(in, modeValidateFormat)
}

// ValidateChecksum checks the format and then the documented internal check
// digit. It returns exactly the report Validate returns; the two names exist
// because callers reason about them differently.
func (e *Engine) ValidateChecksum(in Input) (Report, error) {
	return e.run(in, modeValidate)
}

// Validate checks the format and, when the format holds, the documented
// internal check digit.
//
// A valid format with no published checksum algorithm yields a checksum step of
// Unsupported, never Invalid.
func (e *Engine) Validate(in Input) (Report, error) {
	return e.run(in, modeValidate)
}

func (e *Engine) run(in Input, md mode) (Report, error) {
	if err := in.check(); err != nil {
		return Report{}, err
	}
	r, format, checksum := evaluate(in, md)
	return Report{
		Kind:           r.kind,
		Input:          in.Value,
		CanonicalValue: r.canonical,
		CountryCode:    r.country,
		Profile:        r.profile,
		RulesVersion:   rulesVersion,
		FormatVersion:  formatVersion,
		Format: Step{
			Level: LevelFormat, Status: format.status,
			Reason: format.reason, MessageKey: format.key,
		},
		Checksum: Step{
			Level: LevelChecksum, Status: checksum.status,
			Reason: checksum.reason, MessageKey: checksum.key,
		},
	}, nil
}

// RulesVersion is the business version of the rules compiled into this engine,
// formatted as YYYY.MM.PATCH.
func (e *Engine) RulesVersion() string { return rulesVersion }

// FormatVersion is the structural version of the IR the rules were compiled
// from.
func (e *Engine) FormatVersion() uint32 { return formatVersion }

// RequiredCapabilities lists the capability IDs the compiled rules need.
func (e *Engine) RequiredCapabilities() []uint32 {
	out := make([]uint32, len(requiredCapabilities))
	copy(out, requiredCapabilities[:])
	return out
}

// SupportedCapabilities lists the capability IDs this engine implements. The
// generator refuses a bundle requiring a single ID outside this list, so the
// two lists agree by construction.
func SupportedCapabilities() []uint32 {
	out := make([]uint32, len(supportedCapabilities))
	copy(out, supportedCapabilities[:])
	return out
}

// Kinds lists the canonical identifier kinds this engine can route.
func (e *Engine) Kinds() []string {
	out := make([]string, 0, len(kinds))
	out = append(out, kinds[:]...)
	return out
}

// Coverage describes one identifier definition the engine implements.
type Coverage struct {
	// Kind is the canonical identifier family.
	Kind string

	// CountryCode is the ISO 3166-1 alpha-2 code, empty for a global
	// definition such as the LEI.
	CountryCode string

	// HasChecksum reports whether a checksum algorithm is published for this
	// definition. When it is false, Validate reports the checksum step as
	// Unsupported with AbsentChecksumReason.
	HasChecksum bool

	// AbsentChecksumReason explains why no checksum runs. It is
	// ReasonUnspecified when HasChecksum is true.
	AbsentChecksumReason Reason

	// DefaultProfile is the profile the definition documents as its default.
	DefaultProfile Profile

	// Sources are the provenance records backing the rule.
	Sources []Source
}

// Source is a provenance record: where a rule comes from and when it was
// checked.
type Source struct {
	ID             string
	URL            string
	Authority      string
	Title          string
	AccessedAt     string
	Jurisdiction   string
	Language       string
	Notes          string
	LicenseOrTerms string
	ArchiveURL     string

	// Tier says how close the source sits to the authority that defines the
	// format. A rule documented only by a third party is not less true, but a
	// reader deciding how far to trust a check digit wants to know.
	Tier SourceTier
}

// SourceTier ranks a provenance record by its distance from the authority.
type SourceTier uint8

// The source tiers.
const (
	// TierUnspecified is the zero value.
	TierUnspecified SourceTier = iota

	// TierPrimary is published by the authority that owns the format, or by
	// the law establishing it.
	TierPrimary

	// TierSecondary is a third party description — a reference
	// implementation, an industry note, an encyclopedia. It is used when the
	// authority documents the format but not its check algorithm.
	TierSecondary
)

func (t SourceTier) String() string {
	switch t {
	case TierPrimary:
		return "primary"
	case TierSecondary:
		return "secondary"
	}
	return "unspecified"
}

// Coverage lists every identifier definition this engine implements, with its
// provenance. It answers "what does this engine actually know" without
// requiring the caller to probe it with sample values.
func (e *Engine) Coverage() []Coverage { return coverageTable() }
