package merchant_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/kanaka/rpc"

	"github.com/karnikara/padma/internal/merchant"
	"github.com/karnikara/padma/internal/platform/db/dbtest"
)

type fakeRPC struct {
	inv rpc.Invoice
	pay rpc.Payment
}

func (f *fakeRPC) NewInvoice(context.Context, rpc.NewInvoiceParams) (rpc.Invoice, error) {
	return f.inv, nil
}
func (f *fakeRPC) SendPayment(context.Context, rpc.SendPaymentParams) (rpc.Payment, error) {
	return f.pay, nil
}

func provision(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id, _, err := merchant.Provision(context.Background(), pool, "Acme", "CKB")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	return id
}

func TestCreateOrderIsIdempotent(t *testing.T) {
	pool := dbtest.Pool(t)
	mid := provision(t, pool)
	svc := merchant.NewService(pool, &fakeRPC{inv: rpc.Invoice{PaymentHash: "0xinv1"}})
	ctx := context.Background()

	o1, inv, err := svc.CreateOrder(ctx, mid, "ext-1", "0x64", "CKB")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if o1.Status != "pending" || o1.InvoicePaymentHash != "0xinv1" || string(inv.PaymentHash) != "0xinv1" {
		t.Fatalf("unexpected order/invoice: %+v %+v", o1, inv)
	}
	if o1.Amount != "0x64" {
		t.Errorf("amount = %q, want 0x64", o1.Amount)
	}

	o2, _, err := svc.CreateOrder(ctx, mid, "ext-1", "0x64", "CKB")
	if err != nil {
		t.Fatalf("CreateOrder repeat: %v", err)
	}
	if o2.ID != o1.ID {
		t.Errorf("idempotency broken: %s != %s", o2.ID, o1.ID)
	}
}

func markPaid(t *testing.T, pool *pgxpool.Pool, orderID, invoiceHash, payer, paidAt string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`UPDATE merchant_orders SET status='paid', paid_at=$2 WHERE id=$1`, orderID, paidAt); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO payments (payment_hash, direction, status, asset, amount, peer, created_at, updated_at)
		VALUES ($1, 'received', 'succeeded', 'CKB', 100, $2, now(), now())`, invoiceHash, payer); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
}

func TestRefundFlow(t *testing.T) {
	pool := dbtest.Pool(t)
	mid := provision(t, pool)
	svc := merchant.NewService(pool, &fakeRPC{
		inv: rpc.Invoice{PaymentHash: "0xinv2"},
		pay: rpc.Payment{PaymentHash: "0xrefund", Status: "sent"},
	})
	ctx := context.Background()

	o, _, err := svc.CreateOrder(ctx, mid, "ext-2", "0x64", "CKB")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	// Refund before payment is rejected.
	if _, err := svc.Refund(ctx, mid, o.ID, "0x64", "oops"); err == nil {
		t.Fatal("expected refund of unpaid order to fail")
	}

	markPaid(t, pool, o.ID, "0xinv2", "0xpayerpub", "2026-07-09T10:00:00Z")

	ref, err := svc.Refund(ctx, mid, o.ID, "0x64", "customer request")
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if ref.FiberPaymentHash != "0xrefund" || ref.Status != "sent" {
		t.Errorf("refund = %+v", ref)
	}
	got, _ := svc.GetOrder(ctx, mid, o.ID)
	if got.Status != "refunded" {
		t.Errorf("order status = %q, want refunded", got.Status)
	}
}

func TestSettlementAndReconciliation(t *testing.T) {
	pool := dbtest.Pool(t)
	mid := provision(t, pool)
	svc := merchant.NewService(pool, &fakeRPC{inv: rpc.Invoice{PaymentHash: "0xinvS"}})
	ctx := context.Background()

	o, _, err := svc.CreateOrder(ctx, mid, "ext-S", "0x64", "CKB")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	markPaid(t, pool, o.ID, "0xinvS", "0xpayer", "2026-07-15T10:00:00Z")

	// Settlement for the period: gross = 0x64, no refunds → net = 0x64.
	rows, err := svc.Settlement(ctx, mid, "2026-07")
	if err != nil {
		t.Fatalf("Settlement: %v", err)
	}
	if len(rows) != 1 || rows[0].Asset != "CKB" || rows[0].Gross != "0x64" || rows[0].Net != "0x64" {
		t.Fatalf("settlement = %+v", rows)
	}

	// Reconciliation: the paid order matches a payments row on invoice hash.
	rec, err := svc.Reconciliation(ctx, mid, "2026-07")
	if err != nil {
		t.Fatalf("Reconciliation: %v", err)
	}
	if rec.Matched != 1 || rec.UnmatchedCount != 0 {
		t.Errorf("reconciliation = %+v, want matched 1 / unmatched 0", rec)
	}

	// An order paid but with no matching payment row shows as a discrepancy.
	o2, _, _ := svc.CreateOrder(ctx, mid, "ext-S2", "0xc8", "CKB")
	if _, err := pool.Exec(ctx,
		`UPDATE merchant_orders SET status='paid', paid_at='2026-07-16T10:00:00Z', invoice_payment_hash='0xorphan' WHERE id=$1`,
		o2.ID); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	rec2, err := svc.Reconciliation(ctx, mid, "2026-07")
	if err != nil {
		t.Fatalf("Reconciliation2: %v", err)
	}
	if rec2.UnmatchedCount != 1 || rec2.DiscrepancyTotal != "0xc8" {
		t.Errorf("reconciliation2 = %+v, want 1 unmatched / 0xc8", rec2)
	}
}
