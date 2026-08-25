// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This command is the minimal consumer engine-go.md asks every engine to keep:
// it imports the published package and nothing else, so it is what would break
// first if the public API stopped being usable from outside.
//
// Its whole body is main, which calls os.Exit, so no test can call it. It is
// run as a process instead, which is the only way to observe what it does, and
// it is excluded from the coverage figure for the same reason: a statement
// counter cannot see a subprocess. What the counter cannot see, the assertions
// below still check.
//
// The identifier used here is the one the rest of this repository uses:
// BE0123456749, the value of the conformance case vat-be-valid-001, which the
// corpus classifies as synthetic and which designates no company.
func TestTheExampleConsumerRuns(t *testing.T) {
	// Built once and executed directly: go run reports its own exit status
	// rather than the program's, so it cannot answer what the program returns.
	binary := filepath.Join(t.TempDir(), "entid-demo")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the example consumer: %v\n%s", err, out)
	}

	run := func(t *testing.T, args ...string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		code := 0
		var exit *exec.ExitError
		if err != nil {
			if !asExitError(err, &exit) {
				t.Fatalf("run the example consumer: %v\n%s", err, stderr.String())
			}
			code = exit.ExitCode()
		}
		return stdout.String(), stderr.String(), code
	}

	t.Run("a valid identifier is reported on every step", func(t *testing.T) {
		out, errOut, code := run(t, "vat", "BE 0123.456.749")
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errOut)
		}
		for _, want := range []string{
			"kind      vat", "country   BE", "canonical BE0123456749",
			"format    valid (ok)", "checksum  valid (ok)",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("the output does not carry %q:\n%s", want, out)
			}
		}
	})

	t.Run("a kind no rule covers is reported, not refused", func(t *testing.T) {
		out, errOut, code := run(t, "no-such-kind", "whatever")
		if code != 0 {
			t.Fatalf("an unsupported kind is a result, not an engine error: exit %d, %s", code, errOut)
		}
		if !strings.Contains(out, "unsupported") {
			t.Errorf("the output does not say the kind is unsupported:\n%s", out)
		}
	})

	t.Run("the wrong number of arguments prints a usage line", func(t *testing.T) {
		_, errOut, code := run(t, "vat")
		if code != 2 {
			t.Fatalf("exit %d, want 2", code)
		}
		if !strings.Contains(errOut, "usage:") {
			t.Errorf("no usage line:\n%s", errOut)
		}
	})
}

// asExitError keeps the unwrapping out of the table above.
func asExitError(err error, out **exec.ExitError) bool {
	return errors.As(err, out)
}
