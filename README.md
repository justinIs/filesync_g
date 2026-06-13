# Filesync, G

[![CI][badge]][ci]

File sync CLI with TOML config.

## Usage

filesync scans `-source` (the current directory by default), diffs it against the
local manifest in `.filesync/`, and uploads changed files to the configured S3 store.

```sh
filesync                  # sync the current directory
filesync -source ~/notes  # sync a specific directory
filesync -dry-run         # preview changes without uploading or deleting
filesync -delete          # also remove remote files deleted locally
```

| Flag       | Default | Description                                                            |
| ---------- | ------- | ---------------------------------------------------------------------- |
| `-source`  | `.`     | Directory to sync.                                                     |
| `-delete`  | `false` | Delete remote files that were removed locally (asks for confirmation). |
| `-dry-run` | `false` | Show what would be uploaded or deleted without touching the store.     |
| `-v`       | `false` | Print per-file scan and change tables.                                 |

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

### Remote store (S3)

The `[store]` table points filesync at the S3 bucket it syncs to:

```toml
[store]
bucket  = "my-filesync-bucket" # required — destination bucket
region  = "us-east-1"          # optional — falls back to AWS_REGION / the profile's region
prefix  = "backups/"           # optional — key namespace within the bucket
profile = "filesync"           # optional — named AWS profile to use
```

| Field     | Type     | Default | Description                                                                   |
| --------- | -------- | ------- | ----------------------------------------------------------------------------- |
| `bucket`  | `string` | —       | **Required.** Destination S3 bucket.                                          |
| `region`  | `string` | `""`    | AWS region of the bucket. If empty, resolved from the environment/profile.    |
| `prefix`  | `string` | `""`    | Key prefix prepended to every object, so files land under one "folder".       |
| `profile` | `string` | `""`    | Named AWS profile to load credentials from. If empty, uses the default chain. |

## Development

`scripts/check.sh` runs the same things as the CI workflow.

```sh
scripts/check.sh         # build, vet, race tests, and format check (what CI runs)
scripts/check.sh --fix   # auto-format with goimports + gofumpt
```

[ci]: https://github.com/justinIs/filesync_g/actions/workflows/ci.yml
[badge]: https://github.com/justinIs/filesync_g/actions/workflows/ci.yml/badge.svg
[match]: https://pkg.go.dev/path#Match
