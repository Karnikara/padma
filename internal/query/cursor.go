// Package query is padma's read side (#11): filtered, cursor-paginated access to
// the payments/channels/events projections plus asset aggregation the node can't
// do.
package query

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// Cursor is an opaque keyset position: the (Time, ID) of the last row returned.
// Pagination continues strictly after this position, so it is stable under
// concurrent inserts (unlike OFFSET).
type Cursor struct {
	Time time.Time
	ID   string
}

// IsZero reports whether the cursor is the zero (start-of-results) position.
func (c Cursor) IsZero() bool {
	return c.Time.IsZero() && c.ID == ""
}

// Encode renders the cursor as an opaque URL-safe string.
func (c Cursor) Encode() string {
	raw := c.Time.UTC().Format(time.RFC3339Nano) + "|" + c.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor parses a cursor produced by Encode. An empty string decodes to the
// zero cursor (start of results).
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("query: invalid cursor encoding: %w", err)
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return Cursor{}, fmt.Errorf("query: cursor missing separator")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return Cursor{}, fmt.Errorf("query: invalid cursor time: %w", err)
	}
	return Cursor{Time: t, ID: id}, nil
}
