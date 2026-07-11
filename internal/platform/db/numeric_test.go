package db

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/karnikara/kanaka/fibertypes"
)

func TestAmountNumericRoundTrip(t *testing.T) {
	cases := []string{
		"0x0",
		"0x1",
		"0xde0b6b3a7640000",                  // 1e18
		"0xffffffffffffffffffffffffffffffff", // max u128
	}
	for _, hex := range cases {
		t.Run(hex, func(t *testing.T) {
			a, err := fibertypes.ParseAmount(hex)
			if err != nil {
				t.Fatalf("ParseAmount(%q): %v", hex, err)
			}
			n := AmountToNumeric(a)
			if !n.Valid {
				t.Fatal("AmountToNumeric produced an invalid Numeric")
			}
			got, err := NumericToAmount(n)
			if err != nil {
				t.Fatalf("NumericToAmount: %v", err)
			}
			if got.Hex() != a.Hex() {
				t.Fatalf("round trip mismatch: got %s want %s", got.Hex(), a.Hex())
			}
		})
	}
}

// NumericToAmount must normalize positive-exponent representations (Postgres may
// return 1000 as {Int:1, Exp:3}) back to the integer smallest-unit value.
func TestNumericToAmountPositiveExponent(t *testing.T) {
	n := pgtype.Numeric{Int: big.NewInt(1), Exp: 3, Valid: true}
	got, err := NumericToAmount(n)
	if err != nil {
		t.Fatalf("NumericToAmount: %v", err)
	}
	want, _ := fibertypes.ParseAmount("0x3e8") // 1000
	if got.Hex() != want.Hex() {
		t.Fatalf("got %s want %s", got.Hex(), want.Hex())
	}
}

func TestNumericToAmountRejectsInvalid(t *testing.T) {
	if _, err := NumericToAmount(pgtype.Numeric{Valid: false}); err == nil {
		t.Fatal("expected error for invalid Numeric, got nil")
	}
}

func TestNumericToAmountRejectsFractional(t *testing.T) {
	// 15 with Exp -1 == 1.5, not an integer smallest-unit amount.
	n := pgtype.Numeric{Int: big.NewInt(15), Exp: -1, Valid: true}
	if _, err := NumericToAmount(n); err == nil {
		t.Fatal("expected error for fractional Numeric, got nil")
	}
}
