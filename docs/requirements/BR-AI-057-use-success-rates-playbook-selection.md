# BR-AI-057: Use Success Rates for Playbook Selection

**Business Requirement ID**: BR-AI-057
**Category**: AI/LLM Service
**Priority**: P0
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces **data-driven playbook selection** as the primary AI capability (90% of Hybrid Model). The AI/LLM Service must query historical success rates from the Context API and use this data to select the most effective playbook for each incident type.

**Current Limitations**:
- ❌ AI lacks access to historical remediation effectiveness data
- ❌ Playbook selection is based on heuristics or random selection
- ❌ AI cannot learn from past remediation successes/failures
- ❌ No data-driven optimization of playbook selection
- ❌ Cannot validate if selected playbook historically works for this incident type

**Impact**:
- Suboptimal playbook selection (low success rate)
- AI cannot fulfill ADR-033 Hybrid Model (90% catalog-based selection)
- No continuous improvement through data-driven learning
- Users lack confidence in AI-selected remediations

---

## 🎯 **Business Objective**

**Enable AI Service to query incident-type and playbook success rates from Context API and use this data to select the most effective playbook for each remediation request.**

### **Success Criteria**
1. ✅ AI queries Context API for incident-type success rates
2. ✅ AI queries Playbook Catalog for available playbooks matching incident type
3. ✅ AI selects playbook with highest success rate for that incident type
4. ✅ AI includes success rate in confidence score calculation
5. ✅ AI logs playbook selection rationale (data-driven decision)
6. ✅ 90%+ of AI remediation decisions use data-driven playbook selection (ADR-033 target)
7. ✅ Measurable improvement in remediation success rate (10%+ increase)

---

## 📊 **Use Cases**

### **Use Case 1: Data-Driven Playbook Selection for Known Incident**

**Scenario**: AI receives `pod-oom-killer` alert and selects playbook based on historical success rates.

**Current Flow** (Without BR-AI-057):
```
1. AI receives pod-oom-killer alert
2. AI queries Playbook Catalog: GET /playbooks?incident_type=pod-oom-killer
3. Response: [pod-oom-recovery v1.2, pod-oom-recovery v1.1, pod-oom-vertical-scaling v1.0]
4. ❌ AI has no success rate data
5. ❌ AI selects based on heuristics (e.g., latest version) or random
6. ❌ May select ineffective playbook (e.g., v1.1 with 40% success rate)
7. Low remediation success rate
```

**Desired Flow with BR-AI-057**:
```
1. AI receives pod-oom-killer alert
2. AI queries Playbook Catalog: GET /playbooks?incident_type=pod-oom-killer
3. Response: [pod-oom-recovery v1.2, pod-oom-recovery v1.1, pod-oom-vertical-scaling v1.0]
4. AI queries Context API for success rates:
   - GET /incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2
     → Response: success_rate=0.89, total_executions=90
   - GET /incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.1
     → Response: success_rate=0.40, total_executions=10
   - GET /incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-vertical-scaling&playbook_version=v1.0
     → Response: success_rate=0.75, total_executions=20
5. ✅ AI selects pod-oom-recovery v1.2 (highest success rate: 89%)
6. ✅ AI confidence score includes success rate: confidence=0.92 (0.89 success rate + 0.03 execution volume bonus)
7. ✅ AI logs decision: "Selected pod-oom-recovery v1.2 (89% success rate, 90 executions)"
8. ✅ Higher remediation success rate (data-driven selection)
```

---

### **Use Case 2: Insufficient Historical Data - Graceful Degradation**

**Scenario**: AI receives `rare-database-failure` alert with no historical success rate data.

**Current Flow**:
```
1. AI receives rare-database-failure alert
2. AI queries Context API for success rates
3. ❌ No historical data available (0 executions)
4. ❌ AI has no fallback strategy
5. ❌ AI fails or selects randomly
```

**Desired Flow with BR-AI-057**:
```
1. AI receives rare-database-failure alert
2. AI queries Playbook Catalog: 1 playbook found (database-recovery v1.0)
3. AI queries Context API: success_rate=0.0, total_executions=0 (no data)
4. ✅ AI detects insufficient data (min_samples_met=false)
5. ✅ AI gracefully degrades to fallback selection strategy:
   - Option A: Use playbook tagged as "default" for this incident type
   - Option B: Recommend manual investigation (ADR-033 <1% manual path)
   - Option C: Use playbook with most similar incident type success data
6. ✅ AI logs decision: "Selected database-recovery v1.0 (no historical data, using default)"
7. ✅ AI confidence score reflects uncertainty: confidence=0.50 (low confidence)
8. ✅ Operator receives recommendation with low confidence warning
```

---

### **Use Case 3: Continuous Learning - Success Rate Decay**

**Scenario**: `pod-oom-recovery v1.2` had 89% success rate last week, but dropped to 60% this week due to infrastructure changes.

**Current Flow**:
```
1. AI selects playbook based on last week's data
2. ❌ AI unaware of recent effectiveness drop
3. ❌ Continues selecting ineffective playbook
4. ❌ Poor remediation outcomes
```

**Desired Flow with BR-AI-057**:
```
1. AI queries Context API with time_range=7d (last 7 days)
2. Response: success_rate=0.60, total_executions=50 (recent data)
3. ✅ AI detects significant drop from historical 89%
4. ✅ AI queries Playbook Catalog for alternative playbooks
5. ✅ AI finds pod-oom-vertical-scaling v1.0: success_rate=0.75 (7d)
6. ✅ AI selects alternative playbook (higher recent success rate)
7. ✅ AI logs decision: "Selected pod-oom-vertical-scaling v1.0 (75% recent success vs pod-oom-recovery 60%)"
8. ✅ Adaptable to changing infrastructure conditions
```

---

## 🔧 **Functional Requirements**

### **FR-AI-057-01: Query Context API for Success Rates**

**Requirement**: AI Service SHALL query Context API to retrieve playbook success rates for incident types.

**Implementation Example**:
```go
package ai

// PlaybookSelectionService selects playbooks based on success rates
type PlaybookSelectionService struct {
    contextAPIClient  ContextAPIClient
    playbookCatalog   PlaybookCatalogClient
    logger            *zap.Logger
}

// SelectPlaybook queries success rates and returns best playbook
func (s *PlaybookSelectionService) SelectPlaybook(ctx context.Context, incidentType string) (*PlaybookRecommendation, error) {
    // Step 1: Query available playbooks for incident type
    playbooks, err := s.playbookCatalog.ListPlaybooks(ctx, incidentType, "active")
    if err != nil {
        return nil, fmt.Errorf("failed to query playbook catalog: %w", err)
    }

    if len(playbooks) == 0 {
        return nil, fmt.Errorf("no playbooks found for incident_type=%s", incidentType)
    }

    // Step 2: Query success rates for each playbook
    var bestPlaybook *PlaybookRecommendation
    bestSuccessRate := 0.0

    for _, playbook := range playbooks {
        successRate, err := s.contextAPIClient.GetPlaybookSuccessRate(ctx, playbook.PlaybookID, playbook.Version, "7d")
        if err != nil {
            s.logger.Warn("failed to get success rate",
                zap.String("playbook_id", playbook.PlaybookID),
                zap.String("version", playbook.Version),
                zap.Error(err))
            continue // Skip playbook if success rate unavailable
        }

        // Step 3: Select playbook with highest success rate
        if successRate.SuccessRate > bestSuccessRate {
            bestSuccessRate = successRate.SuccessRate
            bestPlaybook = &PlaybookRecommendation{
                PlaybookID:       playbook.PlaybookID,
                Version:          playbook.Version,
                SuccessRate:      successRate.SuccessRate,
                TotalExecutions:  successRate.TotalExecutions,
                Confidence:       s.calculateConfidence(successRate),
                SelectionReason:  fmt.Sprintf("Highest success rate: %.1f%% (%d executions)", successRate.SuccessRate*100, successRate.TotalExecutions),
            }
        }
    }

    if bestPlaybook == nil {
        return nil, fmt.Errorf("no playbook with success rate data found")
    }

    return bestPlaybook, nil
}
```

**Acceptance Criteria**:
- ✅ Queries Context API for all candidate playbooks
- ✅ Handles API errors gracefully (skips playbook if success rate unavailable)
- ✅ Selects playbook with highest success rate
- ✅ Logs selection rationale (success rate, execution count)
- ✅ Returns error if no playbooks have success rate data

---

### **FR-AI-057-02: Confidence Score Calculation**

**Requirement**: AI Service SHALL calculate confidence score incorporating success rate and sample size.

**Confidence Calculation**:
```go
// calculateConfidence computes confidence score from success rate and sample size
func (s *PlaybookSelectionService) calculateConfidence(successRate *SuccessRateResponse) float64 {
    baseConfidence := successRate.SuccessRate // 0.0-1.0

    // Adjust for sample size (more executions = higher confidence)
    sampleSizeBonus := 0.0
    if successRate.TotalExecutions >= 100 {
        sampleSizeBonus = 0.05 // +5% for large sample
    } else if successRate.TotalExecutions >= 50 {
        sampleSizeBonus = 0.03 // +3% for medium sample
    } else if successRate.TotalExecutions >= 20 {
        sampleSizeBonus = 0.01 // +1% for small sample
    }
    // Total executions < 20: no bonus (low confidence)

    // Confidence = success_rate + sample_size_bonus (capped at 1.0)
    confidence := math.Min(1.0, baseConfidence + sampleSizeBonus)

    // If insufficient samples (< 5), cap confidence at 0.60
    if successRate.TotalExecutions < 5 {
        confidence = math.Min(confidence, 0.60)
    }

    return confidence
}
```

**Acceptance Criteria**:
- ✅ Confidence incorporates success rate (primary factor)
- ✅ Confidence adjusted for sample size (bonus for >50 executions)
- ✅ Confidence capped at 1.0
- ✅ Confidence capped at 0.60 for insufficient samples (<5 executions)
- ✅ Confidence score returned in playbook recommendation

---

### **FR-AI-057-03: Fallback Strategy for Insufficient Data**

**Requirement**: AI Service SHALL implement fallback strategy when success rate data is unavailable or insufficient.

**Fallback Priority**:
1. **No playbooks found**: Return error, recommend manual investigation
2. **Playbooks found, no success data**: Use default playbook (tagged as "default")
3. **Multiple playbooks, equal success rates**: Select latest version
4. **Insufficient samples (<5)**: Recommend with low confidence (0.50)

**Implementation**:
```go
// SelectPlaybookWithFallback includes fallback strategies
func (s *PlaybookSelectionService) SelectPlaybookWithFallback(ctx context.Context, incidentType string) (*PlaybookRecommendation, error) {
    recommendation, err := s.SelectPlaybook(ctx, incidentType)
    if err == nil {
        return recommendation, nil // Success
    }

    // Fallback 1: Check if any playbooks are tagged as "default"
    playbooks, _ := s.playbookCatalog.ListPlaybooks(ctx, incidentType, "active")
    for _, playbook := range playbooks {
        if contains(playbook.Tags, "default") {
            return &PlaybookRecommendation{
                PlaybookID:      playbook.PlaybookID,
                Version:         playbook.Version,
                SuccessRate:     0.0,  // Unknown
                Confidence:      0.50, // Low confidence
                SelectionReason: "Default playbook (no historical data)",
            }, nil
        }
    }

    // Fallback 2: No default playbook, recommend manual investigation
    return nil, fmt.Errorf("no data-driven playbook selection possible, recommend manual investigation")
}
```

**Acceptance Criteria**:
- ✅ Returns error if no playbooks exist for incident type
- ✅ Falls back to "default" tagged playbook if no success data
- ✅ Recommends manual investigation if no fallback possible
- ✅ Logs fallback strategy used
- ✅ Sets confidence=0.50 for fallback selections

---

## 📈 **Non-Functional Requirements**

### **NFR-AI-057-01: Performance**

- ✅ Playbook selection latency <500ms (including Context API queries)
- ✅ Parallel queries to Context API for multiple playbooks
- ✅ Cache success rates for 5 minutes (reduce Context API load)

### **NFR-AI-057-02: Reliability**

- ✅ Graceful degradation if Context API unavailable (use fallback)
- ✅ Timeout for Context API queries (2 seconds)
- ✅ Retry logic for transient failures (3 retries with exponential backoff)

### **NFR-AI-057-03: Observability**

- ✅ Log every playbook selection decision with rationale
- ✅ Prometheus metrics: `ai_playbook_selections_total{incident_type="...", playbook_id="...", selection_method="data_driven|fallback"}`
- ✅ Track success rate usage: `ai_success_rate_queries_total{status="success|error"}`

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Remediation Playbook Catalog (defines data-driven selection as primary AI capability)
- ✅ BR-INTEGRATION-008: Context API exposes incident-type success rate endpoint
- ✅ BR-PLAYBOOK-001: Playbook Catalog provides available playbooks

### **Downstream Impacts**
- ✅ BR-REMEDIATION-016: RemediationExecutor uses AI-selected playbook
- ✅ BR-EFFECTIVENESS-002: Effectiveness Monitor tracks AI selection accuracy

---

## 🚀 **Implementation Phases**

### **Phase 1: Context API Client** (Day 10 - 4 hours)
- Implement Context API client for success rate queries
- Add retry logic and timeout handling
- Add unit tests

### **Phase 2: Selection Logic** (Day 11 - 4 hours)
- Implement `SelectPlaybook()` with success rate comparison
- Implement confidence calculation
- Add fallback strategies

### **Phase 3: Integration** (Day 12 - 4 hours)
- Integrate with existing AI recommendation flow
- Add logging and metrics
- Integration tests with real Context API

### **Phase 4: Monitoring** (Day 13 - 2 hours)
- Add Prometheus metrics
- Add alerting for high fallback usage
- Dashboard for playbook selection analytics

**Total Estimated Effort**: 14 hours (1.75 days)

---

## 📊 **Success Metrics**

### **Data-Driven Selection Rate**
- **Target**: 90%+ of remediation decisions use data-driven playbook selection
- **Measure**: `ai_playbook_selections_total{selection_method="data_driven"}` / total selections

### **Remediation Success Rate Improvement**
- **Target**: 10%+ increase in overall remediation success rate
- **Measure**: Compare success rate before/after BR-AI-057 implementation

### **Context API Query Success**
- **Target**: 95%+ Context API queries succeed
- **Measure**: `ai_success_rate_queries_total{status="success"}` / total queries

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Hardcoded Playbook Priority**

**Approach**: Maintain hardcoded priority list of playbooks per incident type

**Rejected Because**:
- ❌ Cannot learn from historical data
- ❌ Requires manual updates for every infrastructure change
- ❌ No continuous improvement

---

### **Alternative 2: AI Learns Without Success Rates**

**Approach**: AI uses LLM reasoning without historical data

**Rejected Because**:
- ❌ LLM lacks context-specific remediation effectiveness knowledge
- ❌ Cannot validate effectiveness of selected playbook
- ❌ Lower success rate than data-driven approach

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P0 priority (core of ADR-033 Hybrid Model)
**Rationale**: Enables 90% of AI decisions to be data-driven (ADR-033 target)
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-INTEGRATION-008: Context API exposes incident-type success rate endpoint
- BR-PLAYBOOK-001: Playbook Catalog provides available playbooks
- BR-STORAGE-031-01: Data Storage incident-type success rate API
- BR-STORAGE-031-02: Data Storage playbook success rate API

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-034: BR Template Standard](../architecture/decisions/ADR-034-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

