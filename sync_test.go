// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Section 11.4 of engine.md gives the synchronization one ordering rule:
// "Rien n'est écrit avant que l'étape 2 ne passe. Une release dont l'attestation
// ne vérifie pas ne doit pas même toucher l'arbre de travail."
//
// These tests run scripts/sync_release.sh against a sandbox repository with a
// stubbed gh on PATH, so the failures a real release would only produce by being
// tampered with can be produced on demand. They assert the tree, byte for byte,
// rather than the script's own account of what it did.

const syncLockTag = "v9.9.9"

// syncStub is a gh that answers from the environment and records what it was
// asked for, so a test can prove which step a run reached.
const syncStub = `#!/usr/bin/env bash
printf '%s\n' "$*" >>"${STUB_LOG}"
case "$1 $2" in
"release list")
  printf '%s\n' "${STUB_TAG}"
  ;;
"release download")
  dir=""
  while [ $# -gt 0 ]; do
    if [ "$1" = "--dir" ]; then dir="$2"; fi
    shift
  done
  cp "${STUB_ARTIFACTS}"/* "${dir}/"
  ;;
"attestation verify")
  if [ "${STUB_ATTESTATION}" != "ok" ]; then
    echo "Error: verifying with issuer \"sigstore.dev\"" >&2
    exit 1
  fi
  ;;
*)
  echo "unexpected gh invocation: $*" >&2
  exit 64
  ;;
esac
`

// syncSandbox builds a repository the script can be pointed at: the script
// itself, a lock naming syncLockTag, and a spec/ mirror of sentinel files.
func syncSandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatalf("cannot create %s: %v", p, err)
		}
	}
	write := func(p, content string, mode fs.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, p), []byte(content), mode); err != nil {
			t.Fatalf("cannot write %s: %v", p, err)
		}
	}

	mkdir("scripts")
	script, err := os.ReadFile(filepath.Join("scripts", "sync_release.sh"))
	if err != nil {
		t.Fatalf("cannot read the synchronization script: %v", err)
	}
	write(filepath.Join("scripts", "sync_release.sh"), string(script), 0o755)

	write("rules.lock", strings.Join([]string{
		`rules_version = "2999.01.0"`,
		`format_version = 1`,
		`source_commit = "0000000000000000000000000000000000000000"`,
		`attestation_identity = "libbusinessid/spec/.github/workflows/release.yml@refs/tags/` + syncLockTag + `"`,
		"",
	}, "\n"), 0o644)

	mkdir("spec")
	write(filepath.Join("spec", "businessid-rules.binpb"), "the bundle already here", 0o644)
	write(filepath.Join("spec", "PROVENANCE.md"), "the provenance already here", 0o644)
	return root
}

// syncArtifacts fabricates a release directory. sums decides whether the
// digests it publishes describe the files beside them.
func syncArtifacts(t *testing.T, honestSums bool) string {
	t.Helper()
	dir := t.TempDir()
	const version = "2999.02.0"
	// The manifest carries every field a lock needs except conformanceJsonlSha256,
	// which is the shape the real one had before spec#81 — release v0.1.0 still
	// publishes it that way.
	manifest := `{` +
		`"rulesVersion":"` + version + `",` +
		`"formatVersion":1,` +
		`"artifactSha256":"` + strings.Repeat("a", 64) + `",` +
		`"conformanceSha256":"` + strings.Repeat("b", 64) + `",` +
		`"rulesProtoSha256":"` + strings.Repeat("c", 64) + `",` +
		`"conformanceProtoSha256":"` + strings.Repeat("d", 64) + `",` +
		`"testeeProtoSha256":"` + strings.Repeat("e", 64) + `",` +
		`"irDocSha256":"` + strings.Repeat("f", 64) + `",` +
		`"featuresDocSha256":"` + strings.Repeat("0", 64) + `",` +
		`"stability":"alpha",` +
		`"sourceCommit":"` + strings.Repeat("1", 40) + `"}`
	files := map[string]string{
		"businessid-rules-" + version + ".binpb":       "fabricated bundle",
		"businessid-conformance-" + version + ".binpb": "fabricated corpus",
		"businessid-manifest-" + version + ".json":     manifest,
	}
	var sums strings.Builder
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("cannot write %s: %v", name, err)
		}
		digest := sha256.Sum256([]byte(content))
		if !honestSums {
			digest = sha256.Sum256([]byte("something else entirely"))
		}
		sums.WriteString(hex.EncodeToString(digest[:]) + "  " + name + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums.String()), 0o644); err != nil {
		t.Fatalf("cannot write SHA256SUMS: %v", err)
	}
	return dir
}

// syncSnapshot digests every file under root except the stub's own leavings, so
// a test can assert that a run left the tree exactly as it found it.
func syncSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		out[rel] = hex.EncodeToString(digest[:])
		return nil
	})
	if err != nil {
		t.Fatalf("cannot walk %s: %v", root, err)
	}
	return out
}

type syncRun struct {
	root   string
	stdout string
	stderr string
	log    string
	err    error
	before map[string]string
	after  map[string]string
}

func runSync(t *testing.T, attestation string, honestSums bool) syncRun {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the synchronization is a POSIX shell script")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not available: %v", err)
	}

	root := syncSandbox(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(syncStub), 0o755); err != nil {
		t.Fatalf("cannot write the gh stub: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "gh.log")

	run := syncRun{root: root, before: syncSnapshot(t, root)}
	cmd := exec.Command(filepath.Join(root, "scripts", "sync_release.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_LOG="+logPath,
		"STUB_TAG=v9.9.10",
		"STUB_ATTESTATION="+attestation,
		"STUB_ARTIFACTS="+syncArtifacts(t, honestSums),
		"GITHUB_OUTPUT=",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	run.err = cmd.Run()
	run.stdout, run.stderr = stdout.String(), stderr.String()
	if recorded, err := os.ReadFile(logPath); err == nil {
		run.log = string(recorded)
	}
	run.after = syncSnapshot(t, root)
	return run
}

func (r syncRun) assertTreeUntouched(t *testing.T) {
	t.Helper()
	for name, digest := range r.before {
		got, ok := r.after[name]
		if !ok {
			t.Errorf("%s disappeared from the working tree", name)
			continue
		}
		if got != digest {
			t.Errorf("%s was rewritten: %s became %s", name, digest, got)
		}
	}
	for name := range r.after {
		if _, ok := r.before[name]; !ok {
			t.Errorf("%s was written into the working tree", name)
		}
	}
}

func TestSyncDoesNothingWhenTheLockAlreadyRecordsTheRelease(t *testing.T) {
	root := syncSandbox(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(syncStub), 0o755); err != nil {
		t.Fatalf("cannot write the gh stub: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "gh.log")
	output := filepath.Join(t.TempDir(), "outputs")

	before := syncSnapshot(t, root)
	cmd := exec.Command(filepath.Join(root, "scripts", "sync_release.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_LOG="+logPath,
		"STUB_TAG="+syncLockTag,
		"STUB_ATTESTATION=ok",
		"STUB_ARTIFACTS="+syncArtifacts(t, true),
		"GITHUB_OUTPUT="+output,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("a synchronization that has nothing to do must succeed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "nothing to do") {
		t.Errorf("the run does not say it had nothing to do:\n%s", out)
	}

	recorded, _ := os.ReadFile(logPath)
	if strings.Contains(string(recorded), "release download") {
		t.Errorf("a run with nothing to do downloaded the release anyway:\n%s", recorded)
	}

	outputs, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("cannot read the step outputs: %v", err)
	}
	if !strings.Contains(string(outputs), "changed=false") {
		t.Errorf("the step reports %q, which does not say the tree is unchanged", outputs)
	}

	after := syncSnapshot(t, root)
	for name, digest := range before {
		if after[name] != digest {
			t.Errorf("%s changed although there was nothing to do", name)
		}
	}
}

func TestSyncWritesNothingWhenTheAttestationDoesNotVerify(t *testing.T) {
	run := runSync(t, "broken", true)
	if run.err == nil {
		t.Fatalf("a release whose attestation does not verify must fail the run:\n%s%s", run.stdout, run.stderr)
	}
	if !strings.Contains(run.log, "attestation verify") {
		t.Fatalf("the run never reached the attestation, so this proves nothing:\n%s", run.log)
	}
	if !strings.Contains(run.stderr, "attestation") {
		t.Errorf("the failure does not name the attestation:\n%s", run.stderr)
	}
	run.assertTreeUntouched(t)
}

// The stub's manifest carries rulesVersion and nothing else, which is the shape
// of the real one before spec#81: `jq -r` answers "null" for a field that is not
// there, and "null" written into rules.lock is indistinguishable from a digest.
// The first run of this script against release v0.1.0 wrote exactly that.
func TestSyncWritesNothingWhenTheManifestIsIncomplete(t *testing.T) {
	run := runSync(t, "ok", true)
	if run.err == nil {
		t.Fatalf("a manifest missing a digest must fail the run:\n%s%s", run.stdout, run.stderr)
	}
	if !strings.Contains(run.stderr, "conformanceJsonlSha256") {
		t.Errorf("the failure does not name the missing field:\n%s", run.stderr)
	}
	run.assertTreeUntouched(t)
}

func TestSyncWritesNothingWhenTheDigestsDisagree(t *testing.T) {
	run := runSync(t, "ok", false)
	if run.err == nil {
		t.Fatalf("a release whose SHA256SUMS do not describe it must fail the run:\n%s%s", run.stdout, run.stderr)
	}
	if strings.Contains(run.log, "attestation verify") {
		t.Errorf("the digests were rejected only after the attestation was asked for:\n%s", run.log)
	}
	run.assertTreeUntouched(t)
}
