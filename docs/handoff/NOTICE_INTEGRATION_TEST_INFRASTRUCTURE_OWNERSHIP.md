# NOTICE: Integration Test Infrastructure Ownership Clarification

**Date**: 2025-12-11
**Version**: 1.0
**From**: AIAnalysis Team (Triage)
**To**: All Service Teams
**Status**: 🟡 **DECISION REQUIRED**
**Priority**: HIGH

---

## 📋 Summary

**Issue**: The shared `podman-compose.test.yml` is causing port collisions when multiple services run integration tests in parallel. Services are incorrectly starting PostgreSQL/Redis containers they don't actually need.

**Root Cause**: Services are starting their own database containers when they only need HTTP access to Data Storage.

---

## 🎯 Architectural Clarification

### Service Dependencies (Actual vs Assumed)

| Service | Assumed Dependencies | **Actual Dependencies** |
|---------|---------------------|-------------------------|
| **DataStorage** | PostgreSQL, Redis | ✅ PostgreSQL, Redis (owns the data) |
| **AIAnalysis** | PostgreSQL, Redis, DS, HAPI | ✅ **DS HTTP API**, **HAPI HTTP API** only |
| **Gateway** | PostgreSQL, Redis, DS | ✅ **DS HTTP API** only |
| **Notification** | PostgreSQL, Redis, DS | ✅ **DS HTTP API** only |
| **RO/WE/SP** | PostgreSQL, Redis, DS | ✅ **DS HTTP API** only |

### Key Insight

**Only DataStorage** needs direct database access:
- Tests vector queries against PostgreSQL + pgvector
- Tests Redis caching and DLQ behavior
- **Owns `podman-compose.test.yml`**

**All CRD Controllers** need HTTP APIs:
- Data Storage HTTP API for audit (`:18090`)
- HAPI HTTP API for AI analysis (`:8081` - AIAnalysis only)
- **Do NOT need their own PostgreSQL/Redis containers**

---

## 🏗️ Proposed Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ DataStorage Service (OWNER of podman-compose.test.yml)      │
│                                                             │
│   podman-compose.test.yml:                                  │
│     - PostgreSQL (:15433) - vector database                 │
│     - Redis (:16379) - caching, DLQ                         │
│     - Data Storage (:18090) - HTTP API                      │
│     - Goose migrations                                      │
│                                                             │
│   Tests: Vector queries, caching, persistence, migrations   │
└─────────────────────────────────────────────────────────────┘
                              ↓ HTTP API (:18090)
┌─────────────────────────────────────────────────────────────┐
│ CRD Controller Integration Tests                            │
│ (AIAnalysis, Gateway, Notification, RO, WE, SP)             │
│                                                             │
│   Infrastructure: envtest only                              │
│   External: Connect to running Data Storage HTTP API        │
│                                                             │
│   NO PostgreSQL containers needed                           │
│   NO Redis containers needed                                │
└─────────────────────────────────────────────────────────────┘
                              ↓ (AIAnalysis only)
┌─────────────────────────────────────────────────────────────┐
│ HAPI Service                                                │
│                                                             │
│   holmesgpt-api/podman-compose.yml (HAPI-owned):            │
│     - HolmesGPT-API (:8081) - AI analysis                   │
│     - MOCK_LLM_MODE=true for tests                          │
│                                                             │
│   Only AIAnalysis integration tests need this               │
└─────────────────────────────────────────────────────────────┘
```

---

## 📝 Required Changes

### 1. Move `podman-compose.test.yml` to DataStorage

| Current | Proposed |
|---------|----------|
| `podman-compose.test.yml` (root) | `test/integration/datastorage/podman-compose.yml` |

### 2. Create HAPI-specific compose (if needed)

| Service | Compose File |
|---------|--------------|
| **HAPI** | `holmesgpt-api/podman-compose.test.yml` |

### 3. Update CRD Controller Tests

| Service | Current | Proposed |
|---------|---------|----------|
| **AIAnalysis** | Starts own PostgreSQL | Connect to running DS + HAPI |
| **Gateway** | Starts own PostgreSQL/Redis | Connect to running DS |
| **Notification** | Starts own PostgreSQL/Redis | Connect to running DS |
| **RO/WE/SP** | Starts own PostgreSQL | Connect to running DS |

### 4. Update Documentation

| File | Change |
|------|--------|
| `TESTING_GUIDELINES.md` | Clarify DS ownership of podman-compose |
| Service test docs | Remove "start postgres/redis" instructions |
| Integration test comments | Update startup commands |

---

## 🔧 Port Allocation (DD-TEST-001 Compliant)

### DataStorage-Owned Ports

| Port | Service | Owner |
|------|---------|-------|
| **15433** | PostgreSQL | DataStorage |
| **16379** | Redis | DataStorage |
| **18090** | Data Storage API | DataStorage |
| **19090** | Data Storage Metrics | DataStorage |

### HAPI-Owned Ports

| Port | Service | Owner |
|------|---------|-------|
| **8081** | HolmesGPT-API | HAPI Team |

### No Collision Possible

With this architecture, there's only ONE set of containers:
- DataStorage starts PostgreSQL + Redis + DS
- HAPI starts HolmesGPT-API
- CRD controllers connect via HTTP (no containers to start)

---

## ✅ Benefits

1. **No port collisions** - Single source of truth for infrastructure
2. **Faster tests** - CRD controllers don't wait for container startup
3. **Simpler setup** - Run DS once, then run any controller tests
4. **Clear ownership** - DS team owns database infra, HAPI team owns AI infra

---

## 🗳️ Response Requested

Please confirm:

| Team | Approval | Notes |
|------|----------|-------|
| **DataStorage** | ⏳ Pending | Will you own `podman-compose.test.yml`? |
| **HAPI** | ⏳ Pending | Will you create `holmesgpt-api/podman-compose.test.yml`? |
| **AIAnalysis** | ✅ Proposed | Will update tests to connect to DS + HAPI |
| **Gateway** | ⏳ Pending | Will update tests to connect to DS |
| **Notification** | ⏳ Pending | Will update tests to connect to DS |
| **RO/WE/SP** | ⏳ Pending | Will update tests to connect to DS |

---

## 📚 References

- `docs/development/business-requirements/TESTING_GUIDELINES.md` - Authoritative testing policy
- `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` - Port allocation
- `podman-compose.test.yml` - Current shared file (to be moved)

---

**Next Steps**:
1. Get team approvals
2. Move `podman-compose.test.yml` to DataStorage
3. Update CRD controller tests to connect via HTTP
4. Update documentation

