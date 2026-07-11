// Package merchant is padma's merchant backend (#3): orders backed by Fiber
// invoices, automatic payment matching, refunds via keysend, per-asset
// settlement, reconciliation against the payments projection, and CSV/XLSX
// export. It depends on the Fiber node only through the small FiberRPC seam, so
// it is unit-testable without a live node.
package merchant

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/kanaka/rpc"
)

// FiberRPC is the slice of the library rpc.Client the merchant backend needs.
// *rpc.Client satisfies it; tests substitute a fake.
type FiberRPC interface {
	NewInvoice(ctx context.Context, p rpc.NewInvoiceParams) (rpc.Invoice, error)
	SendPayment(ctx context.Context, p rpc.SendPaymentParams) (rpc.Payment, error)
}

// Service holds the merchant backend's dependencies.
type Service struct {
	pool *pgxpool.Pool
	rpc  FiberRPC
}

// NewService returns a merchant Service over pool using fiber for node calls.
func NewService(pool *pgxpool.Pool, fiber FiberRPC) *Service {
	return &Service{pool: pool, rpc: fiber}
}

// Order is a merchant order row.
type Order struct {
	ID                 string  `json:"id"`
	MerchantID         string  `json:"merchant_id"`
	ExternalOrderID    string  `json:"external_order_id,omitempty"`
	InvoicePaymentHash string  `json:"invoice_payment_hash,omitempty"`
	Amount             string  `json:"amount,omitempty"` // 0x-hex
	Asset              string  `json:"asset,omitempty"`
	Status             string  `json:"status"`
	CreatedAt          string  `json:"created_at,omitempty"`
	PaidAt             *string `json:"paid_at,omitempty"`
}

// Refund is a refund row.
type Refund struct {
	ID               string `json:"id"`
	OrderID          string `json:"order_id"`
	Amount           string `json:"amount,omitempty"`
	Asset            string `json:"asset,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Status           string `json:"status"`
	FiberPaymentHash string `json:"fiber_payment_hash,omitempty"`
}

// SettlementRow is one per-asset settlement bucket for a period.
type SettlementRow struct {
	Asset  string `json:"asset"`
	Gross  string `json:"gross"`
	Fees   string `json:"fees"`
	Net    string `json:"net"`
	Period string `json:"period"`
}

// Reconciliation summarizes a period's order-vs-payment match.
type Reconciliation struct {
	Period           string   `json:"period"`
	Matched          int      `json:"matched"`
	UnmatchedCount   int      `json:"unmatched_count"`
	DiscrepancyTotal string   `json:"discrepancy_total"`
	UnmatchedOrders  []string `json:"unmatched_orders"`
}
