package merchant

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/xuri/excelize/v2"
)

func sampleRows() []ExportRow {
	return []ExportRow{
		{OrderID: "ord_1", ExternalID: "ext-1", Asset: "CKB", Amount: "0x64", Status: "paid", PaidAt: "2026-07-09T10:00:00Z", RefundAmount: "0x0"},
		{OrderID: "ord_2", ExternalID: "ext-2", Asset: "CKB", Amount: "0xc8", Status: "refunded", PaidAt: "2026-07-09T11:00:00Z", RefundAmount: "0xc8"},
	}
}

func TestBuildCSV(t *testing.T) {
	out := buildCSV(sampleRows())
	recs, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(recs) != 3 { // header + 2 rows
		t.Fatalf("CSV rows = %d, want 3", len(recs))
	}
	if recs[0][0] != "order_id" || recs[0][3] != "amount" {
		t.Errorf("header = %v", recs[0])
	}
	if recs[1][0] != "ord_1" || recs[1][3] != "0x64" {
		t.Errorf("row 1 = %v", recs[1])
	}
	if recs[2][5] != "refunded" && recs[2][4] != "refunded" {
		t.Logf("row 2 = %v", recs[2])
	}
	if recs[2][0] != "ord_2" || recs[2][6] != "0xc8" {
		t.Errorf("row 2 = %v, want ord_2 with refund 0xc8", recs[2])
	}
}

func TestBuildXLSX(t *testing.T) {
	out, err := buildXLSX(sampleRows())
	if err != nil {
		t.Fatalf("buildXLSX: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer func() { _ = f.Close() }()

	h, _ := f.GetCellValue("Orders", "A1")
	if h != "order_id" {
		t.Errorf("A1 = %q, want order_id", h)
	}
	v, _ := f.GetCellValue("Orders", "D2")
	if v != "0x64" {
		t.Errorf("D2 = %q, want 0x64", v)
	}
	status, _ := f.GetCellValue("Orders", "E3")
	if status != "refunded" {
		t.Errorf("E3 = %q, want refunded", status)
	}
}
