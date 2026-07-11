package indexer

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/karnikara/kanaka/events"
)

// matchMerchantOrder flips a pending merchant order to 'paid' when its invoice is
// settled, in the projector's transaction (spec-B.md §5, automatic payment
// matching — instant finality, no polling). It is a no-op before migration 0003
// creates merchant_orders, so the indexer runs standalone at Gates A/B.
func matchMerchantOrder(ctx context.Context, tx pgx.Tx, d events.InvoicePaidData) error {
	var exists bool
	if err := tx.QueryRow(ctx,
		"SELECT to_regclass('merchant_orders') IS NOT NULL").Scan(&exists); err != nil {
		return fmt.Errorf("indexer: probe merchant_orders: %w", err)
	}
	if !exists {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE merchant_orders
		SET status = 'paid', paid_at = $2
		WHERE invoice_payment_hash = $1 AND status = 'pending'`,
		string(d.PaymentHash), d.SettledAt)
	if err != nil {
		return fmt.Errorf("indexer: match merchant order: %w", err)
	}
	return nil
}
