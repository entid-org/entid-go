// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"strings"
)

// naming assigns a stable Go identifier to every program, definition and
// dispatcher of a bundle.
//
// Names come from the business identity of a rule — kind and country — so that
// a diff of the generated code reads as a diff of the rules. A program no
// definition owns falls back to its numeric id, which the compiler assigns
// deterministically from the sorted symbol names.
type naming struct {
	program    map[uint32]string // program id -> Go function name
	definition map[uint32]string // definition id -> Go suffix, e.g. "VatBE"
	dispatcher map[string]string // dispatcher kind -> Go suffix, e.g. "Vat"
}

func newNaming(b *Bundle) *naming {
	n := &naming{
		program:    make(map[uint32]string, len(b.Programs)),
		definition: make(map[uint32]string, len(b.Identifiers)),
		dispatcher: make(map[string]string, len(b.Dispatchers)),
	}
	for _, d := range b.Identifiers {
		n.definition[d.ID] = definitionSuffix(d)
	}
	for _, d := range b.Dispatchers {
		n.dispatcher[d.Kind] = pascal(d.Kind)
	}

	// A program named after the definition that owns it reads best. The order
	// below is the priority when a program fills several roles.
	for _, d := range b.Identifiers {
		suffix := n.definition[d.ID]
		n.claim(d.CanonicalizationProgram, "canonicalize"+suffix)
		n.claim(d.FormatProgram, "format"+suffix)
		if d.HasChecksumProgram {
			n.claim(d.ChecksumProgram, "checksum"+suffix)
		}
	}
	for _, d := range b.Dispatchers {
		n.claim(d.PreCanonicalizationProgram, fmt.Sprintf("preCanonicalize%d", d.PreCanonicalizationProgram))
	}
	for _, p := range b.Programs {
		n.claim(p.ID, fmt.Sprintf("program%d", p.ID))
	}
	return n
}

// claim names a program unless it already carries a name.
func (n *naming) claim(id uint32, name string) {
	if _, taken := n.program[id]; !taken {
		n.program[id] = name
	}
}

// Program returns the Go function name of a program.
func (n *naming) Program(id uint32) string { return n.program[id] }

// Definition returns the Go suffix identifying a definition.
func (n *naming) Definition(id uint32) string { return n.definition[id] }

// DefConst returns the Go constant naming a definition.
func (n *naming) DefConst(id uint32) string { return "def" + n.definition[id] }

// DispatcherConst returns the Go constant naming a dispatcher.
func (n *naming) DispatcherConst(kind string) string { return "dispatcher" + n.dispatcher[kind] }

// definitionSuffix builds "VatBE", "SirenFR" or "Lei" from a definition.
func definitionSuffix(d *IdentifierDefinition) string {
	if !d.HasCountryCode {
		return pascal(d.Kind)
	}
	return pascal(d.Kind) + d.CountryCode
}

// pascal turns a kind token into a Go identifier fragment: "vat" becomes "Vat",
// "fr_siren" becomes "FrSiren".
func pascal(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		switch {
		case r == '_' || r == '-':
			upper = true
		case upper:
			b.WriteRune(r - 32*boolToRune(r >= 'a' && r <= 'z'))
			upper = false
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func boolToRune(b bool) rune {
	if b {
		return 1
	}
	return 0
}

// capitalize upper cases the first byte of an ASCII identifier.
func capitalize(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-32) + s[1:]
}
