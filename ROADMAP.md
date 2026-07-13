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
- [x] Finality sweep — `storage.MarkFinalized` flips `is_final` from false to true once a transfer's block catches up to `finalizedBlock`, run once per poll tick. Before this, `is_final` was set once at decode time and never revisited — a transfer seen early would stay stamped `false` forever, even long after it genuinely finalized. Minimal seed of the full payment-lifecycle item below, not the whole state machine yet.
- [x] HTTP server — `server` package wraps `*http.Server` with real timeouts, runs in its own goroutine alongside the poll loop. `GET /health` is the only route so far; the query API below is still ahead.
- [x] Payment-status webhook — the first thing in `webhooks/`. `storage.GetUnnotifiedFinalTransfers` finds final transfers that haven't been notified about (`webhook_sent`, tracked separately from `is_final` on purpose — settlement status and delivery status are different questions); `webhooks.SendPaymentStatus` signs the payload with HMAC-SHA256 and POSTs it. A delivery failure isn't fatal and isn't marked sent, so it retries automatically next poll tick. `WEBHOOK_URL`/`WEBHOOK_SECRET` from `.env`; unset disables delivery entirely rather than sending nowhere.
- [x] 18 passing tests: 12 in `storage`, 4 in `chainsync` (the fork-point walk), 2 in `webhooks` (signature + delivery, verified against a real local HTTP receiver via `httptest`)

## Known gaps (small, near-term)

- [ ] **Only USDC is configured.** The engine decodes any ERC-20 `Transfer`, but `watchedTokens` in `main.go` is a hardcoded map with one entry. No config file or flag to add a token without editing source and rebuilding.
- [ ] **A reorg that doesn't advance the tip goes unnoticed.** The continuity check only runs inside `if latestNum > cp.BlockNumber`. If a reorg leaves the tip at or below the checkpoint, that block never gets re-fetched, so the check never fires.
- [ ] **Bootstrap edge case in the continuity check.** `cp.BlockHash != ""` guards the very first comparison after a fresh start, since there's no prior hash to compare against yet. Works correctly, but it's a special case sitting in the main loop rather than handled structurally.
- [ ] **Webhook delivery is single-destination and best-effort.** One URL from config, no registration system, no exponential backoff/dead-lettering — just "retry next tick forever." Fine for v1, not yet what's described in the "reliable webhooks" planned item below.

## Planned (bigger, later)

Roughly in order, each one a real chunk of work rather than a next-session task:

1. **`GET /transfers` query endpoint** — filters (address, token, final) on top of the HTTP server that already exists. Right now the only way to see indexed data is opening the SQLite file directly or waiting for a webhook.
2. **Real webhook delivery guarantees** — exponential backoff, dead-lettering, and multi-destination registration instead of one config URL, resolving the "known gap" above.
3. **A full payment lifecycle** — track each transfer through `seen → confirmed → final` (or `reverted`, if a reorg drops it) as real tracked state, not a boolean flipped by a periodic sweep
4. **Deposit-address attribution** — a small registry mapping customer/account references to watched addresses, so an inbound transfer is attributed deterministically instead of guessed from amount/timing
5. **A self-reconciling ledger** — an internal double-entry record that continuously checks itself against what the chain actually says happened
6. **Multi-chain support** — a second EVM chain (Base is the current candidate) to prove the design isn't locked to one chain
7. **Config-driven token list** — resolves the "known gap" above properly, likely alongside #4

## Explicit non-goals

See the README's "Explicit non-goals" section — deliberate scope boundaries (no custody, no compliance implementation, no volatile assets), kept there since they're pitch/positioning content, not something that changes as work ships.
