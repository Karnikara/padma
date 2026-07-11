# API Reference

`padma` serves one HTTP surface (`app api`) covering three feature areas —
query, webhooks, and merchant — plus an unauthenticated health check. All
routes below are relative to the base URL (default `http://localhost:8080`).

`GET /healthz` → `200 {"status":"ok"}`. Not part of the `/api` group; used for
liveness checks.

## Query

Read-only projections of the node's event stream: filtered, paginated
payments/channels/events history and aggregation the node itself can't do.

| Method & path | Description |
| ------------- | ------------ |
| `GET /api/payments` | List payments, newest first, cursor-paginated |
| `GET /api/payments/{payment_hash}` | Get a single payment by hash |
| `GET /api/channels` | List channels, cursor-paginated |
| `GET /api/channels/{channel_id}` | Get a single channel by id |
| `GET /api/events` | List raw semantic events, cursor-paginated |
| `GET /api/stats/payments` | Aggregate payment counts/totals by asset |

Query parameters (all optional unless noted):

| Endpoint | Params |
| -------- | ------ |
| `GET /api/payments` | `direction`, `status`, `asset`, `peer`, `from`, `to`, `cursor`, `limit` (default 50) |
| `GET /api/channels` | `state`, `peer`, `cursor`, `limit` (default 50) |
| `GET /api/events` | `type`, `from`, `to`, `cursor`, `limit` (default 50) |
| `GET /api/stats/payments` | `from`, `to` |

`cursor` is an opaque, stable pagination token returned in the previous
response — pass it back unmodified to get the next page. `from`/`to` bound a
time range.

## Webhooks

Subscriptions backed by a signed, at-least-once delivery outbox with
retry/backoff, dead-letter, and manual replay.

| Method & path | Description |
| ------------- | ------------ |
| `POST /api/webhooks` | Create a subscription (`url`, `event_types`) — returns endpoint id + secret |
| `GET /api/webhooks` | List subscriptions |
| `DELETE /api/webhooks/{id}` | Remove a subscription |
| `POST /api/webhooks/{id}/test` | Send a synthetic test delivery to the endpoint |
| `POST /api/webhooks/{id}/replay/{delivery_id}` | Re-queue a specific past delivery |

Deliveries are signed with an HMAC secret (returned once, at creation) and
carry `X-Fiber-Event`, `X-Fiber-Delivery-Id`, `X-Fiber-Timestamp`, and
`X-Fiber-Signature` headers, verifiable with `kanaka`'s `webhook.Verify`.

## Merchant

Orders backed by Fiber invoices, automatic payment matching, keysend refunds,
per-asset settlement, reconciliation, and accounting export.

| Method & path | Description |
| ------------- | ------------ |
| `POST /api/merchant/orders` | Create an order (idempotent on `external_order_id`) — mints a Fiber invoice |
| `GET /api/merchant/orders/{id}` | Get an order by id |
| `POST /api/merchant/orders/{id}/refund` | Send a keysend refund for a paid order |
| `GET /api/merchant/settlements` | Per-asset gross/fees/net for a period |
| `GET /api/merchant/reconciliation` | Match orders against indexed payments; report discrepancies |
| `GET /api/merchant/export` | Export orders as CSV or XLSX (`?format=csv\|xlsx`) |

All merchant endpoints require:

```
Authorization: Bearer <api-key>
```

Provision a merchant and its one-time API key with:

```bash
./app merchant-key <name>
```

The plaintext key is printed once and never stored — only its hash is
persisted. Requests without a valid key are rejected with `401`.

!!! note "Amounts are `0x`-hex on the wire"
    Every `Amount` field (payment totals, settlement gross/fees/net, stats
    totals) is JSON-encoded as a `0x`-prefixed hex string, e.g. `"0x2bc"` —
    never a JSON number. This avoids float precision loss on large,
    multi-asset values. Internally amounts are stored as exact `NUMERIC`, not
    the hex string.
