# Roadmap

Living status doc. Updated as things actually ship — checked off against real commits and tests, not against intentions. If you're reading this to decide whether to build on top of something, trust this file over the README's pitch section.

## Done

- [x] Poll-based block indexing with a crash-safe checkpoint (survives restarts without reprocessing or skipping blocks)
- [x] ERC-20 `Transfer` event watching — `eth_getLogs` scoped by contract address + topic hash, one block at a time
- [x] Multi-token decoding — `tokenwatch.CheckTransfers` takes `map[common.Address]TokenInfo`, so a batch with more than one token decodes each entry with its own correct decimals, not a flat assumption
- [x] Real finality — `chainsync.GetFinalizedBlock` uses the chain's actual `"finalized"` RPC tag, not a guessed confirmation depth
- [x] Reorg detection — every block's `ParentHash` checked against the last trusted hash
- [x] Reorg correction — `chainsync.FindForkPoint` walks backwards to the last height both chains agree on; `HandleReorg` rolls back to it
- [x] SQLite persistence — `transfers`, `blocks`, and `checkpoint` all in one database, one transaction domain (`storage.RecordBlock`, `storage.Rollback` update multiple tables atomically, not as separate writes that can drift apart on a crash)
- [x] Structured, leveled logging (`log/slog`) — no more `fmt.Println`
- [x] Package split: `chainsync` (block/checkpoint/reorg), `storage` (persistence), `tokenwatch` (decoding), `main` (wiring only)
- [x] `FindForkPoint` is tested — a `HeaderFetcher` interface (just the one `HeaderByNumber` method it actually calls) lets a fake chain stand in, since real mainnet reorgs aren't triggerable on demand. Covers no-reorg, a shallow reorg, a reorg deeper than any stored history, and the already-at-finality early return.
- [x] 12 passing tests: 8 in `storage` (idempotency, reorg rollback, checkpoint atomicity), 4 in `chainsync` (the fork-point walk)

## Known gaps (small, near-term)

- [ ] **Only USDC is configured.** The engine decodes any ERC-20 `Transfer`, but `watchedTokens` in `main.go` is a hardcoded map with one entry. No config file or flag to add a token without editing source and rebuilding.
- [ ] **A reorg that doesn't advance the tip goes unnoticed.** The continuity check only runs inside `if latestNum > cp.BlockNumber`. If a reorg leaves the tip at or below the checkpoint, that block never gets re-fetched, so the check never fires.
- [ ] **Bootstrap edge case in the continuity check.** `cp.BlockHash != ""` guards the very first comparison after a fresh start, since there's no prior hash to compare against yet. Works correctly, but it's a special case sitting in the main loop rather than handled structurally.

## Planned (bigger, later)

Roughly in order, each one a real chunk of work rather than a next-session task:

1. **A payment lifecycle** — track each transfer through `seen → confirmed → final` (or `reverted`, if a reorg drops it) as real state, not a boolean computed at insert time
2. **Deposit-address attribution** — a small registry mapping customer/account references to watched addresses, so an inbound transfer is attributed deterministically instead of guessed from amount/timing
3. **A self-reconciling ledger** — an internal double-entry record that continuously checks itself against what the chain actually says happened
4. **Reliable webhooks** — signed, retried, delivered exactly once, so "this payment is final" reliably reaches the systems that need to know
5. **Multi-chain support** — a second EVM chain (Base is the current candidate) to prove the design isn't locked to one chain
6. **Config-driven token list** — resolves the "known gap" above properly, likely alongside #2

## Explicit non-goals

See the README's "Explicit non-goals" section — deliberate scope boundaries (no custody, no compliance implementation, no volatile assets), kept there since they're pitch/positioning content, not something that changes as work ships.
