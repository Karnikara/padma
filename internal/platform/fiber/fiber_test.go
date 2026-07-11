package fiber

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/karnikara/kanaka/events"
)

func rawEvent(t *testing.T, seq int64, e events.Event) events.RawChange {
	t.Helper()
	body, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return events.RawChange{Seq: seq, Body: body}
}

func TestStubNormalizerDecodesEvent(t *testing.T) {
	want := events.Event{ID: "evt_1", Type: events.InvoicePaid, FiberRef: "0xhash"}
	var n StubNormalizer
	got, err := n.Normalize(rawEvent(t, 1, want))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(got) != 1 || got[0].ID != want.ID || got[0].Type != want.Type {
		t.Fatalf("Normalize returned %+v, want single %+v", got, want)
	}
}

func TestStubNormalizerRejectsGarbage(t *testing.T) {
	var n StubNormalizer
	if _, err := n.Normalize(events.RawChange{Seq: 1, Body: []byte("not json")}); err == nil {
		t.Fatal("expected error for non-JSON body, got nil")
	}
}

func TestFakeSourceReplaysInOrderThenCloses(t *testing.T) {
	changes := []events.RawChange{
		rawEvent(t, 1, events.Event{ID: "evt_1", Type: events.InvoiceCreated}),
		rawEvent(t, 2, events.Event{ID: "evt_2", Type: events.InvoicePaid}),
	}
	src := &FakeSource{Changes: changes}

	ch, err := src.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	var seqs []int64
	for raw := range ch {
		seqs = append(seqs, raw.Seq)
	}
	if len(seqs) != 2 || seqs[0] != 1 || seqs[1] != 2 {
		t.Fatalf("got seqs %v, want [1 2] then close", seqs)
	}
}

func TestFakeSourceHonorsContextCancel(t *testing.T) {
	src := &FakeSource{Changes: []events.RawChange{
		rawEvent(t, 1, events.Event{ID: "evt_1"}),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before subscribing

	ch, err := src.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected no delivery on a canceled context")
		}
	case <-time.After(time.Second):
		t.Fatal("channel was not closed on cancellation")
	}
}
