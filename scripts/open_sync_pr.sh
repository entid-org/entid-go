#!/usr/bin/env bash
# Copyright The LibBusinessID Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Step 6 of engine.md section 11.4: commit what scripts/sync_release.sh wrote,
# push it, and open the pull request — green or red. A red one is opened too,
# because a release the engine cannot yet follow is exactly what a human has to
# see; it is never merged to unblock the chain.
#
# Everything here runs on GITHUB_TOKEN. Nothing in this repository holds a token
# for another one, and nothing in another repository holds one for this: that is
# the second reason section 11.4 gives for the engine fetching rather than the
# release pushing.
#
# Usage: open_sync_pr.sh <version> <tag> <branch> <verify-outcome> <writer-ref>
set -euo pipefail

version="$1"
tag="$2"
branch="$3"
verify="$4"
writer_ref="$5"

repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}"
run_url="${GITHUB_SERVER_URL:-https://github.com}/${repo}/actions/runs/${GITHUB_RUN_ID:-0}"

git config user.name "libbusinessid-bot"
git config user.email "bot@libbusinessid.invalid"

# -C rather than -c: a re-run after a fixed defect lands on the branch it made
# last time instead of refusing to create it.
git switch -C "${branch}"
git add spec rules.lock rules_gen.go
if git diff --cached --quiet; then
	echo "the release is already what this branch carries; no pull request to open"
	exit 0
fi
git commit --quiet -m "rules: update to ${version}"
head="$(git rev-parse HEAD)"

# A re-run has to replace its own branch rather than be refused as non fast
# forward, and --force-with-lease is the safe way. A lease needs something to
# compare against, though: with nothing fetched git refuses with "stale info",
# so the branch is fetched into a tracking ref first and the expected value is
# spelled out. Both were found the hard way on the release side.
if git ls-remote --exit-code --heads origin "${branch}" >/dev/null 2>&1; then
	git fetch --quiet origin "+${branch}:refs/remotes/origin/${branch}"
	git push --quiet \
		"--force-with-lease=${branch}:$(git rev-parse "refs/remotes/origin/${branch}")" \
		--set-upstream origin "${branch}"
else
	git push --quiet --set-upstream origin "${branch}"
fi

# The §12.5 entry point already ran, on this exact commit, in the step before
# this one. It is reported here as a commit status because GitHub does not start
# a workflow for a pull request opened with GITHUB_TOKEN — "events triggered by
# the GITHUB_TOKEN will not create a new workflow run" — so ci.yml never fires
# on this branch and the pull request would otherwise carry no check at all.
#
# This is not a second definition of green. It is the same `make verify`, on the
# same commit, under the same name the CI job reports, so one required check
# covers a human's pull request and this one alike.
state=failure
description="make verify failed; read the run"
if [ "${verify}" = "success" ]; then
	state=success
	description="make verify passed"
fi
gh api --method POST "repos/${repo}/statuses/${head}" \
	-f "state=${state}" \
	-f "context=verify" \
	-f "description=${description}" \
	-f "target_url=${run_url}" >/dev/null

body="$(
	cat <<BODY
Automated synchronization of the LibBusinessID rules, per section 11.4 of \`engine.md\`.

- rules version: \`${version}\`
- source tag: \`${tag}\`
- artifacts verified: \`SHA256SUMS\`, then the GitHub provenance attestation — owner, repository, signing workflow and tag
- emitted code: regenerated from the new bundle by this run
- \`make verify\`: **${verify}** — [run](${run_url})

The verification above is the section 12.5 entry point, run on this commit and
reported as the \`verify\` status. A red pull request is not merged to unblock
the chain: it is fixed, or the release is refused with the reason written down.
BODY
)"
if [ "${writer_ref}" != "refs/tags/${tag}" ]; then
	body="${body}

Note: \`refs/tags/${tag}\` carries no \`tools/write_provenance.sh\`, so
\`spec/PROVENANCE.md\` was assembled with the writer from \`${writer_ref}\`. Its
inputs — the bundle, the templates and \`cmd/businessidc\` — still come from the
tagged commit."
fi

number="$(gh pr list --repo "${repo}" --head "${branch}" --state open --json number --jq '.[0].number')"
if [ -n "${number}" ]; then
	gh pr edit "${number}" --repo "${repo}" --title "rules: update to ${version}" --body "${body}" >/dev/null
	echo "updated pull request #${number}"
else
	# --head and --base explicitly: gh infers them from the checkout, and has
	# refused with "you must first push the current branch to a remote" on a
	# branch it had just pushed.
	if ! gh pr create --repo "${repo}" --head "${branch}" --base main \
		--title "rules: update to ${version}" --body "${body}"; then
		cat >&2 <<'MISSING'
::error::the pull request could not be created. GITHUB_TOKEN cannot grant this
by itself, and neither can this repository: the repository-level API refuses
with "The organization does not allow GitHub Actions to create or approve pull
requests". An owner of the organization has to set, once:
  Organization > Settings > Actions > General > Workflow permissions >
    "Allow GitHub Actions to create and approve pull requests"
Reaching for a token from another repository instead would give back exactly the
blast radius section 11.4 removes. The branch is pushed and carries its verify
status, so a human can open the pull request from it in the meantime.
MISSING
		exit 1
	fi
	number="$(gh pr list --repo "${repo}" --head "${branch}" --state open --json number --jq '.[0].number')"
fi

# Auto-merge, so a mechanical release needs nobody and only what stayed red
# needs attention. It is safe only because a branch protection requires the
# `verify` status above: auto-merge on an unprotected branch merges as soon as
# nothing blocks, which is not the same thing as on green.
if ! gh pr merge "${number}" --repo "${repo}" --auto --squash; then
	cat >&2 <<'MISSING'
::error::auto-merge could not be enabled on the pull request. GITHUB_TOKEN
cannot grant this by itself; a repository administrator has to set, once, and
both of them:
  Settings > General > Pull Requests > "Allow auto-merge"
  Settings > Branches > protect `main` and require the status check `verify`,
    and only that one
The second is not optional. Auto-merge on an unprotected branch merges as soon
as nothing blocks, which is not the same thing as on green, so the first setting
alone is worse than neither.
MISSING
	exit 1
fi
echo "auto-merge enabled on #${number}"
