# c4 Go reference release v1.0.13 — port report for c4git

c4git imports `github.com/Avalanche-io/c4` as a Go dependency, so it picks
up the algorithmic fixes automatically once it bumps its pin. The release
also adds new public API surface that c4git may want to use.

When you've decided what to do about each item, **delete this file** (or
replace it with `PORT_STATUS.md` recording what you adopted).

## Action: bump the c4 dependency

`go.mod` currently pins `github.com/Avalanche-io/c4 v1.0.10`. Bump to
`v1.0.13` (or whatever the latest is when you read this):

```
go get -u github.com/Avalanche-io/c4
go mod tidy
go test ./...
```

Running the full c4git test suite after the bump is mandatory — the
release fixed a pre-existing **spec violation** in `scan.PropagateMetadata`
(it used to silently skip null values; now it nil-infects per
`SPECIFICATION.md`). Any c4git test that produced manifests with mixed
null/known descendant sizes will now correctly poison parent sizes to
`-1`. If a test was relying on the old wrong behavior, that's the test
that should change, not the new code.

## What the bump gets you for free

### Performance

- Whole-tree metadata propagation is `O(N)` instead of `O(D × N)`.
  Tangible win at any scan over ~500K entries (it was a 16-20 minute
  hang at 5M).
- `c4m.Manifest.SortEntries` is `O(N log N)` worst case instead of
  `O(N²)` on pathologically nested trees. 327× speedup at 100K-depth
  chain.
- `c4m.Manifest.ComputeC4ID` streams canonical bytes through `io.Pipe`,
  dropping `bytes/op` ~23% at 100K entries. Useful if c4git ever hashes
  large manifests.

### Removed APIs (none used by c4git, but verify)

- `scan.PropagateMetadata`
- `scan.CalculateDirectorySize`
- `scan.GetMostRecentModtime`

All three were exported but no external callers were found in the
workspace. If c4git ever called one of these, switch to
`c4m.PropagateMetadata` (now exported as the One True Implementation —
which is what the scan versions internally delegate to anyway).

## New APIs c4git may want to use

c4git is a `c4 + git` integration. The new options on `scan.Generator`
are likely useful here:

### `c4m.PropagateMetadata([]*Entry)` — exported

If c4git ever constructs manifests programmatically (rather than via
`scan.Dir`), call this after `SortEntries` to fill in directory sizes
and timestamps the way `Canonicalize` would.

### `c4m.Manifest.WriteCanonical(w io.Writer) error`

Stream canonical bytes to any writer. Useful if c4git is producing very
large manifests for big repos and wants to hash without materializing
the canonical text. `Canonical()` still works for back-compat.

### `scan.WithProgress(func(ScanStats))`

For c4git's CLI, progress feedback during long scans of git work trees.
Fires at most every 1000 entries or every 250 ms, plus once at end. Zero
overhead when unused.

### `scan.WithMaxConcurrency(n int)`

Parallel subdirectory walk via a bounded worker pool (default
`min(GOMAXPROCS, 16)`). **Output is byte-identical to sequential.**
5.6× cold-cache speedup on the 122K-entry tree we benched.
`WithMaxConcurrency(1)` forces sequential.

### `scan.WithContext(ctx context.Context)`

Context cancellation observed at directory and entry boundaries. Partial
manifest returned alongside the error on cancel. Useful for c4git
operations that the user can Ctrl-C.

### `scan.WithEntryStream(cb func(*c4m.Entry) error)`

Per-entry callback. Useful if c4git wants to emit progress lines, persist
entries incrementally, or integrate with `git fast-import`-style
streaming. The callback may fire from multiple goroutines under parallel
walk but is serialized via an internal mutex, so the body doesn't need
to be thread-safe. Order is discovery order — pair with
`WithMaxConcurrency(1)` if strict walk-order is required.

When either `WithContext` or `WithEntryStream` is set, partial manifests
are returned alongside errors rather than `nil` — preserves the historical
nil-on-error contract for callers that don't opt in.

## Reference material in the Go repo

- `~/ws/active/c4/oss/c4/CHANGELOG.md` — full v1.0.13 entry
- `~/ws/active/c4/oss/c4/design/scan-memory-and-throughput.md` — design
  doc for the propagation fix
- `~/ws/active/c4/oss/c4/c4m/manifest.go` — `PropagateMetadata`,
  `WriteCanonical`
- `~/ws/active/c4/oss/c4/scan/generator.go` — `WithProgress`,
  `WithMaxConcurrency`, `WithContext`, `WithEntryStream`
- `~/ws/active/c4/oss/c4/ARCHITECTURAL_CATALOG.md` — updated API
  inventory

Delete this file once you've bumped the pin and decided which new
options to adopt.
