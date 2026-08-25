#!/usr/bin/env bash
# Copyright The EntID Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Fetch a release of entid-org/spec and bring this engine up to it.
#
# Section 11.4 of engine.md: the engine goes and gets the release, the release
# does not come and push into the engine. This script is steps 1 to 4 of that
# section — download, verify, write, regenerate. Step 5 is `make verify` and
# step 6 is scripts/open_sync_pr.sh; the workflow runs those, because a red
# verification still has to open its pull request.
#
# Usage: sync_release.sh [tag]     (default: the most recent release)
#
# It prints what it did, and when $GITHUB_OUTPUT is set it also writes:
#   changed, tag, version, commit, branch, writer_ref
#
# The order below is the one property that matters: nothing under the working
# tree is written before the provenance attestation has verified. Everything
# up to that point happens in a temporary directory outside the repository, so
# a release whose attestation does not verify cannot leave a trace here.
set -euo pipefail

spec_repo="entid-org/spec"
# The signer identity is written here rather than read from rules.lock: the lock
# is the file being replaced, so trusting its own claim about who may sign the
# replacement would close the loop on nothing.
signer_workflow="${spec_repo}/.github/workflows/release.yml"
engine="entid-go"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${root}"

fail() {
	printf 'sync failed: %s\n' "$1" >&2
	exit 1
}

need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is not on PATH"; }
need gh
need git
need go
need jq
need gzip

# GNU coreutils on a runner, shasum on a developer's macOS. Both read the same
# SHA256SUMS format.
check_sums() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum --check --strict "$1"
	else
		shasum -a 256 --check --strict "$1"
	fi
}

emit() { [ -n "${GITHUB_OUTPUT:-}" ] && printf '%s=%s\n' "$1" "$2" >>"${GITHUB_OUTPUT}"; return 0; }

lock_value() { sed -n "s/^$1[[:space:]]*=[[:space:]]*\"\{0,1\}\([^\"]*\)\"\{0,1\}$/\1/p" rules.lock | head -1; }

# 0. Which release, and is it the one already recorded?
tag="${1:-}"
if [ -z "${tag}" ]; then
	# Not `gh release view`: that resolves /releases/latest, which skips
	# pre-releases, and every release of spec is a pre-release while the rules
	# are alpha. It answered "release not found" for both of them.
	tag="$(gh release list --repo "${spec_repo}" --exclude-drafts --limit 1 --json tagName --jq '.[0].tagName')"
fi
[ -n "${tag}" ] || fail "${spec_repo} publishes no release"

# The lock records which attested release it trusts, tag included, so the tag is
# what is compared. Comparing rules_version alone would miss a re-release that
# ships the same rules from a different tag, and that is exactly the case where
# the recorded identity has to move.
identity="$(lock_value attestation_identity)"
current_tag="${identity##*refs/tags/}"
[ "${identity}" = "${current_tag}" ] && current_tag=""

if [ "${tag}" = "${current_tag}" ]; then
	printf 'rules.lock already records %s; nothing to do\n' "${tag}"
	emit changed false
	emit tag "${tag}"
	exit 0
fi
printf 'rules.lock records %s, the latest release is %s\n' "${current_tag:-no attested release}" "${tag}"

work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT
art="${work}/artifacts"
mkdir -p "${art}"

# 1. Download the release artifacts, outside the working tree.
gh release download "${tag}" --repo "${spec_repo}" --dir "${art}"

# 2a. The digests the release publishes for itself.
(cd "${art}" && check_sums SHA256SUMS)

version="$(sed -n 's/^[0-9a-f]*  entid-rules-\(.*\)\.binpb$/\1/p' "${art}/SHA256SUMS")"
[ -n "${version}" ] || fail "SHA256SUMS names no entid-rules-<version>.binpb"
manifest="${art}/entid-manifest-${version}.json"
[ -f "${manifest}" ] || fail "the release carries no manifest for ${version}"

# 2b. And then the attestation, which is what makes the digests worth anything:
# SHA256SUMS is a file in the same release, so on its own it vouches for nobody.
# The three artifacts below are the attested ones; every other file that lands
# under spec/ has its digest in the manifest, so `make verify` re-checks it
# against what was written.
#
# --repo, never --owner as well: gh refuses the two together, and the release
# side found that out by having the step fail before it verified anything.
for artifact in \
	"entid-rules-${version}.binpb" \
	"entid-conformance-${version}.binpb" \
	"entid-manifest-${version}.json"; do
	gh attestation verify "${art}/${artifact}" \
		--repo "${spec_repo}" \
		--signer-workflow "${signer_workflow}" \
		--source-ref "refs/tags/${tag}" ||
		fail "the attestation of ${artifact} does not verify"
done
printf 'attestation ok: %s at refs/tags/%s\n' "${signer_workflow}" "${tag}"

# 3a. The lock, assembled from the attested manifest rather than from the files.
#
# Every field is required to be there first. `jq -r` answers "null" for a field
# a manifest does not carry, and that string goes into the lock looking exactly
# like a digest, where nothing downstream reads it as missing: the first run of
# this script against v0.1.0 wrote conformance_jsonl_sha256 = "null" and said
# nothing. spec#81 added that field to the manifest, after v0.1.0 was tagged, so
# the case is real and not hypothetical.
for field in rulesVersion formatVersion artifactSha256 conformanceSha256 \
	conformanceJsonlSha256 rulesProtoSha256 conformanceProtoSha256 \
	testeeProtoSha256 irDocSha256 featuresDocSha256 stability sourceCommit; do
	jq -e --arg f "${field}" \
		'has($f) and (.[$f] != null) and (.[$f] | tostring | length > 0)' \
		"${manifest}" >/dev/null ||
		fail "the manifest of ${tag} carries no ${field}, so no lock can be written from it"
done

# The version comes from the artifact names, the lock from the manifest. They
# have to be the same release, or spec/ and rules.lock would describe two.
manifest_version="$(jq -r .rulesVersion "${manifest}")"
[ "${manifest_version}" = "${version}" ] ||
	fail "the release names ${version} in its files and ${manifest_version} in its manifest"

source_commit="$(jq -r .sourceCommit "${manifest}")"
{
	printf 'rules_version = "%s"\n' "$(jq -r .rulesVersion "${manifest}")"
	printf 'format_version = %s\n' "$(jq -r .formatVersion "${manifest}")"
	printf 'rules_sha256 = "%s"\n' "$(jq -r .artifactSha256 "${manifest}")"
	printf 'conformance_sha256 = "%s"\n' "$(jq -r .conformanceSha256 "${manifest}")"
	printf 'conformance_jsonl_sha256 = "%s"\n' "$(jq -r .conformanceJsonlSha256 "${manifest}")"
	printf 'rules_proto_sha256 = "%s"\n' "$(jq -r .rulesProtoSha256 "${manifest}")"
	printf 'conformance_proto_sha256 = "%s"\n' "$(jq -r .conformanceProtoSha256 "${manifest}")"
	printf 'testee_proto_sha256 = "%s"\n' "$(jq -r .testeeProtoSha256 "${manifest}")"
	printf 'ir_doc_sha256 = "%s"\n' "$(jq -r .irDocSha256 "${manifest}")"
	printf 'features_doc_sha256 = "%s"\n' "$(jq -r .featuresDocSha256 "${manifest}")"
	printf 'stability = "%s"\n' "$(jq -r .stability "${manifest}")"
	printf 'source_commit = "%s"\n' "${source_commit}"
	printf 'attestation_identity = "%s@refs/tags/%s"\n' "${signer_workflow}" "${tag}"
} >"${work}/rules.lock"

# 3b. spec/PROVENANCE.md has one writer and it lives in the spec repository. The
# clone is pinned to the tag the attestation just bound, and the tag has to
# resolve to the commit the manifest names — two records of one origin, checked
# against each other before either is written down.
git -c advice.detachedHead=false clone --quiet --depth 1 --branch "${tag}" \
	"https://github.com/${spec_repo}.git" "${work}/spec-src"
head_commit="$(git -C "${work}/spec-src" rev-parse HEAD)"
[ "${head_commit}" = "${source_commit}" ] ||
	fail "refs/tags/${tag} is ${head_commit}, the manifest names ${source_commit}"

writer="${work}/spec-src/tools/write_provenance.sh"
writer_ref="refs/tags/${tag}"
if [ ! -f "${writer}" ]; then
	# Releases cut before spec#84 have no provenance writer: it was extracted
	# from tools/sync_engines.sh after v0.1.1 was tagged. Only the assembler is
	# taken from the default branch; every input it reads — the bundle, the
	# templates under docs/spec/provenance, docs/generated/coverage.md and
	# cmd/entidc — still comes from the pinned clone. Measured at the time
	# this was written: those inputs are byte-identical between b264614 (v0.1.1)
	# and the default branch, so the fallback changes the writer's presence and
	# nothing else. It retires itself with the next release.
	git -C "${work}/spec-src" fetch --quiet --depth 1 origin HEAD
	git -C "${work}/spec-src" checkout --quiet FETCH_HEAD -- tools/write_provenance.sh
	writer_ref="$(git -C "${work}/spec-src" rev-parse FETCH_HEAD)"
	printf 'note: refs/tags/%s carries no tools/write_provenance.sh; using %s\n' "${tag}" "${writer_ref}"
fi

# --- Past this line the working tree is written. Nothing above it wrote here.

# 3c. spec/, the lock, the provenance note.
mkdir -p spec
cp "${art}/entid-rules-${version}.binpb" spec/entid-rules.binpb
cp "${art}/entid-conformance-${version}.binpb" spec/entid-conformance.binpb
# The corpus ships compressed and lands decompressed, because the archive embeds
# SOURCE_DATE_EPOCH and its digest would move with the source commit while its
# content did not. conformance_jsonl_sha256 is taken on what lands here.
gzip -dc "${art}/entid-conformance-${version}.jsonl.gz" >spec/entid-conformance.jsonl
for schema in rules.proto conformance.proto testee.proto ir.md features.md; do
	cp "${art}/${schema}" "spec/${schema}"
done
cp "${work}/rules.lock" rules.lock

# GOTOOLCHAIN is auto for this one call: the writer runs `go run` inside the
# spec module, which asks for a newer Go than this engine pins, and setup-go
# leaves GOTOOLCHAIN at local. What this engine builds stays on its own pin.
GOTOOLCHAIN=auto "${writer}" \
	"${work}/spec-src" "${art}" "${version}" "${source_commit}" "${engine}" "${root}/spec/PROVENANCE.md"

# 4. The emitted code, which is the half a release cannot push. This is why the
# engine fetches rather than the release delivering: spec has no Go toolchain to
# run this with, and four pull requests carrying a new bundle beside code
# generated from the old one were red for exactly that reason.
go generate ./...

printf 'synchronized %s to %s (%s), source commit %s\n' "${engine}" "${version}" "${tag}" "${source_commit}"
emit changed true
emit tag "${tag}"
emit version "${version}"
emit commit "${source_commit}"
emit branch "rules/${version}"
emit writer_ref "${writer_ref}"
