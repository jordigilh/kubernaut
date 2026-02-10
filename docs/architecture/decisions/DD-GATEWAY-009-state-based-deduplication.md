# DD-GATEWAY-009: State-Based Deduplication Strategy

## Status
**⏸️ PARKED** (2025-11-17) - Deferred until after DD-GATEWAY-008 implementation
**Last Reviewed**: 2025-11-17
**Confidence**: 85% (requires K8s API performance validation)
**Next Action**: Implement after DD-GATEWAY-008 (storm buffering) and LLM validation
**Decision**: Pending user approval after performance validation

**⚠️ IMPORTANT**: This DD addresses CRD collision handling. For CRD naming strategy, see **[DD-015: Timestamp-Based CRD Naming](DD-015-timestamp-based-crd-naming.md)** which provides a simpler solution to the collision problem by making CRD names unique per occurrence.

## Context & Problem

### Current Behavior (Time-Based Deduplication)

The Gateway's deduplication service currently uses a **time-based window** (5-minute TTL in Redis) to prevent duplicate CRDs.

**File**: `pkg/gateway/processing/deduplication.go`

**Current Strategy**:
1. Alert arrives with fingerprint `abc123...`
2. Check Redis key `gateway:dedup:fingerprint:abc123...`
3. If key exists (within 5-minute TTL) → Duplicate detected, return 202 Accepted
4. If key doesn't exist → Create new CRD, store fingerprint in Redis with 5-minute TTL

**Example Timeline**:
- **T=0s**: Alert 1 → CRD `rr-abc123` created, Redis key stored (TTL: 5 min)
- **T=30s**: Alert 2 (same fingerprint) → Duplicate detected, return 202
- **T=6min**: Alert 3 (same fingerprint) → **Redis TTL expired** → Try to create CRD `rr-abc123` again
- **Problem**: CRD `rr-abc123` still exists in Kubernetes (being processed) → **AlreadyExists error**

### The Problem

**Time-based deduplication is incorrect** because:
1. ❌ **Arbitrary TTL**: 5-minute window has no relationship to remediation lifecycle
2. ❌ **CRD name collisions**: Redis TTL expires while CRD still exists in Kubernetes
3. ❌ **No distinction**: Can't tell if CRD is "still processing" vs "completed"
4. ❌ **Incorrect behavior**: Should create NEW CRD after remediation completes, not after 5 minutes

### Current Collision Handling

**File**: `pkg/gateway/processing/crd_creator.go:376-400`

```go
if strings.Contains(err.Error(), "already exists") {
    // Fetch existing CRD and return it
    existing, err := c.k8sClient.GetRemediationRequest(ctx, signal.Namespace, crdName)
    return existing, nil
}
```

**Issues**:
- ❌ Relies on string matching error messages (fragile)
- ❌ Extra K8s API call to fetch existing CRD
- ❌ Doesn't distinguish "CRD still processing" vs "CRD completed"

### User's Insight (Correct Behavior)

> "I would expect that this is the correct behavior, just updating the number of duplications in the CRD spec to keep track of how many times this same fingerprint was generated in the duration of the CRD (while it is still being processed for remediation, once the CRD reaches its final state, it should create a new one)"

**Key Insight**: Deduplication should be **STATE-BASED**, not **TIME-BASED**.

### Key Requirements

1. **Correct duplicate handling**: Same incident → update `occurrenceCount`, not create new CRD
2. **New incident after remediation**: Completed CRD → create NEW CRD for recurring issue
3. **No arbitrary TTL**: Deduplication window = CRD lifecycle, not fixed time
4. **Accurate metrics**: `occurrenceCount` reflects true duplicate rate during remediation
5. **No collision risk**: CRD name reused only after remediation completes

---

## Alternatives Considered

### Alternative 1: Current Behavior (Time-Based with 5-Minute TTL)

**Approach**: Use Redis TTL (5 minutes) to track duplicates, handle collisions with error string matching.

**Implementation**:
```go
// Current code (pkg/gateway/processing/deduplication.go:199-222)
key := fmt.Sprintf("gateway:dedup:fingerprint:%s", signal.Fingerprint)
exists, err := s.redisClient.Exists(ctx, key).Result()
if exists == 0 {
    // First occurrence - not a duplicate
    return false, nil, nil
}
// Duplicate detected
return true, metadata, nil
```

**Pros**:
- ✅ **Simple implementation**: No CRD state queries needed
- ✅ **Fast**: Redis query (~1ms P95) faster than K8s API (~5-10ms)
- ✅ **Existing code**: Already implemented and tested

**Cons**:
- ❌ **Arbitrary TTL**: 5-minute window has no relationship to remediation lifecycle
- ❌ **CRD collisions**: Redis TTL expires while CRD still exists
- ❌ **Incorrect behavior**: Creates new CRD after 5 minutes, even if remediation still in progress
- ❌ **No distinction**: Can't tell if incident is resolved or still ongoing

**Confidence**: 40% (current implementation, but incorrect behavior)

---

### Alternative 2: State-Based Deduplication (CRD Lifecycle)

**Approach**: Check CRD state in Kubernetes to determine if duplicate. Update `occurrenceCount` if CRD is being processed, create new CRD if completed.

**Implementation**:
```go
// Proposed: pkg/gateway/processing/deduplication.go
func (s *DeduplicationService) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *DeduplicationMetadata, error) {
    // Generate CRD name from fingerprint (same as current)
    fingerprintPrefix := signal.Fingerprint
    if len(fingerprintPrefix) > 16 {
        fingerprintPrefix = fingerprintPrefix[:16]
    }
    crdName := fmt.Sprintf("rr-%s", fingerprintPrefix)

    // Check if CRD exists in Kubernetes
    existingCRD, err := s.k8sClient.GetRemediationRequest(ctx, signal.Namespace, crdName)
    if err != nil {
        if k8serrors.IsNotFound(err) {
            // CRD doesn't exist → not a duplicate
            return false, nil, nil
        }
        // K8s API error → fall back to Redis check (graceful degradation)
        return s.checkRedisDeduplication(ctx, signal)
    }

    // CRD exists - check state
    switch existingCRD.Status.Phase {
    case "Pending", "Processing":
        // CRD is being processed → this is a duplicate
        return true, &DeduplicationMetadata{
            Fingerprint:           signal.Fingerprint,
            Count:                 existingCRD.Spec.Deduplication.OccurrenceCount + 1,
            RemediationRequestRef: fmt.Sprintf("%s/%s", existingCRD.Namespace, existingCRD.Name),
            FirstSeen:             existingCRD.Spec.Deduplication.FirstSeen.Format(time.RFC3339),
            LastSeen:              time.Now().Format(time.RFC3339),
        }, nil

    case "Completed", "Failed", "Cancelled":
        // CRD is in final state → treat as NEW incident
        // This allows a new remediation attempt for recurring issues
        return false, nil, nil

    default:
        // Unknown state → treat as new (fail safe)
        return false, nil, nil
    }
}
```

**CRD State Lifecycle**:
```
┌─────────────────────────────────────────────────────────────┐
│  RemediationRequest CRD States                              │
├─────────────────────────────────────────────────────────────┤
│  Pending       → Duplicate: Update occurrenceCount          │
│  Processing    → Duplicate: Update occurrenceCount          │
│  Completed     → NEW: Create new CRD (issue resolved)       │
│  Failed        → NEW: Create new CRD (retry remediation)    │
│  Cancelled     → NEW: Create new CRD (retry remediation)    │
└─────────────────────────────────────────────────────────────┘
```

**Update CRD on Duplicate**:
```go
// Proposed: pkg/gateway/server.go
if isDuplicate && metadata != nil {
    // Update existing CRD's occurrenceCount
    err := s.crdUpdater.IncrementOccurrenceCount(ctx, metadata.RemediationRequestRef)
    if err != nil {
        logger.Warn("Failed to update occurrence count", zap.Error(err))
    }

    return NewDuplicateResponse(signal.Fingerprint, metadata), nil
}
```

**New CRD Updater**:
```go
// Proposed: pkg/gateway/processing/crd_updater.go (NEW FILE)
type CRDUpdater struct {
    k8sClient *k8s.Client
    logger    *zap.Logger
}

func (u *CRDUpdater) IncrementOccurrenceCount(ctx context.Context, crdRef string) error {
    namespace, name := parseCRDRef(crdRef) // "namespace/name" → namespace, name

    // Fetch current CRD
    crd, err := u.k8sClient.GetRemediationRequest(ctx, namespace, name)
    if err != nil {
        return fmt.Errorf("failed to fetch CRD: %w", err)
    }

    // Increment occurrence count
    crd.Spec.Deduplication.OccurrenceCount++
    crd.Spec.Deduplication.LastSeen = metav1.Now()

    // Update CRD in Kubernetes
    if err := u.k8sClient.Update(ctx, crd); err != nil {
        return fmt.Errorf("failed to update CRD: %w", err)
    }

    u.logger.Info("Updated CRD occurrence count",
        zap.String("crd", crdRef),
        zap.Int("count", crd.Spec.Deduplication.OccurrenceCount))

    return nil
}
```

**Pros**:
- ✅ **Correct behavior**: Duplicates during processing → update count, completed → new CRD
- ✅ **No arbitrary TTL**: Deduplication window = CRD lifecycle
- ✅ **No collision risk**: CRD name reused only after completion
- ✅ **Accurate metrics**: `occurrenceCount` reflects true duplicate rate
- ✅ **New CRD after remediation**: Allows retry for recurring issues

**Cons & Mitigations**:
- ❌ **K8s API latency**: CRD state query (~5-10ms) vs Redis (~1ms)
  - **Mitigation 1**: Cache CRD state in Redis with 30-second TTL (reduces K8s API load by 97%)
  - **Mitigation 2**: Use K8s client-go cache (informer) for local CRD state lookup
  - **Mitigation 3**: Async CRD updates (don't block alert processing)
  - **Mitigation 4**: Fall back to Redis time-based check if K8s API unavailable

- ❌ **K8s API load**: Every alert queries CRD state
  - **Mitigation 1**: Redis cache with 30-second TTL (1 K8s query per 30 seconds per fingerprint)
  - **Mitigation 2**: Batch CRD state queries (if multiple alerts arrive simultaneously)
  - **Mitigation 3**: Rate limit CRD state queries (max 100 queries/second)

- ❌ **CRD update conflicts**: Concurrent updates to same CRD
  - **Mitigation 1**: Optimistic concurrency control (K8s resourceVersion)
  - **Mitigation 2**: Retry with exponential backoff on conflict
  - **Mitigation 3**: Atomic increment using K8s patch (not full update)

**Confidence**: 85% (requires K8s API performance validation)

---

### Alternative 3: Hybrid Approach (Redis Cache + CRD State)

**Approach**: Use Redis as cache for CRD state, query K8s only on cache miss or expiration.

**Implementation**:
```go
// Proposed: pkg/gateway/processing/deduplication.go
func (s *DeduplicationService) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *DeduplicationMetadata, error) {
    crdName := fmt.Sprintf("rr-%s", signal.Fingerprint[:16])

    // Step 1: Check Redis cache for CRD state
    cacheKey := fmt.Sprintf("gateway:crd:state:%s:%s", signal.Namespace, crdName)
    cachedState, err := s.redisClient.Get(ctx, cacheKey).Result()

    if err == nil {
        // Cache hit - use cached state
        switch cachedState {
        case "Pending", "Processing":
            return true, s.buildMetadata(signal, crdName), nil
        case "Completed", "Failed", "Cancelled":
            return false, nil, nil
        }
    }

    // Step 2: Cache miss - query K8s for CRD state
    existingCRD, err := s.k8sClient.GetRemediationRequest(ctx, signal.Namespace, crdName)
    if err != nil {
        if k8serrors.IsNotFound(err) {
            return false, nil, nil
        }
        return false, nil, fmt.Errorf("failed to check CRD: %w", err)
    }

    // Step 3: Cache CRD state in Redis (30-second TTL)
    s.redisClient.Set(ctx, cacheKey, existingCRD.Status.Phase, 30*time.Second)

    // Step 4: Return result based on CRD state
    switch existingCRD.Status.Phase {
    case "Pending", "Processing":
        return true, s.buildMetadata(signal, crdName), nil
    default:
        return false, nil, nil
    }
}
```

**Pros**:
- ✅ **Best of both worlds**: Redis speed + CRD state accuracy
- ✅ **Low K8s API load**: 1 query per 30 seconds per fingerprint (97% reduction)
- ✅ **Fast**: Redis cache hit ~1ms (same as current)
- ✅ **Correct behavior**: State-based deduplication

**Cons**:
- ❌ **Cache staleness**: 30-second window where state might be outdated
  - **Mitigation**: Acceptable trade-off (30s delay for state change is negligible)
- ❌ **Increased complexity**: Two-tier caching (Redis + K8s)
  - **Mitigation**: Well-defined cache invalidation strategy

**Confidence**: 90% (best balance of performance and correctness)

---

### Alternative 4: Informer-Based (K8s Client-Go Cache) - **RECOMMENDED**

**Approach**: Use Kubernetes informer pattern with dual-cache strategy:
1. **Informer's built-in cache**: Automatically maintains ALL RemediationRequest CRDs (handles CREATE/UPDATE/DELETE internally)
2. **Custom `inFlightCRDs` map**: Tracks ONLY in-flight CRDs (Pending/Processing) for fast deduplication lookups

**Key Insight from User**:
- Informer maintains full CRD cache automatically → We don't manage informer's cache
- We need custom cache for ONLY in-flight CRDs → Use event handlers to filter
- Watch CREATE/UPDATE/DELETE events → Maintain our `inFlightCRDs` map
- Remove from `inFlightCRDs` when CRD completes or is deleted → Automatic cleanup
- No Redis needed → Informer + custom map is sufficient

**Implementation**:
```go
// Proposed: pkg/gateway/processing/deduplication_informer.go (NEW FILE)
package processing

import (
    "context"
    "fmt"
    "sync"
    "time"

    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    "k8s.io/apimachinery/pkg/fields"
    "k8s.io/client-go/tools/cache"
    "go.uber.org/zap"
)

// InformerDeduplicationService uses K8s informer pattern for CRD state tracking
type InformerDeduplicationService struct {
    informer cache.SharedIndexInformer
    stopCh   chan struct{}
    logger   *zap.Logger

    // In-memory cache: fingerprint → CRD metadata (only in-flight CRDs)
    inFlightCRDs sync.Map // map[string]*InFlightCRDMetadata
}

type InFlightCRDMetadata struct {
    Namespace         string
    Name              string
    Fingerprint       string
    Phase             string // "Pending" or "Processing"
    OccurrenceCount   int
    FirstSeen         time.Time
    LastSeen          time.Time
}

func NewInformerDeduplicationService(
    k8sClient *k8s.Client,
    logger *zap.Logger,
) (*InformerDeduplicationService, error) {

    // Create informer that watches RemediationRequest CRDs
    // Watch ALL namespaces (use "" for all namespaces)
    informer := cache.NewSharedIndexInformer(
        cache.NewListWatchFromClient(
            k8sClient.RESTClient(),
            "remediationrequests",
            "", // All namespaces
            fields.Everything(),
        ),
        &remediationv1alpha1.RemediationRequest{},
        0, // No resync period (we don't need periodic full list)
        cache.Indexers{
            // Index by fingerprint for fast lookup
            "fingerprint": func(obj interface{}) ([]string, error) {
                rr := obj.(*remediationv1alpha1.RemediationRequest)
                if rr.Spec.Deduplication.Fingerprint != "" {
                    return []string{rr.Spec.Deduplication.Fingerprint}, nil
                }
                return []string{}, nil
            },
        },
    )

    service := &InformerDeduplicationService{
        informer: informer,
        stopCh:   make(chan struct{}),
        logger:   logger,
    }

    // Register event handlers
    informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        // NO AddFunc - Gateway creates CRDs, we don't need to watch CREATE

        UpdateFunc: func(oldObj, newObj interface{}) {
            service.handleUpdate(newObj.(*remediationv1alpha1.RemediationRequest))
        },

        DeleteFunc: func(obj interface{}) {
            service.handleDelete(obj.(*remediationv1alpha1.RemediationRequest))
        },
    })

    return service, nil
}

func (s *InformerDeduplicationService) Start() error {
    s.logger.Info("Starting RemediationRequest informer")
    go s.informer.Run(s.stopCh)

    // Wait for cache to sync (initial LIST from K8s API)
    if !cache.WaitForCacheSync(s.stopCh, s.informer.HasSynced) {
        return fmt.Errorf("failed to sync informer cache")
    }

    s.logger.Info("RemediationRequest informer cache synced")
    return nil
}

func (s *InformerDeduplicationService) Stop() {
    s.logger.Info("Stopping RemediationRequest informer")
    close(s.stopCh)
}

// handleUpdate processes UPDATE events from K8s
func (s *InformerDeduplicationService) handleUpdate(rr *remediationv1alpha1.RemediationRequest) {
    fingerprint := rr.Spec.Deduplication.Fingerprint
    if fingerprint == "" {
        return // Skip CRDs without fingerprint
    }

    phase := rr.Status.Phase

    switch phase {
    case "Pending", "Processing":
        // CRD is in-flight → Track in cache
        metadata := &InFlightCRDMetadata{
            Namespace:       rr.Namespace,
            Name:            rr.Name,
            Fingerprint:     fingerprint,
            Phase:           phase,
            OccurrenceCount: rr.Spec.Deduplication.OccurrenceCount,
            FirstSeen:       rr.Spec.Deduplication.FirstSeen.Time,
            LastSeen:        rr.Spec.Deduplication.LastSeen.Time,
        }
        s.inFlightCRDs.Store(fingerprint, metadata)

        s.logger.Debug("Tracking in-flight CRD",
            zap.String("fingerprint", fingerprint),
            zap.String("crd", fmt.Sprintf("%s/%s", rr.Namespace, rr.Name)),
            zap.String("phase", phase),
            zap.Int("occurrenceCount", metadata.OccurrenceCount))

    case "Completed", "Failed", "Cancelled":
        // CRD reached final state → Remove from cache
        s.inFlightCRDs.Delete(fingerprint)

        s.logger.Debug("Removed completed CRD from tracking",
            zap.String("fingerprint", fingerprint),
            zap.String("crd", fmt.Sprintf("%s/%s", rr.Namespace, rr.Name)),
            zap.String("phase", phase))
    }
}

// handleDelete processes DELETE events from K8s
func (s *InformerDeduplicationService) handleDelete(rr *remediationv1alpha1.RemediationRequest) {
    fingerprint := rr.Spec.Deduplication.Fingerprint
    if fingerprint == "" {
        return
    }

    // CRD deleted → Remove from cache
    s.inFlightCRDs.Delete(fingerprint)

    s.logger.Debug("Removed deleted CRD from tracking",
        zap.String("fingerprint", fingerprint),
        zap.String("crd", fmt.Sprintf("%s/%s", rr.Namespace, rr.Name)))
}

// Check determines if an alert is a duplicate by checking in-flight CRDs
func (s *InformerDeduplicationService) Check(
    ctx context.Context,
    signal *types.NormalizedSignal,
) (bool, *DeduplicationMetadata, error) {

    // Check if CRD exists in in-flight cache (local memory lookup - ~100ns)
    if metadata, exists := s.inFlightCRDs.Load(signal.Fingerprint); exists {
        inFlightMeta := metadata.(*InFlightCRDMetadata)

        // CRD is in-flight (Pending/Processing) → This is a duplicate
        return true, &DeduplicationMetadata{
            Fingerprint:           signal.Fingerprint,
            Count:                 inFlightMeta.OccurrenceCount + 1,
            RemediationRequestRef: fmt.Sprintf("%s/%s", inFlightMeta.Namespace, inFlightMeta.Name),
            FirstSeen:             inFlightMeta.FirstSeen.Format(time.RFC3339),
            LastSeen:              time.Now().Format(time.RFC3339),
        }, nil
    }

    // CRD not in cache → Either doesn't exist OR already completed
    // Either way, this is a NEW incident
    return false, nil, nil
}
```

**Event Flow**:
```
┌─────────────────────────────────────────────────────────────────┐
│  Informer Event Handling (Only UPDATE and DELETE)               │
├─────────────────────────────────────────────────────────────────┤
│  CREATE event   → IGNORED (Gateway creates CRDs)                │
│  UPDATE event   → Check phase:                                  │
│                   - Pending/Processing → Add to inFlightCRDs    │
│                   - Completed/Failed   → Remove from cache      │
│  DELETE event   → Remove from inFlightCRDs                      │
└─────────────────────────────────────────────────────────────────┘
```

**Deduplication Check Flow**:
```
Alert arrives with fingerprint "abc123..."
    │
    └─> Check inFlightCRDs.Load("abc123...")
        │
        ├─> EXISTS (in-flight CRD found)
        │   └─> Return isDuplicate=true, metadata with occurrenceCount+1
        │
        └─> NOT EXISTS (no in-flight CRD)
            └─> Return isDuplicate=false (create new CRD)
```

**Memory Management**:
```
┌─────────────────────────────────────────────────────────────────┐
│  In-Flight CRD Lifecycle in Informer Cache                      │
├─────────────────────────────────────────────────────────────────┤
│  Gateway creates CRD → Phase: Pending                           │
│      ↓                                                           │
│  Informer receives UPDATE → Add to inFlightCRDs map             │
│      ↓                                                           │
│  CRD processing... (Phase: Processing)                          │
│      ↓                                                           │
│  CRD completes → Phase: Completed                               │
│      ↓                                                           │
│  Informer receives UPDATE → Remove from inFlightCRDs map        │
│      ↓                                                           │
│  (Optional) CRD deleted after TTL                               │
│      ↓                                                           │
│  Informer receives DELETE → Ensure removed from map             │
└─────────────────────────────────────────────────────────────────┘
```

**Pros**:
- ✅ **No Redis dependency**: Informer's local cache is sufficient
- ✅ **Real-time updates**: Informer watches K8s events, no polling or TTL
- ✅ **Memory efficient**: Only tracks in-flight CRDs (Pending/Processing)
- ✅ **Fast lookups**: In-memory map lookup (~100ns vs Redis ~1ms)
- ✅ **No cache staleness**: Informer receives real-time K8s events
- ✅ **No K8s API load**: Informer uses watch API (single connection, push-based)
- ✅ **Automatic cleanup**: Completed/deleted CRDs removed from cache automatically
- ✅ **Correct behavior**: State-based deduplication with CRD lifecycle
- ✅ **Simpler architecture**: No Redis caching layer needed

**Cons & Mitigations**:
- ❌ **Initial sync overhead**: Informer performs LIST on startup
  - **Mitigation 1**: Only lists RemediationRequest CRDs (not all K8s resources)
  - **Mitigation 2**: One-time cost at Gateway startup (negligible)
  - **Mitigation 3**: Informer cache persists for Gateway lifetime

- ❌ **Memory overhead**: All in-flight CRDs stored in memory
  - **Mitigation 1**: Only track in-flight CRDs (Pending/Processing), not completed
  - **Mitigation 2**: Typical cluster: <100 in-flight CRDs (~10KB memory)
  - **Mitigation 3**: Storm scenarios: ~1000 CRDs max (~100KB memory)
  - **Mitigation 4**: Memory usage negligible compared to Gateway's base footprint

- ❌ **Informer connection failure**: If K8s API unavailable, cache becomes stale
  - **Mitigation 1**: Informer automatically reconnects and resyncs
  - **Mitigation 2**: Fall back to direct K8s API query if informer not synced
  - **Mitigation 3**: Metrics to monitor informer sync status

**Confidence**: 95% (best approach - no Redis, real-time, memory efficient)

**Why This is Superior to Alternative 3 (Redis Cache)**:
- **No external dependency**: Removes Redis from deduplication path
- **Real-time**: Informer receives K8s events immediately (no 30-second staleness)
- **Faster**: In-memory lookup (~100ns) vs Redis (~1ms)
- **Simpler**: No cache invalidation logic, no TTL management
- **More reliable**: Single source of truth (K8s API), no cache coherence issues

---

## Decision

**PENDING USER APPROVAL**

**Recommendation**: **Alternative 4 - Informer-Based (K8s Client-Go Cache)**

### Rationale

1. **Correct behavior**: State-based deduplication (not arbitrary TTL)
2. **Performance**: Redis cache reduces K8s API load by 97%
3. **Graceful degradation**: Falls back to Redis time-based check if K8s unavailable
4. **Acceptable staleness**: 30-second cache TTL is negligible for remediation lifecycle

### Key Insight

**Deduplication window should match CRD lifecycle, not arbitrary time**:
- CRD being processed → Same incident → Update `occurrenceCount`
- CRD completed → New incident → Create NEW CRD

**30-second cache TTL is acceptable** because:
- Remediation typically takes 5-10 minutes
- 30-second staleness = 0.5% of remediation time
- Correctness over speed (same principle as DD-GATEWAY-008)

### Implementation

**Primary Implementation Files**:
- `pkg/gateway/processing/deduplication.go` - Modified Check() with CRD state query + Redis cache (est. 100-150 LOC changes)
- `pkg/gateway/processing/crd_updater.go` - New CRD updater for occurrence count (est. 150-200 LOC)
- `pkg/gateway/server.go` - Modified ProcessSignal() to call CRD updater (est. 20-30 LOC changes)
- `test/integration/gateway/deduplication_state_test.go` - Integration tests (est. 400-500 LOC)

**Data Flow**:
1. Alert arrives with fingerprint `abc123...`
2. Check Redis cache for CRD state (`gateway:crd:state:namespace:rr-abc123`)
3. If cache miss → Query K8s for CRD state, cache result (30s TTL)
4. If CRD state = Pending/Processing → Update `occurrenceCount`, return 202
5. If CRD state = Completed/Failed → Create NEW CRD

**Redis Data Structures**:
```
# CRD state cache (string)
gateway:crd:state:<namespace>:<crd-name> = "Pending" | "Processing" | "Completed" | "Failed" | "Cancelled"
TTL: 30 seconds

# Fallback: Time-based deduplication (existing)
gateway:dedup:fingerprint:<fingerprint> = {
    "fingerprint": "...",
    "count": 5,
    "firstSeen": "...",
    "lastSeen": "...",
    "remediationRequestRef": "namespace/crd-name"
}
TTL: 5 minutes (fallback only)
```

**Graceful Degradation**:
- If K8s API unavailable → Fall back to Redis time-based deduplication (5-minute TTL)
- If Redis unavailable → Fall back to K8s API direct query (no cache)
- If CRD update fails → Log warning, don't block alert processing

**Phased Rollout Strategy**:

**Phase 1: Feature Flag (Week 1)**
- Deploy with feature flag `ENABLE_STATE_BASED_DEDUP=false` (disabled by default)
- Monitor existing time-based deduplication behavior
- Validate metrics collection works

**Phase 2: Canary Deployment (Week 2)**
- Enable for 10% of traffic (specific namespaces)
- Monitor K8s API load, cache hit rate, latency impact
- Validate CRD occurrence count updates work correctly
- Rollback criteria: K8s API load >10% increase OR latency P95 >50ms

**Phase 3: Gradual Rollout (Week 3-4)**
- 25% → 50% → 75% → 100% traffic
- Monitor deduplication accuracy improvement
- Validate no CRD name collisions
- Rollback criteria: Collision rate >1% OR deduplication accuracy <95%

**Phase 4: Remove Time-Based Fallback (Week 5)**
- Remove Redis time-based deduplication (5-minute TTL)
- Rely solely on state-based deduplication
- Keep fallback for K8s API unavailability

---

## Consequences

### Positive

- ✅ **Correct behavior**: Duplicates during processing → update count, completed → new CRD
- ✅ **No arbitrary TTL**: Deduplication window = CRD lifecycle
- ✅ **No collision risk**: CRD name reused only after completion
- ✅ **Accurate metrics**: `occurrenceCount` reflects true duplicate rate
- ✅ **New CRD after remediation**: Allows retry for recurring issues
- ✅ **Low K8s API load**: Redis cache reduces queries by 97%

### Negative

- ⚠️ **Cache staleness**: 30-second window where state might be outdated
  - **Mitigation**: Acceptable trade-off (30s delay is negligible for remediation lifecycle)
- ⚠️ **Implementation complexity**: Two-tier caching (Redis + K8s)
  - **Mitigation**: Well-defined cache invalidation strategy, comprehensive tests
- ⚠️ **CRD update overhead**: Extra K8s API call to update `occurrenceCount`
  - **Mitigation**: Async updates, don't block alert processing

### Neutral

- 🔄 **Test updates required**: Integration tests must account for CRD state checks
- 🔄 **Metrics changes**: New metrics for cache hit rate, CRD update success rate
- 🔄 **Documentation updates**: API behavior change (deduplication based on CRD state)

---

## Validation Results

### Confidence Assessment Progression

- **Initial assessment**: 85% confidence (requires K8s API performance validation)
- **After user decision**: TBD
- **After performance validation**: TBD
- **After implementation review**: TBD

### Key Validation Points

- ✅ **Problem identified**: Time-based deduplication causes CRD collisions
- ✅ **Correct behavior defined**: State-based deduplication matches CRD lifecycle
- ✅ **Alternatives evaluated**: 3 approaches with pros/cons
- ⏸️ **User decision pending**: Awaiting approval for Alternative 3
- ⏸️ **Performance validation pending**: K8s API load and latency impact

---

## Related Decisions

- **Builds On**: [BR-GATEWAY-011](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md#br-gateway-011-deduplication) - Deduplication requirement
- **Builds On**: [DD-GATEWAY-008](DD-GATEWAY-008-storm-aggregation-first-alert-handling.md) - Storm buffering (orthogonal concern)
- **Related**: [DD-GATEWAY-004](DD-GATEWAY-004-redis-memory-optimization.md) - Redis memory optimization

---

## Integration with DD-GATEWAY-008 (Storm Buffering)

### Two Orthogonal Concerns

**Deduplication and Storm Buffering are INDEPENDENT**:

| Aspect | Deduplication (DD-009) | Storm Buffering (DD-008) |
|---|---|---|
| **Purpose** | Prevent duplicate CRDs for same incident | Aggregate multiple resources into 1 CRD |
| **Trigger** | Same fingerprint (same incident) | Different fingerprints (different resources) |
| **Window** | STATE-BASED (CRD lifecycle) | THRESHOLD-BASED (60 seconds) |
| **Action** | Increment `occurrenceCount` | Create aggregated CRD with all resources |
| **CRD Count** | 1 CRD (reused) | 1 CRD (aggregated) |

### Example Scenarios

**Scenario 1: Deduplication — K8s Events (Same Deployment, Multiple Pod Crashes/Reasons)**
- Pod A (owned by Deployment D) crashes at T=0s (BackOff), T=30s (OOMKilling)
- Pod B (also owned by Deployment D, recreated) crashes at T=60s (BackOff)
- **Fingerprint**: Same — `SHA256(namespace:Deployment:D)` — owner-chain-based, reason excluded (BR-GATEWAY-004, updated 2026-02-09)
- **Result**: 1 CRD with `occurrenceCount=3` (DEDUPLICATION)

**Scenario 1b: Deduplication — Prometheus Alerts (Multiple Alertnames for Same Resource)**
- Alert "KubePodCrashLooping" for Pod A (owned by Deployment D) fires at T=0s
- Alert "KubePodNotReady" for Pod A (owned by Deployment D) fires at T=30s
- Alert "KubeContainerOOMKilled" for Pod B (also owned by Deployment D, recreated) fires at T=60s
- **Fingerprint**: Same — `SHA256(namespace:Deployment:D)` — owner-chain-based, alertname excluded (BR-GATEWAY-004, updated 2026-02-09, Issue #63)
- **Result**: 1 CRD with `occurrenceCount=3` (DEDUPLICATION)
- **Rationale**: LLM investigates resource state, not signal type. The RCA outcome is independent of which alert triggered it.

**Scenario 2: Storm Buffering (5 Different Deployments Crash)**
- Deployments D1, D2, D3, D4, D5 each produce events at T=0s, T=5s, T=10s, T=15s, T=20s
- **Fingerprints**: Different (each Deployment produces unique fingerprint)
- **Result**: 1 aggregated CRD with 5 resources (STORM BUFFERING)

**Scenario 3: Both (5 Deployments Crash, Each Produces Multiple Events)**
- Deployments D1-D5 each produce BackOff + OOMKilling events
- **Fingerprints**: 5 different fingerprints (one per Deployment), each appears twice
- **Result**: 1 aggregated CRD with 5 resources, each with `occurrenceCount=2` (BOTH)

### Processing Flow

```
Alert arrives
    │
    ├─> Check if CRD exists for this fingerprint (DD-009)
    │   │
    │   ├─> YES: CRD exists
    │   │   │
    │   │   ├─> CRD state = Pending/Processing
    │   │   │   └─> Update occurrenceCount (DEDUPLICATION)
    │   │   │
    │   │   └─> CRD state = Completed/Failed/Cancelled
    │   │       └─> Create NEW CRD (new incident)
    │   │
    │   └─> NO: CRD doesn't exist
    │       │
    │       ├─> Check if storm detected (DD-008)
    │       │   │
    │       │   ├─> YES: Add to buffer (STORM BUFFERING)
    │       │   │   └─> After threshold: Create aggregated CRD
    │       │   │
    │       │   └─> NO: Create individual CRD
    │       │
    │       └─> Cache CRD state in Redis (30s TTL)
```

---

## Review & Evolution

### When to Revisit

- If CRD collision rate >1% in production
- If K8s API load increases >10%
- If deduplication accuracy <95%
- If cache staleness causes user complaints
- If V2.0 considers alternative CRD naming strategy

### Success Metrics

- **Deduplication accuracy**: ≥95% of duplicates correctly identified
- **CRD collision rate**: <1% (should be near 0% with state-based approach)
- **K8s API load**: <10% increase (Redis cache should minimize impact)
- **Cache hit rate**: ≥95% (30-second TTL should be effective)
- **Latency P95**: <50ms for deduplication check (including cache)
- **CRD update success rate**: ≥99% (occurrence count updates)

---

## Next Steps

1. **User Decision**: Approve Alternative 3 (or select different alternative)
2. **Performance Validation**: Measure K8s API load and latency impact in test environment
3. **Implementation**: Create `crd_updater.go` and modify `deduplication.go`
4. **Testing**: Integration tests with real K8s cluster and CRD state transitions
5. **Metrics**: Add cache hit rate, CRD update success rate, deduplication accuracy metrics
6. **Deployment**: Gradual rollout with feature flag and monitoring


│      ↓                                                           │
│  Informer receives UPDATE → Remove from inFlightCRDs map        │
│      ↓                                                           │
│  (Optional) CRD deleted after TTL                               │
│      ↓                                                           │
│  Informer receives DELETE → Ensure removed from map             │
└─────────────────────────────────────────────────────────────────┘
```

**Pros**:
- ✅ **No Redis dependency**: Informer's local cache is sufficient
- ✅ **Real-time updates**: Informer watches K8s events, no polling or TTL
- ✅ **Memory efficient**: Only tracks in-flight CRDs (Pending/Processing)
- ✅ **Fast lookups**: In-memory map lookup (~100ns vs Redis ~1ms)
- ✅ **No cache staleness**: Informer receives real-time K8s events
- ✅ **No K8s API load**: Informer uses watch API (single connection, push-based)
- ✅ **Automatic cleanup**: Completed/deleted CRDs removed from cache automatically
- ✅ **Correct behavior**: State-based deduplication with CRD lifecycle
- ✅ **Simpler architecture**: No Redis caching layer needed

**Cons & Mitigations**:
- ❌ **Initial sync overhead**: Informer performs LIST on startup
  - **Mitigation 1**: Only lists RemediationRequest CRDs (not all K8s resources)
  - **Mitigation 2**: One-time cost at Gateway startup (negligible)
  - **Mitigation 3**: Informer cache persists for Gateway lifetime

- ❌ **Memory overhead**: All in-flight CRDs stored in memory
  - **Mitigation 1**: Only track in-flight CRDs (Pending/Processing), not completed
  - **Mitigation 2**: Typical cluster: <100 in-flight CRDs (~10KB memory)
  - **Mitigation 3**: Storm scenarios: ~1000 CRDs max (~100KB memory)
  - **Mitigation 4**: Memory usage negligible compared to Gateway's base footprint

- ❌ **Informer connection failure**: If K8s API unavailable, cache becomes stale
  - **Mitigation 1**: Informer automatically reconnects and resyncs
  - **Mitigation 2**: Fall back to direct K8s API query if informer not synced
  - **Mitigation 3**: Metrics to monitor informer sync status

**Confidence**: 95% (best approach - no Redis, real-time, memory efficient)

**Why This is Superior to Alternative 3 (Redis Cache)**:
- **No external dependency**: Removes Redis from deduplication path
- **Real-time**: Informer receives K8s events immediately (no 30-second staleness)
- **Faster**: In-memory lookup (~100ns) vs Redis (~1ms)
- **Simpler**: No cache invalidation logic, no TTL management
- **More reliable**: Single source of truth (K8s API), no cache coherence issues

---

## Decision

**✅ APPROVED** (2025-11-17)

**v1.0 Decision**: **Alternative 2 - State-Based Deduplication (Direct K8s API)**
**v1.1 Optimization**: **Alternative 4 - Informer-Based (K8s Client-Go Cache)** - Deferred

### Rationale for v1.0 (Alternative 2)

**User's Key Insight**: "It adds complexity (informer) when we don't expect many alerts at this point, and querying the API server for a few CRDs is acceptable."

1. **Correct behavior**: State-based deduplication (CRD lifecycle, not arbitrary TTL)
2. **Simplicity over optimization**: Direct K8s API queries, no informer complexity
3. **Acceptable performance**: Low alert volume expected initially (<100 alerts/hour)
4. **K8s API query cost**: ~5-10ms per deduplication check is acceptable for v1.0 scale
5. **Faster implementation**: No informer setup, event handlers, or cache management
6. **Lower risk**: Simpler code, easier to debug, validate, and maintain
7. **No Redis dependency**: Direct K8s API queries eliminate Redis from deduplication path

### Why Defer Alternative 4 (Informer) to v1.1

**Complexity vs. Benefit Analysis**:

| Aspect | v1.0 (Alternative 2) | v1.1 (Alternative 4) |
|---|---|---|
| **Expected Load** | <100 alerts/hour | >1000 alerts/hour |
| **K8s API Queries** | <100 queries/hour (~1.7/min) | >1000 queries/hour (~16/min) |
| **Latency Impact** | ~5-10ms per check (acceptable) | ~100ns per check (optimized) |
| **Complexity** | Low (direct API queries) | High (informer + event handlers) |
| **Implementation Time** | 1-2 days | 3-5 days |
| **ROI** | High (correct behavior) | Low (premature optimization) |

**When to Revisit (v1.1 Triggers)**:
- Alert volume exceeds 1000/hour (K8s API load becomes measurable)
- Latency P95 >100ms for deduplication checks
- Multiple Gateway replicas deployed (informer cache per replica reduces API load)
- Performance optimization becomes priority over simplicity

### Key Insight

**Deduplication window should match CRD lifecycle, not arbitrary time**:
- CRD being processed (Pending/Processing) → Same incident → Update `occurrenceCount`
- CRD completed (Completed/Failed/Cancelled) → New incident → Create NEW CRD

### Implementation

**Primary Implementation Files**:
- `pkg/gateway/processing/deduplication.go` - Modified Check() with CRD state query + Redis cache (est. 100-150 LOC changes)
- `pkg/gateway/processing/crd_updater.go` - New CRD updater for occurrence count (est. 150-200 LOC)
- `pkg/gateway/server.go` - Modified ProcessSignal() to call CRD updater (est. 20-30 LOC changes)
- `test/integration/gateway/deduplication_state_test.go` - Integration tests (est. 400-500 LOC)

**Data Flow**:
1. Alert arrives with fingerprint `abc123...`
2. Check Redis cache for CRD state (`gateway:crd:state:namespace:rr-abc123`)
3. If cache miss → Query K8s for CRD state, cache result (30s TTL)
4. If CRD state = Pending/Processing → Update `occurrenceCount`, return 202
5. If CRD state = Completed/Failed → Create NEW CRD

**Redis Data Structures**:
```
# CRD state cache (string)
gateway:crd:state:<namespace>:<crd-name> = "Pending" | "Processing" | "Completed" | "Failed" | "Cancelled"
TTL: 30 seconds

# Fallback: Time-based deduplication (existing)
gateway:dedup:fingerprint:<fingerprint> = {
    "fingerprint": "...",
    "count": 5,
    "firstSeen": "...",
    "lastSeen": "...",
    "remediationRequestRef": "namespace/crd-name"
}
TTL: 5 minutes (fallback only)
```

**Graceful Degradation**:
- If K8s API unavailable → Fall back to Redis time-based deduplication (5-minute TTL)
- If Redis unavailable → Fall back to K8s API direct query (no cache)
- If CRD update fails → Log warning, don't block alert processing

**Phased Rollout Strategy**:

**Phase 1: Feature Flag (Week 1)**
- Deploy with feature flag `ENABLE_STATE_BASED_DEDUP=false` (disabled by default)
- Monitor existing time-based deduplication behavior
- Validate metrics collection works

**Phase 2: Canary Deployment (Week 2)**
- Enable for 10% of traffic (specific namespaces)
- Monitor K8s API load, cache hit rate, latency impact
- Validate CRD occurrence count updates work correctly
- Rollback criteria: K8s API load >10% increase OR latency P95 >50ms

**Phase 3: Gradual Rollout (Week 3-4)**
- 25% → 50% → 75% → 100% traffic
- Monitor deduplication accuracy improvement
- Validate no CRD name collisions
- Rollback criteria: Collision rate >1% OR deduplication accuracy <95%

**Phase 4: Remove Time-Based Fallback (Week 5)**
- Remove Redis time-based deduplication (5-minute TTL)
- Rely solely on state-based deduplication
- Keep fallback for K8s API unavailability

---

## Consequences

### Positive

- ✅ **Correct behavior**: Duplicates during processing → update count, completed → new CRD
- ✅ **No arbitrary TTL**: Deduplication window = CRD lifecycle
- ✅ **No collision risk**: CRD name reused only after completion
- ✅ **Accurate metrics**: `occurrenceCount` reflects true duplicate rate
- ✅ **New CRD after remediation**: Allows retry for recurring issues
- ✅ **Low K8s API load**: Redis cache reduces queries by 97%

### Negative

- ⚠️ **Cache staleness**: 30-second window where state might be outdated
  - **Mitigation**: Acceptable trade-off (30s delay is negligible for remediation lifecycle)
- ⚠️ **Implementation complexity**: Two-tier caching (Redis + K8s)
  - **Mitigation**: Well-defined cache invalidation strategy, comprehensive tests
- ⚠️ **CRD update overhead**: Extra K8s API call to update `occurrenceCount`
  - **Mitigation**: Async updates, don't block alert processing

### Neutral

- 🔄 **Test updates required**: Integration tests must account for CRD state checks
- 🔄 **Metrics changes**: New metrics for cache hit rate, CRD update success rate
- 🔄 **Documentation updates**: API behavior change (deduplication based on CRD state)

---

## Validation Results

### Confidence Assessment Progression

- **Initial assessment**: 85% confidence (requires K8s API performance validation)
- **After user decision**: TBD
- **After performance validation**: TBD
- **After implementation review**: TBD

### Key Validation Points

- ✅ **Problem identified**: Time-based deduplication causes CRD collisions
- ✅ **Correct behavior defined**: State-based deduplication matches CRD lifecycle
- ✅ **Alternatives evaluated**: 3 approaches with pros/cons
- ⏸️ **User decision pending**: Awaiting approval for Alternative 3
- ⏸️ **Performance validation pending**: K8s API load and latency impact

---

## Related Decisions

- **Builds On**: [BR-GATEWAY-011](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md#br-gateway-011-deduplication) - Deduplication requirement
- **Builds On**: [DD-GATEWAY-008](DD-GATEWAY-008-storm-aggregation-first-alert-handling.md) - Storm buffering (orthogonal concern)
- **Related**: [DD-GATEWAY-004](DD-GATEWAY-004-redis-memory-optimization.md) - Redis memory optimization

---

## Integration with DD-GATEWAY-008 (Storm Buffering)

### Two Orthogonal Concerns

**Deduplication and Storm Buffering are INDEPENDENT**:

| Aspect | Deduplication (DD-009) | Storm Buffering (DD-008) |
|---|---|---|
| **Purpose** | Prevent duplicate CRDs for same incident | Aggregate multiple resources into 1 CRD |
| **Trigger** | Same fingerprint (same incident) | Different fingerprints (different resources) |
| **Window** | STATE-BASED (CRD lifecycle) | THRESHOLD-BASED (60 seconds) |
| **Action** | Increment `occurrenceCount` | Create aggregated CRD with all resources |
| **CRD Count** | 1 CRD (reused) | 1 CRD (aggregated) |

### Example Scenarios

**Scenario 1: Deduplication — K8s Events (Same Deployment, Multiple Pod Crashes/Reasons)**
- Pod A (owned by Deployment D) crashes at T=0s (BackOff), T=30s (OOMKilling)
- Pod B (also owned by Deployment D, recreated) crashes at T=60s (BackOff)
- **Fingerprint**: Same — `SHA256(namespace:Deployment:D)` — owner-chain-based, reason excluded (BR-GATEWAY-004, updated 2026-02-09)
- **Result**: 1 CRD with `occurrenceCount=3` (DEDUPLICATION)

**Scenario 1b: Deduplication — Prometheus Alerts (Multiple Alertnames for Same Resource)**
- Alert "KubePodCrashLooping" for Pod A (owned by Deployment D) fires at T=0s
- Alert "KubePodNotReady" for Pod A (owned by Deployment D) fires at T=30s
- Alert "KubeContainerOOMKilled" for Pod B (also owned by Deployment D, recreated) fires at T=60s
- **Fingerprint**: Same — `SHA256(namespace:Deployment:D)` — owner-chain-based, alertname excluded (BR-GATEWAY-004, updated 2026-02-09, Issue #63)
- **Result**: 1 CRD with `occurrenceCount=3` (DEDUPLICATION)
- **Rationale**: LLM investigates resource state, not signal type. The RCA outcome is independent of which alert triggered it.

**Scenario 2: Storm Buffering (5 Different Deployments Crash)**
- Deployments D1, D2, D3, D4, D5 each produce events at T=0s, T=5s, T=10s, T=15s, T=20s
- **Fingerprints**: Different (each Deployment produces unique fingerprint)
- **Result**: 1 aggregated CRD with 5 resources (STORM BUFFERING)

**Scenario 3: Both (5 Deployments Crash, Each Produces Multiple Events)**
- Deployments D1-D5 each produce BackOff + OOMKilling events
- **Fingerprints**: 5 different fingerprints (one per Deployment), each appears twice
- **Result**: 1 aggregated CRD with 5 resources, each with `occurrenceCount=2` (BOTH)

### Processing Flow

```
Alert arrives
    │
    ├─> Check if CRD exists for this fingerprint (DD-009)
    │   │
    │   ├─> YES: CRD exists
    │   │   │
    │   │   ├─> CRD state = Pending/Processing
    │   │   │   └─> Update occurrenceCount (DEDUPLICATION)
    │   │   │
    │   │   └─> CRD state = Completed/Failed/Cancelled
    │   │       └─> Create NEW CRD (new incident)
    │   │
    │   └─> NO: CRD doesn't exist
    │       │
    │       ├─> Check if storm detected (DD-008)
    │       │   │
    │       │   ├─> YES: Add to buffer (STORM BUFFERING)
    │       │   │   └─> After threshold: Create aggregated CRD
    │       │   │
    │       │   └─> NO: Create individual CRD
    │       │
    │       └─> Cache CRD state in Redis (30s TTL)
```

---

## Review & Evolution

### When to Revisit

- If CRD collision rate >1% in production
- If K8s API load increases >10%
- If deduplication accuracy <95%
- If cache staleness causes user complaints
- If V2.0 considers alternative CRD naming strategy

### Success Metrics

- **Deduplication accuracy**: ≥95% of duplicates correctly identified
- **CRD collision rate**: <1% (should be near 0% with state-based approach)
- **K8s API load**: <10% increase (Redis cache should minimize impact)
- **Cache hit rate**: ≥95% (30-second TTL should be effective)
- **Latency P95**: <50ms for deduplication check (including cache)
- **CRD update success rate**: ≥99% (occurrence count updates)

---

## Next Steps

1. **User Decision**: Approve Alternative 3 (or select different alternative)
2. **Performance Validation**: Measure K8s API load and latency impact in test environment
3. **Implementation**: Create `crd_updater.go` and modify `deduplication.go`
4. **Testing**: Integration tests with real K8s cluster and CRD state transitions
5. **Metrics**: Add cache hit rate, CRD update success rate, deduplication accuracy metrics
6. **Deployment**: Gradual rollout with feature flag and monitoring


│      ↓                                                           │
│  Informer receives UPDATE → Remove from inFlightCRDs map        │
│      ↓                                                           │
│  (Optional) CRD deleted after TTL                               │
│      ↓                                                           │
│  Informer receives DELETE → Ensure removed from map             │
└─────────────────────────────────────────────────────────────────┘
```

**Pros**:
- ✅ **No Redis dependency**: Informer's local cache is sufficient
- ✅ **Real-time updates**: Informer watches K8s events, no polling or TTL
- ✅ **Memory efficient**: Only tracks in-flight CRDs (Pending/Processing)
- ✅ **Fast lookups**: In-memory map lookup (~100ns vs Redis ~1ms)
- ✅ **No cache staleness**: Informer receives real-time K8s events
- ✅ **No K8s API load**: Informer uses watch API (single connection, push-based)
- ✅ **Automatic cleanup**: Completed/deleted CRDs removed from cache automatically
- ✅ **Correct behavior**: State-based deduplication with CRD lifecycle
- ✅ **Simpler architecture**: No Redis caching layer needed

**Cons & Mitigations**:
- ❌ **Initial sync overhead**: Informer performs LIST on startup
  - **Mitigation 1**: Only lists RemediationRequest CRDs (not all K8s resources)
  - **Mitigation 2**: One-time cost at Gateway startup (negligible)
  - **Mitigation 3**: Informer cache persists for Gateway lifetime

- ❌ **Memory overhead**: All in-flight CRDs stored in memory
  - **Mitigation 1**: Only track in-flight CRDs (Pending/Processing), not completed
  - **Mitigation 2**: Typical cluster: <100 in-flight CRDs (~10KB memory)
  - **Mitigation 3**: Storm scenarios: ~1000 CRDs max (~100KB memory)
  - **Mitigation 4**: Memory usage negligible compared to Gateway's base footprint

- ❌ **Informer connection failure**: If K8s API unavailable, cache becomes stale
  - **Mitigation 1**: Informer automatically reconnects and resyncs
  - **Mitigation 2**: Fall back to direct K8s API query if informer not synced
  - **Mitigation 3**: Metrics to monitor informer sync status

**Confidence**: 95% (best approach - no Redis, real-time, memory efficient)

**Why This is Superior to Alternative 3 (Redis Cache)**:
- **No external dependency**: Removes Redis from deduplication path
- **Real-time**: Informer receives K8s events immediately (no 30-second staleness)
- **Faster**: In-memory lookup (~100ns) vs Redis (~1ms)
- **Simpler**: No cache invalidation logic, no TTL management
- **More reliable**: Single source of truth (K8s API), no cache coherence issues

---

## Decision

**PENDING USER APPROVAL**

**Recommendation**: **Alternative 4 - Informer-Based (K8s Client-Go Cache)**

### Rationale

1. **Correct behavior**: State-based deduplication (CRD lifecycle, not arbitrary TTL)
2. **No Redis dependency**: Informer's local cache eliminates Redis from deduplication path
3. **Real-time**: Informer receives K8s events immediately (no cache staleness)
4. **Memory efficient**: Only tracks in-flight CRDs (Pending/Processing), auto-cleanup on completion
5. **Fast**: In-memory lookup (~100ns) vs Redis (~1ms) or K8s API (~5-10ms)
6. **Simpler architecture**: No cache invalidation logic, no TTL management

### Key Insights

**User's Key Insights**:
- Gateway creates CRDs → No need to watch CREATE events
- Only care about in-flight CRDs → Remove completed/deleted from cache automatically
- Informer provides local cache → No Redis needed for deduplication

**Deduplication window matches CRD lifecycle**:
- CRD in-flight (Pending/Processing) → Same incident → Update `occurrenceCount`
- CRD completed (Completed/Failed/Cancelled) → New incident → Create NEW CRD
- CRD deleted → Removed from cache → New incident creates NEW CRD

### Implementation

**Primary Implementation Files**:
- `pkg/gateway/processing/deduplication_informer.go` - **NEW**: Informer-based deduplication service (est. 300-400 LOC)
- `pkg/gateway/processing/crd_updater.go` - **NEW**: CRD updater for occurrence count (est. 150-200 LOC)
- `pkg/gateway/server.go` - Modified: Start informer on Gateway startup, call CRD updater (est. 30-40 LOC changes)
- `test/integration/gateway/deduplication_informer_test.go` - **NEW**: Integration tests (est. 500-600 LOC)

**Data Flow**:
1. **Gateway Startup**: Start informer, wait for cache sync (initial LIST of RemediationRequest CRDs)
2. **Informer Events**: Watch UPDATE and DELETE events (not CREATE)
   - UPDATE: If phase = Pending/Processing → Add to `inFlightCRDs` map
   - UPDATE: If phase = Completed/Failed/Cancelled → Remove from `inFlightCRDs` map
   - DELETE: Remove from `inFlightCRDs` map
3. **Alert Arrives**: Check `inFlightCRDs.Load(fingerprint)`
   - EXISTS → Duplicate detected, return metadata with `occurrenceCount+1`
   - NOT EXISTS → New incident, create CRD
4. **CRD Update**: If duplicate, call `crdUpdater.IncrementOccurrenceCount()`

**In-Memory Data Structure**:
```go
// sync.Map: fingerprint → InFlightCRDMetadata
inFlightCRDs sync.Map

type InFlightCRDMetadata struct {
    Namespace         string
    Name              string
    Fingerprint       string
    Phase             string // "Pending" or "Processing"
    OccurrenceCount   int
    FirstSeen         time.Time
    LastSeen          time.Time
}
```

**Memory Footprint**:
- Typical cluster: <100 in-flight CRDs → ~10KB memory
- Storm scenario: ~1000 in-flight CRDs → ~100KB memory
- Negligible compared to Gateway's base footprint (~50MB)

**Graceful Degradation**:
- If informer not synced → Fall back to direct K8s API query
- If K8s API unavailable → Informer automatically reconnects and resyncs
- If CRD update fails → Log warning, don't block alert processing

**Phased Rollout Strategy**:

**Phase 1: Feature Flag (Week 1)**
- Deploy with feature flag `ENABLE_INFORMER_DEDUP=false` (disabled by default)
- Monitor existing time-based deduplication behavior
- Validate informer startup and sync works correctly

**Phase 2: Canary Deployment (Week 2)**
- Enable for 10% of traffic (specific namespaces)
- Monitor informer memory usage, sync status, latency impact
- Validate CRD occurrence count updates work correctly
- Rollback criteria: Memory usage >200MB OR latency P95 >50ms

**Phase 3: Gradual Rollout (Week 3-4)**
- 25% → 50% → 75% → 100% traffic
- Monitor deduplication accuracy improvement
- Validate no CRD name collisions
- Rollback criteria: Collision rate >1% OR deduplication accuracy <95%

**Phase 4: Remove Time-Based Fallback (Week 5)**
- Remove Redis time-based deduplication (5-minute TTL)
- Rely solely on informer-based deduplication
- Keep fallback for informer not synced (direct K8s API query)

---

## Consequences

### Positive

- ✅ **Correct behavior**: Duplicates during processing → update count, completed → new CRD
- ✅ **No arbitrary TTL**: Deduplication window = CRD lifecycle
- ✅ **No collision risk**: CRD name reused only after completion
- ✅ **Accurate metrics**: `occurrenceCount` reflects true duplicate rate
- ✅ **New CRD after remediation**: Allows retry for recurring issues
- ✅ **Low K8s API load**: Redis cache reduces queries by 97%

### Negative

- ⚠️ **Cache staleness**: 30-second window where state might be outdated
  - **Mitigation**: Acceptable trade-off (30s delay is negligible for remediation lifecycle)
- ⚠️ **Implementation complexity**: Two-tier caching (Redis + K8s)
  - **Mitigation**: Well-defined cache invalidation strategy, comprehensive tests
- ⚠️ **CRD update overhead**: Extra K8s API call to update `occurrenceCount`
  - **Mitigation**: Async updates, don't block alert processing

### Neutral

- 🔄 **Test updates required**: Integration tests must account for CRD state checks
- 🔄 **Metrics changes**: New metrics for cache hit rate, CRD update success rate
- 🔄 **Documentation updates**: API behavior change (deduplication based on CRD state)

---

## Validation Results

### Confidence Assessment Progression

- **Initial assessment**: 85% confidence (requires K8s API performance validation)
- **After user decision**: TBD
- **After performance validation**: TBD
- **After implementation review**: TBD

### Key Validation Points

- ✅ **Problem identified**: Time-based deduplication causes CRD collisions
- ✅ **Correct behavior defined**: State-based deduplication matches CRD lifecycle
- ✅ **Alternatives evaluated**: 3 approaches with pros/cons
- ⏸️ **User decision pending**: Awaiting approval for Alternative 3
- ⏸️ **Performance validation pending**: K8s API load and latency impact

---

## Related Decisions

- **Builds On**: [BR-GATEWAY-011](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md#br-gateway-011-deduplication) - Deduplication requirement
- **Builds On**: [DD-GATEWAY-008](DD-GATEWAY-008-storm-aggregation-first-alert-handling.md) - Storm buffering (orthogonal concern)
- **Related**: [DD-GATEWAY-004](DD-GATEWAY-004-redis-memory-optimization.md) - Redis memory optimization

---

## Integration with DD-GATEWAY-008 (Storm Buffering)

### Two Orthogonal Concerns

**Deduplication and Storm Buffering are INDEPENDENT**:

| Aspect | Deduplication (DD-009) | Storm Buffering (DD-008) |
|---|---|---|
| **Purpose** | Prevent duplicate CRDs for same incident | Aggregate multiple resources into 1 CRD |
| **Trigger** | Same fingerprint (same incident) | Different fingerprints (different resources) |
| **Window** | STATE-BASED (CRD lifecycle) | THRESHOLD-BASED (60 seconds) |
| **Action** | Increment `occurrenceCount` | Create aggregated CRD with all resources |
| **CRD Count** | 1 CRD (reused) | 1 CRD (aggregated) |

### Example Scenarios

**Scenario 1: Deduplication — K8s Events (Same Deployment, Multiple Pod Crashes/Reasons)**
- Pod A (owned by Deployment D) crashes at T=0s (BackOff), T=30s (OOMKilling)
- Pod B (also owned by Deployment D, recreated) crashes at T=60s (BackOff)
- **Fingerprint**: Same — `SHA256(namespace:Deployment:D)` — owner-chain-based, reason excluded (BR-GATEWAY-004, updated 2026-02-09)
- **Result**: 1 CRD with `occurrenceCount=3` (DEDUPLICATION)

**Scenario 1b: Deduplication — Prometheus Alerts (Multiple Alertnames for Same Resource)**
- Alert "KubePodCrashLooping" for Pod A (owned by Deployment D) fires at T=0s
- Alert "KubePodNotReady" for Pod A (owned by Deployment D) fires at T=30s
- Alert "KubeContainerOOMKilled" for Pod B (also owned by Deployment D, recreated) fires at T=60s
- **Fingerprint**: Same — `SHA256(namespace:Deployment:D)` — owner-chain-based, alertname excluded (BR-GATEWAY-004, updated 2026-02-09, Issue #63)
- **Result**: 1 CRD with `occurrenceCount=3` (DEDUPLICATION)
- **Rationale**: LLM investigates resource state, not signal type. The RCA outcome is independent of which alert triggered it.

**Scenario 2: Storm Buffering (5 Different Deployments Crash)**
- Deployments D1, D2, D3, D4, D5 each produce events at T=0s, T=5s, T=10s, T=15s, T=20s
- **Fingerprints**: Different (each Deployment produces unique fingerprint)
- **Result**: 1 aggregated CRD with 5 resources (STORM BUFFERING)

**Scenario 3: Both (5 Deployments Crash, Each Produces Multiple Events)**
- Deployments D1-D5 each produce BackOff + OOMKilling events
- **Fingerprints**: 5 different fingerprints (one per Deployment), each appears twice
- **Result**: 1 aggregated CRD with 5 resources, each with `occurrenceCount=2` (BOTH)

### Processing Flow

```
Alert arrives
    │
    ├─> Check if CRD exists for this fingerprint (DD-009)
    │   │
    │   ├─> YES: CRD exists
    │   │   │
    │   │   ├─> CRD state = Pending/Processing
    │   │   │   └─> Update occurrenceCount (DEDUPLICATION)
    │   │   │
    │   │   └─> CRD state = Completed/Failed/Cancelled
    │   │       └─> Create NEW CRD (new incident)
    │   │
    │   └─> NO: CRD doesn't exist
    │       │
    │       ├─> Check if storm detected (DD-008)
    │       │   │
    │       │   ├─> YES: Add to buffer (STORM BUFFERING)
    │       │   │   └─> After threshold: Create aggregated CRD
    │       │   │
    │       │   └─> NO: Create individual CRD
    │       │
    │       └─> Cache CRD state in Redis (30s TTL)
```

---

## Review & Evolution

### When to Revisit

- If CRD collision rate >1% in production
- If K8s API load increases >10%
- If deduplication accuracy <95%
- If cache staleness causes user complaints
- If V2.0 considers alternative CRD naming strategy

### Success Metrics

- **Deduplication accuracy**: ≥95% of duplicates correctly identified
- **CRD collision rate**: <1% (should be near 0% with state-based approach)
- **K8s API load**: <10% increase (Redis cache should minimize impact)
- **Cache hit rate**: ≥95% (30-second TTL should be effective)
- **Latency P95**: <50ms for deduplication check (including cache)
- **CRD update success rate**: ≥99% (occurrence count updates)

---

## Next Steps

1. **User Decision**: Approve Alternative 3 (or select different alternative)
2. **Performance Validation**: Measure K8s API load and latency impact in test environment
3. **Implementation**: Create `crd_updater.go` and modify `deduplication.go`
4. **Testing**: Integration tests with real K8s cluster and CRD state transitions
5. **Metrics**: Add cache hit rate, CRD update success rate, deduplication accuracy metrics
6. **Deployment**: Gradual rollout with feature flag and monitoring

