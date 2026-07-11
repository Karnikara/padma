package indexer_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/kanaka/events"
	"github.com/karnikara/kanaka/fibertypes"
	"github.com/karnikara/kanaka/ingest"

	"github.com/karnikara/padma/internal/indexer"
	"github.com/karnikara/padma/internal/platform/db/dbtest"
	"github.com/karnikara/padma/internal/platform/fiber"
)

func mustAmount(t *testing.T, hex string) fibertypes.Amount {
	t.Helper()
	a, err := fibertypes.ParseAmount(hex)
	if err != nil {
		t.Fatalf("ParseAmount(%q): %v", hex, err)
	}
	return a
}

func rawChange(t *testing.T, seq int64, e events.Event) events.RawChange {
	t.Helper()
	body, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return events.RawChange{Seq: seq, Body: body}
}

func mustData(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	return b
}

// seedChanges returns one raw change per event type we project.
func seedChanges(t *testing.T) []events.RawChange {
	ts := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	return []events.RawChange{
		rawChange(t, 10, events.Event{
			ID: "evt_paid", Type: events.InvoicePaid, CreatedAt: ts, FiberRef: "0xhash1",
			Data: mustData(t, events.InvoicePaidData{
				PaymentHash: "0xhash1", Amount: mustAmount(t, "0x64"), Asset: "CKB", SettledAt: ts,
			}),
		}),
		rawChange(t, 11, events.Event{
			ID: "evt_recv", Type: events.PaymentReceived, CreatedAt: ts, FiberRef: "0xhash2",
			Data: mustData(t, events.PaymentReceivedData{
				PaymentHash: "0xhash2", Amount: mustAmount(t, "0xc8"), Asset: "CKB", PayerPubkey: "0xpayer",
			}),
		}),
		rawChange(t, 12, events.Event{
			ID: "evt_chan", Type: events.ChannelOpened, CreatedAt: ts, FiberRef: "0xchan1",
			Data: mustData(t, events.ChannelOpenedData{
				ChannelID: "0xchan1", Peer: "0xpeer", Capacity: mustAmount(t, "0x3e8"),
			}),
		}),
	}
}

func runIngest(t *testing.T, pool *pgxpool.Pool, changes []events.RawChange) {
	t.Helper()
	p := indexer.New(pool)
	runner := ingest.Runner{
		Source:     &fiber.FakeSource{Changes: changes},
		Normalizer: fiber.StubNormalizer{},
		Checkpoint: p,
	}
	if err := runner.Run(context.Background(), p.Handle); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestProjectorIngestsAndCheckpoints(t *testing.T) {
	pool := dbtest.Pool(t)
	runIngest(t, pool, seedChanges(t))

	if got := count(t, pool, "events"); got != 3 {
		t.Errorf("events count = %d, want 3", got)
	}
	if got := count(t, pool, "invoices"); got != 1 {
		t.Errorf("invoices count = %d, want 1", got)
	}
	if got := count(t, pool, "payments"); got != 1 {
		t.Errorf("payments count = %d, want 1", got)
	}
	if got := count(t, pool, "channels"); got != 1 {
		t.Errorf("channels count = %d, want 1", got)
	}

	// Checkpoint advanced to the last seq.
	var lastSeq int64
	if err := pool.QueryRow(context.Background(),
		"SELECT last_seq FROM ingest_checkpoint WHERE id = 1").Scan(&lastSeq); err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if lastSeq != 12 {
		t.Errorf("checkpoint last_seq = %d, want 12", lastSeq)
	}

	// Projection detail: invoice marked paid, payment captured payer pubkey.
	var invStatus string
	if err := pool.QueryRow(context.Background(),
		"SELECT status FROM invoices WHERE payment_hash = '0xhash1'").Scan(&invStatus); err != nil {
		t.Fatalf("read invoice: %v", err)
	}
	if invStatus != "paid" {
		t.Errorf("invoice status = %q, want paid", invStatus)
	}
	var peer string
	if err := pool.QueryRow(context.Background(),
		"SELECT peer FROM payments WHERE payment_hash = '0xhash2'").Scan(&peer); err != nil {
		t.Fatalf("read payment: %v", err)
	}
	if peer != "0xpayer" {
		t.Errorf("payment peer = %q, want 0xpayer", peer)
	}
}

func TestProjectorFansOutDeliveriesToMatchingEndpoints(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	// One endpoint subscribed to invoice.paid, one to a different type.
	_, err := pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (id, url, secret, event_types, active) VALUES
		 ('ep_paid', 'http://a.test', 's1', ARRAY['invoice.paid'], true),
		 ('ep_other', 'http://b.test', 's2', ARRAY['channel.opened'], true)`)
	if err != nil {
		t.Fatalf("seed endpoints: %v", err)
	}

	runIngest(t, pool, seedChanges(t)) // includes one invoice.paid and one channel.opened

	// invoice.paid fans out to ep_paid; channel.opened to ep_other → 2 deliveries.
	if got := count(t, pool, "webhook_deliveries"); got != 2 {
		t.Fatalf("deliveries = %d, want 2 (one per matching subscription)", got)
	}
	var epForPaid string
	if err := pool.QueryRow(ctx, `
		SELECT endpoint_id FROM webhook_deliveries d
		JOIN events e ON e.id = d.event_id WHERE e.type = 'invoice.paid'`).Scan(&epForPaid); err != nil {
		t.Fatalf("read delivery: %v", err)
	}
	if epForPaid != "ep_paid" {
		t.Errorf("invoice.paid delivered to %q, want ep_paid", epForPaid)
	}

	// Replay must not double-enqueue (deterministic delivery id + ON CONFLICT).
	runIngest(t, pool, seedChanges(t))
	if got := count(t, pool, "webhook_deliveries"); got != 2 {
		t.Errorf("deliveries after replay = %d, want 2", got)
	}
}

func TestProjectorAutoMatchesMerchantOrder(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	// A merchant with a pending order awaiting invoice 0xhash1 (matches seed's invoice.paid).
	if _, err := pool.Exec(ctx, `
		INSERT INTO merchants (id, name, api_key_hash) VALUES ('m1', 'Acme', 'h')`); err != nil {
		t.Fatalf("seed merchant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO merchant_orders (id, merchant_id, external_order_id, invoice_payment_hash, amount, asset, status)
		VALUES ('o1', 'm1', 'ext', '0xhash1', 100, 'CKB', 'pending')`); err != nil {
		t.Fatalf("seed order: %v", err)
	}

	runIngest(t, pool, seedChanges(t)) // includes invoice.paid for 0xhash1

	var status string
	var paidAt *string
	if err := pool.QueryRow(ctx,
		`SELECT status, paid_at::text FROM merchant_orders WHERE id = 'o1'`).Scan(&status, &paidAt); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if status != "paid" {
		t.Errorf("order status = %q, want paid (auto-matched on invoice.paid)", status)
	}
	if paidAt == nil {
		t.Error("paid_at was not set")
	}
}

func TestProjectorReplayIsIdempotent(t *testing.T) {
	pool := dbtest.Pool(t)
	changes := seedChanges(t)
	runIngest(t, pool, changes)
	runIngest(t, pool, changes) // replay the same changes

	if got := count(t, pool, "events"); got != 3 {
		t.Errorf("events count after replay = %d, want 3 (deduped)", got)
	}
	if got := count(t, pool, "payments"); got != 1 {
		t.Errorf("payments count after replay = %d, want 1", got)
	}
}
