// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Section 11.3 of engine.md states, as observable properties, what proves a
// testee does not cheat. The tests below are those properties, one per claim:
//
//	the testee names neither the corpus nor anything that reads one
//	it reaches no file system
//	it answers identically whatever the case identifier
//	it answers identically whatever the order of the requests
//	it answers identically to a repeated request
//
// The requests are built here, byte by byte. The honesty tests never open the
// corpus, because a test that read it would demonstrate the opposite of what it
// claims. The two identifiers they carry are the ones this repository already
// uses everywhere else; they are synthetic values of the shared corpus, not
// values taken from a register, and nothing about these tests depends on
// whether they are valid.

// modulePath reads the module path from go.mod, so that the import graph can be
// split into this repository's packages and the standard library without a
// literal typed here.
func modulePath(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^module\s+(\S+)`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("go.mod declares no module path")
	}
	return m[1]
}

// goPackage is one package of this repository, parsed from source.
type goPackage struct {
	importPath string
	dir        string
	files      []*ast.File
	fset       *token.FileSet
	imports    []string
}

// parsePackage parses the non test files of one directory. Test files are
// excluded on purpose: what ships in the testee is what its non test sources
// name, and this very file reads go.mod.
func parsePackage(t *testing.T, dir, importPath string) goPackage {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	pkg := goPackage{importPath: importPath, dir: dir, fset: token.NewFileSet()}
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// Comments are deliberately not parsed: a comment explaining that the
		// testee never reads the corpus must not be read as naming it.
		f, err := parser.ParseFile(pkg.fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		pkg.files = append(pkg.files, f)
		for _, spec := range f.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			if !seen[path] {
				seen[path] = true
				pkg.imports = append(pkg.imports, path)
			}
		}
	}
	if len(pkg.files) == 0 {
		t.Fatalf("no source file in %s", dir)
	}
	sort.Strings(pkg.imports)
	return pkg
}

// closure parses the testee and every package of this repository it reaches,
// directly or not. The standard library is left out: no standard package opens
// a file on its own, so any file access this executable could perform is
// written in one of the packages returned here.
func closure(t *testing.T) []goPackage {
	t.Helper()
	module := modulePath(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dirOf := func(importPath string) string {
		rel := strings.TrimPrefix(strings.TrimPrefix(importPath, module), "/")
		if rel == "" {
			return root
		}
		return filepath.Join(root, rel)
	}

	testee := module + "/cmd/businessid-testee"
	queue := []string{testee}
	done := map[string]bool{}
	var out []goPackage
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if done[path] {
			continue
		}
		done[path] = true
		pkg := parsePackage(t, dirOf(path), path)
		out = append(out, pkg)
		for _, imported := range pkg.imports {
			if imported == module || strings.HasPrefix(imported, module+"/") {
				queue = append(queue, imported)
			}
		}
	}
	return out
}

// TestTesteeNamesNoCorpus is the first property: nothing the testee compiles
// names the corpus, its serialized form, or a directory holding one. What
// cannot be named cannot be opened, and a case identifier appearing as a
// literal would be recognition rather than translation.
//
// The scan is over the testee's own sources. The other half of the property,
// that it names nothing which reads a corpus either, is what the import check
// below states: no package it links can open a file at all. Widening this scan
// to the whole closure would only catch the provenance prose the bundle carries,
// which names the shared corpus as documentation and reads nothing.
func TestTesteeNamesNoCorpus(t *testing.T) {
	forbidden := []string{"corpus", "conformance", "binpb", "jsonl", "testdata", "expected"}
	module := modulePath(t)
	for _, pkg := range closure(t) {
		if pkg.importPath != module+"/cmd/businessid-testee" {
			continue
		}
		for _, f := range pkg.files {
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				lowered := strings.ToLower(lit.Value)
				for _, word := range forbidden {
					if strings.Contains(lowered, word) {
						t.Errorf("%s: the string %s names %q",
							pkg.fset.Position(lit.Pos()), lit.Value, word)
					}
				}
				return true
			})
		}
	}
}

// TestTesteeReachesNoFileSystem is the second property. The corpus is a file, so
// an executable that opens nothing cannot read it. Two things are checked: no
// package of this repository that the testee reaches imports a package able to
// open a file, a socket or a process, and the testee's own use of os is limited
// to the three standard streams and the exit status.
func TestTesteeReachesNoFileSystem(t *testing.T) {
	// os is handled separately below: the testee needs it for its streams.
	sealed := map[string]bool{
		"embed": true, "io/fs": true, "io/ioutil": true, "net": true,
		"net/http": true, "os/exec": true, "path/filepath": true,
		"syscall": true, "plugin": true, "database/sql": true,
	}
	module := modulePath(t)
	testee := module + "/cmd/businessid-testee"

	for _, pkg := range closure(t) {
		for _, imported := range pkg.imports {
			if sealed[imported] {
				t.Errorf("%s imports %q, which opens things the testee has no business opening",
					pkg.importPath, imported)
			}
			if imported == "os" && pkg.importPath != testee {
				t.Errorf("%s imports os; only the testee itself may, and only for its streams",
					pkg.importPath)
			}
		}
		if pkg.importPath != testee {
			continue
		}
		allowed := map[string]bool{"Stdin": true, "Stdout": true, "Stderr": true, "Exit": true}
		for _, f := range pkg.files {
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok || ident.Name != "os" || allowed[sel.Sel.Name] {
					return true
				}
				t.Errorf("%s: the testee reaches os.%s",
					pkg.fset.Position(sel.Pos()), sel.Sel.Name)
				return true
			})
		}
	}
}

// answers runs one stream of requests through the server and returns one
// response per request, in the order the requests were written. The protocol is
// strictly synchronous, so position is what pairs a response with its request:
// nothing here reads the case identifier back, which is the field under test.
func answers(t *testing.T, reqs ...requestOf) [][]byte {
	t.Helper()
	var in bytes.Buffer
	for _, r := range reqs {
		frame(&in, r.encode())
	}
	var out bytes.Buffer
	if err := serve(&in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	body := out.Bytes()
	var got [][]byte
	for len(body) > 0 {
		if len(body) < 4 {
			t.Fatalf("a trailing %d byte fragment is not a frame", len(body))
		}
		n := int(binary.LittleEndian.Uint32(body[:4]))
		if n > len(body)-4 {
			t.Fatalf("frame declares %d bytes but %d follow", n, len(body)-4)
		}
		got = append(got, body[4:4+n])
		body = body[4+n:]
	}
	if len(got) != len(reqs) {
		t.Fatalf("%d responses for %d requests", len(got), len(reqs))
	}
	return got
}

// verdict is a response stripped of the echoed case identifier: what the testee
// actually decided about the request, with nothing left that names the case.
func verdict(t *testing.T, response []byte) string {
	t.Helper()
	decoded := fields(t, response)
	delete(decoded, 1)
	keys := make([]int, 0, len(decoded))
	for k := range decoded {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%d=%x;", k, decoded[k])
	}
	return b.String()
}

// probe is one request the honesty tests replay. The values are the ones this
// repository already carries; the point is that the answer never moves, not
// what the answer is.
var probes = []requestOf{
	{operation: opValidate, kind: "vat", input: "BE 0123.456.749", profile: "compatible", presentProfile: true},
	{operation: opValidate, kind: "siren", input: "012345674"},
	{operation: opValidateFormat, kind: "siren", input: "0123"},
	{operation: opValidateChecksum, kind: "vat", input: "BE0123456748"},
	{operation: opCanonicalize, kind: "  VAT  ", input: " be 0123.456.749 "},
	{operation: opCanonicalize, kind: "siren", input: "0123\xff"},
	{operation: opValidate, kind: "no-such-kind", input: "whatever"},
	// A bundle written here, byte by byte: field 1, varint 99, which is a
	// format version no generator supports.
	{operation: opLoadRuleset, payload: []byte{0x08, 0x63}},
	{operation: 99, input: "an operation nobody defines"},
}

// TestAnswerDoesNotDependOnTheCaseID is the third property. A testee that
// recognized a case would have to read its identifier; this one echoes it and
// uses it for nothing, so a plausible identifier, an absurd one, one shaped
// like a path and none at all all produce the same verdict.
func TestAnswerDoesNotDependOnTheCaseID(t *testing.T) {
	ids := []string{
		"",
		"siren-valid-001",
		"loader-subject-node-circular-037",
		"../../spec/businessid-conformance.binpb",
		"\x00\x01\x02",
		strings.Repeat("x", 4096),
		"🙂 ni le cas ni son nom",
	}
	for i, probe := range probes {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			var want string
			for j, id := range ids {
				req := probe
				req.caseID = id
				got := verdict(t, answers(t, req)[0])
				if j == 0 {
					want = got
					continue
				}
				if got != want {
					t.Fatalf("case id %q moved the verdict:\n got %s\nwant %s", id, got, want)
				}
			}
		})
	}
}

// TestAnswerDoesNotDependOnTheRequestOrder is the fourth property: the testee
// holds no state across requests, so reordering a stream reorders the responses
// and changes nothing else.
func TestAnswerDoesNotDependOnTheRequestOrder(t *testing.T) {
	alone := make([]string, len(probes))
	for i, probe := range probes {
		req := probe
		req.caseID = "alone"
		alone[i] = verdict(t, answers(t, req)[0])
	}

	orders := [][]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8},
		{8, 7, 6, 5, 4, 3, 2, 1, 0},
		{4, 0, 8, 2, 6, 1, 7, 3, 5},
		{7, 7, 0, 0, 8, 8, 3, 3, 5},
	}
	for _, order := range orders {
		stream := make([]requestOf, len(order))
		for i, index := range order {
			req := probes[index]
			req.caseID = fmt.Sprintf("position-%d", i)
			stream[i] = req
		}
		got := answers(t, stream...)
		for i, index := range order {
			if v := verdict(t, got[i]); v != alone[index] {
				t.Fatalf("probe %d answered differently at position %d of %v:\n got %s\nwant %s",
					index, i, order, v, alone[index])
			}
		}
	}
}

// TestARepeatedRequestGetsTheSameAnswer is the fifth property: the same request
// asked again, in the same stream and in a fresh one, gets the same answer, so
// nothing in the testee is random, timed or counted.
func TestARepeatedRequestGetsTheSameAnswer(t *testing.T) {
	const repeats = 5
	for i, probe := range probes {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			stream := make([]requestOf, repeats)
			for j := range stream {
				req := probe
				req.caseID = "same"
				stream[j] = req
			}
			got := answers(t, stream...)
			want := verdict(t, got[0])
			for j, response := range got[1:] {
				if v := verdict(t, response); v != want {
					t.Fatalf("repetition %d differs:\n got %s\nwant %s", j+1, v, want)
				}
			}
			// A fresh server, a fresh engine, the same answer.
			req := probe
			req.caseID = "same"
			if v := verdict(t, answers(t, req)[0]); v != want {
				t.Fatalf("a fresh process differs:\n got %s\nwant %s", v, want)
			}
		})
	}
}
