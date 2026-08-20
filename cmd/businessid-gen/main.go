// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

// Command businessid-gen compiles a LibBusinessID rule bundle into Go source.
//
// It runs when the engine is built, never when the engine validates an
// identifier. A bundle that does not satisfy the IR contract stops it here,
// which is what section 2.3 of the specification means by closed generation.
//
// Usage:
//
//	businessid-gen -bundle spec/businessid-rules.binpb -lock rules.lock -out rules_gen.go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/libbusinessid/businessid-go/internal/gen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "businessid-gen: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	bundlePath := flag.String("bundle", "spec/businessid-rules.binpb", "path to the compiled rule bundle")
	lockPath := flag.String("lock", "rules.lock", "path to rules.lock, whose digests the bundle must match")
	outPath := flag.String("out", "rules_gen.go", "path of the Go file to write")
	pkg := flag.String("package", "businessid", "package name of the generated file")
	flag.Parse()

	raw, err := os.ReadFile(*bundlePath)
	if err != nil {
		return fmt.Errorf("read the bundle: %w", err)
	}
	if err := verifyLock(*lockPath, raw); err != nil {
		return err
	}

	bundle, err := gen.Load(raw)
	if err != nil {
		// Section 2.3: an unknown version, field, opcode or capability stops
		// generation rather than producing partial code.
		return fmt.Errorf("refusing to generate from %s: %w", *bundlePath, err)
	}

	src, err := gen.Generate(bundle, *pkg)
	if err != nil {
		return err
	}
	// The output is Go source meant to be committed and read, so it carries
	// the permissions a source file normally does.
	if err := os.WriteFile(*outPath, src, 0o644); err != nil { //nolint:gosec // generated source, not a secret
		return fmt.Errorf("write %s: %w", *outPath, err)
	}
	fmt.Fprintf(os.Stderr, "businessid-gen: wrote %s (%d bytes) from rules %s\n",
		*outPath, len(src), bundle.RulesVersion)
	return nil
}

// verifyLock checks the bundle against the digest rules.lock declares.
//
// rules.lock is the only coupling point between this repository and the spec
// repository: it names a release and attests its content. Until a release is
// tagged it carries no attestation identity, which its own header explains, so
// only the digest is checked here.
func verifyLock(path string, bundle []byte) error {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is a flag of a developer tool
	if err != nil {
		return fmt.Errorf("read the lock: %w", err)
	}
	want, err := lockValue(string(raw), "rules_sha256")
	if err != nil {
		return err
	}
	sum := sha256.Sum256(bundle)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("the bundle digest is %s but %s declares %s", got, filepath.Base(path), want)
	}
	return nil
}

// lockValue reads one `key = "value"` entry of rules.lock.
func lockValue(content, key string) (string, error) {
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"`), nil
	}
	return "", errors.New("rules.lock declares no " + key)
}
