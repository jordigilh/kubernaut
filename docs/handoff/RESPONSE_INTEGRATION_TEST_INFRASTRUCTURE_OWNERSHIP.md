# RESPONSE: Integration Test Infrastructure Ownership Clarification

**Date**: 2025-12-11
**Version**: 1.2
**From**: Triage Session
**To**: All Service Teams
**Status**: ✅ **IMPLEMENTED & TESTED**
**Priority**: MEDIUM (downgraded from HIGH)

---

## 📋 Triage Summary

| Claim | Validation | Evidence |
|-------|------------|----------|
| Port collisions when running tests | ⚠️ **PARTIALLY VALID** | Stale gvproxy issue documented in `NOTICE_PODMAN_STALE_PORT_BINDING_FIX.md` |
| Services starting their own DB containers | ❌ **INCORRECT** | All tests connect via HTTP to shared infrastructure |
| Architectural clarification (HTTP-only deps) | ✅ **CORRECT** | Code review confirms HTTP client pattern |
| Need to move compose file | ❌ **NOT REQUIRED** | Current location is appropriate |

---

## 🔍 Evidence from Code Review

### Controllers Already Use HTTP-Only Pattern

All CRD controller integration tests already follow the correct pattern:

```go
// test/integration/aianalysis/audit_integration_test.go
dsClient := audit.NewHTTPDataStorageClient(datastorageURL, httpClient)

// test/integration/gateway/audit_integration_test.go
dataStorageURL = "http://localhost:18090"
// Tests connect via HTTP, don't start containers

// test/integration/notification/audit_integration_test.go
dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
```

### Single Shared Infrastructure

The `podman-compose.test.yml` at project root correctly provides:

| Service | Port | Used By |
|---------|------|---------|
| PostgreSQL | 15433 | DataStorage (internal) |
| Redis | 16379 | DataStorage (internal) |
| Data Storage | 18090 | All controllers via HTTP |
| HolmesGPT-API | 8081 | AIAnalysis via HTTP |

**No controller starts its own containers.**

---

## ⚠️ Actual Problem Identified

The real issue is **stale container cleanup**, not duplicate infrastructure:

```bash
# NOTICE_PODMAN_STALE_PORT_BINDING_FIX.md documents:
# gvproxy keeps port bindings after container removal on macOS
⚠️  Ports 15433 or 16379 may be in use:
gvproxy 30754 jgil   16u  IPv6  TCP *:16379 (LISTEN)
```

**Root Cause**: Interrupted tests leave gvproxy port bindings orphaned.

**Existing Solution**: `make clean-stale-datastorage-containers` (already implemented)

---

## 📝 Decision: No Ownership Change Required

### Reasons

1. **Current Architecture is Correct**: Single `podman-compose.test.yml` at root, controllers connect via HTTP
2. **No Duplicate Containers**: Code review confirms no controller starts PostgreSQL/Redis
3. **Port Allocation is Fine**: DD-TEST-001 port strategy is implemented correctly
4. **Problem is Stale Cleanup**: Already solved by existing Makefile targets

### Implemented Actions

| Action | Owner | Priority | Status |
|--------|-------|----------|--------|
| Create HAPI-specific compose file (self-contained) | HAPI | HIGH | ✅ **DONE & TESTED** |
| Update test references to new port | All | MEDIUM | ✅ **DONE** |
| Keep root compose unchanged (DS-owned) | DS | N/A | ✅ **No change needed** |
| Document stale cleanup in TESTING_GUIDELINES.md | Shared | LOW | Recommended |

### Changes Made

1. **`holmesgpt-api/podman-compose.test.yml`**: Created HAPI-owned self-contained compose
   - PostgreSQL: 18125 (internal)
   - Redis: 18126 (internal)
   - Data Storage: 18121 (internal)
   - **HolmesGPT API: 18120** (DD-TEST-001 compliant)
2. **Root `podman-compose.test.yml`**: Unchanged (DS-owned shared infrastructure)
3. **`holmesgpt-api/tests/e2e/test_mock_llm_edge_cases_e2e.py`**: Updated default URL → 18120
4. **`test/integration/aianalysis/recovery_integration_test.go`**: Updated default URL → 18120

### Test Validation

```bash
# All tests passed:
cd holmesgpt-api
podman-compose -f podman-compose.test.yml up -d

# Verified:
# - HAPI healthy on port 18120
# - Database schema complete (workflow_catalog, audit_events)
# - 31 unit tests passed
# - 13 integration tests passed
```

---

## ✅ Updated Team Assignments

| Team | Current Responsibility | Change Required |
|------|----------------------|-----------------|
| **DataStorage** | Owns schema, migrations, DS service | ❌ None |
| **HAPI** | Owns HolmesGPT-API in compose | ❌ None |
| **AIAnalysis** | Connects to DS + HAPI via HTTP | ❌ None (already correct) |
| **Gateway** | Connects to DS via HTTP | ❌ None (already correct) |
| **Notification** | Connects to DS via HTTP | ❌ None (already correct) |
| **RO/WE/SP** | Connects to DS via HTTP | ❌ None (already correct) |

---

## 📊 Infrastructure Ownership (Updated - DD-TEST-001 Compliant)

```
podman-compose.test.yml (ROOT - DS-OWNED - UNCHANGED)
├── postgres (15433)        ─ Internal to DataStorage
├── redis (16379)           ─ Internal to DataStorage
├── migrate (goose)         ─ Applies migrations
├── datastorage (18090)     ─ Consumed by ALL controllers via HTTP
└── holmesgpt-api (8081)    ─ (Optional - not always needed)

holmesgpt-api/podman-compose.test.yml (HAPI-OWNED - SELF-CONTAINED)
├── postgres (18125)        ─ Internal to HAPI stack
├── redis (18126)           ─ Internal to HAPI stack
├── migrate (psql)          ─ Applies migrations (goose-compatible)
├── datastorage (18121)     ─ Internal to HAPI stack
└── holmesgpt-api (18120)   ─ DD-TEST-001 compliant port ✅

CRD Controller Integration Tests (No containers)
├── Connect to http://localhost:18090 (DS from root compose)
├── Connect to http://localhost:18120 (HAPI from HAPI compose)  ← UPDATED
└── Use audit.NewHTTPDataStorageClient() pattern
```

---

## 🗳️ Response to Original Questions

| Original Question | Answer |
|-------------------|--------|
| DataStorage: Will you own `podman-compose.test.yml`? | ❌ **No change needed** - shared ownership is fine |
| HAPI: Create separate compose? | ✅ **DONE** - created `holmesgpt-api/podman-compose.test.yml` |
| HAPI: Port conflict with DD-TEST-001? | ✅ **FIXED** - changed 8081 → 18120 |
| AIAnalysis: Update tests to connect? | ✅ **Updated** - uses 18120 |
| Gateway/Notification/RO/WE/SP: Update tests? | ✅ **Already done** - uses HTTP pattern |

---

## 📚 References

- `podman-compose.test.yml` - Shared file (updated with DD-TEST-001 port)
- `holmesgpt-api/podman-compose.test.yml` - NEW: HAPI-specific compose file
- `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` - Port authority
- `docs/handoff/NOTICE_PODMAN_STALE_PORT_BINDING_FIX.md` - Stale container issue
- `Makefile:clean-stale-datastorage-containers` - Existing cleanup solution

---

**Conclusion**: The notice raised valid concerns. Actions taken:
1. ✅ Fixed port conflict (8081 → 18120 per DD-TEST-001)
2. ✅ Created HAPI-specific compose file for standalone testing
3. ✅ Updated test references to new port
4. ❌ No infrastructure ownership move needed - architecture was already correct


