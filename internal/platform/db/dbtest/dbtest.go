// Package dbtest provides a Postgres pool for integration tests. Tests that use
// it are skipped under `go test -short` or when no test database is configured,
// so the fast unit suite never needs Docker.
//
// Point DATABASE_URL_TEST (or DATABASE_URL) at a disposable Postgres. Each test
// package gets its OWN freshly-created, migrated database (named per process) so
// packages running in parallel under `go test ./...` never truncate each other's
// data. Within a package, Pool truncates all tables before each test.
package dbtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/padma/internal/platform/db"
)

// allTables lists every table to truncate between tests (TRUNCATE ... CASCADE
// handles FK ordering).
var allTables = []string{
	"ingest_checkpoint", "events", "payments", "channels", "invoices",
	"webhook_endpoints", "webhook_deliveries", "webhook_delivery_log",
	"merchants", "merchant_orders", "refunds", "settlements", "reconciliation_runs",
}

var (
	setupOnce sync.Once
	testDSN   string
	setupErr  error
)

// baseDSN resolves the configured test database DSN, or "" if none.
func baseDSN() string {
	if v := os.Getenv("DATABASE_URL_TEST"); v != "" {
		return v
	}
	return os.Getenv("DATABASE_URL")
}

// setup creates a per-process database and migrates it, once per test binary.
func setup(base string) (string, error) {
	setupOnce.Do(func() {
		u, err := url.Parse(base)
		if err != nil {
			setupErr = fmt.Errorf("dbtest: parse DSN: %w", err)
			return
		}
		// Unique per test process so parallel packages don't collide.
		dbName := "padma_test_" + strconv.Itoa(os.Getpid())

		admin := *u
		admin.Path = "/postgres"
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, admin.String())
		if err != nil {
			setupErr = fmt.Errorf("dbtest: connect admin: %w", err)
			return
		}
		defer func() { _ = conn.Close(ctx) }()
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)"); err != nil {
			setupErr = fmt.Errorf("dbtest: drop old: %w", err)
			return
		}
		if _, err := conn.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
			setupErr = fmt.Errorf("dbtest: create: %w", err)
			return
		}

		testDB := *u
		testDB.Path = "/" + dbName
		testDSN = testDB.String()
		if err := db.Migrate(testDSN); err != nil {
			setupErr = fmt.Errorf("dbtest: migrate: %w", err)
		}
	})
	return testDSN, setupErr
}

// Pool returns a migrated, truncated pool on this package's dedicated test
// database, or skips the test when no database is available.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping DB integration test in -short mode")
	}
	base := baseDSN()
	if base == "" {
		t.Skip("set DATABASE_URL_TEST to run DB integration tests")
	}

	dsn, err := setup(base)
	if err != nil {
		t.Fatalf("dbtest: setup: %v", err)
	}
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("dbtest: open: %v", err)
	}
	t.Cleanup(pool.Close)

	Truncate(t, pool)
	return pool
}

// Truncate empties every table, ignoring tables a migration set has not created.
func Truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range allTables {
		_, err := pool.Exec(ctx,
			"DO $$ BEGIN IF to_regclass('"+tbl+"') IS NOT NULL THEN EXECUTE 'TRUNCATE "+tbl+" CASCADE'; END IF; END $$;")
		if err != nil {
			t.Fatalf("dbtest: truncate %s: %v", tbl, err)
		}
	}
}
