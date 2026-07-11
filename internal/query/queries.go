package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Payments returns a filtered, cursor-paginated page of payments, newest first.
func (r *Repo) Payments(ctx context.Context, f PaymentFilter, cur Cursor, limit int) (Page[Payment], error) {
	limit = clampLimit(limit)
	c := &conds{}
	c.eq("direction", f.Direction)
	c.eq("status", f.Status)
	c.eq("asset", f.Asset)
	c.eq("peer", f.Peer)
	c.tsRange("created_at", f.From, f.To)
	c.keyset("created_at", "payment_hash", cur)

	sql := `SELECT payment_hash, direction, status, asset, amount, fee, peer, created_at, updated_at
		FROM payments` + c.clause() +
		fmt.Sprintf(" ORDER BY created_at DESC, payment_hash DESC LIMIT %d", limit+1)

	rows, err := r.pool.Query(ctx, sql, c.args...)
	if err != nil {
		return Page[Payment]{}, fmt.Errorf("query: payments: %w", err)
	}
	defer rows.Close()

	var items []Payment
	for rows.Next() {
		var p Payment
		var asset, peer pgtype.Text
		var amount, fee pgtype.Numeric
		var created, updated pgtype.Timestamptz
		if err := rows.Scan(&p.PaymentHash, &p.Direction, &p.Status, &asset, &amount, &fee, &peer, &created, &updated); err != nil {
			return Page[Payment]{}, fmt.Errorf("query: scan payment: %w", err)
		}
		p.Asset, p.Peer = str(asset), str(peer)
		p.Amount, p.Fee = hexOrEmpty(amount), hexOrEmpty(fee)
		p.CreatedAt, p.UpdatedAt = tptr(created), tptr(updated)
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return Page[Payment]{}, err
	}
	return paginate(items, limit, func(p Payment) Cursor {
		return Cursor{Time: deref(p.CreatedAt), ID: p.PaymentHash}
	}), nil
}

// PaymentByHash returns a single payment or ErrNotFound.
func (r *Repo) PaymentByHash(ctx context.Context, hash string) (Payment, error) {
	var p Payment
	var asset, peer pgtype.Text
	var amount, fee pgtype.Numeric
	var created, updated pgtype.Timestamptz
	err := r.pool.QueryRow(ctx,
		`SELECT payment_hash, direction, status, asset, amount, fee, peer, created_at, updated_at
		 FROM payments WHERE payment_hash = $1`, hash).
		Scan(&p.PaymentHash, &p.Direction, &p.Status, &asset, &amount, &fee, &peer, &created, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, fmt.Errorf("query: payment by hash: %w", err)
	}
	p.Asset, p.Peer = str(asset), str(peer)
	p.Amount, p.Fee = hexOrEmpty(amount), hexOrEmpty(fee)
	p.CreatedAt, p.UpdatedAt = tptr(created), tptr(updated)
	return p, nil
}

// Channels returns a filtered, cursor-paginated page of channels, newest first.
func (r *Repo) Channels(ctx context.Context, f ChannelFilter, cur Cursor, limit int) (Page[Channel], error) {
	limit = clampLimit(limit)
	c := &conds{}
	c.eq("state", f.State)
	c.eq("peer", f.Peer)
	c.keyset("opened_at", "channel_id", cur)

	sql := `SELECT channel_id, peer, state, capacity, local_balance, remote_balance, opened_at, closed_at
		FROM channels` + c.clause() +
		fmt.Sprintf(" ORDER BY opened_at DESC, channel_id DESC LIMIT %d", limit+1)

	rows, err := r.pool.Query(ctx, sql, c.args...)
	if err != nil {
		return Page[Channel]{}, fmt.Errorf("query: channels: %w", err)
	}
	defer rows.Close()

	var items []Channel
	for rows.Next() {
		var ch Channel
		var peer, state pgtype.Text
		var capa, local, remote pgtype.Numeric
		var opened, closed pgtype.Timestamptz
		if err := rows.Scan(&ch.ChannelID, &peer, &state, &capa, &local, &remote, &opened, &closed); err != nil {
			return Page[Channel]{}, fmt.Errorf("query: scan channel: %w", err)
		}
		ch.Peer, ch.State = str(peer), str(state)
		ch.Capacity, ch.LocalBalance, ch.RemoteBalance = hexOrEmpty(capa), hexOrEmpty(local), hexOrEmpty(remote)
		ch.OpenedAt, ch.ClosedAt = tptr(opened), tptr(closed)
		items = append(items, ch)
	}
	if err := rows.Err(); err != nil {
		return Page[Channel]{}, err
	}
	return paginate(items, limit, func(ch Channel) Cursor {
		return Cursor{Time: deref(ch.OpenedAt), ID: ch.ChannelID}
	}), nil
}

// ChannelByID returns a single channel or ErrNotFound.
func (r *Repo) ChannelByID(ctx context.Context, id string) (Channel, error) {
	var ch Channel
	var peer, state pgtype.Text
	var capa, local, remote pgtype.Numeric
	var opened, closed pgtype.Timestamptz
	err := r.pool.QueryRow(ctx,
		`SELECT channel_id, peer, state, capacity, local_balance, remote_balance, opened_at, closed_at
		 FROM channels WHERE channel_id = $1`, id).
		Scan(&ch.ChannelID, &peer, &state, &capa, &local, &remote, &opened, &closed)
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	if err != nil {
		return Channel{}, fmt.Errorf("query: channel by id: %w", err)
	}
	ch.Peer, ch.State = str(peer), str(state)
	ch.Capacity, ch.LocalBalance, ch.RemoteBalance = hexOrEmpty(capa), hexOrEmpty(local), hexOrEmpty(remote)
	ch.OpenedAt, ch.ClosedAt = tptr(opened), tptr(closed)
	return ch, nil
}

// Events returns a filtered, cursor-paginated page of events, newest first.
func (r *Repo) Events(ctx context.Context, f EventFilter, cur Cursor, limit int) (Page[Event], error) {
	limit = clampLimit(limit)
	c := &conds{}
	c.eq("type", f.Type)
	c.tsRange("occurred_at", f.From, f.To)
	c.keyset("occurred_at", "id", cur)

	sql := `SELECT id, type, fiber_ref, asset, amount, data, occurred_at
		FROM events` + c.clause() +
		fmt.Sprintf(" ORDER BY occurred_at DESC, id DESC LIMIT %d", limit+1)

	rows, err := r.pool.Query(ctx, sql, c.args...)
	if err != nil {
		return Page[Event]{}, fmt.Errorf("query: events: %w", err)
	}
	defer rows.Close()

	var items []Event
	for rows.Next() {
		var e Event
		var fiberRef, asset pgtype.Text
		var amount pgtype.Numeric
		if err := rows.Scan(&e.ID, &e.Type, &fiberRef, &asset, &amount, &e.Data, &e.OccurredAt); err != nil {
			return Page[Event]{}, fmt.Errorf("query: scan event: %w", err)
		}
		e.FiberRef, e.Asset, e.Amount = str(fiberRef), str(asset), hexOrEmpty(amount)
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return Page[Event]{}, err
	}
	return paginate(items, limit, func(e Event) Cursor {
		return Cursor{Time: e.OccurredAt, ID: e.ID}
	}), nil
}

// StatsPayments aggregates payment amounts grouped by asset over an optional time
// range — the SUM/COUNT the node cannot compute.
func (r *Repo) StatsPayments(ctx context.Context, from, to *time.Time) ([]StatRow, error) {
	c := &conds{}
	c.tsRange("created_at", from, to)
	sql := `SELECT COALESCE(asset, 'unknown') AS asset, count(*), COALESCE(sum(amount), 0)
		FROM payments` + c.clause() + ` GROUP BY COALESCE(asset, 'unknown') ORDER BY asset`

	rows, err := r.pool.Query(ctx, sql, c.args...)
	if err != nil {
		return nil, fmt.Errorf("query: stats: %w", err)
	}
	defer rows.Close()

	var out []StatRow
	for rows.Next() {
		var s StatRow
		var total pgtype.Numeric
		if err := rows.Scan(&s.Asset, &s.Count, &total); err != nil {
			return nil, fmt.Errorf("query: scan stat: %w", err)
		}
		s.Total = hexOrEmpty(total)
		out = append(out, s)
	}
	return out, rows.Err()
}

// paginate trims an over-fetched slice (limit+1) to the page size and derives the
// next cursor from the last kept row.
func paginate[T any](items []T, limit int, cursorOf func(T) Cursor) Page[T] {
	if len(items) > limit {
		last := items[limit-1]
		return Page[T]{Items: items[:limit], NextCursor: cursorOf(last).Encode()}
	}
	return Page[T]{Items: items}
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
