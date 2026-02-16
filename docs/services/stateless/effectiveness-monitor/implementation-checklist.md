# Effectiveness Monitor Service - Implementation Checklist

**Version**: 1.1.0 - API GATEWAY MIGRATION REFERENCE ADDED
**Last Updated**: November 2, 2025
**Service Type**: Stateless HTTP API Service (Assessment & Analysis)
**Port**: 8080 (REST API + Health), 9090 (Metrics)
**Status**: 📋 **NOT YET IMPLEMENTED** - Ready for Phase 3 implementation

**Format Optimization (DD-HOLMESGPT-009)**: 🆕
- **Ultra-Compact JSON Format**: 75% token reduction for HolmesGPT API calls
- **Cost Savings**: $1,320/year on LLM API calls (~18K post-execution analyses)
- **Latency Improvement**: 150ms per post-execution analysis
- **Decision Document**: `docs/architecture/decisions/DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md`

---

## 🔄 **FUTURE ARCHITECTURAL CHANGE: API GATEWAY PATTERN**

**⚠️ CRITICAL - MUST IMPLEMENT THIS PATTERN**

**Decision**: [DD-ARCH-001 Alternative 2 (API Gateway Pattern)](../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)

**What**: Effectiveness Monitor will query audit trail data via **Data Storage Service REST API** (not direct PostgreSQL)

**When**: Phase 3 of API Gateway migration - After Data Storage Service Phase 1 complete

**Implementation Plan**: [implementation/API-GATEWAY-MIGRATION.md](./implementation/API-GATEWAY-MIGRATION.md)

**Timeline**: 2-3 days (HTTP client integration + integration test updates)

**Impact**:
- **Reads**: Audit trail data → **Data Storage Service REST API** (NEW)
- **Writes**: Effectiveness assessments → Direct PostgreSQL (unchanged)
- **Code Pattern**: Similar to Context API migration (HTTP client for reads)

**⚠️ IMPORTANT**: Do NOT implement direct PostgreSQL queries for audit data. Use Data Storage Service REST API.

**Status**: 📋 **APPROVED - Must follow this pattern when implementing service**

**Related**:
- [Data Storage Service Phase 1 Plan](../data-storage/implementation/API-GATEWAY-MIGRATION.md) - Dependency
- [Context API Migration Plan](../context-api/implementation/API-GATEWAY-MIGRATION.md) - Similar pattern

---

## 📋 **VERSION HISTORY**

### **v1.1.0** (2025-11-02) - API GATEWAY MIGRATION REFERENCE ADDED

**Purpose**: Document approved architectural pattern (DD-ARCH-001) that MUST be followed during implementation

**Changes**:
- ✅ **Future Architectural Change section added** (~25 lines)
  - Documents required pattern: Query audit data via Data Storage Service REST API
  - Explicitly warns against direct PostgreSQL queries for audit data
  - Links to [DD-ARCH-001 Final Decision](../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)
  - References detailed implementation plan: [implementation/API-GATEWAY-MIGRATION.md](./implementation/API-GATEWAY-MIGRATION.md)
  - Timeline: 2-3 days (Phase 3 - after Data Storage Service Phase 1)
- ✅ **Implementation plan created**: [implementation/API-GATEWAY-MIGRATION.md](./implementation/API-GATEWAY-MIGRATION.md)
  - Day-by-day breakdown (HTTP client, integration tests)
  - Specification updates defined (overview.md, integration-points.md)
  - Read/Write split pattern (reads via API, writes direct)

**Rationale**:
- Service not yet implemented, but approved architectural pattern exists
- Must document in checklist to prevent implementing wrong pattern
- Effectiveness Monitor is Phase 3 (depends on Data Storage Service Phase 1)

**Impact**:
- Implementation Guidance: Clear pattern to follow when implementing service
- Risk Mitigation: Prevents implementing direct PostgreSQL queries (wrong pattern)
- Coordination: References Data Storage Service as Phase 1 dependency

**Time Investment**: 5 minutes (documentation only)

**Related**:
- [DD-ARCH-001 Final Decision](../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)
- [implementation/API-GATEWAY-MIGRATION.md](./implementation/API-GATEWAY-MIGRATION.md) - Phase 3 plan

---

### **v1.0.1** (2025-10-16) - ULTRA-COMPACT JSON FORMAT

**Purpose**: Document format optimization for HolmesGPT API calls

**Changes**: (see above in header)

---

## 📚 Reference Documentation

**CRITICAL**: Read these documents before starting implementation:

- **Testing Strategy**: [testing-strategy.md](./testing-strategy.md) - Comprehensive testing approach (70%+ unit, >50% integration)
- **Security Configuration**: [security-configuration.md](./security-configuration.md) - Authentication & authorization
- **Integration Points**: [integration-points.md](./integration-points.md) - Data Storage, Infrastructure Monitoring dependencies
- **Core Methodology**: [00-core-development-methodology.mdc](../../../.cursor/rules/00-core-development-methodology.mdc) - APDC-Enhanced TDD
- **Business Requirements**: BR-INS-001 through BR-INS-010 (V1 Core Effectiveness Assessment)
  - **V1 Scope**: BR-INS-001 to BR-INS-010 (Core effectiveness assessment with graceful degradation)
  - **Reserved for V2**: BR-INS-011 to BR-INS-100 (Multi-cloud support: AWS, Azure, Datadog, GCP; Advanced ML-based prediction)
- **V1 Strategy**: [README.md](./README.md#v1-graceful-degradation-strategy) - Graceful degradation implementation

---

## 📋 APDC-TDD Implementation Workflow

Following **mandatory** APDC-Enhanced TDD methodology (Analysis → Plan → Do-RED → Do-GREEN → Do-REFACTOR → Check).

---

## 🔍 ANALYSIS PHASE (1-2 days)

### **Context Understanding**

- [ ] **Business Context**: Map BR-INS-001 to BR-INS-010 (Phase 1: BR-INS-001, BR-INS-002, BR-INS-005; Phase 2: BR-INS-003, BR-INS-004, BR-INS-006 to BR-INS-010)
- [ ] **Technical Context**: Search existing implementations
  ```bash
  codebase_search "effectiveness implementations in pkg/"
  grep -r "EffectivenessMonitor\|EffectivenessService" pkg/ --include="*.go"
  # Note: Business logic exists in pkg/ai/insights/ (6,295 lines, 98% complete)
  ```
- [ ] **Integration Context**: Verify dependencies
  ```bash
  grep -r "EffectivenessMonitor" cmd/ --include="*.go"
  ```
- [ ] **V1 Strategy**: Understand DD-017 v2.0: Level 1 (V1.0) Day-1 value; Level 2 (V1.1) requires 8+ weeks of data

### **Analysis Deliverables**

- [ ] Business requirement mapping (Phase 1: BR-INS-001, BR-INS-002, BR-INS-005; Phase 2: BR-INS-003, BR-INS-004, BR-INS-006 to BR-INS-010)
- [ ] Dependency analysis (Data Storage, Infrastructure Monitoring)
- [ ] Integration points identified (Context API, HolmesGPT API)
- [ ] Level 1 vs Level 2 scope understood (DD-017 v2.0: Level 1 Day-1; Level 2 requires 8+ weeks)

---

## 📝 PLAN PHASE (1-2 days)

### **Implementation Strategy**

- [ ] **TDD Strategy**: Define effectiveness assessment interfaces
- [ ] **Integration Plan**: Data Storage queries, Infrastructure Monitoring metrics correlation
- [ ] **Success Definition**:
  - Assessment latency < 5s p95
  - Confidence ≥ 80% with 8+ weeks data
  - Graceful degradation in Week 5-13
- [ ] **Timeline**: RED (2 days) → GREEN (2 days) → REFACTOR (3 days)

---

## 🔴 DO-RED PHASE (2 days)

### **Write Failing Tests First**

#### **Day 1: Effectiveness Calculation Tests (BR-INS-001) — Level 1 (V1.0)**

- [ ] Create `test/unit/effectiveness/calculator_test.go`
- [ ] Test: Traditional effectiveness score calculation
- [ ] Test: Confidence level calculation (based on data availability)
- [ ] Test: Insufficient data response (< 8 weeks)
- [ ] **Validation**: All tests FAIL

#### **Day 2: Side Effect & Trend Tests (BR-INS-003, BR-INS-005) — Side effects: Level 1 (V1.0); Trends: Level 2 (V1.1)**

- [ ] Create `test/unit/effectiveness/side_effects_test.go`
- [ ] Test: Side effect detection (CPU, memory, network)
- [ ] Test: Severity classification (high/low/none)
- [ ] Create `test/unit/effectiveness/trend_test.go`
- [ ] Test: Trend analysis (improving/declining/stable)
- [ ] **Validation**: All tests FAIL

---

## 🟢 DO-GREEN PHASE (2 days)

### **Minimal Implementation**

#### **Day 1: Core Interfaces & Main App**

- [ ] Create `pkg/effectiveness/service.go`
- [ ] Create `cmd/effectiveness-monitor/main.go`
- [ ] **MANDATORY**: Integrate in main app
  ```go
  func main() {
      service := effectiveness.NewEffectivenessMonitorService(deps...)
      http.ListenAndServe(":8080", service.Handler())
  }
  ```
- [ ] Implement graceful degradation check (data weeks < 8)

#### **Day 2: Assessment Implementation**

- [ ] Implement Data Storage client for action history
- [ ] Implement Infrastructure Monitoring client for metrics
- [ ] Implement basic effectiveness score calculation
- [ ] **Validation**: Tests PASS

---

## 🔧 DO-REFACTOR PHASE (3 days)

### **Enhance Existing Code**

#### **Day 1: Multi-Dimensional Assessment (BR-INS-002) — Level 1 (V1.0)**

- [ ] Add environmental impact correlation (memory, CPU, network)
- [ ] Enhance confidence calculation with data quality metrics
- [ ] Add pattern detection (business hours correlation)

#### **Day 2: Advanced Analysis (BR-INS-006, BR-INS-008) — Level 2 (V1.1)**

- [ ] Implement side effect detection algorithms
- [ ] Add trend analysis (improving/declining/stable)
- [ ] Implement pattern insights generation

#### **Day 2b: Hybrid AI Integration (DD-EFFECTIVENESS-001 + DD-HOLMESGPT-009)**

- [ ] Implement HolmesGPT client (`pkg/monitor/holmesgpt_client.go`)
  ```go
  // Client for calling HolmesGPT API post-execution analysis
  type HolmesGPTClient interface {
      AnalyzePostExecution(ctx context.Context, req PostExecRequest) (*PostExecResponse, error)
  }
  ```
- [ ] **REQUIRED**: Implement InvestigationContext for ultra-compact JSON format (DD-HOLMESGPT-009) 🆕
  ```go
  // pkg/ai/analysis/compact_encoder.go
  func (e *InvestigationContext) BuildPostExecCompactContext(req *PostExecRequest) (string, error)
  ```
- [ ] Use InvestigationContext in HolmesGPTClient implementation
  ```go
  compactContext, err := encoder.BuildPostExecCompactContext(request)
  // ~180 tokens vs ~730 tokens (75% reduction)
  ```
- [ ] Implement `shouldCallAI()` decision logic
  ```go
  func shouldCallAI(workflow *WorkflowExecution, basicScore float64, anomalies []string) bool
  ```
- [ ] Add AI analysis Prometheus metrics (including token count)
  ```go
  effectiveness_ai_trigger_total{trigger_type="p0_failure|new_action_type|anomaly_detected|oscillation|routine_skipped"}
  effectiveness_ai_calls_total{status="success|failure|timeout"}
  effectiveness_ai_cost_total_dollars
  effectiveness_ai_call_duration_seconds
  effectiveness_ai_token_count{phase="input|output"}  // NEW
  ```

#### **Day 3: Observability**

- [ ] Add Prometheus metrics (assessment_duration, effectiveness_score, confidence distribution)
- [ ] Add structured logging (Zap) with assessment lifecycle
- [ ] Add health checks with data availability status
- [ ] Implement graceful degradation readiness probe

---

## ✅ CHECK PHASE (1 day)

### **Validation**

- [ ] Business requirements met (BR-INS-001 to BR-INS-010)
- [ ] Performance targets: < 5s p95 assessment latency
- [ ] Level 1 assessment working (Day-1 value); Level 2 when 8+ weeks data available
- [ ] Test coverage: 70%+ unit tests, >50% integration tests
- [ ] Lint clean: `golangci-lint run`
- [ ] Confidence: ≥ 80% with 8+ weeks of data

---

## 📦 Package Structure

```
cmd/effectiveness-monitor/
  └── main.go

pkg/effectiveness/
  ├── service.go           # EffectivenessMonitorService interface
  ├── calculator.go        # Effectiveness score calculation
  ├── side_effects.go      # Side effect detection
  ├── trend.go             # Trend analysis
  ├── pattern.go           # Pattern insights
  ├── clients.go           # Data Storage & Infrastructure Monitoring clients
  └── types.go             # Assessment types

test/unit/effectiveness/
  ├── calculator_test.go
  ├── side_effects_test.go
  ├── trend_test.go
  └── pattern_test.go

test/integration/effectiveness/
  ├── data_storage_test.go
  ├── infrastructure_monitoring_test.go
  └── cross_service_test.go

test/e2e/effectiveness/
  ├── assessment_workflow_test.go
  └── graceful_degradation_test.go
```

---

## 🎯 Timeline Summary

| Phase | Duration | Outcome |
|-------|----------|---------|
| **ANALYSIS** | 1-2 days | Context understanding + V1 strategy |
| **PLAN** | 1-2 days | Implementation strategy + graceful degradation |
| **DO-RED** | 2 days | Failing tests (effectiveness, side effects, trends) |
| **DO-GREEN** | 2 days | Minimal implementation + data availability check |
| **DO-REFACTOR** | 3 days | Multi-dimensional assessment + observability |
| **CHECK** | 1 day | Comprehensive validation + graceful degradation testing |
| **TOTAL** | **9-11 days** | Production-ready service with V1 graceful degradation |

---

## 🔄 V1 Graceful Degradation Implementation

### **Data Availability Check**

```go
func (s *EffectivenessMonitorService) checkDataAvailability(ctx context.Context) (int, bool) {
    weeks, err := s.dataStorageClient.GetDataAvailabilityWeeks(ctx)
    if err != nil {
        return 0, false
    }

    sufficient := weeks >= 8
    return weeks, sufficient
}
```

### **Response Strategy by Week**

| Week | Data Weeks | Response Type | Confidence |
|------|-----------|---------------|------------|
| **Week 5** | 0 weeks | `insufficient_data` | 20-30% |
| **Week 8** | 3 weeks | `basic_assessment` | 40-50% |
| **Week 10** | 5 weeks | `trend_detection` | 60-70% |
| **Week 13+** | 8+ weeks | `full_assessment` | 80-95% |

### **Implementation Checklist**

- [ ] Implement data availability check in main handler
- [ ] Return `insufficient_data` response when weeks < 8
- [ ] Include `estimated_availability` date in response
- [ ] Expose `assessment_data_availability_weeks` metric
- [ ] Update readiness probe to reflect data availability

---

## 🚨 Critical Integration Points

### **Required Services**

| Service | Port | Required For | Failure Mode |
|---------|------|-------------|--------------|
| **Data Storage** | 8085 | Action history, effectiveness storage | Return error (critical) |
| **Infrastructure Monitoring** | 8094 | Metrics correlation, side effects | Graceful degradation (log warning) |

### **Integration Validation**

- [ ] Data Storage connection tested (action history retrieval)
- [ ] Infrastructure Monitoring connection tested (metrics query)
- [ ] Graceful degradation when Infrastructure Monitoring unavailable
- [ ] Error responses clear and actionable

---

## 📊 Success Metrics

### **Week 5 (Deployment)**

- [ ] Service deployed and healthy
- [ ] Returns `insufficient_data` with clear messaging
- [ ] All API endpoints functional
- [ ] Monitoring operational

### **Week 13 (Full Capability)**

- [ ] 8+ weeks of historical data
- [ ] Confidence ≥ 80%
- [ ] Full multi-dimensional assessment
- [ ] Pattern insights generated
- [ ] Side effect detection operational

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

