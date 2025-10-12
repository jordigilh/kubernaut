# Gateway Integration Test Fix Plan

## Executive Summary
**31 failing integration tests** identified after Phase 2 & 3 test extensions.
**Root Cause**: Implementation gaps in Gateway service, not test errors.

## Fix Approach: Option A - Sequential Implementation Gap Resolution

### Phase 1: Storm Aggregation Response (Fixes 11 tests) 🚨 HIGHEST IMPACT
**Issue**: Gateway returns `"status": "created"` instead of `"status": "accepted"` for alerts during storm aggregation window.

**Root Cause**: `server.go:processSignal()` returns "created" response even when alert is added to aggregation window.

**Fix**:
```go
// In pkg/gateway/server.go:processSignal()

// After: s.stormAggregator.AddResource(ctx, windowID, signal)
// WRONG (current):
return ProcessingResponse{
    Status: StatusCreated, // ❌ Should be StatusAccepted
    // ...
}

// CORRECT (fix):
return ProcessingResponse{
    Status: StatusAccepted, // ✅ Alert accepted for aggregation
    Message: "Alert accepted for storm aggregation",
    IsStorm: true,
    StormType: stormMetadata.StormType,
    WindowID: windowID,
    // ... (do NOT include RemediationRequestName/Namespace)
}
```

**Files to Modify**:
- `pkg/gateway/server.go` (2 locations in `processSignal`)

**Tests Fixed**: 11
- BR-GATEWAY-016 storm aggregation tests (all permutations)

---

### Phase 2: Health Check Endpoint (Fixes 1 test) 🏥
**Issue**: `/healthz` endpoint returns 404.

**Root Cause**: Health check endpoint not registered in `server.go`.

**Fix**:
```go
// In pkg/gateway/server.go:setupRoutes()

func (s *Server) setupRoutes() {
    // ... existing routes ...

    // Add health check endpoint
    s.router.HandleFunc("/healthz", s.handleHealthCheck).Methods("GET")
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
    // Check critical dependencies
    ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()

    healthy := true
    checks := make(map[string]string)

    // Redis check
    if err := s.redisClient.Ping(ctx).Err(); err != nil {
        healthy = false
        checks["redis"] = fmt.Sprintf("unhealthy: %v", err)
    } else {
        checks["redis"] = "healthy"
    }

    // Kubernetes API check
    if _, err := s.k8sClient.RESTMapper().RESTMappings(schema.GroupKind{Group: "remediation.kubernaut.ai", Kind: "RemediationRequest"}); err != nil {
        healthy = false
        checks["kubernetes"] = fmt.Sprintf("unhealthy: %v", err)
    } else {
        checks["kubernetes"] = "healthy"
    }

    status := "healthy"
    statusCode := http.StatusOK
    if !healthy {
        status = "degraded"
        statusCode = http.StatusServiceUnavailable
    }

    response := map[string]interface{}{
        "status": status,
        "checks": checks,
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(response)
}
```

**Files to Modify**:
- `pkg/gateway/server.go` (add endpoint + handler)

**Tests Fixed**: 1
- "Gateway should remain operational when Redis is unavailable"

---

### Phase 3: Redis Failure Handling in Tests (Fixes 19 tests) 🔧
**Issue**: `BeforeEach` blocks fail with "dial tcp [::1]:9999: connect: connection refused"

**Root Cause**: Tests configure Gateway to use Redis on port 9999 for failure simulation, but Gateway attempts connection during startup, causing `BeforeEach` to fail before test body runs.

**Fix Strategy**: Lazy Redis connection initialization + graceful degradation

**Option 3A (Recommended): Lazy Connection Pattern**
```go
// In pkg/gateway/processing/deduplicator.go, storm_detector.go, storm_aggregator.go

type Deduplicator struct {
    redisClient  *redis.Client
    config       DeduplicationConfig
    connected    atomic.Bool  // Track connection state
    connCheckMu  sync.Mutex
}

// Add connection health check method
func (d *Deduplicator) ensureConnection(ctx context.Context) error {
    // Fast path: already connected
    if d.connected.Load() {
        return nil
    }

    d.connCheckMu.Lock()
    defer d.connCheckMu.Unlock()

    // Double-check after acquiring lock
    if d.connected.Load() {
        return nil
    }

    // Try to connect
    if err := d.redisClient.Ping(ctx).Err(); err != nil {
        return fmt.Errorf("redis unavailable: %w", err)
    }

    d.connected.Store(true)
    return nil
}

// Modify all Redis operations to check connection first
func (d *Deduplicator) IsDuplicate(ctx context.Context, fingerprint string) (bool, error) {
    if err := d.ensureConnection(ctx); err != nil {
        // Graceful degradation: treat as non-duplicate
        logrus.WithError(err).Warn("Redis unavailable, treating alert as non-duplicate")
        return false, nil // ✅ Allow alert through
    }

    // ... existing Redis logic ...
}
```

**Files to Modify**:
- `pkg/gateway/processing/deduplicator.go`
- `pkg/gateway/processing/storm_detector.go`
- `pkg/gateway/processing/storm_aggregator.go`

**Tests Fixed**: 19
- All Redis failure scenario tests in Phase 1 & 2

---

## Implementation Order

### Step 1: Fix Storm Aggregation Response (30 min) 🚨
**Why First**: Highest impact (11 tests), simplest fix (2-line change in 2 locations)

```bash
# Edit pkg/gateway/server.go
# Search for: s.stormAggregator.AddResource
# Change response to StatusAccepted
# Search for: s.stormAggregator.StartAggregation
# Change response to StatusAccepted
```

**Verification**:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/... -run "storm aggregation" -timeout 5m
```

### Step 2: Add Health Check Endpoint (20 min) 🏥
**Why Second**: Medium impact (1 test), low complexity, clear requirement

```bash
# Edit pkg/gateway/server.go
# Add /healthz endpoint to setupRoutes()
# Implement handleHealthCheck method
```

**Verification**:
```bash
go test -v ./test/integration/gateway/... -run "Redis is unavailable" -timeout 5m
```

### Step 3: Implement Lazy Redis Connection (60 min) 🔧
**Why Last**: Highest complexity, requires changes across 3 files, affects multiple tests

```bash
# Edit pkg/gateway/processing/deduplicator.go
# Edit pkg/gateway/processing/storm_detector.go
# Edit pkg/gateway/processing/storm_aggregator.go
# Add ensureConnection() method to each
# Update all Redis operations to check connection first
```

**Verification**:
```bash
go test -v ./test/integration/gateway/... -timeout 10m
```

---

## Success Criteria
- ✅ All 47 integration tests pass
- ✅ No timeout failures
- ✅ Storm aggregation returns "accepted" status
- ✅ Health check endpoint returns 200/503 appropriately
- ✅ Gateway gracefully handles Redis failures (no crashes)

---

## Confidence Assessment: 95%

**Why 95%**:
- ✅ All 3 issues are clearly identified with specific code locations
- ✅ Fixes are straightforward with minimal risk of regression
- ✅ Storm response fix is a 2-line change (highest confidence)
- ✅ Health check is a standard pattern (high confidence)
- ✅ Lazy Redis connection is well-tested pattern (high confidence)

**5% Risk**:
- Potential edge cases in Redis reconnection logic
- Possible race conditions in lazy connection initialization

**Mitigation**:
- Use atomic.Bool for connection state
- Comprehensive integration test coverage already in place

---

## Time Estimate
- Phase 1: 30 minutes
- Phase 2: 20 minutes
- Phase 3: 60 minutes
- **Total: 110 minutes (1h 50min)**

---

## Business Requirements Coverage
After fixes, all Gateway BRs will be fully tested:
- BR-GATEWAY-016: Storm Aggregation ✅ (11 tests)
- BR-GATEWAY-013: Graceful Degradation ✅ (19 tests)
- BR-GATEWAY-006: Health Monitoring ✅ (1 test)

**Result**: 100% Gateway BR coverage with production-grade test suite.

