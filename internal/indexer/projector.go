// Package indexer projects the library's semantic events into Postgres. The
// Projector is the ingest.Runner handler and also implements ingest.Checkpoint,
// so all writes for one raw change — the append-only events rows, the read-model
// projections, and the checkpoint advance — commit in a single transaction
// (spec-B.md §3, transactional outbox). Because events.Event carries no seq, the
// Projector buffers each raw change's events and flushes them when the Runner
// calls Save with that change's seq.
package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/kanaka/events"

	"github.com/karnikara/padma/internal/platform/db"
)

// Projector applies events to Postgres transactionally per raw change.
type Projector struct {
	pool *pgxpool.Pool
	buf  []events.Event
}

// New returns a Projector writing to pool.
func New(pool *pgxpool.Pool) *Projector {
	return &Projector{pool: pool}
}

// Handle buffers one event for the current raw change. It is the handler passed
// to ingest.Runner.Run and matches its func(events.Event) error signature (no
// context and no seq are available here — the flush happens in Save).
func (p *Projector) Handle(e events.Event) error {
	p.buf = append(p.buf, e)
	return nil
}

// Load implements ingest.Checkpoint, returning the last committed seq (0 if none).
func (p *Projector) Load(ctx context.Context) (int64, error) {
	var seq int64
	err := p.pool.QueryRow(ctx, "SELECT last_seq FROM ingest_checkpoint WHERE id = 1").Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("indexer: load checkpoint: %w", err)
	}
	return seq, nil
}

// Save implements ingest.Checkpoint. It flushes the buffered events for this raw
// change — inserting event rows, updating projections, and advancing the
// checkpoint — in one transaction, then clears the buffer.
func (p *Projector) Save(ctx context.Context, seq int64) error {
	buf := p.buf
	p.buf = nil
	return db.InTx(ctx, p.pool, func(tx pgx.Tx) error {
		for _, e := range buf {
			if err := p.applyEvent(ctx, tx, e, seq); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO ingest_checkpoint (id, last_seq, updated_at)
			VALUES (1, $1, now())
			ON CONFLICT (id) DO UPDATE SET last_seq = EXCLUDED.last_seq, updated_at = now()`, seq)
		if err != nil {
			return fmt.Errorf("indexer: save checkpoint: %w", err)
		}
		return nil
	})
}

// applyEvent inserts the append-only event row and updates the matching
// projection. Insert uses ON CONFLICT DO NOTHING on the deterministic event id
// so replays are idempotent.
func (p *Projector) applyEvent(ctx context.Context, tx pgx.Tx, e events.Event, seq int64) error {
	asset, amount := assetAmount(e)
	_, err := tx.Exec(ctx, `
		INSERT INTO events (id, type, fiber_ref, asset, amount, data, occurred_at, checkpoint)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO NOTHING`,
		e.ID, string(e.Type), nullText(e.FiberRef), asset, amount, []byte(e.Data), e.CreatedAt, seq)
	if err != nil {
		return fmt.Errorf("indexer: insert event %s: %w", e.ID, err)
	}

	switch e.Type {
	case events.InvoicePaid:
		if err := projectInvoicePaid(ctx, tx, e); err != nil {
			return err
		}
	case events.PaymentReceived:
		if err := projectPaymentReceived(ctx, tx, e); err != nil {
			return err
		}
	case events.ChannelOpened:
		if err := projectChannelOpened(ctx, tx, e); err != nil {
			return err
		}
	default:
		// Other event types are recorded in the events table only; their
		// projections land as the concrete Normalizer defines more payloads.
	}

	// Transactional outbox: fan the event out to matching webhook subscriptions
	// in the same transaction so an event and its deliveries commit atomically.
	return enqueueDeliveries(ctx, tx, e)
}

func projectInvoicePaid(ctx context.Context, tx pgx.Tx, e events.Event) error {
	var d events.InvoicePaidData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		return fmt.Errorf("indexer: decode invoice.paid: %w", err)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO invoices (payment_hash, status, asset, amount, settled_at)
		VALUES ($1, 'paid', $2, $3, $4)
		ON CONFLICT (payment_hash) DO UPDATE SET
			status = 'paid',
			settled_at = EXCLUDED.settled_at,
			asset = COALESCE(invoices.asset, EXCLUDED.asset),
			amount = COALESCE(invoices.amount, EXCLUDED.amount)`,
		string(d.PaymentHash), nullText(d.Asset), db.AmountToNumeric(d.Amount), d.SettledAt)
	if err != nil {
		return fmt.Errorf("indexer: project invoice.paid: %w", err)
	}
	// Instant-finality payment matching: settle any pending merchant order.
	return matchMerchantOrder(ctx, tx, d)
}

func projectPaymentReceived(ctx context.Context, tx pgx.Tx, e events.Event) error {
	var d events.PaymentReceivedData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		return fmt.Errorf("indexer: decode payment.received: %w", err)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO payments (payment_hash, direction, status, asset, amount, peer, created_at, updated_at)
		VALUES ($1, 'received', 'succeeded', $2, $3, $4, $5, $5)
		ON CONFLICT (payment_hash) DO UPDATE SET
			direction = 'received', status = 'succeeded',
			asset = EXCLUDED.asset, amount = EXCLUDED.amount,
			peer = EXCLUDED.peer, updated_at = EXCLUDED.updated_at`,
		string(d.PaymentHash), nullText(d.Asset), db.AmountToNumeric(d.Amount),
		nullText(d.PayerPubkey), e.CreatedAt)
	if err != nil {
		return fmt.Errorf("indexer: project payment.received: %w", err)
	}
	return nil
}

func projectChannelOpened(ctx context.Context, tx pgx.Tx, e events.Event) error {
	var d events.ChannelOpenedData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		return fmt.Errorf("indexer: decode channel.opened: %w", err)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO channels (channel_id, peer, state, capacity, opened_at)
		VALUES ($1, $2, 'open', $3, $4)
		ON CONFLICT (channel_id) DO UPDATE SET
			peer = EXCLUDED.peer, state = 'open',
			capacity = EXCLUDED.capacity, opened_at = EXCLUDED.opened_at`,
		string(d.ChannelID), nullText(d.Peer), db.AmountToNumeric(d.Capacity), e.CreatedAt)
	if err != nil {
		return fmt.Errorf("indexer: project channel.opened: %w", err)
	}
	return nil
}

// assetAmount extracts the asset key and amount for the events row from the typed
// payload, returning nils when the event type carries neither.
func assetAmount(e events.Event) (asset any, amount any) {
	switch e.Type {
	case events.InvoicePaid:
		var d events.InvoicePaidData
		if json.Unmarshal(e.Data, &d) == nil {
			return nullText(d.Asset), db.AmountToNumeric(d.Amount)
		}
	case events.PaymentReceived:
		var d events.PaymentReceivedData
		if json.Unmarshal(e.Data, &d) == nil {
			return nullText(d.Asset), db.AmountToNumeric(d.Amount)
		}
	}
	return nil, nil
}

// nullText maps an empty string to a SQL NULL, otherwise the string itself.
func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}
