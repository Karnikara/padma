package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/padma/internal/platform/db"
)

// ErrNotFound is returned by the single-row lookups when nothing matches.
var ErrNotFound = errors.New("query: not found")

// Repo is the read-model repository over the projections.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo returns a Repo reading from pool.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Page is a slice of results plus the cursor to fetch the next (older) page.
// NextCursor is empty when the last page has been reached.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// --- response models (amounts rendered as 0x-hex, matching the library wire form) ---

// Payment is a row of the payments projection.
type Payment struct {
	PaymentHash string     `json:"payment_hash"`
	Direction   string     `json:"direction"`
	Status      string     `json:"status"`
	Asset       string     `json:"asset,omitempty"`
	Amount      string     `json:"amount,omitempty"`
	Fee         string     `json:"fee,omitempty"`
	Peer        string     `json:"peer,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

// Channel is a row of the channels projection.
type Channel struct {
	ChannelID     string     `json:"channel_id"`
	Peer          string     `json:"peer,omitempty"`
	State         string     `json:"state,omitempty"`
	Capacity      string     `json:"capacity,omitempty"`
	LocalBalance  string     `json:"local_balance,omitempty"`
	RemoteBalance string     `json:"remote_balance,omitempty"`
	OpenedAt      *time.Time `json:"opened_at,omitempty"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
}

// Event is a row of the append-only events store.
type Event struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	FiberRef   string          `json:"fiber_ref,omitempty"`
	Asset      string          `json:"asset,omitempty"`
	Amount     string          `json:"amount,omitempty"`
	Data       json.RawMessage `json:"data"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// StatRow is one aggregation bucket for /stats/payments.
type StatRow struct {
	Asset string `json:"asset"`
	Count int64  `json:"count"`
	Total string `json:"total"` // summed amount as 0x-hex
}

// --- filters ---

// PaymentFilter narrows the payments query. Empty fields are ignored.
type PaymentFilter struct {
	Direction string
	Status    string
	Asset     string
	Peer      string
	From, To  *time.Time
}

// ChannelFilter narrows the channels query.
type ChannelFilter struct {
	State string
	Peer  string
}

// EventFilter narrows the events query.
type EventFilter struct {
	Type     string
	From, To *time.Time
}

// conds accumulates SQL conditions with positional placeholders.
type conds struct {
	where []string
	args  []any
}

// eq adds "col = $n" for a non-empty string value.
func (c *conds) eq(col, val string) {
	if val == "" {
		return
	}
	c.args = append(c.args, val)
	c.where = append(c.where, fmt.Sprintf("%s = $%d", col, len(c.args)))
}

// tsRange adds ">=" / "<=" bounds on a timestamp column.
func (c *conds) tsRange(col string, from, to *time.Time) {
	if from != nil {
		c.args = append(c.args, *from)
		c.where = append(c.where, fmt.Sprintf("%s >= $%d", col, len(c.args)))
	}
	if to != nil {
		c.args = append(c.args, *to)
		c.where = append(c.where, fmt.Sprintf("%s <= $%d", col, len(c.args)))
	}
}

// keyset adds the "(tsCol, idCol) < ($n, $m)" strict-less condition for a cursor,
// matching the DESC ordering (next page = older rows).
func (c *conds) keyset(tsCol, idCol string, cur Cursor) {
	if cur.IsZero() {
		return
	}
	c.args = append(c.args, cur.Time, cur.ID)
	c.where = append(c.where, fmt.Sprintf("(%s, %s) < ($%d, $%d)", tsCol, idCol, len(c.args)-1, len(c.args)))
}

func (c *conds) clause() string {
	if len(c.where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(c.where, " AND ")
}

// clampLimit bounds the page size to [1, 200] with a default of 50.
func clampLimit(n int) int {
	switch {
	case n <= 0:
		return 50
	case n > 200:
		return 200
	default:
		return n
	}
}

func hexOrEmpty(n pgtype.Numeric) string {
	if !n.Valid {
		return ""
	}
	a, err := db.NumericToAmount(n)
	if err != nil {
		return ""
	}
	return a.Hex()
}

func str(p pgtype.Text) string {
	if !p.Valid {
		return ""
	}
	return p.String
}

func tptr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
