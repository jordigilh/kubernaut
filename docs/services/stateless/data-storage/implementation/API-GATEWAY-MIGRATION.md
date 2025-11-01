# Data Storage Service - API Gateway Migration

**Related Decision**: [DD-ARCH-001: Alternative 2 (API Gateway Pattern)](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)
**Date**: November 2, 2025
**Status**: ✅ **APPROVED FOR IMPLEMENTATION**
**Service**: Data Storage Service
**Timeline**: **4-5 Days** (Phase 1 of overall migration)

---

## 🎯 **WHAT THIS SERVICE NEEDS TO DO**

**Current State**: Data Storage Service only handles audit trail writes (`POST /api/v1/audit`)

**New State**: Data Storage Service becomes **REST API Gateway for ALL database access**

**Changes Needed**:
1. ✅ Add read API endpoints (`GET /api/v1/incidents`, `GET /api/v1/effectiveness/metrics`)
2. ✅ Reuse Context API's SQL builder (extract to shared package)
3. ✅ Update service specification and API documentation
4. ✅ Add integration tests for read endpoints

---

## 📋 **SPECIFICATION CHANGES**

### **1. Service Overview Update**

**File**: `overview.md`

**Current**:
> Data Storage Service provides audit trail write operations for Kubernaut.

**New**:
> Data Storage Service is the **REST API Gateway for all database access** in Kubernaut.
>
> **Responsibilities**:
> - **Write API**: Audit trail data from CRD controllers
> - **Read API**: Historical queries from Context API, Effectiveness Monitor
>
> **Design Decision**: [DD-ARCH-001 Alternative 2](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)

---

### **2. API Specification Update**

**File**: `api-specification.md`

**Add New Section**: Read Endpoints

```markdown
## Read Endpoints (DD-ARCH-001)

### GET /api/v1/incidents

Query historical incidents with filters and pagination.

**Query Parameters**:
- `namespace` (string, optional) - Filter by namespace
- `severity` (string, optional) - Filter by severity (critical, high, medium, low)
- `cluster` (string, optional) - Filter by cluster name
- `environment` (string, optional) - Filter by environment
- `action_type` (string, optional) - Filter by action type
- `limit` (integer, optional, default: 100, max: 1000) - Results per page
- `offset` (integer, optional, default: 0) - Pagination offset

**Response** (200 OK):
```json
{
  "incidents": [
    {
      "id": 123,
      "name": "pod-oom-alert",
      "namespace": "production",
      "severity": "high",
      "phase": "completed",
      "...": "..."
    }
  ],
  "total": 150,
  "limit": 100,
  "offset": 0
}
```

**Error Responses** (RFC 7807):
- 400 Bad Request - Invalid query parameters
- 500 Internal Server Error - Database error
```

---

### **3. Integration Points Update**

**File**: `integration-points.md`

**Add New Section**: Read API Clients

```markdown
## Clients

### Write API Clients
- RemediationProcessor Controller
- AIAnalysis Controller
- All CRD controllers

### Read API Clients (NEW - DD-ARCH-001)
- **Context API** - Queries historical incidents for context enrichment
- **Effectiveness Monitor** - Queries effectiveness metrics and analysis data

**API Contract**: See [api-specification.md](./api-specification.md)
```

---

## 🚀 **IMPLEMENTATION PLAN**

### **Day 1: Extract SQL Builder** (4-6 hours)

**Objective**: Move Context API's SQL builder to shared package

**Tasks**:
1. Create `pkg/datastorage/query/` package
2. Extract SQL builder from `pkg/contextapi/sqlbuilder/builder.go`
3. Extract validation from `pkg/contextapi/sqlbuilder/validation.go`
4. Extract errors from `pkg/contextapi/sqlbuilder/errors.go`
5. Update Context API imports

**Code to Extract** (~600 lines):
- `pkg/contextapi/sqlbuilder/builder.go` → `pkg/datastorage/query/builder.go`
- `pkg/contextapi/sqlbuilder/validation.go` → `pkg/datastorage/query/validation.go`
- `pkg/contextapi/sqlbuilder/errors.go` → `pkg/datastorage/query/errors.go`

**Deliverables**:
- ✅ SQL builder in shared package
- ✅ Context API still works (imports updated)
- ✅ All tests passing

---

### **Day 2-3: REST API Endpoints** (8-12 hours)

**Objective**: Implement read endpoints using extracted SQL builder

**Tasks**:
1. Create `pkg/datastorage/server/` package structure
2. Implement `ListIncidents()` handler (reuse SQL builder)
3. Implement `GetIncident(id)` handler
4. Create HTTP server with routing
5. Add health endpoint

**New Files**:
- `pkg/datastorage/server/server.go` - HTTP server
- `pkg/datastorage/server/handlers.go` - Request handlers
- `pkg/datastorage/server/router.go` - HTTP routing

**Example Handler** (reuses extracted SQL builder):
```go
func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
    params := h.parseListParams(r)

    // Use extracted SQL builder (from Context API)
    builder := query.NewBuilder()
    if params.Namespace != nil {
        builder = builder.WithNamespace(*params.Namespace)
    }
    // ... apply other filters

    sqlQuery, args, _ := builder.Build()

    var incidents []*models.IncidentEvent
    h.db.SelectContext(ctx, &incidents, sqlQuery, args...)

    json.NewEncoder(w).Encode(&ListIncidentsResponse{
        Incidents: incidents,
        Total: total,
    })
}
```

**Deliverables**:
- ✅ REST API endpoints working
- ✅ Manual testing with curl
- ✅ SQL builder reused successfully

---

### **Day 4: Integration Tests** (4-6 hours)

**Objective**: Test read endpoints with real PostgreSQL

**Tasks**:
1. Create `test/integration/datastorage/01_read_api_test.go`
2. Test filtering (namespace, severity, cluster)
3. Test pagination (limit, offset)
4. Test error cases (invalid params)

**Test Example**:
```go
var _ = Describe("Data Storage Read API", func() {
    It("should filter incidents by namespace", func() {
        // Insert test data
        testutil.InsertIncident(db, &models.IncidentEvent{
            Namespace: "production",
            Severity: "high",
        })

        // Query via REST API
        resp, _ := http.Get("http://localhost:8080/api/v1/incidents?namespace=production")

        var result models.ListIncidentsResponse
        json.NewDecoder(resp.Body).Decode(&result)

        Expect(result.Incidents).To(HaveLen(1))
        Expect(result.Incidents[0].Namespace).To(Equal("production"))
    })
})
```

**Deliverables**:
- ✅ Integration tests passing
- ✅ All query scenarios validated

---

### **Day 5: Documentation & Deployment** (2-4 hours)

**Objectives**: Update documentation and deployment configs

**Tasks**:
1. Update `overview.md` (service purpose)
2. Update `api-specification.md` (add read endpoints)
3. Update `integration-points.md` (add clients)
4. Update deployment manifests (if needed)

**Deliverables**:
- ✅ All documentation updated
- ✅ Deployment ready
- ✅ **Data Storage Service now serves as API Gateway**

---

## 📊 **CODE REUSE SUMMARY**

| Component | Source | Lines | Reuse % |
|-----------|--------|-------|---------|
| SQL Builder | Context API | 285 | 100% |
| SQL Validation | Context API | 50 | 100% |
| SQL Errors | Context API | 30 | 100% |
| **Total Reused** | | **365** | **100%** |
| **New Code** | | **~300** | N/A |

**Total Implementation**: ~665 lines (55% reused, 45% new)

---

## ✅ **SUCCESS CRITERIA**

- ✅ SQL builder extracted to `pkg/datastorage/query/`
- ✅ REST API endpoints implemented:
  - `GET /api/v1/incidents` with filtering and pagination
  - `GET /api/v1/incidents/:id`
- ✅ Integration tests passing (all query scenarios)
- ✅ Service specification updated
- ✅ API specification updated with read endpoints
- ✅ Manual testing successful (curl)
- ✅ **Ready for Context API to migrate** (Phase 2)

---

## 🔗 **RELATED DOCUMENTATION**

- [DD-ARCH-001 Final Decision](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md) - Architecture decision
- [Context API Migration](../../context-api/implementation/API-GATEWAY-MIGRATION.md) - Phase 2 (depends on this)
- [Effectiveness Monitor Migration](../../effectiveness-monitor/implementation/API-GATEWAY-MIGRATION.md) - Phase 3 (depends on this)

---

**Status**: ✅ **APPROVED - Ready for implementation**
**Dependencies**: None (this is Phase 1)
**Blocks**: Context API migration (Phase 2), Effectiveness Monitor migration (Phase 3)

