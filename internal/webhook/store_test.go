package webhook_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/padma/internal/platform/db/dbtest"
	"github.com/karnikara/padma/internal/webhook"
)

func seedEndpointAndEvent(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (id, url, secret, event_types, active)
		VALUES ('ep_1', 'http://example.test/hook', 'sekret', ARRAY['invoice.paid'], true)`)
	if err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO events (id, type, fiber_ref, data, occurred_at, checkpoint)
		VALUES ('evt_1', 'invoice.paid', '0xhash', '{}'::jsonb, now(), 1)`)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
}

func insertDelivery(t *testing.T, pool *pgxpool.Pool, id string, dueOffset time.Duration, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO webhook_deliveries (id, endpoint_id, event_id, url, secret, payload, status, next_attempt_at)
		VALUES ($1, 'ep_1', 'evt_1', 'http://example.test/hook', 'sekret', '{"hi":1}'::jsonb, $2, now() + $3)`,
		id, status, dueOffset)
	if err != nil {
		t.Fatalf("insert delivery %s: %v", id, err)
	}
}

func statusOf(t *testing.T, pool *pgxpool.Pool, id string) (string, int) {
	t.Helper()
	var status string
	var attempts int
	if err := pool.QueryRow(context.Background(),
		"SELECT status, attempts FROM webhook_deliveries WHERE id = $1", id).Scan(&status, &attempts); err != nil {
		t.Fatalf("read delivery %s: %v", id, err)
	}
	return status, attempts
}

func TestClaimDueOnlyReturnsDueAndJoinsEventType(t *testing.T) {
	pool := dbtest.Pool(t)
	seedEndpointAndEvent(t, pool)
	insertDelivery(t, pool, "dlv_due", -time.Minute, "pending") // due now
	insertDelivery(t, pool, "dlv_future", time.Hour, "pending") // not due yet

	store := webhook.NewStore(pool)
	claimed, err := store.ClaimDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != "dlv_due" {
		t.Fatalf("claimed = %+v, want only dlv_due", claimed)
	}
	if claimed[0].EventType != "invoice.paid" {
		t.Errorf("EventType = %q, want invoice.paid", claimed[0].EventType)
	}
	if claimed[0].Attempts != 0 {
		t.Errorf("Attempts = %d, want 0 (not incremented at claim time)", claimed[0].Attempts)
	}
	if string(claimed[0].Payload) == "" {
		t.Error("Payload was not loaded")
	}
	if st, _ := statusOf(t, pool, "dlv_due"); st != "delivering" {
		t.Errorf("claimed delivery status = %q, want delivering", st)
	}
}

func TestMarkTransitions(t *testing.T) {
	pool := dbtest.Pool(t)
	seedEndpointAndEvent(t, pool)
	insertDelivery(t, pool, "dlv_f", -time.Minute, "delivering")
	insertDelivery(t, pool, "dlv_d", -time.Minute, "delivering")
	insertDelivery(t, pool, "dlv_s", -time.Minute, "delivering")
	store := webhook.NewStore(pool)
	ctx := context.Background()

	next := time.Now().Add(30 * time.Second)
	if err := store.MarkFailed(ctx, "dlv_f", next, "boom"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if st, at := statusOf(t, pool, "dlv_f"); st != "failed" || at != 1 {
		t.Errorf("after MarkFailed: status=%q attempts=%d, want failed/1", st, at)
	}

	if err := store.MarkDead(ctx, "dlv_d"); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}
	if st, at := statusOf(t, pool, "dlv_d"); st != "dead" || at != 1 {
		t.Errorf("after MarkDead: status=%q attempts=%d, want dead/1", st, at)
	}

	if err := store.MarkSucceeded(ctx, "dlv_s"); err != nil {
		t.Fatalf("MarkSucceeded: %v", err)
	}
	if st, _ := statusOf(t, pool, "dlv_s"); st != "succeeded" {
		t.Errorf("after MarkSucceeded: status=%q, want succeeded", st)
	}
}

func TestClaimDueSkipLockedNoDoubleClaim(t *testing.T) {
	pool := dbtest.Pool(t)
	seedEndpointAndEvent(t, pool)
	for i := 0; i < 10; i++ {
		insertDelivery(t, pool, "dlv_"+string(rune('a'+i)), -time.Minute, "pending")
	}
	store := webhook.NewStore(pool)

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				claimed, err := store.ClaimDue(context.Background(), 3)
				if err != nil || len(claimed) == 0 {
					return
				}
				mu.Lock()
				for _, d := range claimed {
					seen[d.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != 10 {
		t.Errorf("claimed %d distinct deliveries, want 10", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("delivery %s claimed %d times, want exactly 1", id, n)
		}
	}
}
