# Running a Fiber node

padma's **indexer, query API, and webhooks need no node** — they run off the
seeded/ingested event stream. Only the **merchant flows** (creating invoices,
sending refunds) call a Fiber node's RPC.

There is **no public/shared Fiber RPC**: the RPC (`new_invoice`, `send_payment`)
moves real funds, so every node keeps it private (bound to localhost) — the same
model as a Lightning node. So you run your own node on testnet and point
`FIBER_RPC_ENDPOINT` at it.

!!! note "Two different RPCs"
    **CKB** (layer 1) *does* have a public testnet RPC (`https://testnet.ckbapp.dev/`).
    The **Fiber node** RPC (`:8227`) is the private one you run. Your Fiber node
    talks to CKB over the public CKB RPC.

## Testnet resources

| Resource | URL |
| -------- | --- |
| CKB testnet RPC | `https://testnet.ckbapp.dev/` |
| CKB faucet (fund your node) | <https://faucet.nervos.org> |
| RUSD stablecoin faucet (multi-asset) | <https://testnet0815.stablepp.xyz/stablecoin> |
| CKB testnet explorer | <https://explorer.nervos.org/aggron> |
| Fiber network dashboard | <https://dashboard.fiber.channel/nodes> |
| P2P bootnodes + pubkeys | <https://www.fiber.world/docs/quick-start/network-resources> |
| Node binary + Docker guide | <https://github.com/nervosnetwork/fiber> |

## 1. Start the node (Docker)

```bash
mkdir -p fiber-node/ckb
# place your testnet CKB private key at: fiber-node/ckb/key
# add fiber-node/config.yml (below)

docker run -d --name fiber \
  -e FIBER_SECRET_KEY_PASSWORD='your-password' \
  -e RUST_LOG=info \
  -v "$PWD/fiber-node:/fiber" \
  -p 8227:8227 -p 8228:8228 \
  nervos/fiber:latest
```

Minimal testnet `config.yml` — note `rpc.listening_addr` on `0.0.0.0` so padma
(in another container) can reach it:

```yaml
rpc:
  listening_addr: "0.0.0.0:8227"
fiber:
  listening_addr: "/ip4/0.0.0.0/tcp/8228"
  announced_addrs: []
  chain: testnet
ckb:
  rpc_url: "https://testnet.ckbapp.dev/"
```

!!! warning "Keep the RPC private"
    `0.0.0.0:8227` exposes a funds-controlling API. Bind it only to a trusted
    private network — never publish `8227` to the public internet.

## 2. Fund it, peer, and open a channel

For **refunds** (`send_payment`) to work, the node needs funds and an open
channel:

1. **Fund** the node's CKB address from the [faucet](https://faucet.nervos.org).
2. **Peer** with a testnet bootnode — `connect_peer <pubkey>` (pubkeys on the
   [network resources](https://www.fiber.world/docs/quick-start/network-resources)
   page; the node resolves the address via gossip).
3. **Open a channel** to a peer so payments can route.

Exact RPC calls and `fnn-cli` usage are in the
[Fiber docs](https://github.com/nervosnetwork/fiber). Invoices (`new_invoice`)
work as soon as the node is up; refunds need the funded channel above.

## 3. Point padma at it

```bash
FIBER_RPC_ENDPOINT=http://127.0.0.1:8227          # running the app on the host
# from docker-compose (node on the host):
FIBER_RPC_ENDPOINT=http://host.docker.internal:8227
```

That's the only wiring padma needs — everything else (queries, indexing,
webhooks) already works without a node.
