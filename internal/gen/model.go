// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

// Package gen reads a LibBusinessID rule bundle, validates it against the IR
// contract, and emits Go source implementing its rules.
//
// It runs when the engine is built, never when the engine validates an
// identifier. Section 2.3 of the specification makes refusal a property of
// generation time: an unknown version, field, opcode or capability stops the
// generator instead of producing partial code.
package gen

// ValueType is the static type of a node output.
type ValueType int32

// Value types of rules.proto. Zero is UNSPECIFIED and always refused.
const (
	ValueUnspecified ValueType = iota
	ValueString
	ValueInteger
	ValueBoolean
	ValueCanonicalizationStep
	ValueAssertion
	ValueChecksumOutcome
)

func (v ValueType) String() string {
	switch v {
	case ValueString:
		return "string"
	case ValueInteger:
		return "integer"
	case ValueBoolean:
		return "boolean"
	case ValueCanonicalizationStep:
		return "canonicalization step"
	case ValueAssertion:
		return "assertion"
	case ValueChecksumOutcome:
		return "checksum outcome"
	}
	return "unspecified"
}

// ProgramKind constrains which operations a program may contain.
type ProgramKind int32

// Program kinds of rules.proto.
const (
	ProgramUnspecified ProgramKind = iota
	ProgramCanonicalization
	ProgramFormat
	ProgramChecksum
)

func (k ProgramKind) String() string {
	switch k {
	case ProgramCanonicalization:
		return "canonicalization"
	case ProgramFormat:
		return "format"
	case ProgramChecksum:
		return "checksum"
	}
	return "unspecified"
}

// ReasonCode is the immutable V1 registry of machine readable reasons. The
// numeric values mirror rules.proto and are part of the wire contract.
type ReasonCode int32

// Reason codes of rules.proto.
const (
	ReasonUnspecified ReasonCode = iota
	ReasonOK
	ReasonEmpty
	ReasonInvalidLength
	ReasonInvalidCharacters
	ReasonInvalidFormat
	ReasonInvalidChecksum
	ReasonMissingCountryCode
	ReasonCountryMismatch
	ReasonUnsupportedKind
	ReasonUnsupportedCountry
	ReasonUnsupportedFormat
	ReasonUnsupportedChecksum
	ReasonChecksumNotPublished
	ReasonNotRequested
	ReasonNotRunFormatInvalid
	ReasonNotRunFormatUnsupported
	ReasonRegistryNotConfigured
	ReasonIncompatibleRuleset
	ReasonInvalidRuleset
	ReasonInputTooLong
	ReasonInvalidEncoding
)

var reasonNames = [...]string{
	ReasonUnspecified:             "unspecified",
	ReasonOK:                      "ok",
	ReasonEmpty:                   "empty",
	ReasonInvalidLength:           "invalid_length",
	ReasonInvalidCharacters:       "invalid_characters",
	ReasonInvalidFormat:           "invalid_format",
	ReasonInvalidChecksum:         "invalid_checksum",
	ReasonMissingCountryCode:      "missing_country_code",
	ReasonCountryMismatch:         "country_mismatch",
	ReasonUnsupportedKind:         "unsupported_kind",
	ReasonUnsupportedCountry:      "unsupported_country",
	ReasonUnsupportedFormat:       "unsupported_format",
	ReasonUnsupportedChecksum:     "unsupported_checksum",
	ReasonChecksumNotPublished:    "checksum_not_published",
	ReasonNotRequested:            "not_requested",
	ReasonNotRunFormatInvalid:     "not_run_format_invalid",
	ReasonNotRunFormatUnsupported: "not_run_format_unsupported",
	ReasonRegistryNotConfigured:   "registry_not_configured",
	ReasonIncompatibleRuleset:     "incompatible_ruleset",
	ReasonInvalidRuleset:          "invalid_ruleset",
	ReasonInputTooLong:            "input_too_long",
	ReasonInvalidEncoding:         "invalid_encoding",
}

func (c ReasonCode) String() string {
	if c < 0 || int(c) >= len(reasonNames) {
		return "unspecified"
	}
	return reasonNames[c]
}

// GoName is the identifier of this reason code in the generated engine.
func (c ReasonCode) GoName() string {
	switch c {
	case ReasonOK:
		return "ReasonOK"
	case ReasonEmpty:
		return "ReasonEmpty"
	case ReasonInvalidLength:
		return "ReasonInvalidLength"
	case ReasonInvalidCharacters:
		return "ReasonInvalidCharacters"
	case ReasonInvalidFormat:
		return "ReasonInvalidFormat"
	case ReasonInvalidChecksum:
		return "ReasonInvalidChecksum"
	case ReasonMissingCountryCode:
		return "ReasonMissingCountryCode"
	case ReasonCountryMismatch:
		return "ReasonCountryMismatch"
	case ReasonUnsupportedKind:
		return "ReasonUnsupportedKind"
	case ReasonUnsupportedCountry:
		return "ReasonUnsupportedCountry"
	case ReasonUnsupportedFormat:
		return "ReasonUnsupportedFormat"
	case ReasonUnsupportedChecksum:
		return "ReasonUnsupportedChecksum"
	case ReasonChecksumNotPublished:
		return "ReasonChecksumNotPublished"
	case ReasonNotRequested:
		return "ReasonNotRequested"
	case ReasonNotRunFormatInvalid:
		return "ReasonNotRunFormatInvalid"
	case ReasonNotRunFormatUnsupported:
		return "ReasonNotRunFormatUnsupported"
	case ReasonRegistryNotConfigured:
		return "ReasonRegistryNotConfigured"
	case ReasonIncompatibleRuleset:
		return "ReasonIncompatibleRuleset"
	case ReasonInvalidRuleset:
		return "ReasonInvalidRuleset"
	case ReasonInputTooLong:
		return "ReasonInputTooLong"
	case ReasonInvalidEncoding:
		return "ReasonInvalidEncoding"
	}
	return "ReasonUnspecified"
}

// provesInvalidity reports whether REQUIRE may carry this reason code. Section
// 4 of ir.md restricts it to codes that prove an invalidity.
func (c ReasonCode) provesInvalidity() bool {
	switch c {
	case ReasonEmpty, ReasonInvalidLength, ReasonInvalidCharacters, ReasonInvalidFormat, ReasonCountryMismatch:
		return true
	}
	return false
}

// absentChecksumReason reports whether the code may explain a missing or
// unsupported checksum.
func (c ReasonCode) absentChecksumReason() bool {
	return c == ReasonUnsupportedChecksum || c == ReasonChecksumNotPublished
}

// WeightAlignment describes how weights are paired with input code points.
type WeightAlignment int32

// Weight alignments of rules.proto.
const (
	AlignUnspecified WeightAlignment = iota
	AlignLeft
	AlignRight
	AlignCycle
)

// CharMapping maps a code point to its numeric contribution.
type CharMapping int32

// Character mappings of rules.proto.
const (
	MappingUnspecified CharMapping = iota
	MappingDigitValue
	MappingAlnumBase36
	MappingCustomAlphabet
)

// Bundle is a decoded and fully validated rule bundle.
type Bundle struct {
	FormatVersion      uint32
	RulesVersion       string
	RequiredFeatureIDs []uint32
	SourceDigest       []byte
	Identifiers        []*IdentifierDefinition
	Programs           []*Program
	Dispatchers        []*IdentifierDispatcher

	programByID    map[uint32]*Program
	identifierByID map[uint32]*IdentifierDefinition
}

// Program returns the program with the given id, or nil.
func (b *Bundle) Program(id uint32) *Program { return b.programByID[id] }

// Identifier returns the definition with the given id, or nil.
func (b *Bundle) Identifier(id uint32) *IdentifierDefinition { return b.identifierByID[id] }

// IdentifierDefinition binds a canonicalizer, a format rule and an optional
// checksum rule to a (kind, country) pair.
type IdentifierDefinition struct {
	ID                      uint32
	Kind                    string
	CountryCode             string // empty means GLOBAL
	HasCountryCode          bool
	CanonicalizationProgram uint32
	FormatProgram           uint32
	ChecksumProgram         uint32
	HasChecksumProgram      bool
	DefaultProfile          string
	Sources                 []Source
	AbsentChecksumReason    ReasonCode
	HasAbsentChecksumReason bool
}

// SourceTier says how close a source sits to the authority that defines the
// format. A rule documented only by a third party is not less true, but a
// reader deciding how much to trust a check digit wants to know.
type SourceTier int32

// Source tiers of rules.proto.
const (
	TierUnspecified SourceTier = iota
	// TierPrimary is published by the authority that owns the format, or by
	// the law establishing it.
	TierPrimary
	// TierSecondary is a third party description, used when the authority
	// documents the format but not its check algorithm.
	TierSecondary
)

// Source is the provenance record of a rule.
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
	HasArchiveURL  bool
	Tier           SourceTier
}

// IdentifierDispatcher routes a (kind, optional country, raw value) triple to
// exactly one identifier definition.
type IdentifierDispatcher struct {
	Kind                       string
	KindAliases                []string
	PreCanonicalizationProgram uint32
	CountryAliases             []CountryAlias
	Targets                    []*DispatchTarget

	// Global is the single GLOBAL target, when the dispatcher declares one.
	Global *DispatchTarget
	// Unprefixed is the single target selectable without country or prefix.
	Unprefixed *DispatchTarget
}

// CountryAlias maps a non ISO 3166-1 alpha-2 token to a canonical country code.
type CountryAlias struct {
	Alias       string
	CountryCode string
}

// DispatchTarget is one routing entry of a dispatcher.
type DispatchTarget struct {
	CountryCode                   string // empty means GLOBAL
	HasCountryCode                bool
	AcceptedPrefixes              []string
	CanonicalPrefix               string
	HasCanonicalPrefix            bool
	IdentifierDefinitionID        uint32
	AllowUnprefixedWithoutCountry bool

	Definition *IdentifierDefinition
}

// Program is a typed acyclic node graph in topological order.
type Program struct {
	ID          uint32
	Kind        ProgramKind
	Nodes       []*Node
	RootNode    uint32
	Captures    []Capture
	SubjectNode uint32
	HasSubject  bool
}

// Capture names a node of a format program.
type Capture struct {
	Name string
	Node uint32
}

// Node is one operation of a program.
type Node struct {
	OutputType ValueType
	InputNodes []uint32

	// Op identifies the operation across all seven categories. Exactly one of
	// the operation messages is present on the wire; the decoder folds it into
	// Op plus the parameter fields below.
	Op Opcode

	// Parameters. Presence flags distinguish an absent optional field from its
	// zero value, which the loader needs to refuse a stray parameter.
	Text            string
	HasText         bool
	Replacement     string
	HasReplacement  bool
	Start           uint32
	HasStart        bool
	End             uint32
	HasEnd          bool
	Index           uint32
	HasIndex        bool
	Length          uint32
	HasLength       bool
	MinLength       uint32
	HasMinLength    bool
	MaxLength       uint32
	HasMaxLength    bool
	Values          []string
	Lengths         []uint32
	Modulus         int64
	HasModulus      bool
	Weights         []int64
	Alignment       WeightAlignment
	HasAlignment    bool
	Mapping         CharMapping
	HasMapping      bool
	Alphabet        string
	HasAlphabet     bool
	RemainderValues []int64
	Constant        int64
	HasConstant     bool
	ReasonCode      ReasonCode
	HasReasonCode   bool
	MessageKey      string
	HasMessageKey   bool
	ProgramID       uint32
}
