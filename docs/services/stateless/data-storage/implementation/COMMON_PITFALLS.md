# Data Storage Service - Common Pitfalls

**Service**: Data Storage Service  
**Purpose**: Document common mistakes and prevention strategies  
**Authority**: Gateway v2.23 (lines 519-905) + Testing Strategy Rules  
**Date**: November 2, 2025

---

## 🚨 **CRITICAL PITFALLS** (Production-Blocking)

### **Pitfall 1: SQL Injection via String Concatenation** ⚠️ **MOST CRITICAL**

**Problem**: Using `fmt.Sprintf` or string concatenation to build SQL queries with user input

**Business Impact**:
- Violates BR-STORAGE-025 (Security: SQL injection prevention)
- **DATA BREACH RISK**: Attacker can access/modify any data
- **DATA CORRUPTION RISK**: `DROP TABLE` attacks
- **COMPLIANCE VIOLATION**: GDPR, SOC 2 failures

**Example**:
```go
package datastorage

import (
    "database/sql"
    "fmt"
)

// ❌ BAD: SQL injection vulnerability
func (q *QueryExecutor) ListIncidents(namespace string) ([]Incident, error) {
    query := fmt.Sprintf("SELECT * FROM resource_action_traces WHERE namespace = '%s'", namespace)
    rows, err := q.db.Query(query)
    // VULNERABILITY: namespace = "'; DROP TABLE resource_action_traces; --"
    // Result: Table is dropped!
    return results, err
}

// ✅ GOOD: Parameterized query (SQL injection safe)
func (q *QueryExecutor) ListIncidents(namespace string) ([]Incident, error) {
    query := "SELECT * FROM resource_action_traces WHERE namespace = ?"
    rows, err := q.db.Query(query, namespace)
    // SAFE: namespace is properly escaped by database driver
    return results, err
}
```

**Prevention**:
1. **ALWAYS** use parameterized queries with `?` placeholders
2. **NEVER** use `fmt.Sprintf` to inject values into SQL
3. **NEVER** use string concatenation (`+`) to build SQL queries
4. Use SQL query builders that enforce parameterization

**Detection**:
```bash
# Find potential SQL injection vulnerabilities
grep -rn "fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE" pkg/datastorage/
# If this returns results, investigate immediately!
```

**Test Coverage**:
See `test/unit/datastorage/sql_injection_test.go` - Lines 189-193 validate SQL injection prevention

---

### **Pitfall 2: Missing Package Declarations in Test Files** ⚠️

**Problem**: Test files missing `package` declaration or using wrong package name

**Business Impact**:
- Tests won't compile
- Delays implementation by 1-2 hours
- Violates project testing strategy (white-box testing)

**Example**:
```go
// ❌ BAD: Missing package declaration
import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Query Builder", func() {
    // ...
})

// ❌ BAD: Wrong package (black-box testing)
package datastorage_test

import (
    . "github.com/onsi/ginkgo/v2"
)

// ✅ GOOD: Correct package declaration (white-box testing)
package datastorage

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Query Builder", func() {
    // ...
})
```

**Prevention**:
- **ALWAYS** start test files with `package datastorage` (or appropriate package name)
- Use white-box testing (same package as code under test)
- Never use `package datastorage_test` suffix

**Detection**:
```bash
# Find test files missing package declarations
head -1 test/unit/datastorage/*_test.go | grep -v "^package"
```

---

### **Pitfall 3: Null Testing Anti-Pattern** ⚠️

**Problem**: Weak assertions like `Expect(x).ToNot(BeNil())` instead of specific value checks

**Business Impact**:
- Tests pass but don't validate business logic
- Production bugs not caught (false sense of security)
- Violates 03-testing-strategy.mdc

**Example**:
```go
package datastorage

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

// ❌ BAD: Weak assertions (null testing)
It("should list incidents", func() {
    incidents, err := storage.ListIncidents(ctx, params)
    Expect(err).ToNot(HaveOccurred())    // Passes if no error
    Expect(incidents).ToNot(BeNil())      // Passes for ANY non-nil value
    Expect(len(incidents)).To(BeNumerically(">", 0)) // Passes for any count > 0
})

// ✅ GOOD: Specific value assertions (business validation)
It("should list incidents with correct namespace filter - BR-STORAGE-022", func() {
    params := &ListParams{Namespace: "production"}
    incidents, err := storage.ListIncidents(ctx, params)
    
    Expect(err).ToNot(HaveOccurred())
    Expect(len(incidents)).To(Equal(3)) // Specific expected count
    
    // Validate EVERY incident matches filter
    for _, inc := range incidents {
        Expect(inc.Namespace).To(Equal("production"))
        Expect(inc.Severity).ToNot(BeEmpty())
        Expect(inc.Timestamp).ToNot(BeZero())
    }
})
```

**Prevention**:
- Assert specific expected values, not just "not nil"
- Validate all important fields in returned objects
- Use `Equal()` instead of `BeNumerically(">", 0)`
- Map tests to business requirements (BR-XXX-XXX)

**Detection**:
```bash
# Find weak assertions
grep -rn "ToNot(BeNil())\|BeNumerically(\">\"" test/unit/datastorage/
# Review each occurrence for business value validation
```

---

## ⚠️ **HIGH-PRIORITY PITFALLS**

### **Pitfall 4: Missing Import Statements in Code Examples**

**Problem**: Code examples in documentation lack import statements

**Business Impact**:
- Code not copy-pasteable
- Developers waste time figuring out imports
- Increased implementation errors

**Example**:
```go
// ❌ BAD: No imports
type QueryBuilder struct {
    db *sqlx.DB
}

// ✅ GOOD: Complete with imports
package datastorage

import (
    "context"
    "database/sql"
    
    "github.com/jmoiron/sqlx"
    "go.uber.org/zap"
)

type QueryBuilder struct {
    db     *sqlx.DB
    logger *zap.Logger
}
```

**Prevention**: All code examples must include complete import statements

---

### **Pitfall 5: Unicode Edge Cases Not Tested**

**Problem**: Not testing Arabic, Chinese, emoji in filter values

**Business Impact**:
- BR-STORAGE-026 violation
- Production failures for international users
- Data loss for non-ASCII characters

**Example**:
```go
package datastorage

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

// ❌ BAD: Only ASCII test cases
DescribeTable("namespace filtering",
    func(namespace string) {
        // Test with namespace
    },
    Entry("production", "production"),
    Entry("staging", "staging"),
)

// ✅ GOOD: Unicode edge cases included
DescribeTable("namespace filtering - BR-STORAGE-026",
    func(namespace string) {
        params := &ListParams{Namespace: namespace}
        incidents, err := storage.ListIncidents(ctx, params)
        
        Expect(err).ToNot(HaveOccurred())
        for _, inc := range incidents {
            Expect(inc.Namespace).To(Equal(namespace))
        }
    },
    Entry("ASCII", "production"),
    Entry("Arabic", "مساحة-الإنتاج"),
    Entry("Chinese", "生产环境"),
    Entry("Emoji", "prod-🚀"),
    Entry("Mixed", "prod-环境-🔥"),
)
```

**Prevention**: Always include Arabic, Chinese, and emoji test cases for string fields

---

### **Pitfall 6: Pagination Boundary Errors**

**Problem**: Off-by-one errors in limit/offset calculations

**Business Impact**:
- BR-STORAGE-023 violation
- Missing or duplicate records in pagination
- User complaints about data inconsistency

**Example**:
```go
package datastorage

import (
    "fmt"
)

// ❌ BAD: Off-by-one error
func (q *QueryBuilder) Build() (string, []interface{}, error) {
    // limit=10, offset=0 should return records 0-9
    // But this returns records 1-10 (missing record 0!)
    query := fmt.Sprintf("SELECT * FROM incidents LIMIT %d OFFSET %d", q.limit, q.offset+1)
    return query, nil, nil
}

// ✅ GOOD: Correct boundary handling
func (q *QueryBuilder) Build() (string, []interface{}, error) {
    if q.limit <= 0 || q.limit > 1000 {
        return "", nil, fmt.Errorf("limit must be 1-1000")
    }
    if q.offset < 0 {
        return "", nil, fmt.Errorf("offset must be >= 0")
    }
    
    query := "SELECT * FROM incidents LIMIT ? OFFSET ?"
    args := []interface{}{q.limit, q.offset}
    return query, args, nil
}
```

**Prevention**:
- Test boundary cases: limit=1, limit=1000, offset=0
- Test off-by-one: Compare first/last records
- Validate pagination math in unit tests

---

### **Pitfall 7: Missing RFC 7807 Error Types**

**Problem**: Using generic `errors.New()` instead of structured Problem Details

**Business Impact**:
- BR-STORAGE-024 violation
- Clients can't parse error details
- Poor error UX

**Example**:
```go
package datastorage

import (
    "errors"
    "fmt"
    "net/http"
    
    sharedErrors "github.com/jordigilh/kubernaut/pkg/shared/errors"
)

// ❌ BAD: Generic error
func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
    if limit > 1000 {
        http.Error(w, "limit too large", http.StatusBadRequest)
        return
    }
}

// ✅ GOOD: RFC 7807 Problem Details
func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
    if limit > 1000 {
        problemDetail := sharedErrors.NewProblemDetail(
            http.StatusBadRequest,
            "Invalid Limit Parameter",
            fmt.Errorf("limit %d exceeds maximum 1000", limit),
        )
        problemDetail.Type = "https://api.datastorage.local/errors/invalid-limit"
        problemDetail.Instance = r.URL.Path
        sharedErrors.WriteProblemDetail(w, problemDetail)
        return
    }
}
```

**Prevention**: Always use `pkg/shared/errors.ProblemDetail` for API errors

---

### **Pitfall 8: Missing Context Cancellation Checks**

**Problem**: Not checking `ctx.Done()` in long-running operations

**Business Impact**:
- Resource leaks (goroutines, DB connections)
- Slow shutdown (operations don't cancel)
- Wasted compute on cancelled requests

**Example**:
```go
package datastorage

import (
    "context"
    "fmt"
)

// ❌ BAD: No context cancellation check
func (q *QueryExecutor) ListIncidents(ctx context.Context, params *ListParams) ([]Incident, error) {
    rows, err := q.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var incidents []Incident
    for rows.Next() {
        // If context is cancelled, this keeps iterating!
        var inc Incident
        rows.Scan(&inc)
        incidents = append(incidents, inc)
    }
    return incidents, nil
}

// ✅ GOOD: Explicit context cancellation handling
func (q *QueryExecutor) ListIncidents(ctx context.Context, params *ListParams) ([]Incident, error) {
    // Check before starting
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    rows, err := q.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var incidents []Incident
    for rows.Next() {
        // Check during iteration
        select {
        case <-ctx.Done():
            return nil, fmt.Errorf("query cancelled: %w", ctx.Err())
        default:
        }
        
        var inc Incident
        if err := rows.Scan(&inc); err != nil {
            return nil, err
        }
        incidents = append(incidents, inc)
    }
    return incidents, nil
}
```

**Prevention**: Add `select { case <-ctx.Done(): }` checks in loops and before expensive operations

---

### **Pitfall 9: Missing DD-007 Graceful Shutdown**

**Problem**: Using standard Go `http.Server.Shutdown()` without Kubernetes coordination

**Business Impact**:
- **5-10% request failures** during rolling updates
- Data corruption from aborted database writes
- Resource leaks (unclosed connections)
- Production-blocking

**Example**:
```go
package datastorage

import (
    "context"
    "net/http"
    "sync/atomic"
    "time"
    
    "go.uber.org/zap"
)

// ❌ BAD: No Kubernetes coordination
func (s *Server) Shutdown(ctx context.Context) error {
    return s.httpServer.Shutdown(ctx) // 5-10% failures during rolling updates
}

// ✅ GOOD: DD-007 4-step graceful shutdown
type Server struct {
    httpServer     *http.Server
    dbClient       *sql.DB
    logger         *zap.Logger
    isShuttingDown atomic.Bool  // REQUIRED for readiness coordination
}

func (s *Server) Shutdown(ctx context.Context) error {
    // STEP 1: Set shutdown flag (readiness probe → 503)
    s.isShuttingDown.Store(true)
    s.logger.Info("Shutdown flag set - readiness probe now returns 503")
    
    // STEP 2: Wait for Kubernetes endpoint removal propagation
    time.Sleep(5 * time.Second)
    
    // STEP 3: Drain in-flight HTTP connections
    if err := s.httpServer.Shutdown(ctx); err != nil {
        return err
    }
    
    // STEP 4: Close external resources
    s.dbClient.Close()
    return nil
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
    // Check shutdown flag FIRST
    if s.isShuttingDown.Load() {
        w.WriteHeader(503)
        return
    }
    // ... normal health checks ...
}
```

**Prevention**: Implement DD-007 pattern (see Day 7 in implementation plan)

**Reference**: [DD-007: Kubernetes-Aware Graceful Shutdown](../../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md)

---

### **Pitfall 10: Hard-Coded Configuration**

**Problem**: Hard-coded database hosts, ports, credentials

**Business Impact**:
- Cannot deploy to different environments
- Security risk (credentials in code)
- Inflexible configuration

**Example**:
```go
package datastorage

import (
    "database/sql"
    "os"
    
    _ "github.com/lib/pq"
)

// ❌ BAD: Hard-coded configuration
func NewDB() (*sql.DB, error) {
    connStr := "host=localhost port=5432 user=postgres password=secret dbname=action_history"
    return sql.Open("postgres", connStr)
}

// ✅ GOOD: Environment-based configuration
func NewDB() (*sql.DB, error) {
    connStr := fmt.Sprintf(
        "host=%s port=%s user=%s password=%s dbname=%s",
        os.Getenv("DB_HOST"),
        os.Getenv("DB_PORT"),
        os.Getenv("DB_USER"),
        os.Getenv("DB_PASSWORD"),
        os.Getenv("DB_NAME"),
    )
    return sql.Open("postgres", connStr)
}
```

**Prevention**: Use environment variables or config files for all configuration

---

## 📋 **PITFALL DETECTION CHECKLIST**

Before submitting code, run these checks:

```bash
# 1. Check for SQL injection vulnerabilities
grep -rn "fmt.Sprintf.*SELECT\|fmt.Sprintf.*WHERE" pkg/datastorage/

# 2. Check for missing package declarations
head -1 test/**/*_test.go | grep -v "^package"

# 3. Check for null testing anti-pattern
grep -rn "ToNot(BeNil())" test/unit/datastorage/

# 4. Check for missing imports in docs
grep -A5 "```go" docs/services/stateless/data-storage/ | grep -v "^import"

# 5. Check for missing context cancellation
grep -rn "for.*rows.Next()" pkg/datastorage/ | xargs grep -L "ctx.Done()"

# 6. Check for hard-coded config
grep -rn "localhost\|127.0.0.1\|password=" pkg/datastorage/

# 7. Check for missing DD-007
grep -rn "isShuttingDown" pkg/datastorage/server/ || echo "❌ Missing DD-007"
```

---

## 🎯 **SUCCESS METRICS**

**Pitfall Prevention Success**:
- ✅ Zero SQL injection vulnerabilities (security audit passed)
- ✅ All test files have package declarations
- ✅ No null testing anti-patterns (all assertions specific)
- ✅ All code examples have imports
- ✅ Unicode test coverage 100%
- ✅ Zero pagination boundary bugs
- ✅ All API errors use RFC 7807
- ✅ Context cancellation in all long operations
- ✅ DD-007 implemented (0% request failures during deployments)
- ✅ Zero hard-coded configuration

---

**Date**: November 2, 2025  
**Author**: AI Assistant (Claude Sonnet 4.5)  
**Authority**: Gateway v2.23 (lines 519-905) + Project Testing Strategy

