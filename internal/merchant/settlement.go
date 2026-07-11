package merchant

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/karnikara/kanaka/fibertypes"

	"github.com/karnikara/padma/internal/platform/db"
	"github.com/karnikara/padma/internal/platform/id"
)

// Settlement aggregates a merchant's paid orders for a period (YYYY-MM) into
// per-asset gross/fees/net rows, persists them, and returns them. Fees are the
// refunds paid back out; net = gross - fees. Multi-asset by construction.
func (s *Service) Settlement(ctx context.Context, merchantID, period string) ([]SettlementRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(o.asset, 'CKB') AS asset,
		       COALESCE(SUM(o.amount), 0) AS gross,
		       COALESCE(SUM(r.amt), 0) AS fees
		FROM merchant_orders o
		LEFT JOIN (SELECT order_id, SUM(amount) AS amt FROM refunds GROUP BY order_id) r
		       ON r.order_id = o.id
		WHERE o.merchant_id = $1 AND o.paid_at IS NOT NULL
		  AND to_char(o.paid_at, 'YYYY-MM') = $2
		GROUP BY COALESCE(o.asset, 'CKB')
		ORDER BY asset`, merchantID, period)
	if err != nil {
		return nil, fmt.Errorf("merchant: settlement query: %w", err)
	}
	defer rows.Close()

	var out []SettlementRow
	for rows.Next() {
		var asset string
		var gross, fees pgtype.Numeric
		if err := rows.Scan(&asset, &gross, &fees); err != nil {
			return nil, fmt.Errorf("merchant: scan settlement: %w", err)
		}
		grossA := mustAmount(gross)
		feesA := mustAmount(fees)
		net := new(big.Int).Sub(grossA.BigInt(), feesA.BigInt())
		if net.Sign() < 0 {
			net.SetInt64(0)
		}
		netA, _ := fibertypes.ParseAmount("0x" + net.Text(16))
		out = append(out, SettlementRow{
			Asset: asset, Period: period,
			Gross: grossA.Hex(), Fees: feesA.Hex(), Net: netA.Hex(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, r := range out {
		g, _ := fibertypes.ParseAmount(r.Gross)
		f, _ := fibertypes.ParseAmount(r.Fees)
		n, _ := fibertypes.ParseAmount(r.Net)
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO settlements (id, merchant_id, period, gross, fees, net, asset, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'generated')`,
			id.New(), merchantID, period,
			db.AmountToNumeric(g), db.AmountToNumeric(f), db.AmountToNumeric(n), r.Asset); err != nil {
			return nil, fmt.Errorf("merchant: persist settlement: %w", err)
		}
	}
	return out, nil
}

// Reconciliation matches a merchant's paid orders for a period against the
// payments projection (from #11), records the run, and returns the summary.
func (s *Service) Reconciliation(ctx context.Context, merchantID, period string) (Reconciliation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, (p.payment_hash IS NOT NULL) AS matched, COALESCE(o.amount, 0)
		FROM merchant_orders o
		LEFT JOIN payments p ON p.payment_hash = o.invoice_payment_hash
		WHERE o.merchant_id = $1 AND o.paid_at IS NOT NULL
		  AND to_char(o.paid_at, 'YYYY-MM') = $2`, merchantID, period)
	if err != nil {
		return Reconciliation{}, fmt.Errorf("merchant: reconcile query: %w", err)
	}
	defer rows.Close()

	rec := Reconciliation{Period: period, UnmatchedOrders: []string{}}
	discrepancy := new(big.Int)
	for rows.Next() {
		var orderID string
		var matched bool
		var amount pgtype.Numeric
		if err := rows.Scan(&orderID, &matched, &amount); err != nil {
			return Reconciliation{}, fmt.Errorf("merchant: scan reconcile: %w", err)
		}
		if matched {
			rec.Matched++
		} else {
			rec.UnmatchedCount++
			rec.UnmatchedOrders = append(rec.UnmatchedOrders, orderID)
			discrepancy.Add(discrepancy, mustAmount(amount).BigInt())
		}
	}
	if err := rows.Err(); err != nil {
		return Reconciliation{}, err
	}
	discA, _ := fibertypes.ParseAmount("0x" + discrepancy.Text(16))
	rec.DiscrepancyTotal = discA.Hex()

	report, _ := json.Marshal(rec)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO reconciliation_runs
			(id, merchant_id, period, matched, unmatched_count, discrepancy_total, report)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		id.New(), merchantID, period, rec.Matched, rec.UnmatchedCount,
		db.AmountToNumeric(discA), report); err != nil {
		return Reconciliation{}, fmt.Errorf("merchant: persist reconciliation: %w", err)
	}
	return rec, nil
}

// mustAmount converts a NUMERIC to an Amount, treating conversion failure as zero
// (aggregation results are always whole, non-negative sums).
func mustAmount(n pgtype.Numeric) fibertypes.Amount {
	a, err := db.NumericToAmount(n)
	if err != nil {
		zero, _ := fibertypes.ParseAmount("0x0")
		return zero
	}
	return a
}
