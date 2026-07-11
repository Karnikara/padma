# Library: `kanaka`

[`kanaka`](https://github.com/Karnikara/kanaka) is the reusable half of the
kanaka/padma split — a Go module that turns a Fiber node's raw RPC surface and
undocumented change stream into typed, composable building blocks. It has no
database, no HTTP server, and no business rules: no schema to migrate, no
policy to enforce. Just the machine that [`padma`](service.md) is built on.

Anyone building on Fiber — a dashboard, a notification service, a checkout
flow — can import `kanaka` directly without adopting `padma`'s Postgres schema
or merchant semantics.

## Packages

| Package | Provides |
| ------- | -------- |
| `fibertypes` | `Amount` / `Asset` primitives — safe arithmetic and formatting over Fiber's multi-asset values |
| `rpc` | a typed JSON-RPC client over the node's RPC surface (invoices, refunds, queries) |
| `events` | a semantic event model plus a `Normalizer` that turns raw store changes into typed events |
| `ingest` | the `Source` / `Checkpoint` interfaces and a `Runner` that drives the event stream to completion |
| `webhook` | `Sign` / `Verify` for payload authenticity, plus a retrying `Dispatcher` with backoff and dead-lettering |

## Quickstart

Verify an incoming webhook (anti-replay window included):

```go
import "github.com/karnikara/kanaka/webhook"

err := webhook.Verify(secret, r.Header, body, webhook.VerifyOpts{
    Tolerance: 5 * time.Minute,
})
if err != nil {
    http.Error(w, "invalid signature", http.StatusUnauthorized)
    return
}
```

Create an invoice through the typed RPC client:

```go
import (
    "github.com/karnikara/kanaka/fibertypes"
    "github.com/karnikara/kanaka/rpc"
)

client := rpc.NewClient(endpoint, rpc.WithAuthToken(token))

amount, _ := fibertypes.ParseAmount("0x3e8") // 1000 shannons
invoice, err := client.NewInvoice(ctx, rpc.NewInvoiceParams{
    Amount:      amount,
    Description: "order #1042",
})
// invoice.PaymentHash, invoice.Address
```

!!! info "Separate repo, separate module"
    `kanaka` lives in its own repository and Go module
    (`github.com/karnikara/kanaka`). `padma` imports it like any external
    dependency — there is no shared source tree or build step between the two.
