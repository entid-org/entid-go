// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCommentText covers the guard that keeps a bundle string inside a single
// line Go comment. Check 6 of section 10 now constrains rules_version, so Load
// no longer lets such a version through; Generate is exported and takes a
// Bundle, so the guard stays as the last line of defence.
func TestCommentText(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2026.08.13", "2026.08.13"},
		{"0\x000", `"0\x000"`},
		{"1.0\nfunc x() {}", `"1.0\nfunc x() {}"`},
		{"\x7f", `"\x7f"`},
	} {
		if got := commentText(tc.in); got != tc.want {
			t.Errorf("commentText(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// TestExpansionProfile publishes what the shipped bundle expands to, and holds
// the invariant of check 14 over it.
//
// The figures matter beyond this repository. No real rule comes near the budget,
// so no conformance case can establish that two engines count the same thing;
// comparing the profile of the same bundle is the only way. Run it with:
//
//	go test -v -run TestExpansionProfile ./internal/gen
func TestExpansionProfile(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "businessid-rules.binpb"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(raw)
	if err != nil {
		t.Fatal(err)
	}

	total, worst := int64(0), int64(0)
	worstID := uint32(0)
	for _, p := range b.Programs {
		n := expansionOf(p)
		if n > MaxSteps {
			t.Errorf("program %d expands past the budget, which Load should have refused", p.ID)
		}
		total += n
		if n > worst {
			worst, worstID = n, p.ID
		}
	}
	t.Logf("rules %s: %d programs, %d instances in total, worst program %d at %d, budget %d",
		b.RulesVersion, len(b.Programs), total, worstID, worst, MaxSteps)
}

// TestExpansionCountsEachEmissionRootOnce pins the subtle half of check 14. The
// emission roots are the root node and every capture, but a capture the root
// already reaches is the same subtree under a name, not a second emission.
//
// It matters beyond this repository: the shipped bundle declares 54 captures
// and the root reaches all of them, so counting them twice reports 3204
// instances where the other implementations report 3069, and no conformance
// case could see the disagreement.
func TestExpansionCountsEachEmissionRootOnce(t *testing.T) {
	// n0 subject, n1 = f(n0), n2 = f(n1, n1), n3 = f(n0) unreachable from n2.
	program := func(captures ...uint32) *Program {
		p := &Program{
			ID: 1, Kind: ProgramFormat, RootNode: 2,
			Nodes: []*Node{
				{Op: OpSubject, OutputType: ValueString},
				{Op: OpSliceFrom, OutputType: ValueString, InputNodes: []uint32{0}},
				{Op: OpConcat, OutputType: ValueString, InputNodes: []uint32{1, 1}},
				{Op: OpSliceTo, OutputType: ValueString, InputNodes: []uint32{0}},
			},
		}
		for _, c := range captures {
			p.Captures = append(p.Captures, Capture{Name: "c", Node: c})
		}
		return p
	}

	// From the root alone: 1 + 2*(1 + 1) = 5.
	const rootAlone = 5
	for _, tc := range []struct {
		name     string
		captures []uint32
		want     int64
	}{
		{name: "no capture", want: rootAlone},
		{name: "a capture the root reaches", captures: []uint32{1}, want: rootAlone},
		{name: "the root itself as a capture", captures: []uint32{2}, want: rootAlone},
		// n3 is emitted by nobody else, so its two instances are added.
		{name: "a capture the root does not reach", captures: []uint32{3}, want: rootAlone + 2},
		{name: "both", captures: []uint32{1, 3}, want: rootAlone + 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := expansionOf(program(tc.captures...)); got != tc.want {
				t.Fatalf("expansion %d, want %d", got, tc.want)
			}
		})
	}
}

// TestExpansionCountsTheSubjectNode covers the third emission root. A program
// that declares a subject_node has its subtree emitted whether or not the root
// reaches it, so it is a root of its own; no program of the shipped bundle
// declares one, which is exactly why nothing would notice it missing.
func TestExpansionCountsTheSubjectNode(t *testing.T) {
	// n0 subject, n1 = f(n0), n2 = f(n1, n1) the root, n3 = f(n0) reached by
	// nothing but a subject_node.
	program := func(subject uint32, has bool) *Program {
		return &Program{
			ID: 1, Kind: ProgramFormat, RootNode: 2,
			SubjectNode: subject, HasSubject: has,
			Nodes: []*Node{
				{Op: OpSubject, OutputType: ValueString},
				{Op: OpSliceFrom, OutputType: ValueString, InputNodes: []uint32{0}},
				{Op: OpConcat, OutputType: ValueString, InputNodes: []uint32{1, 1}},
				{Op: OpSliceTo, OutputType: ValueString, InputNodes: []uint32{0}},
			},
		}
	}

	// From the root alone: 1 + 2*(1 + 1) = 5.
	const rootAlone = 5
	for _, tc := range []struct {
		name    string
		subject uint32
		has     bool
		want    int64
	}{
		{name: "no subject node", want: rootAlone},
		{name: "a subject node the root reaches", subject: 1, has: true, want: rootAlone},
		{name: "a subject node the root does not reach", subject: 3, has: true, want: rootAlone + 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := expansionOf(program(tc.subject, tc.has)); got != tc.want {
				t.Fatalf("expansion %d, want %d", got, tc.want)
			}
		})
	}
}

// TestExpansionIgnoresTheCaptureOrder covers the last decision of check 14. A
// capture may be reached by another capture, not only by the program root, and
// walking the list in its declared order then makes the count depend on how the
// captures happen to be listed — which is not a property of the bundle.
//
// Taking them from the highest index down settles it in one pass: an operand
// always sits at a lower index than the node reading it, so a capture reached by
// another is always seen after the one reaching it.
func TestExpansionIgnoresTheCaptureOrder(t *testing.T) {
	// n0 subject, n1 = f(n0) the root. n2 = f(n0) and n3 = f(n2) are reached by
	// no root but their own captures, and n3 reads n2.
	program := func(captures ...uint32) *Program {
		p := &Program{
			ID: 1, Kind: ProgramFormat, RootNode: 1,
			Nodes: []*Node{
				{Op: OpSubject, OutputType: ValueString},
				{Op: OpSliceFrom, OutputType: ValueString, InputNodes: []uint32{0}},
				{Op: OpSliceTo, OutputType: ValueString, InputNodes: []uint32{0}},
				{Op: OpConcat, OutputType: ValueString, InputNodes: []uint32{2, 2}},
			},
		}
		for i, c := range captures {
			p.Captures = append(p.Captures, Capture{Name: string(rune('a' + i)), Node: c})
		}
		return p
	}

	// The root costs 2. n3 costs 1 + 2*2 = 5 and contains n2, so the two
	// captures together add 5 whichever way round they are listed.
	const want = 2 + 5
	for _, order := range [][]uint32{{2, 3}, {3, 2}} {
		got := expansionOf(program(order...))
		if got != want {
			t.Errorf("captures listed as %v give %d instances, want %d", order, got, want)
		}
	}

	// The same shape once more, with the reaching capture listed first and the
	// reached one repeated, which is what an ordered walk gets wrong.
	if got := expansionOf(program(2, 3, 2)); got != want {
		t.Errorf("a repeated capture gives %d instances, want %d", got, want)
	}
}
