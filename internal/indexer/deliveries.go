package indexer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/karnikara/kanaka/events"

	"github.com/karnikara/padma/internal/platform/id"
)

// endpointSnapshot is the subset of a subscription needed to enqueue a delivery.
type endpointSnapshot struct {
	id     string
	url    string
	secret string
}

// enqueueDeliveries inserts one webhook_deliveries row per active subscription
// whose event_types include e.Type. The delivery id is derived from
// (event, endpoint) so replays dedup via ON CONFLICT, and the url/secret/payload
// are snapshotted at enqueue time. It is a no-op before migration 0002 exists
// (webhook_endpoints absent), so the indexer runs standalone at Gate A.
func enqueueDeliveries(ctx context.Context, tx pgx.Tx, e events.Event) error {
	eps, err := matchingEndpoints(ctx, tx, string(e.Type))
	if err != nil {
		return err
	}
	if len(eps) == 0 {
		return nil
	}

	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("indexer: marshal delivery payload: %w", err)
	}
	for _, ep := range eps {
		did := id.DeriveWithPrefix("dlv_", "delivery", e.ID, ep.id)
		_, err := tx.Exec(ctx, `
			INSERT INTO webhook_deliveries
				(id, endpoint_id, event_id, url, secret, payload, status, next_attempt_at)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, 'pending', now())
			ON CONFLICT (id) DO NOTHING`,
			did, ep.id, e.ID, ep.url, ep.secret, payload)
		if err != nil {
			return fmt.Errorf("indexer: enqueue delivery: %w", err)
		}
	}
	return nil
}

// matchingEndpoints returns active subscriptions for eventType. It tolerates the
// webhook_endpoints table being absent (Gate A, before migration 0002).
func matchingEndpoints(ctx context.Context, tx pgx.Tx, eventType string) ([]endpointSnapshot, error) {
	var exists bool
	if err := tx.QueryRow(ctx,
		"SELECT to_regclass('webhook_endpoints') IS NOT NULL").Scan(&exists); err != nil {
		return nil, fmt.Errorf("indexer: probe webhook_endpoints: %w", err)
	}
	if !exists {
		return nil, nil
	}

	rows, err := tx.Query(ctx,
		`SELECT id, url, secret FROM webhook_endpoints WHERE active AND $1 = ANY(event_types)`, eventType)
	if err != nil {
		return nil, fmt.Errorf("indexer: select endpoints: %w", err)
	}
	defer rows.Close()

	var out []endpointSnapshot
	for rows.Next() {
		var ep endpointSnapshot
		if err := rows.Scan(&ep.id, &ep.url, &ep.secret); err != nil {
			return nil, fmt.Errorf("indexer: scan endpoint: %w", err)
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}
