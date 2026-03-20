# Contributing to c4git

Thank you for your interest in contributing to c4git! This document provides guidelines for contributors.

## Project Structure

c4git is a git clean/smudge filter that replaces large files with 90-byte C4 IDs on commit and restores them on checkout. It depends on the [c4](https://github.com/Avalanche-io/c4) library for C4 ID computation and the TreeStore content store.

```
cmd/c4git/main.go    — command dispatch, init, clean/smudge wiring
cmd/c4git/git.go     — shared git helpers (managed file enumeration)
cmd/c4git/status.go  — status command
cmd/c4git/verify.go  — verify command
cmd/c4git/gc.go      — garbage collection command
filter/filter.go     — core clean/smudge filter logic
config/config.go     — .c4git.yaml loading and defaults
```

## Development

```bash
git clone https://github.com/Avalanche-io/c4git.git
cd c4git
go build ./...
go vet ./...
go test ./...
```

During development, `go.mod` may use a `replace` directive to reference a local checkout of the c4 library. For releases, this is replaced with a tagged version.

## Code Style

- Follow standard Go conventions (`gofmt`)
- Keep functions focused -- c4git is a thin bridge between git and c4
- The clean filter must produce exactly 90 bytes (bare C4 ID, no newline)
- The smudge filter must pass through bare IDs when content is unavailable

## Pull Requests

1. Create your branch from `main`
2. Make clear, concise commits (single-line messages)
3. Ensure `go build ./...`, `go vet ./...`, and `go test ./...` pass
4. Submit PR against `main`

## Bugs

Report issues at https://github.com/Avalanche-io/c4git/issues

## License

By contributing to c4git, you agree that your contributions will be licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.
