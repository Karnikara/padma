# Service: `padma`

[`padma`](https://github.com/Karnikara/padma) is the opinionated service half
of the `karnikara` split. It imports [`kanaka`](library.md) for the plumbing
and adds the policy: a Postgres schema, a query API, a webhook delivery
outbox, and a merchant backend (orders, refunds, settlement, reconciliation,
accounting exports).

## What it does

| Feature | Endpoints |
| ------- | --------- |
| **Indexer + Query API** — project the node's events into Postgres; serve filtered, paginated history + aggregation the node can't do | `GET /api/payments`, `/channels`, `/events`, `/stats/payments` |
| **Webhook delivery** — subscriptions, a signed at-least-once outbox with retry/backoff/dead-letter, manual replay | `POST/GET/DELETE /api/webhooks`, `/test`, `/replay/{id}` |
| **Merchant backend** — orders backed by Fiber invoices, automatic payment matching, keysend refunds, per-asset settlement, reconciliation, CSV/XLSX export | `/api/merchant/orders`, `/refund`, `/settlements`, `/reconciliation`, `/export` |

## Subcommands

`app` is a single multi-mode binary:

| Command | Does |
| ------- | ---- |
| `app migrate` | apply database migrations, then exit |
| `app indexer` | run the ingest runner + projector |
| `app api` | serve the query/webhook/merchant HTTP API |
| `app dispatcher` | run the webhook delivery loop |
| `app merchant-key <name>` | provision a merchant, print its id + API key once |

## Configuration

All configuration is environment-based (see `internal/platform/config`):

| Variable | Default | Meaning |
| -------- | ------- | ------- |
| `DATABASE_URL` | *(required)* | Postgres DSN |
| `FIBER_RPC_ENDPOINT` | — | Fiber node RPC (needed by `api` for merchant flows) |
| `FIBER_BISCUIT_TOKEN` | — | optional bearer token for the node RPC |
| `INGEST_SOURCE` | `fake` | `fake` \| `polling` \| `pubsub` |
| `WEBHOOK_MAX_ATTEMPTS` | `8` | dead-letter after this many failed deliveries |
| `WEBHOOK_WORKERS` | `4` | concurrent dispatcher workers |
| `HTTP_ADDR` | `:8080` | API listen address |

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

!!! warning "Live ingest (`store_changes`) is deferred"
    The indexer runs on `kanaka`'s `ingest.Runner`, which needs a concrete
    `Source` and `Normalizer`. The real ones — a polling/pubsub source and the
    mapping from the node's raw, undocumented `store_changes` payload to
    semantic events — are deferred in the `kanaka` library until that payload
    is captured on a live node.

    Everything today runs against `INGEST_SOURCE=fake`, which replays seeded
    events and exercises every gate end-to-end. Setting
    `INGEST_SOURCE=polling` or `INGEST_SOURCE=pubsub` currently returns a
    clear "spike pending" error rather than silently doing nothing — swapping
    in the real source is a single wiring change in `cmd/app` once it lands.
