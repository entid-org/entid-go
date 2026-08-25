#!/usr/bin/env bash
# Copyright The EntID Authors.
# SPDX-License-Identifier: Apache-2.0
#
# One entry point for the whole verification, as section 12.5 of engine.md
# requires: lock digests, regeneration, build, tests, conformance against the
# runner from spec, lint, format, coverage and its thresholds, packaging.
#
# On success it prints one line carrying the numbers that matter. On failure it
# prints the output of the failing step, named, and nothing else, and exits non
# zero at once. Nothing is swallowed: a step that cannot run is a failure, never
# a skip, because a skipped check reads exactly like a passing one.
set -uo pipefail

# awk and the go tools format numbers by locale; the one line this prints is
# read by people and by CI logs, so it is pinned to one.
export LC_ALL=C

cd "$(dirname "$0")/.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# run executes one step, keeping its output aside. Only a failing step's output
# is ever printed, which is the whole point: a green run of thirty commands has
# nothing to say except its numbers.
step_log=""
run() {
	local name="$1"
	shift
	step_log="$work/$(printf '%s' "$name" | tr -c 'a-zA-Z0-9' '_').log"
	if ! "$@" >"$step_log" 2>&1; then
		printf '%s failed\n\n' "$name" >&2
		cat "$step_log" >&2
		exit 1
	fi
}

# fail reports a step that failed on its own terms rather than by exit status.
fail() {
	printf '%s failed\n\n' "$1" >&2
	shift
	printf '%s\n' "$@" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$2" "$1 is not on PATH, so the step cannot run"
}

need go "toolchain"
need git "toolchain"

# 1. The eight digests rules.lock declares, against the files under spec/.
lock_value() { sed -n "s/^$1[[:space:]]*=[[:space:]]*\"\([^\"]*\)\".*/\1/p" rules.lock | head -1; }

digest_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

declare -a digest_keys=(
	"rules_sha256:entid-rules.binpb"
	"conformance_sha256:entid-conformance.binpb"
	"conformance_jsonl_sha256:entid-conformance.jsonl"
	"rules_proto_sha256:rules.proto"
	"conformance_proto_sha256:conformance.proto"
	"testee_proto_sha256:testee.proto"
	"ir_doc_sha256:ir.md"
	"features_doc_sha256:features.md"
)
checked=0
for pair in "${digest_keys[@]}"; do
	key="${pair%%:*}"
	file="spec/${pair#*:}"
	want="$(lock_value "$key")"
	[ -n "$want" ] || fail "lock digests" "rules.lock declares no $key"
	[ -f "$file" ] || fail "lock digests" "$file is missing"
	got="$(digest_of "$file")"
	[ "$got" = "$want" ] || fail "lock digests" "$file hashes to $got, rules.lock declares $want"
	checked=$((checked + 1))
done
# Every *_sha256 the lock declares must be one of the eight, so a ninth added
# upstream fails here instead of going unverified.
declared="$(grep -cE '^[a-z_]+_sha256[[:space:]]*=' rules.lock || true)"
[ "$declared" = "$checked" ] || fail "lock digests" \
	"rules.lock declares $declared digests and this script verifies $checked"

rules_version="$(lock_value rules_version)"
source_commit="$(lock_value source_commit)"
[ -n "$source_commit" ] || fail "lock digests" "rules.lock declares no source_commit"

# 1b. The mirror says where it came from, and must say the same thing as the
# lock. A release pull request that updates one and not the other leaves the
# repository stating two different origins for one set of files, and a digest
# check cannot see it because PROVENANCE.md is prose.
grep -q -- "$source_commit" spec/PROVENANCE.md ||
	fail "provenance" \
		"rules.lock records source_commit $source_commit, which spec/PROVENANCE.md does not name" \
		"$(sed -n '3,5p' spec/PROVENANCE.md)"
grep -q -- "$rules_version" spec/PROVENANCE.md ||
	fail "provenance" \
		"rules.lock records rules_version $rules_version, which spec/PROVENANCE.md does not name" \
		"$(sed -n '3,5p' spec/PROVENANCE.md)"

# 2. The generated rules match the bundle they are generated from.
run "regeneration" go generate ./...
run "regeneration" git diff --exit-code -- rules_gen.go

# 3, 4, 5. Build, vet, format.
run "build" go build ./...
run "vet" go vet ./...
gofmt -l . >"$work/fmt.log" 2>&1
[ -s "$work/fmt.log" ] && fail "format" "these files are not gofmt clean:" "$(cat "$work/fmt.log")"

# 6. Lint. An absent linter is a failure: the alternative is a run that looks
# green because nothing looked.
need golangci-lint "lint"
run "lint" golangci-lint run ./...
lint_line="$(tail -1 "$step_log" | sed 's/\.$//')"

# 7. Tests, under the race detector.
run "tests" go test -race ./... -timeout 900s

# 7b. Fuzz smoke over the two trust boundaries: the bundle decoder and the
# public API. Short, because a long run belongs to a schedule, but present,
# because engine-go.md asks for it on every change and a gate outside this
# script is a second definition of green.
run "fuzz smoke" go test ./internal/gen -run FuzzLoad -fuzz FuzzLoad -fuzztime 20s
run "fuzz smoke" go test . -run FuzzValidate -fuzz FuzzValidate -fuzztime 20s

# 8. Conformance, against the runner from spec, pinned to the commit rules.lock
# records. GOTOOLCHAIN is auto for this step alone: the spec module asks for a
# newer Go than this one pins, and the testee stays built with the pinned one.
# The testee is instrumented so the conformance run also reports what the
# emitted rules cover. That figure describes the corpus, not this engine, which
# is why it is published below and never gated.
mkdir -p "$work/covdata"
run "testee" go build -cover -coverpkg=./... -o "$work/entid-testee" ./cmd/entid-testee
GOCOVERDIR="$work/covdata" GOTOOLCHAIN=auto run "conformance" \
	go run "github.com/libbusinessid/spec/cmd/conformance-runner@$source_commit" \
	-corpus spec/entid-conformance.binpb -- "$work/entid-testee"
grep -q '^conformant$' "$step_log" || fail "conformance" "$(cat "$step_log")"
conformance_line="$(grep -E '^rules .*cases' "$step_log" | head -1)"

# 9. Coverage of hand-written code, and its threshold. Section 12.2 of
# engine.md gates hand-written code only; code emitted from the bundle is
# measured and published, never gated, because its coverage describes the
# corpus rather than this engine.
run "coverage" go test ./... -coverprofile="$work/coverage.out" -covermode=atomic -coverpkg=./... -timeout 900s
covered="$(awk 'NR > 1 {
	if ($1 ~ /rules_gen\.go/) next
	if ($1 ~ /cmd\/entid-demo/) next
	stmt[$1] = $2
	if ($3 > 0) hit[$1] = 1
}
END {
	for (block in stmt) { total += stmt[block]; if (block in hit) covered += stmt[block] }
	if (total == 0) exit 1
	printf "%d %d %.2f", covered, total, 100 * covered / total
}' "$work/coverage.out")" || fail "coverage" "the profile carries no hand-written block"
set -- $covered
gate_ok="$(awk -v pct="$3" 'BEGIN { print (pct >= 95.0) ? "yes" : "no" }')"
[ "$gate_ok" = yes ] || fail "coverage" \
	"hand-written coverage is $3% of $2 statements, below the 95% gate of engine.md section 12.2"
coverage_line="$3% of $2"
run "coverage" go tool covdata textfmt -i="$work/covdata" -o="$work/conformance.out"
generated="$(awk 'NR > 1 && $1 ~ /rules_gen\.go/ {
	stmt[$1] = $2
	if ($3 > 0) hit[$1] = 1
}
END {
	for (block in stmt) { total += stmt[block]; if (block in hit) covered += stmt[block] }
	if (total > 0) printf "%.2f", 100 * covered / total
}' "$work/coverage.out" "$work/conformance.out")"

# 9b. The expansion profile. No conformance case can compare it, because no
# real rule comes near the budget; comparing the same bundle across engines is
# the only way, so the number is printed rather than merely asserted.
run "expansion profile" go test ./internal/gen -run TestExpansionProfile -v -count=1
expansion="$(sed -n 's/.*rules [^:]*: \(.*\)$/\1/p' "$step_log" | head -1)"

# 10. Packaging: the module zip a consumer downloads carries no rule or corpus
# data. go list -deps cannot see this, because a zip carries every file under
# the module root and not only the ones a package compiles.
mkdir -p "$work/zip"
cat >"$work/zip/main.go" <<'PROBE'
package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path"
	"strings"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func main() {
	out, err := os.Create(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot write the zip:", err)
		os.Exit(2)
	}
	v := module.Version{Path: os.Args[3], Version: "v0.0.0-00010101000000-000000000000"}
	if err := modzip.CreateFromDir(out, v, os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "cannot build the zip:", err)
		os.Exit(2)
	}
	out.Close()

	r, err := zip.OpenReader(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot open the zip:", err)
		os.Exit(2)
	}
	defer r.Close()
	var total uint64
	refused := 0
	for _, f := range r.File {
		total += f.UncompressedSize64
		name := f.Name
		ext := path.Ext(name)
		if strings.Contains(name, "/spec/") || ext == ".binpb" || ext == ".jsonl" {
			fmt.Fprintf(os.Stderr, "the published module carries %s\n", name)
			refused++
		}
	}
	if len(r.File) == 0 {
		fmt.Fprintln(os.Stderr, "the zip is empty, so nothing was checked")
		os.Exit(2)
	}
	if refused > 0 {
		os.Exit(1)
	}
	fmt.Printf("%d files, %d bytes\n", len(r.File), total)
}
PROBE
(
	cd "$work/zip" &&
		GOTOOLCHAIN=auto go mod init zipcheck >/dev/null 2>&1 &&
		GOTOOLCHAIN=auto go get golang.org/x/mod@latest >/dev/null 2>&1 &&
		GOTOOLCHAIN=auto go build -o zipcheck .
) >"$work/zipbuild.log" 2>&1 || fail "packaging" "$(cat "$work/zipbuild.log")"
run "packaging" "$work/zip/zipcheck" "$PWD" "$work/module.zip" github.com/entid-org/entid-go
zip_line="$(cat "$step_log")"

printf 'verify ok — rules %s, %s, coverage %s hand-written (generated %s%%, published not gated), lint %s, %s, module zip %s\n' \
	"$rules_version" \
	"$(printf '%s' "$conformance_line" | sed 's/^rules [^:]*: //')" \
	"$coverage_line" \
	"$generated" \
	"$lint_line" \
	"$expansion" \
	"$zip_line"
