# Gates & Demo

Each of the three feature gates was demoed end-to-end against a live
Postgres and the built `app` binary — fresh schema, seeded fake ingest, real
HTTP requests. What follows is the observed command → result log for each
gate (adapted from the full `TEST-REPORT.md` §5 in the repo root; `/api` is
the current route prefix — the original run predates the `/v1` → `/api`
rename).

## Gate A — indexer + query API

Migrations create the schema, the indexer projects seeded events into it with
a resumable checkpoint, and the query API serves filtered + aggregated
results back out.

```
app migrate                          → migrations applied; 14 tables created
app indexer  (INGEST_SOURCE=fake)    → ingests 4 seeded events; checkpoint=4
app indexer  (replay)                → events=4 (idempotent, no duplicates)

GET /healthz                         → 200 {"status":"ok"}
GET /api/payments                    → 200; 2 payments, newest 0xhashD amount 0x1f4
GET /api/stats/payments              → 200; {asset:CKB, count:2, total:0x2bc}
GET /api/events?type=payment.received → 200; [evt_seed_recv_d, evt_seed_recv_b]
```

Re-running the indexer against the same seed does not duplicate rows or
double-fire deliveries — the checkpoint and the transactional outbox make
replay safe.

## Gate B — webhook delivery

A subscription is created, a matching event is projected, and the dispatcher
delivers a signed payload — with retry/backoff and dead-letter on repeated
failure.

```
POST /api/webhooks {url,event_types}  → 201; endpoint id + 64-char secret
indexer fan-out                       → matching invoice.paid enqueues a delivery
app dispatcher                        → receiver gets X-Fiber-Event: invoice.paid
                                         + valid sha256= signature; delivery → succeeded
(failure path)                        → retries with backoff, then dead-letter
```

The signed-delivery and dead-letter paths are also covered directly by the
dispatcher unit tests (`TestDispatcherDeliversAndSignsSuccessfully`,
`TestDispatcherDeadLettersAfterMaxAttempts`).

## Gate C — merchant backend

An order is created against a freshly provisioned merchant, auto-matched to
payment, refunded, settled, reconciled, and exported — all through the HTTP
API.

```
app merchant-key <name>                          → merchant_id + api_key (shown once)
POST /api/merchant/orders (Bearer key)           → 201; order pending, invoice 0xdemoinvoice
POST /api/merchant/orders (repeat)               → same order id (idempotent)
indexer invoice.paid for order's invoice         → order auto-matched → paid (+paid_at)
POST /api/merchant/orders/{id}/refund            → 200; keysend refund recorded, order → refunded
GET  /api/merchant/settlements?period=2026-07    → 200; {asset:CKB, gross:0xc8, fees:0x0, net:0xc8}
GET  /api/merchant/reconciliation?period=2026-07 → 200; matched:1, unmatched:0
GET  /api/merchant/export?format=csv             → 200; header + row
GET  /api/merchant/export?format=xlsx            → 200; valid "Microsoft Excel 2007+" (PK magic, ~6 KB)
GET  /api/merchant/orders/x   (no api key)       → 401 (auth enforced)
```

## Definition-of-done

- **Gate A:** indexer resumes from checkpoint; query API returns filtered +
  aggregated data. ✅
- **Gate B:** subscribe → semantic event delivered to endpoint; retry/backoff
  proven; signature verifiable via `webhook.Verify`. ✅
- **Gate C:** order created → paid → auto-matched; settlement + reconciliation
  + export work; refund proven. ✅

Full test suite (42 cases, race detector, all packages) and static-analysis
results live in `TEST-REPORT.md` at the repo root.
