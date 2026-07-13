# Surety

A self-hosted indexer built specifically for **stablecoin payment settlement** — not a generic "watch any contract" tool. It watches stablecoin transfers and is working toward telling you the moment a payment is truly final and safe to act on, not just "seen" on-chain.

Run it yourself, next to your own systems, pointed at your own RPC endpoint or your own Ethereum node. Nothing about this is hosted or SaaS — it's a package you install and run on your own infra, because it's watching money move for your business, and you shouldn't have to trust a third party's uptime or hand them the list of addresses you're watching.

**Status: early and honest about it.** See [ROADMAP.md](ROADMAP.md) for what's actually done, what's a known gap, and what's still ahead — that file gets updated as things ship, this README doesn't try to track it in parallel.

## Why stablecoins specifically, not any token

With a stablecoin, the amount transferred *is* the value — no price oracle, no volatility between "seen" and "final." That's what makes "this payment is safe to act on" a guarantee you can actually make. The moment you allow volatile assets, you've added a second, unrelated problem (price risk, oracles) that has nothing to do with settlement correctness. So this project stays narrow on purpose.

## Why self-hosted, not a hosted API

This tool never generates or holds private keys — it *registers* addresses you already control and watches them. No custody, no key management, no liability for someone else's funds. It's a watcher/notifier, deliberately scoped, not a wallet.

---

## Explicit non-goals

- **No custody or key management.** This registers addresses, it never generates or holds private keys.
- **No compliance/sanctions screening implementation.** The pipeline will have a clearly documented hook point for where a screening check would plug in, but wiring up a real provider (Chainalysis, TRM, etc.) is out of scope for this project.
- **No support for volatile or non-stablecoin assets.** See "why stablecoins" above — this is a deliberate, permanent scope boundary, not a temporary limitation.

---

## Running it right now

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

It'll start polling from the current chain tip, watch for new blocks, and record decoded USDC transfers into a local `surety.db` SQLite file as they're found. Structured logs print to stdout as it runs (`log/slog`, human-readable by default).

An HTTP server also comes up on `:8080` alongside it (`GET /health` for now — the data query endpoints are still on [ROADMAP.md](ROADMAP.md)). If `WEBHOOK_URL` is set, a signed POST fires at that URL every time a transfer reaches final status; leaving it unset disables delivery entirely rather than sending nowhere.

## License

MIT (or will be, once formally added) — self-hostable, no restrictions on running this as part of your own infrastructure.

---

Building this in public. Progress, dead ends, and the actual bugs hit along the way get posted as they happen, not just the wins.
