package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/kanaka/events"
	"github.com/karnikara/kanaka/ingest"

	"github.com/karnikara/padma/internal/platform/config"
	"github.com/karnikara/padma/internal/platform/fiber"
)

// Run wires an ingest.Source through the Projector and runs until the source
// closes or ctx is canceled. The source is selected by cfg.IngestSource; the
// real polling/pubsub sources are deferred until the store_changes spike, so
// only the "fake" source (seeded from SEED_FILE) is wired today.
func Run(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	src, norm, err := buildSource(cfg)
	if err != nil {
		return err
	}
	p := New(pool)
	runner := ingest.Runner{Source: src, Normalizer: norm, Checkpoint: p}
	return runner.Run(ctx, p.Handle)
}

func buildSource(cfg config.Config) (ingest.Source, events.Normalizer, error) {
	switch cfg.IngestSource {
	case "fake":
		changes, err := loadSeed(os.Getenv("SEED_FILE"))
		if err != nil {
			return nil, nil, err
		}
		return &fiber.FakeSource{Changes: changes}, fiber.StubNormalizer{}, nil
	case "polling", "pubsub":
		return nil, nil, fmt.Errorf("indexer: INGEST_SOURCE=%s needs the store_changes spike (concrete Source/Normalizer not yet in the library)", cfg.IngestSource)
	default:
		return nil, nil, fmt.Errorf("indexer: unknown INGEST_SOURCE %q", cfg.IngestSource)
	}
}

// seedRecord is one line of a seed file: a raw change whose body is a JSON event.
type seedRecord struct {
	Seq   int64        `json:"seq"`
	Event events.Event `json:"event"`
}

// loadSeed reads a JSON array of {seq, event} records into RawChanges. An empty
// path yields no changes (the indexer then exits immediately).
func loadSeed(path string) ([]events.RawChange, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("indexer: read seed %s: %w", path, err)
	}
	var recs []seedRecord
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil, fmt.Errorf("indexer: parse seed %s: %w", path, err)
	}
	changes := make([]events.RawChange, 0, len(recs))
	for _, rec := range recs {
		body, err := json.Marshal(rec.Event)
		if err != nil {
			return nil, fmt.Errorf("indexer: encode seed event: %w", err)
		}
		changes = append(changes, events.RawChange{Seq: rec.Seq, Body: body})
	}
	return changes, nil
}
