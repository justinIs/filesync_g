# filesync_g — Design Plan

A Go project that watches a directory, tracks file state, and syncs changes to a
pluggable backend ("file store"). This document captures the objectives, the
high-level architecture, and the concrete decisions made for the proof of concept.

## PoC decisions

| Area | Decision |
|---|---|
| Config format | **TOML** (`github.com/BurntSushi/toml`) |
| Change detection | **Size+mtime gate, hash (SHA-256) fallback** |
| Run mode | **One-shot CLI** (cron/systemd or a later `--watch` flag add scheduling) |
| First backend | **AWS S3** (AWS SDK for Go v2), behind a `FileStore` interface |

---

## 1. Objectives

**Long-term vision**
- Watch a directory, track file state, and sync changes to a pluggable backend.
- Support multiple backends behind one interface: AWS S3, Google Drive, Git/GitHub.
- Respect a config file that declares what to ignore.

**PoC scope**
1. Config file parsing (TOML) with ignore rules for files/directories.
2. File tracking that detects changes (added / modified / deleted) between runs.
3. AWS S3 backend (upload changed files), with a clear IAM permission model.
4. A one-shot CLI as the foundation for how the app runs.

Guiding principle: **keep the sync logic backend-agnostic from day one**, even
though only S3 exists. That single decision is what lets Drive/Git be added later
without rewriting the core.

---

## 2. High-level architecture

A pipeline with one pluggable end:

```
┌─────────────┐   ┌──────────────┐   ┌──────────────┐   ┌─────────────┐
│   Config    │──▶│   Scanner    │──▶│   Tracker    │──▶│   Syncer    │
│ (load+parse)│   │(walk + ignore)│  │ (diff state) │   │(orchestrate)│
└─────────────┘   └──────────────┘   └──────────────┘   └──────┬──────┘
                                                                │
                                                         ┌──────▼──────┐
                                                         │  FileStore  │  ◀── interface
                                                         │  (S3 impl)  │
                                                         └─────────────┘
```

Suggested package layout (`internal/` so nothing leaks as a public API yet):

```
filesync_g/
  cmd/filesync/main.go      // flag parsing, wiring, entrypoint
  internal/config/          // Config struct, Load(), ignore matching
  internal/scan/            // directory walk, applies ignore rules
  internal/track/           // state file, diffing (the "what changed" brain)
  internal/store/           // FileStore interface + registry
  internal/store/s3/        // S3 implementation
  internal/store/local/     // local-dir fake backend for testing
  internal/sync/            // orchestration: scan -> diff -> push
  filesync.toml             // sample config
```

The key seam is the interface — keep it tiny:

```go
type FileStore interface {
    Put(ctx context.Context, key string, r io.Reader) error
    Delete(ctx context.Context, key string) error
    // List(ctx) → size + stored checksum per key, for reconciliation (§4 Verify)
}
```

Everything above `store` deals only with this interface. S3, Drive, and Git each
become one file implementing it. This is the dependency-inversion payoff.

---

## 3. Config file — reading & parsing

**Format: TOML** via `github.com/BurntSushi/toml` — the most Go-idiomatic parser,
comment-friendly (matters for a hand-edited ignore list), maps cleanly to a struct.

```toml
source = "."
ignore = [".git/", "node_modules/", "*.tmp", "**/secrets.json"]

[store]
type   = "s3"
bucket = "my-sync-bucket"
prefix = "backups/laptop"
region = "us-east-1"
```

- `toml.DecodeFile` reads + parses in one call.
- Add a separate **validate** step afterward: non-empty bucket, normalize `source`
  to an absolute path, compile the ignore patterns once into a matcher struct.
  Validate eagerly at startup so misconfiguration fails fast.

**Ignore matching** — three escalating levels:
1. Simplest: `filepath.Match` against the base name (stdlib). Handles `*.tmp`, not nested paths.
2. Better: match against the path relative to `source`, with a `/` suffix meaning
   "directory". Still stdlib. **← PoC target.**
3. Gitignore-compatible (`**`, negation `!`, anchoring): implement a subset yourself
   or pull `github.com/sabhiram/go-gitignore`.

Implement **level 2** yourself — a satisfying, self-contained Go exercise
(`filepath.Rel`, `filepath.Match`, string manipulation) with no dependency.
Important detail: when a *directory* matches an ignore rule, skip the whole subtree
(return `filepath.SkipDir` from `WalkDir`) — correct and a big performance win.

---

## 4. File tracking — detecting changes

**Strategy: size+mtime fast gate, SHA-256 fallback** (what rsync-style tools do).

Per-file logic against the manifest:
1. Not in manifest → **Added**.
2. In manifest, `size` differs → **Modified** (skip hashing — size proves it).
3. In manifest, size same but `mtime` differs → *suspect*: compute hash, compare.
   Equal → **Unchanged** (refresh mtime in manifest); differ → **Modified**.
4. Size and mtime both match → **Unchanged**, no hash needed.
5. In manifest but not in scan → **Deleted** (or skip, if backup-only semantics).

A later `--verify` flag has two flavors: a *local* full re-hash (catches the rare
mtime-preserving edit) and a *remote reconcile* against the backend (see **Verify /
reconciliation** below). Both are post-PoC.

**State storage — the manifest:**

```
.filesync/state.json   (in the source dir — and add it to the ignore list!)
```
```json
{
  "version": 1,
  "files": {
    "docs/readme.md": { "size": 1234, "mtime": "2026-06-01T10:00:00Z", "hash": "ab12…" }
  }
}
```

- Store all three fields so step 3 has something to compare against.
- **Write the manifest atomically**: write `state.json.tmp`, then `os.Rename`, so an
  interrupted run can't corrupt state.
- The manifest is backend-independent — S3/Drive/Git implementations never see it.

**Verify / reconciliation — local vs. remote:**

**Decision: `.filesync/` is never synced.** The manifest stays local; on a remote
`--verify`, the backend's own listing is the source of truth for "what the store
has." A remote copy of the manifest would only record what the source *believed* it
pushed — it lies the moment the store changes out-of-band, which is the exact drift
verify exists to catch.

Comparison is a **cost ladder** — cheap, definitive checks first, content hashing
only for the ambiguous remainder:

1. **Size** (from a bulk list) differs → **changed**. Free, definitive.
2. **Server-stored checksum** present → recompute the same algorithm locally and
   compare. Reliable, **no download**. The goal tier.
3. **Download + hash** → only when 1–2 can't decide (or the backend exposes no hash).

Tier 2 depends on what each backend stores:

| Backend | Hashable metadata | No-download compare? |
|---|---|---|
| **Git** | blob OID per path | Always — content-addressed; hash the local blob the Git way, compare OIDs |
| **Google Drive** | `md5Checksum`/`sha1Checksum` + `size`, in one `files.list` | Yes (binary files; Google-native Docs have no hash — special-case) |
| **S3** | `size`+`ETag` from `ListObjectsV2`; checksum via `HeadObject` | Yes, with the caveats below |
| **Local/SFTP** | `size`, mtime only | No server hash — hash remotely (`sha256sum` over SSH) or download |

**S3 specifics** (we control uploads, so make verify cheap by construction):
- **Don't trust ETag as a hash.** Single-part PUT: `ETag == MD5(content)`. **Multipart**:
  `ETag = MD5(concat of part MD5s) + "-N"` — *not* the object MD5; only matches if you
  replay the exact part size.
- **Set an additional checksum on every PUT** (`x-amz-checksum-{crc32c|sha256}`) —
  stored on the object, returned by `HeadObject`, and multipart-safe (full-object CRC).
  crc32c is cheap/hardware-accelerated; sha256 for crypto strength. Record the *same*
  hash in the local manifest at upload time so verify is a metadata-only compare.
- `ListObjectsV2` returns size+ETag for ~1000 keys/call but **not** checksums or
  `x-amz-meta-*` — those need one `HeadObject` per key. List to gate on size, Head only
  the survivors.

**Cross-cutting cautions:**
- **mtime doesn't cross backends.** S3 `LastModified` / Drive `modifiedTime` are server
  write times, not the local file's mtime — never compare for equality; at most a coarse
  newer/older signal.
- **Pick one checksum algorithm per backend** and store it consistently — the manifest's
  `hash` must match the algorithm the backend reports, or tier 2 is unusable.

This is why the manifest still records `hash` even though it's never uploaded, and why
`FileStore` needs a `List`/`Stat` returning **size + stored checksum** per key — the
reconciliation seam noted in §2.

---

## 5. How the app runs — options

| Mode | How | Pros | Cons |
|---|---|---|---|
| **One-shot CLI** | `filesync sync` runs once, exits | Dead simple; testable; composes with external schedulers | No automatic syncing |
| **Internal interval daemon** | one-shot logic in `for { sync(); time.Sleep(d) }`, `--watch` flag | Self-contained; no OS setup | Polling wastes work; reimplements scheduling |
| **OS scheduler** (cron/systemd timer) | ship the one-shot binary, OS calls it | Zero scheduling code; OS handles restarts/logging | Per-OS setup; not portable to Windows |
| **Filesystem events** | `fsnotify` (inotify/kqueue) | Near-real-time; no polling | A dependency; editor/rename edge cases; needs debouncing |

**Foundation: one-shot CLI.** It's the unit everything else wraps:
- Trivially testable (run, assert on the store).
- cron/systemd timers give "periodic" for free with zero extra code.
- `--watch` (interval loop) is ~15 lines later.
- Event-based (`fsnotify`) layers on the same `sync()` later, with debouncing
  (coalesce a burst of events, then call `sync()` once).

Implementation notes:
- `sync.Run(ctx, cfg)` is the pure-ish orchestrator: scan → diff → push.
  Every run mode just decides *when* to call it.
- `cmd/filesync/main.go`: parse flags (`-config`, `-dry-run`), use
  `signal.NotifyContext` for Ctrl-C, call `sync.Run`, exit non-zero on error.
- Thread `context.Context` from the start — cheap now, painful to retrofit, idiomatic.
- Add `-dry-run` early: it prints the diff without touching the store. Trivial since
  the diff is already a separate step, and invaluable while building.

---

## 6. AWS S3 integration & IAM

**Library: AWS SDK for Go v2** (`github.com/aws/aws-sdk-go-v2`) — the first-party
exception to the few-dependencies goal. Don't hand-roll SigV4. Modules: `config`,
`service/s3`, `feature/s3/manager` (multipart uploads).

**Credentials — use the SDK default chain.** `config.LoadDefaultConfig(ctx)` walks:
env vars → shared `~/.aws/credentials` profile → SSO → EC2/ECS/IRSA roles. The app
must **never** read credentials itself or take keys in its own config file.

**Upload mechanics:**
- Use `manager.NewUploader` — auto-switches to multipart for large files, handles concurrency.
- Key mapping: S3 object key = `prefix + "/" + relativePath`. Keep forward slashes
  (S3 keys are flat strings; this gives a folder-like view in the console).
- Deletes: `DeleteObject`. Consider making delete-on-remote opt-in for a backup tool.

**IAM — least privilege**, scoped to one bucket+prefix:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ListScopedToPrefix",
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": "arn:aws:s3:::my-sync-bucket",
      "Condition": { "StringLike": { "s3:prefix": ["backups/laptop/*"] } }
    },
    {
      "Sid": "ObjectRW",
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::my-sync-bucket/backups/laptop/*"
    }
  ]
}
```

Notes:
- `ListBucket` is a *bucket-level* action (resource = bucket ARN), constrained to the
  prefix via condition. `PutObject`/etc. are *object-level* (resource = object ARN
  with `/*`). Mixing these up is the #1 S3 IAM gotcha.
- Local dev: a dedicated IAM user with this policy + an access key in a named profile.
  Deployed: an IAM role (instance profile / IRSA), no static keys.
- Defense in depth: enable bucket versioning, block public access, consider SSE
  (KMS adds a `kms:GenerateDataKey` permission).

---

## 7. Build order for the PoC

1. `config` package: struct + `Load` + ignore matcher (+ tests on the matcher — pure functions, great to TDD).
2. `scan` package: `WalkDir` + ignore + `SkipDir`, returns `[]FileInfo`.
3. `track` package: manifest load/save (atomic) + diff function (pure, easy to test).
4. `store` interface + a `store/local` impl that copies to another dir —
   **lets the whole pipeline be tested with zero AWS/network.**
5. `sync` orchestration wiring 1–4 together; one-shot CLI in `cmd`.
6. Swap in `store/s3`; add `*.amazonaws.com` to the sandbox allowlist; test against a real bucket.

Step 4 is the trick that keeps you productive: a fake/local backend means the entire
engine is buildable and testable before touching AWS.

---

## Sandbox / environment notes

This machine runs Claude Code with a network sandbox. Before any real S3 call works
locally you'll need to add `*.amazonaws.com` to `sandbox.network.allowedDomains` in
`~/.claude/settings.json`. The hooks also block reads of `~/.aws/credentials`, so
credential setup is something to do outside the agent's bash environment.
