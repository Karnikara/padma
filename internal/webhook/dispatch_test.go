package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	kwebhook "github.com/karnikara/kanaka/webhook"

	"github.com/karnikara/padma/internal/platform/db/dbtest"
	"github.com/karnikara/padma/internal/webhook"
)

func seedDelivery(t *testing.T, pool *pgxpool.Pool, url, secret, eventType string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (id, url, secret, event_types, active)
		VALUES ('ep_x', $1, $2, ARRAY[$3::text], true)`, url, secret, eventType)
	if err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO events (id, type, fiber_ref, data, occurred_at, checkpoint)
		VALUES ('evt_x', $1, '0xhash', '{"hello":"world"}'::jsonb, now(), 1)`, eventType)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (id, endpoint_id, event_id, url, secret, payload, status, next_attempt_at)
		VALUES ('dlv_x', 'ep_x', 'evt_x', $1, $2, '{"hello":"world"}'::jsonb, 'pending', now())`, url, secret)
	if err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
}

func runDispatcherUntil(t *testing.T, pool *pgxpool.Pool, maxAttempts int, want string) string {
	t.Helper()
	store := webhook.NewStore(pool)
	disp := &kwebhook.Dispatcher{
		Store:        store,
		Sender:       &kwebhook.HTTPSender{Client: &http.Client{Timeout: 2 * time.Second}},
		Backoff:      func(int) time.Duration { return 10 * time.Millisecond },
		MaxAttempts:  maxAttempts,
		Workers:      2,
		PollInterval: 10 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = disp.Run(ctx) }()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			st, _ := statusOf(t, pool, "dlv_x")
			t.Fatalf("delivery did not reach %q in time (last status %q)", want, st)
		default:
		}
		if st, _ := statusOf(t, pool, "dlv_x"); st == want {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDispatcherDeliversAndSignsSuccessfully(t *testing.T) {
	pool := dbtest.Pool(t)
	const secret = "topsecret"

	var got struct {
		verified atomic.Bool
		body     atomic.Value
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.body.Store(string(body))
		err := kwebhook.Verify(secret, r.Header, body, kwebhook.VerifyOpts{Tolerance: time.Minute})
		got.verified.Store(err == nil)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	seedDelivery(t, pool, srv.URL, secret, "invoice.paid")
	runDispatcherUntil(t, pool, 5, "succeeded")

	if !got.verified.Load() {
		t.Error("receiver could not verify the HMAC signature")
	}
	// JSONB reformats whitespace; compare semantically. The signature verifying
	// above already proves the bytes sent match the bytes signed.
	b, _ := got.body.Load().(string)
	var payload map[string]string
	if err := json.Unmarshal([]byte(b), &payload); err != nil || payload["hello"] != "world" {
		t.Errorf("receiver body = %q, want the delivery payload", b)
	}
}

func TestDispatcherDeadLettersAfterMaxAttempts(t *testing.T) {
	pool := dbtest.Pool(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError) // always fail
	}))
	defer srv.Close()

	seedDelivery(t, pool, srv.URL, "s", "invoice.paid")
	runDispatcherUntil(t, pool, 3, "dead")

	_, attempts := statusOf(t, pool, "dlv_x")
	if attempts != 3 {
		t.Errorf("attempts at dead-letter = %d, want 3", attempts)
	}
	if hits.Load() < 3 {
		t.Errorf("receiver hit %d times, want >= 3", hits.Load())
	}

	// The delivery log recorded each attempt.
	var logs int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM webhook_delivery_log WHERE delivery_id = 'dlv_x'").Scan(&logs); err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if logs != 3 {
		t.Errorf("delivery log rows = %d, want 3", logs)
	}
}
