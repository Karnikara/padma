# padma

Indexer, webhook delivery, and merchant backend for the Nervos CKB Fiber Network.

## The problem

A Fiber node answers point queries and emits a raw, undocumented change stream —
but it has no queryable history, no developer-friendly webhooks, and no merchant
tooling. Anything you'd want to build on top of a node — a dashboard, a payment
notification, a checkout flow — has to be built from scratch on top of that raw
stream.

## The answer

The gap is filled by a deliberate two-artifact split:

- **[`kanaka`](library.md)** — a reusable library: a typed RPC client, an event
  model, an ingest runner, and a webhook delivery engine. The machine.
- **[`padma`](service.md)** — an opinionated service built on `kanaka`: a
  Postgres schema, a query API, a webhook delivery outbox, and a merchant
  backend. The policy.

## The three gates

| Feature | Summary |
| ------- | ------- |
| **Indexer + Query API** | Project the node's raw events into Postgres and serve filtered, paginated history + aggregation the node can't do itself. |
| **Webhook delivery** | Subscriptions backed by a signed, at-least-once outbox with retry/backoff/dead-letter and manual replay. |
| **Merchant backend** | Orders backed by Fiber invoices, automatic payment matching, keysend refunds, per-asset settlement, reconciliation, and CSV/XLSX export. |

!!! success "Status: all gates built and verified"
    All three gates are implemented and tested. **42 tests green under `-race`**;
    `go build` / `go vet` / `gofmt` / `golangci-lint` are all clean. Every gate
    has been demoed end-to-end against a live Postgres and the HTTP API. See
    [Gates & Demo](gates.md) for the walkthrough (full report: `TEST-REPORT.md`
    in the repo).

## Quick links

- Repos: [`github.com/Karnikara/padma`](https://github.com/Karnikara/padma) ·
  [`github.com/Karnikara/kanaka`](https://github.com/Karnikara/kanaka)
- [Service quickstart](service.md) — build, migrate, and run `padma` locally.
- [Gates & Demo](gates.md) — end-to-end walkthrough of each feature gate.
