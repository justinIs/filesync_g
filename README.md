# Filesync, G

[![CI][badge]][ci]

File sync CLI with TOML config.

Uses file size + mtime gate and hashing to track file changes.

## Configuration

filesync reads `filesync.toml` from the working directory:

```toml
# Glob patterns for paths to skip during sync.
ignore = [
  "*.tmp",        # any *.tmp file, at any depth
  "node_modules", # any file or dir named node_modules, at any depth
  ".git/",        # any directory named .git (trailing slash = dirs only)
  "build/*.log",  # *.log files directly under the top-level build/ dir
]
```

| Field    | Type       | Default | Description                                                                                   |
| -------- | ---------- | ------- | --------------------------------------------------------------------------------------------- |
| `ignore` | `[]string` | `[]`    | Glob patterns to skip ([`path.Match`][match]); `/` anchors to root, trailing `/` = dirs only. |

## Development

`scripts/check.sh` runs the same things as the CI workflow.

```sh
scripts/check.sh         # build, vet, race tests, and format check (what CI runs)
scripts/check.sh --fix   # auto-format with goimports + gofumpt
```

[ci]: https://github.com/justinIs/filesync_g/actions/workflows/ci.yml
[badge]: https://github.com/justinIs/filesync_g/actions/workflows/ci.yml/badge.svg
[match]: https://pkg.go.dev/path#Match
