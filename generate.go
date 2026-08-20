// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package businessid

// The rules are compiled into Go source when the engine is built, never
// interpreted at run time. Regeneration is a deliberate maintainer action —
// after rules.lock names a new release — and never a build step: a consumer
// builds this module with the Go toolchain alone, offline.
//
//go:generate go run ./cmd/businessid-gen -bundle spec/businessid-rules.binpb -lock rules.lock -out rules_gen.go
