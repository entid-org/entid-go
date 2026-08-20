// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// readSpec reads one artifact copied from the spec repository.
func readSpec(name string) ([]byte, error) {
	dir, err := filepath.Abs(filepath.Join("..", "..", "spec"))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return raw, nil
}

func readSpecFile(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := readSpec(name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// loadFixture is one hostile bundle carried by the conformance corpus, together
// with the engine error the corpus expects the generator to produce.
type loadFixture struct {
	ID          string
	Description string
	Expected    string
	Payload     []byte
}

// loadFixtures extracts the load_ruleset cases from the authoritative corpus.
// Their fixtures do not exist as files: the bytes live in the rules_payload
// field of each case, so the binary corpus is the only usable source.
//
// The reader below is deliberately minimal and lives in a test file: it never
// reads an expected business result, only the payload and the expected engine
// error, which section 8.8 addresses to the generator.
func loadFixtures(t *testing.T) []loadFixture {
	t.Helper()
	raw := readSpecFile(t, "businessid-conformance.binpb")

	var out []loadFixture
	walkFields(raw, func(field, wire int, payload []byte, _ uint64) {
		if field != 4 || wire != 2 { // ConformanceBundle.cases
			return
		}
		var f loadFixture
		operation := uint64(0)
		walkFields(payload, func(field, wire int, body []byte, v uint64) {
			switch {
			case field == 1 && wire == 2:
				f.ID = string(body)
			case field == 2 && wire == 2:
				f.Description = string(body)
			case field == 7 && wire == 0:
				operation = v
			case field == 11 && wire == 2:
				f.Payload = body
			case field == 12 && wire == 2:
				f.Expected = string(body)
			}
		})
		const operationLoadRuleset = 5
		if operation == operationLoadRuleset {
			out = append(out, f)
		}
	})
	return out
}

// walkFields walks a Protobuf message body, calling visit for every field it
// can parse. It stops at the first byte it cannot make sense of rather than
// failing: it reads artifacts, and a fuzz seed may well be malformed.
func walkFields(buf []byte, visit func(field, wire int, payload []byte, value uint64)) {
	varint := func(i int) (uint64, int, bool) {
		var v uint64
		for shift := uint(0); i < len(buf); shift += 7 {
			b := buf[i]
			i++
			v |= uint64(b&0x7f) << shift
			if b < 0x80 {
				return v, i, true
			}
		}
		return 0, 0, false
	}
	for i := 0; i < len(buf); {
		tag, next, ok := varint(i)
		if !ok {
			return
		}
		i = next
		field, wire := int(tag>>3), int(tag&7)
		switch wire {
		case 0:
			v, next, ok := varint(i)
			if !ok {
				return
			}
			i = next
			visit(field, wire, nil, v)
		case 2:
			n, next, ok := varint(i)
			if !ok || int(n) > len(buf)-next {
				return
			}
			i = next
			visit(field, wire, buf[i:i+int(n)], 0)
			i += int(n)
		case 5:
			i += 4
		case 1:
			i += 8
		default:
			return
		}
	}
}
