# Implementation Plan: RecoveryStatus Field Population

**Feature**: Populate `status.recoveryStatus` from HolmesGPT-API recovery analysis
**Business Requirement**: BR-AI-080-083 (Recovery Flow) - observability enhancement
**Priority**: 🔴 **BLOCKING V1.0**
**Estimated Effort**: 2-3 hours
**Date**: December 11, 2025
**Methodology**: APDC + TDD (RED-GREEN-REFACTOR)

---

## 🎯 Business Context

**Problem**: AIAnalysis recovery scenarios don't populate `status.recoveryStatus`, but crd-schema.md example shows it populated. Operators lose visibility into HAPI's failure assessment.

**Solution**: Parse `recovery_analysis` from HAPI `/recovery/analyze` response and map to `RecoveryStatus` struct.

**Business Value**:
- ✅ Operators see HAPI's failure assessment via `kubectl describe`
- ✅ Status shows if system state changed after failed workflow
- ✅ Better recovery troubleshooting without checking audit trail
- ✅ Compliance with crd-schema.md authoritative spec

---

## 📋 APDC Phase Breakdown

### **ANALYSIS Phase** (15 minutes)

**Objectives**:
1. Understand HAPI response structure
2. Identify existing code patterns
3. Assess integration points
4. Define success criteria

**Discovery Actions**:
```bash
# Search existing HAPI response handling
grep -r "InvestigateRecovery\|recovery_analysis" pkg/aianalysis/

# Check existing status field population patterns
grep -r "Status\." pkg/aianalysis/handlers/investigating.go

# Review HAPI mock responses
grep -r "recovery_analysis" holmesgpt-api/src/mock_responses.py
```

**Questions to Answer**:
- ✅ Where is HAPI response parsed? → `pkg/aianalysis/handlers/investigating.go`
- ✅ What's the response struct? → `holmesgpt-api` ogen-generated client
- ✅ When should RecoveryStatus be set? → Only when `spec.isRecoveryAttempt = true`
- ✅ What if HAPI doesn't return recovery_analysis? → Leave nil (optional field)

---

### **PLAN Phase** (20 minutes)

**Implementation Strategy**:

**Phase Mapping**:
| Phase | Action | Test Type |
|-------|--------|-----------|
| **RED** | Write failing unit test | Unit test (mock HAPI response) |
| **GREEN** | Parse response, populate status | Minimal implementation |
| **REFACTOR** | Error handling, edge cases | Enhanced implementation |

**Files to Modify**:
1. `pkg/aianalysis/handlers/investigating.go` (~30 lines)
2. `test/unit/aianalysis/investigating_handler_test.go` (~60 lines new test)
3. `test/integration/aianalysis/recovery_integration_test.go` (~10 lines assertion)

**Integration Points**:
- HAPI client: `pkg/aianalysis/client/holmesgpt.go`
- Status update: `investigating.go` after HAPI call
- Test mocks: Use existing test patterns

**Success Criteria**:
- ✅ RecoveryStatus populated when `isRecoveryAttempt = true`
- ✅ RecoveryStatus is `nil` for initial incidents
- ✅ All fields mapped correctly from HAPI response
- ✅ Unit tests pass
- ✅ Integration tests pass
- ✅ E2E tests show field in `kubectl describe`

---

## 🧪 TDD Implementation Plan

### **DO-RED Phase** (30 minutes)

**Objective**: Write failing tests that define the contract

#### **Test 1: Unit Test - Recovery Scenario**

**File**: `test/unit/aianalysis/investigating_handler_test.go`

**Test Name**: `"should populate RecoveryStatus when isRecoveryAttempt is true"`

```go
var _ = Describe("InvestigatingHandler - RecoveryStatus", func() {
    var (
        handler    *handlers.InvestigatingHandler
        analysis   *aianalysisv1.AIAnalysis
        mockClient *mock.MockHolmesGPTClient
    )

    BeforeEach(func() {
        // Setup: Create recovery analysis with isRecoveryAttempt = true
        analysis = &aianalysisv1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "recovery-test",
                Namespace: "default",
            },
            Spec: aianalysisv1.AIAnalysisSpec{
                IsRecoveryAttempt:     true,
                RecoveryAttemptNumber: 2,
                // ... other required fields
            },
        }
        
        // Mock HAPI client
        mockClient = &mock.MockHolmesGPTClient{
            InvestigateRecoveryFunc: func(ctx context.Context, req *holmesgpt.RecoveryRequest) (*holmesgpt.RecoveryResponse, error) {
                return &holmesgpt.RecoveryResponse{
                    RecoveryAnalysis: &holmesgpt.RecoveryAnalysis{
                        PreviousAttemptAssessment: &holmesgpt.PreviousAttemptAssessment{
                            FailureUnderstood:     true,
                            FailureReasonAnalysis: "RBAC permissions insufficient",
                            StateChanged:          false,
                            CurrentSignalType:     "OOMKilled",
                        },
                    },
                    SelectedWorkflow: &holmesgpt.SelectedWorkflow{
                        WorkflowID:  "oomkill-restart-pods",
                        Confidence:  0.78,
                        // ... other fields
                    },
                }, nil
            },
        }
        
        handler = handlers.NewInvestigatingHandler(mockClient, nil, nil)
    })

    It("should populate RecoveryStatus when isRecoveryAttempt is true", func() {
        // Act
        result, err := handler.Handle(ctx, analysis)
        
        // Assert
        Expect(err).ToNot(HaveOccurred())
        Expect(result.Requeue).To(BeFalse())
        
        // RecoveryStatus should be populated
        Expect(analysis.Status.RecoveryStatus).ToNot(BeNil())
        Expect(analysis.Status.RecoveryStatus.PreviousAttemptAssessment).ToNot(BeNil())
        
        // Verify field mapping
        assessment := analysis.Status.RecoveryStatus.PreviousAttemptAssessment
        Expect(assessment.FailureUnderstood).To(BeTrue())
        Expect(assessment.FailureReasonAnalysis).To(Equal("RBAC permissions insufficient"))
        
        Expect(analysis.Status.RecoveryStatus.StateChanged).To(BeFalse())
        Expect(analysis.Status.RecoveryStatus.CurrentSignalType).To(Equal("OOMKilled"))
    })

    It("should NOT populate RecoveryStatus for initial incidents", func() {
        // Arrange: Initial incident (not recovery)
        analysis.Spec.IsRecoveryAttempt = false
        
        mockClient.InvestigateFunc = func(ctx context.Context, req *holmesgpt.IncidentRequest) (*holmesgpt.IncidentResponse, error) {
            return &holmesgpt.IncidentResponse{
                SelectedWorkflow: &holmesgpt.SelectedWorkflow{
                    WorkflowID: "oomkill-increase-memory",
                },
            }, nil
        }
        
        // Act
        result, err := handler.Handle(ctx, analysis)
        
        // Assert
        Expect(err).ToNot(HaveOccurred())
        Expect(analysis.Status.RecoveryStatus).To(BeNil()) // Should be nil for initial incidents
    })

    Context("when HAPI doesn't return recovery_analysis", func() {
        It("should leave RecoveryStatus as nil", func() {
            // Arrange: HAPI returns response without recovery_analysis
            mockClient.InvestigateRecoveryFunc = func(ctx context.Context, req *holmesgpt.RecoveryRequest) (*holmesgpt.RecoveryResponse, error) {
                return &holmesgpt.RecoveryResponse{
                    RecoveryAnalysis: nil, // HAPI didn't include recovery analysis
                    SelectedWorkflow: &holmesgpt.SelectedWorkflow{
                        WorkflowID: "fallback-workflow",
                    },
                }, nil
            }
            
            // Act
            result, err := handler.Handle(ctx, analysis)
            
            // Assert
            Expect(err).ToNot(HaveOccurred())
            Expect(analysis.Status.RecoveryStatus).To(BeNil()) // Gracefully handle missing data
        })
    })
})
```

**Expected Result**: ❌ Tests FAIL (RecoveryStatus not implemented yet)

---

#### **Test 2: Integration Test - Recovery Flow**

**File**: `test/integration/aianalysis/recovery_integration_test.go`

**Add Assertion** (~10 lines):

```go
// In existing recovery integration test
It("should populate RecoveryStatus in recovery scenarios", func() {
    // ... existing test setup ...
    
    // Create recovery AIAnalysis
    recoveryAnalysis := &aianalysisv1.AIAnalysis{
        Spec: aianalysisv1.AIAnalysisSpec{
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: 2,
            // ... other fields
        },
    }
    
    Expect(k8sClient.Create(ctx, recoveryAnalysis)).To(Succeed())
    
    // Wait for reconciliation
    Eventually(func() string {
        err := k8sClient.Get(ctx, client.ObjectKeyFromObject(recoveryAnalysis), recoveryAnalysis)
        if err != nil {
            return ""
        }
        return recoveryAnalysis.Status.Phase
    }, timeout, interval).Should(Equal("Completed"))
    
    // NEW ASSERTION: Verify RecoveryStatus populated
    Expect(recoveryAnalysis.Status.RecoveryStatus).ToNot(BeNil(), "RecoveryStatus should be populated for recovery attempts")
    Expect(recoveryAnalysis.Status.RecoveryStatus.PreviousAttemptAssessment).ToNot(BeNil())
    Expect(recoveryAnalysis.Status.RecoveryStatus.PreviousAttemptAssessment.FailureUnderstood).To(BeTrue())
})
```

**Expected Result**: ❌ Integration test FAILS (RecoveryStatus not implemented)

---

### **DO-GREEN Phase** (45 minutes)

**Objective**: Minimal implementation to make tests pass

#### **Implementation**

**File**: `pkg/aianalysis/handlers/investigating.go`

**Location**: After HAPI call, before status update

```go
// Around line 400-450 in investigating.go
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    // ... existing code ...
    
    // Call HAPI
    var resp interface{}
    var err error
    
    if analysis.Spec.IsRecoveryAttempt {
        // Recovery scenario
        recoveryReq := h.buildRecoveryRequest(ctx, analysis)
        recoveryResp, err := h.hapiClient.InvestigateRecovery(ctx, recoveryReq)
        if err != nil {
            return ctrl.Result{}, fmt.Errorf("HAPI recovery investigation failed: %w", err)
        }
        resp = recoveryResp
        
        // NEW: Populate RecoveryStatus from HAPI response
        if recoveryResp.RecoveryAnalysis != nil {
            h.populateRecoveryStatus(analysis, recoveryResp.RecoveryAnalysis)
        }
        
    } else {
        // Initial incident scenario
        incidentReq := h.buildIncidentRequest(ctx, analysis)
        incidentResp, err := h.hapiClient.Investigate(ctx, incidentReq)
        if err != nil {
            return ctrl.Result{}, fmt.Errorf("HAPI investigation failed: %w", err)
        }
        resp = incidentResp
        // RecoveryStatus remains nil for initial incidents
    }
    
    // ... existing status population code ...
}

// NEW HELPER FUNCTION
func (h *InvestigatingHandler) populateRecoveryStatus(
    analysis *aianalysisv1.AIAnalysis,
    recoveryAnalysis *holmesgpt.RecoveryAnalysis,
) {
    // Defensive: Only populate if recovery_analysis exists
    if recoveryAnalysis == nil {
        return
    }
    
    // Map HAPI RecoveryAnalysis to AIAnalysis RecoveryStatus
    analysis.Status.RecoveryStatus = &aianalysisv1.RecoveryStatus{
        StateChanged:      recoveryAnalysis.PreviousAttemptAssessment.StateChanged,
        CurrentSignalType: recoveryAnalysis.PreviousAttemptAssessment.CurrentSignalType,
    }
    
    // Map PreviousAttemptAssessment if present
    if recoveryAnalysis.PreviousAttemptAssessment != nil {
        analysis.Status.RecoveryStatus.PreviousAttemptAssessment = &aianalysisv1.PreviousAttemptAssessment{
            FailureUnderstood:     recoveryAnalysis.PreviousAttemptAssessment.FailureUnderstood,
            FailureReasonAnalysis: recoveryAnalysis.PreviousAttemptAssessment.FailureReasonAnalysis,
        }
    }
}
```

**Expected Result**: ✅ Unit tests PASS, ✅ Integration tests PASS

---

### **DO-REFACTOR Phase** (30 minutes)

**Objective**: Enhance with error handling, logging, edge cases

#### **Enhancements**

1. **Add Logging**:
```go
func (h *InvestigatingHandler) populateRecoveryStatus(
    analysis *aianalysisv1.AIAnalysis,
    recoveryAnalysis *holmesgpt.RecoveryAnalysis,
) {
    log := h.log.WithValues("analysis", analysis.Name, "namespace", analysis.Namespace)
    
    if recoveryAnalysis == nil {
        log.V(1).Info("HAPI did not return recovery_analysis, skipping RecoveryStatus population")
        return
    }
    
    log.Info("Populating RecoveryStatus from HAPI response",
        "stateChanged", recoveryAnalysis.PreviousAttemptAssessment.StateChanged,
        "currentSignalType", recoveryAnalysis.PreviousAttemptAssessment.CurrentSignalType,
    )
    
    // ... existing mapping ...
}
```

2. **Add Metrics** (optional):
```go
// In populateRecoveryStatus
if analysis.Status.RecoveryStatus != nil {
    h.metrics.RecoveryStatusPopulated.Inc()
    if analysis.Status.RecoveryStatus.PreviousAttemptAssessment.FailureUnderstood {
        h.metrics.RecoveryFailureUnderstood.Inc()
    }
}
```

3. **Add Edge Case Tests**:
```go
Context("edge cases", func() {
    It("should handle nil PreviousAttemptAssessment gracefully", func() {
        mockClient.InvestigateRecoveryFunc = func(ctx context.Context, req *holmesgpt.RecoveryRequest) (*holmesgpt.RecoveryResponse, error) {
            return &holmesgpt.RecoveryResponse{
                RecoveryAnalysis: &holmesgpt.RecoveryAnalysis{
                    PreviousAttemptAssessment: nil, // Nil assessment
                },
            }, nil
        }
        
        result, err := handler.Handle(ctx, analysis)
        
        Expect(err).ToNot(HaveOccurred())
        Expect(analysis.Status.RecoveryStatus).ToNot(BeNil())
        Expect(analysis.Status.RecoveryStatus.PreviousAttemptAssessment).To(BeNil())
    })
    
    It("should handle empty CurrentSignalType", func() {
        mockClient.InvestigateRecoveryFunc = func(ctx context.Context, req *holmesgpt.RecoveryRequest) (*holmesgpt.RecoveryResponse, error) {
            return &holmesgpt.RecoveryResponse{
                RecoveryAnalysis: &holmesgpt.RecoveryAnalysis{
                    PreviousAttemptAssessment: &holmesgpt.PreviousAttemptAssessment{
                        FailureUnderstood:     true,
                        FailureReasonAnalysis: "Test",
                        StateChanged:          false,
                        CurrentSignalType:     "", // Empty signal type
                    },
                },
            }, nil
        }
        
        result, err := handler.Handle(ctx, analysis)
        
        Expect(err).ToNot(HaveOccurred())
        Expect(analysis.Status.RecoveryStatus.CurrentSignalType).To(Equal(""))
    })
})
```

**Expected Result**: ✅ All tests PASS with enhanced coverage

---

### **CHECK Phase** (15 minutes)

**Validation Checklist**:

#### **Business Alignment**
- [ ] RecoveryStatus matches crd-schema.md example format
- [ ] Operators can see failure assessment via `kubectl describe`
- [ ] Recovery scenarios show HAPI's analysis

#### **Technical Validation**
- [ ] Unit tests pass (3+ test cases)
- [ ] Integration tests pass (recovery assertion added)
- [ ] E2E tests show RecoveryStatus populated (manual verification)
- [ ] No lint errors: `golangci-lint run pkg/aianalysis/handlers/investigating.go`
- [ ] No compilation errors: `go build ./pkg/aianalysis/...`

#### **Code Quality**
- [ ] Defensive nil checks
- [ ] Logging added for observability
- [ ] Error handling follows project patterns
- [ ] Field mapping is complete (all 4 RecoveryStatus fields)

#### **Integration Points**
- [ ] HAPI client types match (ogen-generated)
- [ ] Status update follows existing patterns
- [ ] No breaking changes to existing recovery flow

#### **Documentation**
- [ ] AIANALYSIS_TRIAGE.md updated (mark RecoveryStatus as COMPLETE)
- [ ] V1.0_FINAL_CHECKLIST.md updated (Task 4 complete)
- [ ] Inline code comments explain mapping logic

---

## 📊 Success Metrics

| Metric | Target | Verification |
|--------|--------|--------------|
| **Unit Test Coverage** | 3+ test cases | `go test -v ./test/unit/aianalysis/...` |
| **Integration Tests** | 1+ assertion | `make test-integration-aianalysis` |
| **Field Completeness** | 4/4 fields mapped | Review `populateRecoveryStatus()` |
| **Recovery Scenarios** | RecoveryStatus != nil | Integration test assertion |
| **Initial Incidents** | RecoveryStatus == nil | Unit test assertion |
| **Build Success** | No errors | `make build-aianalysis` |

---

## 🚀 Execution Timeline

| Phase | Duration | Deliverable |
|-------|----------|-------------|
| **ANALYSIS** | 15 min | Discovery findings documented |
| **PLAN** | 20 min | This implementation plan |
| **DO-RED** | 30 min | Failing unit + integration tests |
| **DO-GREEN** | 45 min | `populateRecoveryStatus()` function |
| **DO-REFACTOR** | 30 min | Logging, edge cases, metrics |
| **CHECK** | 15 min | Validation checklist complete |
| **Documentation** | 20 min | Update TRIAGE + CHECKLIST |
| **TOTAL** | **2h 35m** | RecoveryStatus V1.0 complete |

---

## 🎯 APDC Compliance

### **Analysis Prevention**
✅ Searched existing patterns before implementing
✅ Understood HAPI response contract
✅ Identified integration points

### **Plan Prevention**
✅ TDD phases defined (RED-GREEN-REFACTOR)
✅ Success criteria established
✅ Timeline realistic (2-3 hours)

### **Do Prevention**
✅ Tests written FIRST (RED phase)
✅ Minimal implementation (GREEN phase)
✅ Enhancement only in REFACTOR phase

### **Check Prevention**
✅ Business alignment verified
✅ Integration tested
✅ Documentation updated

---

## 📝 Implementation Checklist

### **Pre-Implementation**
- [ ] Read HAPI OpenAPI spec for `/recovery/analyze` response
- [ ] Review existing `investigating.go` code patterns
- [ ] Check ogen-generated types in `pkg/clients/holmesgpt/`

### **RED Phase**
- [ ] Write unit test: "should populate RecoveryStatus when isRecoveryAttempt is true"
- [ ] Write unit test: "should NOT populate RecoveryStatus for initial incidents"
- [ ] Write unit test: "should leave RecoveryStatus as nil when HAPI doesn't return recovery_analysis"
- [ ] Add integration test assertion in `recovery_integration_test.go`
- [ ] Run tests: `go test -v ./test/unit/aianalysis/...` → Expected: FAIL ❌

### **GREEN Phase**
- [ ] Implement `populateRecoveryStatus()` helper function
- [ ] Add call after `InvestigateRecovery()` in `investigating.go`
- [ ] Map all 4 fields: `StateChanged`, `CurrentSignalType`, `FailureUnderstood`, `FailureReasonAnalysis`
- [ ] Run tests: `go test -v ./test/unit/aianalysis/...` → Expected: PASS ✅
- [ ] Run integration tests: `make test-integration-aianalysis` → Expected: PASS ✅

### **REFACTOR Phase**
- [ ] Add structured logging
- [ ] Add metrics (optional)
- [ ] Add edge case tests (nil assessment, empty signal type)
- [ ] Add defensive nil checks
- [ ] Run all tests: `make test-unit-aianalysis test-integration-aianalysis` → Expected: PASS ✅

### **CHECK Phase**
- [ ] Verify crd-schema.md example matches implementation
- [ ] Run lint: `golangci-lint run pkg/aianalysis/handlers/investigating.go`
- [ ] Build: `make build-aianalysis`
- [ ] E2E test (manual): Verify RecoveryStatus in `kubectl describe`
- [ ] Update AIANALYSIS_TRIAGE.md: Mark RecoveryStatus as COMPLETE
- [ ] Update V1.0_FINAL_CHECKLIST.md: Mark Task 4 as COMPLETE

---

## 🔗 References

| Document | Purpose |
|----------|---------|
| `api/aianalysis/v1alpha1/aianalysis_types.go:528` | RecoveryStatus type definition |
| `docs/services/crd-controllers/02-aianalysis/crd-schema.md:679` | Example showing populated RecoveryStatus |
| `docs/architecture/decisions/DD-RECOVERY-002` | Recovery flow architecture |
| `pkg/aianalysis/handlers/investigating.go` | Existing HAPI integration code |
| `holmesgpt-api/src/extensions/recovery.py:603-609` | HAPI response structure |
| `test/unit/aianalysis/investigating_handler_test.go` | Existing test patterns |

---

**Status**: 📋 READY FOR IMPLEMENTATION
**Priority**: 🔴 BLOCKING V1.0
**Methodology**: APDC + TDD
**Estimated Completion**: 2-3 hours

