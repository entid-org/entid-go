// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/libbusinessid/businessid-go/internal/gen"
)

// A bundle is untrusted input, and the branches that refuse a damaged one are
// the branches that run when someone hands the generator bytes it should
// reject. They were the least covered code in this repository, because the
// corpus carries thirty six hostile bundles and each proves exactly one rule.
//
// What follows damages a real encoding in five systematic ways and requires the
// loader to survive every one of them. It is not a fuzzer: the mutations are
// derived from the encoding itself and are the same on every run, so a failure
// names a reproducible input.

// wireSite is one field occurrence in an encoding, located precisely enough to
// be damaged.
type wireSite struct {
	tag      int // offset of the tag varint
	body     int // offset of the payload, past the tag and any length
	end      int // offset just past the whole field
	lenOff   int // offset of the length varint, -1 unless the wire type is 2
	lenWidth int
	field    int
	wire     int
	depth    int
}

// readVarintAt decodes a varint and reports its width.
func readVarintAt(buf []byte, i int) (value uint64, width int, ok bool) {
	start := i
	for shift := uint(0); i < len(buf) && i-start < 10; shift += 7 {
		b := buf[i]
		i++
		value |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return value, i - start, true
		}
	}
	return 0, 0, false
}

// walkSites lists every field occurrence of an encoding, descending into any
// length delimited payload that parses completely as a message.
//
// The descent is deliberately naive: a string whose bytes happen to parse is
// walked as though it were a message. That costs a few mutations that damage a
// constant rather than a structure, which the contract below tolerates, and it
// buys a walker that owes nothing to a schema this test would otherwise have to
// restate and keep in step.
func walkSites(buf []byte, depth int, out *[]wireSite) bool {
	const maxDepth = 8
	for i := 0; i < len(buf); {
		tagOff := i
		tag, width, ok := readVarintAt(buf, i)
		if !ok {
			return false
		}
		i += width
		field, wire := int(tag>>3), int(tag&7)
		if field == 0 {
			return false
		}
		site := wireSite{tag: tagOff, field: field, wire: wire, depth: depth, lenOff: -1}
		switch wire {
		case 0:
			_, w, ok := readVarintAt(buf, i)
			if !ok {
				return false
			}
			site.body, i = i, i+w
		case 1:
			if i+8 > len(buf) {
				return false
			}
			site.body, i = i, i+8
		case 5:
			if i+4 > len(buf) {
				return false
			}
			site.body, i = i, i+4
		case 2:
			site.lenOff = i
			n, w, ok := readVarintAt(buf, i)
			if !ok {
				return false
			}
			site.lenWidth = w
			i += w
			if n > uint64(len(buf)-i) {
				return false
			}
			site.body = i
			payload := buf[i : i+int(n)]
			i += int(n)
			site.end = i
			*out = append(*out, site)
			if depth < maxDepth && len(payload) > 0 {
				var nested []wireSite
				if walkSites(payload, depth+1, &nested) {
					for _, s := range nested {
						s.tag += site.body
						s.body += site.body
						s.end += site.body
						if s.lenOff >= 0 {
							s.lenOff += site.body
						}
						*out = append(*out, s)
					}
				}
			}
			continue
		default:
			return false
		}
		site.end = i
		*out = append(*out, site)
	}
	return true
}

// Splicing bytes into a nested payload invalidates every enclosing length, so
// the mutations that reach inside one go through a re-encoder instead. A path
// names a nested message by its position among the length delimited fields of
// each level, and editNested rebuilds the encoding around whatever the mutation
// returns, recomputing each length on the way out.

func nestedPaths(buf []byte, prefix []int, depth int, out *[][]int) {
	const maxPathDepth = 5
	if depth > maxPathDepth {
		return
	}
	index := 0
	for i := 0; i < len(buf); {
		tag, width, ok := readVarintAt(buf, i)
		if !ok {
			return
		}
		i += width
		field, wire := int(tag>>3), int(tag&7)
		if field == 0 {
			return
		}
		switch wire {
		case 0:
			_, w, ok := readVarintAt(buf, i)
			if !ok {
				return
			}
			i += w
		case 1:
			i += 8
		case 5:
			i += 4
		case 2:
			n, w, ok := readVarintAt(buf, i)
			if !ok || n > uint64(len(buf)-i-w) {
				return
			}
			i += w
			payload := buf[i : i+int(n)]
			i += int(n)
			path := append(append([]int(nil), prefix...), index)
			index++
			// Only a payload that parses whole is worth descending into; a
			// string that happens to parse costs one useless mutation, which
			// the contract tolerates.
			var probe []wireSite
			if len(payload) > 0 && walkSites(payload, 0, &probe) {
				*out = append(*out, path)
				nestedPaths(payload, path, depth+1, out)
			}
		default:
			return
		}
	}
}

// editNested rebuilds buf with mutate applied to the payload the path names.
func editNested(buf []byte, path []int, mutate func([]byte) []byte) []byte {
	var out []byte
	index := 0
	for i := 0; i < len(buf); {
		start := i
		tag, width, ok := readVarintAt(buf, i)
		if !ok {
			return append(out, buf[start:]...)
		}
		i += width
		wire := int(tag & 7)
		switch wire {
		case 0:
			_, w, _ := readVarintAt(buf, i)
			i += w
			out = append(out, buf[start:i]...)
		case 1:
			i += 8
			out = append(out, buf[start:i]...)
		case 5:
			i += 4
			out = append(out, buf[start:i]...)
		case 2:
			n, w, _ := readVarintAt(buf, i)
			i += w
			payload := buf[i : i+int(n)]
			i += int(n)
			if index != path[0] {
				index++
				out = append(out, buf[start:i]...)
				continue
			}
			index++
			var replacement []byte
			if len(path) == 1 {
				replacement = mutate(payload)
			} else {
				replacement = editNested(payload, path[1:], mutate)
			}
			out = append(out, buf[start:start+width]...)
			out = appendVarint(out, uint64(len(replacement)))
			out = append(out, replacement...)
		default:
			return append(out, buf[start:]...)
		}
	}
	return out
}

func appendVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

// stride keeps a family of mutations bounded without making the selection
// depend on anything but the encoding, so two runs damage the same bytes.
func stride(n, want int) int {
	if n <= want {
		return 1
	}
	return n / want
}

// mutantsOf builds every damaged encoding this suite knows how to make.
func mutantsOf(t *testing.T, label string, raw []byte, perFamily, perNested int) []struct {
	name string
	body []byte
	// fatal marks a mutation whose damage cannot leave a loadable bundle, so
	// the loader must refuse it rather than merely survive it.
	fatal bool
} {
	t.Helper()
	var sites []wireSite
	if !walkSites(raw, 0, &sites) {
		t.Fatalf("%s: the encoding does not parse, so no mutation is meaningful", label)
	}
	if len(sites) == 0 {
		t.Fatalf("%s: the walk found no field", label)
	}

	type mutant struct {
		name  string
		body  []byte
		fatal bool
	}
	var out []mutant
	splice := func(parts ...[]byte) []byte {
		n := 0
		for _, p := range parts {
			n += len(p)
		}
		b := make([]byte, 0, n)
		for _, p := range parts {
			b = append(b, p...)
		}
		return b
	}

	// 1. Truncation, one byte into a field and again just before its end. Both
	// cut a field in half, which no complete message can survive.
	step := stride(len(sites), perFamily/2)
	for k := 0; k < len(sites); k += step {
		s := sites[k]
		if s.depth != 0 {
			continue
		}
		if s.body > s.tag+1 {
			out = append(out, mutant{
				fmt.Sprintf("%s/truncate inside the header of field %d", label, s.field),
				splice(raw[:s.tag+1]), true,
			})
		}
		if s.end > s.body+1 {
			out = append(out, mutant{
				fmt.Sprintf("%s/truncate one byte before the end of field %d", label, s.field),
				splice(raw[:s.end-1]), true,
			})
		}
	}

	// 2. Wire type flips. The tag varint keeps its width, since the width
	// follows the field number and only the low three bits move.
	step = stride(len(sites), perFamily/3)
	for k := 0; k < len(sites); k += step {
		s := sites[k]
		for _, wire := range []int{0, 1, 2, 5} {
			if wire == s.wire {
				continue
			}
			body := splice(raw)
			tag := uint64(s.field)<<3 | uint64(wire)
			w := 0
			for v := tag; ; v >>= 7 {
				if v < 0x80 {
					body[s.tag+w] = byte(v)
					w++
					break
				}
				body[s.tag+w] = byte(v) | 0x80
				w++
			}
			out = append(out, mutant{
				fmt.Sprintf("%s/field %d at depth %d carries wire type %d instead of %d",
					label, s.field, s.depth, wire, s.wire),
				body, false,
			})
		}
	}

	// 3. Length overrun: a length delimited field claiming the largest value
	// its varint width can hold.
	step = stride(len(sites), perFamily)
	for k := 0; k < len(sites); k += step {
		s := sites[k]
		if s.lenOff < 0 {
			continue
		}
		body := splice(raw)
		for w := 0; w < s.lenWidth; w++ {
			if w == s.lenWidth-1 {
				body[s.lenOff+w] = 0x7f
			} else {
				body[s.lenOff+w] = 0xff
			}
		}
		maxForWidth := uint64(1)<<(7*uint(s.lenWidth)) - 1
		out = append(out, mutant{
			fmt.Sprintf("%s/field %d at depth %d claims %d bytes",
				label, s.field, s.depth, maxForWidth),
			body, maxForWidth > uint64(len(raw)-s.body),
		})
	}

	// 4. An unknown field spliced in front of a known one, at every depth.
	// Field 1729 is declared by no message of this IR, and check 5 refuses an
	// unknown field wherever it appears.
	unknown := []byte{0xc8, 0x6b, 0x01} // field 1729, varint, value 1
	step = stride(len(sites), perFamily)
	for k := 0; k < len(sites); k += step {
		s := sites[k]
		if s.depth != 0 {
			// Splicing into a nested payload would need every enclosing length
			// rewritten; at depth 0 the message is the buffer.
			continue
		}
		out = append(out, mutant{
			fmt.Sprintf("%s/an unknown field 1729 before field %d", label, s.field),
			splice(raw[:s.tag], unknown, raw[s.tag:]), true,
		})
	}

	// 5. A field repeated. Whether that is legal depends on the field, so the
	// contract is only that the loader answers.
	step = stride(len(sites), perFamily)
	for k := 0; k < len(sites); k += step {
		s := sites[k]
		if s.depth != 0 {
			continue
		}
		out = append(out, mutant{
			fmt.Sprintf("%s/field %d repeated", label, s.field),
			splice(raw[:s.end], raw[s.tag:s.end], raw[s.end:]), false,
		})
	}

	// 6 to 9. The same damage, applied inside a nested message rather than at
	// the top level. These are what reach the per field decoders: the duplicate
	// detector of every message, the width and range checks of every scalar.
	var paths [][]int
	nestedPaths(raw, nil, 0, &paths)
	pstep := stride(len(paths), perNested)
	for k := 0; k < len(paths); k += pstep {
		path := paths[k]
		key := fmt.Sprint(path)

		out = append(out, mutant{
			fmt.Sprintf("%s/an unknown field 1729 inside the message at %s", label, key),
			editNested(raw, path, func(payload []byte) []byte {
				return append(append([]byte(nil), payload...), unknown...)
			}), false,
		})

		// Every field of the message repeated, one at a time. Duplicate
		// detection is per field, so repeating only the first would leave the
		// detector of every other field of every message unexercised.
		var probe []wireSite
		if walkSites(rawAt(raw, path), 0, &probe) {
			const perMessage = 12
			for j := 0; j < len(probe) && j < perMessage; j++ {
				if probe[j].depth != 0 {
					continue
				}
				index := j
				out = append(out, mutant{
					fmt.Sprintf("%s/field %d of the message at %s repeated", label, probe[j].field, key),
					editNested(raw, path, func(payload []byte) []byte {
						var inner []wireSite
						if !walkSites(payload, 0, &inner) || index >= len(inner) {
							return payload
						}
						f := inner[index]
						return append(append(append([]byte(nil), payload[:f.end]...),
							payload[f.tag:f.end]...), payload[f.end:]...)
					}), false,
				})
			}
		}

		// The largest uint64, which overflows every narrower field the IR
		// declares, and eleven continuation bytes, which overflow the varint
		// itself.
		for _, v := range []struct {
			name  string
			bytes []byte
		}{
			{"the largest uint64", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}},
			{"a varint of eleven bytes", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}},
			{"the value 2, which no bool holds", []byte{0x02}},
		} {
			value := v.bytes
			out = append(out, mutant{
				fmt.Sprintf("%s/the first varint of the message at %s carries %s", label, key, v.name),
				editNested(raw, path, func(payload []byte) []byte {
					return replaceFirstVarint(payload, value)
				}), false,
			})
		}
	}

	res := make([]struct {
		name  string
		body  []byte
		fatal bool
	}, len(out))
	for i, m := range out {
		res[i].name, res[i].body, res[i].fatal = m.name, m.body, m.fatal
	}
	return res
}

// loadOutcome runs one mutant through the loader and reports what happened,
// turning a panic into a value so that the mutant responsible can be named.
func loadOutcome(body []byte) (b *gen.Bundle, panicked any, err error) {
	defer func() { panicked = recover() }()
	b, err = gen.Load(body)
	return b, nil, err
}

// TestWireMutationsAreAnswered is the contract. For every damaged encoding the
// loader must return exactly one of a bundle or an error, never both and never
// neither; it must not panic; an error must be one of the two documented kinds
// and must say something; and an accepted bundle must be usable, which is what
// generating from it proves.
//
// Mutations that cannot leave a loadable bundle are marked as such and must be
// refused rather than merely survived.
func TestWireMutationsAreAnswered(t *testing.T) {
	shipped := readSpecFile(t, "businessid-rules.binpb")
	synthetic := allOpcodesBundle().encode()
	t.Logf("shipped bundle %d bytes, synthetic bundle %d bytes", len(shipped), len(synthetic))

	for _, src := range []struct {
		label     string
		raw       []byte
		perFamily int
		perNested int
	}{
		// The synthetic bundle holds every operation and every parameter of the
		// IR in a few kilobytes, so it is damaged densely.
		{"synthetic", synthetic, 100000, 100000},
		// The shipped bundle is mostly membership tables, so it is sampled: what
		// it adds is the shapes only a real bundle has.
		{"shipped", shipped, 2500, 200},
	} {
		mutants := mutantsOf(t, src.label, src.raw, src.perFamily, src.perNested)
		var refused, accepted int
		reasons := map[string]int{}

		for _, m := range mutants {
			b, panicked, err := loadOutcome(m.body)
			if panicked != nil {
				t.Fatalf("%s: the loader panicked: %v", m.name, panicked)
			}
			switch {
			case err != nil && b != nil:
				t.Fatalf("%s: the loader returned both a bundle and %v", m.name, err)
			case err == nil && b == nil:
				t.Fatalf("%s: the loader returned neither a bundle nor an error", m.name)
			case err != nil:
				refused++
				if !errors.Is(err, gen.ErrInvalidRuleset) && !errors.Is(err, gen.ErrIncompatibleRuleset) {
					t.Fatalf("%s: refused with %v, which is neither documented kind", m.name, err)
				}
				text := err.Error()
				if _, detail, found := strings.Cut(text, ": "); !found || strings.TrimSpace(detail) == "" {
					t.Fatalf("%s: refused with %q, which states no reason", m.name, text)
				}
				reasons[firstWords(text)]++
			default:
				accepted++
				if m.fatal {
					t.Fatalf("%s: accepted, though the damage cannot leave a loadable bundle", m.name)
				}
				// An accepted bundle has to be usable, or the loader let
				// through something the emitter cannot handle.
				if _, err := gen.Generate(b, "businessid"); err != nil {
					t.Fatalf("%s: accepted, then failed to generate: %v", m.name, err)
				}
			}
		}

		if len(mutants) == 0 {
			t.Fatalf("%s: no mutant was built, so nothing was proved", src.label)
		}
		// A suite that stopped damaging anything would pass silently, so the
		// proportion refused is itself asserted.
		if refused*4 < len(mutants)*3 {
			t.Errorf("%s: only %d of %d mutants were refused; the mutations have stopped biting",
				src.label, refused, len(mutants))
		}
		t.Logf("%s: %d mutants, %d refused, %d accepted and generated, %d distinct refusals",
			src.label, len(mutants), refused, accepted, len(reasons))
	}
}

// firstWords keys a diagnostic by its opening, so the log can count how many
// different rules the mutations reached without printing thousands of lines.
func firstWords(s string) string {
	if _, detail, ok := strings.Cut(s, ": "); ok {
		s = detail
	}
	if i := strings.IndexByte(s, ':'); i > 0 {
		s = s[:i]
	}
	fields := strings.Fields(s)
	if len(fields) > 4 {
		fields = fields[:4]
	}
	return strings.Join(fields, " ")
}

// replaceFirstVarint rewrites the value of the first varint field of a message,
// leaving everything else in place.
func replaceFirstVarint(payload, value []byte) []byte {
	for i := 0; i < len(payload); {
		start := i
		tag, width, ok := readVarintAt(payload, i)
		if !ok {
			return payload
		}
		i += width
		switch int(tag & 7) {
		case 0:
			_, w, ok := readVarintAt(payload, i)
			if !ok {
				return payload
			}
			out := append([]byte(nil), payload[:i]...)
			out = append(out, value...)
			return append(out, payload[i+w:]...)
		case 1:
			i += 8
		case 5:
			i += 4
		case 2:
			n, w, ok := readVarintAt(payload, i)
			if !ok || n > uint64(len(payload)-i-w) {
				return payload
			}
			i += w + int(n)
		default:
			return payload
		}
		_ = start
	}
	return payload
}

// rawAt returns the payload the path names, so a mutation can be planned from
// what is actually there rather than guessed.
func rawAt(buf []byte, path []int) []byte {
	out := buf
	for _, want := range path {
		index := 0
		found := false
		for i := 0; i < len(out) && !found; {
			tag, width, ok := readVarintAt(out, i)
			if !ok {
				return nil
			}
			i += width
			switch int(tag & 7) {
			case 0:
				_, w, ok := readVarintAt(out, i)
				if !ok {
					return nil
				}
				i += w
			case 1:
				i += 8
			case 5:
				i += 4
			case 2:
				n, w, ok := readVarintAt(out, i)
				if !ok || n > uint64(len(out)-i-w) {
					return nil
				}
				i += w
				if index == want {
					out = out[i : i+int(n)]
					found = true
					break
				}
				index++
				i += int(n)
			default:
				return nil
			}
		}
		if !found {
			return nil
		}
	}
	return out
}
