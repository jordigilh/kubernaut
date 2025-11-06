# DD-010: PostgreSQL Driver Migration - lib/pq to pgx

**Status**: ✅ Approved (2025-11-03)
**Date**: 2025-11-03
**Decision Makers**: Development Team
**Priority**: **P0 - CRITICAL RISK** (Maintenance mode driver in production)
**Supersedes**: None
**Related To**: ADR-027 (Multi-Architecture Builds), DD-INFRASTRUCTURE-002 (Data Storage Redis)

---

## 📋 **Context**

**Critical Risk Identified**: `lib/pq` PostgreSQL driver is **no longer actively maintained** (maintenance mode since 2021).

**Current Usage**:
- Data Storage Service: `lib/pq` v1.10.9
- All PostgreSQL connections use `database/sql` standard library
- Production deployment planned for V1.0 (Q1 2025)

**Risk Impact**:
- ❌ No security patches for new vulnerabilities
- ❌ No bug fixes for discovered issues
- ❌ No support for new PostgreSQL features
- ❌ No performance improvements
- ❌ Community moving away from lib/pq

**Business Requirements**:
- BR-PLATFORM-006: Production-ready infrastructure with active support
- ADR-032: 7+ year audit retention (requires long-term driver support)
- BR-SECURITY-001: Security vulnerability management

---

## 🎯 **Decision**

**APPROVED**: Migrate from `lib/pq` to **`pgx`** (jackc/pgx) as the PostgreSQL driver.

**Migration Strategy**: Use `pgx/stdlib` adapter for minimal code changes (compatible with `database/sql`).

**Timeline**: Complete before V1.0 production release (Q1 2025).

---

## 🔍 **Alternatives Considered**

### **Alternative A: pgx with stdlib adapter** ✅ **APPROVED**

**Approach**: Use `pgx/v5/stdlib` adapter with existing `database/sql` code

**Migration Impact**:
```go
// BEFORE (lib/pq)
import _ "github.com/lib/pq"
db, err := sql.Open("postgres", connStr)

// AFTER (pgx with stdlib adapter)
import _ "github.com/jackc/pgx/v5/stdlib"
db, err := sql.Open("pgx", connStr) // Only change: driver name
```

**Pros**:
- ✅ **Actively Maintained**: Jack Christensen (PostgreSQL core contributor)
- ✅ **Minimal Code Changes**: Only import + driver name change
- ✅ **Drop-in Replacement**: Compatible with `database/sql` API
- ✅ **Better Performance**: Uses PostgreSQL binary protocol
- ✅ **Pure Go**: No CGO required (`CGO_ENABLED=0` works)
- ✅ **Industry Standard**: Default choice for new Go + PostgreSQL projects
- ✅ **Security**: Active CVE monitoring and patches
- ✅ **PostgreSQL Features**: LISTEN/NOTIFY, COPY, arrays, JSON support
- ✅ **Connection Pooling**: Built-in pgxpool for advanced use cases

**Cons**:
- ⚠️ **Migration Effort**: 2-4 hours (import changes, testing, validation)
- ⚠️ **Dependency Change**: New module in go.mod
- ⚠️ **Slightly Larger Binary**: ~500KB increase (negligible)

**Confidence**: **95%** - This is the right choice for production

---

### **Alternative B: Keep lib/pq** ❌ **REJECTED**

**Approach**: Continue using `lib/pq` in maintenance mode

**Pros**:
- ✅ **No Migration**: Zero effort
- ✅ **Stable**: No breaking changes expected

**Cons**:
- ❌ **CRITICAL RISK**: No security patches for new CVEs
- ❌ **No Bug Fixes**: Known issues won't be fixed
- ❌ **No Community Support**: GitHub issues closed, no active development
- ❌ **Technical Debt**: Will require migration eventually (harder later)
- ❌ **Compliance Risk**: 7+ year audit retention with unmaintained driver

**Confidence**: **10%** - Unacceptable risk for production

---

### **Alternative C: Native pgx API** ❌ **REJECTED**

**Approach**: Use native `pgx` API (not `database/sql` adapter)

**Pros**:
- ✅ **Maximum Performance**: Direct pgx API is faster than stdlib adapter
- ✅ **Full Feature Access**: All pgx features available

**Cons**:
- ❌ **Major Code Changes**: Rewrite all database code
- ❌ **High Migration Effort**: 2-3 weeks of work
- ❌ **Breaking Changes**: Different API patterns
- ❌ **Testing Overhead**: Extensive regression testing required

**Confidence**: **30%** - Overkill for current needs, consider for V2.0

---

## 📊 **Migration Plan**

### **Phase 1: Data Storage Service** (2-3 hours)

**Scope**: Migrate Data Storage Service first (isolated, well-tested)

**Steps**:
1. Update `go.mod`:
   ```bash
   go get github.com/jackc/pgx/v5
   go mod tidy
   ```

2. Update imports (3 files):
   ```go
   // pkg/datastorage/server/server.go
   - import _ "github.com/lib/pq"
   + import _ "github.com/jackc/pgx/v5/stdlib"

   // pkg/datastorage/repository/*.go (same change)
   ```

3. Update connection strings (1 file):
   ```go
   // cmd/datastorage/main.go
   - db, err := sql.Open("postgres", connStr)
   + db, err := sql.Open("pgx", connStr)
   ```

4. Run tests:
   ```bash
   go test ./pkg/datastorage/... -v
   go test ./test/integration/datastorage -v
   ```

5. Validate:
   - ✅ All unit tests pass
   - ✅ All integration tests pass
   - ✅ HTTP API integration tests pass
   - ✅ DLQ fallback works
   - ✅ RFC 7807 errors work

**Success Criteria**:
- 100% test pass rate
- No performance regression (p95 latency < 1s)
- Binary size increase < 1MB

---

### **Phase 2: Other Services** (1-2 hours each)

**Services to Migrate** (in order):
1. **Context API Service** (reads from Data Storage)
2. **Gateway Service** (if using PostgreSQL directly)
3. **Notification Controller** (audit writes via Data Storage)
4. **Remediation Processor** (audit writes via Data Storage)

**Note**: Most services use Data Storage HTTP API, so they're unaffected by driver change.

---

### **Phase 3: Integration Testing** (2-3 hours)

**Test Matrix**:
| Test Type | Scope | Success Criteria |
|-----------|-------|------------------|
| **Unit Tests** | All services | 100% pass rate |
| **Integration Tests** | Data Storage, Context API | 100% pass rate |
| **E2E Tests** | Full workflow | 100% pass rate |
| **Performance Tests** | Data Storage Write API | p95 < 1s, 50 writes/sec |
| **Stress Tests** | 10K+ records | No memory leaks, stable latency |

---

### **Phase 4: Production Validation** (1 hour)

**Validation Steps**:
1. Deploy to staging environment
2. Run smoke tests (create/read/update audit records)
3. Monitor metrics (latency, error rate, memory usage)
4. Verify DLQ fallback works
5. Verify graceful shutdown works
6. Promote to production

**Rollback Plan**:
- Keep `lib/pq` in go.mod as fallback
- Revert import changes if issues found
- Estimated rollback time: 15 minutes

---

## 🔧 **Implementation Details**

### **Connection String Compatibility**

**Good News**: `pgx` uses same connection string format as `lib/pq`

```go
// Both drivers support this format
connStr := fmt.Sprintf(
    "host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
    host, port, dbname, user, password,
)

// pgx also supports PostgreSQL URI format
connStr := "postgresql://user:password@host:port/dbname?sslmode=disable"
```

**No connection string changes required** ✅

---

### **Error Handling Compatibility**

**pgx errors are compatible with `database/sql` errors**:

```go
// Existing error handling works unchanged
if err == sql.ErrNoRows {
    return nil, ErrNotFound
}

// Unique constraint violation detection works
if strings.Contains(err.Error(), "duplicate key") {
    return nil, ErrConflict
}
```

**No error handling changes required** ✅

---

### **Transaction Support**

**pgx fully supports `database/sql` transactions**:

```go
// Existing transaction code works unchanged
tx, err := db.BeginTx(ctx, nil)
if err != nil {
    return err
}
defer tx.Rollback()

// ... database operations ...

return tx.Commit()
```

**No transaction code changes required** ✅

---

## 📈 **Performance Comparison**

### **Benchmark Results** (from pgx documentation)

| Operation | lib/pq | pgx (stdlib) | pgx (native) | Improvement |
|-----------|--------|--------------|--------------|-------------|
| **Simple Query** | 100 µs | 80 µs | 60 µs | 20-40% faster |
| **Prepared Statement** | 90 µs | 70 µs | 50 µs | 22-44% faster |
| **Batch Insert** | 1000 µs | 800 µs | 500 µs | 20-50% faster |
| **Connection Pool** | N/A | Good | Excellent | Built-in pgxpool |

**Expected Impact for Data Storage**:
- ✅ 20-30% faster audit writes
- ✅ Lower p95/p99 latency
- ✅ Better connection pooling
- ✅ Lower memory usage

---

## 🔒 **Security Considerations**

### **CVE Management**

**lib/pq** (maintenance mode):
- ❌ Last security patch: 2021
- ❌ Known issues: No fixes planned
- ❌ CVE response time: N/A (no active maintenance)

**pgx** (actively maintained):
- ✅ Active CVE monitoring
- ✅ Security patches within days
- ✅ Community security audits
- ✅ PostgreSQL core contributor involvement

### **Compliance Impact**

**ADR-032 Requirement**: 7+ year audit retention

**Risk with lib/pq**:
- ❌ Unmaintained driver for 7+ years
- ❌ Potential security vulnerabilities
- ❌ Compliance audit failures

**Mitigation with pgx**:
- ✅ Active maintenance for 7+ years (likely)
- ✅ Security patches available
- ✅ Compliance audit ready

---

## 📋 **Migration Checklist**

### **Pre-Migration**
- [ ] Review pgx documentation
- [ ] Backup current go.mod/go.sum
- [ ] Document current performance baselines
- [ ] Identify all PostgreSQL connection points

### **Migration**
- [ ] Update go.mod (add pgx, keep lib/pq for rollback)
- [ ] Update imports (server.go, repository files)
- [ ] Update sql.Open() calls (postgres → pgx)
- [ ] Run unit tests
- [ ] Run integration tests
- [ ] Run performance tests

### **Validation**
- [ ] All tests pass (100% pass rate)
- [ ] Performance baseline maintained (p95 < 1s)
- [ ] Memory usage stable
- [ ] DLQ fallback works
- [ ] Graceful shutdown works
- [ ] RFC 7807 errors work

### **Post-Migration**
- [ ] Remove lib/pq from go.mod
- [ ] Update documentation
- [ ] Update Dockerfile comments
- [ ] Update ADRs/DDs
- [ ] Monitor production metrics (1 week)

---

## 🎯 **Success Metrics**

| Metric | Target | Actual (TBD) |
|--------|--------|--------------|
| **Migration Time** | < 4 hours | TBD |
| **Test Pass Rate** | 100% | TBD |
| **Performance** | p95 < 1s | TBD |
| **Binary Size Increase** | < 1MB | TBD |
| **Memory Usage** | No regression | TBD |
| **Rollback Time** | < 15 min | N/A (no rollback) |

---

## 🔗 **Related Decisions**

- **Builds On**: ADR-027 (Multi-Architecture Builds - CGO_ENABLED=0 requirement)
- **Supports**: ADR-032 (Data Access Layer Isolation - 7+ year audit retention)
- **Related To**: DD-INFRASTRUCTURE-002 (Data Storage Redis Strategy)

---

## 📚 **References**

- **pgx GitHub**: https://github.com/jackc/pgx
- **pgx Documentation**: https://pkg.go.dev/github.com/jackc/pgx/v5
- **lib/pq Maintenance Mode**: https://github.com/lib/pq/issues/1030
- **PostgreSQL Go Drivers Comparison**: https://gosamples.dev/list-postgresql-drivers/
- **pgx Performance Benchmarks**: https://github.com/jackc/pgx/wiki/Performance

---

## ✅ **Approval**

**Decision**: Alternative A - pgx with stdlib adapter
**Confidence**: **95%**
**Status**: ✅ **APPROVED** (2025-11-03)
**Priority**: **P0 - CRITICAL** (Must complete before V1.0 production release)

**Rationale**:
1. **Risk Mitigation**: Eliminates unmaintained driver risk
2. **Minimal Effort**: 2-4 hours migration time
3. **Drop-in Replacement**: Compatible with existing code
4. **Performance Improvement**: 20-30% faster
5. **Industry Standard**: Default choice for Go + PostgreSQL
6. **Security**: Active CVE monitoring and patches
7. **Compliance**: Supports 7+ year audit retention requirement

**Next Steps**:
1. Execute Phase 1 (Data Storage Service migration) - **IMMEDIATE**
2. Validate integration tests pass - **SAME DAY**
3. Execute Phase 2 (other services) - **WITHIN 1 WEEK**
4. Production deployment - **BEFORE V1.0 RELEASE**

---

**Last Updated**: 2025-11-03
**Next Review**: After Phase 1 completion (validate decision)

