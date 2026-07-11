-- +goose Up
-- Webhook subscriptions + delivery outbox + attempt log (#10).

CREATE TABLE webhook_endpoints (
  id          TEXT PRIMARY KEY,
  merchant_id TEXT,
  url         TEXT NOT NULL,
  secret      TEXT NOT NULL,
  event_types TEXT[] NOT NULL,
  active      BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Outbox (implements the library's webhook.Store).
CREATE TABLE webhook_deliveries (
  id              TEXT PRIMARY KEY,
  endpoint_id     TEXT NOT NULL REFERENCES webhook_endpoints(id),
  event_id        TEXT NOT NULL REFERENCES events(id),
  url             TEXT NOT NULL,               -- snapshot at enqueue
  secret          TEXT NOT NULL,               -- snapshot at enqueue
  payload         JSONB NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending',  -- pending|delivering|succeeded|failed|dead
  attempts        INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error      TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries (status, next_attempt_at);

CREATE TABLE webhook_delivery_log (
  delivery_id  TEXT,
  attempt_no   INT,
  http_status  INT,
  response_ms  INT,
  error        TEXT,
  attempted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE webhook_delivery_log;
DROP TABLE webhook_deliveries;
DROP TABLE webhook_endpoints;
