// Package fiber holds padma's temporary stand-ins for the library's still-
// deferred node plumbing: a FakeSource that replays seeded raw changes and a
// StubNormalizer that treats each raw body as an already-formed semantic event.
//
// TEMPORARY: the real ingest.Source (polling/pubsub) and the concrete
// events.Normalizer belong in the kanaka library and land after the
// store_changes spike. They plug in behind INGEST_SOURCE without touching any
// downstream package — this file is the single seam that isolates that gap.
package fiber

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/karnikara/kanaka/events"
)

// StubNormalizer decodes each RawChange.Body as a JSON-encoded events.Event.
// This is padma's own controlled wire format for seeded/test data, letting the
// full indexer pipeline run against real event types and payloads before the
// node's raw payload shape is known.
type StubNormalizer struct{}

// Normalize implements events.Normalizer.
func (StubNormalizer) Normalize(raw events.RawChange) ([]events.Event, error) {
	var e events.Event
	if err := json.Unmarshal(raw.Body, &e); err != nil {
		return nil, fmt.Errorf("fiber: stub normalize seq %d: %w", raw.Seq, err)
	}
	return []events.Event{e}, nil
}

// FakeSource replays a fixed slice of raw changes in order, then closes the
// stream. It honors context cancellation between sends. It implements
// ingest.Source.
type FakeSource struct {
	Changes []events.RawChange
}

// Subscribe implements ingest.Source.
func (s *FakeSource) Subscribe(ctx context.Context) (<-chan events.RawChange, error) {
	out := make(chan events.RawChange)
	go func() {
		defer close(out)
		for _, c := range s.Changes {
			// Honor cancellation deterministically: select picks randomly among
			// ready cases, so check the context before racing it against the send.
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- c:
			}
		}
	}()
	return out, nil
}
