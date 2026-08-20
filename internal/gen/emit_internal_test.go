// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

import "testing"

// TestCommentText covers the guard that keeps a bundle string inside a single
// line Go comment. Check 6 of section 10 now constrains rules_version, so Load
// no longer lets such a version through; Generate is exported and takes a
// Bundle, so the guard stays as the last line of defence.
func TestCommentText(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2026.08.13", "2026.08.13"},
		{"0\x000", `"0\x000"`},
		{"1.0\nfunc x() {}", `"1.0\nfunc x() {}"`},
		{"\x7f", `"\x7f"`},
	} {
		if got := commentText(tc.in); got != tc.want {
			t.Errorf("commentText(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
