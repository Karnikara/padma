# Contributing to padma

Thanks for your interest in improving padma. This is the **service** that builds
on the [`kanaka`](../kanaka) library. The bar is correctness, a clean layering,
and behavior proven by tests against a real database.

## Ground rules

1. **Service, not library.** padma holds the *policy*: the Postgres schema,
   business rules, and HTTP routes. Generic, node-facing plumbing (RPC types, the
   event model, the webhook engine, the ingest runner) belongs in `kanaka`, not
   here. When in doubt, apply the split:

   > Usable by another Fiber team without caring about your business → `kanaka`.
   > Your specific decision (tables, reconciliation, endpoints) → `padma`.

2. **Respect the layering.** Imports point inward:
   `cmd → {indexer, query, webhook, merchant} → domain`; every package may use
   `platform/*`; `platform/fiber` is the only place the deferred library
   `Source`/`Normalizer` seam lives. Don't reach sideways between feature
   packages, and don't let `platform` depend on a feature.

3. **Test-first (TDD).** No production code without a failing test first.
   - Write the smallest test that expresses the desired behavior.
   - Run it; watch it fail for the right reason.
   - Write the minimal code to pass.
   - Refactor with the tests green.

4. **Money never touches float.** Amounts are `fibertypes.Amount` in memory and
   `NUMERIC` in the database, bridged only through `platform/db.AmountToNumeric` /
   `NumericToAmount`. Never format an amount as its `0x`-hex wire string into a
   `NUMERIC` column, and never introduce a `float` in a money path.

5. **New dependencies need a reason.** The runtime set is intentionally small
   (pgx, chi, goose, excelize, ulid). Justify additions in the PR.

## Database changes

- Schema changes are **goose migrations** in `migrations/` (`NNNN_name.sql`,
  embedded via `//go:embed`). Add a new numbered file; never edit an applied one.
- Every migration has a working `-- +goose Down`.
- Statuses are `TEXT`, not `ENUM`; every table carries audit timestamps.

## Local workflow

```bash
make check          # fmt-check + vet + lint + fast unit tests (no DB)
make ci             # build + vet + lint + test-race

# integration tests need Postgres (each test package gets its own throwaway db):
docker run -d --rm --name padma-pg \
  -e POSTGRES_PASSWORD=padma -e POSTGRES_DB=padma -p 55432:5432 postgres:16-alpine
export DATABASE_URL="postgres://postgres:padma@127.0.0.1:55432/padma?sslmode=disable"
go test -race ./...
```

DB-backed tests use the `dbtest` helper, which skips under `-short` or when no
`DATABASE_URL`/`DATABASE_URL_TEST` is set — so `make check` stays fast and
Docker-free. A change is ready when `make ci` and the full `go test -race ./...`
are clean.

## Pull requests

- One focused change per PR. Describe *what* and *why*.
- Include tests for new behavior and for any bug you fix (a failing test that
  reproduces the bug, then the fix). DB behavior gets a `dbtest`-backed
  integration test; pure logic gets a fast unit test.
- Update `README.md`, package doc comments, and the migration set when you change
  the schema or the public HTTP surface.
- Keep exported identifiers documented — a doc comment that reads as a sentence
  starting with the identifier's name.

## Commit messages

Short imperative subject (“Add reconciliation endpoint”), with a body explaining
the reasoning when it isn't obvious from the diff.

## Reporting bugs

Open an issue with: what you expected, what happened, the smallest reproduction
(ideally a failing test), and your Go, Postgres, and node versions.

## Code of Conduct

Participation is governed by our [Code of Conduct](CODE_OF_CONDUCT.md).
