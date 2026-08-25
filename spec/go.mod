// This directory is a mirror of the specification repository, copied file by
// file by its tools/sync_engines.sh. It is an input to the generator, which
// reads it from disk at build time, and to the tests; no published package
// reads any of it.
//
// The go.mod is what keeps it out of the published module. A Go module zip
// carries every file under the module root, so without this file a consumer
// downloading the engine would also download the bundle, the corpus and the
// JSONL - measured at 1 095 540 bytes, more than half the archive - for code
// that never opens them. A directory holding a go.mod is a module of its own,
// which the parent module's zip excludes.
//
// Nothing is ever built from here. The module path is local and unpublished,
// and the directory holds no Go source at all.
module github.com/entid-org/entid-go/spec

go 1.24
