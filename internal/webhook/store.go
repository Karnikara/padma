// Package webhook is padma's webhook layer: the Postgres delivery outbox that
// implements the library's webhook.Store, subscription CRUD, and the dispatcher
// wiring. The retry/backoff/dead-letter engine itself lives in the library
// (kanaka/webhook.Dispatcher); this package only persists and moves deliveries.
package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	kwebhook "github.com/karnikara/kanaka/webhook"
)

// defaultLeaseTTL is how long a claimed ('delivering') delivery is considered
// in-flight before Requeue may reclaim it (crash recovery).
const defaultLeaseTTL = 5 * time.Minute

// Store is the Postgres implementation of kanaka's webhook.Store, backing the
// webhook_deliveries outbox.
type Store struct {
	pool     *pgxpool.Pool
	leaseTTL time.Duration
}

// NewStore returns a Store over pool with the default in-flight lease.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, leaseTTL: defaultLeaseTTL}
}

// Compile-time assertion that Store satisfies the library interface.
var _ kwebhook.Store = (*Store)(nil)

// ClaimDue atomically claims up to limit due deliveries with SELECT ... FOR
// UPDATE SKIP LOCKED so concurrent dispatcher workers never double-send. It only
// marks them 'delivering' — attempts are incremented at Mark time, keeping the
// library's `attempt = Attempts+1 >= MaxAttempts` dead-letter check correct.
func (s *Store) ClaimDue(ctx context.Context, limit int) ([]kwebhook.Delivery, error) {
	// Push next_attempt_at out by the lease so a claimed row is not "due" again
	// until the lease expires; Requeue reclaims rows whose worker died.
	rows, err := s.pool.Query(ctx, `
		UPDATE webhook_deliveries d
		SET status = 'delivering', next_attempt_at = now() + $2::interval
		FROM (
			SELECT id FROM webhook_deliveries
			WHERE status IN ('pending', 'failed') AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		) c
		WHERE d.id = c.id
		RETURNING d.id, d.endpoint_id, d.event_id, d.url, d.secret, d.payload,
		          d.attempts, d.next_attempt_at,
		          (SELECT type FROM events e WHERE e.id = d.event_id)`,
		limit, fmt.Sprintf("%d seconds", int(s.leaseTTL.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("webhook: claim due: %w", err)
	}
	defer rows.Close()

	var out []kwebhook.Delivery
	for rows.Next() {
		var d kwebhook.Delivery
		if err := rows.Scan(&d.ID, &d.EndpointID, &d.EventID, &d.URL, &d.Secret,
			&d.Payload, &d.Attempts, &d.NextAttemptAt, &d.EventType); err != nil {
			return nil, fmt.Errorf("webhook: scan delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkSucceeded records a successful delivery.
func (s *Store) MarkSucceeded(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		WITH upd AS (
			UPDATE webhook_deliveries SET status = 'succeeded' WHERE id = $1
			RETURNING attempts + 1 AS attempt_no
		)
		INSERT INTO webhook_delivery_log (delivery_id, attempt_no)
		SELECT $1, attempt_no FROM upd`, id)
	if err != nil {
		return fmt.Errorf("webhook: mark succeeded: %w", err)
	}
	return nil
}

// MarkFailed increments the attempt count, schedules the next retry, and logs the
// error.
func (s *Store) MarkFailed(ctx context.Context, id string, nextAttempt time.Time, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		WITH upd AS (
			UPDATE webhook_deliveries
			SET attempts = attempts + 1, status = 'failed',
			    next_attempt_at = $2, last_error = $3
			WHERE id = $1
			RETURNING attempts AS attempt_no
		)
		INSERT INTO webhook_delivery_log (delivery_id, attempt_no, error)
		SELECT $1, attempt_no, $3 FROM upd`, id, nextAttempt, errMsg)
	if err != nil {
		return fmt.Errorf("webhook: mark failed: %w", err)
	}
	return nil
}

// MarkDead moves a delivery to the dead-letter state after the final attempt.
func (s *Store) MarkDead(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		WITH upd AS (
			UPDATE webhook_deliveries
			SET attempts = attempts + 1, status = 'dead'
			WHERE id = $1
			RETURNING attempts AS attempt_no
		)
		INSERT INTO webhook_delivery_log (delivery_id, attempt_no, error)
		SELECT $1, attempt_no, 'max attempts exceeded' FROM upd`, id)
	if err != nil {
		return fmt.Errorf("webhook: mark dead: %w", err)
	}
	return nil
}

// Requeue re-arms deliveries stuck in 'delivering' whose lease has expired (e.g.
// after a dispatcher crash), so at-least-once delivery survives worker death. It
// returns the number of deliveries re-armed.
func (s *Store) Requeue(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = 'pending', next_attempt_at = now()
		WHERE status = 'delivering' AND next_attempt_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("webhook: requeue stuck: %w", err)
	}
	return tag.RowsAffected(), nil
}
