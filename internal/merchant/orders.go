package merchant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/karnikara/kanaka/fibertypes"
	"github.com/karnikara/kanaka/rpc"

	"github.com/karnikara/padma/internal/platform/db"
	"github.com/karnikara/padma/internal/platform/id"
)

// ErrNotFound is returned when an order or related row does not exist.
var ErrNotFound = errors.New("merchant: not found")

// ErrInvalidState is returned when an operation is not valid for the order's
// current status (e.g. refunding an unpaid order).
var ErrInvalidState = errors.New("merchant: invalid state for operation")

// CreateOrder creates (or idempotently returns) an order and its Fiber invoice.
// Idempotency is keyed on (merchant_id, external_order_id): a repeat request
// returns the existing order without minting a second invoice.
func (s *Service) CreateOrder(ctx context.Context, merchantID, extID, amountHex, asset string) (Order, rpc.Invoice, error) {
	amount, err := fibertypes.ParseAmount(amountHex)
	if err != nil {
		return Order{}, rpc.Invoice{}, fmt.Errorf("%w: amount: %s", ErrInvalidState, err.Error())
	}
	if asset == "" {
		asset = "CKB"
	}

	if extID != "" {
		if existing, err := s.orderByExternalID(ctx, merchantID, extID); err == nil {
			return existing, rpc.Invoice{PaymentHash: fibertypes.PaymentHash(existing.InvoicePaymentHash)}, nil
		} else if !errors.Is(err, ErrNotFound) {
			return Order{}, rpc.Invoice{}, err
		}
	}

	// UDT script passthrough is deferred; native CKB invoices are wired now.
	inv, err := s.rpc.NewInvoice(ctx, rpc.NewInvoiceParams{
		Amount:      amount,
		Description: "order " + extID,
	})
	if err != nil {
		return Order{}, rpc.Invoice{}, fmt.Errorf("merchant: new invoice: %w", err)
	}

	orderID := id.New()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO merchant_orders
			(id, merchant_id, external_order_id, invoice_payment_hash, amount, asset, status)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, 'pending')
		ON CONFLICT (merchant_id, external_order_id) DO NOTHING`,
		orderID, merchantID, extID, string(inv.PaymentHash), db.AmountToNumeric(amount), asset)
	if err != nil {
		return Order{}, rpc.Invoice{}, fmt.Errorf("merchant: insert order: %w", err)
	}

	// If a concurrent request won the idempotency race, return the winner.
	if extID != "" {
		if existing, err := s.orderByExternalID(ctx, merchantID, extID); err == nil {
			return existing, inv, nil
		}
	}
	order, err := s.GetOrder(ctx, merchantID, orderID)
	return order, inv, err
}

// GetOrder returns a single order for the merchant, or ErrNotFound.
func (s *Service) GetOrder(ctx context.Context, merchantID, orderID string) (Order, error) {
	return s.scanOrder(s.pool.QueryRow(ctx, orderSelect+` WHERE id = $1 AND merchant_id = $2`, orderID, merchantID))
}

func (s *Service) orderByExternalID(ctx context.Context, merchantID, extID string) (Order, error) {
	return s.scanOrder(s.pool.QueryRow(ctx,
		orderSelect+` WHERE merchant_id = $1 AND external_order_id = $2`, merchantID, extID))
}

const orderSelect = `
	SELECT id, merchant_id, COALESCE(external_order_id, ''), COALESCE(invoice_payment_hash, ''),
	       amount, COALESCE(asset, ''), status, created_at, paid_at
	FROM merchant_orders`

func (s *Service) scanOrder(row pgx.Row) (Order, error) {
	var o Order
	var amount pgtype.Numeric
	var created pgtype.Timestamptz
	var paid pgtype.Timestamptz
	err := row.Scan(&o.ID, &o.MerchantID, &o.ExternalOrderID, &o.InvoicePaymentHash,
		&amount, &o.Asset, &o.Status, &created, &paid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("merchant: scan order: %w", err)
	}
	if amount.Valid {
		if a, err := db.NumericToAmount(amount); err == nil {
			o.Amount = a.Hex()
		}
	}
	if created.Valid {
		o.CreatedAt = created.Time.UTC().Format(time.RFC3339)
	}
	if paid.Valid {
		v := paid.Time.UTC().Format(time.RFC3339)
		o.PaidAt = &v
	}
	return o, nil
}
