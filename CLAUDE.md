# Working in this repository

## Verify with one command

```sh
make verify
```

That is the whole verification: the eight digests `rules.lock` declares against
the files under `spec/`, regeneration, build, vet, format, lint, tests under the
race detector, fuzz smoke, conformance against the runner from `spec`, coverage
and its threshold, the expansion profile, and the module zip a consumer would
download.

**Do not run the steps individually to check whether the repository is sound.**
A resync round is about thirty commands, and twenty-nine of their outputs say
nothing except "this passes". Reading them all is what makes a round expensive,
and expensive is what stops it happening often enough.

- It succeeds with **one line**, carrying the numbers that matter.
- It fails with **the output of the failing step, named, and nothing else**, and
  exits non-zero at once.
- Nothing is skipped. A tool that is missing is a failure, never a pass, because
  a check that did not run reads exactly like a check that passed.

This is also what CI runs, so green has one definition. If you add a gate, add
it to `scripts/verify.sh`; a gate that lives only in the workflow is a second
definition of green and will drift.

Run the individual commands when you are *debugging* a failure `make verify`
already reported. That is what they are for.

## Rules the specification places on this engine

- **Generator, not interpreter.** The rules are compiled into `rules_gen.go` at
  build time. No public API takes a bundle in bytes, nothing embeds one, and the
  published module carries no `.binpb`.
- **The conformance runner comes from `spec`**, pinned to the `source_commit`
  of `rules.lock`. Never write one here: an engine that grades itself can
  compare too weakly and call the result conformance.
- **Never invent a business identifier.** Values come from the shared corpus or
  an issuer's register, with their provenance written beside them, README
  included.
- **A bug is proved by a test that fails first.** If you did not watch it fail,
  you did not prove anything — and check that the probe actually compiled before
  believing what it reports.
