# DataStorage - DLQ Drain During Graceful Shutdown (V1.1 Enhancement)
**Date**: December 20, 2025
**Status**: 📋 **DOCUMENTED FOR V1.1**
**Priority**: P2 - Enhancement (No data loss, but delayed retry)
**Related**: DD-007 (Kubernetes-Aware Graceful Shutdown)

---

## 🎯 Enhancement Request

**Add time-boxed DLQ drain to graceful shutdown sequence**

---

## 📋 Current Behavior (V1.0)

**Graceful Shutdown Sequence** (DD-007):
1. Set shutdown flag → Readiness returns 503
2. Wait 5s for K8s endpoint removal propagation
3. Drain HTTP connections (up to 30s)
4. Close resources:
   - ✅ **Flush audit traces buffer** (DD-STORAGE-012) - COMPLETE
   - ❌ **DLQ drain** - NOT IMPLEMENTED
   - ✅ **Close DB connections** - COMPLETE

**Impact**:
- DLQ messages (failed audit writes) are NOT retried during shutdown
- Messages remain in Redis DLQ (persistent, no data loss)
- **Next pod** will pick up and retry these messages
- **Delay**: Could be minutes before retry (depends on pod scheduling)

---

## ✅ Proposed Enhancement (V1.1)

### Enhanced Shutdown Sequence

```
Step 1: Set shutdown flag (immediate)
Step 2: Wait for K8s endpoint removal (5s)
Step 3: Drain HTTP connections (up to 30s)
Step 4: Close resources with PRIORITY:
  ├─ 4a: Flush audit traces buffer (MANDATORY, unbounded)
  ├─ 4b: Drain DLQ (BEST-EFFORT, max 10s) ← NEW
  └─ 4c: Close DB connections (final cleanup)
```

### Time Budget Rationale

**Kubernetes Grace Period**: 30s default
- Endpoint propagation: 5s (fixed)
- Connection draining: Variable (depends on in-flight requests)
- **Audit buffer flush**: MUST complete (business requirement BR-STORAGE-014)
- **DLQ drain**: Best-effort, max 10s (enhancement)
- Final cleanup: ~1-2s buffer

**Priority Order**:
1. **Audit traces buffer** (highest priority):
   - In-memory data
   - Would be lost if not flushed
   - Business requirement BR-STORAGE-014

2. **DLQ messages** (best-effort):
   - Already persistent in Redis
   - No data loss risk
   - Improves retry latency

3. **DB connections** (final cleanup):
   - Safe to close anytime
   - No data at risk

---

## 💻 Implementation Approach

### 1. DLQ Client Enhancement

**New Method**: `DrainWithTimeout(ctx context.Context) (*DrainStats, error)`

```go
// pkg/datastorage/dlq/client.go

type DrainStats struct {
    Processed int64 // Messages successfully retried
    Remaining int64 // Messages still in DLQ
    Errors    int64 // Messages that failed retry
}

// DrainWithTimeout attempts to process all DLQ messages within the timeout.
// Returns stats showing how many were processed vs. remaining.
// This is best-effort - timeout is acceptable (messages persist in Redis).
func (c *Client) DrainWithTimeout(ctx context.Context) (*DrainStats, error) {
    stats := &DrainStats{}

    // Get depths for both audit types
    eventDepth, _ := c.GetDLQDepth(ctx, "events")
    notifDepth, _ := c.GetDLQDepth(ctx, "notifications")
    stats.Remaining = eventDepth + notifDepth

    if stats.Remaining == 0 {
        return stats, nil // Nothing to drain
    }

    // Process messages until timeout or queue empty
    for {
        select {
        case <-ctx.Done():
            return stats, ctx.Err() // Timeout - return progress
        default:
            // Try to process one message from each queue
            // Increment stats.Processed or stats.Errors
            // If both queues empty, return success
        }
    }
}
```

### 2. Server Shutdown Enhancement

**File**: `pkg/datastorage/server/server.go`

```go
func (s *Server) shutdownStep4CloseResources(ctx context.Context) error {
    s.logger.Info("Closing external resources (PostgreSQL, audit store, DLQ)",
        "dd", "DD-007-step-4")

    // 4a. PRIORITY: Flush audit traces buffer (DD-STORAGE-012)
    // MANDATORY - audit traces would be lost otherwise
    if s.auditStore != nil {
        s.logger.Info("Flushing audit events (DD-STORAGE-012)",
            "dd", "DD-007-step-4-audit-flush")
        if err := s.auditStore.Close(); err != nil {
            s.logger.Error(err, "Failed to flush audit events")
            // Continue - audit failures should not block shutdown
        }
    }

    // 4b. BEST-EFFORT: Drain DLQ with timeout (V1.1 ENHANCEMENT)
    if s.dlqClient != nil {
        // Calculate remaining time, cap at 10 seconds
        deadline, hasDeadline := ctx.Deadline()
        var drainTimeout time.Duration

        if hasDeadline {
            remaining := time.Until(deadline)
            drainTimeout = min(remaining-2*time.Second, 10*time.Second)
        } else {
            drainTimeout = 10 * time.Second
        }

        if drainTimeout > 0 {
            s.logger.Info("Draining DLQ with timeout",
                "timeout", drainTimeout,
                "dd", "DD-007-step-4-dlq-drain")

            drainCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
            defer cancel()

            stats, err := s.dlqClient.DrainWithTimeout(drainCtx)
            if err != nil {
                s.logger.Warn("DLQ drain timeout",
                    "processed", stats.Processed,
                    "remaining", stats.Remaining)
            } else {
                s.logger.Info("DLQ drained",
                    "processed", stats.Processed)
            }
        }
    }

    // 4c. FINAL: Close database connection
    if s.db != nil {
        if err := s.db.Close(); err != nil {
            return err
        }
    }

    return nil
}
```

### 3. Testing Requirements

**New Test**: `graceful_shutdown_test.go`

```go
It("should drain DLQ messages during shutdown with timeout", func() {
    // ARRANGE: Seed DLQ with messages
    testServer := createTestServer(...)
    seedDLQWithMessages(dlqClient, 50) // 50 failed messages

    // ACT: Trigger graceful shutdown
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    go testServer.Shutdown(shutdownCtx)

    // ASSERT:
    // - DLQ drain attempted
    // - Some messages processed (best-effort)
    // - Shutdown completes within grace period
    Eventually(func() bool {
        return testServer.IsShutdown()
    }, "30s", "100ms").Should(BeTrue())

    // Check DLQ was drained (at least partially)
    depth, _ := dlqClient.GetDLQDepth(context.Background(), "events")
    Expect(depth).To(BeNumerically("<", 50),
        "DLQ should have processed some messages")
})
```

---

## 📊 Benefits

| Aspect | Current (V1.0) | Enhanced (V1.1) |
|--------|----------------|-----------------|
| **Data Loss** | ✅ None | ✅ None |
| **Retry Latency** | ⚠️ Minutes (next pod) | ✅ Immediate (during shutdown) |
| **Shutdown Time** | ✅ Predictable | ✅ Still predictable (10s cap) |
| **In-Flight Retries** | ❌ Interrupted | ✅ Completed (if within timeout) |
| **User Experience** | ⚠️ Delayed audit visibility | ✅ Faster audit availability |

---

## ⚠️ V1.0 Known Limitation

**Current Behavior**:
- DLQ messages are NOT retried during graceful shutdown
- Messages remain in Redis (persistent)
- Next pod will pick up and retry

**Impact**:
- **Data Loss**: ❌ None (DLQ is persistent)
- **Retry Delay**: ⚠️ Could be minutes until next pod processes
- **User Impact**: ⚠️ Audit events may appear delayed in queries

**Mitigation**:
- DLQ is persistent (no data loss)
- Retry will happen on next pod startup
- Consider reducing pod startup time for faster DLQ processing

---

## 🎯 V1.1 Implementation Tasks

1. **Add `DrainWithTimeout()` to DLQ client** (~2 hours)
   - Implement drain loop with context cancellation
   - Return statistics (processed/remaining/errors)
   - Handle both "events" and "notifications" queues

2. **Enhance graceful shutdown sequence** (~1 hour)
   - Add DLQ drain between audit flush and DB close
   - Calculate remaining time with 10s cap
   - Log drain statistics

3. **Add graceful shutdown test** (~1 hour)
   - Seed DLQ with messages
   - Trigger shutdown
   - Verify drain attempted and some messages processed

4. **Update DD-007 documentation** (~30 minutes)
   - Document 4-step sequence enhancement
   - Add DLQ drain timing details
   - Update shutdown flow diagram

**Total Effort**: ~4.5 hours

---

## 📚 Related Documents

- **DD-007**: Kubernetes-Aware Graceful Shutdown (4-step pattern)
- **DD-009**: Dead Letter Queue Pattern (Audit Write Error Recovery)
- **BR-STORAGE-014**: Flush remaining audit events before shutdown
- **`pkg/datastorage/dlq/client.go`**: DLQ client implementation
- **`pkg/datastorage/server/server.go`**: Graceful shutdown implementation

---

## ✅ Acceptance Criteria (V1.1)

- [ ] `DrainWithTimeout()` method implemented in DLQ client
- [ ] Graceful shutdown calls DLQ drain with 10s timeout
- [ ] Audit buffer flush still happens BEFORE DLQ drain
- [ ] Timeout is respected (no risk of exceeding K8s grace period)
- [ ] Statistics logged (processed/remaining/errors)
- [ ] New test covers DLQ drain during shutdown
- [ ] DD-007 documentation updated
- [ ] No regression in existing graceful shutdown tests

---

**Status**: 📋 **DOCUMENTED FOR V1.1** (not blocking V1.0 release)
**Decision**: Document limitation, implement in V1.1
**Justification**: No data loss, enhancement improves retry latency but not critical for V1.0


