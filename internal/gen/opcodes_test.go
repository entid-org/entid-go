// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/libbusinessid/businessid-go/internal/gen"
)

// allOpcodesBundle exercises every one of the 63 V1 operations and every variant of them, including the
// sixteen the shipped rules never reach.
//
// A generator only ever meets the operations the current bundle happens to use;
// the rest break silently the day a rule starts using them. This bundle is what
// stops that.
func allOpcodesBundle() bundle {
	// Canonicalization: every step, including the conditional one.
	canon := prog{
		id: 1, kind: kCanon, root: 14,
		nodes: []node{
			cn(cTrim, nil),                     // 0
			cn(cRemoveWS, nil),                 // 1
			cn(cUpper, nil),                    // 2
			cn(cRemoveChars, nil, text("-./")), // 3
			cn(cReplacePrefix, nil, text("XX"), replacement("Y")), // 4
			cn(cPrepend, nil, text("P")),                          // 5
			cn(cAppend, nil, text("S")),                           // 6
			cn(cInsert, nil, index(1), text("I")),                 // 7
			cn(cLeftPad, nil, length(12), text("0")),              // 8
			cn(cCountry, nil),                                     // 9
			sn(sValue, nil),                                       // 10
			pr(pLenBetween, []uint32{10}, minLen(1), maxLen(99)),  // 11
			cn(cAppend, nil, text("!")),                           // 12
			cn(cWhen, []uint32{11, 12}),                           // 13
			cn(cSeq, []uint32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 13}),  // 14
		},
	}

	// Format: every string constructor, every predicate, and a call.
	format := prog{
		id: 2, kind: kFormat, root: 45,
		nodes: []node{
			sn(sSubject, nil),                                                   // 0
			sn(sConstant, nil, text("ZZ")),                                      // 1
			sn(sCountry, nil),                                                   // 2
			sn(sValue, nil),                                                     // 3
			sn(sSlice, []uint32{0}, start(0), end(2)),                           // 4
			sn(sSliceFrom, []uint32{0}, start(1)),                               // 5
			sn(sSliceTo, []uint32{0}, end(3)),                                   // 6
			sn(sBefore, []uint32{0}, text(".")),                                 // 7
			sn(sAfter, []uint32{0}, text(".")),                                  // 8
			sn(sStrip, []uint32{0}, text("P")),                                  // 9
			sn(sConcat, []uint32{1, 4, 5}),                                      // 10
			pr(pIsEmpty, []uint32{0}),                                           // 11
			pr(pNot, []uint32{11}),                                              // 12
			as(aRequire, []uint32{12}, reason(rcInvalidFormat), key("t.empty")), // 13
			pr(pIsAbsent, []uint32{9}),                                          // 14
			pr(pNot, []uint32{14}),                                              // 15
			as(aRequire, []uint32{15}, reason(rcInvalidFormat)),                 // 16
			pr(pEquals, []uint32{4, 1}),                                         // 17
			pr(pLenEq, []uint32{0}, length(12)),                                 // 18
			pr(pLenIn, []uint32{0}, lengths(3, 12, 20)),                         // 19
			pr(pLenBetween, []uint32{0}, minLen(1), maxLen(64)),                 // 20
			pr(pDigits, []uint32{5}),                                            // 21
			pr(pUpper, []uint32{4}),                                             // 22
			pr(pAlnum, []uint32{0}),                                             // 23
			pr(pCharset, []uint32{4}, text("XYZ")),                              // 24
			pr(pStarts, []uint32{0}, text("P")),                                 // 25
			pr(pEnds, []uint32{0}, text("S")),                                   // 26
			pr(pPrefixIn, []uint32{0}, values("AA", "PP")),                      // 27
			pr(pCharAt, []uint32{0}, index(0), text("PX")),                      // 28
			pr(pContains, []uint32{0}, text(".")),                               // 29
			pr(pProfile, nil, text("compatible")),                               // 30
			pr(pAny, []uint32{17, 18, 19}),                                      // 31
			pr(pAll, []uint32{20, 21, 22}),                                      // 32
			pr(pAny, []uint32{23, 24, 25, 26, 27, 28, 29, 30}),                  // 33
			pr(pAll, []uint32{31, 32, 33}),                                      // 34
			as(aRequire, []uint32{34}, reason(rcInvalidChars), key("t.shape")),  // 35
			pr(pIsAbsent, []uint32{10}),                                         // 36
			pr(pNot, []uint32{36}),                                              // 37
			as(aRequire, []uint32{37}, reason(rcInvalidFormat)),                 // 38
			// The views the assertions below reach keep every remaining string
			// constructor alive; an unreferenced node is dead code the emitter
			// is free to drop.
			pr(pIsAbsent, []uint32{2}),                          // 39, country_code()
			pr(pIsEmpty, []uint32{6}),                           // 40, slice_to
			pr(pIsAbsent, []uint32{7}),                          // 41, before_first
			pr(pAny, []uint32{39, 40, 41}),                      // 42
			as(aRequire, []uint32{42}, reason(rcInvalidFormat)), // 43
			// A call runs the callee on a caller supplied view.
			call(callFormat, tAssertion, []uint32{8}, program(3)), // 44
			as(aSeq, []uint32{13, 16, 35, 38, 43, 44}),            // 45
		},
	}

	// The callee of the format call.
	callee := prog{
		id: 3, kind: kFormat, root: 3,
		nodes: []node{
			sn(sSubject, nil),
			pr(pDigits, []uint32{0}),
			as(aRequire, []uint32{1}, reason(rcInvalidChars), key("t.callee")),
			as(aSeq, []uint32{2}),
		},
	}

	// Checksum: every primitive, every integer constructor, every combinator.
	checksum := prog{
		id: 4, kind: kCheck, root: 30,
		nodes: []node{
			sn(sSubject, nil),                          // 0
			sn(sSlice, []uint32{0}, start(2), end(10)), // 1
			sn(sSliceFrom, []uint32{0}, start(2)),      // 2
			in64(iDigits, []uint32{1}),                 // 3
			in64(iModDigits, []uint32{2}, modulus(97)), // 4
			in64(iWeighted, []uint32{1}, weights(1, 2, 3), alignment(alignLeft), mapping(mapDigit)), // 5
			in64(iWeighted, []uint32{1}, weights(9, 8), alignment(alignRight), mapping(mapDigit)),   // 6
			in64(iWeighted, []uint32{1}, weights(7, 6), alignment(alignCycle), mapping(mapBase36)),  // 7
			in64(iModulo, []uint32{5}, modulus(11)),                                                 // 8
			in64(iComplement, []uint32{4}, modulus(97)),                                             // 9
			in64(iRemainder, []uint32{8}, remainders(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0)),              // 10
			ck(xLuhn, []uint32{2}),                            // 11
			ck(xMod97, []uint32{0}, key("t.mod97")),           // 12
			ck(xCmpDigit, []uint32{10, 0}, index(11)),         // 13
			ck(xCmpSlice, []uint32{9, 0}, start(10), end(12)), // 14
			ck(xCmpSlice, []uint32{3, 0}, start(2), end(10)),  // 15
			ck(xCmpDigit, []uint32{6, 0}, index(0)),           // 16
			ck(xCmpDigit, []uint32{7, 0}, index(1)),           // 17
			pr(pDigits, []uint32{2}),                          // 18
			ck(xWhen, []uint32{18, 13}),                       // 19
			ck(xUnsupported, nil, reason(rcNotPublished)),     // 20
			ck(xChoose, []uint32{19, 20}),                     // 21
			// A remainder branching on its own value, which is what
			// INTEGER_IS and COMPARE_CONSTANT exist for.
			pr(pIntegerIs, []uint32{8}, constant(10)),                  // 22
			ck(xCmpConstant, []uint32{9}, constant(1), key("t.const")), // 23
			ck(xWhen, []uint32{22, 23}),                                // 24
			ck(xUnsupported, nil, reason(rcNotPublished)),              // 25
			ck(xChoose, []uint32{24, 25}),                              // 26
			ck(xAll, []uint32{11, 12, 14, 15, 16, 17, 21, 26}),         // 27
			// The weighted sum variant that carries its own alphabet, the one
			// capability 42 introduces.
			in64(iWeighted, []uint32{1}, weights(5, 4), alignment(alignCycle),
				mapping(mapCustom), alphabet("0123456789ABCDEFGHJKLMNPQRTUWXY")), // 28
			ck(xCmpConstant, []uint32{28}, constant(3)), // 29
			ck(xAll, []uint32{27, 29}),                  // 30
		},
	}

	// A second checksum program reached by a call, using ANY_CHECK.
	checksumCallee := prog{
		id: 5, kind: kCheck, root: 3,
		nodes: []node{
			sn(sSubject, nil),
			ck(xLuhn, []uint32{0}),
			ck(xUnsupported, nil, reason(rcNotPublished)),
			ck(xAny, []uint32{1, 2}),
		},
	}

	// The entry checksum calls the second one.
	entryChecksum := prog{
		id: 6, kind: kCheck, root: 3,
		nodes: []node{
			sn(sSubject, nil),
			sn(sSliceFrom, []uint32{0}, start(2)),
			call(callChecksum, tChecksum, []uint32{1}, program(5)),
			ck(xAll, []uint32{2}),
		},
	}

	// A pre-canonicalizer, restricted to the routing step set.
	pre := prog{
		id: 7, kind: kCanon, root: 3,
		nodes: []node{
			cn(cTrim, nil), cn(cRemoveWS, nil), cn(cUpper, nil),
			cn(cSeq, []uint32{0, 1, 2}),
		},
	}

	return bundle{
		formatVersion: 1,
		rulesVersion:  "2026.08.0",
		features:      allFeatures,
		defs: []def{
			{id: 1, kind: "t", country: "FR", canon: 1, format: 2, checksum: 4, hasChecksum: true, tier: 1},
			{id: 2, kind: "t", country: "GR", canon: 1, format: 2, checksum: 6, hasChecksum: true, tier: 2},
			{id: 3, kind: "u", canon: 7, format: 3, absentReason: rcNotPublished, tier: 1},
		},
		programs: []prog{canon, format, callee, checksum, checksumCallee, entryChecksum, pre},
		dispatchers: []dispatch{
			{kind: "t", aliases: []string{"t_alias"}, pre: 7, targets: []target{
				{country: "FR", prefixes: []string{"P", "PP"}, canonicalPrefix: "P", definition: 1},
				{country: "GR", prefixes: []string{"GG"}, canonicalPrefix: "GG", definition: 2, unprefixed: true},
			}},
			{kind: "u", pre: 7, targets: []target{{definition: 3}}},
		},
	}
}

// TestEveryOpcodeLoadsAndGenerates drives all 63 operations through the loader
// and the emitter.
func TestEveryOpcodeLoadsAndGenerates(t *testing.T) {
	b, err := gen.Load(allOpcodesBundle().encode())
	if err != nil {
		t.Fatalf("a bundle using every operation must load: %v", err)
	}
	src, err := gen.Generate(b, "businessid")
	if err != nil {
		t.Fatalf("a bundle using every operation must generate: %v", err)
	}
	code := string(src)

	// Every operation must reach the emitted code in the shape the runtime
	// implements. A silently dropped lowering would produce code that compiles
	// and answers wrongly.
	for _, want := range []string{
		// String constructors.
		`rt.Value("ZZ")`, `rt.Value(c.value)`, `subject`, `rt.Absent`,
		".Slice(0, 2)", ".SliceFrom(1)", ".SliceTo(3)",
		`.BeforeFirst(".")`, `.AfterFirst(".")`, `.StripPrefix("P")`, "rt.Concat(",
		// Predicates.
		".IsEmpty()", ".IsAbsent()", ".Equals(", ".LengthEq(12)",
		".LengthIn(3, 12, 20)", ".LengthBetween(1, 64)",
		".ASCIIDigits()", ".ASCIIUpperLetters()", ".ASCIIAlphanumeric()",
		`.ASCIICharset("XYZ")`, `.HasPrefix("P")`, `.HasSuffix("S")`,
		`.PrefixIn("AA", "PP")`, `.CharAtIn(0, "PX")`, `.Contains(".")`,
		"c.profile == Compatible", " && ", " || ", "!",
		// Integers.
		"rt.DigitsToInteger(", "rt.ModDigits(", "rt.WeightedSum(",
		"rt.AlignLeft", "rt.AlignRight", "rt.AlignCycle",
		"rt.MapDigitValue", "rt.MapAlnumBase36",
		`rt.WeightedSumAlphabet(`, `"0123456789ABCDEFGHJKLMNPQRTUWXY"`,
		".Modulo(11)", ".Complement(97)", ".RemainderMap(",
		// Canonicalization steps.
		"w.TrimWhitespace()", "w.RemoveWhitespace()", "w.UppercaseASCII()",
		`w.RemoveChars("-./")`, `w.ReplacePrefix("XX", "Y")`, `w.Prepend("P")`,
		`w.Append("S")`, `w.Insert(1, "I")`, `w.LeftPad(12, "0")`,
		// Checksums.
		"rt.Luhn(", "rt.ISO7064Mod97(", "rt.CompareDigit(", "rt.CompareSlice(",
		"rt.CompareConstant(", ".Is(10)",
		"allChecks(", "anyCheck(", "unsupportedStep(ReasonChecksumNotPublished",
		"switch {", // CHOOSE
		// Calls and assertions.
		"invalidStep(", "validStep(",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("the generated code has no %s", want)
		}
	}

	// The conditional canonicalization step must be a real branch.
	if !strings.Contains(code, "if rt.Value(w.Str()).LengthBetween(1, 99) {") {
		t.Error("the conditional canonicalization step did not become a branch")
	}
	// PREPEND_COUNTRY_IF_MISSING must fold against the target's own prefixes.
	if !strings.Contains(code, `if !w.HasPrefix("P") && !w.HasPrefix("PP") {`) {
		t.Error("prepend_country_if_missing did not fold against the accepted prefixes")
	}
	// A global definition reports the absent view for country_code().
	if !strings.Contains(code, "rt.Absent") {
		t.Error("country_code() did not fold to the absent view")
	}
}

// TestRefusalsBeyondTheCorpus covers the checks the hostile fixtures of the
// corpus do not reach, so that every refusal path has a test.
func TestRefusalsBeyondTheCorpus(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*bundle)
		wantErr  error
		contains string
	}{
		{
			name:    "a capability the generator does not implement",
			mutate:  func(b *bundle) { b.features = append(b.features, 77) },
			wantErr: gen.ErrIncompatibleRuleset, contains: "capability 77",
		},
		{
			name:    "capabilities out of order",
			mutate:  func(b *bundle) { b.features = []uint32{2, 1} },
			wantErr: gen.ErrInvalidRuleset, contains: "strictly ascending",
		},
		{
			name:    "a digest of the wrong width",
			mutate:  func(b *bundle) { b.digest = make([]byte, 31) },
			wantErr: gen.ErrInvalidRuleset, contains: "source_digest",
		},
		{
			name:    "an unknown field at the root",
			mutate:  func(b *bundle) { b.extra = vfield(404, 1) },
			wantErr: gen.ErrInvalidRuleset, contains: "unknown field 404",
		},
		{
			name:    "a definition with neither a checksum nor a reason",
			mutate:  func(b *bundle) { b.defs[2].absentReason = 0 },
			wantErr: gen.ErrInvalidRuleset, contains: "neither a checksum program nor an absence reason",
		},
		{
			name: "a definition with both a checksum and a reason",
			mutate: func(b *bundle) {
				b.defs[2].hasChecksum, b.defs[2].checksum = true, 4
			},
			wantErr: gen.ErrInvalidRuleset, contains: "both a checksum program and an absence reason",
		},
		{
			name:    "a malformed kind",
			mutate:  func(b *bundle) { b.defs[0].kind, b.dispatchers[0].kind = "T!", "T!" },
			wantErr: gen.ErrInvalidRuleset, contains: "malformed kind",
		},
		{
			name:    "a malformed country",
			mutate:  func(b *bundle) { b.defs[0].country = "fra" },
			wantErr: gen.ErrInvalidRuleset, contains: "malformed country",
		},
		{
			name: "definitions out of order",
			mutate: func(b *bundle) {
				b.defs[0], b.defs[1] = b.defs[1], b.defs[0]
			},
			wantErr: gen.ErrInvalidRuleset, contains: "normative order",
		},
		{
			name: "a kind alias claimed by two dispatchers",
			mutate: func(b *bundle) {
				b.dispatchers[1].aliases = []string{"t_alias"}
			},
			wantErr: gen.ErrInvalidRuleset, contains: "claimed by both",
		},
		{
			name: "a pre-canonicalizer using a forbidden step",
			mutate: func(b *bundle) {
				b.dispatchers[0].pre = 2 // the full canonicalizer
			},
			wantErr: gen.ErrInvalidRuleset, contains: "pre-canonicalization program",
		},
		{
			name: "two targets selectable without a country",
			mutate: func(b *bundle) {
				b.dispatchers[0].targets[0].unprefixed = true
			},
			wantErr: gen.ErrInvalidRuleset, contains: "without a country",
		},
		{
			name: "a target whose country contradicts its definition",
			mutate: func(b *bundle) {
				b.dispatchers[0].targets[0].country = "BE"
			},
			wantErr: gen.ErrInvalidRuleset, contains: "disagree on the country",
		},
		{
			name: "an identifier id used twice",
			mutate: func(b *bundle) {
				b.defs[1].id = 1
			},
			wantErr: gen.ErrInvalidRuleset, contains: "appears twice",
		},
		{
			name: "a GLOBAL definition whose canonicalizer prepends a country",
			mutate: func(b *bundle) {
				b.defs[2].canon = 1 // the canonicalizer using PREPEND_COUNTRY_IF_MISSING
			},
			wantErr: gen.ErrInvalidRuleset, contains: "GLOBAL but its canonicalizer prepends a country",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := allOpcodesBundle()
			tc.mutate(&b)
			_, err := gen.Load(b.encode())
			if err == nil {
				t.Fatal("the bundle loaded but a refusal was expected")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("diagnostic %q does not mention %q", err, tc.contains)
			}
		})
	}
}

// TestLeftPadIsBounded checks the bound ir.md places on left_pad, which is what
// lets a generator size a workspace from it.
func TestLeftPadIsBounded(t *testing.T) {
	for _, tc := range []struct {
		name string
		size uint32
	}{
		{"above the slice bound", gen.MaxSliceBound + 1},
		{"zero", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := allOpcodesBundle()
			b.programs[0].nodes[8] = cn(cLeftPad, nil, length(tc.size), text("0"))
			_, err := gen.Load(b.encode())
			if !errors.Is(err, gen.ErrInvalidRuleset) {
				t.Fatalf("got %v, want invalid_ruleset", err)
			}
			if !strings.Contains(err.Error(), "left_pad length") {
				t.Fatalf("diagnostic %q does not mention the bound", err)
			}
		})
	}

	// The bound itself loads, and sizes the workspace rather than refusing it.
	b := allOpcodesBundle()
	b.programs[0].nodes[8] = cn(cLeftPad, nil, length(gen.MaxSliceBound), text("0"))
	loaded, err := gen.Load(b.encode())
	if err != nil {
		t.Fatalf("a left_pad at the bound must load: %v", err)
	}
	if _, err := gen.Generate(loaded, "businessid"); err != nil {
		t.Fatalf("a left_pad at the bound must generate: %v", err)
	}
}

// TestOversizedWorkspaceIsRefused checks the one limit this generator adds on
// top of the specification: steps that compose past what a single conforming
// step can add would need a workspace too large for a goroutine stack.
func TestOversizedWorkspaceIsRefused(t *testing.T) {
	b := allOpcodesBundle()
	// Five prepends of the largest constant compose past the derived margin.
	big := strings.Repeat("x", gen.MaxConstantBytes)
	canon := &b.programs[0]
	for i := range 5 {
		canon.nodes[i] = cn(cPrepend, nil, text(big))
	}

	loaded, err := gen.Load(b.encode())
	if err != nil {
		t.Fatalf("the bundle itself is valid: %v", err)
	}
	if _, err := gen.Generate(loaded, "businessid"); err == nil {
		t.Fatal("a canonicalizer beyond the derived margin must be refused")
	} else if !strings.Contains(err.Error(), "a single conforming step can add") {
		t.Fatalf("diagnostic %q does not explain the bound", err)
	}
}

// TestEmptyMessageKeyIsRefused checks that a declared but empty message key is
// refused. An empty key is indistinguishable from an absent one once a result
// reaches a caller, so accepting it would make two bundles observationally
// identical while the corpus treats them as different.
func TestEmptyMessageKeyIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*bundle)
	}{
		{
			name: "on an assertion",
			mutate: func(b *bundle) {
				b.programs[1].nodes[13] = as(aRequire, []uint32{12},
					reason(rcInvalidFormat), key(""))
			},
		},
		{
			name: "on a checksum",
			mutate: func(b *bundle) {
				b.programs[3].nodes[12] = ck(xMod97, []uint32{0}, key(""))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := allOpcodesBundle()
			tc.mutate(&b)
			_, err := gen.Load(b.encode())
			if !errors.Is(err, gen.ErrInvalidRuleset) {
				t.Fatalf("got %v, want invalid_ruleset", err)
			}
			if !strings.Contains(err.Error(), "message key must not be empty") {
				t.Fatalf("diagnostic %q does not explain the refusal", err)
			}
		})
	}
}

// TestSourceTier covers capability 41: the tier a source states reaches the
// generated coverage, so a caller can tell the authority's own text from
// someone's reading of it.
func TestSourceTier(t *testing.T) {
	for _, tc := range []struct {
		tier int
		want gen.SourceTier
		emit string
	}{
		{tier: 1, want: gen.TierPrimary, emit: "Tier:           TierPrimary"},
		{tier: 2, want: gen.TierSecondary, emit: "Tier:           TierSecondary"},
	} {
		raw := allOpcodesBundle()
		raw.defs[0].tier = tc.tier
		b, err := gen.Load(raw.encode())
		if err != nil {
			t.Fatalf("tier %d: %v", tc.tier, err)
		}
		if got := b.Identifiers[0].Sources[0].Tier; got != tc.want {
			t.Errorf("tier %d decoded as %v, want %v", tc.tier, got, tc.want)
		}
		src, err := gen.Generate(b, "businessid")
		if err != nil {
			t.Fatalf("tier %d: %v", tc.tier, err)
		}
		if !strings.Contains(string(src), tc.emit) {
			t.Errorf("tier %d: the generated coverage has no %q", tc.tier, tc.emit)
		}
	}
}

// TestSourceTierDeclaresItsCapability covers capability 41. Section 41 of
// features.md turns on one property of the schema: tier is not optional, so a
// Source that omits the field and one that sets SOURCE_TIER_UNSPECIFIED are the
// same bytes. UNSPECIFIED therefore means the source states no tier, and a
// bundle whose sources all state none does not declare the capability at all.
// Refusing it would make 41 mandatory the moment 40 is, which is the opposite of
// the independence a separate id exists to give.
func TestSourceTierDeclaresItsCapability(t *testing.T) {
	// The bundle carries three definitions; tiers are set on all of them at
	// once so that no source is left stating one.
	statedBy := func(tiers ...int) bundle {
		raw := allOpcodesBundle()
		for i := range raw.defs {
			raw.defs[i].tier = tiers[i]
		}
		return raw
	}

	t.Run("no source states a tier", func(t *testing.T) {
		raw := statedBy(0, 0, 0)
		raw.features = withoutFeature(raw.features, 41)
		b, err := gen.Load(raw.encode())
		if err != nil {
			t.Fatalf("a bundle whose sources state no tier must load without capability 41: %v", err)
		}
		if got := b.Identifiers[0].Sources[0].Tier; got != gen.TierUnspecified {
			t.Errorf("tier %v, want unspecified", got)
		}
	})

	t.Run("one source states a tier", func(t *testing.T) {
		raw := statedBy(0, 2, 0)
		raw.features = withoutFeature(raw.features, 41)
		if _, err := gen.Load(raw.encode()); !errors.Is(err, gen.ErrInvalidRuleset) {
			t.Fatalf("got %v, want invalid_ruleset for an undeclared capability 41", err)
		}
	})

	t.Run("outside the enumeration", func(t *testing.T) {
		if _, err := gen.Load(statedBy(1, 7, 2).encode()); !errors.Is(err, gen.ErrInvalidRuleset) {
			t.Fatalf("got %v, want invalid_ruleset", err)
		}
	})
}

// TestCustomAlphabet covers capability 42. The alphabet is what gives a code
// point its value, so every way of stating an ambiguous one is refused at load
// time: two engines resolving a repeated code point differently is the shape of
// divergence a corpus cannot catch.
func TestCustomAlphabet(t *testing.T) {
	// Node 5 of the checksum program is the LEFT aligned weighted sum.
	const weighted = 5
	for _, tc := range []struct {
		name     string
		mapping  int
		alphabet string
		hasAlpha bool
		wantErr  bool
	}{
		{name: "custom alphabet", mapping: mapCustom, alphabet: "0123456789ABCDEFGHJKLMNPQRTUWXY", hasAlpha: true},
		{name: "missing under custom", mapping: mapCustom, wantErr: true},
		{name: "present under digit value", mapping: mapDigit, alphabet: "0123", hasAlpha: true, wantErr: true},
		{name: "present under base36", mapping: mapBase36, alphabet: "0123", hasAlpha: true, wantErr: true},
		{name: "empty", mapping: mapCustom, alphabet: "", hasAlpha: true, wantErr: true},
		{name: "repeated code point", mapping: mapCustom, alphabet: "0123456789A0", hasAlpha: true, wantErr: true},
		{name: "repeated non ASCII code point", mapping: mapCustom, alphabet: "0é1é", hasAlpha: true, wantErr: true},
		{name: "non ASCII", mapping: mapCustom, alphabet: "0123456789é", hasAlpha: true},
		{name: "257 code points", mapping: mapCustom, alphabet: longAlphabet(257), hasAlpha: true, wantErr: true},
		{name: "256 code points", mapping: mapCustom, alphabet: longAlphabet(256), hasAlpha: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := allOpcodesBundle()
			p := &raw.programs[3] // the checksum program
			fields := []field{weights(1, 2, 3), alignment(alignLeft), mapping(tc.mapping)}
			if tc.hasAlpha {
				fields = append(fields, alphabet(tc.alphabet))
			}
			p.nodes[weighted] = in64(iWeighted, []uint32{1}, fields...)
			_, err := gen.Load(raw.encode())
			if tc.wantErr {
				if !errors.Is(err, gen.ErrInvalidRuleset) {
					t.Fatalf("got %v, want invalid_ruleset", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("the bundle must load: %v", err)
			}
		})
	}
}

// TestCustomAlphabetNeedsItsCapability covers the declaration rule: capability
// 42 belongs to the variant, not to the operation, so a weighted sum over
// digits must not oblige an engine to implement an alphabet it never reads.
func TestCustomAlphabetNeedsItsCapability(t *testing.T) {
	raw := allOpcodesBundle()
	raw.features = withoutFeature(raw.features, 42)
	// Node 28 of the checksum program is the only custom alphabet sum; over
	// digits, the same operation must not need the capability.
	p := &raw.programs[3]
	p.nodes[28] = in64(iWeighted, []uint32{1}, weights(5, 4), alignment(alignCycle), mapping(mapDigit))

	if _, err := gen.Load(raw.encode()); err != nil {
		t.Fatalf("a bundle whose weighted sums read digits must load without capability 42: %v", err)
	}

	p.nodes[28] = in64(iWeighted, []uint32{1},
		weights(5, 4), alignment(alignCycle), mapping(mapCustom), alphabet("0123456789"))
	if _, err := gen.Load(raw.encode()); !errors.Is(err, gen.ErrInvalidRuleset) {
		t.Fatalf("got %v, want invalid_ruleset for an undeclared capability 42", err)
	}
}

// TestUnknownOperationIsAVersionGapWhenDeclared covers the other half of the
// order section 10 fixes. Check 10 names the unknown operation, and it comes
// after the capability check, so an engine that resolved the opcode while
// decoding would answer invalid_ruleset where the bundle is only newer.
//
// The two cases differ by one thing: whether the bundle declares a capability
// this generator does not implement. Same unknown opcode, two different
// answers, and the difference is exactly "upgrade me" against "this file is
// forged".
func TestUnknownOperationIsAVersionGapWhenDeclared(t *testing.T) {
	// Predicate kind 250 belongs to no V1 registry.
	forge := func() bundle {
		raw := allOpcodesBundle()
		p := &raw.programs[1] // the format program
		p.nodes[11] = pr(250, []uint32{0})
		return raw
	}

	t.Run("declared", func(t *testing.T) {
		raw := forge()
		raw.features = append(raw.features, 254)
		if _, err := gen.Load(raw.encode()); !errors.Is(err, gen.ErrIncompatibleRuleset) {
			t.Fatalf("got %v, want incompatible_ruleset", err)
		}
	})

	t.Run("undeclared", func(t *testing.T) {
		if _, err := gen.Load(forge().encode()); !errors.Is(err, gen.ErrInvalidRuleset) {
			t.Fatalf("got %v, want invalid_ruleset", err)
		}
	})
}

// doublingProgram builds a format program whose every string node reads the
// previous one twice. The node count stays tiny and the graph stays acyclic, so
// checks 8 to 13 see nothing; the number of instances a generator emits doubles
// at each level.
//
// With c(0) = 1 for the subject, c(k) = 1 + 2*c(k-1), so c(k) = 2^(k+1) - 1.
func doublingProgram(id uint32, levels int) prog {
	nodes := []node{sn(sSubject, nil)}
	for k := 1; k <= levels; k++ {
		prev := uint32(k - 1)
		nodes = append(nodes, sn(sConcat, []uint32{prev, prev}))
	}
	last := uint32(levels)
	nodes = append(nodes,
		pr(pIsAbsent, []uint32{last}),
		pr(pNot, []uint32{uint32(levels + 1)}),
		as(aRequire, []uint32{uint32(levels + 2)}, reason(rcInvalidFormat)),
		as(aSeq, []uint32{uint32(levels + 3)}),
	)
	return prog{id: id, kind: kFormat, root: uint32(levels + 4), nodes: nodes}
}

// TestExpansionBudget covers check 14. A DAG whose every node reads the previous
// one twice expands exponentially while passing every other check, which is a
// denial of service against the generator rather than against the engine.
//
// The bound is the evaluation budget rather than a new number: a generated
// program may not carry more instances than an interpreter would have taken
// steps to run it once.
func TestExpansionBudget(t *testing.T) {
	// c(15) = 65535 instances for the chain, plus the four nodes above it.
	// c(16) = 131071, past the 100000 budget.
	t.Run("within the budget", func(t *testing.T) {
		raw := allOpcodesBundle()
		raw.programs[1] = doublingProgram(2, 15)
		if _, err := gen.Load(raw.encode()); err != nil {
			t.Fatalf("65539 instances are within the budget: %v", err)
		}
	})

	t.Run("past the budget", func(t *testing.T) {
		raw := allOpcodesBundle()
		raw.programs[1] = doublingProgram(2, 16)
		if _, err := gen.Load(raw.encode()); !errors.Is(err, gen.ErrInvalidRuleset) {
			t.Fatalf("got %v, want invalid_ruleset", err)
		}
	})

	t.Run("a chain no counter could hold", func(t *testing.T) {
		// 200 levels is 2^201 instances: the count must saturate rather than
		// overflow into a small number that passes.
		raw := allOpcodesBundle()
		raw.programs[1] = doublingProgram(2, 200)
		if _, err := gen.Load(raw.encode()); !errors.Is(err, gen.ErrInvalidRuleset) {
			t.Fatalf("got %v, want invalid_ruleset", err)
		}
	})
}
