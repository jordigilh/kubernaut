# DD-STORAGE-011: Data Storage Service V1.1 Implementation Plan

**Date**: November 14, 2025 (Updated: November 22, 2025)
**Status**: 📋 **UPDATED** - V1.0 now includes basic CRUD, V1.1 adds validation/lifecycle
**Decision Maker**: Kubernaut Data Storage Team
**Authority**: DD-STORAGE-008 (Workflow Catalog Schema), DD-STORAGE-006 (Caching Decision), DD-WORKFLOW-004 (Hybrid Weighted Scoring)
**Affects**: Data Storage Service V1.1
**Version**: 2.0

---

## 📋 **Changelog**

### Version 2.0 (November 22, 2025)
- **UPDATED**: V1.0 now includes basic workflow CRUD (POST/PUT/DELETE) - 3 hours
- **UPDATED**: V1.0 now includes label schema versioning - 1 hour
- **UPDATED**: V1.0 now includes hybrid weighted label scoring (DD-WORKFLOW-004) - 4-6 hours
- **UPDATED**: V1.1 scope reduced to validation/lifecycle only - 7 hours (↓ from 10 hours)
- **RATIONALE**: Basic CRUD unblocks testing; validation adds quality controls in V1.1
- **CROSS-REFERENCE**: DD-WORKFLOW-004 (Hybrid Weighted Label Scoring)

### Version 1.0 (November 14, 2025)
- Initial V1.1 implementation plan
- Defined playbook CRUD REST API with validation
- Defined embedding caching strategy
- Defined version history and diff APIs

---

## 🚨 **CRITICAL CLARIFICATION: Data Storage is NOT a CRD Controller**

**Data Storage Service Architecture**:
- ✅ **Stateless HTTP REST API Service** - Provides REST endpoints for playbook management
- ✅ **PostgreSQL Access Layer** - Centralized database access per ADR-032
- ❌ **NOT a CRD Controller** - Does not watch Kubernetes CRDs
- ❌ **NOT a Kubernetes Controller** - Does not reconcile Kubernetes resources

**Playbook Management Architecture**:
```
┌─────────────────────────────────────┐
│ RemediationPlaybook CRD             │  ← Kubernetes Custom Resource
│ (kind: RemediationPlaybook)         │
└────────────┬────────────────────────┘
             │ watches
             ↓
┌─────────────────────────────────────┐
│ Playbook Registry Controller        │  ← THIS is the CRD controller
│ (Separate CRD Controller Service)   │  ← (Not part of Data Storage)
│ - Watches RemediationPlaybook CRDs  │
│ - Validates playbook specs          │
│ - Calls Data Storage REST API       │
└────────────┬────────────────────────┘
             │ HTTP REST calls
             ↓
┌─────────────────────────────────────┐
│ Data Storage Service                │  ← THIS is what we're planning
│ (Stateless REST API - NOT a CRD    │
│  controller)                        │
│                                     │
│ REST Endpoints:                     │
│ - POST   /api/v1/playbooks          │  ← Create/update playbook
│ - GET    /api/v1/playbooks/search   │  ← Semantic search
│ - PATCH  /api/v1/playbooks/{id}     │  ← Disable/enable
│ - DELETE /api/v1/cache/playbooks    │  ← Cache invalidation
└─────────────────────────────────────┘
```

**Key Point**: V1.1 adds REST API endpoints to Data Storage. A separate Playbook Registry Controller (future work, not part of V1.1) would call these endpoints when CRDs are created/updated.

---

## 🎯 **V1.1 Goals**

### **Primary Goal**
Enable playbook lifecycle management via REST API with caching for improved performance.

### **Scope**
1. ✅ Playbook CRUD REST API (create, update, disable, enable)
2. ✅ Semantic version validation (semver)
3. ✅ Embedding caching with Redis
4. ✅ Cache invalidation REST endpoints
5. ✅ Version history and diff APIs

### **Out of Scope (Future Work)**
- ❌ Playbook Registry Controller (separate CRD controller service)
- ❌ RemediationPlaybook CRD implementation (separate service)
- ❌ Kubernetes controller logic (not part of Data Storage)
- ❌ CRD watching/reconciliation (not part of Data Storage)

---

## 📋 **Current State (V1.0 MVP)**

### **What V1.0 Provides**
- ✅ Unified audit table (`audit_events`)
- ✅ Workflow catalog table (`remediation_workflow_catalog`)
- ✅ Semantic search endpoint (`GET /api/v1/workflows/search`)
- ✅ Real-time embedding generation (no caching)
- ✅ PostgreSQL with pgvector
- ✅ Redis DLQ for audit integrity
- ✅ **NEW**: Workflow CRUD endpoints (`POST/PUT/DELETE /api/v1/workflows`)
- ✅ **NEW**: Label schema versioning (`schema_version` field)
- ✅ **NEW**: Hybrid weighted label scoring (DD-WORKFLOW-004)

### **V1.0 Limitations**
- ❌ No semantic version validation (accepts any version string)
- ❌ No version immutability enforcement (can overwrite versions)
- ❌ No lifecycle management API (disable/enable via SQL)
- ❌ No embedding caching (2.5s latency per query)
- ❌ No cache invalidation mechanism
- ❌ No version diff API

---

## 🎯 **Target State (V1.1)**

### **What V1.1 Adds** (On Top of V1.0 Basic CRUD)
- ✅ Semantic version validation (golang.org/x/mod/semver)
- ✅ Version increment validation (must be > current latest)
- ✅ Version immutability enforcement (409 on duplicate)
- ✅ Lifecycle management API (disable/enable with audit trail)
- ✅ Version history API (list versions, get specific version)
- ✅ Version diff API (compare two versions)
- ✅ Embedding caching with Redis (24h TTL)
- ✅ Cache invalidation REST endpoints
- ✅ 50× performance improvement (2.5s → ~50ms)

### **V1.1 Architecture**

```
┌─────────────────────────────────────────────────────────────┐
│ Data Storage Service V1.1 (Stateless REST API)              │
│                                                              │
│ ┌─────────────────────┐  ┌──────────────────────────────┐  │
│ │ Playbook REST API   │  │ Semantic Search (existing)   │  │
│ │ (NEW in V1.1)       │  │ (from V1.0)                  │  │
│ │                     │  │                              │  │
│ │ POST   /playbooks   │  │ GET /playbooks/search        │  │
│ │ PATCH  /playbooks   │  │                              │  │
│ │ GET    /versions    │  │                              │  │
│ │ GET    /diff        │  │                              │  │
│ │ DELETE /cache       │  │                              │  │
│ └──────────┬──────────┘  └────────────┬─────────────────┘  │
│            │                          │                     │
│            ↓                          ↓                     │
│ ┌──────────────────────────────────────────────────────┐   │
│ │ Version Validator (NEW)                              │   │
│ │ - Semver format validation                           │   │
│ │ - Version increment validation                       │   │
│ │ - Immutability enforcement                           │   │
│ └──────────────────────────────────────────────────────┘   │
│            │                          │                     │
│            ↓                          ↓                     │
│ ┌──────────────────────────────────────────────────────┐   │
│ │ Embedding Cache (NEW)                                │   │
│ │ - Redis: embedding:playbook:{id}:{version}           │   │
│ │ - TTL: 24 hours                                      │   │
│ │ - Invalidation on create/update/disable/enable       │   │
│ └──────────────────────────────────────────────────────┘   │
│            │                          │                     │
│            ↓                          ↓                     │
│ ┌──────────────────────────────────────────────────────┐   │
│ │ PostgreSQL (existing)                                │   │
│ │ - playbook_catalog table                             │   │
│ │ - pgvector for embeddings                            │   │
│ └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## 📊 **V1.0 vs V1.1 Feature Comparison**

| Feature | V1.0 MVP | V1.1 Enhancements |
|---------|----------|-------------------|
| **Playbook Creation** | ❌ SQL-only | ✅ REST API (`POST /api/v1/playbooks`) |
| **Playbook Update** | ❌ SQL-only | ✅ REST API (`POST /api/v1/playbooks`) |
| **Version Validation** | ❌ Manual SQL | ✅ Automated (semver, increment, immutability) |
| **Lifecycle Management** | ❌ SQL-only | ✅ REST API (`PATCH /disable`, `/enable`) |
| **Version History** | ❌ Not available | ✅ `GET /api/v1/playbooks/{id}/versions` |
| **Version Diff** | ❌ Not available | ✅ `GET /api/v1/playbooks/{id}/versions/{v1}/diff/{v2}` |
| **Semantic Search** | ✅ Real-time (2.5s) | ✅ Cached (~50ms, 50× faster) |
| **Embedding Cache** | ❌ No cache | ✅ Redis (24h TTL) |
| **Cache Invalidation** | ❌ N/A | ✅ REST endpoints (`DELETE /api/v1/cache/playbooks/{id}`) |
| **Performance** | 2.5s latency | ~50ms latency (50× improvement) |

---

## 🚀 **V1.1 Implementation Phases**

### **Phase 1: Version Validation Library** (Day 1, 8 hours)

**Goal**: Create semantic version validation library

**Deliverables**:
- `pkg/datastorage/playbook/version_validator.go`
  - `ValidateVersionFormat()` using `golang.org/x/mod/semver`
  - `CompareVersions()` for version comparison (-1/0/1)
  - `IsValidIncrement()` to enforce increment requirement
  - Custom errors: `ErrVersionAlreadyExists`, `ErrVersionNotIncremented`
- Unit tests (TDD)
  - Valid semver formats (v1.0.0, v1.2.3, v2.0.0-alpha)
  - Invalid formats (1.0, vv1.0.0, abc)
  - Version increment validation
  - Immutability enforcement

**Success Criteria**:
- ✅ All semver formats validated correctly
- ✅ Version increment enforced (v0.9 after v1.0 rejected)
- ✅ Duplicate versions rejected with clear error
- ✅ 100% unit test coverage

---

### **Phase 2: Playbook Management REST API** (Days 2-3, 16 hours)

**Goal**: Create/update/disable/enable playbooks with version validation

**Deliverables**:
- `POST /api/v1/playbooks` (create/update playbook)
  - Validate semantic version format (semver)
  - Validate version increment (must be > current latest)
  - Prevent overwriting existing versions (immutability)
  - Return clear error messages:
    - 400: Invalid version format
    - 400: Version not incremented
    - 409: Version already exists (immutable)
  - Invalidate embedding cache on create/update
- `PATCH /api/v1/playbooks/{id}/{version}/disable`
  - Capture disabled_by, disabled_reason, disabled_at
  - Invalidate embedding cache on disable
- `PATCH /api/v1/playbooks/{id}/{version}/enable`
  - Clear disabled metadata
  - Invalidate embedding cache on enable
- `GET /api/v1/playbooks/{id}/versions` (list all versions)
- `GET /api/v1/playbooks/{id}/versions/{version}` (get specific version)
- `GET /api/v1/playbooks/{id}/versions/{v1}/diff/{v2}` (compare versions)

**Success Criteria**:
- ✅ Playbook creation with version validation works
- ✅ Duplicate versions rejected with 409 Conflict
- ✅ Version increment enforced (v0.9 after v1.0 rejected)
- ✅ Disable/enable captures audit metadata
- ✅ Version history API returns all versions
- ✅ Version diff API shows field-by-field differences

---

### **Phase 3: Embedding Caching** (Day 4, 8 hours)

**Goal**: Redis-backed embedding cache with REST API invalidation

**Deliverables**:
- `pkg/datastorage/embedding/cache.go`
  - Redis key: `embedding:playbook:{id}:{version}`
  - TTL: 24 hours (configurable)
  - Cache hit/miss metrics
- Update embedding pipeline to use cache
  - Check cache before generating embedding
  - Store embedding in cache after generation
  - Latency improvement: 2.5s → ~50ms (50× faster)
- Cache invalidation REST endpoints
  - `DELETE /api/v1/cache/playbooks/{id}` (invalidate specific playbook)
  - `DELETE /api/v1/cache/playbooks` (invalidate all playbooks)
  - Called by Playbook Management API on create/update/disable/enable
  - Can be called by external services (e.g., future Playbook Registry Controller)

**Success Criteria**:
- ✅ Cache hit reduces latency from 2.5s to ~50ms
- ✅ Cache miss generates embedding and caches it
- ✅ Cache invalidation clears specific playbook embeddings
- ✅ Metrics track cache hit/miss rate

---

### **Phase 4: Integration Tests** (Day 5, 8 hours)

**Goal**: Comprehensive integration tests for V1.1 features

**Deliverables**:
- Test playbook CRUD operations with version validation
  - Test version format validation (invalid semver rejected)
  - Test version increment validation (v0.9 after v1.0 rejected)
  - Test immutability (duplicate version rejected with 409)
- Test lifecycle management (disable/enable)
  - Test disable captures metadata (who/when/why)
  - Test re-enable clears metadata
- Test version history API
  - Test get specific version
  - Test diff between versions
- Test embedding caching
  - Test cache hit (50ms latency)
  - Test cache miss (2.5s latency, then cached)
  - Test cache invalidation on create/update/disable/enable

**Success Criteria**:
- ✅ All integration tests pass
- ✅ Version validation enforced in real scenarios
- ✅ Cache improves performance by 50×
- ✅ Cache invalidation works correctly

---

## 📊 **Timeline & Effort**

| Phase | Duration | Effort | Deliverable |
|-------|----------|--------|-------------|
| **Phase 1: Version Validation** | Day 1 | 8 hours | Version validator library + tests |
| **Phase 2: REST API** | Days 2-3 | 16 hours | Full playbook management REST API |
| **Phase 3: Caching** | Day 4 | 8 hours | Redis cache + invalidation endpoints |
| **Phase 4: Integration Tests** | Day 5 | 8 hours | Comprehensive integration tests |
| **Total** | **5 days** | **40 hours** | **V1.1 Complete** |

---

## 🎯 **Success Criteria**

### **Functional Requirements**
1. ✅ Playbook creation/update via REST API with version validation
2. ✅ Semantic version validation (format, increment, immutability)
3. ✅ Lifecycle management (disable/enable) with audit trail
4. ✅ Version history API (list, get specific, diff)
5. ✅ Embedding caching reduces latency by 50× (2.5s → ~50ms)
6. ✅ Cache invalidation via REST endpoints

### **Non-Functional Requirements**
1. ✅ 100% unit test coverage for version validator
2. ✅ Integration tests for all V1.1 features
3. ✅ Prometheus metrics for cache hit/miss rate
4. ✅ Structured logging for all operations
5. ✅ RFC 7807 error responses for all failures

### **Performance Requirements**
1. ✅ Cache hit latency: < 100ms (target: ~50ms)
2. ✅ Cache miss latency: < 3s (same as V1.0)
3. ✅ Cache hit rate: > 80% (after warm-up)
4. ✅ Version validation: < 10ms

---

## 🔗 **Integration Points**

### **Consumers of V1.1 REST API**

1. **HolmesGPT API** (existing consumer, V1.0)
   - Uses: `GET /api/v1/playbooks/search` (semantic search)
   - V1.1 benefit: 50× faster queries with caching

2. **Playbook Registry Controller** (future, not part of V1.1)
   - Would use: `POST /api/v1/playbooks` (create/update)
   - Would use: `PATCH /api/v1/playbooks/{id}/disable` (lifecycle)
   - Would use: `DELETE /api/v1/cache/playbooks/{id}` (invalidation)
   - **Note**: This is a separate CRD controller service, not part of Data Storage

3. **Operations/SRE Teams** (manual management)
   - Can use: All V1.1 REST endpoints for manual playbook management
   - Alternative to SQL-only management in V1.0

---

## 📚 **Dependencies**

### **External Dependencies**
- ✅ `golang.org/x/mod/semver` - Semantic version validation
- ✅ Redis - Embedding cache (already required for DLQ in V1.0)
- ✅ PostgreSQL with pgvector - Playbook storage (existing)

### **Internal Dependencies**
- ✅ V1.0 MVP complete (unified audit, playbook catalog, semantic search)
- ✅ DD-STORAGE-008 (Playbook Catalog Schema) - Authoritative schema
- ✅ DD-STORAGE-006 (V1.0 No-Cache Decision) - Caching rationale

---

## 🚨 **Risks & Mitigations**

### **Risk 1: Cache Invalidation Complexity**
**Risk**: Cache invalidation logic could become complex with multiple invalidation triggers
**Mitigation**:
- Simple key-based invalidation (`embedding:playbook:{id}:{version}`)
- Clear REST endpoints for external services to trigger invalidation
- Comprehensive integration tests for all invalidation scenarios

### **Risk 2: Version Validation Edge Cases**
**Risk**: Semantic version validation may have edge cases (pre-release, build metadata)
**Mitigation**:
- Use battle-tested `golang.org/x/mod/semver` library
- Comprehensive unit tests for all semver formats
- Clear error messages for invalid formats

### **Risk 3: Cache Stampede**
**Risk**: Multiple concurrent requests for same uncached playbook could cause stampede
**Mitigation**:
- Use Redis SETNX for cache lock
- First request generates embedding, others wait
- Timeout after 5s to prevent deadlock

---

## 📝 **Open Questions**

1. **Cache TTL**: 24 hours is proposed, but should it be configurable per playbook?
   - **Recommendation**: Start with global 24h TTL, make configurable in V1.2 if needed

2. **Cache Eviction Policy**: Should we implement LRU eviction or rely on TTL?
   - **Recommendation**: TTL-only for V1.1, add LRU in V1.2 if memory becomes an issue

3. **Version Diff Format**: Should diff be JSON Patch (RFC 6902) or custom format?
   - **Recommendation**: Custom format for V1.1 (field-by-field), consider JSON Patch in V1.2

---

## 🎯 **Post-V1.1 Roadmap (V1.2+)**

### **V1.1 Enhancements (Deferred from V1.0)**

#### **Query API: Cursor-Based Pagination** (BR-STORAGE-TBD)
**Context**: V1.0 uses offset-based pagination for audit event queries. For real-time data with high write volumes, cursor-based pagination provides more reliable results.

**Benefits**:
- **Consistency**: No missed/duplicate records during pagination
- **Performance**: Efficient for large result sets (uses index on `event_timestamp`)
- **Real-time**: Handles concurrent writes gracefully

**Implementation**:
- Add `cursor` parameter to `GET /api/v1/audit/events` endpoint
- Cursor format: `base64(event_timestamp + event_id)` for uniqueness
- Maintain backward compatibility with `offset`/`limit` parameters

**Effort**: 2 days (8 hours implementation + 8 hours testing)

**Reference**: DD-STORAGE-010 (Query API Pagination Strategy)

---

#### **Audit Events: Parent Event Date Index** (Performance Optimization)
**Context**: V1.0 implements FK constraint on `(parent_event_id, parent_event_date)` but no index for child event lookups.

**Benefits**:
- **Performance**: Faster queries for "find all children of parent X"
- **Observability**: Efficient event chain traversal for debugging
- **AI Analysis**: Faster causality analysis for RCA

**Implementation**:
```sql
CREATE INDEX idx_audit_events_parent_lookup
ON audit_events (parent_event_id, parent_event_date)
WHERE parent_event_id IS NOT NULL;
```

**Effort**: 1 day (4 hours implementation + 4 hours performance testing)

**Reference**: FK_CONSTRAINT_IMPLEMENTATION_SUMMARY.md

---

#### **Audit Events: Historical Parent-Child Backfill** (Data Integrity)
**Context**: V1.0 added `parent_event_date` column, but existing events have NULL values.

**Benefits**:
- **Completeness**: Enable historical event chain queries
- **Compliance**: Full audit trail for all events
- **Analytics**: Complete causality data for trend analysis

**Implementation**:
```sql
-- Backfill parent_event_date from parent event's event_date
UPDATE audit_events child
SET parent_event_date = parent.event_date
FROM audit_events parent
WHERE child.parent_event_id = parent.event_id
  AND child.parent_event_date IS NULL;
```

**Effort**: 1 day (4 hours migration script + 4 hours validation)

**Considerations**:
- Run during maintenance window (may be slow for large datasets)
- Add progress logging for long-running backfill
- Validate FK constraint after backfill

**Reference**: FK_CONSTRAINT_IMPLEMENTATION_SUMMARY.md

---

### **V1.2: Advanced Caching**
- LRU cache eviction policy
- Per-playbook TTL configuration
- Cache warming on startup
- Cache statistics dashboard

### **V1.3: Playbook Registry Controller Integration**
- Playbook Registry Controller (separate CRD controller service)
- RemediationPlaybook CRD implementation
- Automatic playbook registration from CRDs
- RBAC for playbook management

### **V2.0: Audit Embeddings**
- Audit record embeddings for RAR (Remediation Action Recommendations)
- Historical pattern analysis
- Trend detection

---

## ✅ **Approval & Sign-off**

**Status**: 📋 **DRAFT** - Awaiting approval

**Approvers**:
- [ ] Data Storage Team Lead
- [ ] Architecture Review Board
- [ ] DevOps/SRE Team (for operational impact)

**Approval Criteria**:
- ✅ Clear scope (no CRD controller implementation)
- ✅ Realistic timeline (5 days, 40 hours)
- ✅ Well-defined success criteria
- ✅ Risk mitigation strategies documented

---

## 📚 **References**

- **DD-STORAGE-008**: Playbook Catalog Schema (authoritative schema)
- **DD-STORAGE-006**: V1.0 No-Cache Decision (caching rationale)
- **ADR-033**: Remediation Playbook Catalog (overall playbook architecture)
- **ADR-032**: Centralized PostgreSQL Access (Data Storage mandate)
- **DATA-STORAGE-V1.0-MVP-IMPLEMENTATION-PLAN.md**: V1.0 implementation (foundation)

---

**Document Version**: 1.0
**Created**: November 14, 2025
**Last Updated**: November 14, 2025
**Status**: 📋 DRAFT - High-level plan for V1.1 features

