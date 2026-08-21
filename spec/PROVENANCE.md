# Where these files come from, and what to build

Copied from `github.com/libbusinessid/spec` at commit
`3295dc0d26914a3b2e778a5620ab0e5cf1f7e8c9`, rules version
`2026.08.14`, stability `alpha`.

## What this engine is — and is not

**This engine does not interpret the bundle at runtime.** It is not a virtual
machine. The specification (sections 2.2 and 4) splits the work in two:

- a **generator**, which you write, runs when the engine is built. It reads
  `spec/businessid-rules.binpb`, validates it, and emits source code in this
  language.
- the **engine**, which is what ships: the generated code, a small set of
  support primitives it calls, and a public API written by hand.

The generator is not in the spec repository and never will be — the
specification stays agnostic and hosts no target language. Write it in whatever
language suits you, as long as it can read the bundle.

The reasoning, if you want it: all business logic of the 94 current
definitions is 2375 IR nodes using 52 of the 63 opcodes. An interpreter costs
roughly three thousand lines of execution machinery per language to run that,
and buys the ability to swap rules without recompiling — which this project
does not use, since section 3 excludes dynamic rule downloading and section 11
requires a republication of the engine for every rules version.

## What is here

| Path | Role |
|---|---|
| `spec/spec.md` | The normative specification. It governs. |
| `spec/ir.md` | The 61 opcodes, their operands and their semantics. |
| `spec/features.md` | The 18 frozen capability IDs. |
| `spec/rules.proto` | Schema of the rules bundle — the generator's input. |
| `spec/conformance.proto` | Schema of the conformance corpus. |
| `spec/businessid-rules.binpb` | The bundle to generate from. |
| `spec/businessid-conformance.binpb` | The conformance corpus, authoritative form. |
| `spec/businessid-conformance.jsonl` | Same corpus, readable, for development. |
| `rules.lock` | Declared digests of every file above. |

The bundle lives here only until the first release is tagged. After that, the
downstream pull request updates `rules.lock` alone, and the generator resolves
the bundle from the release it names, verifying digests and attestation before
generating. Generated code is committed; binary artifacts are not.

## The rule that makes this exercise meaningful

**Write the generator from `spec.md`, `ir.md` and `rules.proto`. Do not port,
consult or transcribe the reference interpreter that lives in the spec
repository under `internal/reference/`.**

That interpreter exists to validate the specification, not to be copied. An
engine derived from it would pass conformance by construction and prove
nothing — neither that the IR is implementable, nor that `ir.md` suffices to
implement it. Proving exactly that is the purpose of each new engine.

If an opcode turns out to be under-specified in `ir.md`, that is a defect in the
specification: report it upstream so it gets fixed there. Do not work around it
by observing how the reference behaves.

## What the contract actually requires

Section 2.4 binds outputs, never method. For a given bundle, input and context,
the observable result is:

- the canonical value;
- the `format` step: status and reason code;
- the `checksum` step: status and reason code;
- the country code and the kind.

Nothing else. There is **no normative execution trace**, and no conformance case
tests one — so the generator is free to fold branches, inline everything and
allocate nothing.

## Design targets

The specification bounds user input to **1 024 bytes of UTF-8**, call depth to
32, and nodes per program to 4 096. Everything is statically bounded, so:

- canonicalization should write into a fixed-size buffer, never allocate;
- when canonicalization changes nothing — the common case on clean input —
  return the input as-is rather than a copy;
- the step budget of 100 000 does not apply to generated code, which terminates
  by construction. It bounds an interpretation.
- dispatch should be a generated switch, never a hash map built at start-up.

The rule of thumb: nothing should be constructed when the program starts. Tables
of weights, character mappings and remainder maps belong in read-only static
data; control flow belongs in code.

## Conformance

Do not write your own conformance suite. Section 8.7 defines a **runner** that
lives in the spec repository and is the only program that reads expected
results. You provide a **testee**: a small executable that reads requests on
stdin, calls your public API, and writes responses on stdout, framed as a
32-bit little-endian length followed by a serialized protobuf message.

The testee must never read the corpus, interpret an expected result, or adapt
its behaviour to the case it receives. Keeping it small is what makes the
absence of cheating verifiable.

The `load_ruleset` cases address your **generator**, not the engine: a truncated
bundle or one carrying a call cycle must make generation fail. The remaining
cases address the engine through the protocol above.

## Verifying integrity

Every digest in `rules.lock` is a SHA-256 of the corresponding file. Verify them
before starting. `rules.lock` carries no `attestation_identity` because no
release exists yet; its header explains this.


## Language-specific traps

- A package-level `var t = map[string]X{...}` or `[]byte{...}` is **initialized
  at program start-up** and allocates. A `const t = "\x01\x02..."` indexed as
  `t[i]` lives in `.rodata` and costs nothing. Prefer a generated `switch` over
  a map for dispatch.
- `len(s)` on a `string` already counts UTF-8 bytes, which is what the 1 024-byte
  input bound means. This is the one language where the natural expression is the
  correct one — do not "fix" it into a rune count.
- Returning `string(buf[:n])` allocates. Return the original input unchanged when
  canonicalization is a no-op.
