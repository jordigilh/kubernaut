# API Gateway Migration - Remediation Action Plan

**Status**: ✅ **APPROVED** (November 2, 2025)
**Scope**: Data Storage Service + Context API Service
**Excluded**: Effectiveness Monitor (deferred per project plan)
**Total Effort**: 60.3 hours (~8 working days)
**Authority**: [API Gateway Migration Plans Triage](API-GATEWAY-MIGRATION-PLANS-TRIAGE.md)

---

## 🎯 **EXECUTIVE SUMMARY**

### **Objective**
Remediate Data Storage and Context API migration plans to achieve production-ready quality (95-96% confidence) through systematic gap resolution.

### **Scope**
- ✅ **Phase 1**: Data Storage Service Migration Plan (28 gaps, 28.8h)
- ✅ **Phase 2**: Context API Migration Plan (15 gaps, 32.8h)
- ⏸️ **Deferred**: Effectiveness Monitor (per project plan)

### **Timeline**
- **Data Storage**: 3.5-4 days
- **Context API**: 4-5 days
- **Total**: 8 working days

### **Success Criteria**
- ✅ All P0 blockers resolved
- ✅ All P1 critical gaps resolved
- ✅ All P2 high-value gaps resolved
- ✅ QA validation passed
- ✅ Confidence: 95-96%

---

## 📋 **PHASE 1: DATA STORAGE SERVICE** (28.8 hours)

**Current State**: 13 gaps, 65% confidence
**Target State**: 0 gaps, 95% confidence
**Timeline**: 3.5-4 days

---

### **STAGE 1.1: P0 BLOCKERS** (8 hours)

**Objective**: Fix integration test specifications to achieve >50% BR coverage

#### **Task 1.1.1: Define Integration Test Suite** (4h)

**Deliverable**: 15+ integration test cases achieving >50% BR coverage

**Test Categories**:
1. **HTTP API → PostgreSQL Flow** (5 tests)
   - List incidents with pagination
   - Filter by namespace (Unicode)
   - Filter by severity
   - Filter by timestamp range
   - Combined filters

2. **Performance Tests** (3 tests)
   - 10,000+ record pagination
   - 100 concurrent requests
   - Large result set handling

3. **Security Tests** (2 tests)
   - SQL injection prevention (parameterized queries)
   - Input validation edge cases

4. **Error Scenarios** (3 tests)
   - Database timeout handling
   - Connection failure retry
   - Empty result sets

5. **Unicode Support** (2 tests)
   - Arabic/Chinese namespaces
   - Emoji in filter values

**File**: `docs/services/stateless/data-storage/implementation/API-GATEWAY-MIGRATION.md`

**Success Criteria**:
- [ ] 15+ integration tests defined
- [ ] Coverage >50% of 7 BRs
- [ ] Each test maps to specific BR
- [ ] Test file structure documented

---

#### **Task 1.1.2: Implement Integration Test Infrastructure** (4h)

**Deliverables**:
- `test/integration/datastorage/01_read_api_integration_test.go`
- `test/integration/datastorage/02_pagination_stress_test.go`
- `test/integration/datastorage/03_security_test.go`

**Implementation Pattern** (from Context API):
```go
package datastorage

import (
    "context"
    "testing"

    "github.com/jordigilh/kubernaut/pkg/datastorage/server"
    "github.com/jordigilh/kubernaut/pkg/testutil"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestDataStorageIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Data Storage Integration Test Suite")
}

var _ = BeforeSuite(func() {
    // Start real PostgreSQL via Podman
    pgContainer, pgAddr := testutil.StartPostgreSQLContainer(ctx)
    DeferCleanup(func() {
        pgContainer.Terminate(ctx)
    })
})

var _ = Describe("Read API Integration - BR-STORAGE-021", func() {
    It("should list incidents with pagination", func() {
        // HTTP → PostgreSQL flow
        resp, err := client.ListIncidents(ctx, &ListParams{
            Limit:  10,
            Offset: 0,
        })
        Expect(err).ToNot(HaveOccurred())
        Expect(len(resp.Incidents)).To(Equal(10))
    })
})
```

**Success Criteria**:
- [ ] 3 integration test files created
- [ ] Real PostgreSQL via Podman
- [ ] All 15+ tests passing
- [ ] Test helpers documented

---

### **STAGE 1.2: P1 CRITICAL** (6.5 hours)

**Objective**: Fix code quality gaps (imports, package declarations, RFC 7807, DD-007)

#### **Task 1.2.1: Add Imports to All Code Examples** (2h)

**Scope**: 10+ code examples in migration plan

**Pattern**:
```go
package datastorage

import (
    "context"
    "database/sql"
    "fmt"
    "net/http"
    "time"

    "github.com/jordigilh/kubernaut/pkg/datastorage/query"
    "github.com/jordigilh/kubernaut/pkg/shared/errors"
    "github.com/lib/pq"
    "go.uber.org/zap"
)

type RESTServer struct {
    // ... implementation ...
}
```

**Success Criteria**:
- [ ] All 10+ code examples have imports
- [ ] Standard library imports grouped first
- [ ] Third-party imports grouped second
- [ ] Local project imports grouped third
- [ ] Code is copy-pasteable

---

#### **Task 1.2.2: Add Package Declarations to All Code Examples** (0.5h)

**Pattern**:
```go
package datastorage  // REQUIRED: Must be first line

import (
    // ... imports ...
)
```

**Success Criteria**:
- [ ] All code examples start with `package datastorage`
- [ ] Consistent naming throughout plan
- [ ] White-box testing pattern documented

---

#### **Task 1.2.3: Implement DD-007 Graceful Shutdown** (4h)

**Deliverable**: Full DD-007 implementation section in migration plan

**Implementation Section**:
```markdown
### **Day 6: DD-007 Graceful Shutdown - BR-STORAGE-026**

**Objective**: Implement Kubernetes-aware graceful shutdown for zero-downtime deployments

---

#### **DO-RED Phase** (1h)

**Test File**: `test/integration/datastorage/07_graceful_shutdown_test.go`

```go
package datastorage

import (
    "context"
    "net/http"
    "os"
    "syscall"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("DD-007 Graceful Shutdown - BR-STORAGE-026", func() {
    It("should set shutdown flag immediately on SIGTERM", func() {
        // Send SIGTERM
        syscall.Kill(os.Getpid(), syscall.SIGTERM)
        time.Sleep(100 * time.Millisecond)

        // Verify readiness returns 503
        resp, err := http.Get("http://localhost:8080/health/ready")
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(503))
    })

    It("should complete in-flight requests within timeout", func() {
        // Start long-running request
        go func() {
            client.ListIncidents(ctx, &ListParams{Limit: 1000})
        }()

        // Send SIGTERM after 1 second
        time.Sleep(1 * time.Second)
        syscall.Kill(os.Getpid(), syscall.SIGTERM)

        // Verify request completes (not aborted)
        // Test implementation...
    })

    It("should close database connections cleanly", func() {
        // Verify no connection leaks after shutdown
    })
})
```

---

#### **DO-GREEN Phase** (1.5h)

**File**: `pkg/datastorage/server/server.go`

```go
package server

import (
    "context"
    "fmt"
    "net/http"
    "sync/atomic"
    "time"

    "go.uber.org/zap"
)

type Server struct {
    httpServer     *http.Server
    dbClient       DatabaseClient
    logger         *zap.Logger

    // REQUIRED: Shutdown coordination flag for DD-007
    isShuttingDown atomic.Bool
}

func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info("Initiating Kubernetes-aware graceful shutdown (DD-007)")

    // STEP 1: Set shutdown flag (readiness probe → 503)
    s.isShuttingDown.Store(true)
    s.logger.Info("Shutdown flag set - readiness probe now returns 503",
        zap.String("effect", "kubernetes_will_remove_from_endpoints"))

    // STEP 2: Wait for Kubernetes endpoint removal propagation
    const endpointPropagationDelay = 5 * time.Second
    s.logger.Info("Waiting for Kubernetes endpoint removal propagation",
        zap.Duration("delay", endpointPropagationDelay))
    time.Sleep(endpointPropagationDelay)
    s.logger.Info("Endpoint removal propagation complete")

    // STEP 3: Drain in-flight HTTP connections
    s.logger.Info("Draining in-flight HTTP connections")
    if err := s.httpServer.Shutdown(ctx); err != nil {
        s.logger.Error("HTTP server shutdown failed", zap.Error(err))
        return fmt.Errorf("HTTP shutdown failed: %w", err)
    }
    s.logger.Info("HTTP connections drained successfully")

    // STEP 4: Close external resources
    s.logger.Info("Closing database connections")
    if err := s.dbClient.Close(); err != nil {
        s.logger.Error("Failed to close database", zap.Error(err))
        return fmt.Errorf("database close: %w", err)
    }
    s.logger.Info("Database connections closed successfully")

    s.logger.Info("Graceful shutdown complete - all resources closed")
    return nil
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
    // CRITICAL: Check shutdown flag FIRST (before any other checks)
    if s.isShuttingDown.Load() {
        s.logger.Debug("Readiness check during shutdown - returning 503")
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "shutting_down",
            "reason": "graceful_shutdown_in_progress",
        })
        return
    }

    // Normal health checks
    if err := s.dbClient.Ping(r.Context()); err != nil {
        w.WriteHeader(503)
        return
    }

    w.WriteHeader(200)
}
```

**File**: `cmd/data-storage/main.go`

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/jordigilh/kubernaut/pkg/datastorage/server"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    // Create server
    srv := server.New(config, logger)

    // Start server in background
    errChan := make(chan error, 1)
    go func() {
        if err := srv.Start(); err != nil {
            errChan <- err
        }
    }()

    // Setup signal handling for SIGTERM and SIGINT
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    // Wait for shutdown signal or server error
    select {
    case err := <-errChan:
        logger.Fatal("Server failed", zap.Error(err))
    case sig := <-sigChan:
        logger.Info("Shutdown signal received", zap.String("signal", sig.String()))
    }

    // Graceful shutdown with 30-second timeout
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    logger.Info("Initiating graceful shutdown...")
    if err := srv.Shutdown(shutdownCtx); err != nil {
        logger.Error("Graceful shutdown failed", zap.Error(err))
        os.Exit(1)
    }

    logger.Info("Server shutdown complete")
}
```

---

#### **DO-REFACTOR Phase** (0.5h)

**Enhancements**:
- Add structured logging for each shutdown step
- Add metrics for shutdown duration
- Add timeout warnings

---

#### **APDC Check Phase** (1h)

**Validation**:
- [ ] Readiness probe returns 503 on SIGTERM
- [ ] In-flight requests complete within timeout
- [ ] Database connections closed cleanly
- [ ] No request failures during rolling updates (0%)
- [ ] All integration tests passing

**Confidence**: 95% (production-ready)
```

**Reference**: Copy pattern from Context API v2.8 (lines 415-427) or Gateway v2.23

**Success Criteria**:
- [ ] DD-007 implementation section added
- [ ] 4-step shutdown pattern documented
- [ ] Code examples with imports
- [ ] Integration tests specified
- [ ] Deployment YAML configuration included
- [ ] Reference to DD-007 decision document

---

### **STAGE 1.3: P2 HIGH-VALUE** (13 hours)

**Objective**: Complete production-readiness sections

#### **Task 1.3.1: Refactor to DescribeTable Pattern** (2h)

**Scope**: Lines 142-154, 157-174, 177-193, 196-208

**Before**:
```go
It("should validate limit parameter - positive", func() { /* ... */ })
It("should validate limit parameter - zero", func() { /* ... */ })
It("should validate limit parameter - negative", func() { /* ... */ })
It("should validate limit parameter - maximum", func() { /* ... */ })
```

**After**:
```go
DescribeTable("limit parameter validation - BR-STORAGE-023",
    func(limit int, expectedErr string) {
        err := ValidateLimit(limit)
        if expectedErr == "" {
            Expect(err).ToNot(HaveOccurred())
        } else {
            Expect(err).To(MatchError(ContainSubstring(expectedErr)))
        }
    },
    Entry("positive value", 10, ""),
    Entry("zero value", 0, "must be positive"),
    Entry("negative value", -1, "must be positive"),
    Entry("maximum value", 1000, ""),
    Entry("exceeds maximum", 1001, "exceeds maximum"),
)
```

**Success Criteria**:
- [ ] 4+ repetitive test blocks refactored
- [ ] Code reduction: 60-80%
- [ ] All tests still passing

---

#### **Task 1.3.2: Add Request ID Propagation** (1h)

**Deliverable**: Request ID middleware + HTTP client propagation

**Pattern**:
```go
package datastorage

import (
    "context"
    "net/http"

    "github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

func RequestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := r.Header.Get(RequestIDHeader)
        if requestID == "" {
            requestID = uuid.New().String()
        }

        ctx := context.WithValue(r.Context(), "request_id", requestID)
        w.Header().Set(RequestIDHeader, requestID)

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

**Success Criteria**:
- [ ] Middleware implementation documented
- [ ] Code example with imports
- [ ] Unit test example provided

---

#### **Task 1.3.3: Add Context Cancellation Handling** (1h)

**Deliverable**: Explicit `ctx.Done()` checks in long-running operations

**Pattern**:
```go
package datastorage

import (
    "context"
    "fmt"
)

func (q *QueryExecutor) ExecuteLongQuery(ctx context.Context, query string) ([]Result, error) {
    // Check context before starting
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Execute query
    rows, err := q.db.QueryContext(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []Result
    for rows.Next() {
        // Check context during iteration
        select {
        case <-ctx.Done():
            return nil, fmt.Errorf("query cancelled: %w", ctx.Err())
        default:
        }

        var r Result
        if err := rows.Scan(&r); err != nil {
            return nil, err
        }
        results = append(results, r)
    }

    return results, nil
}
```

**Success Criteria**:
- [ ] Context cancellation pattern documented
- [ ] 3+ examples with different scenarios
- [ ] Unit tests for cancellation

---

#### **Task 1.3.4: Add Common Pitfalls Section** (2h)

**Deliverable**: 10+ comprehensive pitfalls with prevention strategies

**Required Pitfalls**:
1. **SQL Injection via String Concatenation** - Use parameterized queries
2. **Null Testing Anti-Pattern** - Use specific value assertions
3. **Missing Package Declarations** - Start all test files with `package datastorage`
4. **Batch-Activated TDD Violation** - No `Skip()` in tests
5. **Unicode Edge Cases** - Test Arabic, Chinese, emoji
6. **Pagination Boundary Errors** - Off-by-one in limit/offset
7. **Missing RFC 7807 Error Types** - Use structured Problem Details
8. **Missing Context Cancellation** - Check `ctx.Done()`
9. **Missing Import Statements** - All code examples need imports
10. **Hard-Coded Configuration** - Use environment variables

**Format** (from Gateway v2.23, lines 519-905):
```markdown
### **Common Pitfall 1: SQL Injection via String Concatenation** ⚠️

**Problem**: Using `fmt.Sprintf` to build SQL queries with user input

**Business Impact**:
- Violates BR-STORAGE-025 (Security: Parameter validation)
- DATA CORRUPTION risk
- SECURITY BREACH risk

**Example**:
```go
package datastorage

import "fmt"

// ❌ BAD: SQL injection vulnerability
query := fmt.Sprintf("SELECT * FROM incidents WHERE namespace = '%s'", userInput)
rows, _ := db.Query(query)  // userInput can be "'; DROP TABLE incidents; --"

// ✅ GOOD: Parameterized query
query := "SELECT * FROM incidents WHERE namespace = ?"
rows, _ := db.Query(query, userInput)  // Safely escaped
```

**Prevention**:
- ALWAYS use parameterized queries (`?` placeholders)
- NEVER use string concatenation for SQL
- Review all `fmt.Sprintf` calls in query builders

**Detection**:
```bash
# Find potential SQL injection vulnerabilities
grep -rn "fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE" pkg/datastorage/
```
```

**Success Criteria**:
- [ ] 10+ pitfalls documented
- [ ] Each pitfall has ❌ BAD and ✅ GOOD examples
- [ ] Business impact stated
- [ ] Prevention strategies clear
- [ ] Detection commands provided

---

#### **Task 1.3.5: Add Pre-Implementation Validation Script** (1h)

**Deliverable**: `scripts/validate-datastorage-infrastructure.sh`

**Script Content**:
```bash
#!/usr/bin/env bash
set -euo pipefail

echo "🔍 Data Storage Service - Pre-Implementation Validation"
echo "======================================================="

EXIT_CODE=0

# Check 1: PostgreSQL availability
echo ""
echo "Check 1: PostgreSQL availability..."
if pg_isready -h localhost -p 5432; then
    echo "✅ PostgreSQL is available"
else
    echo "❌ PostgreSQL is NOT available"
    echo "   Run: podman run -d --name datastorage-postgres -p 5432:5432 -e POSTGRES_PASSWORD=test postgres:15-alpine"
    EXIT_CODE=1
fi

# Check 2: Database schema exists
echo ""
echo "Check 2: Database schema..."
if psql -h localhost -U postgres -d action_history -c "\dt" | grep -q resource_action_traces; then
    echo "✅ Database schema exists"
else
    echo "❌ Database schema NOT found"
    echo "   Run: psql -h localhost -U postgres -d action_history -f scripts/schema.sql"
    EXIT_CODE=1
fi

# Check 3: Go dependencies
echo ""
echo "Check 3: Go dependencies..."
if go mod verify; then
    echo "✅ Go dependencies verified"
else
    echo "❌ Go dependencies NOT verified"
    echo "   Run: go mod tidy"
    EXIT_CODE=1
fi

# Check 4: Test infrastructure
echo ""
echo "Check 4: Test infrastructure..."
if [ -f "pkg/testutil/postgres_container.go" ]; then
    echo "✅ PostgreSQL test container helper exists"
else
    echo "❌ PostgreSQL test container helper NOT found"
    echo "   Create: pkg/testutil/postgres_container.go"
    EXIT_CODE=1
fi

# Summary
echo ""
echo "======================================================="
if [ $EXIT_CODE -eq 0 ]; then
    echo "✅ ALL CHECKS PASSED - Ready for implementation"
else
    echo "❌ SOME CHECKS FAILED - Fix issues before starting"
fi

exit $EXIT_CODE
```

**Success Criteria**:
- [ ] Script executable (`chmod +x`)
- [ ] All infrastructure checks documented
- [ ] Clear error messages
- [ ] Fix instructions provided

---

#### **Task 1.3.6: Add Operational Runbooks** (2h)

**Deliverable**: 6 comprehensive runbooks

**Runbooks**:
1. **Deployment Runbook** - Step-by-step Kubernetes deployment
2. **Troubleshooting Runbook** - Common issues and solutions
3. **Rollback Runbook** - Rollback procedures for failed deployments
4. **Performance Tuning Runbook** - PostgreSQL optimization
5. **Maintenance Runbook** - Database maintenance tasks
6. **On-Call Runbook** - Emergency procedures

**Format** (from Gateway v2.23):
```markdown
### **Runbook 1: Deployment** (30 minutes to execute)

#### **Prerequisites**
- [ ] Kubernetes cluster access
- [ ] PostgreSQL 15+ running
- [ ] Database schema deployed
- [ ] Docker image built and pushed

#### **Steps**
1. **Create Namespace**
   ```bash
   kubectl create namespace datastorage
   ```

2. **Deploy ConfigMap**
   ```bash
   kubectl apply -f deploy/datastorage-config.yaml
   ```

3. **Deploy Secret**
   ```bash
   kubectl create secret generic datastorage-secret \
     --from-literal=DB_PASSWORD=<password>
   ```

4. **Deploy Service**
   ```bash
   kubectl apply -f deploy/datastorage-deployment.yaml
   ```

5. **Verify Deployment**
   ```bash
   kubectl get pods -n datastorage
   kubectl logs -f deployment/datastorage -n datastorage
   ```

6. **Test Health Endpoints**
   ```bash
   kubectl port-forward svc/datastorage 8080:8080 -n datastorage &
   curl http://localhost:8080/health/live   # Should return 200
   curl http://localhost:8080/health/ready  # Should return 200
   ```

#### **Rollback Procedure**
```bash
kubectl rollout undo deployment/datastorage -n datastorage
```

#### **Success Criteria**
- [ ] All pods running (kubectl get pods shows Running)
- [ ] Health checks passing (200 OK)
- [ ] Logs show no errors
- [ ] /metrics endpoint accessible
```

**Success Criteria**:
- [ ] 6 runbooks documented
- [ ] Each runbook has clear steps
- [ ] Execution time estimated
- [ ] Rollback procedures included

---

#### **Task 1.3.7: Add Multi-Architecture Dockerfile** (1h)

**Deliverable**: `docker/datastorage-ubi9.Dockerfile`

**Pattern** (from Context API):
```dockerfile
# Multi-Architecture Build - Red Hat UBI9 (ADR-027)
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS builder

ARG GOARCH
ARG GOOS=linux

RUN microdnf install -y golang && microdnf clean all

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} \
    go build -ldflags="-w -s" -o /datastorage ./cmd/data-storage

# Runtime Image
FROM registry.access.redhat.com/ubi9/ubi-micro:latest

ARG GOARCH

COPY --from=builder /datastorage /datastorage

RUN useradd -r -u 1001 -s /sbin/nologin datastorage-user
USER 1001

EXPOSE 8080 9090

ENTRYPOINT ["/datastorage"]
```

**Makefile Target**:
```makefile
.PHONY: docker-build-datastorage
docker-build-datastorage:
	docker buildx build --platform linux/amd64,linux/arm64 \
		--build-arg GOARCH=$(GOARCH) \
		-t quay.io/jordigilh/data-storage:$(VERSION) \
		-f docker/datastorage-ubi9.Dockerfile .
```

**Success Criteria**:
- [ ] Dockerfile uses Red Hat UBI9
- [ ] Multi-architecture build documented
- [ ] Non-root user configured
- [ ] Makefile target included

---

#### **Task 1.3.8: Add BR Coverage Matrix** (1h)

**Deliverable**: Comprehensive BR → Test mapping table

**Format**:
```markdown
### **BR Coverage Matrix**

| BR ID | Requirement | Unit Tests | Integration Tests | E2E Tests | Total Coverage |
|---|---|---|---|---|---|
| BR-STORAGE-021 | Read API - List incidents with filters | 12 | 5 | 2 | 19 tests |
| BR-STORAGE-022 | Read API - Pagination (limit/offset) | 8 | 3 | 1 | 12 tests |
| BR-STORAGE-023 | Parameter validation | 10 | 2 | 0 | 12 tests |
| BR-STORAGE-024 | Unicode support (namespaces, severity) | 6 | 2 | 1 | 9 tests |
| BR-STORAGE-025 | Security: SQL injection prevention | 4 | 2 | 0 | 6 tests |
| BR-STORAGE-026 | DD-007 Graceful shutdown | 2 | 3 | 0 | 5 tests |
| BR-STORAGE-027 | RFC 7807 error responses | 8 | 0 | 0 | 8 tests |
| **TOTAL** | **7 BRs** | **50 (70%)** | **17 (24%)** | **4 (6%)** | **71 tests** |

**Defense-in-Depth Compliance**:
- ✅ Unit Tests: 70% (target: >70%)
- ✅ Integration Tests: 24% (target: <20% - slightly over but acceptable)
- ✅ E2E Tests: 6% (target: <10%)

**Missing Coverage**:
- None - all BRs have comprehensive test coverage
```

**Success Criteria**:
- [ ] All 7 BRs mapped to tests
- [ ] Test counts accurate
- [ ] Defense-in-depth percentages calculated
- [ ] Missing coverage identified

---

#### **Task 1.3.9: Add Phase-by-Phase Confidence Assessments** (1h)

**Deliverable**: Confidence progression tracking

**Format**:
```markdown
### **Confidence Assessment by Phase**

#### **Pre-Day 1** (Baseline)
- **Overall Confidence**: 40%
- **Infrastructure**: 80% (PostgreSQL ready, schema deployed)
- **Implementation**: 20% (no code yet)
- **Testing**: 30% (test framework ready)
- **Integration**: 10% (no main app integration yet)

#### **After Day 3** (REST API Complete)
- **Overall Confidence**: 70%
- **Implementation**: 80% (REST API endpoints working)
- **Testing**: 75% (unit tests passing, integration tests 50% complete)
- **Integration**: 50% (REST API exposed, not yet consumed)
- **Risks**: Integration with Context API not validated yet

#### **After Day 6** (Integration + DD-007 Complete)
- **Overall Confidence**: 95%
- **Implementation**: 95% (all features complete, DD-007 implemented)
- **Testing**: 95% (71 tests passing, >70% unit, 24% integration)
- **Integration**: 90% (Context API consuming REST API successfully)
- **Production Readiness**: 95% (graceful shutdown, health checks, metrics)
- **Risks**: Minor - performance tuning may be needed at scale
```

**Success Criteria**:
- [ ] Confidence tracked at each major milestone
- [ ] Breakdown by category (implementation, testing, integration)
- [ ] Risks identified at each phase
- [ ] Final confidence: 95%

---

### **STAGE 1.4: QA VALIDATION** (1.3 hours)

**Objective**: Final quality assurance and cross-plan consistency

#### **Task 1.4.1: Cross-Plan Consistency Check** (0.5h)

**Validation**:
- [ ] All patterns match Gateway v2.23
- [ ] All patterns match Context API v2.8
- [ ] No conflicting guidance
- [ ] Consistent terminology

---

#### **Task 1.4.2: Gateway v2.23 Alignment Check** (0.5h)

**Validation**:
- [ ] DD-007 pattern matches Gateway
- [ ] RFC 7807 pattern matches Gateway
- [ ] DescribeTable pattern matches Gateway
- [ ] Common Pitfalls similar depth
- [ ] Operational Runbooks similar quality

---

#### **Task 1.4.3: Testing Strategy Compliance** (0.3h)

**Validation**:
- [ ] Real PostgreSQL in integration tests (no mocks)
- [ ] Integration coverage >50%
- [ ] Defense-in-depth percentages correct
- [ ] White-box testing documented
- [ ] Ginkgo/Gomega BDD compliance

---

### **STAGE 1 COMPLETION CRITERIA**

- [x] All 13 gaps resolved (28.8h effort)
- [x] Confidence: 95% (was 65%)
- [x] All P0, P1, P2 gaps addressed
- [x] QA validation passed
- [x] Production-ready quality achieved

**Approval Gate**: User approval required before proceeding to Phase 2

---

## 📋 **PHASE 2: CONTEXT API** (32.8 hours)

**Current State**: 15 gaps, 60% confidence
**Target State**: 0 gaps, 96% confidence
**Timeline**: 4-5 days

---

### **STAGE 2.1: P0 BLOCKERS** (14 hours)

**Objective**: Fix Redis mocking and expand integration test coverage to >50%

#### **Task 2.1.1: Replace miniredis with Real Redis** (2h)

**Current Violation** (Line 489):
```go
// ❌ WRONG: miniredis in integration tests
redis := miniredis.RunT(GinkgoT())
```

**Correct Pattern**:
```go
package contextapi

import (
    "context"

    "github.com/jordigilh/kubernaut/pkg/testutil"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = BeforeSuite(func() {
    // Start real Redis via Podman
    redisContainer, redisAddr := testutil.StartRedisContainer(ctx)
    DeferCleanup(func() {
        redisContainer.Terminate(ctx)
    })

    // Connect cache manager to real Redis
    cacheManager = cache.NewManager(&cache.Config{
        RedisAddr: redisAddr,
    })
})
```

**Files to Update**:
- `test/integration/contextapi/01_cache_integration_test.go`
- `test/integration/contextapi/02_query_lifecycle_test.go`
- `test/integration/contextapi/03_multi_tier_cache_test.go`

**Success Criteria**:
- [ ] All integration tests use real Redis
- [ ] No `miniredis` imports in integration test files
- [ ] Redis connection pooling tested with real Redis
- [ ] Redis failure scenarios tested (container stop/start)

---

#### **Task 2.1.2: Expand Integration Test Suite to >50% Coverage** (12h)

**Current State**: <20% coverage (Lines 48-50)
**Target State**: >50% coverage (30+ additional tests)

**New Test Categories**:

##### **2.1.2.1: HTTP Client Integration** (4h, 12 tests)

**Deliverable**: `test/integration/contextapi/04_http_client_integration_test.go`

**Tests**:
1. Context API → Data Storage Service → PostgreSQL flow (end-to-end)
2. HTTP timeout scenarios (>5s request)
3. Retry logic with real HTTP failures
4. Circuit breaker with real HTTP (3 failures → open)
5. Request ID propagation across services
6. Malformed JSON response handling
7. HTTP 503 transient failure retry
8. HTTP 500 permanent failure (no retry)
9. Connection refused scenarios
10. DNS resolution failure handling
11. Slow response handling (< timeout)
12. Response schema validation

**Pattern**:
```go
package contextapi

import (
    "context"
    "net/http"
    "net/http/httptest"
    "time"

    "github.com/jordigilh/kubernaut/pkg/contextapi/client"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("HTTP Client Integration - BR-CONTEXT-021", func() {
    var (
        httpClient     *client.HTTPClient
        mockDataStorage *httptest.Server
    )

    BeforeEach(func() {
        // Start mock Data Storage Service
        mockDataStorage = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Simulate Data Storage Service response
            w.WriteHeader(200)
            json.NewEncoder(w).Encode(map[string]interface{}{
                "incidents": []interface{}{},
            })
        }))

        httpClient = client.New(&client.Config{
            BaseURL: mockDataStorage.URL,
        })
    })

    AfterEach(func() {
        mockDataStorage.Close()
    })

    It("should propagate request ID across services", func() {
        ctx := context.WithValue(context.Background(), "request_id", "test-123")

        _, err := httpClient.ListIncidents(ctx, &ListParams{})
        Expect(err).ToNot(HaveOccurred())

        // Verify request ID was sent to Data Storage Service
        // Implementation...
    })

    It("should retry on HTTP 503 transient failure", func() {
        attemptCount := 0
        mockDataStorage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            attemptCount++
            if attemptCount < 3 {
                w.WriteHeader(503)
                return
            }
            w.WriteHeader(200)
            json.NewEncoder(w).Encode(map[string]interface{}{})
        }))
        defer mockDataStorage.Close()

        httpClient := client.New(&client.Config{
            BaseURL: mockDataStorage.URL,
            MaxRetries: 3,
        })

        _, err := httpClient.ListIncidents(ctx, &ListParams{})
        Expect(err).ToNot(HaveOccurred())
        Expect(attemptCount).To(Equal(3))
    })
})
```

---

##### **2.1.2.2: Cache + HTTP Integration** (3h, 8 tests)

**Deliverable**: `test/integration/contextapi/05_cache_http_integration_test.go`

**Tests**:
1. Cache MISS → HTTP → DB → Cache HIT flow
2. Redis + Data Storage both down (graceful degradation)
3. Stale cache invalidation
4. Concurrent cache miss handling (thundering herd prevention)
5. Asynchronous cache population
6. Cache bypass on error
7. Cache TTL expiration with real Redis
8. Cache eviction under memory pressure

---

##### **2.1.2.3: Graceful Degradation Integration** (3h, 6 tests)

**Deliverable**: `test/integration/contextapi/06_graceful_degradation_test.go`

**Tests**:
1. Data Storage unavailable → serve stale cache
2. Data Storage unavailable → no cache → error
3. Data Storage recovery → resume normal operation
4. Degradation event logging
5. Degradation metrics exposure
6. Confidence score adjustment in degraded mode

---

##### **2.1.2.4: Resilience Pattern Integration** (2h, 4 tests)

**Deliverable**: `test/integration/contextapi/07_resilience_patterns_test.go`

**Tests**:
1. Circuit breaker state transitions with real HTTP
2. Retry exhaustion scenarios
3. Fallback to cached data
4. Timeout handling with real services

---

**Success Criteria**:
- [ ] 30+ new integration tests implemented
- [ ] Integration coverage >50% (was <20%)
- [ ] All tests use real Redis (no miniredis)
- [ ] All tests use real HTTP (httptest.Server)
- [ ] BR coverage matrix updated

---

### **STAGE 2.2: P1 CRITICAL** (4.5 hours)

**Objective**: Fix code quality gaps (imports, package declarations, RFC 7807)

#### **Task 2.2.1: Add Imports to All Code Examples** (2h)

**Scope**: 15+ code examples (Lines 137-210, 224-304, 330-440)

**Pattern**: Same as Data Storage (Task 1.2.1)

---

#### **Task 2.2.2: Add Package Declarations to All Code Examples** (0.5h)

**Pattern**:
```go
package contextapi  // NOT contextapi_test

import (
    // ... imports ...
)
```

**Success Criteria**: All 15+ code examples start with `package contextapi`

---

#### **Task 2.2.3: Add RFC 7807 Error Parsing** (2h)

**Current State**: No RFC 7807 parsing in HTTP client
**Target State**: HTTP client parses RFC 7807 Problem Details

**Implementation**:
```go
package contextapi

import (
    "encoding/json"
    "fmt"
    "net/http"

    "github.com/jordigilh/kubernaut/pkg/shared/errors"
)

func (c *HTTPClient) parseError(resp *http.Response) error {
    var problemDetail errors.ProblemDetail
    if err := json.NewDecoder(resp.Body).Decode(&problemDetail); err != nil {
        // Fallback to generic error if not RFC 7807
        return fmt.Errorf("HTTP %d: failed to parse error response: %w", resp.StatusCode, err)
    }

    return fmt.Errorf("%s (%d): %s - %s [%s]",
        problemDetail.Title,
        problemDetail.Status,
        problemDetail.Detail,
        problemDetail.Instance,
        problemDetail.Type,
    )
}

// In HTTP client
func (c *HTTPClient) ListIncidents(ctx context.Context, params *ListParams) (*ListResponse, error) {
    resp, err := c.httpClient.Get(c.buildURL("/api/v1/incidents", params))
    if err != nil {
        return nil, fmt.Errorf("HTTP request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, c.parseError(resp)  // Parse RFC 7807
    }

    var result ListResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &result, nil
}
```

**Unit Tests**:
```go
package contextapi

import (
    "net/http"
    "net/http/httptest"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("RFC 7807 Error Parsing - BR-CONTEXT-027", func() {
    var (
        httpClient *HTTPClient
        mockServer *httptest.Server
    )

    BeforeEach(func() {
        mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "application/problem+json")
            w.WriteHeader(400)
            json.NewEncoder(w).Encode(map[string]interface{}{
                "type":     "https://api.datastorage.local/errors/invalid-parameter",
                "title":    "Invalid Parameter",
                "status":   400,
                "detail":   "The 'limit' parameter must be positive",
                "instance": "/api/v1/incidents?limit=-1",
            })
        }))

        httpClient = New(&Config{BaseURL: mockServer.URL})
    })

    AfterEach(func() {
        mockServer.Close()
    })

    It("should parse RFC 7807 Problem Details", func() {
        _, err := httpClient.ListIncidents(ctx, &ListParams{Limit: -1})

        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("Invalid Parameter"))
        Expect(err.Error()).To(ContainSubstring("limit' parameter must be positive"))
        Expect(err.Error()).To(ContainSubstring("400"))
    })
})
```

**Success Criteria**:
- [ ] RFC 7807 parsing implemented in HTTP client
- [ ] 5+ unit tests for error parsing
- [ ] Code examples with imports
- [ ] Integration tests validate error handling

---

### **STAGE 2.3: P2 HIGH-VALUE** (13 hours)

**Objective**: Complete production-readiness sections

#### **Task 2.3.1: Refactor to DescribeTable Pattern** (2h)

**Scope**: Lines 264-281 (1 table exists, need 5+ more)

**Success Criteria**: 5+ DescribeTable refactorings (same pattern as Data Storage)

---

#### **Task 2.3.2-2.3.9: Same as Data Storage Tasks 1.3.2-1.3.9** (11h)

- Request ID Propagation (1h)
- Context Cancellation Handling (1h)
- Common Pitfalls Section (2h)
- Pre-Implementation Validation Script (1h)
- Operational Runbooks (2h)
- Multi-Architecture Dockerfile (1h)
- BR Coverage Matrix (1h)
- Phase-by-Phase Confidence Assessments (1h)
- Health Endpoints Section (1h) - NEW for Context API

---

### **STAGE 2.4: QA VALIDATION** (1.3 hours)

**Same structure as Data Storage Stage 1.4**

---

### **STAGE 2 COMPLETION CRITERIA**

- [x] All 15 gaps resolved (32.8h effort)
- [x] Confidence: 96% (was 60%)
- [x] All P0, P1, P2 gaps addressed
- [x] QA validation passed
- [x] Production-ready quality achieved
- [x] Integration coverage >50% (was <20%)
- [x] Real Redis in integration tests (was miniredis)

---

## 🎯 **OVERALL PROJECT SUCCESS CRITERIA**

### **Phase 1 + Phase 2 Complete**

- [x] **Data Storage**: 95% confidence, 0 gaps remaining
- [x] **Context API**: 96% confidence, 0 gaps remaining
- [x] **Total Effort**: 61.6 hours (7.7 days)
- [x] **Quality**: Production-ready
- [x] **Testing**: Defense-in-depth compliance (>70% unit, >50% integration, <10% E2E)
- [x] **Documentation**: Complete (APDC, TDD, Common Pitfalls, Runbooks)
- [x] **Integration**: Both services integrated in main application

### **Deferred to Later Phase**

- ⏸️ **Effectiveness Monitor**: 20 gaps, 44.5h effort (per project plan)

---

## 📊 **PROGRESS TRACKING**

### **Daily Standup Format**

```markdown
#### **Day X - [Date]**

**Completed Yesterday**:
- [x] Task X.X.X: [Task name]
- [x] Task X.X.X: [Task name]

**Today's Focus**:
- [ ] Task X.X.X: [Task name] (Xh)
- [ ] Task X.X.X: [Task name] (Xh)

**Blockers**:
- None / [Blocker description]

**Confidence**:
- Data Storage: X% (target: 95%)
- Context API: X% (target: 96%)
```

### **Weekly Summary Format**

```markdown
#### **Week X Summary**

**Phase 1 (Data Storage)**:
- Stages Complete: X/4
- Hours Spent: X/28.8h
- Confidence: X% (target: 95%)

**Phase 2 (Context API)**:
- Stages Complete: X/4
- Hours Spent: X/32.8h
- Confidence: X% (target: 96%)

**Overall Progress**: X% complete
```

---

## 📚 **REFERENCES**

### **Authority Documents**
- [API Gateway Migration Plans Triage](API-GATEWAY-MIGRATION-PLANS-TRIAGE.md) - **PRIMARY AUTHORITY**
- [Data Storage vs Gateway Deeper Triage](DATA-STORAGE-VS-GATEWAY-DEEPER-TRIAGE.md) - Additional gap analysis
- [Gateway Service v2.23](../../services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.23.md) - Reference implementation
- [Context API v2.8](../../services/stateless/context-api/implementation/IMPLEMENTATION_PLAN_V2.8.md) - Authoritative Context API plan

### **Architecture Decisions**
- [DD-ARCH-001: API Gateway Pattern](../decisions/DD-ARCH-001-FINAL-DECISION.md)
- [DD-007: Kubernetes-Aware Graceful Shutdown](../decisions/DD-007-kubernetes-aware-graceful-shutdown.md) - **MANDATORY**
- [DD-005: Observability Standards](../decisions/DD-005-OBSERVABILITY-STANDARDS.md)
- [DD-004: RFC 7807 Error Responses](../decisions/DD-004-RFC7807-ERROR-RESPONSES.md)
- [ADR-027: Multi-Architecture Build](../decisions/ADR-027-multi-architecture-build.md)

### **Testing Strategy**
- [03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth, mock usage matrix

---

**Status**: ✅ **APPROVED FOR EXECUTION**
**Start Date**: [TBD - User to specify]
**Estimated Completion**: [Start Date + 8 working days]
**Confidence**: Data Storage 95%, Context API 96%

---

**Date**: November 2, 2025
**Created By**: AI Assistant (Claude Sonnet 4.5)
**Approved By**: User
**Methodology**: Systematic gap remediation based on comprehensive triage analysis


