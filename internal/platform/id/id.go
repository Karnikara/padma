// Package id generates identifiers for padma rows. New produces fresh monotonic
// ULIDs; Derive produces a stable ULID from input parts so at-least-once replays
// (of the same event or delivery) map to the same primary key and dedup via
// ON CONFLICT DO NOTHING.
package id

import (
	"crypto/rand"
	"crypto/sha256"

	"github.com/oklog/ulid/v2"
)

// New returns a fresh, lexicographically sortable ULID string.
func New() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// Derive returns a deterministic ULID string derived from namespace and parts.
// The same inputs always yield the same id; distinct inputs (including a
// distinct namespace) yield distinct ids with overwhelming probability. Use it
// for outbox rows (event id, delivery id) so replays are idempotent.
func Derive(namespace string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(namespace))
	for _, p := range parts {
		h.Write([]byte{0}) // separator so ("a","b") != ("ab")
		h.Write([]byte(p))
	}
	sum := h.Sum(nil)
	var u ulid.ULID
	copy(u[:], sum[:len(u)])
	return u.String()
}

// DeriveWithPrefix is Derive with a human-readable prefix (e.g. "dlv_") for ids
// stored as opaque text. The ULID portion stays deterministic.
func DeriveWithPrefix(prefix, namespace string, parts ...string) string {
	return prefix + Derive(namespace, parts...)
}
