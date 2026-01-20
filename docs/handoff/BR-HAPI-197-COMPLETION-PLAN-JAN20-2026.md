# BR-HAPI-197 Completion Plan

**Date**: January 20, 2026
**Priority**: P0 (Blocks BR-HAPI-212 and BR-SCOPE-001)
**Status**: 🔄 INCOMPLETE (6 weeks since original approval)
**Original BR**: [BR-HAPI-197](../requirements/BR-HAPI-197-needs-human-review-field.md) (Approved December 6, 2025)

---

## 🎯 **Executive Summary**

**BR-HAPI-197** (Human Review Required Flag) was approved 6 weeks ago but **never fully implemented**:
- ✅ **HAPI side is complete**: API field exists, logic works, OpenAPI spec updated
- ❌ **Consumer side is incomplete**: AIAnalysis reads but doesn't store, RO can't check it
- ⚠️ **Workaround exists**: System uses Phase/Reason instead, but architecturally wrong

**Why Complete This Now**:
1. **BR-HAPI-212** extends BR-HAPI-197 (adds scenario #7: missing `affectedResource`)
2. **Two-flag architecture** needed: `needs_human_review` (HAPI) vs `needs_approval` (Rego)
3. **BR-SCOPE-001** (resource scope management) depends on clean escalation paths

---

## 📊 **Current State Analysis**

### **What's Complete** ✅

| Component | Status | Evidence |
|-----------|--------|----------|
| HAPI API Field | ✅ Complete | `IncidentResponse.needs_human_review` exists |
| HAPI Logic | ✅ Complete | Sets flag for 6 scenarios (BR-HAPI-197.2) |
| HAPI OpenAPI Spec | ✅ Complete | 17 schemas updated |
| AIAnalysis Reads Flag | ✅ Complete | `response_processor.go:71` - `GetOptBoolValue(resp.NeedsHumanReview)` |
| AIAnalysis Handles It | ✅ Complete | `response_processor.go:95` - `handleWorkflowResolutionFailure()` |
| Audit Trail | ✅ Complete | `response_processor.go:333` - `RecordAnalysisFailed()` |
| RO Creates Notification | ✅ Complete | `aianalysis.go:223` - Creates NotificationRequest |

### **What's Missing** ❌

| Component | Status | Impact | Evidence |
|-----------|--------|--------|----------|
| **AIAnalysis CRD Fields** | ❌ Missing | P0 - Blocks all | `aianalysis_types.go:428` - Only has `ApprovalRequired`, no `NeedsHumanReview` |
| **Response Processor Storage** | ❌ Missing | P0 - Blocks RO | Reads flag but doesn't store to `analysis.Status.NeedsHumanReview` |
| **RO Direct Check** | ❌ Missing | P0 - Two-flag architecture | Infers from Phase/Reason, can't distinguish scenarios |
| **Dedicated Metrics** | ⚠️ Partial | P1 - Observability | Generic `FailuresTotal`, not `human_review_required_total` (BR-HAPI-197.9) |
| **DD-CONTRACT-002** | ❌ Missing | P0 - Architecture docs | Service contract doesn't show `needsHumanReview` field |

### **Current Workaround** ⚠️

**How it works today**:
```go
// AIAnalysis detects needs_human_review=true
// Sets: Phase=Failed, Reason="WorkflowResolutionFailed"
// RO sees Phase/Reason → Creates NotificationRequest

// ✅ Functional outcome: Human gets notified
// ❌ Architectural problem: Can't distinguish from other failures
// ❌ Can't implement two-flag architecture (review vs approval)
```

---

## 🚨 **Why This is a Problem**

### **1. Two-Flag Architecture Blocked**

**Required Flow** (can't implement without complete BR-HAPI-197):
```go
// RO Reconciliation Logic
if aiAnalysis.Status.NeedsHumanReview {          // ❌ Field doesn't exist
    return createNotificationRequest(...)         // Manual investigation needed
}

if aiAnalysis.Status.ApprovalRequired {          // ✅ Works today
    return createRemediationApprovalRequest(...)  // Has plan, needs approval
}

return createWorkflowExecution(...)               // Automatic remediation
```

### **2. BR-HAPI-212 Can't Be Properly Implemented**

**BR-HAPI-212** adds scenario #7 to BR-HAPI-197:
- ❌ Can't add scenario #7 if base scenarios (1-6) aren't properly stored
- ❌ Can't distinguish "missing `affectedResource`" from "workflow not found"

### **3. Observability Gap**

**BR-HAPI-197.9** requires dedicated metrics:
```prometheus
kubernaut_aianalysis_human_review_required_total{
  reason="workflow_not_found|no_workflows_matched|rca_incomplete|..."
}
```

**Currently**: Generic failure metrics don't show HAPI decision triggers.

---

## 📋 **Completion Requirements**

### **Phase 1: AIAnalysis CRD Schema Update**

**File**: `api/aianalysis/v1alpha1/aianalysis_types.go`

**Add to `AIAnalysisStatus`**:
```go
// ========================================
// HUMAN REVIEW SIGNALING (BR-HAPI-197)
// Set by HAPI when AI cannot produce reliable result
// ========================================
// True if human review required (HAPI decision: RCA incomplete/unreliable)
NeedsHumanReview bool `json:"needsHumanReview"`
// Reason why human review needed (when NeedsHumanReview=true)
// BR-HAPI-197: Maps to HAPI's human_review_reason enum
// BR-HAPI-212: Includes "rca_incomplete" for missing affectedResource
HumanReviewReason string `json:"humanReviewReason,omitempty"`
```

**Trigger**: `make generate` to update CRD YAML and DeepCopy methods

---

### **Phase 2: AIAnalysis Response Processor Update**

**File**: `pkg/aianalysis/handlers/response_processor.go`

**Update `handleWorkflowResolutionFailureFromIncident()` (line 300)**:
```go
func (p *ResponseProcessor) handleWorkflowResolutionFailureFromIncident(...) {
    // ... existing logic ...

    // BR-HAPI-197: Store HAPI's human review decision
    analysis.Status.NeedsHumanReview = needsHumanReview  // NEW
    analysis.Status.HumanReviewReason = humanReviewReason  // NEW

    // Keep existing Phase/Reason for backward compatibility
    analysis.Status.Phase = aianalysis.PhaseFailed
    analysis.Status.Reason = "WorkflowResolutionFailed"

    // ... rest of existing logic ...
}
```

**Similar update needed in**: `handleWorkflowResolutionFailureFromRecovery()`

---

### **Phase 3: RO Logic Update**

**File**: `pkg/remediationorchestrator/handler/aianalysis.go`

**Update `HandleAIAnalysisStatus()` (line 70)**:
```go
func (h *AIAnalysisHandler) HandleAIAnalysisStatus(...) {
    switch ai.Status.Phase {
    case "Completed":
        // NEW: Check needs_human_review BEFORE checking approval
        // (HAPI might set needs_human_review even if phase=Completed in edge cases)
        if ai.Status.NeedsHumanReview {
            return h.handleHumanReviewRequired(ctx, rr, ai)  // NEW method
        }

        // Existing approval check
        if ai.Status.ApprovalRequired {
            return h.handleApprovalRequired(ctx, rr, ai)
        }

        return h.handleCompleted(ctx, rr, ai)

    case "Failed":
        // NEW: Check needs_human_review BEFORE generic failure handling
        if ai.Status.NeedsHumanReview {
            return h.handleHumanReviewRequired(ctx, rr, ai)  // NEW method
        }

        return h.handleFailed(ctx, rr, ai)

    // ... rest of cases ...
    }
}

// NEW: Dedicated handler for needs_human_review
func (h *AIAnalysisHandler) handleHumanReviewRequired(...) {
    // Create NotificationRequest with human_review_reason context
    // Different from handleWorkflowResolutionFailed (keeps old logic for backward compat)
}
```

---

### **Phase 4: Metrics Update**

**File**: `pkg/aianalysis/metrics/metrics.go`

**Add dedicated counter**:
```go
type Metrics struct {
    // ... existing metrics ...

    // BR-HAPI-197.9: Track human review scenarios
    HumanReviewRequiredTotal *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
    // ... existing metrics ...

    humanReviewRequiredTotal := prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_aianalysis_human_review_required_total",
            Help: "Total number of AIAnalysis that required human review (BR-HAPI-197)",
        },
        []string{"reason"}, // workflow_not_found, no_workflows_matched, rca_incomplete, etc.
    )
    reg.MustRegister(humanReviewRequiredTotal)

    return &Metrics{
        // ... existing ...
        HumanReviewRequiredTotal: humanReviewRequiredTotal,
    }
}
```

**Update `response_processor.go`**:
```go
// In handleWorkflowResolutionFailureFromIncident()
p.metrics.HumanReviewRequiredTotal.WithLabelValues(humanReviewReason).Inc()
```

---

### **Phase 5: DD-CONTRACT-002 Update**

**File**: `docs/architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md`

**Update AIAnalysis → RO contract** (line 130):
```yaml
status:
  phase: string  # Pending, Investigating, Analyzing, Completed, Failed

  # NEW: Human review flag (BR-HAPI-197, BR-HAPI-212)
  needsHumanReview: bool         # HAPI decision: AI can't answer
  humanReviewReason: string      # Why review needed (when needsHumanReview=true)

  # Existing: Approval flag
  approvalRequired: bool         # Rego decision: Policy requires approval
  approvalReason: string         # Why approval needed

  # ... rest of contract ...
```

---

## 🔄 **Implementation Sequence**

### **Task 1: Documentation (This Document)** ✅
- [x] Create completion plan
- [ ] Get user approval

### **Task 2: CRD Schema** (TDD RED)
- [ ] Add fields to `aianalysis_types.go`
- [ ] Run `make generate`
- [ ] Write unit tests for schema validation
- [ ] Verify tests FAIL (RED phase)

### **Task 3: Response Processor** (TDD GREEN + REFACTOR)
- [ ] Update `handleWorkflowResolutionFailureFromIncident()`
- [ ] Update `handleWorkflowResolutionFailureFromRecovery()`
- [ ] Write unit tests for storage
- [ ] Verify tests PASS (GREEN phase)
- [ ] Refactor for clarity (REFACTOR phase)

### **Task 4: RO Logic** (TDD RED-GREEN-REFACTOR)
- [ ] Add `handleHumanReviewRequired()` method
- [ ] Update `HandleAIAnalysisStatus()` decision tree
- [ ] Write unit tests
- [ ] Write integration tests (RO + AIAnalysis interaction)

### **Task 5: Metrics** (TDD RED-GREEN-REFACTOR)
- [ ] Add `HumanReviewRequiredTotal` counter
- [ ] Update response processor to emit metric
- [ ] Write unit tests for metric emission

### **Task 6: Contract Documentation**
- [ ] Update DD-CONTRACT-002
- [ ] Update any affected ADRs/DDs

### **Task 7: Integration Testing**
- [ ] E2E test: HAPI sets needs_human_review → RO creates NotificationRequest
- [ ] E2E test: Verify two-flag distinction (review vs approval)

---

## 📊 **Impacted Services**

| Service | Impact | Files Affected | Test Type |
|---------|--------|----------------|-----------|
| **AIAnalysis** | Schema + Logic | 3 files | Unit + Integration |
| **RemediationOrchestrator** | Logic | 2 files | Unit + Integration |
| **Documentation** | Contract | 1 file | N/A |

### **Detailed File Impact**

#### **AIAnalysis Service**
1. `api/aianalysis/v1alpha1/aianalysis_types.go`
   - Add `NeedsHumanReview` field
   - Add `HumanReviewReason` field
   - **Change Type**: Schema (requires `make generate`)

2. `pkg/aianalysis/handlers/response_processor.go`
   - Update `handleWorkflowResolutionFailureFromIncident()` (line 300)
   - Update `handleWorkflowResolutionFailureFromRecovery()` (line 456)
   - **Change Type**: Business logic (store flag to CRD)

3. `pkg/aianalysis/metrics/metrics.go`
   - Add `HumanReviewRequiredTotal` counter
   - Register metric
   - **Change Type**: Observability

#### **RemediationOrchestrator Service**
1. `pkg/remediationorchestrator/handler/aianalysis.go`
   - Update `HandleAIAnalysisStatus()` (line 70)
   - Add `handleHumanReviewRequired()` method
   - **Change Type**: Routing logic

#### **Documentation**
1. `docs/architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md`
   - Add `needsHumanReview` to AIAnalysis status contract
   - **Change Type**: Architecture documentation

---

## 🎯 **Success Criteria**

### **Functional**
- [x] RO can check `aiAnalysis.Status.NeedsHumanReview` field
- [x] RO creates NotificationRequest when `needsHumanReview=true`
- [x] RO distinguishes `needs_human_review` from `needs_approval`
- [x] AIAnalysis emits `human_review_required_total` metric

### **Technical**
- [x] All tests pass (unit + integration + E2E)
- [x] No breaking changes to existing behavior
- [x] Backward compatible (Phase/Reason still work)

### **Documentation**
- [x] DD-CONTRACT-002 updated
- [x] BR-HAPI-197 marked as complete
- [x] Handoff document created

---

## 📅 **Timeline Estimate**

| Phase | Tasks | Estimated Time | Priority |
|-------|-------|----------------|----------|
| **Phase 1: CRD Schema** | Schema + generate | 30 min | P0 |
| **Phase 2: Response Processor** | Store to CRD | 1 hour | P0 |
| **Phase 3: RO Logic** | Check flag + route | 2 hours | P0 |
| **Phase 4: Metrics** | Add counter + emit | 30 min | P1 |
| **Phase 5: Documentation** | Update contracts | 30 min | P0 |
| **Phase 6: Testing** | Integration + E2E | 2 hours | P0 |
| **Total** | | **~6.5 hours** | |

---

## 🔗 **Dependencies**

### **Blocks**
- **BR-HAPI-212**: Adds scenario #7 (needs base implementation)
- **BR-SCOPE-001**: Resource scope management (needs clean escalation paths)
- **Two-flag architecture**: `needs_human_review` + `needs_approval` distinction

### **Blocked By**
- None (HAPI side is complete, ready for consumer side)

---

## 🚨 **Risk Assessment**

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking existing behavior | Low | High | Keep Phase/Reason for backward compatibility |
| Integration test failures | Medium | Medium | Comprehensive E2E test coverage |
| Metric naming conflicts | Low | Low | Follow BR-HAPI-197.9 spec exactly |
| CRD schema migration | Low | Medium | Use `+optional` tags, graceful handling |

---

## 📚 **Related Documents**

- **Original BR**: [BR-HAPI-197](../requirements/BR-HAPI-197-needs-human-review-field.md)
- **Extension BR**: [BR-HAPI-212](../requirements/BR-HAPI-212-rca-target-resource.md)
- **Contract DD**: [DD-CONTRACT-002](../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)
- **HAPI Validation DD**: [DD-HAPI-002 v1.2](../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md)

---

## ✅ **Completion Checklist**

### **Before Starting Implementation**
- [ ] User approval on this plan
- [ ] Confirm no other teams working on this
- [ ] Review BR-HAPI-197 thoroughly

### **During Implementation**
- [ ] Follow TDD RED-GREEN-REFACTOR for all changes
- [ ] Update tests before updating code
- [ ] Run `make generate` after CRD changes
- [ ] Run integration tests after each phase

### **Before Merging**
- [ ] All tests pass (unit + integration + E2E)
- [ ] No linter errors
- [ ] DD-CONTRACT-002 updated
- [ ] BR-HAPI-197 status updated to "Complete"
- [ ] Handoff document for BR-HAPI-212 next steps

---

## 📞 **Next Steps After Completion**

1. **Update BR-HAPI-197**: Change status from "⏳ Pending" to "✅ Complete"
2. **Resume BR-HAPI-212**: Add scenario #7 (missing `affectedResource`)
3. **Begin BR-SCOPE-001**: Resource scope management implementation

---

**Confidence Assessment**: 95%
- ✅ Clear understanding of what's missing
- ✅ Well-defined implementation steps
- ✅ Existing code provides good examples
- ⚠️ 5% risk: Integration test edge cases
