# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| main branch | :white_check_mark: |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue in c4git, please report it responsibly.

### How to Report

1. **DO NOT** create a public GitHub issue for security vulnerabilities
2. Report security issues via GitHub's private vulnerability reporting:
   - Go to https://github.com/Avalanche-io/c4git/security/advisories
   - Click "Report a vulnerability"
   - Provide a detailed description
   - Include steps to reproduce if possible

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Resolution Target**: 30 days for critical issues, 90 days for lower severity

## Security Considerations

c4git replaces git blob content with C4 IDs and stores the original file content locally. Users should be aware:

- **Store access**: The content store (`.c4/store`) holds files by C4 ID. Anyone with read access to the store directory can read any stored content. Protect the store directory with appropriate filesystem permissions.
- **`.c4git.yaml` trust**: The configuration file controls which file patterns are filtered and where content is stored. Review `.c4git.yaml` when cloning an unfamiliar repository. A malicious config could point the store path outside the repository.
- **Store not committed**: The `.c4/` directory is added to `.gitignore` by `c4git init`. If this entry is removed, stored content could be accidentally committed to git.
- **Content availability**: If the store is missing content for a C4 ID, the smudge filter passes the bare ID through. Files may appear as 90-byte text files until the content is available. This is by design and not a data loss event -- the ID uniquely identifies the content.
