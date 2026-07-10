# Changelog

All notable changes to this project are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning follows [Semantic Versioning](https://semver.org/) — this project is pre-1.0, so behavior and structure can still change between minor versions without notice.

## [Unreleased]

### Added

- `tokenwatch.TokenInfo` (`Symbol`, `Decimals`) so watched tokens carry their own metadata instead of assuming USDC's shape everywhere.
- SQLite persistence (`storage` package, `modernc.org/sqlite` — pure Go, no cgo): a `transfers` table keyed on `UNIQUE(tx_hash, log_index)`, replacing the overwrite-every-block text file.
- `chainsync.GetFinalizedBlock` — real finality via the chain's `finalized` block tag, not a guessed confirmation depth.
- Reorg detection: the checkpoint is now JSON carrying both a block number and hash, and each block's `ParentHash` is checked against it.
- Reorg **correction**: a `blocks` table records one hash per height, `chainsync.FindForkPoint` walks backwards to the last height where our stored hash matches the live chain, and `chainsync.HandleReorg` discards everything above that fork point and resets the checkpoint to it. Bounded below by the finalized block, which cannot reorg.
- `storage.PruneBlocksBelow` drops block hashes at or below finality, so the `blocks` table doesn't grow forever.

### Changed

- Renamed the project and Go module to `surety` (`github.com/ayush18pop/surety`). "Done" was a common English word — unsearchable, and impossible to grep for within your own codebase. *Surety* is the financial term for a guarantee that a payment will be made, which is what this tool exists to provide.
- `tokenwatch.CheckTransfers` now takes `map[common.Address]TokenInfo` instead of a single hardcoded USDC address — multiple tokens can be watched in one call, and each log is decoded using the decimals/symbol of whichever contract actually emitted it, not a flat assumption.
- `finalizedBlock` is fetched once per polling tick and passed down, rather than re-fetched on every block. `tokenwatch` no longer depends on `chainsync` as a result.

### Fixed

- A failed `CheckTransfers` no longer advances the checkpoint past the block it failed on. Previously a transient RPC error printed a warning and moved on, permanently skipping that block's transfers.
- `LoadCheckpoint` no longer calls `log.Fatal` when the checkpoint file is missing — a fresh clone has no checkpoint yet, which is expected, not fatal.
- `InsertTransfer` and `InsertBlock` use `INSERT OR REPLACE`. Re-processing a block is normal (crash mid-block, re-indexing after a reorg) and previously hit a constraint violation on the second pass.

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
