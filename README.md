# Surety

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Status](https://img.shields.io/badge/status-early%20%26%20building%20in%20public-yellow)](ROADMAP.md)
[![Self-Hosted](https://img.shields.io/badge/deployment-self--hosted-blue)](#why-self-hosted-not-a-hosted-api)
[![License](https://img.shields.io/badge/license-MIT-lightgrey)](LICENSE)

**The third option for stablecoin payment settlement.** Not "build reorg-safe finality tracking yourself" (tedious, easy to get subtly wrong). Not "trust a hosted API with your settlement-critical data" (fast, but now someone else's uptime is your uptime). Surety is that correctness work — done, tested, self-hosted — for teams who want to own their infra without building it from zero.

It watches stablecoin transfers, tells you the moment a payment is truly final and safe to act on (not just "seen"), and fires a signed webhook when it does. Run it next to your own systems, pointed at your own RPC endpoint. Nothing here is hosted or SaaS.

> [!NOTE]
> **Early, and building in public.** [ROADMAP.md](ROADMAP.md) tracks what's actually done vs. planned, checked off against real commits and tests — trust it over this page's pitch if the two ever disagree.

## What it does today

- Polls new blocks and decodes ERC-20 `Transfer` events for the tokens you configure
- Tracks real finality off the chain's own `finalized` RPC tag — a protocol-level guarantee, not a guessed confirmation depth
- Detects reorgs and recovers from them — walks back to the last block both chains agree on, atomically
- Fires a signed webhook (HMAC-SHA256, with retry on delivery failure) the moment a transfer reaches final status
- Persists everything to a local SQLite file; crash-safe across restarts, no reprocessing or skipped blocks
- Backed by tests that simulate reorgs on a fake chain rather than hoping mainnet never disagrees with the logic

See [ROADMAP.md](ROADMAP.md) for known gaps and what's planned next — the query API is the current focus.

## Why stablecoins specifically, not any token

With a stablecoin, the amount transferred _is_ the value — no price oracle, no volatility between "seen" and "final." That's what makes "this payment is safe to act on" a guarantee you can actually make. The moment you allow volatile assets, you've added a second, unrelated problem (price risk, oracles) that has nothing to do with settlement correctness. So this project stays narrow on purpose.

## Why self-hosted, not a hosted API

Building payment-settlement infra yourself means getting reorg detection and recovery, real finality (not confirmation-counting), and crash-safe atomic writes right — tedious, easy-to-get-subtly-wrong work, the kind that even teams processing billions of transactions across 100+ chains write engineering blog posts about because it's genuinely hard to do correctly. Using a hosted indexer (Alchemy, Moralis, etc.) skips that work, but now the thing telling you a payment is safe to act on is a third party's uptime, pricing, and roadmap.

This tool never generates or holds private keys — it _registers_ addresses you already control and watches them. No custody, no key management, no liability for someone else's funds. It's a watcher/notifier, deliberately scoped, not a wallet.

## Installation

```bash
git clone https://github.com/ayush18pop/surety.git
cd surety
go build ./...
```

Requires Go 1.26+ and access to an Ethereum RPC endpoint (your own node, or a provider).

## Quickstart

```bash
# .env
ETH_MAINNET_RPC=<your RPC endpoint>

# optional - only needed if you want payment-status notifications
WEBHOOK_URL=<url to receive signed POSTs>
WEBHOOK_SECRET=<shared secret for HMAC-SHA256 signing>
```

```bash
go run .
```

It starts polling from the current chain tip, watches for new blocks, and records decoded USDC transfers into a local `surety.db` SQLite file as they're found. Structured logs print to stdout as it runs (`log/slog`, human-readable by default).

An HTTP server comes up on `:8080` alongside it (`GET /health` for now — data query endpoints are on [ROADMAP.md](ROADMAP.md)). If `WEBHOOK_URL` is set, a signed POST fires at that URL every time a transfer reaches final status; leaving it unset disables delivery entirely rather than sending nowhere.

## Explicit non-goals

- **No custody or key management.** This registers addresses, it never generates or holds private keys.
- **No compliance/sanctions screening implementation.** The pipeline has a documented hook point for where a screening check would plug in, but wiring up a real provider (Chainalysis, TRM, etc.) is out of scope for this project.
- **No support for volatile or non-stablecoin assets.** See "why stablecoins" above — this is a deliberate, permanent scope boundary, not a temporary limitation.

## Contributing

This is early and actively changing. [ROADMAP.md](ROADMAP.md) tracks what's done and what's next; [CHANGELOG.md](CHANGELOG.md) tracks what's already shipped. Issues and PRs against anything on the roadmap are welcome.

## License

[MIT](LICENSE) — self-hostable, no restrictions on running this as part of your own infrastructure.

---

Building this in public. Progress, dead ends, and the actual bugs hit along the way get posted as they happen, not just the wins.
