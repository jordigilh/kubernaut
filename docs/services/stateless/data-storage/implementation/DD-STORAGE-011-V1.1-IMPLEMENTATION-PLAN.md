# DD-STORAGE-011: Data Storage Service V1.1 Implementation Plan

**Date**: November 14, 2025 (Updated: November 28, 2025)
**Status**: ✅ **APPROVED** - Ready for implementation with approved design decisions
**Decision Maker**: Kubernaut Data Storage Team
**Authority**: DD-STORAGE-008 (Workflow Catalog Schema), DD-STORAGE-006 (Caching Decision), DD-WORKFLOW-004 (Hybrid Weighted Scoring), DD-API-001 (HTTP Header vs JSON Body)
**Affects**: Data Storage Service V1.1
**Version**: 3.3

---

## 📋 **Changelog**

### Version 3.3 (November 28, 2025) - DD-API-001 COMPLIANCE
- **FIXED**: Disable reason now in JSON body (not HTTP header)
- **COMPLIANCE**: DD-API-001 (HTTP header vs JSON body pattern)
- **UPDATED**: Phase 2 OpenAPI spec, test cases, and implementation to use JSON body
- **UPDATED**: Test helper `DisableTestWorkflow` to use JSON body

### Version 3.2 (November 28, 2025) - APPROVED DESIGN DECISIONS
- **ADDED**: Approved Design Decisions section (DD-1 through DD-5)
  - DD-1: PUT not allowed (immutability enforced) - use POST for new versions
  - DD-2: DELETE = disable (uses `disabled_at`, preserves audit trail)
  - DD-3: Synchronous embedding generation (~2.5s latency accepted)
  - DD-4: Use `/api/v1/workflows` path (per DD-NAMING-001)
  - DD-5: Implement CRUD first, then update E2E tests
- **ADDED**: Pre-Implementation Checklist (blocking items)
- **ADDED**: Implementation Suggestions (S1, S2, S3)
- **UPDATED**: Phase 1 with OpenAPI spec update step
- **UPDATED**: Phase 2 renamed to "Disable Workflow" (not Update/Delete)
- **UPDATED**: Phase 2 reflects DD-1 (405 for PUT) and DD-2 (disable behavior)

### Version 3.1 (November 28, 2025) - EXPANDED PHASES
- Expanded to 9-day detailed implementation plan with TDD phases and code examples

### Version 3.0 (November 28, 2025) - CRITICAL CORRECTION
- **🚨 CORRECTION**: V1.0 workflow CRUD endpoints (`POST/PUT/DELETE /api/v1/workflows`) were **NOT IMPLEMENTED**
  - v2.0 changelog incorrectly stated these were complete
  - E2E tests (Scenarios 4, 5, 6) fail due to missing `POST /api/v1/workflows` endpoint
  - Verified via code inspection: only `GET /api/v1/workflows` and `POST /api/v1/workflows/search` exist
- **IMPACT**: Workflow CRUD remains in V1.1 scope (not V1.0)
- **VALIDATION REQUIREMENT**: CRUD endpoints must pass all 3 test tiers (unit, integration, E2E) before marking complete
- **CROSS-REFERENCE**: BR-WORKFLOW-001 (FR-PLAYBOOK-001-02 through FR-PLAYBOOK-001-05)

### Version 2.0 (November 22, 2025) - ⚠️ INACCURATE (superseded by v3.0)
- ~~**UPDATED**: V1.0 now includes basic workflow CRUD (POST/PUT/DELETE) - 3 hours~~ **← INCORRECT**
- **UPDATED**: V1.0 now includes label schema versioning - 1 hour
- **UPDATED**: V1.0 now includes hybrid weighted label scoring (DD-WORKFLOW-004) - 4-6 hours
- ~~**UPDATED**: V1.1 scope reduced to validation/lifecycle only - 7 hours (↓ from 10 hours)~~ **← REVERTED**
- ~~**RATIONALE**: Basic CRUD unblocks testing; validation adds quality controls in V1.1~~ **← INVALID**
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

## 📋 **Current State (V1.0 MVP) - CORRECTED November 28, 2025**

### **What V1.0 Actually Provides** (Verified via Code Inspection)
- ✅ Unified audit table (`audit_events`)
- ✅ Workflow catalog table (`remediation_workflow_catalog`)
- ✅ Semantic search endpoint (`POST /api/v1/workflows/search`)
- ✅ List workflows endpoint (`GET /api/v1/workflows`)
- ✅ Real-time embedding generation (no caching)
- ✅ PostgreSQL with pgvector
- ✅ Redis DLQ for audit integrity
- ✅ Label schema versioning (`schema_version` field)
- ✅ Hybrid weighted label scoring - V1.0 base similarity only (DD-WORKFLOW-004 v2.0)
- ✅ Workflow search audit trail (BR-AUDIT-023 through BR-AUDIT-028)

### **V1.0 Missing Features** (🚨 NOT IMPLEMENTED - Contrary to v2.0 Claims)
- ❌ **`POST /api/v1/workflows`** - Create workflow (BR-WORKFLOW-001 FR-PLAYBOOK-001-02)
- ❌ **`PUT /api/v1/workflows/{id}`** - Update workflow
- ❌ **`DELETE /api/v1/workflows/{id}`** - Delete workflow
- ❌ **`GET /api/v1/workflows/{id}/versions`** - List versions (BR-WORKFLOW-001 FR-PLAYBOOK-001-04)
- ❌ **`PATCH /api/v1/workflows/{id}/{version}`** - Update status (BR-WORKFLOW-001 FR-PLAYBOOK-001-05)

### **V1.0 Limitations** (Unchanged)
- ❌ No semantic version validation (accepts any version string)
- ❌ No version immutability enforcement (can overwrite versions)
- ❌ No lifecycle management API (disable/enable via SQL)
- ❌ No embedding caching (2.5s latency per query)
- ❌ No cache invalidation mechanism
- ❌ No version diff API

### **Impact of Missing CRUD Endpoints**
1. **E2E Tests**: Scenarios 4, 5, 6 fail (expect `POST /api/v1/workflows`)
2. **Operations**: Workflows must be managed via SQL (no REST API)
3. **Automation**: Cannot automate workflow registration from CI/CD
4. **HolmesGPT API**: Cannot seed workflows via API for testing

---

## 🎯 **Target State (V1.1)** - CORRECTED November 28, 2025

### **What V1.1 Must Implement** (CRUD + Validation + Caching)

#### **Phase A: Basic CRUD Endpoints** (🚨 PRIORITY - Unblocks E2E Tests)
- ❌ **`POST /api/v1/workflows`** - Create workflow (BR-WORKFLOW-001 FR-PLAYBOOK-001-02)
- ❌ **`PUT /api/v1/workflows/{id}`** - Update workflow
- ❌ **`DELETE /api/v1/workflows/{id}`** - Delete workflow
- ❌ **`GET /api/v1/workflows/{id}`** - Get single workflow
- ❌ **`GET /api/v1/workflows/{id}/versions`** - List versions (BR-WORKFLOW-001 FR-PLAYBOOK-001-04)

#### **Phase B: Lifecycle Management**
- ❌ **`PATCH /api/v1/workflows/{id}/{version}`** - Update status (BR-WORKFLOW-001 FR-PLAYBOOK-001-05)
- ❌ Lifecycle management API (disable/enable with audit trail)

#### **Phase C: Version Validation**
- ❌ Semantic version validation (golang.org/x/mod/semver)
- ❌ Version increment validation (must be > current latest)
- ❌ Version immutability enforcement (409 on duplicate)
- ❌ Version diff API (compare two versions)

#### **Phase D: Caching** (Performance Optimization)
- ❌ Embedding caching with Redis (24h TTL)
- ❌ Cache invalidation REST endpoints
- ❌ 50× performance improvement (2.5s → ~50ms)

### **Validation Requirements** (Before Marking Complete)
Each feature must pass **ALL 3 test tiers**:
1. ✅ Unit tests (70%+ coverage)
2. ✅ Integration tests (handler + repository)
3. ✅ E2E tests (full API flow with real database)

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

## 📊 **V1.0 vs V1.1 Feature Comparison** - CORRECTED November 28, 2025

| Feature | V1.0 MVP (Actual) | V1.1 Target |
|---------|-------------------|-------------|
| **Workflow Creation** | ❌ **NOT IMPLEMENTED** (SQL-only) | ✅ REST API (`POST /api/v1/workflows`) |
| **Workflow Update** | ❌ **NOT IMPLEMENTED** (SQL-only) | ✅ REST API (`PUT /api/v1/workflows/{id}`) |
| **Workflow Delete** | ❌ **NOT IMPLEMENTED** (SQL-only) | ✅ REST API (`DELETE /api/v1/workflows/{id}`) |
| **Workflow List** | ✅ `GET /api/v1/workflows` | ✅ (Existing) |
| **Semantic Search** | ✅ `POST /api/v1/workflows/search` | ✅ (Existing) |
| **Version Validation** | ❌ Manual SQL | ✅ Automated (semver, increment, immutability) |
| **Lifecycle Management** | ❌ SQL-only | ✅ REST API (`PATCH /disable`, `/enable`) |
| **Version History** | ❌ Not available | ✅ `GET /api/v1/workflows/{id}/versions` |
| **Version Diff** | ❌ Not available | ✅ `GET /api/v1/workflows/{id}/versions/{v1}/diff/{v2}` |
| **Embedding Cache** | ❌ No cache | ✅ Redis (24h TTL) |
| **Cache Invalidation** | ❌ N/A | ✅ REST endpoints |
| **Search Audit Trail** | ✅ BR-AUDIT-023-028 | ✅ (Existing) |
| **Performance** | 2.5s latency | ~50ms latency (50× improvement) |

### **🚨 Critical Gap Identified (November 28, 2025)**

The v2.0 changelog incorrectly stated that V1.0 included basic CRUD endpoints. This was **never implemented**. The only workflow-related endpoints in V1.0 are:
- `GET /api/v1/workflows` - List workflows
- `POST /api/v1/workflows/search` - Semantic search

All CRUD operations (`POST`, `PUT`, `DELETE`) remain in V1.1 scope.

---

## 🎯 **Approved Design Decisions** - November 28, 2025

The following decisions were approved before implementation to prevent ambiguity:

### **DD-1: Workflow Immutability (PUT Behavior)**

| Question | Should `PUT /api/v1/workflows/{id}` update existing versions or create new ones? |
|----------|----------------------------------------------------------------------------------|
| **Decision** | **Option C**: PUT is NOT allowed. Immutability is enforced. |
| **Rationale** | DD-WORKFLOW-012 mandates workflow immutability. Any "update" must create a new version via `POST`. |
| **Implementation** | `PUT /api/v1/workflows/{id}` returns 405 Method Not Allowed. Use `POST` with new version. |

### **DD-2: Delete Behavior (Soft Delete vs Disable)**

| Question | Should DELETE remove workflows or disable them? |
|----------|------------------------------------------------|
| **Decision** | **Option C**: Use existing `disabled_at` mechanism (disable, not delete). |
| **Rationale** | Preserves audit trail, aligns with DD-WORKFLOW-012 (immutability). |
| **Implementation** | `DELETE /api/v1/workflows/{id}` sets `disabled_at=NOW()`, `disabled_by`, `disabled_reason`. |

### **DD-3: Embedding Generation Timing**

| Question | Should embedding generation be synchronous or asynchronous? |
|----------|-------------------------------------------------------------|
| **Decision** | **Option A**: Synchronous for V1.1 (accept ~2.5s latency). |
| **Rationale** | Correctness over performance. Async introduces race conditions where newly created workflows aren't immediately searchable. |
| **Implementation** | `POST /api/v1/workflows` blocks until embedding is generated. Async deferred to V1.2. |

### **DD-4: API Terminology**

| Question | Use `/api/v1/workflows` or `/api/v1/playbooks`? |
|----------|------------------------------------------------|
| **Decision** | Use `/api/v1/workflows` (aligns with DD-NAMING-001 and current implementation). |
| **Rationale** | DD-NAMING-001 is authoritative for terminology. BR-WORKFLOW-001 uses "playbook" but DD-NAMING-001 standardized on "workflow". |
| **Implementation** | All endpoints use `/api/v1/workflows/*` path. |

### **DD-5: E2E Test Migration Strategy**

| Question | Update E2E tests before or after CRUD implementation? |
|----------|-------------------------------------------------------|
| **Decision** | **Option B**: Implement CRUD first (Phases 1-3), then update E2E tests to use new API. |
| **Rationale** | Avoids circular dependencies. CRUD endpoints must exist before tests can use them. |
| **Implementation** | Phase 5 updates existing E2E tests (04, 05, 06) to use new CRUD API. |

---

## 📋 **Pre-Implementation Checklist**

**BLOCKING**: Complete ALL items before starting Phase 1.

### **Infrastructure Verification**

- [ ] **Embedding Service**: Verify `pkg/datastorage/embedding/service.go` is fully implemented
  ```bash
  grep -r "GenerateEmbedding" pkg/datastorage/embedding/ --include="*.go"
  ```
- [ ] **Database Schema**: Verify `remediation_workflow_catalog` table has all required columns
  ```bash
  grep -A 50 "CREATE TABLE.*remediation_workflow_catalog" migrations/*.sql
  ```
- [ ] **NodePort Config**: Verify Kind cluster creates with NodePort config
  ```bash
  kind create cluster --name test-nodeport --config test/infrastructure/kind-datastorage-config.yaml
  curl http://localhost:8081/health/ready
  kind delete cluster --name test-nodeport
  ```

### **Code Preparation**

- [ ] **OpenAPI Spec**: Update `docs/services/stateless/data-storage/openapi/v2.yaml` with new endpoints
- [ ] **Test Helper**: Create `test/e2e/datastorage/workflow_crud_helpers.go` with CRUD helper functions
- [ ] **Models**: Verify `pkg/datastorage/models/workflow.go` has `CreateWorkflowRequest` struct

### **Documentation Review**

- [ ] **BR-WORKFLOW-001**: Review all functional requirements (FR-PLAYBOOK-001-01 through 05)
- [ ] **DD-WORKFLOW-012**: Review immutability constraints
- [ ] **DD-NAMING-001**: Confirm "workflow" terminology

---

## 🛠️ **Implementation Suggestions** (Approved)

### **S1: Minimal POST Endpoint First**

Instead of full validation in Phase 1, implement a minimal `POST /api/v1/workflows`:
1. Accept workflow JSON
2. Generate embedding (synchronous)
3. Insert into database
4. Return 201 Created

Add validation (required fields, label schema) in Phase 3 during REFACTOR.

**Benefit**: Unblocks E2E tests faster.

### **S2: Workflow CRUD Test Helper**

Create a shared test helper before Phase 4:

```go
// test/e2e/datastorage/workflow_crud_helpers.go
package datastorage

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

// CreateTestWorkflow creates a workflow via REST API
func CreateTestWorkflow(httpClient *http.Client, baseURL string, workflow map[string]interface{}) (*http.Response, error) {
    body, _ := json.Marshal(workflow)
    return httpClient.Post(baseURL+"/api/v1/workflows", "application/json", bytes.NewBuffer(body))
}

// GetTestWorkflow retrieves a workflow by ID
func GetTestWorkflow(httpClient *http.Client, baseURL, workflowID string) (*http.Response, error) {
    return httpClient.Get(fmt.Sprintf("%s/api/v1/workflows/%s", baseURL, workflowID))
}

// DisableTestWorkflow disables a workflow (soft-delete)
// Reason is passed in JSON body (not HTTP header) per DD-WORKFLOW-014 v2.1 pattern
func DisableTestWorkflow(httpClient *http.Client, baseURL, workflowID, reason string) (*http.Response, error) {
    body, _ := json.Marshal(map[string]string{"reason": reason})
    req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/workflows/%s", baseURL, workflowID), bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    return httpClient.Do(req)
}
```

**Benefit**: All E2E tests use same helper, reducing duplication.

### **S3: OpenAPI Spec First**

Before implementing handlers, update OpenAPI spec with new endpoints:

```yaml
# docs/services/stateless/data-storage/openapi/v2.yaml
paths:
  /api/v1/workflows:
    post:
      summary: Create a new workflow
      operationId: createWorkflow
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateWorkflowRequest'
      responses:
        '201':
          description: Workflow created successfully
        '400':
          description: Invalid request (missing fields, invalid labels)
        '409':
          description: Workflow already exists (duplicate workflow_id + version)
```

**Benefit**: Documents API contract before implementation, enables client code generation.

---

## 🚀 **V1.1 Implementation Phases** - EXPANDED November 28, 2025

### **Phase Overview**

| Phase | Days | Hours | Focus | Priority |
|-------|------|-------|-------|----------|
| **Phase 1** | Day 1 | 8h | Create Workflow Endpoint (POST) | 🔴 P0 - Unblocks E2E |
| **Phase 2** | Day 2 | 8h | Update/Delete Workflow Endpoints | 🔴 P0 - Unblocks E2E |
| **Phase 3** | Day 3 | 6h | Get Single Workflow + Unit Tests | 🔴 P0 - Unblocks E2E |
| **Phase 4** | Day 4 | 8h | Integration Tests for CRUD | 🔴 P0 - Validates CRUD |
| **Phase 5** | Day 5 | 6h | E2E Tests for CRUD | 🔴 P0 - Final Validation |
| **Phase 6** | Day 6 | 8h | Version History + Lifecycle APIs | 🟡 P1 - Enhanced Features |
| **Phase 7** | Day 7 | 8h | Version Validation Library | 🟡 P1 - Quality Controls |
| **Phase 8** | Day 8 | 8h | Embedding Caching | 🟢 P2 - Performance |
| **Phase 9** | Day 9 | 6h | Cache Invalidation + Final Tests | 🟢 P2 - Performance |
| **Total** | **9 days** | **66h** | **V1.1 Complete** | |

---

### **Phase 1: Create Workflow Endpoint** (Day 1, 8 hours) 🔴 P0

**Goal**: Implement `POST /api/v1/workflows` to create workflows via REST API

**Business Requirement**: BR-WORKFLOW-001 (FR-PLAYBOOK-001-02)

**Approved Decisions Applied**:
- DD-1: New versions created via POST (immutability enforced)
- DD-3: Synchronous embedding generation (~2.5s latency accepted)
- DD-4: Uses `/api/v1/workflows` path
- S1: Minimal implementation first, validation in REFACTOR

#### **Pre-Phase: OpenAPI Spec Update** (30 min)
Update `docs/services/stateless/data-storage/openapi/v2.yaml`:
```yaml
paths:
  /api/v1/workflows:
    post:
      summary: Create a new workflow version
      operationId: createWorkflow
      tags: [Workflow Catalog]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateWorkflowRequest'
      responses:
        '201':
          description: Workflow created successfully
        '400':
          description: Invalid request
        '409':
          description: Workflow version already exists
```

#### **TDD RED Phase** (2 hours)
**File**: `test/unit/datastorage/workflow_crud_test.go`

```go
var _ = Describe("Workflow CRUD - Create", func() {
    Context("POST /api/v1/workflows", func() {
        It("should create workflow with valid data (BR-WORKFLOW-001)", func() {
            // Test: 201 Created with valid workflow
        })
        It("should reject workflow with missing required fields", func() {
            // Test: 400 Bad Request for missing workflow_id, version, name
        })
        It("should reject duplicate workflow_id + version (DD-1: immutability)", func() {
            // Test: 409 Conflict for duplicate
        })
        It("should auto-generate embedding synchronously (DD-3)", func() {
            // Test: Embedding field populated after create (~2.5s latency)
        })
    })
})
```

#### **TDD GREEN Phase** (4 hours)
**Files**:
- `pkg/datastorage/server/workflow_handlers.go` - Add `HandleCreateWorkflow`
- `pkg/datastorage/repository/workflow_repository.go` - Add `CreateWorkflow`
- `pkg/datastorage/server/server.go` - Register `POST /api/v1/workflows` route

**Implementation**:
```go
// HandleCreateWorkflow handles POST /api/v1/workflows
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
    var req models.CreateWorkflowRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // Validate required fields
    if err := h.validateWorkflowRequest(&req); err != nil {
        h.writeError(w, http.StatusBadRequest, err.Error())
        return
    }

    // Generate embedding
    embedding, err := h.embeddingService.GenerateEmbedding(r.Context(), req.Description)
    if err != nil {
        h.writeError(w, http.StatusInternalServerError, "failed to generate embedding")
        return
    }

    // Create workflow
    workflow, err := h.workflowRepo.CreateWorkflow(r.Context(), &req, embedding)
    if err != nil {
        if errors.Is(err, repository.ErrWorkflowExists) {
            h.writeError(w, http.StatusConflict, "workflow already exists")
            return
        }
        h.writeError(w, http.StatusInternalServerError, "failed to create workflow")
        return
    }

    h.writeJSON(w, http.StatusCreated, workflow)
}
```

#### **TDD REFACTOR Phase** (2 hours)
- Extract validation to `pkg/datastorage/validation/workflow_validator.go`
- Add structured logging for create operations
- Add Prometheus metrics: `datastorage_workflow_creates_total`

**Success Criteria**:
- ✅ `POST /api/v1/workflows` returns 201 Created
- ✅ Workflow persisted to `remediation_workflow_catalog` table
- ✅ Embedding auto-generated and stored
- ✅ 409 Conflict on duplicate workflow_id + version
- ✅ Unit tests pass (70%+ coverage)

---

### **Phase 2: Disable Workflow Endpoint** (Day 2, 8 hours) 🔴 P0

**Goal**: Implement `DELETE /api/v1/workflows/{id}` as a disable operation (not hard delete)

**Business Requirement**: BR-WORKFLOW-001 (FR-PLAYBOOK-001-05)

**Approved Decisions Applied**:
- DD-1: PUT is NOT allowed (returns 405). Immutability enforced - use POST for new versions.
- DD-2: DELETE = disable (uses `disabled_at` field, preserves audit trail)

#### **Pre-Phase: OpenAPI Spec Update** (30 min)
```yaml
paths:
  /api/v1/workflows/{workflow_id}:
    delete:
      summary: Disable a workflow (soft-delete)
      operationId: disableWorkflow
      tags: [Workflow Catalog]
      parameters:
        - name: workflow_id
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [reason]
              properties:
                reason:
                  type: string
                  description: Reason for disabling (audit trail)
                  example: "Deprecated - replaced by workflow-v2"
      responses:
        '204':
          description: Workflow disabled successfully
        '400':
          description: Bad Request - reason is required
        '404':
          description: Workflow not found
    put:
      summary: Update workflow (NOT ALLOWED - immutability enforced)
      operationId: updateWorkflow
      responses:
        '405':
          description: Method Not Allowed - workflows are immutable, create new version via POST
```

#### **TDD RED Phase** (2 hours)
**File**: `test/unit/datastorage/workflow_crud_test.go` (extend)

```go
Context("PUT /api/v1/workflows/{id} (DD-1: immutability)", func() {
    It("should return 405 Method Not Allowed", func() {
        // Test: PUT returns 405, not 200
        // Rationale: DD-WORKFLOW-012 immutability - use POST for new versions
    })
})

Context("DELETE /api/v1/workflows/{id} (DD-2: disable)", func() {
    It("should disable workflow and set disabled_at", func() {
        // Test: 204 No Content, disabled_at populated
    })
    It("should require reason in JSON body", func() {
        // Test: 400 Bad Request if reason missing from body
    })
    It("should reject disable of non-existent workflow", func() {
        // Test: 404 Not Found
    })
    It("should exclude disabled workflows from search", func() {
        // Test: Disabled workflow not returned in search
    })
    It("should preserve workflow in database (audit trail)", func() {
        // Test: Workflow still exists with disabled_at set
    })
})
```

#### **TDD GREEN Phase** (4 hours)
**Files**:
- `pkg/datastorage/server/workflow_handlers.go` - Add `HandleDisableWorkflow`, `HandleUpdateWorkflowNotAllowed`
- `pkg/datastorage/repository/workflow_repository.go` - Add `DisableWorkflow`
- `pkg/datastorage/server/server.go` - Register routes

**Implementation**:
```go
// HandleUpdateWorkflowNotAllowed returns 405 for PUT requests (DD-1: immutability)
func (h *Handler) HandleUpdateWorkflowNotAllowed(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Allow", "GET, DELETE")
    h.writeError(w, http.StatusMethodNotAllowed,
        "workflows are immutable per DD-WORKFLOW-012; create new version via POST /api/v1/workflows")
}

// DisableWorkflowRequest contains the reason for disabling (audit trail)
type DisableWorkflowRequest struct {
    Reason string `json:"reason" validate:"required"`
}

// HandleDisableWorkflow handles DELETE /api/v1/workflows/{id} (DD-2: disable)
func (h *Handler) HandleDisableWorkflow(w http.ResponseWriter, r *http.Request) {
    workflowID := chi.URLParam(r, "workflow_id")

    // Parse reason from JSON body (not HTTP header - per DD-WORKFLOW-014 v2.1 pattern)
    var req DisableWorkflowRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    if req.Reason == "" {
        h.writeError(w, http.StatusBadRequest, "reason is required in request body")
        return
    }

    // Get actor from context (authenticated service)
    actor := h.getActorFromContext(r.Context())

    err := h.workflowRepo.DisableWorkflow(r.Context(), workflowID, actor, req.Reason)
    if err != nil {
        if errors.Is(err, repository.ErrWorkflowNotFound) {
            h.writeError(w, http.StatusNotFound, "workflow not found")
            return
        }
        h.writeError(w, http.StatusInternalServerError, "failed to disable workflow")
        return
    }

    w.WriteHeader(http.StatusNoContent)
}
```

**Routes**:
```go
r.Put("/workflows/{workflow_id}", s.handler.HandleUpdateWorkflowNotAllowed)  // 405
r.Delete("/workflows/{workflow_id}", s.handler.HandleDisableWorkflow)        // Disable
```

#### **TDD REFACTOR Phase** (2 hours)
- Add structured logging for disable operations
- Add Prometheus metrics: `datastorage_workflow_disables_total`
- Ensure disabled workflows excluded from search (update `SearchByEmbedding` WHERE clause)

**Success Criteria**:
- ✅ `PUT /api/v1/workflows/{id}` returns 405 Method Not Allowed
- ✅ `DELETE /api/v1/workflows/{id}` returns 204 No Content
- ✅ `disabled_at`, `disabled_by`, `disabled_reason` fields populated
- ✅ Disabled workflows excluded from search results
- ✅ Workflow preserved in database (audit trail intact)
- ✅ Unit tests pass

---

### **Phase 3: Get Single Workflow + Unit Tests** (Day 3, 6 hours) 🔴 P0

**Goal**: Complete CRUD with GET single workflow and finalize unit test coverage

**Business Requirement**: BR-WORKFLOW-001 (FR-PLAYBOOK-001-03, implied)

#### **TDD RED Phase** (1.5 hours)
**File**: `test/unit/datastorage/workflow_crud_test.go` (extend)

```go
Context("GET /api/v1/workflows/{id}", func() {
    It("should return workflow by ID", func() {
        // Test: 200 OK with workflow details
    })
    It("should return 404 for non-existent workflow", func() {
        // Test: 404 Not Found
    })
    It("should return latest version by default", func() {
        // Test: Returns is_latest_version=true workflow
    })
})
```

#### **TDD GREEN Phase** (3 hours)
**Files**:
- `pkg/datastorage/server/workflow_handlers.go` - Add `HandleGetWorkflow`
- `pkg/datastorage/repository/workflow_repository.go` - Add `GetWorkflowByID`
- `pkg/datastorage/server/server.go` - Register route

**Route**:
```go
r.Get("/workflows/{workflow_id}", s.handler.HandleGetWorkflow)
```

#### **TDD REFACTOR Phase** (1.5 hours)
- Ensure consistent error responses (RFC 7807)
- Add request/response logging
- Run full unit test suite, ensure 70%+ coverage

**Success Criteria**:
- ✅ `GET /api/v1/workflows/{id}` returns 200 OK
- ✅ Returns latest version by default
- ✅ 404 for non-existent workflows
- ✅ All CRUD unit tests pass
- ✅ Unit test coverage ≥70%

---

### **Phase 4: Integration Tests for CRUD** (Day 4, 8 hours) 🔴 P0

**Goal**: Validate CRUD operations with real PostgreSQL database

**Business Requirement**: Testing validation per `.cursor/rules/03-testing-strategy.mdc`

#### **Test Implementation** (6 hours)
**File**: `test/integration/datastorage/workflow_crud_integration_test.go`

```go
var _ = Describe("Workflow CRUD Integration", Label("integration"), func() {
    var (
        db         *sql.DB
        httpClient *http.Client
        baseURL    string
    )

    BeforeAll(func() {
        // Start PostgreSQL container
        // Start Data Storage service
        // Configure test client
    })

    Context("Complete CRUD Lifecycle", func() {
        It("should create, read, update, and delete workflow", func() {
            // 1. POST /api/v1/workflows - Create
            // 2. GET /api/v1/workflows/{id} - Read
            // 3. PUT /api/v1/workflows/{id} - Update
            // 4. DELETE /api/v1/workflows/{id} - Delete
            // 5. GET /api/v1/workflows/{id} - Verify 404
        })

        It("should auto-generate embedding on create", func() {
            // Verify embedding column is populated
        })

        It("should update embedding when description changes", func() {
            // Verify embedding regenerated on description update
        })

        It("should exclude deleted workflows from search", func() {
            // 1. Create workflow
            // 2. Delete workflow
            // 3. Search - verify not returned
        })
    })
})
```

#### **Infrastructure Setup** (2 hours)
- Ensure `test/integration/datastorage/suite_test.go` starts PostgreSQL container
- Add helper functions for workflow CRUD operations
- Configure embedding service mock (or real service)

**Success Criteria**:
- ✅ Integration tests run against real PostgreSQL
- ✅ Full CRUD lifecycle validated
- ✅ Embedding generation validated
- ✅ Soft-delete behavior validated
- ✅ Integration test coverage >50%

---

### **Phase 5: E2E Tests for CRUD** (Day 5, 6 hours) 🔴 P0

**Goal**: Validate CRUD operations in full Kind cluster environment

**Business Requirement**: Testing validation per `.cursor/rules/03-testing-strategy.mdc`

#### **Test Implementation** (4 hours)
**File**: `test/e2e/datastorage/07_workflow_crud_test.go`

```go
var _ = Describe("Scenario 7: Workflow CRUD Operations", Label("e2e", "workflow-crud", "p0"), Ordered, func() {
    Context("when managing workflows via REST API", func() {
        It("should create workflow via POST /api/v1/workflows", func() {
            // Use shared dataStorageURL (NodePort)
            // POST workflow with all required fields
            // Verify 201 Created
        })

        It("should retrieve workflow via GET /api/v1/workflows/{id}", func() {
            // GET created workflow
            // Verify all fields returned correctly
        })

        It("should update workflow via PUT /api/v1/workflows/{id}", func() {
            // PUT updated workflow
            // Verify 200 OK
            // Verify changes persisted
        })

        It("should delete workflow via DELETE /api/v1/workflows/{id}", func() {
            // DELETE workflow
            // Verify 204 No Content
            // Verify excluded from search
        })
    })
})
```

#### **Fix Existing E2E Tests** (2 hours)
- Update `04_workflow_search_test.go` to use new `POST /api/v1/workflows`
- Update `05_embedding_service_integration_test.go` to use new endpoint
- Update `06_workflow_search_audit_test.go` to use new endpoint
- Remove SQL-based workflow seeding (use API instead)

**Success Criteria**:
- ✅ New E2E test (Scenario 7) passes
- ✅ Existing E2E tests (Scenarios 4, 5, 6) now pass
- ✅ All E2E tests use REST API (no SQL seeding)
- ✅ E2E test coverage <10% (per testing strategy)

---

### **Phase 6: Version History + Lifecycle APIs** (Day 6, 8 hours) 🟡 P1

**Goal**: Implement version history and lifecycle management endpoints

**Business Requirement**: BR-WORKFLOW-001 (FR-PLAYBOOK-001-04, FR-PLAYBOOK-001-05)

#### **TDD RED Phase** (2 hours)
```go
Context("GET /api/v1/workflows/{id}/versions", func() {
    It("should list all versions of a workflow", func() {})
    It("should order versions by created_at DESC", func() {})
})

Context("PATCH /api/v1/workflows/{id}/{version}", func() {
    It("should disable workflow version", func() {})
    It("should enable workflow version", func() {})
    It("should capture audit metadata on disable", func() {})
})
```

#### **TDD GREEN Phase** (4 hours)
**Endpoints**:
- `GET /api/v1/workflows/{id}/versions` - List all versions
- `GET /api/v1/workflows/{id}/versions/{version}` - Get specific version
- `PATCH /api/v1/workflows/{id}/versions/{version}` - Update status (disable/enable)

#### **TDD REFACTOR Phase** (2 hours)
- Add integration tests for version history
- Add audit logging for lifecycle changes
- Add Prometheus metrics for lifecycle operations

**Success Criteria**:
- ✅ Version history API returns all versions
- ✅ Disable/enable captures audit metadata
- ✅ Disabled workflows excluded from search
- ✅ Integration tests pass

---

### **Phase 7: Version Validation Library** (Day 7, 8 hours) 🟡 P1

**Goal**: Implement semantic version validation with immutability enforcement

**Business Requirement**: BR-WORKFLOW-001 (NFR-PLAYBOOK-001-03)

#### **TDD RED Phase** (2 hours)
**File**: `test/unit/datastorage/version_validator_test.go`

```go
var _ = Describe("Version Validator", func() {
    Context("ValidateVersionFormat", func() {
        It("should accept valid semver (v1.0.0)", func() {})
        It("should accept semver with pre-release (v1.0.0-alpha)", func() {})
        It("should reject invalid format (1.0, vv1.0.0)", func() {})
    })

    Context("IsValidIncrement", func() {
        It("should accept v1.1.0 after v1.0.0", func() {})
        It("should reject v0.9.0 after v1.0.0", func() {})
    })

    Context("Immutability", func() {
        It("should reject duplicate version with 409", func() {})
    })
})
```

#### **TDD GREEN Phase** (4 hours)
**File**: `pkg/datastorage/validation/version_validator.go`

```go
type VersionValidator struct{}

func (v *VersionValidator) ValidateFormat(version string) error {
    if !semver.IsValid(version) {
        return ErrInvalidVersionFormat
    }
    return nil
}

func (v *VersionValidator) IsValidIncrement(current, new string) error {
    if semver.Compare(new, current) <= 0 {
        return ErrVersionNotIncremented
    }
    return nil
}
```

#### **TDD REFACTOR Phase** (2 hours)
- Integrate validator into create/update handlers
- Add clear error messages for validation failures
- Add integration tests

**Success Criteria**:
- ✅ Semver format validation works
- ✅ Version increment enforced
- ✅ Immutability enforced (409 on duplicate)
- ✅ 100% unit test coverage for validator

---

### **Phase 8: Embedding Caching** (Day 8, 8 hours) 🟢 P2

**Goal**: Redis-backed embedding cache for 50× performance improvement

**Business Requirement**: Performance optimization (NFR-PLAYBOOK-001-01)

#### **TDD RED Phase** (2 hours)
**File**: `test/unit/datastorage/embedding_cache_test.go`

```go
var _ = Describe("Embedding Cache", func() {
    It("should return cached embedding on hit", func() {})
    It("should generate and cache embedding on miss", func() {})
    It("should invalidate cache on workflow update", func() {})
    It("should track cache hit/miss metrics", func() {})
})
```

#### **TDD GREEN Phase** (4 hours)
**File**: `pkg/datastorage/embedding/cache.go`

```go
type EmbeddingCache struct {
    redis  *redis.Client
    ttl    time.Duration
}

func (c *EmbeddingCache) Get(ctx context.Context, workflowID, version string) ([]float32, error) {
    key := fmt.Sprintf("embedding:workflow:%s:%s", workflowID, version)
    // Check Redis cache
    // Return cached embedding or ErrCacheMiss
}

func (c *EmbeddingCache) Set(ctx context.Context, workflowID, version string, embedding []float32) error {
    // Store in Redis with TTL
}

func (c *EmbeddingCache) Invalidate(ctx context.Context, workflowID string) error {
    // Delete all versions for workflow
}
```

#### **TDD REFACTOR Phase** (2 hours)
- Integrate cache into semantic search pipeline
- Add Prometheus metrics: `datastorage_embedding_cache_hits_total`, `datastorage_embedding_cache_misses_total`
- Add integration tests with real Redis

**Success Criteria**:
- ✅ Cache hit reduces latency from 2.5s to ~50ms
- ✅ Cache miss generates and caches embedding
- ✅ Cache invalidation works on create/update/delete
- ✅ Metrics track hit/miss rate

---

### **Phase 9: Cache Invalidation + Final Tests** (Day 9, 6 hours) 🟢 P2

**Goal**: REST API for cache invalidation and final validation

#### **Implementation** (3 hours)
**Endpoints**:
- `DELETE /api/v1/cache/workflows/{id}` - Invalidate specific workflow
- `DELETE /api/v1/cache/workflows` - Invalidate all workflows

**Integration with CRUD**:
- Auto-invalidate on `POST /api/v1/workflows`
- Auto-invalidate on `PUT /api/v1/workflows/{id}`
- Auto-invalidate on `DELETE /api/v1/workflows/{id}`

#### **Final Validation** (3 hours)
- Run full unit test suite (70%+ coverage)
- Run full integration test suite (>50% coverage)
- Run full E2E test suite (<10% coverage)
- Verify all BR-WORKFLOW-001 requirements met
- Update documentation

**Success Criteria**:
- ✅ Cache invalidation REST endpoints work
- ✅ Auto-invalidation on CRUD operations
- ✅ All test suites pass
- ✅ BR-WORKFLOW-001 fully implemented
- ✅ Documentation updated

---

## 📊 **Timeline & Effort Summary** - EXPANDED November 28, 2025

| Phase | Day | Hours | Focus | Priority | Status |
|-------|-----|-------|-------|----------|--------|
| **Phase 1** | Day 1 | 8h | Create Workflow (`POST`) | 🔴 P0 | ❌ NOT STARTED |
| **Phase 2** | Day 2 | 8h | Update/Delete Workflow | 🔴 P0 | ❌ NOT STARTED |
| **Phase 3** | Day 3 | 6h | Get Single + Unit Tests | 🔴 P0 | ❌ NOT STARTED |
| **Phase 4** | Day 4 | 8h | Integration Tests (CRUD) | 🔴 P0 | ❌ NOT STARTED |
| **Phase 5** | Day 5 | 6h | E2E Tests (CRUD) | 🔴 P0 | ❌ NOT STARTED |
| **Phase 6** | Day 6 | 8h | Version History + Lifecycle | 🟡 P1 | ❌ NOT STARTED |
| **Phase 7** | Day 7 | 8h | Version Validation | 🟡 P1 | ❌ NOT STARTED |
| **Phase 8** | Day 8 | 8h | Embedding Caching | 🟢 P2 | ❌ NOT STARTED |
| **Phase 9** | Day 9 | 6h | Cache Invalidation + Final | 🟢 P2 | ❌ NOT STARTED |
| **Total** | **9 days** | **66 hours** | **V1.1 Complete** | | ❌ NOT STARTED |

### **Priority Breakdown**

| Priority | Days | Hours | Features | Business Impact |
|----------|------|-------|----------|-----------------|
| 🔴 **P0** | Days 1-5 | 36h | Basic CRUD + Tests | Unblocks E2E tests (Scenarios 4, 5, 6) |
| 🟡 **P1** | Days 6-7 | 16h | Version History + Validation | Quality controls, BR-WORKFLOW-001 compliance |
| 🟢 **P2** | Days 8-9 | 14h | Caching | 50× performance improvement |

### **Milestone Checkpoints**

| Checkpoint | After Phase | Validation |
|------------|-------------|------------|
| **MVP CRUD** | Phase 3 (Day 3) | All CRUD unit tests pass |
| **CRUD Validated** | Phase 5 (Day 5) | E2E tests (4, 5, 6, 7) pass |
| **Full BR-WORKFLOW-001** | Phase 7 (Day 7) | All BR-WORKFLOW-001 FRs implemented |
| **V1.1 Complete** | Phase 9 (Day 9) | All test tiers pass, 50× perf improvement |

---

## 🎯 **Success Criteria** - EXPANDED November 28, 2025

### **P0 Success Criteria (Days 1-5)** - Unblocks E2E Tests

| Requirement | Endpoint | Test Tier | Status |
|-------------|----------|-----------|--------|
| Create workflow | `POST /api/v1/workflows` | Unit + Integration + E2E | ❌ |
| Update workflow | `PUT /api/v1/workflows/{id}` | Unit + Integration + E2E | ❌ |
| Delete workflow | `DELETE /api/v1/workflows/{id}` | Unit + Integration + E2E | ❌ |
| Get workflow | `GET /api/v1/workflows/{id}` | Unit + Integration + E2E | ❌ |
| Auto-generate embedding | On create/update | Integration + E2E | ❌ |
| Soft-delete behavior | On delete | Integration + E2E | ❌ |
| E2E Scenario 4 passes | Workflow Search | E2E | ❌ |
| E2E Scenario 5 passes | Embedding Service | E2E | ❌ |
| E2E Scenario 6 passes | Search Audit Trail | E2E | ❌ |
| E2E Scenario 7 passes | CRUD Operations | E2E | ❌ |

### **P1 Success Criteria (Days 6-7)** - BR-WORKFLOW-001 Compliance

| Requirement | Endpoint | FR Reference | Status |
|-------------|----------|--------------|--------|
| List versions | `GET /api/v1/workflows/{id}/versions` | FR-PLAYBOOK-001-04 | ❌ |
| Get specific version | `GET /api/v1/workflows/{id}/versions/{v}` | FR-PLAYBOOK-001-04 | ❌ |
| Disable workflow | `PATCH /api/v1/workflows/{id}/{v}` | FR-PLAYBOOK-001-05 | ❌ |
| Enable workflow | `PATCH /api/v1/workflows/{id}/{v}` | FR-PLAYBOOK-001-05 | ❌ |
| Semver validation | On create | NFR-PLAYBOOK-001-03 | ❌ |
| Version increment | On create | NFR-PLAYBOOK-001-03 | ❌ |
| Immutability (409) | On duplicate | NFR-PLAYBOOK-001-03 | ❌ |

### **P2 Success Criteria (Days 8-9)** - Performance Optimization

| Requirement | Target | Metric | Status |
|-------------|--------|--------|--------|
| Cache hit latency | < 100ms | `datastorage_embedding_cache_latency_seconds` | ❌ |
| Cache miss latency | < 3s | `datastorage_embedding_generation_seconds` | ❌ |
| Cache hit rate | > 80% | `datastorage_embedding_cache_hits_total` | ❌ |
| Cache invalidation | On CRUD | Auto-triggered | ❌ |
| REST invalidation | Manual trigger | `DELETE /api/v1/cache/workflows` | ❌ |

### **Test Coverage Requirements**

| Tier | Target | Current | Status |
|------|--------|---------|--------|
| Unit Tests | ≥70% | TBD | ❌ |
| Integration Tests | >50% | TBD | ❌ |
| E2E Tests | <10% | TBD | ❌ |

### **Validation Gate**
🚨 **CRITICAL**: No feature is marked complete until it passes ALL 3 test tiers:
1. ✅ Unit tests pass
2. ✅ Integration tests pass
3. ✅ E2E tests pass

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

**Document Version**: 3.3
**Created**: November 14, 2025
**Last Updated**: November 28, 2025
**Status**: ✅ **APPROVED** - Ready for implementation with approved design decisions

### **Version History**
| Version | Date | Status | Notes |
|---------|------|--------|-------|
| 1.0 | Nov 14, 2025 | Initial | V1.1 plan with full CRUD scope |
| 2.0 | Nov 22, 2025 | ⚠️ Inaccurate | Incorrectly claimed V1.0 includes CRUD |
| 3.0 | Nov 28, 2025 | ✅ Corrected | CRUD confirmed NOT implemented; V1.1 scope restored |
| 3.1 | Nov 28, 2025 | 📋 Expanded | Detailed 9-day plan with TDD phases, code examples, test specs |
| 3.2 | Nov 28, 2025 | ✅ Approved | Added approved design decisions (DD-1 through DD-5), pre-impl checklist |
| 3.3 | Nov 28, 2025 | ✅ DD-API-001 | Fixed disable reason: JSON body (not HTTP header) |

### **Document Summary**
- **Total Implementation Time**: 9 days (66 hours)
- **P0 (Unblocks E2E)**: Days 1-5 (36 hours) - Create + Disable + Get + Tests
- **P1 (BR Compliance)**: Days 6-7 (16 hours) - Version History + Validation
- **P2 (Performance)**: Days 8-9 (14 hours) - Caching
- **Validation Requirement**: All features must pass unit + integration + E2E tests
- **Key Decisions**: PUT not allowed (immutability), DELETE = disable, sync embedding

