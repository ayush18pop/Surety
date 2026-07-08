# Changelog

All notable changes to this project are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning follows [Semantic Versioning](https://semver.org/) — this project is pre-1.0, so behavior and structure can still change between minor versions without notice.

## [Unreleased]

### Added
- `tokenwatch.TokenInfo` (`Symbol`, `Decimals`) so watched tokens carry their own metadata instead of assuming USDC's shape everywhere.

### Changed
- `tokenwatch.CheckTransfers` now takes `map[common.Address]TokenInfo` instead of a single hardcoded USDC address — multiple tokens can be watched in one call, and each log is decoded using the decimals/symbol of whichever contract actually emitted it, not a flat assumption.
- Output formatting (amount, decimals, label) is now driven by the matched token's `TokenInfo` instead of hardcoded USDC values.
- Formatting internals switched from `builder.WriteString(fmt.Sprintf(...))` to `fmt.Fprintf(&builder, ...)` where formatting is actually happening.

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
