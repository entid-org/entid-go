// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"errors"
	"testing"

	"github.com/entid-org/entid-go/internal/gen"
)

// FuzzLoad drives arbitrary bytes through the bundle loader. The bundle is
// untrusted input — a release artifact fetched over a network — and the loader
// is the only place its structure is ever checked, so no input may cause a
// panic, an unbounded allocation or a loop that does not end.
//
// Every refusal must carry one of the two typed errors: a bundle that is
// neither accepted nor typed as refused would leave a caller unable to tell a
// corrupt artifact from an incompatible one.
func FuzzLoad(f *testing.F) {
	f.Add(readSpecCorpus(f, "entid-rules.binpb"))
	for _, fixture := range loadFuzzFixtures(f) {
		f.Add(fixture)
	}
	// Shapes that exercise the decoder rather than the graph.
	f.Add([]byte{})
	f.Add([]byte{0x08, 0x01})                   // format_version = 1 alone
	f.Add([]byte{0x08, 0xff, 0xff, 0xff, 0xff}) // a truncated varint
	f.Add([]byte{0x3a, 0x02, 0x08, 0x01})       // a program with a stray body
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})       // an unknown field number

	f.Fuzz(func(t *testing.T, data []byte) {
		bundle, err := gen.Load(data)
		switch {
		case err == nil:
			if bundle == nil {
				t.Fatal("Load returned neither a bundle nor an error")
			}
			// A bundle the loader accepted must also be generatable: the two
			// steps agree on what a valid bundle is.
			if _, err := gen.Generate(bundle, "entid"); err != nil &&
				!errors.Is(err, gen.ErrInvalidRuleset) {
				t.Fatalf("an accepted bundle failed to generate: %v", err)
			}
		case errors.Is(err, gen.ErrInvalidRuleset), errors.Is(err, gen.ErrIncompatibleRuleset):
			if bundle != nil {
				t.Fatal("a refused bundle must not yield a model")
			}
		default:
			t.Fatalf("refusal carries neither typed error: %v", err)
		}
	})
}

func readSpecCorpus(f *testing.F, name string) []byte {
	f.Helper()
	raw, err := readSpec(name)
	if err != nil {
		f.Fatal(err)
	}
	return raw
}

// loadFuzzFixtures seeds the corpus with the hostile bundles the conformance
// suite carries, which are the shapes a real attacker would start from.
func loadFuzzFixtures(f *testing.F) [][]byte {
	f.Helper()
	raw, err := readSpec("entid-conformance.binpb")
	if err != nil {
		f.Fatal(err)
	}
	var out [][]byte
	walkFields(raw, func(field, wire int, payload []byte, _ uint64) {
		if field != 4 || wire != 2 {
			return
		}
		walkFields(payload, func(field, wire int, body []byte, _ uint64) {
			if field == 11 && wire == 2 {
				out = append(out, body)
			}
		})
	})
	return out
}
