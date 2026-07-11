package merchant

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/karnikara/kanaka/fibertypes"
	"github.com/karnikara/kanaka/rpc"

	"github.com/karnikara/padma/internal/platform/db"
	"github.com/karnikara/padma/internal/platform/id"
)

// Refund sends a keysend refund back to the original payer and records it. The
// order must be 'paid'; the payer pubkey is recovered from the payments
// projection captured at payment time (no private keys on the server — the node
// signs).
func (s *Service) Refund(ctx context.Context, merchantID, orderID, amountHex, reason string) (Refund, error) {
	order, err := s.GetOrder(ctx, merchantID, orderID)
	if err != nil {
		return Refund{}, err
	}
	if order.Status != "paid" {
		return Refund{}, fmt.Errorf("%w: order status is %q, must be paid", ErrInvalidState, order.Status)
	}
	amount, err := fibertypes.ParseAmount(amountHex)
	if err != nil {
		return Refund{}, fmt.Errorf("%w: amount: %s", ErrInvalidState, err.Error())
	}

	payer, err := s.payerPubkey(ctx, order.InvoicePaymentHash)
	if err != nil {
		return Refund{}, err
	}

	pay, err := s.rpc.SendPayment(ctx, rpc.SendPaymentParams{
		TargetPubkey: payer,
		Amount:       amount,
		Keysend:      true,
	})
	if err != nil {
		return Refund{}, fmt.Errorf("merchant: send refund: %w", err)
	}

	refundID := id.New()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO refunds (id, order_id, amount, asset, reason, status, fiber_payment_hash)
		VALUES ($1, $2, $3, $4, $5, 'sent', $6)`,
		refundID, orderID, db.AmountToNumeric(amount), order.Asset, reason, string(pay.PaymentHash))
	if err != nil {
		return Refund{}, fmt.Errorf("merchant: insert refund: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE merchant_orders SET status = 'refunded' WHERE id = $1`, orderID); err != nil {
		return Refund{}, fmt.Errorf("merchant: mark order refunded: %w", err)
	}

	return Refund{
		ID: refundID, OrderID: orderID, Amount: amount.Hex(), Asset: order.Asset,
		Reason: reason, Status: "sent", FiberPaymentHash: string(pay.PaymentHash),
	}, nil
}

// payerPubkey looks up the pubkey of whoever paid the invoice, captured on the
// payments projection (payment.received carries the payer pubkey for refunds).
func (s *Service) payerPubkey(ctx context.Context, paymentHash string) (string, error) {
	var peer *string
	err := s.pool.QueryRow(ctx,
		`SELECT peer FROM payments WHERE payment_hash = $1`, paymentHash).Scan(&peer)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (peer == nil || *peer == "")) {
		return "", fmt.Errorf("%w: no payer pubkey captured for this order", ErrInvalidState)
	}
	if err != nil {
		return "", fmt.Errorf("merchant: lookup payer: %w", err)
	}
	return *peer, nil
}
