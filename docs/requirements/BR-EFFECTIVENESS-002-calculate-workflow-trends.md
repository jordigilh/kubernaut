# BR-EFFECTIVENESS-002: Calculate Playbook Effectiveness Trends

> **ARCHIVED** (February 2026)
>
> This BR describes trend calculation and REST API design that has been **superseded** by DD-017 v2.0.
> Trend calculation and playbook effectiveness scoring are Level 2 capabilities (V1.1), not Level 1.
> DD-017 v2.0 defines the authoritative EM Level 1 scope: automated health checks, metric comparison,
> formula-based scoring, and audit event emission.
>
> **Authoritative source**: `docs/architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md` (v2.0)
> **V1.0 BRs**: BR-INS-001, BR-INS-002, BR-INS-005 (from DD-017 v2.0)

**Business Requirement ID**: BR-EFFECTIVENESS-002
**Category**: Effectiveness Monitor Service
**Priority**: P1
**Target Version**: V1
**Status**: ⚠️ ARCHIVED — Superseded by DD-017 v2.0
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 requires continuous learning feedback loops to optimize workflow selection and identify degrading playbooks. The Effectiveness Monitor must analyze historical success rate data (collected via BR-EFFECTIVENESS-001) to calculate trends, identify statistically significant changes, and generate effectiveness scores for each playbook.

**Current Limitations**:
- ❌ No automated trend analysis of playbook effectiveness
- ❌ Cannot detect statistically significant effectiveness degradation
- ❌ Manual comparison required to identify improving vs declining playbooks
- ❌ No effectiveness scoring system for playbook ranking
- ❌ Cannot prioritize playbook improvement efforts

**Impact**:
- Degrading playbooks go undetected (e.g., 89% → 60% success rate)
- No data-driven prioritization of playbook improvements
- Missing foundation for automated AI feedback (BR-EFFECTIVENESS-003)
- Teams lack actionable insights for playbook optimization

---

## 🎯 **Business Objective**

**Analyze historical success rate trends to calculate playbook effectiveness scores, identify improving/declining playbooks, and provide actionable recommendations for playbook optimization.**

### **Success Criteria**
1. ✅ Effectiveness Monitor calculates trend direction (improving/stable/declining) for all playbooks
2. ✅ Detects statistically significant changes (>10% success rate change)
3. ✅ Generates effectiveness scores (0.0-1.0) per playbook
4. ✅ Compares current vs previous period success rates (7d vs 7d, 30d vs 30d)
5. ✅ Identifies top 10 improving and declining playbooks
6. ✅ Provides REST API to query trend analysis results
7. ✅ Dashboard displays trend analysis with actionable recommendations

---

## 📊 **Use Cases**

### **Use Case 1: Detect Significant Playbook Degradation**

**Scenario**: `pod-oom-recovery v1.2` success rate drops from 89% to 60% (29% drop - statistically significant).

**Current Flow** (Without BR-EFFECTIVENESS-002):
```
1. Historical data collected (BR-EFFECTIVENESS-001)
2. No automated trend analysis
3. ❌ Degradation visible in raw data but not actionable
4. ❌ Team must manually analyze trends
5. ❌ Delayed response to degradation
```

**Desired Flow with BR-EFFECTIVENESS-002**:
```
1. Historical data collected every 5 minutes
2. Effectiveness Monitor calculates trend (daily at midnight):
   - Current period (7d): 60% success (50 executions)
   - Previous period (7d): 89% success (90 executions)
   - Change: -29% (SIGNIFICANT: exceeds 10% threshold)
   - Trend direction: "declining"
   - Effectiveness score: 0.60 (low)
3. Effectiveness Monitor stores trend analysis:
   {
     "playbook_id": "pod-oom-recovery",
     "playbook_version": "v1.2",
     "trend_direction": "declining",
     "change_percent": -32.6,
     "current_success_rate": 0.60,
     "previous_success_rate": 0.89,
     "effectiveness_score": 0.60,
     "confidence": "high",
     "statistically_significant": true,
     "recommendation": "URGENT: Investigate degradation, consider v1.3 or deprecation"
   }
4. ✅ Dashboard displays alert: "pod-oom-recovery v1.2 degraded by 32.6%"
5. ✅ Team investigates immediately (automated detection)
6. ✅ Actionable recommendation provided
```

---

### **Use Case 2: Identify Top Improving Playbooks**

**Scenario**: Operations team wants to identify which playbooks have improved most over last 30 days.

**Current Flow**:
```
1. Team wants to find improving playbooks
2. No trend analysis available
3. ❌ Manual query and analysis of historical data
4. ❌ Time-consuming process
```

**Desired Flow with BR-EFFECTIVENESS-002**:
```
1. Team queries Effectiveness Monitor:
   GET /api/v1/effectiveness/analysis/top-improving?time_range=30d&limit=10
2. Effectiveness Monitor returns top 10 improving playbooks:
   [
     {
       "playbook_id": "database-recovery",
       "playbook_version": "v2.0",
       "trend_direction": "improving",
       "change_percent": +45.0,  // 50% → 95%
       "effectiveness_score": 0.95,
       "recommendation": "PROMOTE: Consider as default for database incidents"
     },
     {
       "playbook_id": "pod-oom-recovery",
       "playbook_version": "v1.3",
       "trend_direction": "improving",
       "change_percent": +25.0,  // 70% → 95%
       "effectiveness_score": 0.93,
       "recommendation": "PROMOTE: Consider deprecating v1.2"
     },
     ...
   ]
3. ✅ Dashboard displays "Top Improving Playbooks" chart
4. ✅ Team promotes v1.3 as default
5. ✅ Team deprecates v1.2
6. ✅ Data-driven playbook lifecycle management
```

---

### **Use Case 3: Effectiveness Score-Based Playbook Ranking**

**Scenario**: AI needs to rank 5 playbooks for `pod-oom-killer` incident type by overall effectiveness.

**Current Flow**:
```
1. AI queries workflow success rates (5 playbooks)
2. No effectiveness scoring system
3. ❌ AI uses raw success rate only (ignores trends, execution volume, confidence)
4. ❌ May select degrading playbook with temporarily high success rate
```

**Desired Flow with BR-EFFECTIVENESS-002**:
```
1. AI queries Effectiveness Monitor:
   GET /api/v1/effectiveness/analysis/playbooks?incident_type=pod-oom-killer
2. Effectiveness Monitor returns effectiveness-ranked playbooks:
   [
     {
       "playbook_id": "pod-oom-recovery",
       "playbook_version": "v1.3",
       "effectiveness_score": 0.95,  // HIGHEST
       "success_rate": 0.93,
       "trend_direction": "improving",
       "execution_volume": 150,
       "confidence": "high"
     },
     {
       "playbook_id": "pod-oom-recovery",
       "playbook_version": "v1.2",
       "effectiveness_score": 0.60,  // LOW (declining trend)
       "success_rate": 0.60,
       "trend_direction": "declining",
       "execution_volume": 50,
       "confidence": "medium"
     },
     ...
   ]
3. ✅ AI selects v1.3 (highest effectiveness score)
4. ✅ AI avoids v1.2 (declining trend despite being in catalog)
5. ✅ Trend-aware workflow selection
```

---

## 🔧 **Functional Requirements**

### **FR-EFFECTIVENESS-002-01: Trend Direction Calculation**

**Requirement**: Effectiveness Monitor SHALL calculate trend direction by comparing current vs previous period success rates.

**Trend Classification**:
```go
package effectivenessmonitor

// TrendDirection represents playbook effectiveness trend
type TrendDirection string

const (
    TrendImproving TrendDirection = "improving" // Success rate increased >5%
    TrendStable    TrendDirection = "stable"    // Success rate changed <5%
    TrendDeclining TrendDirection = "declining" // Success rate decreased >5%
)

// CalculateTrendDirection compares current vs previous period
func CalculateTrendDirection(currentRate, previousRate float64) TrendDirection {
    changePercent := ((currentRate - previousRate) / previousRate) * 100

    if changePercent > 5.0 {
        return TrendImproving
    } else if changePercent < -5.0 {
        return TrendDeclining
    } else {
        return TrendStable
    }
}
```

**Acceptance Criteria**:
- ✅ `improving`: Success rate increased >5%
- ✅ `stable`: Success rate changed <5%
- ✅ `declining`: Success rate decreased >5%
- ✅ Handles zero previous rate gracefully (returns "stable")
- ✅ Calculates change_percent as `((current - previous) / previous) * 100`

---

### **FR-EFFECTIVENESS-002-02: Effectiveness Score Calculation**

**Requirement**: Effectiveness Monitor SHALL calculate effectiveness score incorporating success rate, trend, execution volume, and confidence.

**Effectiveness Score Formula**:
```go
// CalculateEffectivenessScore generates 0.0-1.0 score
func CalculateEffectivenessScore(successRate float64, trend TrendDirection, executions int, confidence string) float64 {
    baseScore := successRate // 0.0-1.0

    // Trend adjustment
    trendBonus := 0.0
    switch trend {
    case TrendImproving:
        trendBonus = 0.05 // +5% for improving trend
    case TrendDeclining:
        trendBonus = -0.10 // -10% for declining trend (penalty)
    case TrendStable:
        trendBonus = 0.0
    }

    // Execution volume adjustment (confidence in data)
    volumeBonus := 0.0
    if executions >= 100 {
        volumeBonus = 0.05 // +5% for high volume
    } else if executions >= 50 {
        volumeBonus = 0.03 // +3% for medium volume
    } else if executions >= 20 {
        volumeBonus = 0.01 // +1% for low volume
    }
    // < 20 executions: no bonus

    // Confidence adjustment
    confidenceBonus := 0.0
    switch confidence {
    case "high":
        confidenceBonus = 0.02 // +2% for high confidence
    case "medium":
        confidenceBonus = 0.0
    case "low":
        confidenceBonus = -0.05 // -5% for low confidence (penalty)
    }

    // Calculate final score (capped at 1.0)
    effectivenessScore := baseScore + trendBonus + volumeBonus + confidenceBonus
    return math.Min(1.0, math.Max(0.0, effectivenessScore))
}
```

**Example Scores**:
| Success Rate | Trend | Executions | Confidence | Effectiveness Score |
|---|---|---|---|---|
| 0.90 | improving | 150 | high | 1.00 (capped) |
| 0.89 | stable | 90 | high | 0.96 |
| 0.60 | declining | 50 | medium | 0.53 (penalty applied) |
| 0.40 | stable | 10 | low | 0.30 |

**Acceptance Criteria**:
- ✅ Score range: 0.0-1.0
- ✅ Declining trend applies -10% penalty
- ✅ Improving trend applies +5% bonus
- ✅ High execution volume applies +5% bonus
- ✅ Low confidence applies -5% penalty

---

### **FR-EFFECTIVENESS-002-03: Statistical Significance Detection**

**Requirement**: Effectiveness Monitor SHALL detect statistically significant changes (>10% change with sufficient sample size).

**Implementation**:
```go
// IsStatisticallySignificant determines if trend change is significant
func IsStatisticallySignificant(currentRate, previousRate float64, currentExecutions, previousExecutions int) bool {
    // Requirement 1: Minimum 20 executions in each period
    if currentExecutions < 20 || previousExecutions < 20 {
        return false // Insufficient data
    }

    // Requirement 2: >10% absolute change
    changePercent := math.Abs(((currentRate - previousRate) / previousRate) * 100)
    if changePercent < 10.0 {
        return false // Change too small
    }

    return true
}
```

**Acceptance Criteria**:
- ✅ Requires ≥20 executions in both current and previous periods
- ✅ Requires >10% absolute change in success rate
- ✅ Returns `false` if insufficient data
- ✅ Returns `true` if both requirements met

---

### **FR-EFFECTIVENESS-002-04: Trend Analysis API**

**Requirement**: Effectiveness Monitor SHALL provide REST API to query trend analysis results.

**API Specification**:
```http
GET /api/v1/effectiveness/analysis/playbooks

Query Parameters:
- incident_type (string, optional): Filter by incident type
- min_effectiveness_score (float, optional): Minimum effectiveness score threshold
- trend_direction (string, optional): Filter by trend (improving/stable/declining)
- limit (int, optional, default: 50): Maximum results

Response (200 OK):
[
  {
    "playbook_id": "pod-oom-recovery",
    "playbook_version": "v1.3",
    "incident_types": ["pod-oom-killer", "container-memory-pressure"],
    "effectiveness_score": 0.95,
    "current_success_rate": 0.93,
    "previous_success_rate": 0.70,
    "trend_direction": "improving",
    "change_percent": +32.9,
    "current_executions": 150,
    "previous_executions": 100,
    "confidence": "high",
    "statistically_significant": true,
    "recommendation": "PROMOTE: Consider as default for incident type"
  },
  ...
]

---

GET /api/v1/effectiveness/analysis/top-improving

Query Parameters:
- time_range (string, optional, default: "30d"): Comparison window
- limit (int, optional, default: 10): Top N results

Response (200 OK):
[
  {
    "playbook_id": "database-recovery",
    "playbook_version": "v2.0",
    "effectiveness_score": 0.95,
    "change_percent": +45.0,
    "trend_direction": "improving",
    "recommendation": "PROMOTE: Significant improvement detected"
  },
  ...
]

---

GET /api/v1/effectiveness/analysis/top-declining

Query Parameters:
- time_range (string, optional, default: "30d")
- limit (int, optional, default: 10)

Response (200 OK):
[
  {
    "playbook_id": "pod-oom-recovery",
    "playbook_version": "v1.2",
    "effectiveness_score": 0.53,
    "change_percent": -32.6,
    "trend_direction": "declining",
    "recommendation": "URGENT: Investigate degradation or deprecate"
  },
  ...
]
```

**Acceptance Criteria**:
- ✅ Returns 200 OK for valid queries
- ✅ Filters by incident_type, min_effectiveness_score, trend_direction
- ✅ Results sorted by effectiveness_score DESC
- ✅ Top-improving sorted by change_percent DESC
- ✅ Top-declining sorted by change_percent ASC (most negative)

---

## 📈 **Non-Functional Requirements**

### **NFR-EFFECTIVENESS-002-01: Performance**

- ✅ Trend calculation runs daily at midnight (batch job)
- ✅ Batch job completes within 10 minutes (500+ playbooks)
- ✅ API query response time <200ms

### **NFR-EFFECTIVENESS-002-02: Accuracy**

- ✅ Trend calculation uses 7-day rolling windows (current 7d vs previous 7d)
- ✅ Effectiveness score accurately reflects playbook quality
- ✅ Statistical significance detection prevents false positives (<5% false positive rate)

### **NFR-EFFECTIVENESS-002-03: Observability**

- ✅ Prometheus metrics: `effectiveness_monitor_trend_calculations_total{status="success|error"}`
- ✅ Log all statistically significant changes (for audit trail)
- ✅ Alert if >10 playbooks show declining trend simultaneously (potential infrastructure issue)

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Defines continuous learning feedback loops
- ✅ BR-EFFECTIVENESS-001: Provides historical trend data
- ✅ PostgreSQL: Stores historical data

### **Downstream Impacts**
- ✅ BR-EFFECTIVENESS-003: Uses trend analysis to trigger learning feedback loops
- ✅ BR-AI-057: AI uses effectiveness scores for workflow selection
- ✅ Operations Dashboard: Displays trend analysis charts

---

## 🚀 **Implementation Phases**

### **Phase 1: Trend Calculation Logic** (Day 16 - 4 hours)
- Implement `CalculateTrendDirection()`
- Implement `CalculateEffectivenessScore()`
- Implement `IsStatisticallySignificant()`
- Unit tests (20+ test cases)

### **Phase 2: Batch Job** (Day 17 - 4 hours)
- Implement daily batch job (runs at midnight)
- Query historical data from PostgreSQL
- Calculate trends for all playbooks
- Store trend analysis results

### **Phase 3: Trend Analysis API** (Day 17 - 4 hours)
- Implement `GET /analysis/playbooks` endpoint
- Implement `GET /analysis/top-improving` endpoint
- Implement `GET /analysis/top-declining` endpoint
- Integration tests

### **Phase 4: Monitoring & Alerting** (Day 18 - 2 hours)
- Add Prometheus metrics for batch job success/failure
- Add alerting for simultaneous declining trends (>10 playbooks)
- Dashboard for trend analysis visualization

**Total Estimated Effort**: 14 hours (1.75 days)

---

## 📊 **Success Metrics**

### **Trend Calculation Success Rate**
- **Target**: 100% of batch jobs succeed
- **Measure**: `effectiveness_monitor_trend_calculations_total{status="success"}` / total

### **Actionable Insights Generated**
- **Target**: 20+ playbooks with statistically significant changes per week
- **Measure**: Count playbooks with `statistically_significant=true`

### **Dashboard Usage**
- **Target**: 50+ queries per day to trend analysis API
- **Measure**: Track API request count

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Real-Time Trend Calculation**

**Approach**: Calculate trends on-demand for each API query

**Rejected Because**:
- ❌ Expensive: Re-calculates trends for every query
- ❌ High database load
- ❌ Slower API response time

---

### **Alternative 2: Simple Success Rate Comparison (No Effectiveness Score)**

**Approach**: Use only success rate, ignore trends/volume/confidence

**Rejected Because**:
- ❌ Less accurate playbook ranking
- ❌ Cannot distinguish degrading playbooks with temporarily high success rate
- ❌ No incorporation of execution volume confidence

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P1 priority (enables data-driven playbook optimization)
**Rationale**: Required for automated detection of playbook degradation and continuous improvement
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Workflow Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-EFFECTIVENESS-001: Consume success rate data (provides historical data)
- BR-EFFECTIVENESS-003: Trigger learning feedback loops (consumes trend analysis)
- BR-AI-057: AI uses effectiveness scores for workflow selection

### **Related Documents**
- [ADR-033: Remediation Workflow Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

