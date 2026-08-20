// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/libbusinessid/businessid-go/internal/gen"
)

func TestLoadShippedBundle(t *testing.T) {
	b, err := gen.Load(readSpecFile(t, "businessid-rules.binpb"))
	if err != nil {
		t.Fatalf("the shipped bundle must load: %v", err)
	}
	if b.FormatVersion != gen.SupportedFormatVersion {
		t.Fatalf("format version %d", b.FormatVersion)
	}
	// The version the bundle carries must be the one rules.lock attests, or
	// spec/ and rules.lock have drifted apart.
	if want := lockValue(t, "rules_version"); b.RulesVersion != want {
		t.Fatalf("bundle carries rules %q, rules.lock names %q", b.RulesVersion, want)
	}
	if len(b.SourceDigest) != 32 {
		t.Fatalf("source digest of %d bytes", len(b.SourceDigest))
	}
	// Every capability the bundle needs must be one this generator implements,
	// which Load already enforces; this states the expectation the other way
	// round, so an unused capability is visible rather than silent.
	supported := map[uint32]bool{}
	for _, id := range gen.SupportedFeatures {
		supported[id] = true
	}
	for _, id := range b.RequiredFeatureIDs {
		if !supported[id] {
			t.Errorf("the bundle requires capability %d, which this generator does not implement", id)
		}
	}
	if len(b.Identifiers) == 0 || len(b.Dispatchers) == 0 || len(b.Programs) == 0 {
		t.Fatalf("the bundle is empty: %d identifiers, %d dispatchers, %d programs",
			len(b.Identifiers), len(b.Dispatchers), len(b.Programs))
	}
	// Every definition must be reachable through a dispatcher, which check 22
	// enforces; counting here would only freeze a number that grows.
	t.Logf("rules %s: %d definitions, %d dispatchers, %d programs, %d nodes",
		b.RulesVersion, len(b.Identifiers), len(b.Dispatchers), len(b.Programs), countNodes(b))
}

func countNodes(b *gen.Bundle) int {
	n := 0
	for _, p := range b.Programs {
		n += len(p.Nodes)
	}
	return n
}

// lockValue reads one entry of rules.lock, so that the tests assert against
// what the repository attests rather than against a number typed here.
func lockValue(t *testing.T, key string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "rules.lock"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^` + key + `\s*=\s*"([^"]+)"`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatalf("rules.lock declares no %s", key)
	}
	return m[1]
}

// TestLoadRulesetCases runs the hostile bundle corpus. Section 8.8 addresses
// these cases to the generator: a truncated bundle or one carrying a call cycle
// must stop generation rather than reach the engine.
func TestLoadRulesetCases(t *testing.T) {
	fixtures := loadFixtures(t)
	if len(fixtures) == 0 {
		t.Fatal("the corpus carries no load_ruleset fixture")
	}
	t.Logf("%d hostile bundles", len(fixtures))
	for _, f := range fixtures {
		t.Run(f.ID, func(t *testing.T) {
			var want error
			switch f.Expected {
			case "invalid_ruleset":
				want = gen.ErrInvalidRuleset
			case "incompatible_ruleset":
				want = gen.ErrIncompatibleRuleset
			default:
				t.Fatalf("unexpected expected_engine_error %q", f.Expected)
			}
			b, err := gen.Load(f.Payload)
			if err == nil {
				t.Fatalf("the bundle loaded but %s was expected (%s)", f.Expected, f.Description)
			}
			if b != nil {
				t.Fatal("a refused bundle must not yield a usable model")
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %s (%s)", err, f.Expected, f.Description)
			}
			t.Logf("refused with: %v", err)
		})
	}
}

func TestLoadRefusesOversizedBundle(t *testing.T) {
	if _, err := gen.Load(make([]byte, gen.MaxBundleBytes+1)); !errors.Is(err, gen.ErrInvalidRuleset) {
		t.Fatalf("expected invalid_ruleset, got %v", err)
	}
}

func TestLoadRefusesEmptyPayloadAsIncompatible(t *testing.T) {
	// An empty payload decodes into a bundle whose format version is zero,
	// which no generator supports.
	if _, err := gen.Load(nil); !errors.Is(err, gen.ErrIncompatibleRuleset) {
		t.Fatalf("expected incompatible_ruleset, got %v", err)
	}
}

func TestLoadRefusesUnknownRootField(t *testing.T) {
	// Field 999, wire type 0, value 1 appended to the shipped bundle.
	forged := append(append([]byte(nil), readSpecFile(t, "businessid-rules.binpb")...), 0xb8, 0x3e, 0x01)
	if _, err := gen.Load(forged); !errors.Is(err, gen.ErrInvalidRuleset) {
		t.Fatalf("expected invalid_ruleset, got %v", err)
	}
}

func TestLoadRefusesReservedField(t *testing.T) {
	// Field 5 is reserved for the removed generated_at.
	forged := append(append([]byte(nil), readSpecFile(t, "businessid-rules.binpb")...), 0x28, 0x01)
	if _, err := gen.Load(forged); !errors.Is(err, gen.ErrInvalidRuleset) {
		t.Fatalf("expected invalid_ruleset for a reserved field, got %v", err)
	}
}

func TestLoadRefusesTruncatedBundle(t *testing.T) {
	raw := readSpecFile(t, "businessid-rules.binpb")
	if _, err := gen.Load(raw[:len(raw)/2]); !errors.Is(err, gen.ErrInvalidRuleset) {
		t.Fatalf("expected invalid_ruleset, got %v", err)
	}
}

// TestVersionChecksPrecedeUnknownFields pins the order section 10 gives its
// checks: format_version is check 3 and the capability ids check 4, while the
// unknown field scan is check 5. A bundle built against a later version carries
// both, and reporting the field first would call a legitimate version gap a
// forged bundle.
func TestVersionChecksPrecedeUnknownFields(t *testing.T) {
	// The shipped bundle, plus a field no V1 message declares, at three
	// depths: the root, a definition, and a source.
	raw := readSpecFile(t, "businessid-rules.binpb")

	t.Run("unknown capability wins over an unknown root field", func(t *testing.T) {
		forged := append(append([]byte(nil), raw...), 0xb8, 0x3e, 0x01) // field 999, varint 1
		// required_feature_ids is field 3, so a capability no generator
		// implements is one varint.
		forged = append(forged, 0x18, 0xfe, 0x01) // field 3, varint 254
		if _, err := gen.Load(forged); !errors.Is(err, gen.ErrIncompatibleRuleset) {
			t.Fatalf("got %v, want incompatible_ruleset", err)
		}
	})

	t.Run("unsupported format version wins over an unknown root field", func(t *testing.T) {
		// Field 1 already carries version 1, so a second occurrence would be
		// refused as a duplicate; the bundle is rebuilt from a bare root
		// instead.
		forged := []byte{0x08, 0x63} // field 1, varint 99
		forged = append(forged, 0xb8, 0x3e, 0x01)
		if _, err := gen.Load(forged); !errors.Is(err, gen.ErrIncompatibleRuleset) {
			t.Fatalf("got %v, want incompatible_ruleset", err)
		}
	})

	t.Run("an unknown field alone is still refused", func(t *testing.T) {
		forged := append(append([]byte(nil), raw...), 0xb8, 0x3e, 0x01)
		if _, err := gen.Load(forged); !errors.Is(err, gen.ErrInvalidRuleset) {
			t.Fatalf("got %v, want invalid_ruleset", err)
		}
	})
}
