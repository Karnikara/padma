// Package db holds the Postgres platform layer: connection pool, migrations,
// transaction helpers, and the Amount<->NUMERIC bridge that is the single source
// of truth for money-column conversions.
package db

import (
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/karnikara/kanaka/fibertypes"
)

// AmountToNumeric converts a fibertypes.Amount (a non-negative integer in the
// token's smallest unit) into a pgtype.Numeric for a NUMERIC column. The value
// is always an integer, so the exponent is zero.
//
// This is the only place amounts cross into the database; never format an Amount
// as its 0x-hex wire string into a NUMERIC column.
func AmountToNumeric(a fibertypes.Amount) pgtype.Numeric {
	return pgtype.Numeric{Int: a.BigInt(), Exp: 0, Valid: true}
}

// NumericToAmount converts a NUMERIC value read from Postgres back into an
// Amount. It normalizes the {Int, Exp} representation Postgres may return (e.g.
// 1000 as {Int:1, Exp:3}) to the underlying integer. A non-integer value (a
// negative exponent that does not divide evenly) or an invalid/NaN Numeric is an
// error, since amounts are always whole smallest-unit quantities.
func NumericToAmount(n pgtype.Numeric) (fibertypes.Amount, error) {
	if !n.Valid || n.NaN {
		return fibertypes.Amount{}, fmt.Errorf("db: NUMERIC is not a valid finite number")
	}
	if n.InfinityModifier != pgtype.Finite {
		return fibertypes.Amount{}, fmt.Errorf("db: NUMERIC is infinite")
	}

	v := new(big.Int).Set(n.Int)
	switch {
	case n.Exp > 0:
		v.Mul(v, pow10(n.Exp))
	case n.Exp < 0:
		divisor := pow10(-n.Exp)
		var rem big.Int
		v.QuoRem(v, divisor, &rem)
		if rem.Sign() != 0 {
			return fibertypes.Amount{}, fmt.Errorf("db: NUMERIC has a fractional part, not a whole amount")
		}
	}

	if v.Sign() < 0 {
		return fibertypes.Amount{}, fmt.Errorf("db: NUMERIC is negative")
	}
	return fibertypes.ParseAmount("0x" + v.Text(16))
}

func pow10(exp int32) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
}
