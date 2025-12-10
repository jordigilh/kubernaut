# ⚠️ NOTICE: Data Storage Migrate Service Bug - Runs DOWN Migrations

**From**: WorkflowExecution Team
**To**: Data Storage Team
**Date**: December 10, 2025
**Priority**: 🔴 HIGH
**Status**: ⚠️ **REQUIRES FIX**

---

## 📋 Problem Summary

The `migrate` service in `podman-compose.test.yml` runs both UP and DOWN migrations, with **DOWN running last** - effectively dropping all tables that were just created.

---

## 🔍 Root Cause

### The Bug

**File**: `podman-compose.test.yml` (lines 26-29)

```yaml
command:
  - |
    for f in /migrations/*.sql; do
      echo "📄 Applying: $$(basename $$f)"
      # Strip goose directives and apply SQL
      sed -e '/^-- +goose/d' "$$f" | psql -h postgres -U slm_user -d action_history -f - 2>&1 || true
    done
```

### The Problem

The `sed -e '/^-- +goose/d'` command removes goose directive lines but **keeps all SQL**, including:

1. **UP section** (CREATE TABLE statements)
2. **DOWN section** (DROP TABLE statements)

Since DOWN comes after UP in goose migration files, the DROP TABLE statements run **last**, undoing the CREATE statements.

### Evidence

```bash
$ podman logs kubernaut_migrate_1
📄 Applying: 013_create_audit_events_table.sql
CREATE TABLE    # UP runs first
DROP TABLE      # DOWN runs last - undoes the CREATE!
✅ All migrations applied successfully
```

---

## 🛠️ Fix Required

### Option A: Extract Only UP Section (Recommended)

```yaml
command:
  - |
    for f in /migrations/*.sql; do
      echo "📄 Applying: $$(basename $$f)"
      # Extract only the UP section (between '-- +goose Up' and '-- +goose Down')
      sed -n '/-- +goose Up/,/-- +goose Down/p' "$$f" | \
        sed -e '/^-- +goose/d' | \
        psql -h postgres -U slm_user -d action_history -f - 2>&1 || true
    done
```

### Option B: Use Goose Tool

```yaml
migrate:
  image: pressly/goose:latest
  volumes:
    - ./migrations:/migrations
  command: ["-dir", "/migrations", "postgres", "postgres://slm_user:test_password@postgres:5432/action_history?sslmode=disable", "up"]
```

---

## 📊 Impact

| Service | Impact |
|---------|--------|
| **All Integration Tests** | ❌ Fail with "relation does not exist" |
| **WorkflowExecution** | BR-WE-005 audit tests fail |
| **All Services** | Cannot use audit persistence |

---

## ✅ Current Workaround

Teams must manually apply migrations after `podman-compose up`:

```bash
podman exec kubernaut_postgres_1 psql -U slm_user -d action_history -c "
CREATE TABLE IF NOT EXISTS audit_events (...);
CREATE TABLE IF NOT EXISTS audit_events_y2025m12 PARTITION OF audit_events ...;
"
```

---

## 📞 Questions?

Contact: WorkflowExecution Team

---

**Document Version**: 1.0
**Created**: December 10, 2025

