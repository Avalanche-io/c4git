# c4git

Git clean/smudge filter that turns git into a media asset version control system using C4 IDs ([SMPTE ST 2114:2017](https://ieeexplore.ieee.org/document/8255805)).

Large files are replaced with their 90-character C4 ID on commit and rehydrated from any configured store on checkout. Artists don't need to know it's happening.

## Quick Start

```bash
# Install
go install github.com/Avalanche-io/c4git/cmd/c4git@latest

# Initialize in your repo
cd my-project
c4git init

# That's it. Large files are now managed by C4.
```

## How It Works

c4git uses git's native [clean/smudge filter](https://git-scm.com/book/en/v2/Customizing-Git-Git-Attributes) mechanism:

- **Clean** (on commit): Computes the C4 ID of the file, stores the content in a configured backing store, replaces the file in git with the 90-character C4 ID.
- **Smudge** (on checkout): Reads the C4 ID, fetches the content from the backing store, writes it to the working tree.

```
Working Tree          Git Repository         Backing Store
┌──────────┐   clean   ┌──────────┐   store   ┌──────────┐
│ hero.exr │ ───────→ │ c43zYc.. │ ───────→ │ hero.exr │
│ (200 MB) │          │ (90 B)   │          │ (200 MB) │
└──────────┘          └──────────┘          └──────────┘
      ↑       smudge        │       fetch        │
      └─────────────────────┘ ←──────────────────┘
```

## Configuration

`c4git init` creates a `.c4git.yaml` in the repo root and configures `.gitattributes`:

```yaml
# .c4git.yaml
stores:
  - type: directory
    path: .c4store/       # Local store (default)

# Files larger than this are offloaded
threshold: 1MB

# File patterns to always offload (in addition to threshold)
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

### Backing Stores

c4git is storage-agnostic. Configure one or more stores:

```yaml
stores:
  # Local directory (simplest, works offline)
  - type: directory
    path: /shared/c4store/

  # S3 bucket
  - type: s3
    bucket: studio-c4-assets
    region: us-west-2

  # c4d instance (local or remote)
  - type: c4d
    address: c4d.local:4444

  # Multiple stores for redundancy — fetch from fastest available
```

## Commands

```bash
c4git init          # Configure repo for c4git
c4git status        # Show managed files and store status
c4git fetch         # Pre-fetch all managed files from stores
c4git push          # Push local store contents to remote stores
c4git verify        # Verify working tree files against C4 IDs
c4git gc            # Remove unreferenced objects from local store
```

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

Apache 2.0 — see [LICENSE](./LICENSE).
