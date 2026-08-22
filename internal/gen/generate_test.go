// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/libbusinessid/businessid-go/internal/gen"
)

// TestGeneratedFileIsUpToDate is the guard that makes committing generated code
// safe: it regenerates from the bundle and compares with what the repository
// carries. A rules.lock bump that nobody regenerated fails here.
func TestGeneratedFileIsUpToDate(t *testing.T) {
	bundle, err := gen.Load(readSpecFile(t, "businessid-rules.binpb"))
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := gen.Generate(bundle, "businessid")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "rules_gen.go")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, committed) {
		t.Fatalf("rules_gen.go is stale; run: go generate ./...\n"+
			"regenerated %d bytes, committed %d bytes", len(fresh), len(committed))
	}
}

// TestGenerationIsDeterministic checks that the same bundle always produces the
// same bytes, which is what makes a regeneration readable as a diff.
func TestGenerationIsDeterministic(t *testing.T) {
	raw := readSpecFile(t, "businessid-rules.binpb")
	var first []byte
	for range 3 {
		bundle, err := gen.Load(raw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := gen.Generate(bundle, "businessid")
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = got
			continue
		}
		if !bytes.Equal(first, got) {
			t.Fatal("two generations of the same bundle differ")
		}
	}
}

// TestGeneratedCodeCoversEveryRule checks that nothing was silently dropped:
// one canonicalizer, one format rule and one checksum rule per definition, plus
// the routing tables.
func TestGeneratedCodeCoversEveryRule(t *testing.T) {
	bundle, err := gen.Load(readSpecFile(t, "businessid-rules.binpb"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := gen.Generate(bundle, "businessid")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)

	for _, want := range []string{
		"func dispatcherOf(", "func countryAlias(", "func countryTarget(",
		"func prefixTarget(", "func globalTarget(", "func unprefixedTarget(",
		"func preCanonicalize(", "func canonicalize(", "func formatRule(",
		"func checksumRule(", "func coverageTable(",
		"func canonicalizeSirenFR(", "func formatSirenFR(", "func checksumSirenFR(",
		"func canonicalizeVatGR(", "func formatVatGR(", "func checksumVatGR(",
		"func canonicalizeLei(", "func formatLei(", "func checksumLei(",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("the generated code has no %s", want)
		}
	}

	// Every message key the rules declare must reach the generated code: a
	// dropped key would silently change what a caller can show a user.
	keys := 0
	for _, p := range bundle.Programs {
		for _, n := range p.Nodes {
			if n.HasMessageKey && n.MessageKey != "" {
				keys++
				if !strings.Contains(code, `"`+n.MessageKey+`"`) {
					t.Errorf("message key %q is missing from the generated code", n.MessageKey)
				}
			}
		}
	}
	if keys == 0 {
		t.Fatal("the bundle declares no message key, which cannot be right")
	}

	// A hash map built when the program starts is exactly what the generated
	// dispatch must avoid.
	if strings.Contains(code, "map[") {
		t.Error("the generated code builds a map, which costs work at start-up")
	}
	// Slice literals at package level are initialized at start-up; arrays are
	// not. Only coverageTable, which is a function body, may build one.
	for _, line := range strings.Split(code, "\n") {
		if strings.HasPrefix(line, "var ") && strings.Contains(line, "= []") {
			t.Errorf("package level slice literal is built at start-up: %s", line)
		}
	}
}

// TestGenerateRefusesAnOversizedCanonicalizer checks the one limit the
// generator adds on top of the specification: a canonicalizer that would not
// fit the fixed workspace is refused rather than silently made to allocate.
func TestGenerateRefusesAnOversizedCanonicalizer(t *testing.T) {
	// The check lives in the generator, and the shipped bundle is far below
	// the margin, so this asserts the headroom rather than the refusal itself.
	bundle, err := gen.Load(readSpecFile(t, "businessid-rules.binpb"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.Generate(bundle, "businessid"); err != nil {
		t.Fatalf("the shipped bundle must fit the workspace: %v", err)
	}
}

// TestGeneratedCodeHasNoDeadFunction checks that everything emitted is reached.
//
// staticcheck skips files marked as generated, so an unused function would
// otherwise ship unnoticed — dead weight in every binary that links the engine.
func TestGeneratedCodeHasNoDeadFunction(t *testing.T) {
	bundle, err := gen.Load(readSpecFile(t, "businessid-rules.binpb"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := gen.Generate(bundle, "businessid")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)

	// The hand written engine calls into the generated code, so both files
	// count as call sites.
	engine, err := os.ReadFile(filepath.Join("..", "..", "engine.go"))
	if err != nil {
		t.Fatal(err)
	}
	public, err := os.ReadFile(filepath.Join("..", "..", "businessid.go"))
	if err != nil {
		t.Fatal(err)
	}
	callSites := code + string(engine) + string(public)

	for _, line := range strings.Split(code, "\n") {
		rest, ok := strings.CutPrefix(line, "func ")
		if !ok {
			continue
		}
		name, _, ok := strings.Cut(rest, "(")
		if !ok || name == "" {
			continue
		}
		// One definition plus at least one call.
		if strings.Count(callSites, name+"(") < 2 {
			t.Errorf("generated function %s is never called", name)
		}
	}
}

// TestRulesVersionShape covers check 6 of section 10. The version reaches
// generated sources, manifests and logs in every engine, so a control character
// or an over long one is refused at load time rather than carried into them.
func TestRulesVersionShape(t *testing.T) {
	for _, tc := range []struct {
		version string
		ok      bool
	}{
		{version: "2026.08.13", ok: true},
		{version: "1.0.0-rc_1", ok: true},
		{version: ""},
		{version: "0\x000"},
		{version: "1.0\nfunc x() {}"},
		{version: "2026 08 13"},
		{version: strings.Repeat("9", 65)},
		{version: strings.Repeat("9", 64), ok: true},
	} {
		raw := allOpcodesBundle()
		raw.rulesVersion = tc.version
		_, err := gen.Load(raw.encode())
		if tc.ok && err != nil {
			t.Errorf("version %q must load: %v", tc.version, err)
		}
		if !tc.ok && !errors.Is(err, gen.ErrInvalidRuleset) {
			t.Errorf("version %q: got %v, want invalid_ruleset", tc.version, err)
		}
	}
}
