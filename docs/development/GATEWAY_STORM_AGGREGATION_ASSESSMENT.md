# Gateway Storm Detection Aggregation - Implementation Assessment

**Date**: October 10, 2025
**Issue**: Storm detection works, but aggregation is not implemented
**Business Requirement**: BR-GATEWAY-016 - Storm aggregation
**Status**: 🔴 Missing Implementation

---

## Executive Summary

**Problem**: Gateway detects storms correctly but does NOT aggregate them into a single RemediationRequest CRD.

**Current Behavior**:
- Storm detected: 50 pod crashes in 1 minute
- Gateway creates: **50 separate CRDs** (each marked with `isStorm=true`)
- Result: Overwhelms downstream services with 50 individual remediation workflows

**Expected Behavior** (BR-GATEWAY-016):
- Storm detected: 50 pod crashes in 1 minute
- Gateway creates: **1 aggregated CRD** with all 50 resources listed
- Result: Single coordinated remediation workflow

**Impact**: High - defeats the purpose of storm detection

---

## Current Implementation Analysis

### What Works ✅

**Storm Detection** (BR-GATEWAY-015): Fully functional
```go
// pkg/gateway/processing/storm_detection.go
func (d *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error)
```

**Detection methods**:
1. ✅ Rate-based: >10 alerts/minute for same alertname
2. ✅ Pattern-based: >5 similar alerts across different resources in 2 minutes

**Storm metadata captured**:
- ✅ `stormType` (rate vs pattern)
- ✅ `alertCount` (number of alerts in storm)
- ✅ `window` (1m for rate, 2m for pattern)
- ✅ `affectedResources` (list of resource IDs for pattern storms)

### What's Missing ❌

**Storm Aggregation** (BR-GATEWAY-016): NOT implemented

**Current flow** (server.go:499-521):
```go
// 2. Storm detection
isStorm, stormMetadata, err := s.stormDetector.Check(ctx, signal)
if err != nil {
    s.logger.Warn("Storm detection failed")
} else if isStorm && stormMetadata != nil {
    // ❌ PROBLEM: Just enriches the signal but continues to create individual CRD
    signal.IsStorm = true
    signal.StormType = stormMetadata.StormType
    signal.StormWindow = stormMetadata.Window
    signal.AlertCount = stormMetadata.AlertCount
}

// 3. Environment classification
environment := s.classifier.Classify(ctx, signal.Namespace)

// 4. Priority assignment
priority := s.priorityEngine.Assign(ctx, signal.Severity, environment)

// 5. Create RemediationRequest CRD
// ❌ PROBLEM: Creates CRD for EVERY alert, even during storms
crd, err := s.crdCreator.Create(ctx, signal, environment, priority)
```

**Result**: Each alert still creates its own CRD during a storm.

---

## Business Impact

### Scenario: 50 Pod Crashes in 1 Minute

**Current Behavior** (Broken):
```
Storm Detected: true
CRDs Created: 50

Result:
- 50 RemediationRequest CRDs in Kubernetes
- 50 separate AIAnalysis workflows triggered
- 50 independent WorkflowExecution processes
- 50 parallel remediation attempts
- Gateway, AIAnalysis, and WorkflowExecution overwhelmed
```

**Expected Behavior** (BR-GATEWAY-016):
```
Storm Detected: true
CRDs Created: 1 (aggregated)

Result:
- 1 RemediationRequest CRD with 50 resources listed
- 1 AIAnalysis workflow (analyzes all 50 resources together)
- 1 coordinated WorkflowExecution
- 1 smart remediation strategy (fix root cause, not 50 individual issues)
- Efficient resource usage, better AI decisions
```

### Why Aggregation Matters

**Without aggregation**:
- ❌ Overwhelms Kubernetes API with 50 CRD creates
- ❌ Overwhelms AIAnalysis service with 50 parallel requests
- ❌ AI makes 50 individual decisions (misses root cause)
- ❌ 50 separate remediations (may conflict with each other)
- ❌ Wasted compute resources

**With aggregation**:
- ✅ Single CRD create (minimal K8s API load)
- ✅ Single AI analysis (understands the full picture)
- ✅ AI identifies root cause (e.g., "deployment config issue" not "50 pod crashes")
- ✅ Coordinated remediation (fix once, not 50 times)
- ✅ Efficient resource usage

---

## Implementation Options

### Option 1: Storm Aggregation Window (Recommended)

**Approach**: When storm detected, aggregate alerts for a fixed time window (e.g., 1 minute)

**Algorithm**:
```go
// 1. First alert in storm creates aggregation entry in Redis
//    Key: alert:storm:aggregate:<alertname>
//    Value: <CRD-ID>
//    TTL: 1 minute

// 2. Subsequent alerts in storm:
//    - Check if aggregation entry exists
//    - If yes: Add resource to existing CRD's aggregated list (Redis ZADD)
//    - If no: Storm window expired, create new aggregated CRD

// 3. After 1 minute:
//    - Aggregation window closes
//    - Create RemediationRequest CRD with all aggregated resources
//    - Clear Redis aggregation entry
```

**Pros**:
- ✅ Simple time-based window (predictable behavior)
- ✅ Automatically closes after 1 minute
- ✅ Works for both rate-based and pattern-based storms
- ✅ No complex state management

**Cons**:
- ⚠️ Slight delay (up to 1 minute) before CRD creation
- ⚠️ If storm continues past window, creates multiple aggregated CRDs

**Confidence**: 90% (Very High)

---

### Option 2: Dynamic Storm Aggregation

**Approach**: Keep aggregation window open as long as storm continues

**Algorithm**:
```go
// 1. Storm detected: Create aggregation window
// 2. Keep window open while alerts keep arriving
// 3. Close window when alert rate drops below threshold
// 4. Create aggregated CRD when window closes
```

**Pros**:
- ✅ More adaptive (handles long-lived storms)
- ✅ Creates fewer CRDs for extended storms

**Cons**:
- ❌ Complex state management (when to close?)
- ❌ Unpredictable CRD creation timing
- ❌ Could delay remediation if storm never "ends"

**Confidence**: 65% (Medium) - Too complex for V1

---

### Option 3: Hybrid Approach

**Approach**: Combine Options 1 and 2

**Algorithm**:
```go
// 1. First storm alert: Start 1-minute aggregation window
// 2. If storm continues: Extend window (max 5 minutes)
// 3. Create CRD when:
//    - Window expires (1 minute), OR
//    - Max window reached (5 minutes), OR
//    - Storm ends (no new alerts for 30 seconds)
```

**Pros**:
- ✅ Balances simplicity and adaptability
- ✅ Handles both short and long storms

**Cons**:
- ⚠️ More complex than Option 1
- ⚠️ More configuration parameters

**Confidence**: 75% (High) - Good for V2, overkill for V1

---

## Recommended Approach: Option 1 (Fixed Window)

**Implementation Plan**:

### Phase 1: Add Storm Aggregation Logic (2-3 hours)

**File**: `pkg/gateway/processing/storm_aggregator.go` (new)

```go
package processing

import (
    "context"
    "fmt"
    "time"

    "github.com/go-redis/redis/v8"
    "github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// StormAggregator aggregates alerts during storm windows
//
// When a storm is detected, instead of creating individual RemediationRequest
// CRDs for each alert, StormAggregator collects alerts for a fixed window
// (1 minute) and creates a single aggregated CRD.
//
// Redis keys:
// - alert:storm:aggregate:<alertname> (stores aggregation window ID)
// - alert:storm:resources:<window-id> (sorted set of affected resources)
type StormAggregator struct {
    redisClient *redis.Client
    windowDuration time.Duration // Default: 1 minute
}

// NewStormAggregator creates a new storm aggregator
func NewStormAggregator(redisClient *redis.Client) *StormAggregator {
    return &StormAggregator{
        redisClient: redisClient,
        windowDuration: 1 * time.Minute,
    }
}

// ShouldAggregate checks if alert should be aggregated
//
// Returns:
// - bool: true if aggregation window exists
// - string: window ID (for adding resources)
// - error: Redis errors
func (a *StormAggregator) ShouldAggregate(ctx context.Context, signal *types.NormalizedSignal) (bool, string, error) {
    key := fmt.Sprintf("alert:storm:aggregate:%s", signal.AlertName)

    windowID, err := a.redisClient.Get(ctx, key).Result()
    if err == redis.Nil {
        // No aggregation window exists
        return false, "", nil
    } else if err != nil {
        return false, "", fmt.Errorf("failed to check aggregation window: %w", err)
    }

    return true, windowID, nil
}

// StartAggregation creates a new aggregation window
//
// Returns window ID (timestamp-based unique identifier)
func (a *StormAggregator) StartAggregation(ctx context.Context, signal *types.NormalizedSignal) (string, error) {
    windowID := fmt.Sprintf("%s-%d", signal.AlertName, time.Now().Unix())
    key := fmt.Sprintf("alert:storm:aggregate:%s", signal.AlertName)

    // Store window ID with TTL
    if err := a.redisClient.Set(ctx, key, windowID, a.windowDuration).Err(); err != nil {
        return "", fmt.Errorf("failed to start aggregation window: %w", err)
    }

    // Add first resource
    if err := a.AddResource(ctx, windowID, signal); err != nil {
        return "", err
    }

    return windowID, nil
}

// AddResource adds a resource to the aggregation window
func (a *StormAggregator) AddResource(ctx context.Context, windowID string, signal *types.NormalizedSignal) error {
    key := fmt.Sprintf("alert:storm:resources:%s", windowID)
    resourceID := signal.Resource.String()

    // Add to sorted set (score = timestamp)
    if err := a.redisClient.ZAdd(ctx, key, &redis.Z{
        Score:  float64(time.Now().Unix()),
        Member: resourceID,
    }).Err(); err != nil {
        return fmt.Errorf("failed to add resource to aggregation: %w", err)
    }

    // Set TTL (2 minutes to allow retrieval after window closes)
    a.redisClient.Expire(ctx, key, 2*time.Minute)

    return nil
}

// GetAggregatedResources retrieves all resources in the window
func (a *StormAggregator) GetAggregatedResources(ctx context.Context, windowID string) ([]string, error) {
    key := fmt.Sprintf("alert:storm:resources:%s", windowID)

    resources, err := a.redisClient.ZRange(ctx, key, 0, -1).Result()
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve aggregated resources: %w", err)
    }

    return resources, nil
}
```

---

### Phase 2: Modify Server Flow (1-2 hours)

**File**: `pkg/gateway/server.go` (modify)

```go
// Current: processSignal() method

// Add storm aggregator
s.stormAggregator = processing.NewStormAggregator(s.redisClient)

// Modified flow:
func (s *Server) processSignal(ctx context.Context, signal *types.NormalizedSignal) (*SignalResponse, error) {
    // 1. Deduplication (unchanged)
    isDuplicate, metadata, err := s.deduplicationService.CheckDuplicate(ctx, signal)
    if isDuplicate {
        return &SignalResponse{Status: StatusDuplicate, ...}, nil
    }

    // 2. Storm detection (unchanged)
    isStorm, stormMetadata, err := s.stormDetector.Check(ctx, signal)
    if err != nil {
        s.logger.Warn("Storm detection failed")
    }

    // 3. NEW: Storm aggregation logic
    if isStorm && stormMetadata != nil {
        // Check if aggregation window exists
        shouldAggregate, windowID, err := s.stormAggregator.ShouldAggregate(ctx, signal)
        if err != nil {
            s.logger.Warn("Storm aggregation check failed")
            // Fall back to individual CRD creation
        } else if shouldAggregate {
            // Add to existing aggregation window
            if err := s.stormAggregator.AddResource(ctx, windowID, signal); err != nil {
                s.logger.Warn("Failed to add resource to storm aggregation")
            } else {
                // Return success without creating CRD
                return &SignalResponse{
                    Status:      StatusAccepted,
                    Message:     "Alert added to storm aggregation window",
                    Fingerprint: signal.Fingerprint,
                    IsStorm:     true,
                    StormType:   stormMetadata.StormType,
                    WindowID:    windowID,
                }, nil
            }
        } else {
            // Start new aggregation window
            windowID, err := s.stormAggregator.StartAggregation(ctx, signal)
            if err != nil {
                s.logger.Warn("Failed to start storm aggregation")
                // Fall back to individual CRD creation
            } else {
                // Schedule aggregated CRD creation after window expires
                go s.createAggregatedCRDAfterWindow(ctx, windowID, signal, stormMetadata)

                return &SignalResponse{
                    Status:      StatusAccepted,
                    Message:     "Storm aggregation window started",
                    Fingerprint: signal.Fingerprint,
                    IsStorm:     true,
                    StormType:   stormMetadata.StormType,
                    WindowID:    windowID,
                }, nil
            }
        }
    }

    // 4. Environment classification (unchanged)
    // 5. Priority assignment (unchanged)
    // 6. Create RemediationRequest CRD (unchanged)
    // ... rest of flow
}

// NEW: Create aggregated CRD after window expires
func (s *Server) createAggregatedCRDAfterWindow(
    ctx context.Context,
    windowID string,
    firstSignal *types.NormalizedSignal,
    stormMetadata *processing.StormMetadata,
) {
    // Wait for window to expire
    time.Sleep(s.stormAggregator.windowDuration)

    // Retrieve all aggregated resources
    resources, err := s.stormAggregator.GetAggregatedResources(ctx, windowID)
    if err != nil {
        s.logger.WithError(err).Error("Failed to retrieve aggregated resources")
        return
    }

    s.logger.WithFields(logrus.Fields{
        "windowID":      windowID,
        "alertName":     firstSignal.AlertName,
        "resourceCount": len(resources),
    }).Info("Creating aggregated RemediationRequest CRD for storm")

    // Create aggregated signal
    aggregatedSignal := *firstSignal
    aggregatedSignal.IsStorm = true
    aggregatedSignal.StormType = stormMetadata.StormType
    aggregatedSignal.AlertCount = len(resources)
    aggregatedSignal.AffectedResources = resources

    // Environment classification
    environment := s.classifier.Classify(ctx, aggregatedSignal.Namespace)

    // Priority assignment
    priority := s.priorityEngine.Assign(ctx, aggregatedSignal.Severity, environment)

    // Create single aggregated CRD
    crd, err := s.crdCreator.Create(ctx, &aggregatedSignal, environment, priority)
    if err != nil {
        s.logger.WithError(err).Error("Failed to create aggregated RemediationRequest CRD")
        return
    }

    s.logger.WithFields(logrus.Fields{
        "crdName":       crd.Name,
        "resourceCount": len(resources),
    }).Info("Aggregated RemediationRequest CRD created successfully")
}
```

---

### Phase 3: Update Tests (1 hour)

**File**: `test/unit/gateway/storm_detection_test.go` (modify)

Add new test for aggregation:

```go
Context("when storm aggregation is enabled", func() {
    It("aggregates multiple alerts into a single CRD", func() {
        // Send 15 alerts in 1 minute (exceeds rate threshold)
        for i := 0; i < 15; i++ {
            response := sendAlert(fmt.Sprintf("pod-%d", i))
            if i == 0 {
                // First alert: window started
                Expect(response.Status).To(Equal("accepted"))
                Expect(response.Message).To(ContainSubstring("Storm aggregation window started"))
            } else {
                // Subsequent alerts: added to window
                Expect(response.Status).To(Equal("accepted"))
                Expect(response.Message).To(ContainSubstring("Alert added to storm aggregation window"))
            }
        }

        // Wait for window to expire
        time.Sleep(65 * time.Second)

        // Verify only 1 CRD was created
        crds := listRemediationRequests()
        Expect(crds).To(HaveLen(1))

        // Verify CRD contains all 15 resources
        crd := crds[0]
        Expect(crd.Spec.IsStorm).To(BeTrue())
        Expect(crd.Spec.AffectedResources).To(HaveLen(15))
    })
})
```

---

## Implementation Timeline

**Total Effort**: 4-6 hours

| Phase | Task | Effort | Files |
|-------|------|--------|-------|
| **Phase 1** | Create StormAggregator | 2-3 hours | `storm_aggregator.go` (new, ~150 lines) |
| **Phase 2** | Modify Server Flow | 1-2 hours | `server.go` (modify, +80 lines) |
| **Phase 3** | Update Tests | 1 hour | `storm_detection_test.go` (modify, +40 lines) |

---

## Risk Assessment

### Low Risk ⚡
- Storm detection already works (just adding aggregation logic)
- Redis operations similar to existing deduplication code
- Clear fallback strategy (if aggregation fails, create individual CRD)

### Testing Strategy
1. Unit tests: Storm aggregation window logic
2. Integration tests: Redis persistence, CRD creation timing
3. Manual testing: Send 50 alerts, verify single CRD created

---

## Success Criteria

### Functional Requirements ✅
- [ ] When 15 alerts detected in 1 minute → 1 aggregated CRD created
- [ ] Aggregated CRD contains all 15 resource IDs
- [ ] CRD created after 1-minute window expires
- [ ] If aggregation fails, falls back to individual CRD creation

### Business Requirements ✅
- [ ] BR-GATEWAY-016: Storm aggregation implemented
- [ ] Reduces CRD count by 10-50x during storms
- [ ] AI service receives single aggregated analysis request
- [ ] Downstream remediation is coordinated, not parallel

### Performance Requirements ✅
- [ ] Redis aggregation operations < 10ms
- [ ] CRD creation latency +1 minute (acceptable for storms)
- [ ] No impact on non-storm alerts (immediate CRD creation)

---

## Recommendation

**Proceed with Option 1**: Fixed 1-minute aggregation window

**Rationale**:
1. ✅ Simple, predictable implementation
2. ✅ Low risk (clear fallback strategy)
3. ✅ Solves 90% of storm scenarios
4. ✅ Can enhance with Option 3 (hybrid) in V2 if needed

**Confidence**: 90% (Very High)

**Next Step**: Get user approval and implement Phase 1 (StormAggregator)

---

**Assessment Status**: ✅ Complete
**Implementation Ready**: Yes (pending approval)
**Estimated Completion**: 4-6 hours

