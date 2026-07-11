package merchant

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"
)

// exportHeader is the column order shared by the CSV and XLSX exporters.
var exportHeader = []string{
	"order_id", "external_order_id", "asset", "amount", "status", "paid_at", "refund_amount",
}

// ExportRow is one accounting row for a period.
type ExportRow struct {
	OrderID      string
	ExternalID   string
	Asset        string
	Amount       string
	Status       string
	PaidAt       string
	RefundAmount string
}

func (r ExportRow) cells() []string {
	return []string{r.OrderID, r.ExternalID, r.Asset, r.Amount, r.Status, r.PaidAt, r.RefundAmount}
}

// buildCSV renders rows as CSV bytes with a header.
func buildCSV(rows []ExportRow) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(exportHeader)
	for _, r := range rows {
		_ = w.Write(r.cells())
	}
	w.Flush()
	return buf.Bytes()
}

// buildXLSX renders rows as an .xlsx workbook (one "Orders" sheet).
func buildXLSX(rows []ExportRow) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Orders"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return nil, fmt.Errorf("merchant: xlsx sheet: %w", err)
	}
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")

	for c, h := range exportHeader {
		cell, _ := excelize.CoordinatesToCellName(c+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for i, r := range rows {
		for c, v := range r.cells() {
			cell, _ := excelize.CoordinatesToCellName(c+1, i+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("merchant: xlsx write: %w", err)
	}
	return buf.Bytes(), nil
}

// Export builds an accounting export for a period in the requested format
// ("csv" or "xlsx"), returning a filename, content type, and bytes.
func (s *Service) Export(ctx context.Context, merchantID, period, format string) (filename, contentType string, data []byte, err error) {
	rows, err := s.exportRows(ctx, merchantID, period)
	if err != nil {
		return "", "", nil, err
	}
	switch format {
	case "", "csv":
		return "settlement-" + period + ".csv", "text/csv", buildCSV(rows), nil
	case "xlsx":
		b, err := buildXLSX(rows)
		if err != nil {
			return "", "", nil, err
		}
		return "settlement-" + period + ".xlsx",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", b, nil
	default:
		return "", "", nil, fmt.Errorf("%w: format %q (want csv|xlsx)", ErrInvalidState, format)
	}
}

func (s *Service) exportRows(ctx context.Context, merchantID, period string) ([]ExportRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, COALESCE(o.external_order_id, ''), COALESCE(o.asset, ''),
		       o.amount, o.status, o.paid_at,
		       COALESCE((SELECT SUM(amount) FROM refunds r WHERE r.order_id = o.id), 0)
		FROM merchant_orders o
		WHERE o.merchant_id = $1 AND o.paid_at IS NOT NULL
		  AND to_char(o.paid_at, 'YYYY-MM') = $2
		ORDER BY o.paid_at`, merchantID, period)
	if err != nil {
		return nil, fmt.Errorf("merchant: export query: %w", err)
	}
	defer rows.Close()

	var out []ExportRow
	for rows.Next() {
		var r ExportRow
		var amount, refund pgtype.Numeric
		var paidAt pgtype.Timestamptz
		if err := rows.Scan(&r.OrderID, &r.ExternalID, &r.Asset, &amount, &r.Status, &paidAt, &refund); err != nil {
			return nil, fmt.Errorf("merchant: scan export row: %w", err)
		}
		r.Amount = mustAmount(amount).Hex()
		r.RefundAmount = mustAmount(refund).Hex()
		if paidAt.Valid {
			r.PaidAt = paidAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
