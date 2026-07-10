# Surety

A self-hosted indexer built specifically for **stablecoin payment settlement** — not a generic "watch any contract" tool. It watches stablecoin transfers and is working toward telling you the moment a payment is truly final and safe to act on, not just "seen" on-chain.

Run it yourself, next to your own systems, pointed at your own RPC endpoint or your own Ethereum node. Nothing about this is hosted or SaaS — it's a package you install and run on your own infra, because it's watching money move for your business, and you shouldn't have to trust a third party's uptime or hand them the list of addresses you're watching.

**Status: early and honest about it.** This README is updated as things actually land, not as things are planned — the "in progress" and "planned" sections below are real, not filler.

## Why stablecoins specifically, not any token

With a stablecoin, the amount transferred *is* the value — no price oracle, no volatility between "seen" and "final." That's what makes "this payment is safe to act on" a guarantee you can actually make. The moment you allow volatile assets, you've added a second, unrelated problem (price risk, oracles) that has nothing to do with settlement correctness. So this project stays narrow on purpose.

## Why self-hosted, not a hosted API

This tool never generates or holds private keys — it *registers* addresses you already control and watches them. No custody, no key management, no liability for someone else's funds. It's a watcher/notifier, deliberately scoped, not a wallet.

---

## What's actually done

- RPC connection via `.env` config (`ETH_MAINNET_RPC`), using `ethclient`
- A polling loop that tracks the chain tip and walks forward block by block, with a checkpoint file so it resumes correctly after a restart instead of reprocessing or skipping blocks
- USDC `Transfer` event detection: `eth_getLogs` scoped to the USDC contract address and filtered to the `Transfer` event's topic hash, queried one block at a time
- Decoding: `from`/`to` addresses recovered from indexed log topics, transfer amount decoded from raw log data (both the raw integer and a human-readable USDC amount)
- Output currently written to a local text file for inspection — this is scratch/debug output, not the real persistence layer, and is expected to be replaced

**Real bugs hit and fixed along the way** (kept here on purpose — this is the actual work):
- Skipped blocks during polling
- Duplicate block processing
- Recovering correctly after a crash or restart
- Corrupted/empty checkpoint files
- An off-by-one in the block range passed to `eth_getLogs` (inclusive ranges are inclusive on both ends — an easy one to get wrong)
- `eth_getLogs` range limits enforced by RPC providers (free tiers commonly cap this at a small number of blocks per call)

## What's in progress / known limitations right now

- Everything lives in a single `main.go` — no package structure yet
- Hardcoded to one contract (USDC) and one hardcoded address — not yet generalized to watch an arbitrary set of tokens/addresses
- No real finality logic yet — a transfer is only ever "seen," there's no confirmed/final distinction, and no reorg handling. A chain reorg right now could leave stale data behind uncorrected
- No persistent database — output is a flat text file, overwritten on every run
- The original block/transaction-printing code (`processBlock`) is left over from the very first version of this project and is likely to be removed — it doesn't serve the current direction

## The plan, roughly in order

1. **Refactor into packages** — split chain interaction, checkpointing, and token-watching into separate, testable pieces instead of one file
2. **Generalize beyond USDC** — support watching an arbitrary set of token contracts/addresses, not a hardcoded one
3. **Real finality** — use the chain's actual finality signal (e.g. the `"finalized"` block tag on post-Merge Ethereum) instead of a guessed confirmation count, with a fallback for chains that don't expose it
4. **A reorg guard** — detect a chain rewrite via parent-hash continuity checks and correctly roll back and re-index affected data, instead of silently keeping bad state
5. **Real persistence** — replace the text file with a proper local database (SQLite to start, so self-hosting still needs zero external infra)
6. **A payment lifecycle** — track each transfer through `seen → confirmed → final` (or `reverted`, if a reorg drops it) instead of a single flat event log
7. **Deposit-address attribution** — a small registry mapping your own customer/account references to the addresses you're watching, so an inbound transfer can be attributed deterministically instead of guessed from amount/timing
8. **A self-reconciling ledger** — an internal double-entry record that continuously checks itself against what the chain actually says happened, not just what it recorded
9. **Reliable webhooks** — signed, retried, delivered exactly once, so "this payment is final" reliably reaches your systems
10. **Multi-chain support** — starting with a second EVM chain (Base is the current candidate, since it's a common USDC settlement chain) to prove the design isn't locked to one chain

## Explicit non-goals

- **No custody or key management.** This registers addresses, it never generates or holds private keys.
- **No compliance/sanctions screening implementation.** The pipeline will have a clearly documented hook point for where a screening check would plug in, but wiring up a real provider (Chainalysis, TRM, etc.) is out of scope for this project.
- **No support for volatile or non-stablecoin assets.** See "why stablecoins" above — this is a deliberate, permanent scope boundary, not a temporary limitation.

---

## Running it right now

```bash
# .env
ETH_MAINNET_RPC=<your RPC endpoint>
```

```bash
go run .
```

It'll start polling from the current chain tip, watch for new blocks, and write decoded USDC transfer events to `usdc_transfers.txt` in the working directory as they're found.

## License

MIT (or will be, once formally added) — self-hostable, no restrictions on running this as part of your own infrastructure.

---

Building this in public. Progress, dead ends, and the actual bugs hit along the way get posted as they happen, not just the wins.
