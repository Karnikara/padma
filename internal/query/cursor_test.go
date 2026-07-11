package query

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 9, 12, 30, 45, 123456789, time.UTC)
	c := Cursor{Time: ts, ID: "evt_abc"}

	enc := c.Encode()
	if enc == "" {
		t.Fatal("Encode returned empty string")
	}
	got, err := DecodeCursor(enc)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if !got.Time.Equal(c.Time) || got.ID != c.ID {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, c)
	}
}

func TestDecodeCursorEmptyIsZero(t *testing.T) {
	got, err := DecodeCursor("")
	if err != nil {
		t.Fatalf("DecodeCursor(\"\"): %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("empty cursor should be zero, got %+v", got)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	if _, err := DecodeCursor("!!!not-base64!!!"); err == nil {
		t.Fatal("expected error for malformed cursor, got nil")
	}
	if _, err := DecodeCursor("Zm9vYmFy"); err == nil { // "foobar", no separator
		t.Fatal("expected error for cursor without separator, got nil")
	}
}
