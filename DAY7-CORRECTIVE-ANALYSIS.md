# Day 7 Corrective Action - ANALYSIS Phase

**Date**: 2025-11-03  
**Phase**: APDC ANALYSIS  
**Duration**: 30 minutes  
**Status**: ✅ Complete

---

## 🎯 **Objective**

Analyze requirements for HTTP WRITE API implementation to correct Day 7 gap (missing POST `/api/v1/audit/notifications` endpoint).

---

## 📋 **Current State Assessment**

### **What EXISTS** ✅
| Component | Status | File | Notes |
|-----------|--------|------|-------|
| **HTTP Server Infrastructure** | ✅ Complete | `cmd/datastorage/main.go` | Main entry point, DB connection, graceful shutdown |
| **Router Setup** | ✅ Complete | `pkg/datastorage/server/server.go` | Chi router, middleware, health endpoints |
| **Health Endpoints** | ✅ Complete | `pkg/datastorage/server/server.go` | `/health`, `/health/ready`, `/health/live` (DD-007) |
| **READ API Handlers** | ✅ Complete | `pkg/datastorage/server/handler.go` | `ListIncidents`, `GetIncident` |
| **Repository Layer** | ✅ Complete | `pkg/datastorage/repository/notification_audit_repository.go` | PostgreSQL persistence with RFC 7807 |
| **DLQ Client** | ✅ Complete | `pkg/datastorage/dlq/client.go` | Redis Streams fallback (DD-009) |
| **Validation Layer** | ✅ Complete | `pkg/datastorage/validation/notification_audit_validator.go` | Input validation + RFC 7807 errors |
| **Data Models** | ✅ Complete | `pkg/datastorage/models/notification_audit.go` | NotificationAudit struct |
| **Database Schema** | ✅ Complete | `migrations/010_audit_write_api_phase1.sql` | notification_audit table |
| **Dockerfile** | ✅ Complete | `docker/data-storage.Dockerfile` | ADR-027 compliant (UBI base) |

### **What is MISSING** ❌
| Component | Status | Expected File | Requirement |
|-----------|--------|---------------|-------------|
| **WRITE API Handler** | ❌ Missing | `pkg/datastorage/server/audit_handlers.go` | POST `/api/v1/audit/notifications` |
| **WRITE API Routes** | ❌ Missing | `pkg/datastorage/server/server.go` (add routes) | Register audit endpoints |
| **HTTP Integration Tests** | ❌ Missing | `test/integration/datastorage/http_api_test.go` | Test HTTP → DB flow |

---

## 📖 **Plan Requirements Analysis**

### **From IMPLEMENTATION_PLAN_V4.8.md (Lines 1402-1600)**

**Day 7 Specification**:
- **Duration**: 3 hours
- **Infrastructure**: Podman PostgreSQL + Redis + **Data Storage Service container**
- **Tests**: HTTP API integration tests (POST `/api/v1/audit/notifications`)
- **Scope**: 1 service (Notification)

**Key Requirements**:
1. Build Data Storage Service image (line 1479)
2. Start service container exposing port 8080 (line 1482-1496)
3. Wait for service `/health` endpoint (line 1502-1507)
4. Test POST `/api/v1/audit/notifications` (implied by service container requirement)
5. Verify data in PostgreSQL (correctness testing)
6. Test DLQ fallback when DB is down

---

## 🔍 **Existing Server Architecture**

### **Router Structure** (`pkg/datastorage/server/server.go`)

```go
// Current routes (READ API only)
r.Route("/api/v1", func(r chi.Router) {
    // BR-STORAGE-021: Incident query endpoints
    r.Get("/incidents", s.handler.ListIncidents)
    r.Get("/incidents/{id}", s.handler.GetIncident)
})
```

**Required Addition** (WRITE API):
```go
r.Route("/api/v1", func(r chi.Router) {
    // READ API (existing)
    r.Get("/incidents", s.handler.ListIncidents)
    r.Get("/incidents/{id}", s.handler.GetIncident)
    
    // WRITE API (NEW - Day 7)
    r.Post("/audit/notifications", s.handleCreateNotificationAudit)
})
```

### **Server Dependencies**

**Current**:
- `Handler` (READ API): `pkg/datastorage/server/handler.go`
- `DBAdapter`: Wraps `*sql.DB` for query operations

**Required**:
- `NotificationAuditRepository`: Already exists ✅
- `DLQClient`: Already exists ✅
- `NotificationAuditValidator`: Already exists ✅

**Integration Point**: Server needs to instantiate repository + DLQ client + validator

---

## 🎯 **HTTP WRITE API Requirements**

### **Endpoint Specification**

**POST `/api/v1/audit/notifications`**

**Request**:
```json
{
  "remediation_id": "remediation-123",
  "notification_id": "notification-456",
  "recipient": "ops-team@example.com",
  "channel": "email",
  "message_summary": "Alert: High CPU usage on prod-server-01",
  "status": "sent",
  "sent_at": "2025-11-03T10:30:00Z",
  "delivery_status": "200 OK",
  "error_message": "",
  "escalation_level": 0
}
```

**Success Response** (201 Created):
```json
{
  "id": 123,
  "remediation_id": "remediation-123",
  "notification_id": "notification-456",
  "recipient": "ops-team@example.com",
  "channel": "email",
  "message_summary": "Alert: High CPU usage on prod-server-01",
  "status": "sent",
  "sent_at": "2025-11-03T10:30:00Z",
  "delivery_status": "200 OK",
  "error_message": "",
  "escalation_level": 0,
  "created_at": "2025-11-03T10:30:01Z",
  "updated_at": "2025-11-03T10:30:01Z"
}
```

**Error Response** (400 Bad Request - RFC 7807):
```json
{
  "type": "https://kubernaut.io/errors/validation-error",
  "title": "Validation Error",
  "status": 400,
  "detail": "Validation failed for resource 'notification_audit'",
  "instance": "/api/v1/audit/notifications",
  "resource": "notification_audit",
  "field_errors": {
    "remediation_id": "required field is empty",
    "notification_id": "required field is empty"
  }
}
```

**Error Response** (409 Conflict - RFC 7807):
```json
{
  "type": "https://kubernaut.io/errors/conflict",
  "title": "Resource Conflict",
  "status": 409,
  "detail": "Resource 'notification_audit' already exists with notification_id 'notification-456'",
  "instance": "/api/v1/audit/notifications",
  "resource": "notification_audit",
  "field": "notification_id",
  "value": "notification-456"
}
```

---

## 🏗️ **Handler Implementation Strategy**

### **Option A: Separate Audit Handler File** (Recommended)

**File**: `pkg/datastorage/server/audit_handlers.go`

**Pros**:
- ✅ Clean separation (READ API vs WRITE API)
- ✅ Easier to test independently
- ✅ Scales well (add more audit endpoints later)
- ✅ Follows existing pattern (`handler.go` for READ, `audit_handlers.go` for WRITE)

**Cons**:
- ⚠️ One more file to maintain

### **Option B: Add to Existing Handler**

**File**: `pkg/datastorage/server/handler.go`

**Pros**:
- ✅ All handlers in one place

**Cons**:
- ❌ Mixes READ and WRITE concerns
- ❌ File becomes large (already 65 lines, would grow to 200+)
- ❌ Harder to test independently

**Recommendation**: **Option A** (separate file)

---

## 🧪 **Integration Test Strategy**

### **Test Infrastructure** (Already Working ✅)

From `test/integration/datastorage/suite_test.go`:
- PostgreSQL container running (port 5433)
- Redis container running (port 6379)
- Schema applied with migrations
- Permissions granted

**Missing**: Data Storage Service container

### **Required BeforeSuite Changes**

```go
// ADDITION: Build and start Data Storage Service
GinkgoWriter.Println("🏗️  Building Data Storage Service image...")
buildCmd := exec.Command("podman", "build",
    "-t", "data-storage:test",
    "-f", "docker/data-storage.Dockerfile",
    ".")
output, err := buildCmd.CombinedOutput()
Expect(err).ToNot(HaveOccurred(), "Failed to build service: %s", string(output))

GinkgoWriter.Println("🚀 Starting Data Storage Service container...")
serviceContainer = "datastorage-service-test"
startCmd := exec.Command("podman", "run", "-d",
    "--name", serviceContainer,
    "-p", "8080:8080",
    "-e", "DB_HOST=host.containers.internal",
    "-e", "DB_PORT=5433",
    "-e", "DB_NAME=action_history",
    "-e", "DB_USER=slm_user",
    "-e", "DB_PASSWORD=test_password",
    "-e", "REDIS_ADDR=host.containers.internal:6379",
    "data-storage:test")
output, err = startCmd.CombinedOutput()
Expect(err).ToNot(HaveOccurred(), "Failed to start service: %s", string(output))

// Wait for service health endpoint
datastorageURL = "http://localhost:8080"
Eventually(func() int {
    resp, err := http.Get(datastorageURL + "/health")
    if err != nil || resp == nil {
        return 0
    }
    return resp.StatusCode
}, "30s", "1s").Should(Equal(200), "Service should be healthy")
```

### **Test Scenarios**

**File**: `test/integration/datastorage/http_api_test.go`

1. **Successful Write** (Behavior + Correctness)
   - POST valid payload
   - Expect 201 Created
   - Verify data in PostgreSQL (correctness)

2. **Validation Error** (RFC 7807)
   - POST invalid payload (missing required fields)
   - Expect 400 Bad Request
   - Verify RFC 7807 error structure

3. **Conflict Error** (RFC 7807)
   - POST same notification_id twice
   - Expect 409 Conflict
   - Verify RFC 7807 error structure

4. **DLQ Fallback** (DD-009)
   - Stop PostgreSQL container
   - POST valid payload
   - Expect 201 Created (async write to DLQ)
   - Verify message in Redis DLQ

---

## 📊 **Dependencies Analysis**

### **Server Needs** (for WRITE API)

| Dependency | Status | Integration Point |
|------------|--------|-------------------|
| `NotificationAuditRepository` | ✅ Exists | Instantiate in `NewServer()` |
| `DLQClient` | ✅ Exists | Instantiate in `NewServer()` |
| `NotificationAuditValidator` | ✅ Exists | Instantiate in handler |
| `Redis Client` | ❌ Missing | Add to `NewServer()` parameters |

**Required Change**: `NewServer()` signature needs Redis connection string

```go
// Current
func NewServer(connStr string, logger *zap.Logger, cfg *Config) (*Server, error)

// Required
func NewServer(dbConnStr, redisAddr string, logger *zap.Logger, cfg *Config) (*Server, error)
```

---

## 🎯 **Success Criteria**

### **Implementation Complete When**:
1. ✅ POST `/api/v1/audit/notifications` endpoint exists
2. ✅ Request validation working (RFC 7807 errors)
3. ✅ Data persisted to PostgreSQL
4. ✅ DLQ fallback working when DB down
5. ✅ HTTP integration tests passing (4 scenarios)
6. ✅ Service container builds and starts
7. ✅ Health endpoint responsive

---

## 📋 **PLAN Phase Preview**

### **Estimated Effort**

| Task | Duration | Confidence |
|------|----------|------------|
| DO-RED: Write HTTP integration tests | 1.5h | 95% |
| DO-GREEN: Implement audit handler | 1.5h | 95% |
| DO-GREEN: Update server routes | 0.5h | 100% |
| DO-GREEN: Update BeforeSuite | 0.5h | 95% |
| CHECK: Validate all tests pass | 0.5h | 90% |
| **TOTAL** | **4.5h** | **95%** |

**Note**: Slightly over original 3h estimate due to corrective nature (need to refactor existing integration tests)

---

## 🔗 **References**

- **Plan**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.8.md` (lines 1402-1600)
- **Existing Repository**: `pkg/datastorage/repository/notification_audit_repository.go`
- **Existing DLQ**: `pkg/datastorage/dlq/client.go`
- **Existing Validation**: `pkg/datastorage/validation/notification_audit_validator.go`
- **DD-007**: Graceful Shutdown (already implemented in server)
- **DD-009**: Audit Write Error Recovery (DLQ pattern)
- **ADR-027**: Multi-Architecture Container Build Strategy
- **ADR-032**: Data Access Layer Isolation

---

## ✅ **ANALYSIS Complete**

**Confidence**: **95%**

**Rationale**:
1. ✅ All dependencies exist (repository, DLQ, validation)
2. ✅ Server infrastructure exists (router, middleware, health)
3. ✅ Integration test infrastructure working (PostgreSQL + Redis)
4. ✅ Clear requirements from plan
5. ⚠️ Minor uncertainty: Redis connection string propagation (5% risk)

**Next**: PLAN Phase - Define detailed implementation strategy

