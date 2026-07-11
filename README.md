# padma

**Indexer + webhook delivery + merchant backend for the [Nervos CKB Fiber Network](https://github.com/nervosnetwork/fiber).**

A Fiber node answers point queries and emits a raw, undocumented change stream —
but it has no queryable history, no developer-friendly webhooks, and no merchant
tooling. `padma` is the opinionated service that fills that gap: it imports the
[`kanaka`](../kanaka) library for the plumbing and adds the **policy** — a
Postgres schema, a query API, a webhook delivery outbox, and a merchant backend
(orders, refunds, settlement, reconciliation, accounting exports).

> **Status: early / alpha.** Built for the "Gone in 60ms" hackathon (submission
> category 3 — Merchant/Liquidity/Multi-Asset). All three feature gates are
> implemented and tested; see [Verification](#verification). The live ingest
> source is the one deferred piece — see [Ingest](#ingest-fake-vs-live).

`padma` is the **service** half of a deliberate two-artifact split. `kanaka` is
the machine (typed RPC client, event model, ingest runner, webhook engine);
`padma` supplies the database, business rules, and HTTP routes. See
[Design](#design-the-libraryservice-split).

## What it does

| # | Feature | Endpoints |
| - | ------- | --------- |
| **#11** | **Indexer + Query API** — project the node's events into Postgres; serve filtered, paginated history + aggregation the node can't do | `GET /v1/payments`, `/channels`, `/events`, `/stats/payments` |
| **#10** | **Webhook delivery** — subscriptions, a signed at-least-once outbox with retry/backoff/dead-letter, manual replay | `POST/GET/DELETE /v1/webhooks`, `/test`, `/replay/{id}` |
| **#3** | **Merchant backend** — orders backed by Fiber invoices, automatic payment matching, keysend refunds, per-asset settlement, reconciliation, CSV/XLSX export | `/v1/merchant/orders`, `/refund`, `/settlements`, `/reconciliation`, `/export` |

## Requirements

- Go 1.25+
- PostgreSQL 14+
- A Fiber node's RPC endpoint (for merchant invoice/refund calls)

## Quickstart

```bash
# 1. a database
export DATABASE_URL="postgres://user:pass@localhost:5432/padma?sslmode=disable"

# 2. build
go build -o app ./cmd/app

# 3. create the schema
./app migrate

# 4. run the pieces (separate processes, or one host each)
./app indexer      # project node events into Postgres
./app api          # serve the query / webhook / merchant HTTP API
./app dispatcher   # deliver queued webhooks
```

`app` is a single multi-mode binary. Subcommands:

| Command | Does |
| ------- | ---- |
| `app migrate` | apply database migrations, then exit |
| `app indexer` | run the ingest runner + projector (#11) |
| `app api` | serve the query/webhook/merchant HTTP API |
| `app dispatcher` | run the webhook delivery loop (#10) |
| `app merchant-key <name>` | provision a merchant, print its id + API key once |

## Configuration

All configuration is environment-based (see `internal/platform/config`):

| Variable | Default | Meaning |
| -------- | ------- | ------- |
| `DATABASE_URL` | *(required)* | Postgres DSN |
| `FIBER_RPC_ENDPOINT` | — | Fiber node RPC (needed by `api` for merchant flows) |
| `FIBER_BISCUIT_TOKEN` | — | optional bearer token for the node RPC |
| `INGEST_SOURCE` | `fake` | `fake` \| `polling` \| `pubsub` (see [Ingest](#ingest-fake-vs-live)) |
| `WEBHOOK_MAX_ATTEMPTS` | `8` | dead-letter after this many failed deliveries |
| `WEBHOOK_WORKERS` | `4` | concurrent dispatcher workers |
| `HTTP_ADDR` | `:8080` | API listen address |

## Architecture

Clean Architecture; imports point inward (`cmd → features → domain`, everyone
may use `platform/*`).

```
cmd/app/                     single multi-mode binary
internal/
  platform/{config,db,httpx,auth,id,fiber}   infra: pool, migrations, Amount↔NUMERIC, auth, seam
  indexer/                   #11 transactional-outbox projector + PostgresCheckpoint
  query/                     #11 read repos + REST handlers (cursor pagination, filters, stats)
  webhook/                   #10 PostgresDeliveryStore + subscription CRUD + dispatcher wiring
  merchant/                  #3  orders / refunds / settlement / reconciliation / export
  apiserver/                 chi router assembling the HTTP surface
migrations/                  goose SQL (embedded), applied by `app migrate`
```

Two load-bearing details worth knowing:

- **Transactional outbox.** For each raw change the projector writes the
  append-only `events` row, updates the read-model projections, enqueues matching
  `webhook_deliveries`, and advances the checkpoint — all in **one transaction**.
  A crash replays the whole change; deterministic ids make it idempotent.
- **SKIP-LOCKED delivery.** `PostgresDeliveryStore.ClaimDue` uses
  `SELECT … FOR UPDATE SKIP LOCKED`, so any number of dispatcher workers claim
  disjoint batches with no double-send. Attempts are counted at `MarkFailed`/
  `MarkDead` time to stay aligned with the library's dead-letter logic.

## Design: the library/service split

`padma` never re-implements the plumbing it gets from `kanaka`:

| From `kanaka` | `padma` uses it for |
| ------------- | ------------------- |
| `rpc.Client` | create invoices, send refunds |
| `ingest.Runner` + `Source` | drive the event stream |
| `events.Normalizer` + `Event` | raw store change → semantic event |
| `webhook.Sign` / `Verify` | sign outbound deliveries |
| `webhook.Dispatcher` | the retry/backoff/dead-letter engine |

And it implements the interfaces `kanaka` leaves open:

| Library interface | `padma` implementation |
| ----------------- | ---------------------- |
| `ingest.Checkpoint` | `indexer` → `ingest_checkpoint` table |
| `webhook.Store` | `PostgresDeliveryStore` → `webhook_deliveries` outbox (SKIP LOCKED) |

> Library = machine + interfaces. Service = Postgres + business logic. No
> duplication. `kanaka` and `padma` are separate concerns and will live in
> separate repositories.

## Ingest: fake vs. live

The indexer runs the library's `ingest.Runner`, which needs a concrete `Source`
and `Normalizer`. The real ones — a polling/pubsub source and the mapping from
the node's raw `store_changes` payload to semantic events — are **deferred in the
`kanaka` library** until that undocumented payload is captured on a live node.

Everything downstream is built and tested against a fake source
(`INGEST_SOURCE=fake`) that replays seeded events from `SEED_FILE`
(see `testdata/seed/`). Swapping in the real source is a single wiring change in
`cmd/app`; `INGEST_SOURCE=polling|pubsub` currently returns a clear
"spike pending" error.

## Verification

- **42 test cases**, run with the race detector, all passing.
- `go build` / `go vet` / `gofmt` / `golangci-lint` all clean (0 issues).
- Every gate verified end-to-end against a live Postgres and the HTTP API.

Full results, coverage, and the end-to-end command/response log are in
[`../docs/TEST-REPORT.md`](../docs/TEST-REPORT.md).

## Development

```bash
make check          # fmt-check + vet + lint + fast unit tests (no DB)
make ci             # build + vet + lint + test-race

# DB integration tests need a Postgres; each package gets its own throwaway db:
docker run -d --rm --name padma-pg \
  -e POSTGRES_PASSWORD=padma -e POSTGRES_DB=padma -p 55432:5432 postgres:16-alpine
export DATABASE_URL="postgres://postgres:padma@127.0.0.1:55432/padma?sslmode=disable"
go test -race ./...
```

Built test-first (red → green → refactor). New behavior comes with a failing test
first — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Contributing

Issues and PRs welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) and abide by
our [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](../kanaka/LICENSE) © savioruz
