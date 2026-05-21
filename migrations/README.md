# Goose SQL migrations (`migrations/`)

## Transaction behavior and index safety

[Goose](https://github.com/pressly/goose) runs each migration variant inside a PostgreSQL transaction (see also `StatementBegin` / `StatementEnd` blocks in migrations).

PostgreSQL forbids **`CREATE INDEX CONCURRENTLY`** (and **`DROP INDEX CONCURRENTLY`**) inside a transaction block.

Implications:

- Prefer plain `CREATE INDEX` in Goose migrations when tables are empty or expected to remain small (e.g. fresh installs in CI/KinD).
- For **production deployments with large tables**, long-running exclusive locks from non-concurrent index builds can cause unacceptable outage. Mitigation: ship the DDL as a **separate operational step** (job or manual playbook) executed **outside Goose’s transaction envelope**, using `CREATE INDEX CONCURRENTLY`, then optionally add an idempotent no-op migration for bookkeeping if your process requires migration version parity across environments.

Historical note: archived migration `v0-archived/023_add_event_hashing.sql` explicitly documents dropping `CONCURRENTLY` because transaction-bound migrations cannot use it (lines 55–57 reference E2E/atomic apply vs. Postgres rules).

---

## Timestamp types (audit of `001`–`010`)

Across numbered migrations **`001_v1_schema.sql`** through **`010_retention_default_alignment.sql`**:

| Location | Columns / literals | Finding |
|---------|---------------------|--------|
| `001_v1_schema.sql` | Most timestamps | **`TIMESTAMP WITH TIME ZONE`** (consistent for instants-in-UTC semantics). |
| `001_v1_schema.sql` | `audit_events.legal_hold_placed_at` | **`TIMESTAMP`** without TZ — migrated to **`TIMESTAMP WITH TIME ZONE`** via **`011_timestamp_timezone_alignment.sql`**. |
| `001_v1_schema.sql` | `action_type_taxonomy.created_at`, `updated_at` | **`TIMESTAMP NOT NULL`** (no TZ); consider a future alignment migration if audit/compliance requires zone-aware semantics. |
| `001_v1_schema.sql` | `audit_retention_policies.created_at`, `updated_at` | **`TIMESTAMP NOT NULL`** (no TZ); optional future alignment as above. |
| `002`–`010` | — | No new plain-`TIMESTAMP` column definitions observed; **`010`** only adjusts **`audit_events.retention_days`** default. |

---

## Type migration playbook

When changing a column type (e.g. `TIMESTAMP` → `TIMESTAMP WITH TIME ZONE`):

1. **New migration file**: Create `NNN_<description>.sql` with `ALTER TABLE ... ALTER COLUMN ... TYPE ... USING ...`.
2. **`USING` clause**: Always include a `USING` expression for safe conversion (e.g. `USING col AT TIME ZONE 'UTC'` for TZ alignment).
3. **Dependent code**: Search for the old type in repository code, builders, and test fixtures. Update Go types if the Go↔SQL mapping changes (e.g. `time.Time` already handles both, but `sql.NullTime` may need attention).
4. **Sync embedded copy**: Update `pkg/shared/assets/migrations/` to mirror the new migration.
5. **Validate**: Run `go build ./...` and the full unit test suite to catch any column-count or type-assertion mismatches in mock rows.

Reference: migration `011_timestamp_timezone_alignment.sql` follows this pattern for `legal_hold_placed_at`.

---

## Embedded copies

Operational and test code may load SQL from **`pkg/shared/assets/migrations/`**. Keep that tree in sync with top-level **`migrations/`** when adding new versions (same filename and body).
