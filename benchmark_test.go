// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid_test

import (
	"strings"
	"testing"

	businessid "github.com/libbusinessid/businessid-go"
)

// The benchmarks below cover the whole public surface. They exist to pin two
// properties that motivated compiling the rules into Go instead of
// interpreting them: a validation of a clean value allocates nothing, and the
// cost does not depend on how many rules the engine carries, since dispatch is
// a switch rather than a lookup that grows.
//
// Read them with -benchmem. The allocation column is the one that matters; the
// nanoseconds move with the machine.

// samples are the inputs the benchmarks share, one per shape a caller meets.
var samples = []struct {
	name string
	in   businessid.Input
}{
	{
		// Already canonical, the shape a system that stores canonical values
		// hands back. Nothing is copied.
		name: "canonical/vat-be",
		in:   businessid.Input{Kind: "vat", Value: "BE0123456749"},
	},
	{
		name: "canonical/siren",
		in:   businessid.Input{Kind: "siren", Value: "012345674"},
	},
	{
		// Twenty alphanumeric characters through ISO 7064 MOD 97-10, the most
		// arithmetic any current rule performs.
		name: "canonical/lei",
		in:   businessid.Input{Kind: "lei", Value: "00000000000000000098"},
	},
	{
		name: "canonical/euid",
		in:   businessid.Input{Kind: "euid", Value: "FRTVX.012345674"},
	},
	{
		// The French VAT rule calls into the SIREN rule for both format and
		// checksum, so this walks two definitions.
		name: "canonical/vat-fr-nested",
		in:   businessid.Input{Kind: "vat", Value: "FR09012345674"},
	},
	{
		// A weighted sum over the issuer's own alphabet, eighteen positions
		// modulo 31. The value is the one case uscc-real-001 of the
		// conformance corpus carries.
		name: "canonical/uscc",
		in:   businessid.Input{Kind: "uscc", Value: "9144030071526726XG"},
	},
	{
		// Two chained weighted sums over the ASCII-48 alphabet, from case
		// cnpj-real-001 of the conformance corpus.
		name: "canonical/cnpj",
		in:   businessid.Input{Kind: "cnpj", Value: "00623904000173"},
	},
	{
		// Separators, case and a missing country prefix: canonicalization
		// rewrites the value, which costs the one allocation of the result.
		name: "dirty/vat-be",
		in:   businessid.Input{Kind: "vat", Value: "be 0123.456.749"},
	},
	{
		name: "dirty/siren",
		in:   businessid.Input{Kind: "siren", Value: " 012 345-674 "},
	},
	{
		// A country hint rather than a prefix.
		name: "hinted/vat-be",
		in:   businessid.Input{Kind: "vat", Value: "0123456749", CountryCode: "BE"},
	},
	{
		// Fails on the first assertion of the format rule.
		name: "reject/wrong-length",
		in:   businessid.Input{Kind: "siren", Value: "0123"},
	},
	{
		// Passes the format, fails the check digit: the whole pipeline runs.
		name: "reject/wrong-check-digit",
		in:   businessid.Input{Kind: "siren", Value: "012345675"},
	},
	{
		// Refused before any rule runs.
		name: "reject/unknown-kind",
		in:   businessid.Input{Kind: "nope", Value: "012345674"},
	},
	{
		name: "reject/invalid-encoding",
		in:   businessid.Input{Kind: "siren", Value: "0123456\xff"},
	},
	{
		name: "reject/input-too-long",
		in:   businessid.Input{Kind: "siren", Value: strings.Repeat("1", 2000)},
	},
}

func BenchmarkValidate(b *testing.B) {
	engine := businessid.New()
	for _, s := range samples {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := engine.Validate(s.in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkValidateFormat(b *testing.B) {
	engine := businessid.New()
	for _, s := range samples {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := engine.ValidateFormat(s.in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCanonicalize(b *testing.B) {
	engine := businessid.New()
	for _, s := range samples {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := engine.Canonicalize(s.in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkValidateChecksum exists to show it costs exactly what Validate
// costs: the two names are the same call, and the benchmark says so rather
// than leaving a reader to wonder.
func BenchmarkValidateChecksum(b *testing.B) {
	engine := businessid.New()
	in := businessid.Input{Kind: "vat", Value: "BE0123456749"}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := engine.ValidateChecksum(in); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProfiles measures the two frozen profiles on the one rule that
// distinguishes them, so that opting into strict_current can be costed.
func BenchmarkProfiles(b *testing.B) {
	engine := businessid.New()
	for _, profile := range []businessid.Profile{"", businessid.Compatible, businessid.StrictCurrent} {
		name := string(profile)
		if name == "" {
			name = "unset"
		}
		b.Run(name, func(b *testing.B) {
			in := businessid.Input{Kind: "vat", Value: "FR09012345674", Profile: profile}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := engine.Validate(in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkKindAliases measures the cost of the alias space: resolving "vat"
// and resolving "vat_number" must land on the same work.
//
// A kind that needs case folding costs one small allocation, because lower
// casing produces a string the engine did not receive. Callers that pass the
// canonical token — which is what the rules document — never pay it.
func BenchmarkKindAliases(b *testing.B) {
	engine := businessid.New()
	for _, kind := range []string{"vat", "vat_number", "  VAT  "} {
		b.Run(kind, func(b *testing.B) {
			in := businessid.Input{Kind: kind, Value: "BE0123456749"}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := engine.Validate(in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkValidateParallel checks that the engine scales across goroutines.
// It holds no mutable state, so nothing should serialize.
func BenchmarkValidateParallel(b *testing.B) {
	engine := businessid.New()
	in := businessid.Input{Kind: "vat", Value: "BE0123456749"}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := engine.Validate(in); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkNew measures constructing an engine. It should be free: the rules
// are Go code, so there is nothing to parse, load or build.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if businessid.New() == nil {
			b.Fatal("New returned nil")
		}
	}
}

// BenchmarkIntrospection measures the accessors. They copy so that a caller
// cannot mutate the engine's own tables, which is worth the allocation on a
// path nobody calls in a loop.
func BenchmarkIntrospection(b *testing.B) {
	engine := businessid.New()

	b.Run("Kinds", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = engine.Kinds()
		}
	})
	b.Run("Coverage", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = engine.Coverage()
		}
	})
	b.Run("RequiredCapabilities", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = engine.RequiredCapabilities()
		}
	})
	b.Run("RulesVersion", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = engine.RulesVersion()
		}
	})
}
