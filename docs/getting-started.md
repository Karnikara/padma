# Getting Started

Get `padma` running locally and see real data in a couple of minutes. This is the
short path; for the full config reference, subcommands, and modes see
[Service (padma)](service.md).

## Prerequisites

- **Go 1.25+** — to build the `app` binary
- **PostgreSQL 14+** — the read model + outbox live here
- **Docker** *(optional)* — for the one-command stack below
- **A Fiber node** *(recommended)* — for the full experience (merchant invoices +
  refunds). The easy way is [`lazy-fnn`](https://github.com/Karnikara/lazy-fnn),
  one command to a testnet node. Queries, indexing, and webhooks work without one.
  See [Running a Fiber node](running-a-node.md).

## Clone & install

`padma` builds against the [`kanaka`](library.md) library via a local
`replace => ../kanaka`, so clone both as siblings:

```bash
git clone https://github.com/Karnikara/kanaka
git clone https://github.com/Karnikara/padma
cd padma
go build -o app ./cmd/app
```

## Quick config

Copy the env template and tweak as needed:

```bash
cp .env.example .env
```

Everything is environment-driven. The only required value is `DATABASE_URL`; the
default ingest source is the seeded `fake` source, so you can run the whole stack
with no Fiber node. The values you'll usually touch in `.env`:

```bash
DATABASE_URL=postgres://padma:padma@localhost:5432/padma?sslmode=disable
INGEST_SOURCE=fake            # replays seeded events (default)
# FIBER_RPC_ENDPOINT=...      # only for merchant invoice/refund flows
```

`docker compose` reads `.env` automatically. For a manual run, load it into your
shell — `export $(grep -v '^#' .env | xargs)` — or export the vars directly.

Full variable table: [Service → Configuration](service.md#configuration).

## Run & verify

### Fastest — one command (Docker)

Brings up Postgres, applies migrations, seeds the indexer, and serves the API:

```bash
docker compose up --build
```

Then confirm data is flowing:

```bash
curl localhost:8080/healthz
# {"status":"ok"}

curl localhost:8080/api/stats/payments
# {"buckets":[{"asset":"CKB","count":2,"total":"0x2bc"}],"group_by":"asset"}

curl localhost:8080/api/events
# the seeded events the indexer just projected
```

### Manual — run the pieces yourself

```bash
./app migrate       # create the schema
./app indexer       # project seeded node events into Postgres (exits when done)
./app api           # serve the query / webhook / merchant HTTP API
# in another shell:
curl localhost:8080/api/payments
```

Seeing the `/api/stats/payments` bucket (or events from `/api/events`) means the
indexer ran and the query API is live. 🎉

## Next steps

- [Service (padma)](service.md) — every subcommand, config variable, and the
  ingest caveat
- [API Reference](api.md) — the full REST surface
- [Running a Fiber node](running-a-node.md) — wire up merchant invoice/refund flows
- [Gates & Demo](gates.md) — the end-to-end walkthrough of every feature
