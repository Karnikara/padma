-- +goose Up
-- Merchant backend: merchants, orders, refunds, settlements, reconciliation (#3).

CREATE TABLE merchants (
  id              TEXT PRIMARY KEY,
  name            TEXT,
  api_key_hash    TEXT NOT NULL,
  settlement_asset TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX merchants_api_key_hash_idx ON merchants (api_key_hash);

CREATE TABLE merchant_orders (
  id                   TEXT PRIMARY KEY,
  merchant_id          TEXT NOT NULL REFERENCES merchants(id),
  external_order_id    TEXT,
  invoice_payment_hash TEXT,
  amount               NUMERIC,
  asset                TEXT,
  status               TEXT NOT NULL DEFAULT 'pending',  -- pending|paid|expired|refunded
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at              TIMESTAMPTZ,
  UNIQUE (merchant_id, external_order_id)                -- idempotency
);
CREATE INDEX merchant_orders_invoice_idx ON merchant_orders (invoice_payment_hash);

CREATE TABLE refunds (
  id                 TEXT PRIMARY KEY,
  order_id           TEXT NOT NULL REFERENCES merchant_orders(id),
  amount             NUMERIC,
  asset              TEXT,
  reason             TEXT,
  status             TEXT,
  fiber_payment_hash TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE settlements (
  id           TEXT PRIMARY KEY,
  merchant_id  TEXT NOT NULL,
  period       TEXT,
  gross        NUMERIC,
  fees         NUMERIC,
  net          NUMERIC,
  asset        TEXT,
  status       TEXT,
  generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE reconciliation_runs (
  id                TEXT PRIMARY KEY,
  merchant_id       TEXT NOT NULL,
  period            TEXT,
  matched           INT,
  unmatched_count   INT,
  discrepancy_total NUMERIC,
  report            JSONB,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE reconciliation_runs;
DROP TABLE settlements;
DROP TABLE refunds;
DROP TABLE merchant_orders;
DROP TABLE merchants;
