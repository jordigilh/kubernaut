# Effectiveness Monitor Service - Overview

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ Design Complete (100%)
**Port**: 8080 (REST + Health), 9090 (Metrics)

---

## Table of Contents

1. [Purpose & Scope](#purpose--scope)
2. [Architecture Overview](#architecture-overview)
3. [Assessment Processing Pipeline](#assessment-processing-pipeline)
4. [Key Architectural Decisions](#key-architectural-decisions)
5. [V1 Graceful Degradation Strategy](#v1-graceful-degradation-strategy)
6. [System Context Diagram](#system-context-diagram)
7. [Data Flow Diagram](#data-flow-diagram)
8. [Business Requirements Mapping](#business-requirements-mapping)

---

## Purpose & Scope

### Core Purpose

Effectiveness Monitor Service is the **intelligent assessment engine** that evaluates the effectiveness of remediation actions using **multi-dimensional analysis**. It serves as the **outcome validator** that:

1. **Assesses** traditional effectiveness (success/failure rate)
2. **Correlates** environmental impact (memory, CPU, network metrics)
3. **Detects** adverse side effects from actions
4. **Analyzes** long-term trends (improving/declining/stable)
5. **Recognizes** patterns in effectiveness (temporal, environmental)
6. **Provides** confidence levels based on data quality

### Why Effectiveness Monitor Exists

**Problem**: Without Effectiveness Monitor, Kubernaut would:
- ❌ Repeat ineffective remediation actions indefinitely
- ❌ Miss adverse side effects (e.g., CPU spike after memory fix)
- ❌ Fail to detect declining action effectiveness over time
- ❌ Lack confidence metrics for AI-driven recommendations
- ❌ Have no historical basis for improvement suggestions

**Solution**: Effectiveness Monitor provides **intelligent outcome analysis** that:
- ✅ Identifies ineffective actions before they waste resources
- ✅ Detects side effects within 10 minutes of action execution
- ✅ Tracks long-term trends (90-day rolling window)
- ✅ Provides confidence-adjusted recommendations (60-95% confidence)
- ✅ Enables continuous learning and improvement

---

## Architecture Overview

### Service Characteristics

- **Type**: Stateless HTTP API server (Assessment & Analysis)
- **Deployment**: Kubernetes Deployment with horizontal scaling (2-3 replicas)
- **Data Dependencies**: Data Storage (critical), Infrastructure Monitoring (graceful degradation)
- **Integration Pattern**: On-demand assessment → Multi-dimensional evaluation → Result persistence

### V1 Timeline Strategy

| Week | Data Available | Capability | Confidence |
|------|---------------|------------|------------|
| **Week 5** | 0 weeks | `insufficient_data` response only | 20-30% |
| **Week 8** | 3 weeks | Basic effectiveness scoring | 40-50% |
| **Week 10** | 5 weeks | Side effect detection + trend analysis | 60-70% |
| **Week 13+** | 8+ weeks | Full multi-dimensional assessment | 80-95% |

**Week 5 Deployment**: Service is functional but returns `insufficient_data` status with estimated availability date.

**Week 13+ Goal**: Full capability with high-confidence assessments powered by 8+ weeks of historical data.

### Component Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                  Effectiveness Monitor Service                   │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │              HTTP API Layer (Port 8080)                    │ │
│  │  /api/v1/assess/effectiveness/:actionID                    │ │
│  │  /api/v1/assess/batch                                      │ │
│  │  /health, /ready                                           │ │
│  └────────────────┬───────────────────────────────────────────┘ │
│                   │                                              │
│  ┌────────────────▼───────────────────────────────────────────┐ │
│  │          Data Availability Check (8+ weeks?)               │ │
│  └────────────────┬───────────────────────────────────────────┘ │
│                   │                                              │
│  ┌────────────────▼───────────────────────────────────────────┐ │
│  │     Action History Retrieval (Data Storage Client)         │ │
│  │     - 90-day rolling window                                │ │
│  │     - Action type filtering                                │ │
│  └────────────────┬───────────────────────────────────────────┘ │
│                   │                                              │
│  ┌────────────────▼───────────────────────────────────────────┐ │
│  │  Metrics Correlation (Infrastructure Monitoring Client)    │ │
│  │  - Memory/CPU impact (10-minute post-action window)        │ │
│  │  - Network stability assessment                            │ │
│  │  - Graceful degradation if unavailable                     │ │
│  └────────────────┬───────────────────────────────────────────┘ │
│                   │                                              │
│  ┌────────────────▼───────────────────────────────────────────┐ │
│  │              Effectiveness Calculator                       │ │
│  │  ┌──────────────────────────────────────────────────────┐ │ │
│  │  │ 1. Traditional Score (success rate)                  │ │ │
│  │  │ 2. Side Effect Detection (CPU/memory/network)        │ │ │
│  │  │ 3. Trend Analysis (improving/declining/stable)       │ │ │
│  │  │ 4. Pattern Recognition (temporal, environmental)     │ │ │
│  │  │ 5. Confidence Calculation (data quality)             │ │ │
│  │  └──────────────────────────────────────────────────────┘ │ │
│  └────────────────┬───────────────────────────────────────────┘ │
│                   │                                              │
│  ┌────────────────▼───────────────────────────────────────────┐ │
│  │  Assessment Result Persistence (Data Storage Client)       │ │
│  │  - Best-effort write (log failure, continue)               │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

---

## Assessment Processing Pipeline

### Step-by-Step Flow

#### 1. **Assessment Request** (API Entry Point)

**From Context API Service**:
```http
GET /api/v1/assess/effectiveness/act-abc123
Authorization: Bearer <k8s-serviceaccount-token>
```

**Request Body** (optional parameters):
```json
{
  "action_id": "act-abc123",
  "wait_for_stabilization": true,
  "assessment_interval": "10m"
}
```

#### 2. **Data Availability Check** (10-20ms)

**Query Data Storage** for oldest remediation action:
```go
func (s *EffectivenessMonitorService) checkDataAvailability(ctx context.Context) (int, bool) {
    oldestAction, err := s.dataStorageClient.GetOldestAction(ctx)
    if err != nil {
        return 0, false
    }

    dataWeeks := int(time.Since(oldestAction.CreatedAt).Hours() / (24 * 7))
    sufficient := dataWeeks >= 8 // 8 weeks minimum

    return dataWeeks, sufficient
}
```

**Week 5 Response** (insufficient data):
```json
{
  "assessment_id": "assess-xyz789",
  "status": "insufficient_data",
  "confidence": 0.25,
  "estimated_availability": "2025-11-19T00:00:00Z",
  "message": "Effectiveness assessment requires 8+ weeks of historical data. Current: 0 weeks."
}
```

**Week 13+ Response** (sufficient data) → Continue to next step.

#### 3. **Action History Retrieval** (50-100ms)

**Query Data Storage** for action history (90-day window):
```go
history, err := s.dataStorageClient.GetActionHistory(ctx, "restart-pod", 90*24*time.Hour)

// Returns:
// [
//   {ActionID: "act-001", Status: "success", ExecutedAt: "2025-08-01T10:00:00Z"},
//   {ActionID: "act-002", Status: "success", ExecutedAt: "2025-08-05T14:30:00Z"},
//   {ActionID: "act-003", Status: "failure", ExecutedAt: "2025-08-10T09:15:00Z"},
//   ...
// ]
```

#### 4. **Traditional Effectiveness Calculation** (5-10ms)

```go
func (c *Calculator) CalculateTraditionalScore(history []ActionHistory) float64 {
    successCount := 0
    for _, h := range history {
        if h.Status == "success" {
            successCount++
        }
    }

    if len(history) == 0 {
        return 0.5 // Neutral score
    }

    return float64(successCount) / float64(len(history))
}

// Example: 87 successes out of 100 actions = 0.87 traditional score
```

#### 5. **Environmental Impact Correlation** (100-200ms, graceful degradation)

**Query Infrastructure Monitoring** for metrics after action execution:
```go
metrics, err := s.infraMonitorClient.GetMetricsAfterAction(ctx, "act-abc123", 10*time.Minute)

// Returns:
// {
//   MemoryImprovement: 0.25,  // 25% memory reduction
//   CPUImpact: -0.05,         // 5% CPU increase (negative = adverse)
//   NetworkStability: 0.92,   // 92% network stability
// }
```

**Graceful Degradation** (if Infrastructure Monitoring unavailable):
```go
if err != nil {
    s.logger.Warn("Failed to retrieve environmental metrics, continuing with basic assessment")
    metrics = &EnvironmentalMetrics{} // Default to zero impact
}
```

#### 6. **Side Effect Detection** (5-10ms)

```go
func (c *Calculator) DetectSideEffects(metrics *EnvironmentalMetrics) (bool, string) {
    // Detect negative side effects
    if metrics.CPUImpact < -0.1 || metrics.NetworkStability < 0.7 {
        if metrics.CPUImpact < -0.3 {
            return true, "high" // High severity: >30% CPU increase
        }
        return true, "low" // Low severity: 10-30% CPU increase or network instability
    }
    return false, "none"
}

// Example: CPUImpact = -0.05 (5% increase) → No side effects detected
```

#### 7. **Trend Analysis** (10-20ms)

```go
func (c *Calculator) AnalyzeTrend(history []ActionHistory) string {
    if len(history) < 10 {
        return "insufficient_data"
    }

    // Compare recent (last 5) vs older (previous 5)
    recent := history[len(history)-5:]
    older := history[len(history)-10 : len(history)-5]

    recentSuccess := c.CalculateTraditionalScore(recent)
    olderSuccess := c.CalculateTraditionalScore(older)

    if recentSuccess > olderSuccess+0.1 {
        return "improving" // 10%+ improvement
    } else if recentSuccess < olderSuccess-0.1 {
        return "declining" // 10%+ decline
    }
    return "stable"
}

// Example: Recent = 0.92, Older = 0.78 → "improving"
```

#### 8. **Pattern Recognition** (20-30ms)

```go
func (c *Calculator) GeneratePatternInsights(history []ActionHistory, action *ActionData) []string {
    insights := []string{}

    // Pattern 1: Environment-specific success rate
    prodSuccessRate := c.getEnvironmentSuccessRate(history, "production")
    if prodSuccessRate > 0.8 {
        insights = append(insights, fmt.Sprintf(
            "Similar actions successful in %.0f%% of production cases", prodSuccessRate*100))
    }

    // Pattern 2: Temporal correlation
    if c.hasBusinessHoursCorrelation(history) {
        insights = append(insights, "Effectiveness 12% lower during business hours")
    }

    return insights
}

// Example output:
// [
//   "Similar actions successful in 87% of production cases",
//   "Effectiveness 12% lower during business hours"
// ]
```

#### 9. **Confidence Calculation** (5ms)

```go
func (c *Calculator) CalculateConfidence(history []ActionHistory, dataWeeks int) float64 {
    baseConfidence := 0.5

    // Increase confidence with more data weeks
    if dataWeeks >= 12 {
        baseConfidence = 0.9
    } else if dataWeeks >= 8 {
        baseConfidence = 0.8
    }

    // Increase confidence with more action history
    if len(history) > 100 {
        baseConfidence += 0.05
    }

    if baseConfidence > 0.95 {
        baseConfidence = 0.95 // Cap at 95%
    }

    return baseConfidence
}

// Example: 10 weeks of data + 120 actions in history = 0.85 confidence
```

#### 10. **Assessment Result Persistence** (50-100ms, best-effort)

**Write to Data Storage**:
```go
assessment := &EffectivenessScore{
    AssessmentID:        "assess-xyz789",
    ActionID:            "act-abc123",
    ActionType:          "restart-pod",
    TraditionalScore:    0.87,
    EnvironmentalImpact: EnvironmentalMetrics{
        MemoryImprovement: 0.25,
        CPUImpact:         -0.05,
        NetworkStability:  0.92,
    },
    Confidence:          0.85,
    Status:              "assessed",
    SideEffectsDetected: false,
    SideEffectSeverity:  "none",
    TrendDirection:      "improving",
    PatternInsights:     []string{
        "Similar actions successful in 87% of production cases",
    },
    AssessedAt:          time.Now(),
}

// Best-effort write (log failure, continue)
if err := s.dataStorageClient.PersistAssessment(ctx, assessment); err != nil {
    s.logger.Error("Failed to persist assessment, continuing", zap.Error(err))
}
```

#### 11. **Response** (HTTP 200 OK)

**Week 13+ Full Assessment Response**:
```json
{
  "assessment_id": "assess-xyz789",
  "action_id": "act-abc123",
  "action_type": "restart-pod",
  "traditional_score": 0.87,
  "environmental_impact": {
    "memory_improvement": 0.25,
    "cpu_impact": -0.05,
    "network_stability": 0.92
  },
  "confidence": 0.85,
  "status": "assessed",
  "side_effects_detected": false,
  "side_effect_severity": "none",
  "trend_direction": "improving",
  "pattern_insights": [
    "Similar actions successful in 87% of production cases"
  ],
  "assessed_at": "2025-10-06T10:15:30Z"
}
```

**Total Latency**: 250-500ms (typical), < 5s p95

---

## Key Architectural Decisions

### 1. **V1 Graceful Degradation Strategy**

**Decision**: Deploy service in Week 5 with `insufficient_data` responses, gradually enable features as data accumulates.

**Why**:
- ✅ Early deployment validates integration points
- ✅ Monitoring and observability operational from Day 1
- ✅ Smooth transition to full capability (no "big bang" cutover)
- ✅ User expectations set via clear messaging

**Alternative Rejected**: Wait until Week 13 to deploy (delays validation).

---

### 2. **Infrastructure Monitoring is Non-Critical**

**Decision**: Use circuit breaker pattern with graceful degradation for Infrastructure Monitoring Service.

**Why**:
- ✅ Environmental metrics enhance assessments but aren't required
- ✅ Basic effectiveness scoring works without metrics
- ✅ Prevents cascading failures if Infrastructure Monitoring down
- ✅ Degrades user experience gracefully (no assessment failures)

**Implementation**:
```go
metrics, err := circuitBreaker.Call(ctx, func(ctx context.Context) (*EnvironmentalMetrics, error) {
    return s.infraMonitorClient.GetMetricsAfterAction(ctx, actionID, 10*time.Minute)
})
// If circuit open or call fails → return default zero-impact metrics
```

---

### 3. **Data Storage is Critical Dependency**

**Decision**: Fail assessment requests if Data Storage unavailable.

**Why**:
- ✅ Action history is **required** for effectiveness calculation
- ✅ Cannot provide meaningful assessments without historical data
- ✅ Fail-fast prevents misleading results
- ✅ Clear error messaging guides troubleshooting

**Error Response** (Data Storage unavailable):
```json
{
  "error": "data_storage_unavailable",
  "message": "Cannot retrieve action history from Data Storage Service",
  "retry_after": "60s"
}
```

---

### 4. **Assessment Persistence is Best-Effort**

**Decision**: Log error but continue if assessment persistence fails.

**Why**:
- ✅ Assessment result already returned to client
- ✅ Retrying write could cause request timeout
- ✅ Historical assessments are "nice to have" not critical
- ✅ Monitoring alerts on persistence failures

---

### 5. **8-Week Minimum for High Confidence**

**Decision**: Require 8+ weeks of historical data for confidence ≥ 80%.

**Why**:
- ✅ Statistical significance: 8 weeks ≈ 50-200 actions (depending on frequency)
- ✅ Captures seasonal patterns (e.g., business hours vs off-hours)
- ✅ Reduces impact of outliers and anomalies
- ✅ Industry standard for trend analysis

**Confidence Levels**:
- **< 5 weeks**: 30-50% confidence (insufficient data)
- **5-7 weeks**: 50-70% confidence (basic assessment)
- **8-11 weeks**: 70-85% confidence (good assessment)
- **12+ weeks**: 85-95% confidence (excellent assessment)

---

## V1 Graceful Degradation Strategy

### Deployment Timeline

| Week | Milestone | Capability | User Experience |
|------|-----------|------------|-----------------|
| **Week 5** | Service Deployed | `insufficient_data` responses only | Clear messaging: "Data accumulating, ETA Week 13" |
| **Week 8** | 3 weeks of data | Basic traditional scoring | 40-50% confidence, no trends/patterns |
| **Week 10** | 5 weeks of data | Side effect detection + trends | 60-70% confidence, limited pattern recognition |
| **Week 13** | 8 weeks of data | **Full capability** | 80-95% confidence, complete multi-dimensional assessment |
| **Week 16+** | 11+ weeks of data | Enhanced confidence | 90-95% confidence, mature pattern insights |

### Week 5 Implementation

**Health Check** (Week 5):
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
# Ready = "service operational" (even if returning insufficient_data)
```

**`/ready` Endpoint** (Week 5):
```go
func (s *EffectivenessMonitorService) readinessHandler(w http.ResponseWriter, r *http.Request) {
    // Check critical dependencies
    if !s.dataStorageClient.Healthy() {
        http.Error(w, "Data Storage unavailable", http.StatusServiceUnavailable)
        return
    }

    // Check data availability (informational only)
    dataWeeks, _ := s.checkDataAvailability(r.Context())

    response := map[string]interface{}{
        "status": "ready",
        "data_weeks": dataWeeks,
        "full_capability": dataWeeks >= 8,
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}
```

**Metrics** (Week 5):
```go
var (
    dataAvailabilityWeeks = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "effectiveness_data_availability_weeks",
            Help: "Number of weeks of historical data available",
        },
    )

    insufficientDataResponses = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "effectiveness_insufficient_data_responses_total",
            Help: "Total number of insufficient_data responses",
        },
    )
)
```

### Week 13+ Full Capability

**All features operational**:
- ✅ Traditional effectiveness scoring
- ✅ Environmental impact correlation
- ✅ Side effect detection
- ✅ Trend analysis (improving/declining/stable)
- ✅ Pattern recognition (temporal, environmental)
- ✅ High confidence (80-95%)

---

## System Context Diagram

```
┌───────────────────────────────────────────────────────────────────┐
│                     External Context                              │
│                                                                   │
│  ┌─────────────────┐                 ┌─────────────────┐        │
│  │   Context API   │◄────────────────┤  HolmesGPT API  │        │
│  │   Service       │  GET assessment │   Service       │        │
│  │   (Port 8080)   │                 │   (Port 8090)   │        │
│  └────────┬────────┘                 └─────────────────┘        │
│           │                                                       │
│           │ Assessment Requests                                  │
│           │ (HTTP GET /api/v1/assess/effectiveness/:actionID)    │
│           │                                                       │
└───────────┼───────────────────────────────────────────────────────┘
            │
            ▼
┌───────────────────────────────────────────────────────────────────┐
│            Effectiveness Monitor Service (Port 8080)              │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  HTTP API                                                  │ │
│  │  - POST /api/v1/assess/effectiveness                       │ │
│  │  - GET  /api/v1/assess/effectiveness/:actionID             │ │
│  │  - POST /api/v1/assess/batch                               │ │
│  │  - GET  /health, /ready                                    │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Effectiveness Calculator                                  │ │
│  │  - Traditional Score Calculation                           │ │
│  │  - Side Effect Detection                                   │ │
│  │  - Trend Analysis                                          │ │
│  │  - Pattern Recognition                                     │ │
│  │  - Confidence Calculation                                  │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                   │
└─────────┬───────────────────────────────────────┬─────────────────┘
          │                                       │
          │ Action History                        │ Environmental Metrics
          │ (GET /api/v1/audit/actions)           │ (GET /api/v1/metrics/after-action)
          │                                       │
          ▼                                       ▼
┌─────────────────────────┐          ┌─────────────────────────────┐
│  Data Storage Service   │          │ Infrastructure Monitoring   │
│  (Port 8085)            │          │ Service (Port 8094)         │
│                         │          │                             │
│  - Action history       │          │  - CPU/Memory metrics       │
│  - Assessment storage   │          │  - Network stability        │
│  - PostgreSQL + pgvector│          │  - Prometheus queries       │
│                         │          │  - Graceful degradation     │
└─────────────────────────┘          └─────────────────────────────┘
```

---

## Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                 Assessment Request Flow                         │
└─────────────────────────────────────────────────────────────────┘

1. Context API → Effectiveness Monitor
   GET /api/v1/assess/effectiveness/act-abc123
   Authorization: Bearer <token>

   ↓

2. Effectiveness Monitor → Data Storage
   GET /api/v1/audit/actions/oldest
   (Check data availability: 10 weeks ✓)

   ↓

3. Effectiveness Monitor → Data Storage
   GET /api/v1/audit/actions?action_type=restart-pod&time_range=90d
   Returns: 120 actions (87 success, 33 failure)

   ↓

4. Calculate Traditional Score
   87 / 120 = 0.725 (72.5% success rate)

   ↓

5. Effectiveness Monitor → Infrastructure Monitoring
   GET /api/v1/metrics/after-action?action_id=act-abc123&window=10m
   Returns: {
     memory_improvement: 0.25,
     cpu_impact: -0.05,
     network_stability: 0.92
   }

   ↓

6. Side Effect Detection
   CPU impact = -5% (minimal) → No side effects

   ↓

7. Trend Analysis
   Recent 5: 0.80, Older 5: 0.68 → "improving"

   ↓

8. Pattern Recognition
   Production success rate: 87% → Pattern insight

   ↓

9. Confidence Calculation
   10 weeks data + 120 actions → 0.85 confidence

   ↓

10. Effectiveness Monitor → Data Storage
    POST /api/v1/audit/effectiveness
    (Persist assessment result - best effort)

    ↓

11. Effectiveness Monitor → Context API
    HTTP 200 OK
    {
      "assessment_id": "assess-xyz789",
      "traditional_score": 0.725,
      "confidence": 0.85,
      "trend_direction": "improving",
      ...
    }
```

---

## Business Requirements Mapping

| Business Requirement | Implementation | Validation |
|---------------------|----------------|------------|
| **BR-INS-001**: Traditional effectiveness assessment | `CalculateTraditionalScore()` | Unit test: success rate calculation |
| **BR-INS-002**: Environmental impact correlation | `GetMetricsAfterAction()` | Integration test: Infrastructure Monitoring query |
| **BR-INS-003**: Long-term trend detection | `AnalyzeTrend()` | Unit test: improving/declining/stable detection |
| **BR-INS-005**: Side effect detection | `DetectSideEffects()` | Unit test: CPU/memory/network severity classification |
| **BR-INS-006**: Advanced pattern recognition | `GeneratePatternInsights()` | Unit test: temporal & environmental patterns |
| **BR-INS-008**: Confidence-adjusted scoring | `CalculateConfidence()` | Unit test: confidence levels by data availability |
| **BR-INS-010**: Multi-dimensional assessment | `AssessEffectiveness()` | E2E test: complete assessment workflow |

**BR Range Allocation**:
- **V1 Scope**: BR-INS-001 to BR-INS-010 (Core effectiveness assessment with graceful degradation strategy)
- **Reserved for V2**: BR-INS-011 to BR-INS-100 (Multi-cloud support: AWS CloudWatch, Azure Monitor, Datadog, GCP Monitoring; Advanced ML-based effectiveness prediction)

**Rationale for Limited V1 Scope**: Effectiveness Monitor focuses on core assessment capabilities for Kubernetes-only environments, with graceful degradation (Week 5: insufficient data → Week 13+: full capability). V2 will expand to multi-cloud observability sources and ML-based prediction models.

---

## Performance Targets

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Assessment Latency** | < 5s p95 | Prometheus histogram: `effectiveness_assessment_duration_seconds` |
| **Data Storage Query** | < 100ms p95 | Internal metric: `data_storage_query_duration_seconds` |
| **Infrastructure Monitoring Query** | < 200ms p95 | Internal metric: `infrastructure_monitoring_query_duration_seconds` |
| **Confidence Threshold** | ≥ 80% | With 8+ weeks of data |
| **Data Availability** | 8+ weeks | For full capability |

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

