package query_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/karnikara/padma/internal/platform/db/dbtest"
	"github.com/karnikara/padma/internal/query"
)

func TestQueryEndpoints(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	// Seed two payments (CKB + a UDT) and one event directly.
	_, err := pool.Exec(ctx, `
		INSERT INTO payments (payment_hash, direction, status, asset, amount, peer, created_at, updated_at) VALUES
		 ('0xp1', 'received', 'succeeded', 'CKB', 100, '0xpeerA', now() - interval '2 min', now()),
		 ('0xp2', 'received', 'succeeded', 'CKB', 200, '0xpeerB', now() - interval '1 min', now())`)
	if err != nil {
		t.Fatalf("seed payments: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO events (id, type, fiber_ref, asset, amount, data, occurred_at, checkpoint)
		VALUES ('evt_1', 'payment.received', '0xp1', 'CKB', 100, '{}'::jsonb, now(), 1)`)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}

	h := query.NewHandler(query.NewRepo(pool))
	r := chi.NewRouter()
	r.Route("/v1", h.Routes)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// List payments — newest first, amounts as 0x-hex.
	var page query.Page[query.Payment]
	getJSON(t, srv.URL+"/v1/payments", &page)
	if len(page.Items) != 2 {
		t.Fatalf("payments = %d, want 2", len(page.Items))
	}
	if page.Items[0].PaymentHash != "0xp2" {
		t.Errorf("newest first violated: got %s", page.Items[0].PaymentHash)
	}
	if page.Items[0].Amount != "0xc8" { // 200
		t.Errorf("amount hex = %q, want 0xc8", page.Items[0].Amount)
	}

	// Filter by peer.
	var filtered query.Page[query.Payment]
	getJSON(t, srv.URL+"/v1/payments?peer=0xpeerA", &filtered)
	if len(filtered.Items) != 1 || filtered.Items[0].PaymentHash != "0xp1" {
		t.Errorf("peer filter failed: %+v", filtered.Items)
	}

	// Single lookup + 404.
	var one query.Payment
	getJSON(t, srv.URL+"/v1/payments/0xp1", &one)
	if one.Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", one.Status)
	}
	if code := getStatus(t, srv.URL+"/v1/payments/0xmissing"); code != http.StatusNotFound {
		t.Errorf("missing payment status = %d, want 404", code)
	}

	// Stats: SUM grouped by asset = 300 = 0x12c.
	var stats struct {
		Buckets []query.StatRow `json:"buckets"`
	}
	getJSON(t, srv.URL+"/v1/stats/payments", &stats)
	if len(stats.Buckets) != 1 || stats.Buckets[0].Asset != "CKB" {
		t.Fatalf("stats buckets = %+v", stats.Buckets)
	}
	if stats.Buckets[0].Count != 2 || stats.Buckets[0].Total != "0x12c" {
		t.Errorf("stat = %+v, want count 2 total 0x12c", stats.Buckets[0])
	}
}

func TestPaymentsCursorPagination(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO payments (payment_hash, direction, status, asset, amount, created_at, updated_at) VALUES
		 ('0xa', 'received', 'succeeded', 'CKB', 1, now() - interval '3 min', now()),
		 ('0xb', 'received', 'succeeded', 'CKB', 2, now() - interval '2 min', now()),
		 ('0xc', 'received', 'succeeded', 'CKB', 3, now() - interval '1 min', now())`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := query.NewHandler(query.NewRepo(pool))
	r := chi.NewRouter()
	r.Route("/v1", h.Routes)
	srv := httptest.NewServer(r)
	defer srv.Close()

	var p1 query.Page[query.Payment]
	getJSON(t, srv.URL+"/v1/payments?limit=2", &p1)
	if len(p1.Items) != 2 || p1.NextCursor == "" {
		t.Fatalf("first page = %d items, cursor %q; want 2 + cursor", len(p1.Items), p1.NextCursor)
	}
	if p1.Items[0].PaymentHash != "0xc" || p1.Items[1].PaymentHash != "0xb" {
		t.Errorf("first page order = %s,%s; want 0xc,0xb", p1.Items[0].PaymentHash, p1.Items[1].PaymentHash)
	}

	var p2 query.Page[query.Payment]
	getJSON(t, srv.URL+"/v1/payments?limit=2&cursor="+p1.NextCursor, &p2)
	if len(p2.Items) != 1 || p2.Items[0].PaymentHash != "0xa" {
		t.Fatalf("second page = %+v; want [0xa]", p2.Items)
	}
	if p2.NextCursor != "" {
		t.Errorf("last page should have empty cursor, got %q", p2.NextCursor)
	}
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}

func getStatus(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
