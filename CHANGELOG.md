# Changelog

All notable changes to this project are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning follows [Semantic Versioning](https://semver.org/) — this project is pre-1.0, so behavior and structure can still change between minor versions without notice.

## [Unreleased]

### Added

- Structured, leveled logging (`log/slog`) throughout — no more `fmt.Println`/`fmt.Printf`. Text output by default (readable during active development); the JSON-handler swap for a log aggregator is a one-line change, documented in a comment in `main.go`.
- `ROADMAP.md` — the living status doc (done / known gaps / planned), split out so it can be updated independently of the README's pitch.
- `chainsync.HeaderFetcher` interface and a real test for `FindForkPoint` — a fake chain client stands in, since a real mainnet reorg can't be triggered on demand. Covers no-reorg, a shallow reorg, a reorg deeper than any stored history, and the already-at-finality early return.
- Finality sweep (`storage.MarkFinalized`) — `is_final` was previously set once at decode time and never revisited, so a transfer seen before it was finalized stayed stamped `false` forever, even long after it genuinely settled. Now flipped correctly once per poll tick.
- HTTP server (`server` package) — wraps `*http.Server` with real read/write timeouts, runs in its own goroutine. `GET /health` is the only route so far.
- First webhook (`webhooks/payment_status.go`) — notifies a configured URL when a transfer reaches final status, HMAC-SHA256 signed (`X-Signature` header) so the receiver can verify authenticity. `storage.GetUnnotifiedFinalTransfers`/`MarkWebhookSent` track delivery separately from settlement status (`webhook_sent` is its own column, not inferred from `is_final`). `WEBHOOK_URL`/`WEBHOOK_SECRET` from `.env`; unset disables delivery entirely.

### Changed

- **Checkpoint moved from a JSON file into SQLite** (a `checkpoint` table, single row enforced by `CHECK(id = 1)`). It has to live in the same database as the data it tracks progress against — a separate file can never be updated in the same transaction, which is exactly the crash window that matters (a reorg rollback that deletes rows but doesn't also move the checkpoint back). `storage.RecordBlock` and `storage.Rollback` (replacing `DeleteAbove`) update the relevant tables and the checkpoint atomically.
- `chainsync.go` (189 lines, one file) split into `block.go`, `checkpoint.go`, `reorg.go` by concern. `ProcessBlock` — which fetched full blocks and ran per-transaction signature recovery just to print to stdout — is gone entirely, replaced by `GetBlockHashes`, backed by `HeaderByNumber` instead of `BlockByNumber` (only the header was ever needed).
- Renamed the project and Go module to `surety` (`github.com/ayush18pop/surety`). "Done" was a common English word — unsearchable, and impossible to grep for within your own codebase. *Surety* is the financial term for a guarantee that a payment will be made, which is what this tool exists to provide.
- `tokenwatch.CheckTransfers` now takes `map[common.Address]TokenInfo` instead of a single hardcoded USDC address — multiple tokens can be watched in one call, and each log is decoded using the decimals/symbol of whichever contract actually emitted it, not a flat assumption.
- `finalizedBlock` is fetched once per polling tick and passed down, rather than re-fetched on every block. `tokenwatch` no longer depends on `chainsync` as a result.

### Fixed

- A failed `CheckTransfers` no longer advances the checkpoint past the block it failed on. Previously a transient RPC error printed a warning and moved on, permanently skipping that block's transfers.
- `LoadCheckpoint` no longer calls `log.Fatal` when the checkpoint file is missing — a fresh clone has no checkpoint yet, which is expected, not fatal. (Superseded by the JSON-to-SQLite move above, but the underlying principle — library code returns errors, only `main` exits — carried forward into `storage`/`server`/`webhooks` too.)
- `InsertTransfer` and `InsertBlock` use `INSERT OR REPLACE`. Re-processing a block is normal (crash mid-block, re-indexing after a reorg) and previously hit a constraint violation on the second pass.

## [0.4.0] - 2026-07-10

### Added

- `tokenwatch.TokenInfo` (`Symbol`, `Decimals`) so watched tokens carry their own metadata instead of assuming USDC's shape everywhere.
- SQLite persistence (`storage` package, `modernc.org/sqlite` — pure Go, no cgo): a `transfers` table keyed on `UNIQUE(tx_hash, log_index)`, replacing the overwrite-every-block text file.
- `chainsync.GetFinalizedBlock` — real finality via the chain's `finalized` block tag, not a guessed confirmation depth.
- Reorg detection: the checkpoint carries both a block number and hash, and each block's `ParentHash` is checked against it.
- Reorg **correction**: a `blocks` table records one hash per height, `chainsync.FindForkPoint` walks backwards to the last height where our stored hash matches the live chain, and `chainsync.HandleReorg` discards everything above that fork point and resets the checkpoint to it. Bounded below by the finalized block, which cannot reorg.
- `storage.PruneBlocksBelow` drops block hashes at or below finality, so the `blocks` table doesn't grow forever.

## [0.3.0] - 2026-07-08

### Changed

- Renamed the project and Go module to `done` (`github.com/ayush18pop/done`), matching a GitHub repo rename.
- Split the single `main.go` into packages: `chainsync` (block processing, checkpoint load/save) and `tokenwatch` (token transfer watching). `main.go` is now wiring only.

### Added

- `README.md` documenting current status, in-progress work, and the project roadmap.

## [0.2.0] - 2026-07-08

### Added

- USDC `Transfer` event watching: `eth_getLogs` scoped to the USDC contract and the `Transfer` event's topic hash, one block at a time.
- Decoding of `from`/`to` addresses from indexed log topics and transfer amount from log data, written to a local text file for inspection.

### Fixed

- Off-by-one in the block range passed to `eth_getLogs` (an inclusive `[from, to]` range is inclusive on both ends).
- RPC provider block-range limits on `eth_getLogs` (free-tier endpoints commonly cap this).

## [0.1.1] - 2026-07-08

### Changed

- Refactored block processing and transaction extraction — internal cleanup, no behavior change.

## [0.1.0] - 2026-07-08

### Added

- Initial block indexing pipeline: RPC connection via `.env`, a polling loop that walks the chain from a checkpoint, and checkpoint recovery so the indexer resumes correctly after a restart instead of reprocessing or skipping blocks.
- Handling for skipped blocks, duplicate processing, and corrupted/empty checkpoint files.
