# c4git Architectural Catalog

## Packages

### `filter` — Clean and smudge logic
- **`Clean(r, w, store)`** — Reads content from r, stores it via the Store interface, writes a bare 90-byte C4 ID (no newline) to w. Idempotent: detects already-cleaned C4 IDs and passes through unchanged.
- **`Smudge(r, w, store)`** — Reads a C4 ID from r (capped at 256 bytes), fetches content from the store, writes it to w. If content is missing, passes the bare ID through with a warning on stderr.
- **`Store` interface** — Minimal interface requiring `Has(id)`, `Open(id)`, and `Put(r)`.

### `config` — Configuration loading
- **`Config`** — YAML config struct with stores and patterns
- **`Load(dir)`** — Reads `.c4git.yaml`, returns defaults if absent
- **`Config.Validate()`** — Checks at least one store with non-empty path
- **`Config.Write(dir)`** — Serializes config to `.c4git.yaml`
- **`Default()`** — Returns default config: single directory store at `.c4/store`, common media patterns

### `cmd/c4git` — CLI entry point
- `init` — Creates store dir, writes config, updates `.gitignore`, `.gitattributes`, configures git filter
- `clean` — Runs clean filter (stdin -> stdout)
- `smudge` — Runs smudge filter (stdin -> stdout)
- `status` — Lists managed files with store presence (ok/missing), shows full relative paths
- `verify` — Checks working tree files against stored C4 IDs (ok/modified/not restored/missing/error)
- `gc` — Removes unreferenced objects from local store (dry run by default, `--force` to delete). Tracks freed size separately from total unreferenced size.
- `version` — Prints version string

### `cmd/c4git/git.go` — Shared git helpers
- **`managedFile`** — Struct pairing a tracked file path with its C4 ID
- **`managedFiles()`** — Enumerates stage-0 tracked files whose blob content is a 90-byte C4 ID, using `git ls-files -s` + `git cat-file --batch`
- **`parseBatchOutput()`** — Parses batch output, returns error on unexpected read failures (EOF is not an error)

## Dependencies
- `github.com/Avalanche-io/c4` — C4 ID computation, parsing, and TreeStore
- `gopkg.in/yaml.v3` — Config file parsing
