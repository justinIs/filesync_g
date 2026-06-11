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

## Remote store (S3)

The `[store]` table points filesync at the S3 bucket it syncs to:

```toml
[store]
bucket  = "my-filesync-bucket" # required — destination bucket
region  = "us-east-1"          # optional — falls back to AWS_REGION / the profile's region
prefix  = "backups/"           # optional — key namespace within the bucket
profile = "filesync"           # optional — named AWS profile to use
```

| Field     | Type     | Default | Description                                                                       |
| --------- | -------- | ------- | --------------------------------------------------------------------------------- |
| `bucket`  | `string` | —       | **Required.** Destination S3 bucket.                                              |
| `region`  | `string` | `""`    | AWS region of the bucket. If empty, resolved from the environment/profile.        |
| `prefix`  | `string` | `""`    | Key prefix prepended to every object, so files land under one "folder".           |
| `profile` | `string` | `""`    | Named AWS profile to load credentials from. If empty, uses the default chain.     |

### Credentials

filesync **never** reads AWS credentials from `filesync.toml` — keep that file in your
repo, keep secrets out of it. Credentials come from the standard AWS chain (environment
variables, `~/.aws/credentials`, SSO, or an instance/role), so anything the AWS CLI can
use, filesync can use.

### One-time AWS setup (IAM user + access keys)

1. **Create the bucket** (or use an existing one), noting its name and region.

2. **Create a least-privilege policy** scoped to your bucket/prefix. Replace
   `MY-BUCKET` and `MY-PREFIX/` (drop `MY-PREFIX/` to allow the whole bucket):

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Sid": "FilesyncWrite",
         "Effect": "Allow",
         "Action": ["s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload"],
         "Resource": "arn:aws:s3:::MY-BUCKET/MY-PREFIX/*"
       }
     ]
   }
   ```

   `AbortMultipartUpload` lets the SDK clean up if a large, multipart upload fails.

3. **Create an IAM user**, attach the policy, and generate an access key
   (Security credentials → Create access key → "Command Line Interface").

4. **Configure a profile** so the key lives in `~/.aws/`, not in the repo:

   ```sh
   aws configure --profile filesync   # enter the access key, secret, and region
   ```

   Then set `profile = "filesync"` in `[store]` (or export `AWS_PROFILE=filesync`).

## Development

`scripts/check.sh` runs the same things as the CI workflow.

```sh
scripts/check.sh         # build, vet, race tests, and format check (what CI runs)
scripts/check.sh --fix   # auto-format with goimports + gofumpt
```

[ci]: https://github.com/justinIs/filesync_g/actions/workflows/ci.yml
[badge]: https://github.com/justinIs/filesync_g/actions/workflows/ci.yml/badge.svg
[match]: https://pkg.go.dev/path#Match
