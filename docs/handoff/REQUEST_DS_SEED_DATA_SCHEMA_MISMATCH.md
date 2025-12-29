# REQUEST: Data Storage - Seed Data Schema Mismatch Fix

**Date**: 2025-12-11
**From**: HAPI Team (Triage Session)
**To**: Data Storage Team
**Status**: ✅ **RESOLVED** (2025-12-11)
**Resolution**: See [RESPONSE_DS_SEED_DATA_SCHEMA_MISMATCH.md](./RESPONSE_DS_SEED_DATA_SCHEMA_MISMATCH.md)
**Priority**: P2 (Blocks integration tests)
**Blocking**: HAPI integration tests that require workflow catalog data

---

## Issue Summary

The test seed data file uses deprecated column names that were renamed in migration 011.

### Error
```
ERROR:  column "alert_fingerprint" of relation "resource_action_traces" does not exist
LINE 3:     alert_name, alert_fingerprint, alert_severity, alert_lab...
                        ^
```

### Root Cause
- **Migration 011** (`011_rename_alert_to_signal.sql`) renamed columns from `alert_*` to `signal_*`
- **Seed data** (`migrations/testdata/seed_test_data.sql`) was NOT updated to match

---

## Affected Files

| File | Issue |
|------|-------|
| `migrations/testdata/seed_test_data.sql` | Uses deprecated `alert_*` column names |

---

## Required Changes

Update `seed_test_data.sql` lines 47+ to use post-migration column names:

| Old Name | New Name |
|----------|----------|
| `alert_fingerprint` | `signal_fingerprint` |
| `alert_name` | `signal_name` |
| `alert_severity` | `signal_severity` |
| `alert_labels` | `signal_labels` |

### Example Fix
```sql
-- Before (line 47):
INSERT INTO resource_action_traces (
    action_id, action_history_id, action_timestamp,
    alert_name, alert_fingerprint, alert_severity, alert_labels,
    ...

-- After:
INSERT INTO resource_action_traces (
    action_id, action_history_id, action_timestamp,
    signal_name, signal_fingerprint, signal_severity, signal_labels,
    ...
```

---

## Impact

### Blocked Tests
- `tests/integration/test_workflow_catalog_data_storage.py` (all tests)
- `tests/integration/test_workflow_catalog_data_storage_integration.py` (all tests)
- `tests/integration/test_workflow_catalog_container_image_integration.py` (all tests)

### Services Affected
- HAPI integration tests
- Any service that uses `seed_test_data.sql` for test setup

---

## Verification

After fix, run:
```bash
# Load seed data
podman exec -i <postgres_container> psql -U slm_user -d action_history < migrations/testdata/seed_test_data.sql

# Verify data loaded
podman exec <postgres_container> psql -U slm_user -d action_history -c "SELECT COUNT(*) FROM remediation_workflow_catalog;"
# Should return > 0 rows
```

---

## Timeline Request

This is blocking HAPI V1.0 integration test validation. Please address when possible.

---

**Triage Confidence**: 95%
- Clear schema mismatch identified
- Migration 011 authority confirmed
- Fix is straightforward column rename

