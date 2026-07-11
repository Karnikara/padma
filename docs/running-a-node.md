# Running a Fiber node

Running a Fiber node is **recommended** — it unlocks padma's merchant flows
(creating invoices, sending refunds) and is the full setup. The [easy path](#easy-path-lazy-fnn)
below spins one up in a single command. (padma's indexer, query API, and webhooks
work without a node, off the ingested/seeded event stream — so you can start
without one and add it when you need merchant flows.)

There is **no public/shared Fiber RPC**: the RPC (`new_invoice`, `send_payment`)
moves real funds, so every node keeps it private (bound to `127.0.0.1:8227` by
default) — the same model as a Lightning node. So you run your own node and point
`FIBER_RPC_ENDPOINT` at it.

!!! note "Two different RPCs"
    **CKB** (layer 1) has a public testnet RPC (`https://testnet.ckbapp.dev/`).
    The **Fiber node** RPC (`:8227`) is the private one you run — it talks to CKB
    over the public CKB RPC.

## Easy path — `lazy-fnn`

For a dev/testnet node in one command, use
[`lazy-fnn`](https://github.com/Karnikara/lazy-fnn) — it downloads `ckb-cli` +
`fnn`, generates a key, writes a testnet config, and starts the node with RPC on
`127.0.0.1:8227`:

```bash
curl -fsSL https://raw.githubusercontent.com/Karnikara/lazy-fnn/main/install.sh | bash
```

Then point padma at it:

```bash
FIBER_RPC_ENDPOINT=http://127.0.0.1:8227           # padma on the host
FIBER_RPC_ENDPOINT=http://host.docker.internal:8227 # padma in docker-compose
```

macOS + Linux (amd64/arm64), no sudo. For merchant **refunds**, fund the node's
address from the [faucet](https://faucet.nervos.org) and open a channel. Prefer to
set it up by hand? Follow the official docs below.

## Run the node — official docs

Follow the official quick-start; it stays current with node releases:

- **Run a node:** <https://www.fiber.world/docs/quick-start/run-a-node>
  - **Native (`fnn`, Rust — servers/production):** <https://www.fiber.world/docs/quick-start/run-a-node/rust>
  - **WASM (`fiber-js` — browser):** <https://www.fiber.world/docs/quick-start/run-a-node/fiberjs>

In short, per those docs: generate/import a **CKB private key**, copy
`config/testnet/config.yml`, set `FIBER_SECRET_KEY_PASSWORD`, and start the node
(it exposes JSON-RPC on `127.0.0.1:8227`, testnet selected via `fiber.chain`).

## Testnet resources

| Resource | URL |
| -------- | --- |
| CKB testnet RPC | `https://testnet.ckbapp.dev/` |
| CKB faucet (fund your node) | <https://faucet.nervos.org> |
| RUSD stablecoin faucet (multi-asset) | <https://testnet0815.stablepp.xyz/stablecoin> |
| CKB testnet explorer | <https://explorer.nervos.org/aggron> |
| Fiber network dashboard | <https://dashboard.fiber.channel/nodes> |
| P2P bootnodes + pubkeys | <https://www.fiber.world/docs/quick-start/network-resources> |

## Wiring it to padma

Two padma-specific points on top of the official setup:

1. **Reachability.** The node binds its RPC to `127.0.0.1:8227` by default. If
   padma runs on the **same host**, that's fine. If padma runs in a **container /
   pod**, set the node's `rpc.listening_addr` to `0.0.0.0:8227` so it's reachable
   over the network — but keep it on a trusted private network, never the public
   internet (it's a funds-controlling API).

2. **Endpoint.** Point padma at the node:

    ```bash
    FIBER_RPC_ENDPOINT=http://127.0.0.1:8227           # app on the host
    FIBER_RPC_ENDPOINT=http://host.docker.internal:8227 # from docker-compose
    ```

**Funding.** Invoices (`new_invoice`) work as soon as the node is up. **Refunds**
(`send_payment`) need a funded testnet wallet with an open channel — fund the
node's address from the [faucet](https://faucet.nervos.org) and open a channel
per the official docs.
