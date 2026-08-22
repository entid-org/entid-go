// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

// Structural limits of section 8 of ir.md. Every limit is normative: an engine
// may raise an internal limit, never lower it.
const (
	MaxBundleBytes       = 16777216
	MaxIdentifiers       = 10000
	MaxTotalNodes        = 500000
	MaxNodesPerProgram   = 4096
	MaxCallDepth         = 32
	MaxConstantBytes     = 4096
	MaxInputBytes        = 1024
	MaxCapturesPerFormat = 128
	MaxSteps             = 100000
	MaxRulesVersionBytes = 64
	MaxAlphabetPoints    = 256
)

// Arithmetic limits of section 8 of ir.md.
const (
	MinModulus         = 2
	MaxModulus         = 1000000000
	MaxWeightMagnitude = 1000000
	MinWeights         = 1
	MaxWeights         = 256
	MinRemainderValues = 1
	MaxRemainderValues = 1000000
	MaxSliceBound      = 4096
	MinProvableDigits  = 1
	MaxProvableDigits  = 18
	MaxConstant        = 1000000000
	MinConstant        = -MaxConstant
)

// SupportedFormatVersion is the only IR structural version this generator
// understands. Any other value closes generation with incompatible_ruleset.
const SupportedFormatVersion = 1

// Capability IDs of features.md. The set is frozen: the generator refuses a
// bundle declaring a single id outside it.
const (
	FeatureCoreGraph                 uint32 = 1
	FeatureASCIIAndWhitespace        uint32 = 2
	FeatureCanonicalizationBasic     uint32 = 3
	FeatureCanonicalizationCondition uint32 = 4
	FeatureIdentifierDispatch        uint32 = 5
	FeatureStringViews               uint32 = 10
	FeatureCapturesAndCalls          uint32 = 11
	FeatureFormatAssertions          uint32 = 20
	FeatureProfiles                  uint32 = 21
	FeatureChecksumTristate          uint32 = 30
	FeatureChecksumLuhn              uint32 = 31
	FeatureChecksumMod97             uint32 = 32
	FeatureChecksumWeighted          uint32 = 33
	FeatureChecksumCompareConstant   uint32 = 34
	FeatureChecksumIntegerPredicate  uint32 = 35
	FeatureProvenance                uint32 = 40
	FeatureProvenanceTier            uint32 = 41
	FeatureChecksumCustomAlphabet    uint32 = 42
)

// SupportedFeatures is the exact list of capability IDs this generator
// implements. It is published so that a bundle can be compared with it.
var SupportedFeatures = []uint32{
	FeatureCoreGraph,
	FeatureASCIIAndWhitespace,
	FeatureCanonicalizationBasic,
	FeatureCanonicalizationCondition,
	FeatureIdentifierDispatch,
	FeatureStringViews,
	FeatureCapturesAndCalls,
	FeatureFormatAssertions,
	FeatureProfiles,
	FeatureChecksumTristate,
	FeatureChecksumLuhn,
	FeatureChecksumMod97,
	FeatureChecksumWeighted,
	FeatureChecksumCompareConstant,
	FeatureChecksumIntegerPredicate,
	FeatureProvenance,
	FeatureProvenanceTier,
	FeatureChecksumCustomAlphabet,
}

var supportedFeatureSet = func() map[uint32]bool {
	m := make(map[uint32]bool, len(SupportedFeatures))
	for _, id := range SupportedFeatures {
		m[id] = true
	}
	return m
}()

// Profile names of features.md capability 21.
const (
	ProfileCompatible    = "compatible"
	ProfileStrictCurrent = "strict_current"
)

func validProfile(name string) bool {
	return name == ProfileCompatible || name == ProfileStrictCurrent
}
