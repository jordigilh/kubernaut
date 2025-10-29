# Integration Tests & Redis Storage Triage

**Date**: October 28, 2025
**Context**: Triage integration test usage of storm fields and Redis storage patterns
**Finding**: Integration tests USE `StormAggregation` struct, NOT scattered fields

---

## 🚨 CRITICAL FINDING: Integration Tests vs. CRD Schema Mismatch

### Discovery

**Integration Tests** (`test/integration/gateway/storm_aggregation_test.go`):
```go
// Tests expect StormAggregation struct
Expect(stormCRD.Spec.StormAggregation).ToNot(BeNil())
Expect(stormCRD.Spec.StormAggregation.Pattern).To(Equal("HighCPUUsage in prod-api"))
Expect(stormCRD.Spec.StormAggregation.AlertCount).To(Equal(1))
Expect(stormCRD.Spec.StormAggregation.AffectedResources).To(HaveLen(1))
Expect(stormCRD.Spec.StormAggregation.AffectedResources[0].Kind).To(Equal("Pod"))
```

**Actual CRD Schema** (`api/remediation/v1alpha1/remediationrequest_types.go`):
```go
// CRD has scattered fields
IsStorm           bool     `json:"isStorm,omitempty"`
StormType         string   `json:"stormType,omitempty"`
StormWindow       string   `json:"stormWindow,omitempty"`
StormAlertCount   int      `json:"stormAlertCount,omitempty"`
AffectedResources []string `json:"affectedResources,omitempty"`
```

**Mismatch**: Integration tests expect consolidated `StormAggregation` struct that **doesn't exist** in CRD schema!

---

## 📊 Integration Test Analysis

### Test File: `test/integration/gateway/storm_aggregation_test.go`

**Lines Using `StormAggregation` Struct**: 11 references

1. **Line 114**: `Expect(stormCRD.Spec.StormAggregation).ToNot(BeNil())`
2. **Line 115**: `Expect(stormCRD.Spec.StormAggregation.Pattern).To(Equal(...))`
3. **Line 116**: `Expect(stormCRD.Spec.StormAggregation.AlertCount).To(Equal(1))`
4. **Line 117**: `Expect(stormCRD.Spec.StormAggregation.AffectedResources).To(HaveLen(1))`
5. **Line 118**: `Expect(stormCRD.Spec.StormAggregation.AffectedResources[0].Kind).To(Equal("Pod"))`
6. **Line 119**: `Expect(stormCRD.Spec.StormAggregation.AffectedResources[0].Name).To(Equal("api-server-1"))`
7. **Line 155**: `Expect(stormCRD2.Spec.StormAggregation.AlertCount).To(Equal(2))`
8. **Line 156**: `Expect(stormCRD2.Spec.StormAggregation.AffectedResources).To(HaveLen(2))`
9. **Line 193**: `Expect(stormCRD.Spec.StormAggregation.AlertCount).To(Equal(15))`
10. **Line 194**: `Expect(stormCRD.Spec.StormAggregation.AffectedResources).To(HaveLen(15))`
11. **Line 399**: `Expect(crd.Spec.StormAggregation.AlertCount).To(Equal(4))`

### Test Expectations

**Structured `AffectedResource` Type**:
```go
// Tests expect structured type with fields:
stormCRD.Spec.StormAggregation.AffectedResources[0].Kind
stormCRD.Spec.StormAggregation.AffectedResources[0].Name
```

**Current CRD Schema**:
```go
// CRD has simple string array:
AffectedResources []string `json:"affectedResources,omitempty"`
```

---

## 🔍 Redis Storage Pattern Analysis

### Redis Key Patterns Used

| Purpose | Redis Key Pattern | Type | TTL | Location |
|---------|------------------|------|-----|----------|
| **Deduplication** | `alert:fingerprint:<sha256>` | Hash | 5 min | `deduplication.go:178` |
| **Storm Rate Detection** | `alert:storm:rate:<alertname>` | Counter | 1 min | `storm_detection.go:170` |
| **Storm Pattern Detection** | `alert:pattern:<alertname>` | Sorted Set | 2 min | `storm_detection.go:229` |
| **Storm Aggregation Window** | `alert:storm:aggregate:<alertname>` | String | 1 min | `storm_aggregator.go:146` |
| **Storm Resources** | `alert:storm:resources:<window-id>` | Sorted Set | 1 min | `storm_aggregator.go:180` |
| **Storm Metadata** | `alert:storm:metadata:<window-id>` | Hash | 1 min | `storm_aggregator.go:293` |

### Deduplication Storage

**Key**: `alert:fingerprint:<sha256-hash>`
**Type**: Redis Hash
**TTL**: 5 minutes (configurable)

**Fields Stored**:
```go
{
    "fingerprint": "<sha256>",
    "remediationRequestRef": "<namespace>/<name>",
    "count": "<number>",
    "firstSeen": "<RFC3339 timestamp>",
    "lastSeen": "<RFC3339 timestamp>"
}
```

**Purpose**: Track duplicate alerts within 5-minute window

**Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

### Storm Detection Storage

#### Rate-Based Storm Detection

**Key**: `alert:storm:rate:<alertname>`
**Type**: Redis Counter
**TTL**: 1 minute (sliding window)

**Value**: Integer count of alerts in last minute

**Algorithm**:
1. `INCR alert:storm:rate:<alertname>`
2. If count >= 10 → storm detected
3. TTL auto-expires after 1 minute

**Purpose**: Detect alert storms (>10 alerts/minute)

**Business Requirement**: BR-GATEWAY-009 (Storm Detection)

---

#### Pattern-Based Storm Detection

**Key**: `alert:pattern:<alertname>`
**Type**: Redis Sorted Set
**TTL**: 2 minutes

**Members**: Resource identifiers (e.g., "namespace:Pod:name")
**Score**: Unix timestamp

**Algorithm**:
1. `ZADD alert:pattern:<alertname> <timestamp> <resource-id>`
2. `ZREMRANGEBYSCORE` to remove entries older than 2 minutes
3. If count >= 5 similar resources → pattern storm detected

**Purpose**: Detect similar alerts across different resources

**Business Requirement**: BR-GATEWAY-009 (Storm Detection)

---

### Storm Aggregation Storage

#### Aggregation Window Tracking

**Key**: `alert:storm:aggregate:<alertname>`
**Type**: Redis String
**TTL**: 1 minute (configurable)

**Value**: Window ID (e.g., "HighCPUUsage-1698765432")

**Purpose**: Track active aggregation window for alertname

**Business Requirement**: BR-GATEWAY-016 (Storm Aggregation)

---

#### Aggregated Resources

**Key**: `alert:storm:resources:<window-id>`
**Type**: Redis Sorted Set
**TTL**: 1 minute (window duration)

**Members**: Resource identifiers (e.g., "namespace:Pod:name")
**Score**: Unix timestamp when resource was added

**Purpose**: Collect all resources affected during storm window

**Business Requirement**: BR-GATEWAY-016 (Storm Aggregation)

---

#### Storm Metadata

**Key**: `alert:storm:metadata:<window-id>`
**Type**: Redis Hash
**TTL**: 1 minute (window duration)

**Fields Stored**:
```go
{
    "alertName": "<alert-name>",
    "namespace": "<namespace>",
    "severity": "<severity>",
    "fingerprint": "<fingerprint>",
    "stormType": "rate|pattern",
    "window": "1m",
    "alertCount": "<count>"
}
```

**Purpose**: Store first signal metadata for aggregated CRD creation

**Business Requirement**: BR-GATEWAY-016 (Storm Aggregation)

---

## 📋 Redis Storage Decision Document (DD-GATEWAY-006)

### Recommendation: Create DD-GATEWAY-006

**Title**: Redis Storage Patterns for Gateway Service

**Content Required**:

1. **Key Naming Conventions**:
   - Prefix: `alert:` for all Gateway keys
   - Namespace: `fingerprint`, `storm`, `pattern`
   - Identifier: Hash, alertname, or window-id

2. **Data Types Used**:
   - **Hash**: Structured data (deduplication metadata, storm metadata)
   - **String**: Simple values (window IDs)
   - **Counter**: Numeric counts (rate detection)
   - **Sorted Set**: Time-series data (pattern detection, resource aggregation)

3. **TTL Strategy**:
   - Deduplication: 5 minutes (configurable, default)
   - Storm rate: 1 minute (sliding window)
   - Storm pattern: 2 minutes (wider window for pattern detection)
   - Storm aggregation: 1 minute (window duration)

4. **Memory Optimization** (per DD-GATEWAY-004):
   - Store lightweight metadata (2KB) instead of full CRDs (30KB)
   - 93% memory reduction achieved
   - Prevents Redis OOM errors

5. **Alternatives Considered**:
   - **Alternative 1**: Store full RemediationRequest CRDs in Redis
     - **Rejected**: 30KB per CRD, causes OOM, 95% waste
   - **Alternative 2**: Store lightweight metadata (2KB)
     - **Approved**: 93% memory reduction, no OOM
   - **Alternative 3**: No Redis storage (in-memory only)
     - **Rejected**: Loses state on restart, no HA support

6. **Consequences**:
   - ✅ Survives Gateway restarts
   - ✅ HA multi-instance support (shared state)
   - ✅ Automatic TTL expiration (no manual cleanup)
   - ✅ 93% memory reduction vs. storing full CRDs
   - ⚠️ ~1ms lookup latency (acceptable trade-off)

---

## 🚨 Critical Issue: Test vs. Schema Mismatch

### Problem

**Integration tests expect**:
```go
stormCRD.Spec.StormAggregation.AlertCount
stormCRD.Spec.StormAggregation.AffectedResources[0].Kind
```

**CRD schema provides**:
```go
stormCRD.Spec.StormAlertCount
stormCRD.Spec.AffectedResources // []string, not []AffectedResource
```

### Impact

**Tests CANNOT pass** with current CRD schema!

Integration tests were written expecting consolidated `StormAggregation` struct that doesn't exist in the CRD.

### Resolution Options

**Option A: Fix CRD Schema to Match Tests** ⭐ **RECOMMENDED**
- Add `StormAggregation` struct to CRD
- Add `AffectedResource` struct with `Kind`, `Name`, `Namespace` fields
- Update 522 references across 74 files
- **Effort**: 2-3 hours
- **Benefit**: Tests pass, cleaner API
- **Risk**: Breaking change, but pre-release

**Option B: Fix Tests to Match CRD Schema**
- Update 11 test references to use scattered fields
- Change `AffectedResources[0].Kind` to parse string
- **Effort**: 30 minutes
- **Benefit**: Quick fix
- **Risk**: Tests become more complex, API stays cluttered

**Option C: Hybrid Approach**
- Keep scattered fields in CRD
- Add helper methods to access as if consolidated
- Update tests to use helper methods
- **Effort**: 1 hour
- **Benefit**: No breaking change
- **Risk**: Additional complexity, helper method maintenance

---

## 📊 Revised Assessment

### Previous Triage Conclusion

**CRD_SCHEMA_CONSOLIDATION_TRIAGE.md** concluded:
- **DEFER consolidation** - Keep scattered fields
- **Rationale**: Works correctly, zero risk, 95% confidence

### New Finding

**Integration tests REQUIRE consolidated struct!**

Tests expect:
- `StormAggregation` struct with nested fields
- `AffectedResource` struct with `Kind`, `Name`, `Namespace`

**Current CRD schema does NOT provide this structure.**

### Revised Recommendation

**CONSOLIDATE CRD SCHEMA** (Option A) ⭐

**New Rationale**:
1. **Tests already written** for consolidated struct (11 references)
2. **Tests CANNOT pass** with current scattered fields
3. **Integration tests are correct** - they test the business requirement (BR-GATEWAY-016)
4. **CRD schema is incorrect** - doesn't match test expectations
5. **Pre-release** - no backward compatibility concerns

**Confidence**: 85% (tests drive the requirement)

---

## 🎯 Action Items

### Immediate Actions

1. **Create DD-GATEWAY-006**: Redis Storage Patterns
   - Document key naming conventions
   - Document data types and TTL strategy
   - Document memory optimization (DD-GATEWAY-004 reference)
   - Document alternatives considered

2. **Fix CRD Schema** (Option A):
   - Add `StormAggregation` struct to CRD
   - Add `AffectedResource` struct
   - Update 522 references across 74 files
   - Run integration tests to verify

3. **Update Phase 2 Plan**:
   - CRD consolidation is **REQUIRED** (not optional)
   - Integration tests already expect consolidated struct
   - Effort: 2-3 hours (as originally estimated)

---

## 📝 Redis Storage Summary Table

| Feature | Redis Keys | Data Stored | TTL | Memory per Key | Business Requirement |
|---------|-----------|-------------|-----|----------------|---------------------|
| **Deduplication** | `alert:fingerprint:<hash>` | Fingerprint, CRD ref, count, timestamps | 5 min | ~200 bytes | BR-GATEWAY-008 |
| **Storm Rate** | `alert:storm:rate:<name>` | Alert count | 1 min | ~50 bytes | BR-GATEWAY-009 |
| **Storm Pattern** | `alert:pattern:<name>` | Resource IDs + timestamps | 2 min | ~500 bytes | BR-GATEWAY-009 |
| **Storm Window** | `alert:storm:aggregate:<name>` | Window ID | 1 min | ~50 bytes | BR-GATEWAY-016 |
| **Storm Resources** | `alert:storm:resources:<id>` | Resource IDs + timestamps | 1 min | ~2KB | BR-GATEWAY-016 |
| **Storm Metadata** | `alert:storm:metadata:<id>` | Signal metadata | 1 min | ~300 bytes | BR-GATEWAY-016 |

**Total Memory (typical storm)**:
- Deduplication: 200 bytes × 100 alerts = 20KB
- Storm detection: 550 bytes × 10 alertnames = 5.5KB
- Storm aggregation: 2.35KB × 5 windows = 11.75KB
- **Total**: ~37KB for 100 alerts with 5 concurrent storms

**Memory Optimization**: 93% reduction vs. storing full CRDs (30KB each = 3MB for 100 alerts)

---

## ✅ Conclusion

### Key Findings

1. **Integration tests USE consolidated `StormAggregation` struct** (11 references)
2. **CRD schema has scattered fields** (5 separate fields)
3. **Mismatch prevents tests from passing**
4. **Redis storage is well-designed** (6 key patterns, optimized memory usage)
5. **DD-GATEWAY-006 needed** to document Redis patterns

### Revised Recommendation

**CONSOLIDATE CRD SCHEMA** (reverse previous decision)

**Rationale**:
- Integration tests are correct (test business requirements)
- CRD schema is incorrect (doesn't match tests)
- Tests cannot pass without consolidated struct
- Pre-release = no backward compatibility concerns

**Confidence**: 85% (tests drive the requirement, not aesthetic preference)

---

**Status**: ✅ **TRIAGE COMPLETE** - CRD consolidation is REQUIRED, not optional

