// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid_test

import (
	"go/ast"
	"go/parser"
	"go/token"
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
