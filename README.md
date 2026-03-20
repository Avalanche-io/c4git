# c4git

[![CI](https://github.com/Avalanche-io/c4git/actions/workflows/ci.yml/badge.svg)](https://github.com/Avalanche-io/c4git/actions/workflows/ci.yml)
[![Apache 2.0 License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)

Git clean/smudge filter that turns git into a media asset version control system using C4 IDs ([SMPTE ST 2114:2017](https://ieeexplore.ieee.org/document/8255805)).

Large files are replaced with their 90-byte C4 ID on commit and restored from any configured store on checkout. Artists don't need to know it's happening.

## Install

c4git is part of the [c4 toolkit](https://github.com/Avalanche-io/c4).
See the c4 README for installation instructions.

## Quick Start

```bash
# Initialize in your repo
cd my-project
c4git init

# That's it. Large files are now managed by C4.
```

## How It Works

c4git uses git's native [clean/smudge filter](https://git-scm.com/book/en/v2/Customizing-Git-Git-Attributes) mechanism:

- **Clean** (on `git add`): Computes the C4 ID of the file, stores the original content in the backing store, and writes a bare 90-byte C4 ID to git. No newline, no prefix -- exactly 90 characters of base58-encoded SHA-512 hash.
- **Smudge** (on `git checkout`): Reads the 90-byte C4 ID from the git blob, fetches the original content from the store, and writes it to the working tree. If the content is not in the store, the bare ID passes through unchanged.

The clean filter is idempotent: if the input is already a 90-byte C4 ID (or 91 bytes with a trailing newline), it passes through without re-storing.

```
Working Tree          Git Repository         Backing Store
+-----------+   clean   +-----------+   store   +-----------+
| hero.exr  | --------> | c43zYc..  | --------> | hero.exr  |
| (200 MB)  |           | (90 B)    |           | (200 MB)  |
+-----------+           +-----------+           +-----------+
      ^        smudge         |        fetch         |
      +----------------------------------------------|
```

Every commit that touches a managed file stores only 90 bytes in git. The actual content lives in `.c4/store`, a local directory-based content store shared with the rest of the c4 toolkit (`c4`, `c4sh`). Because content is addressed by its C4 ID, identical files across branches, repos, or machines are automatically deduplicated.

## Configuration

`c4git init` creates a `.c4git.yaml` in the repo root and configures `.gitattributes`:

```yaml
# .c4git.yaml
stores:
  - type: directory
    path: .c4/store       # Local tree store (default)

# File patterns to manage via C4
patterns:
  - "*.exr"
  - "*.dpx"
  - "*.mov"
  - "*.mp4"
  - "*.abc"
  - "*.vdb"
  - "*.bgeo"
  - "*.usd"
  - "*.usdc"
  - "*.usdz"
```

The store at `.c4/store` is a tree store -- a content-addressed directory layout used by the c4 library. It is the same store format used by `c4sh` and `c4d`, so content written by any tool is available to all of them.

`c4git init` also:
- Creates `.c4/store` if it doesn't exist
- Adds `.c4` to `.gitignore` (the store is never committed)
- Writes `.gitattributes` entries mapping each pattern to the `c4` filter
- Configures git's `filter.c4.clean`, `filter.c4.smudge`, and `filter.c4.required` settings

## Commands

| Command | Description |
|---|---|
| `c4git init` | Configure the current git repo for c4git |
| `c4git clean` | Clean filter (stdin/stdout, called by git) |
| `c4git smudge` | Smudge filter (stdin/stdout, called by git) |
| `c4git status` | Show managed files and store status (ok/missing) |
| `c4git verify` | Verify working tree files against committed C4 IDs |
| `c4git gc` | Show unreferenced objects in the local store (dry run) |
| `c4git gc --force` | Remove unreferenced objects from the local store |
| `c4git version` | Print version |

## Comparison

| | c4git | git-lfs | git-annex | Perforce |
|---|---|---|---|---|
| Server required | No | Yes | No | Yes |
| Storage backends | Any | LFS server | Various | P4 server |
| Deduplication | Automatic | No | Yes | No |
| Cross-repo sharing | Yes | No | Possible | No |
| Complexity | Minimal | Moderate | High | High |
| Content standard | SMPTE ST 2114 | None | None | None |
| Offline support | Full | Partial | Full | None |

## License

Apache 2.0 -- see [LICENSE](./LICENSE).
