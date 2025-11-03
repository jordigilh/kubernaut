# Day 7 Corrective Action - PLAN Phase

**Date**: 2025-11-03  
**Phase**: APDC PLAN  
**Duration**: 20 minutes  
**Status**: ✅ Complete

---

## 🎯 **Objective**

Define detailed implementation strategy for HTTP WRITE API to correct Day 7 gap.

---

## 📋 **Implementation Strategy**

### **Approach: Minimal Changes to Existing Infrastructure**

**Rationale**:
- Server infrastructure (router, middleware, health) already exists ✅
- Repository, DLQ, Validation already exist ✅
- Only need to add WRITE endpoint and wire dependencies

**Confidence**: 95%

---

## 🏗️ **Architecture Decision**

### **Handler File Structure**

**Decision**: Create separate `audit_handlers.go` file

**File**: `pkg/datastorage/server/audit_handlers.go`

**Rationale**:
1. Clean separation (READ vs WRITE API)
2. Easier to test independently
3. Scales for future audit endpoints (signal-processing, orchestration, etc.)
4. Follows single responsibility principle

---

## 📝 **Detailed Implementation Plan**

### **Phase 1: DO-RED - Write HTTP Integration Tests FIRST** (1.5h)

#### **File 1: Update BeforeSuite** (`test/integration/datastorage/suite_test.go`)

**Changes**:
1. Add Redis client initialization (for DLQ)
2. Build Data Storage Service image
3. Start service container with DB + Redis env vars
4. Wait for `/health` endpoint
5. Store `datastorageURL` for tests

**Code Structure**:
```go
var (
    db                *sql.DB
    redisClient       *redis.Client
    datastorageURL    string
    postgresContainer string
    redisContainer    string
    serviceContainer  string
)

var _ = BeforeSuite(func() {
    // 1. Start PostgreSQL (existing) ✅
    // 2. Start Redis (existing) ✅
    // 3. Apply migrations (existing) ✅
    
    // 4. NEW: Build Data Storage Service
    buildDataStorageImage()
    
    // 5. NEW: Start service container
    startDataStorageService()
    
    // 6. NEW: Wait for health endpoint
    waitForServiceReady()
})
```

#### **File 2: Create HTTP API Integration Tests** (`test/integration/datastorage/http_api_test.go`)

**Test Structure**:
```go
package datastorage

import (
    "bytes"
    "encoding/json"
    "net/http"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    
    "github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("HTTP API Integration - POST /api/v1/audit/notifications", func() {
    var (
        client      *http.Client
        validAudit  *models.NotificationAudit
    )
    
    BeforeEach(func() {
        client = &http.Client{Timeout: 5 * time.Second}
        validAudit = &models.NotificationAudit{
            RemediationID:   "test-remediation-1",
            NotificationID:  fmt.Sprintf("test-notification-%d", time.Now().UnixNano()),
            Recipient:       "test@example.com",
            Channel:         "email",
            MessageSummary:  "Test notification",
            Status:          "sent",
            SentAt:          time.Now(),
            DeliveryStatus:  "200 OK",
            EscalationLevel: 0,
        }
    })
    
    Context("Successful write", func() {
        It("should accept valid audit record (Behavior + Correctness)", func() {
            // BEHAVIOR: HTTP 201 Created
            resp := postAudit(client, validAudit)
            Expect(resp.StatusCode).To(Equal(201))
            
            // CORRECTNESS: Data in PostgreSQL
            var count int
            db.QueryRow("SELECT COUNT(*) FROM notification_audit WHERE notification_id = $1",
                validAudit.NotificationID).Scan(&count)
            Expect(count).To(Equal(1))
            
            // CORRECTNESS: Response contains created record
            var created models.NotificationAudit
            json.NewDecoder(resp.Body).Decode(&created)
            Expect(created.ID).To(BeNumerically(">", 0))
            Expect(created.NotificationID).To(Equal(validAudit.NotificationID))
        })
    })
    
    Context("Validation errors", func() {
        It("should return RFC 7807 error for missing required fields", func() {
            invalidAudit := &models.NotificationAudit{
                // Missing required fields
            }
            
            resp := postAudit(client, invalidAudit)
            Expect(resp.StatusCode).To(Equal(400))
            Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
            
            var errorResp map[string]interface{}
            json.NewDecoder(resp.Body).Decode(&errorResp)
            Expect(errorResp["type"]).To(Equal("https://kubernaut.io/errors/validation-error"))
            Expect(errorResp["status"]).To(BeNumerically("==", 400))
        })
    })
    
    Context("Conflict errors", func() {
        It("should return RFC 7807 error for duplicate notification_id", func() {
            // First write
            resp1 := postAudit(client, validAudit)
            Expect(resp1.StatusCode).To(Equal(201))
            
            // Duplicate write
            resp2 := postAudit(client, validAudit)
            Expect(resp2.StatusCode).To(Equal(409))
            Expect(resp2.Header.Get("Content-Type")).To(Equal("application/problem+json"))
            
            var errorResp map[string]interface{}
            json.NewDecoder(resp2.Body).Decode(&errorResp)
            Expect(errorResp["type"]).To(Equal("https://kubernaut.io/errors/conflict"))
        })
    })
    
    Context("DLQ fallback (DD-009)", func() {
        It("should write to DLQ when database is unavailable", func() {
            // Stop PostgreSQL container
            stopPostgres()
            defer startPostgres()
            
            // POST should still succeed (async DLQ write)
            resp := postAudit(client, validAudit)
            Expect(resp.StatusCode).To(Equal(202)) // Accepted (async)
            
            // Verify message in Redis DLQ
            depth, err := getDLQDepth(redisClient, "notification")
            Expect(err).ToNot(HaveOccurred())
            Expect(depth).To(BeNumerically(">", 0))
        })
    })
})

// Helper functions
func postAudit(client *http.Client, audit *models.NotificationAudit) *http.Response {
    payload, _ := json.Marshal(audit)
    req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/notifications",
        bytes.NewBuffer(payload))
    req.Header.Set("Content-Type", "application/json")
    resp, _ := client.Do(req)
    return resp
}
```

**Test Count**: 4 tests
**Expected Result**: All tests FAIL (no handler exists yet)

---

### **Phase 2: DO-GREEN - Implement HTTP WRITE Handlers** (2h)

#### **File 1: Create Audit Handlers** (`pkg/datastorage/server/audit_handlers.go`)

**Structure**:
```go
package server

import (
    "context"
    "encoding/json"
    "net/http"
    "time"
    
    "github.com/jordigilh/kubernaut/pkg/datastorage/models"
    "github.com/jordigilh/kubernaut/pkg/datastorage/repository"
    "github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
    "github.com/jordigilh/kubernaut/pkg/datastorage/validation"
    "go.uber.org/zap"
)

// handleCreateNotificationAudit handles POST /api/v1/audit/notifications
// BR-STORAGE-001 to BR-STORAGE-020: Audit write API
// DD-009: DLQ fallback on database errors
func (s *Server) handleCreateNotificationAudit(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    
    // 1. Parse request body
    var audit models.NotificationAudit
    if err := json.NewDecoder(r.Body).Decode(&audit); err != nil {
        writeRFC7807Error(w, validation.NewValidationErrorProblem(
            "notification_audit",
            map[string]string{"body": "invalid JSON"},
        ))
        return
    }
    
    // 2. Validate input
    if err := s.validator.Validate(&audit); err != nil {
        if valErr, ok := err.(*validation.ValidationError); ok {
            writeRFC7807Error(w, valErr.ToRFC7807())
            return
        }
        writeRFC7807Error(w, validation.NewInternalErrorProblem(err.Error()))
        return
    }
    
    // 3. Attempt database write
    created, err := s.repository.Create(ctx, &audit)
    if err != nil {
        // Check if it's a known error type (RFC 7807)
        if rfc7807Err, ok := err.(*validation.RFC7807Problem); ok {
            writeRFC7807Error(w, rfc7807Err)
            return
        }
        
        // DD-009: Unknown error → DLQ fallback
        s.logger.Error("Database write failed, using DLQ fallback",
            zap.Error(err),
            zap.String("notification_id", audit.NotificationID))
        
        if dlqErr := s.dlqClient.EnqueueNotificationAudit(ctx, &audit, err); dlqErr != nil {
            s.logger.Error("DLQ fallback also failed", zap.Error(dlqErr))
            writeRFC7807Error(w, validation.NewServiceUnavailableProblem(
                "database and DLQ both unavailable"))
            return
        }
        
        // DLQ success - return 202 Accepted
        w.WriteHeader(http.StatusAccepted)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "accepted",
            "message": "audit record queued for processing",
        })
        return
    }
    
    // 4. Success - return 201 Created
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(created)
}

// writeRFC7807Error writes an RFC 7807 error response
func writeRFC7807Error(w http.ResponseWriter, problem *validation.RFC7807Problem) {
    w.Header().Set("Content-Type", "application/problem+json")
    w.WriteHeader(problem.Status)
    json.NewEncoder(w).Encode(problem)
}
```

#### **File 2: Update Server to Add Dependencies** (`pkg/datastorage/server/server.go`)

**Changes**:
1. Add `repository`, `dlqClient`, `validator` fields to `Server` struct
2. Update `NewServer()` to accept Redis address
3. Instantiate repository, DLQ client, validator
4. Add audit route to router

**Code Changes**:
```go
// Server struct - ADD FIELDS
type Server struct {
    handler    *Handler
    db         *sql.DB
    logger     *zap.Logger
    httpServer *http.Server
    isShuttingDown atomic.Bool
    
    // NEW: Audit write dependencies
    repository *repository.NotificationAuditRepository
    dlqClient  *dlq.Client
    validator  *validation.NotificationAuditValidator
}

// NewServer - UPDATE SIGNATURE
func NewServer(
    dbConnStr string,
    redisAddr string,  // NEW parameter
    logger *zap.Logger,
    cfg *Config,
) (*Server, error) {
    // ... existing DB connection ...
    
    // NEW: Create Redis client for DLQ
    redisClient := redis.NewClient(&redis.Options{
        Addr: redisAddr,
    })
    if err := redisClient.Ping(context.Background()).Err(); err != nil {
        _ = db.Close()
        return nil, fmt.Errorf("failed to connect to Redis: %w", err)
    }
    
    // NEW: Create dependencies
    repo := repository.NewNotificationAuditRepository(db, logger)
    dlqClient := dlq.NewClient(redisClient, logger)
    validator := validation.NewNotificationAuditValidator()
    
    return &Server{
        handler:    handler,
        db:         db,
        logger:     logger,
        httpServer: &http.Server{...},
        repository: repo,
        dlqClient:  dlqClient,
        validator:  validator,
    }, nil
}

// Handler() - ADD AUDIT ROUTE
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()
    
    // ... existing middleware and health endpoints ...
    
    // API v1 routes
    r.Route("/api/v1", func(r chi.Router) {
        // READ API (existing)
        r.Get("/incidents", s.handler.ListIncidents)
        r.Get("/incidents/{id}", s.handler.GetIncident)
        
        // WRITE API (NEW)
        r.Post("/audit/notifications", s.handleCreateNotificationAudit)
    })
    
    return r
}
```

#### **File 3: Update Main to Pass Redis Address** (`cmd/datastorage/main.go`)

**Changes**:
```go
// Add Redis address flag
var (
    addr       = flag.String("addr", getEnv("HTTP_PORT", ":8080"), "HTTP server address")
    dbHost     = flag.String("db-host", getEnv("DB_HOST", "localhost"), "PostgreSQL host")
    dbPort     = flag.Int("db-port", 5432, "PostgreSQL port")
    dbName     = flag.String("db-name", getEnv("DB_NAME", "action_history"), "PostgreSQL database name")
    dbUser     = flag.String("db-user", getEnv("DB_USER", "db_user"), "PostgreSQL user")
    dbPassword = flag.String("db-password", getEnv("DB_PASSWORD", ""), "PostgreSQL password")
    redisAddr  = flag.String("redis-addr", getEnv("REDIS_ADDR", "localhost:6379"), "Redis address")  // NEW
)

// Update server creation
srv, err := server.NewServer(dbConnStr, *redisAddr, logger, serverCfg)
```

---

### **Phase 3: CHECK - Validate All Tests Pass** (0.5h)

#### **Validation Steps**:
1. Build Data Storage Service image
2. Run integration tests
3. Verify all 4 HTTP API tests pass
4. Verify existing repository/DLQ tests still pass
5. Check service logs for errors

#### **Success Criteria**:
- ✅ All 4 HTTP API integration tests pass
- ✅ Existing 14 repository/DLQ tests still pass
- ✅ Service container starts successfully
- ✅ Health endpoint responsive
- ✅ No errors in service logs

---

## 📊 **File Changes Summary**

| File | Type | Lines | Complexity |
|------|------|-------|------------|
| `test/integration/datastorage/suite_test.go` | Modify | +50 | Medium |
| `test/integration/datastorage/http_api_test.go` | New | +150 | Medium |
| `pkg/datastorage/server/audit_handlers.go` | New | +80 | Medium |
| `pkg/datastorage/server/server.go` | Modify | +30 | Low |
| `cmd/datastorage/main.go` | Modify | +5 | Low |
| **TOTAL** | 5 files | **+315 lines** | **Medium** |

---

## ⏱️ **Timeline**

| Phase | Duration | Confidence |
|-------|----------|------------|
| DO-RED: HTTP integration tests | 1.5h | 95% |
| DO-GREEN: Audit handlers | 1h | 95% |
| DO-GREEN: Server updates | 0.5h | 100% |
| DO-GREEN: Main updates | 0.25h | 100% |
| CHECK: Validation | 0.5h | 90% |
| **TOTAL** | **3.75h** | **95%** |

**Note**: Slightly under original 4.5h estimate (good news!)

---

## 🎯 **Success Criteria**

### **Implementation Complete When**:
1. ✅ POST `/api/v1/audit/notifications` endpoint exists
2. ✅ Request validation working (RFC 7807 errors)
3. ✅ Data persisted to PostgreSQL (verified in tests)
4. ✅ DLQ fallback working when DB down (verified in tests)
5. ✅ HTTP integration tests passing (4 scenarios)
6. ✅ Service container builds and starts
7. ✅ Health endpoint responsive
8. ✅ Existing tests still passing (14 repository/DLQ tests)

---

## 🔗 **Dependencies**

### **External Dependencies**:
- PostgreSQL container (already running ✅)
- Redis container (already running ✅)
- Podman (already available ✅)

### **Code Dependencies**:
- `pkg/datastorage/repository/notification_audit_repository.go` ✅
- `pkg/datastorage/dlq/client.go` ✅
- `pkg/datastorage/validation/notification_audit_validator.go` ✅
- `pkg/datastorage/models/notification_audit.go` ✅
- `github.com/redis/go-redis/v9` ✅ (already in use)
- `github.com/go-chi/chi/v5` ✅ (already in use)

---

## 📋 **Risk Assessment**

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Redis connection issues | Low (10%) | Medium | Test Redis connectivity in NewServer() |
| Service container build fails | Low (5%) | High | Use existing Dockerfile (ADR-027 compliant) |
| Integration tests flaky | Medium (20%) | Medium | Use Eventually() for service readiness |
| DLQ fallback not working | Low (10%) | High | Test with real PostgreSQL stop/start |

**Overall Risk**: **Low** (95% confidence)

---

## ✅ **PLAN Complete**

**Confidence**: **95%**

**Rationale**:
1. ✅ Clear implementation strategy
2. ✅ All dependencies exist
3. ✅ Server infrastructure ready
4. ✅ Test infrastructure working
5. ⚠️ Minor risk: Redis connection propagation (5%)

**Next**: DO-RED Phase - Write HTTP API integration tests FIRST

---

## 🔗 **References**

- **Analysis**: `DAY7-CORRECTIVE-ANALYSIS.md`
- **Plan**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.8.md`
- **DD-007**: Graceful Shutdown
- **DD-009**: Audit Write Error Recovery (DLQ)
- **ADR-027**: Multi-Architecture Container Build Strategy
- **ADR-032**: Data Access Layer Isolation

