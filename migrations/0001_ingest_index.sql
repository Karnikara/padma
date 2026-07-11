-- +goose Up
-- Ingest checkpoint + append-only event store + read-model projections (#11).

CREATE TABLE ingest_checkpoint (
  id         INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  last_seq   BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE events (
  id          TEXT PRIMARY KEY,          -- evt_ULID from the library
  type        TEXT NOT NULL,
  fiber_ref   TEXT,
  asset       TEXT,
  amount      NUMERIC,
  data        JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  checkpoint  BIGINT NOT NULL
);
CREATE INDEX events_type_occurred_idx ON events (type, occurred_at);
CREATE INDEX events_fiber_ref_idx ON events (fiber_ref);

CREATE TABLE payments (
  payment_hash TEXT PRIMARY KEY,
  direction    TEXT NOT NULL,            -- sent | received
  status       TEXT NOT NULL,            -- created | succeeded | failed
  asset        TEXT,
  amount       NUMERIC,
  fee          NUMERIC,
  peer         TEXT,
  created_at   TIMESTAMPTZ,
  updated_at   TIMESTAMPTZ
);

CREATE TABLE channels (
  channel_id     TEXT PRIMARY KEY,
  peer           TEXT,
  state          TEXT,
  capacity       NUMERIC,
  local_balance  NUMERIC,
  remote_balance NUMERIC,
  opened_at      TIMESTAMPTZ,
  closed_at      TIMESTAMPTZ
);

CREATE TABLE invoices (
  payment_hash TEXT PRIMARY KEY,
  status       TEXT,
  asset        TEXT,
  amount       NUMERIC,
  created_at   TIMESTAMPTZ,
  settled_at   TIMESTAMPTZ
);

-- +goose Down
DROP TABLE invoices;
DROP TABLE channels;
DROP TABLE payments;
DROP TABLE events;
DROP TABLE ingest_checkpoint;
