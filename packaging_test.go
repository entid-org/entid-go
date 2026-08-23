// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid_test

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Section 1.2 of engine.md splits the work in two: a generator, which reads the
// bundle at build time, and the engine, which is what ships. The tests below
// inspect what ships rather than restating the intention, because the intention
// is exactly what a mistake here would keep intact.

// goTool locates the toolchain the inspection below questions. A missing one is
// a hard failure rather than a skip: a test that quietly stopped running would
// report the absence of a finding as a finding of absence.
func goTool(t *testing.T) string {
	t.Helper()
	found, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("no go on PATH, so nothing can be asked about the published package: %v", err)
	}
	return found
}

// TestPublishedPackageCarriesNoBundle asks the toolchain what the published
// import path pulls in. Nothing it reaches may embed a file, so no bundle
// travels with the engine; nothing it reaches may be a third party package, so
// no Protobuf runtime does either; and the generator, which does decode
// Protobuf, must stay outside the graph.
func TestPublishedPackageCarriesNoBundle(t *testing.T) {
	out, err := exec.Command(goTool(t), "list", "-deps",
		"-f", "{{.ImportPath}}\t{{.Standard}}\t{{len .EmbedFiles}}", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	const module = "github.com/libbusinessid/businessid-go"
	published := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("unreadable go list line %q", line)
		}
		path, standard, embedded := parts[0], parts[1] == "true", parts[2] != "0"
		if embedded {
			t.Errorf("%s embeds a file; the engine carries its rules as generated code, not as data", path)
		}
		if standard {
			continue
		}
		published++
		if path != module && !strings.HasPrefix(path, module+"/") {
			t.Errorf("the published package reaches %s, which is neither the standard library nor this module", path)
		}
		if strings.Contains(path, "protobuf") || strings.Contains(path, "/proto") {
			t.Errorf("the published package reaches %s: Protobuf belongs to the generator", path)
		}
		if strings.HasSuffix(path, "/internal/gen") {
			t.Errorf("the published package reaches the generator %s; it runs at build time and ships with nothing", path)
		}
	}
	if published == 0 {
		t.Fatal("go list reported no package at all; the inspection proved nothing")
	}
	t.Logf("%d non standard packages behind the published import path", published)
}

// TestModuleRequiresNothing states the same property from the other end: a
// module with no requirement cannot pull a Protobuf runtime into a consumer's
// build, whatever anyone later imports.
func TestModuleRequiresNothing(t *testing.T) {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "require" {
			t.Errorf("go.mod requires something: %q", strings.TrimSpace(line))
		}
	}
}

// TestPublishedAPITakesNoBundleBytes covers section 7.2: a custom rule set goes
// through the generator, at build time, and no exported entry point accepts a
// bundle at run time. A factory taking bytes is what such an entry point would
// look like, so nothing exported may take a byte slice at all.
func TestPublishedAPITakesNoBundleBytes(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		checked++
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || fn.Type.Params == nil {
				continue
			}
			if fn.Recv != nil && !receiverIsExported(fn.Recv) {
				continue
			}
			for _, param := range fn.Type.Params.List {
				if isByteSlice(param.Type) {
					t.Errorf("%s: %s takes a byte slice; section 7.2 forbids a run time bundle entry point",
						fset.Position(param.Pos()), fn.Name.Name)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source file was inspected; the test proved nothing")
	}
}

func receiverIsExported(recv *ast.FieldList) bool {
	if len(recv.List) == 0 {
		return false
	}
	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.IsExported()
}

func isByteSlice(expr ast.Expr) bool {
	arr, ok := expr.(*ast.ArrayType)
	if !ok || arr.Len != nil {
		return false
	}
	ident, ok := arr.Elt.(*ast.Ident)
	return ok && (ident.Name == "byte" || ident.Name == "uint8")
}

// TestNoRunnerLivesHere covers the rule section 8.7 of spec.md states: the
// conformance runner comes from the spec repository and nowhere else, because
// an engine that graded itself could compare too weakly and call the result
// conformance. What this repository builds is a testee and a generator, and the
// workflow must fetch the runner pinned to the commit rules.lock records, so
// that the comparator and the corpus can never come from different commits.
func TestNoRunnerLivesHere(t *testing.T) {
	entries, err := os.ReadDir("cmd")
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, e := range entries {
		if e.IsDir() {
			commands = append(commands, e.Name())
		}
	}
	sort.Strings(commands)
	want := []string{"businessid-demo", "businessid-gen", "businessid-testee"}
	if strings.Join(commands, ",") != strings.Join(want, ",") {
		t.Errorf("this repository builds %v, and may build only %v", commands, want)
	}

	workflow, err := os.ReadFile(filepath.Join(".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	ci := string(workflow)

	// The workflow must fetch the runner from the spec module.
	const runner = "github.com/libbusinessid/spec/cmd/conformance-runner@"
	if !strings.Contains(ci, runner) {
		t.Fatalf("the workflow never runs %s", runner)
	}
	// It must read the commit from rules.lock rather than repeat it, so that a
	// resynchronization cannot leave the comparator on one commit and the
	// corpus on another. A literal commit is what that mistake looks like.
	for _, m := range regexp.MustCompile(regexp.QuoteMeta(runner)+`(\S+)`).FindAllStringSubmatch(ci, -1) {
		if regexp.MustCompile(`^[0-9a-f]{7,40}`).MatchString(m[1]) {
			t.Errorf("the workflow pins the runner at the literal %q; it must read source_commit from rules.lock", m[1])
		}
		if !strings.Contains(m[1], "$") {
			t.Errorf("the runner reference %q expands no variable, so nothing ties it to rules.lock", m[1])
		}
	}
	if !regexp.MustCompile(`(?m)source_commit.*rules\.lock|rules\.lock.*source_commit`).MatchString(ci) {
		t.Error("the workflow never reads source_commit from rules.lock")
	}
	if !strings.Contains(ci, "spec/businessid-conformance.binpb") {
		t.Error("the workflow runs the runner against no corpus")
	}
	// rules.lock itself must carry a commit for the workflow to read.
	if commit := lockValue(t, "source_commit"); !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(commit) {
		t.Errorf("rules.lock records source_commit %q, which is no commit", commit)
	}
}

// moduleFiles lists the files this module owns, applying the rule the Go
// modules reference gives for what a module zip carries: everything under the
// module root, except version control directories and except any directory
// holding a go.mod of its own, which is a module in its own right.
func moduleFiles(t *testing.T) []string {
	t.Helper()
	vcs := map[string]bool{".git": true, ".hg": true, ".bzr": true, ".svn": true}
	var out []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, filepath.ToSlash(path))
			return nil
		}
		if path == "." {
			return nil
		}
		if vcs[d.Name()] {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("the walk found no file at all; the test proved nothing")
	}
	return out
}

// TestPublishedModuleCarriesNoSpecMirror covers the granularity go list cannot
// see. A module zip carries every file under the module root, not only the ones
// a package compiles, so the spec mirror travelled to every consumer's module
// cache - measured at 1 095 540 bytes of bundle, corpus and JSONL, more than
// half the archive - for code that never opens any of it.
//
// The go.mod under spec/ makes it a module of its own, which the parent's zip
// excludes. The generator still reads the files from disk, and sync_engines.sh
// copies file by file without erasing the directory, so the go.mod survives a
// resynchronization.
func TestPublishedModuleCarriesNoSpecMirror(t *testing.T) {
	// The mirror must still be there: a test that passed because the files had
	// been deleted would prove the opposite of what it claims.
	for _, needed := range []string{
		filepath.Join("spec", "go.mod"),
		filepath.Join("spec", "businessid-rules.binpb"),
		filepath.Join("spec", "businessid-conformance.binpb"),
		filepath.Join("spec", "businessid-conformance.jsonl"),
	} {
		if _, err := os.Stat(needed); err != nil {
			t.Fatalf("%s must exist and be excluded, not be missing: %v", needed, err)
		}
	}

	// The toolchain must agree that spec/ is a separate module, which is the
	// premise the exclusion rests on.
	out, err := exec.Command(goTool(t), "list", "-m").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	parent := strings.TrimSpace(string(out))
	cmd := exec.Command(goTool(t), "list", "-m")
	cmd.Dir = "spec"
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("go list -m in spec: %v", err)
	}
	if nested := strings.TrimSpace(string(out)); nested == parent {
		t.Fatalf("spec/ reports the module %q, the same as the parent: it is not a nested module", nested)
	}

	for _, path := range moduleFiles(t) {
		if strings.HasPrefix(path, "spec/") {
			t.Errorf("the published module carries %s", path)
		}
		switch filepath.Ext(path) {
		case ".binpb", ".jsonl":
			t.Errorf("the published module carries %s, which is rule or corpus data", path)
		}
	}
}

// TestRulesLockDigestsMatchTheMirror recomputes every digest rules.lock
// declares. The workflow checked only the bundle, so the corpus, the three
// .proto files and the two documents could have drifted from what the lock
// attests without anything noticing; the JSONL was not even listed until
// 2026.08.26, and engine tests cite its case ids as provenance.
func TestRulesLockDigestsMatchTheMirror(t *testing.T) {
	digests := map[string]string{
		"rules_sha256":             "businessid-rules.binpb",
		"conformance_sha256":       "businessid-conformance.binpb",
		"conformance_jsonl_sha256": "businessid-conformance.jsonl",
		"rules_proto_sha256":       "rules.proto",
		"conformance_proto_sha256": "conformance.proto",
		"testee_proto_sha256":      "testee.proto",
		"ir_doc_sha256":            "ir.md",
		"features_doc_sha256":      "features.md",
	}
	// Every key rules.lock declares must be one of these, so a digest added
	// upstream fails here instead of going unverified.
	raw, err := os.ReadFile("rules.lock")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`(?m)^(\w+_sha256)\s*=`).FindAllStringSubmatch(string(raw), -1) {
		if _, known := digests[m[1]]; !known {
			t.Errorf("rules.lock declares %s, which nothing here verifies", m[1])
		}
	}

	for key, name := range digests {
		content, err := os.ReadFile(filepath.Join("spec", name))
		if err != nil {
			t.Errorf("%s: %v", key, err)
			continue
		}
		sum := sha256.Sum256(content)
		if got, want := hex.EncodeToString(sum[:]), lockValue(t, key); got != want {
			t.Errorf("%s: spec/%s hashes to %s, rules.lock declares %s", key, name, got, want)
		}
	}
}
