package id

import "testing"

func TestNewIsUniqueAndWellFormed(t *testing.T) {
	a, b := New(), New()
	if len(a) != 26 {
		t.Fatalf("ULID length = %d, want 26 (%q)", len(a), a)
	}
	if a == b {
		t.Fatal("two New() calls returned the same id")
	}
}

func TestDeriveIsDeterministic(t *testing.T) {
	x := Derive("delivery", "evt_1", "ep_1")
	y := Derive("delivery", "evt_1", "ep_1")
	if x != y {
		t.Fatalf("Derive not deterministic: %q != %q", x, y)
	}
	if len(x) != 26 {
		t.Fatalf("derived id length = %d, want 26 (%q)", len(x), x)
	}
}

func TestDeriveDistinguishesInputs(t *testing.T) {
	base := Derive("delivery", "evt_1", "ep_1")
	if Derive("delivery", "evt_1", "ep_2") == base {
		t.Error("different endpoint produced same derived id")
	}
	if Derive("delivery", "evt_2", "ep_1") == base {
		t.Error("different event produced same derived id")
	}
	if Derive("event", "evt_1", "ep_1") == base {
		t.Error("different namespace produced same derived id")
	}
}
