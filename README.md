# entid-go

Offline canonicalization and validation of business identifiers — VAT numbers,
EUID, the national company number of every EU member state, SIREN, SIRET, LEI,
USCC, CNPJ, DUNS, EORI, EIN — from the rules published by the
[EntID spec repository](https://github.com/entid-org/spec).

No network. No registry lookup. No allocation on a clean value.

```sh
go get github.com/entid-org/entid-go
```

```go
package main

import (
	"fmt"

	entid "github.com/entid-org/entid-go"
)

func main() {
	engine := entid.New()

	report, err := engine.Validate(entid.Input{
		Kind:  "vat",
		Value: "BE 0123.456.749",
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(report.CanonicalValue)  // BE0123456749
	fmt.Println(report.CountryCode)     // BE
	fmt.Println(report.Format.Status)   // valid
	fmt.Println(report.Checksum.Status) // valid
}
```

Full API documentation: [pkg.go.dev](https://pkg.go.dev/github.com/entid-org/entid-go).

Every identifier shown in this README and in the runnable examples is a value the
shared conformance corpus carries — `BE0123456749` under `vat-be-valid-001`,
`012345674` under `siren-valid-001`. The corpus classifies them as **synthetic**:
they are built by the documented generator of the specification's data policy so
that they satisfy the published check digit, and they are not taken from any
register. No identifier here designates a company, and none was made up on the
spot.

## What it answers, and what it does not

Two questions, and no others: **does this value have the shape of a documented
variant**, and **does its internal check digit hold**.

A valid format means the shape matches a documented variant. A valid checksum
means the published internal check is satisfied. Neither is evidence that a
company exists, is active, or belongs to anyone — that needs a register, which
this library never contacts.

Nor does a valid result say *who* an identifier belongs to. A Spanish sole
trader invoices under their DNI, a Bulgarian one under their EGN: those forms
are accepted because refusing them would reject millions of real businesses,
and nothing in the format itself separates a sole trader from a private
individual. The caller knows that context; the format does not. `euid` is the
exception, and only because it is issued by a company register rather than by a
tax role.

## The one thing to get right

**`Unsupported` means "no verdict". It is not a rejection.**

Refusing a valid identifier is the most serious defect this project recognizes,
so a value is reported `Invalid` only when a documented, applicable rule proves
it. Everything else — an unpublished algorithm, an ambiguous variant, a country
this engine carries no rule for — is `Unsupported`.

German VAT numbers make the point concretely: no check digit algorithm is
published for them, so their checksum step is *always* `Unsupported`. Code that
rejects on anything other than `Valid` rejects every German VAT number there is.

```go
switch report.Checksum.Status {
case entid.Valid:
    // The check digit holds.
case entid.Invalid:
    // A documented rule proved this value wrong. Safe to reject.
default:
    // No verdict. Fall back to the format result, or to a register.
}
```

`report.OK()` is a narrow helper for "both steps concluded positively". Anything
more nuanced should read the two steps.

## Choosing an entry point

| Call | Runs | Use it for |
|---|---|---|
| `Validate` | format, then checksum | the usual case |
| `ValidateChecksum` | identical to `Validate` | when that name reads better at the call site |
| `ValidateFormat` | format only | a form validating as the user types |
| `Canonicalize` | no rule at all | what you store, or use as a database key |

## Reading a report

A `Report` carries two steps. Format guards checksum: a check digit is never
computed on a shape it was not designed for, so a format that did not hold
leaves the checksum `NotRun`.

```go
report, _ := engine.Validate(entid.Input{Kind: "siren", Value: "0123"})

report.Format.Status      // invalid
report.Format.Reason      // invalid_length
report.Format.MessageKey  // fr.siren.length
report.Checksum.Status    // not_run
report.Checksum.Reason    // not_run_format_invalid
```

`Reason` is a frozen registry every EntID engine agrees on — branch on
it, not on a message. `MessageKey` names an entry in *your* catalogue, for the
rules that declare one, so a user-facing message can be translated without
parsing anything.

## Country hints

`Input.CountryCode` disambiguates a value that carries no country prefix, and
contradicts one that carries a different country.

```go
// Without a prefix, the hint selects the definition.
engine.Validate(entid.Input{Kind: "vat", Value: "0123456749", CountryCode: "BE"})
// → BE0123456749, valid

// Without either, nothing can be selected.
engine.Validate(entid.Input{Kind: "vat", Value: "0123456749"})
// → unsupported, missing_country_code

// A hint that contradicts the prefix is the one dispatch failure that
// proves an invalidity.
engine.Validate(entid.Input{Kind: "vat", Value: "BE0123456749", CountryCode: "FR"})
// → invalid, country_mismatch
```

## Profiles

`Compatible` accepts current variants and the documented historical ones that
can still legitimately appear in data being processed today. `StrictCurrent` is
opt-in and accepts only variants that are currently issued. Neither changes the
canonical form.

```go
in := entid.Input{Kind: "vat", Value: "FRK7012345674"}

engine.Validate(in)             // valid: the computation key may be alphanumeric
in.Profile = entid.StrictCurrent
engine.Validate(in)             // invalid: strict_current wants a numeric key
```

Leaving `Profile` unset is not the same as passing `Compatible`: it lets the
definition the value routes to apply its own documented default.

## Coverage

94 definitions across 37 families and 37 countries. 60 of them carry a published
checksum algorithm.

| Family | Coverage | With a checksum |
|---|---|---|
| `vat` | 31 countries | 26 |
| `euid` | 27 member states | 15 |
| national company numbers | 27 member states | 15 |
| `siren`, `siret` | FR | both |
| `uscc`, `cnpj`, `corporate_number` | CN, BR, JP | all three |
| `lei` | global | yes |
| `duns`, `eori` | global | neither |
| `ein` | US | no |

Each `euid` applies the national rule of its member state to the part after the
dot, rather than restating it, so the two always agree and a report names the
national message key (`be.enterprise_number.length`, not an EUID-specific one).

`engine.Coverage()` returns all of this at run time — every definition with the
authority, source URL, access date and tier behind its rule; `engine.Kinds()`
lists what can be routed.

Thirty-four definitions report `Unsupported` for the checksum step on purpose,
and say why. Twenty-nine have no published algorithm at all. The other five have
one the V1 rule language cannot express: Austria adds a constant to a
Luhn-shaped sum, Cyprus runs a non-linear table and compares against a letter,
Croatia and Germany close on the iterative ISO 7064 MOD 11,10.
`AbsentChecksumReason` distinguishes the two cases.

A sole trader is covered where the register covers one: a Spanish *autónomo*
invoices under a DNI and a Czech OSVČ under a number built on their birth
number, and both are accepted. `euid` is the exception, since a company
register rather than a tax role issues it.

## Errors and bounds

`Validate` returns an error for exactly one thing: a `Profile` outside the
frozen set (`ErrUnknownProfile`). Every value, however malformed, is answered
with a verdict instead.

Two safety bounds are raised before any rule runs, both `Unsupported` rather
than verdicts, and both reporting the value **verbatim**:

- above 1024 UTF-8 bytes → `ReasonInputTooLong`
- not valid UTF-8 → `ReasonInvalidEncoding`

## Cost

An `Engine` holds no mutable state and is safe for concurrent use. `New()` is
free — the rules are Go code, so there is nothing to parse or build.

```
BenchmarkValidate/canonical/siren-24        253 ns/op     0 B/op   0 allocs/op
BenchmarkValidate/canonical/vat-be-24       355 ns/op     0 B/op   0 allocs/op
BenchmarkValidate/canonical/lei-24          508 ns/op     0 B/op   0 allocs/op
BenchmarkValidate/canonical/cnpj-24         555 ns/op     0 B/op   0 allocs/op
BenchmarkValidate/canonical/uscc-24         597 ns/op     0 B/op   0 allocs/op
BenchmarkValidate/dirty/vat-be-24           473 ns/op    16 B/op   1 allocs/op
BenchmarkValidate/reject/unknown-kind-24     72 ns/op     0 B/op   0 allocs/op
BenchmarkValidateParallel-24                 20 ns/op     0 B/op   0 allocs/op
BenchmarkNew-24                             2.1 ns/op     0 B/op   0 allocs/op
```

A value that is already canonical costs **no allocation**: canonicalization
writes into a stack workspace and hands your own string back when no step
changed it. One that needs rewriting costs a single allocation, for the
canonical form itself.

Run them yourself with `go test -bench . -benchmem`.

## How the rules get here

The engine does not interpret a rule bundle. The work splits in two:

```
spec/entid-rules.binpb             the attested artifact
        │
        ▼
cmd/entid-gen                      the generator: validates, then emits Go
        │
        ▼
rules_gen.go                       generated, committed, reviewable in a diff
        │
        ▼
internal/runtime + the public API  what ships
```

`rules.lock` is the only coupling point with the spec repository: it names a
release and attests its content. Keeping it current is not a maintainer's job.
`.github/workflows/sync.yml` runs on a daily clock and on demand: it compares
the latest release of `spec` with the lock, does nothing when they agree, and
otherwise verifies the artifacts — `SHA256SUMS`, then the provenance
attestation — rewrites `spec/`, regenerates, runs `make verify` and opens a pull
request with the lot, green or red. Nothing is written before the attestation
verifies, and nothing in `spec` holds a token for this repository.

Regenerating by hand is for a lock edited by hand:

```sh
go generate ./...
```

Generated code is committed on purpose. Building this module needs the Go
toolchain and nothing else — no network, no access to the spec repository.

A bundle that does not satisfy the IR contract stops the generator. An unknown
version, field, opcode or capability produces no code at all rather than partial
code, which is what the specification calls closed generation.

Nothing is built when the program starts: dispatch is a generated `switch`, and
every weight and remainder table is a package level array living in read-only
data.

## Conformance

Conformance is not self-declared. The spec repository publishes a runner that is
the only program reading expected results; this repository provides the testee
it drives, over the protocol of `spec/testee.proto`:

```sh
go build -o bin/entid-testee ./cmd/entid-testee

# the runner, pinned to the source_commit rules.lock records
go run github.com/entid-org/spec/cmd/conformance-runner@<source_commit> \
    -corpus spec/entid-conformance.binpb \
    -- bin/entid-testee
```

```
rules 2026.08.38: 676 cases, 676 matched, 0 differed
conformant
```

The exchange is strictly synchronous: one request, one response, then the next.
The testee never reads the corpus and never sees an expected result; it echoes
the case identifier so a desynchronized exchange is detected, and uses it for
nothing else.

The 38 `load_ruleset` cases address the generator rather than the engine, since
this engine loads no bundle at run time. The testee routes them to it, and
`go test ./internal/gen` covers them too.

The runner can also sweep an issuer's full register, asking one question of
every row: is this identifier refused. That needs a dump this repository does
not carry; `conformance/registers.json` in the spec repository names the three
sources and how to read them.

## Layout

| Path | Role |
|---|---|
| `entid.go`, `result.go` | the public API |
| `engine.go` | dispatch and the validation pipeline, independent of the rules |
| `rules_gen.go` | the compiled rules — generated, do not edit |
| `internal/runtime` | the support primitives the generated code calls |
| `internal/gen` | the generator: decoder, IR validation, Go emission |
| `cmd/entid-gen` | the generator command |
| `cmd/entid-testee` | the conformance protocol adapter |
| `cmd/entid-demo` | a one-shot command line check |
| `spec/` | artifacts copied from the spec repository; never edited here |

## License

Apache-2.0.
