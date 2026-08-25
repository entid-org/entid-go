// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package entid

import (
	"unicode/utf8"

	rt "github.com/entid-org/entid-go/internal/runtime"
)

// This file holds the parts of the engine that do not depend on the rules: the
// dispatch algorithm of section 5 of ir.md, the validation pipeline of section
// 6, and the small vocabulary the generated rules are written in. Everything
// that does depend on the rules lives in rules_gen.go.

// ruleCtx carries everything a generated rule may read. It is passed by value:
// three words on the stack, never a pointer to keep alive.
type ruleCtx struct {
	// value is what value() yields inside a format or checksum rule: the
	// canonical value of the identifier under validation.
	value string

	// profile is the effective validation profile, which PROFILE_IS reads.
	profile Profile
}

// stepResult is the outcome of one generated rule.
type stepResult struct {
	status Status
	reason Reason
	key    string
}

func validStep() stepResult { return stepResult{status: Valid, reason: ReasonOK} }

func invalidStep(r Reason, key string) stepResult {
	return stepResult{status: Invalid, reason: r, key: key}
}

//nolint:unparam // key is empty for every rule of the current bundle; it carries the message key of any rule that declares one.
func unsupportedStep(r Reason, key string) stepResult {
	return stepResult{status: Unsupported, reason: r, key: key}
}

// fromOutcome turns a checksum primitive result into a step, attaching the
// message key the rule declared, if any.
//
//nolint:unparam // key is empty for every checksum of the current bundle; it carries the message key of any that declares one.
func fromOutcome(o rt.Outcome, key string) stepResult {
	switch o {
	case rt.Valid:
		return stepResult{status: Valid, reason: ReasonOK, key: key}
	case rt.Invalid:
		return stepResult{status: Invalid, reason: ReasonInvalidChecksum, key: key}
	}
	return stepResult{status: Unsupported, reason: ReasonUnsupportedChecksum, key: key}
}

// routed is what the dispatch algorithm reports, whether or not it selected a
// definition. Its identity fields follow the table of section 5.1 of ir.md.
type routed struct {
	kind      string
	canonical string
	country   string
	def       definition
	selected  bool

	// profile is the effective profile of section 5.2: the caller's, or the
	// default of the selected definition when the caller supplied none.
	profile Profile

	// status and reason describe why routing stopped, when it did.
	status Status
	reason Reason
}

// route implements the nine step dispatch algorithm of section 5 of ir.md.
//
// requested is the profile the caller supplied, empty when they supplied none.
// Section 5.2 resolves the profile in two moments: dispatch runs under the
// caller's profile, or compatible; once a definition is selected, its own
// default applies if and only if the caller supplied nothing.
func route(in Input, requested Profile, w *rt.Buf) routed {
	dispatchProfile := requested
	if dispatchProfile == "" {
		dispatchProfile = Compatible
	}
	out := routeUnder(in, dispatchProfile, w)
	out.profile = dispatchProfile
	if out.selected && requested == "" {
		out.profile = definitionDefaultProfile(out.def)
	}
	return out
}

// routeUnder runs the dispatch algorithm under one profile. The
// pre-canonicalizer runs as soon as the dispatcher resolves, before any country
// decision, so a result that stops later still carries the pre-canonical value.
func routeUnder(in Input, profile Profile, w *rt.Buf) routed {
	// The kind reported before a dispatcher resolves is the requested token
	// after trim and lower casing, even when it is malformed.
	token := rt.LowerASCII(rt.TrimASCII(in.Kind))
	out := routed{kind: token, canonical: in.Value, country: in.CountryCode, status: Valid, reason: ReasonOK}

	// 1. An identifier is a sequence of code points, and bytes that do not
	// form one have none to evaluate. Refusing here is also what keeps
	// canonicalization from growing: a step that filters by code point would
	// otherwise substitute U+FFFD for every malformed byte, tripling the value
	// and making two engines disagree on the canonical value they report.
	if !utf8.ValidString(in.Value) {
		out.status, out.reason = Unsupported, ReasonInvalidEncoding
		return out
	}

	// 2 and 3. Resolve the dispatcher through the kind and its aliases.
	d := dispatcherOf(token)
	if d == noDispatcher {
		out.status, out.reason = Unsupported, ReasonUnsupportedKind
		return out
	}
	out.kind = canonicalKind(d)

	// 4. Run the pre-canonicalizer exactly once on the raw value.
	preCanonicalize(d, w)

	stop := func(status Status, reason Reason) routed {
		out.canonical = canonicalOf(in.Value, w)
		out.status, out.reason = status, reason
		return out
	}

	// 5. Normalize an explicit country. An empty token behaves like an absent
	// context; a syntactically invalid one keeps the raw context in the result.
	var country string
	if raw := rt.TrimASCII(in.CountryCode); raw != "" {
		country = rt.UpperASCII(raw)
		if !isCountryToken(country) {
			return stop(Unsupported, ReasonUnsupportedCountry)
		}
		country = countryAlias(d, country)
		out.country = country
	} else {
		out.country = ""
	}

	byCountry, hasCountryTarget := countryTarget(d, country)
	global, hasGlobal := globalTarget(d)
	if country != "" && !hasCountryTarget && !hasGlobal {
		// A country specific dispatcher has no target for this country.
		return stop(Unsupported, ReasonUnsupportedCountry)
	}

	// 6. Select the target owning the longest exactly matching prefix.
	byPrefix, hasPrefixTarget := prefixTarget(d, w.Str())

	// 7. An explicit country and a recognized prefix pointing at two different
	// targets is a mismatch, the only dispatch failure that proves invalidity.
	if hasCountryTarget && hasPrefixTarget && byCountry != byPrefix {
		return stop(Invalid, ReasonCountryMismatch)
	}

	// 8. Country target, then prefix target, then GLOBAL, then the single
	// target selectable without a country.
	var def definition
	switch {
	case hasCountryTarget:
		def = byCountry
	case hasPrefixTarget:
		def = byPrefix
	case hasGlobal:
		def = global
	default:
		unprefixed, ok := unprefixedTarget(d)
		if !ok {
			// 9. Nothing is selectable.
			return stop(Unsupported, ReasonMissingCountryCode)
		}
		def = unprefixed
	}

	// 10. Run the canonicalizer of the selected definition exactly once on the
	// pre-canonical value. The canonicalizer belongs to the definition, so it
	// runs under the resolved profile, per section 5.2.
	effective := profile
	if in.Profile == "" {
		effective = definitionDefaultProfile(def)
	}
	canonicalize(def, w, ruleCtx{profile: effective})

	out.def, out.selected = def, true
	out.canonical = canonicalOf(in.Value, w)
	// A country target reports its ISO code; a GLOBAL target keeps the well
	// formed country context without having used it for routing.
	if c := definitionCountry(def); c != "" {
		out.country = c
	}
	return out
}

// canonicalOf reads the value out of the workspace, handing back the caller's
// own string when no step changed it. That is what makes a clean input cost no
// allocation at all.
func canonicalOf(original string, w *rt.Buf) string {
	if !w.Modified() {
		return original
	}
	return w.String()
}

// isCountryToken matches exactly [A-Z]{2}.
func isCountryToken(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}

// mode selects how far the validation pipeline runs.
type mode uint8

const (
	modeCanonicalize mode = iota
	modeValidateFormat
	modeValidate
)

// evaluate drives the pipeline of section 6 of ir.md.
//
// The workspace lives on this stack frame, which is what keeps canonicalization
// allocation free.
func evaluate(in Input, md mode) (routed, stepResult, stepResult) {
	// 1. An input above the byte limit is reported with the raw value and the
	// raw context, before any rule runs.
	if len(in.Value) > rt.MaxInput {
		profile := in.Profile
		if profile == "" {
			// No definition was selected, so no default can apply.
			profile = Compatible
		}
		r := routed{
			kind:      rt.LowerASCII(rt.TrimASCII(in.Kind)),
			canonical: in.Value,
			country:   in.CountryCode,
			profile:   profile,
			status:    Unsupported,
			reason:    ReasonInputTooLong,
		}
		return r, unsupportedStep(ReasonInputTooLong, ""), notRunAfter(Unsupported)
	}

	// The workspace is an array on this stack frame, sized by the generator
	// from the growth the compiled canonicalizers can add. That is what keeps
	// canonicalization allocation free.
	var scratch [canonicalizationScratch]byte
	w := rt.New(in.Value, scratch[:], canonicalizationLeftMargin)

	// 2. Route, running each canonicalization phase at most once.
	r := route(in, in.Profile, &w)

	// 3 and 4. A dispatch failure decides the format step.
	if !r.selected {
		format := stepResult{status: r.status, reason: r.reason}
		return r, format, notRunAfter(r.status)
	}
	if md == modeCanonicalize {
		return r, stepResult{}, stepResult{}
	}

	// 5. Run the format rule on the canonical value, under the profile
	// section 5.2 resolved.
	c := ruleCtx{value: r.canonical, profile: r.profile}
	format := formatRule(r.def, c)
	if format.status != Valid {
		// 6 and 7. A format that did not hold keeps the checksum from running.
		return r, format, notRunAfter(format.status)
	}

	if md == modeValidateFormat {
		// validateFormat stops here and never asks for a checksum.
		return r, format, stepResult{status: NotRun, reason: ReasonNotRequested}
	}

	// 8. A valid format without a checksum rule reports the declared absence
	// reason. 9. Otherwise the checksum runs.
	return r, format, checksumRule(r.def, c)
}

// notRunAfter maps a format step that did not hold onto the checksum step that
// follows it.
func notRunAfter(format Status) stepResult {
	reason := ReasonNotRunFormatUnsupported
	if format == Invalid {
		reason = ReasonNotRunFormatInvalid
	}
	return stepResult{status: NotRun, reason: reason}
}
