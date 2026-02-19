# AIAnalysis Integration Tests - Business Outcome Edge Case Triage

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed: DetectedLabels is now computed by HAPI post-RCA (ADR-056), and OwnerChain is resolved via get_resource_context (ADR-055).

**Date**: December 16, 2025
**Service**: AIAnalysis (AA)
**Phase**: V1.0 Business Outcome Gap Analysis
**Focus**: Business value delivered to operators/users, NOT infrastructure concerns
**Status**: 🔍 ANALYSIS COMPLETE - 11 Business Outcome Edge Cases Identified (1 deferred to V1.1)
**Confidence**: 90%+ Business Value

---

## 🎯 **Executive Summary**

**Objective**: Identify integration test gaps for AIAnalysis focused on **business outcomes** - what the system delivers to operators and users, not infrastructure/performance concerns.

**Focus Areas**:
- ✅ **Investigation Quality**: Can operators understand what happened?
- ✅ **Recommendation Correctness**: Are AI recommendations accurate and safe?
- ✅ **Approval Decision Quality**: Can operators make informed approval decisions?
- ✅ **Audit Trail Completeness**: Can operators trace what the system did?
- ✅ **Recovery Effectiveness**: Do recovery attempts learn from failures?
- ✅ **Cross-Phase Context**: Is business context preserved across phases?

**Out of Scope** (Non-Functional):
- ❌ Performance tuning (timeouts, payload sizes)
- ❌ Infrastructure reliability (retries, circuit breakers)
- ❌ Operational concerns (hot-reload, resource limits)

**Findings**:
- **Current Coverage**: Strong on happy path, weak on error scenarios
- **Gaps Identified**: 12 high-value business outcome edge cases
- **Business Impact**: High (operator trust, decision quality, compliance)

---

## 📊 **Current Business Outcome Coverage**

### Well-Covered Business Outcomes ✅

| Business Outcome | Test File | What Operators Get |
|------------------|-----------|-------------------|
| **AI produces workflow recommendations** | holmesgpt_integration_test.go | Selected workflow with rationale |
| **Approval routing is correct** | rego_integration_test.go | Production requires approval |
| **Audit trail captures key events** | audit_integration_test.go | 6 event types with full payloads |
| **Recovery attempts use prior context** | recovery_integration_test.go | Previous failures inform recovery |
| **Alternative workflows are available** | holmesgpt_integration_test.go | Operator has choices |
| **Confidence scores guide automation** | metrics_integration_test.go | Histogram tracks confidence |

---

### Business Outcome Gaps ⚠️

| Business Outcome | Current Coverage | Operator Impact |
|------------------|------------------|-----------------|
| **Operators can trace cross-phase decisions** | 🟡 Partial | Medium - Hard to debug multi-phase flows |
| **Audit trail survives error scenarios** | ❌ None | High - Lose context when failures occur |
| **Root cause evidence is actionable** | ❌ None | High - Operators can't validate AI claims |
| **Workflow selection rationale is complete** | 🟡 Partial | Medium - Hard to trust AI decisions |
| **Data quality issues are visible** | ❌ None | High - Operators don't know why approval required |
| **Recovery learns from failure patterns** | ❌ NOT IMPLEMENTED | N/A - PreviousExecution context not passed to HAPI (V1.1) |
| **Policy decisions are auditable** | ❌ None | High - Compliance gap |
| **Human review reasons are specific** | 🟡 Partial | Medium - Generic "needs review" unhelpful |
| **Alternative workflow trade-offs clear** | ❌ None | Medium - Operators can't compare options |
| **Approval context is decision-ready** | ❌ None | High - Operators need more info to approve |
| **Investigation summary is complete** | ❌ None | Medium - Hard to understand what AI found |
| **Confidence scores match reality** | ❌ None | Medium - Trust calibration |

---

## 🚨 **High-Priority Business Outcome Edge Cases**

### Priority 1: Audit Trail Completeness (Critical for Compliance)

#### 1. **Cross-Phase Audit Correlation** 🔥

**Business Outcome**: Operators can trace a complete analysis journey across all phases

**Example**: "Show me everything that happened for remediation-request-12345"

**Current Gap**: Audit events exist but cross-phase correlation not tested

```go
Context("Cross-Phase Audit Correlation - DD-AUDIT-003", func() {
    It("should maintain complete audit trail across all 4 phases", func() {
        By("Creating AIAnalysis and progressing through phases")
        analysis := createTestAnalysis()
        analysis.Spec.RemediationID = "test-remediation-12345"

        // Phase 1: Pending → Investigating
        mockClient.WithFullResponse(...)
        handler.Handle(ctx, analysis)

        // Phase 2: Investigating → Analyzing
        analysis.Status.Phase = "Analyzing"
        handler.Handle(ctx, analysis)

        // Phase 3: Analyzing → Completed
        analysis.Status.Phase = "Completed"
        handler.Handle(ctx, analysis)

        By("Querying all audit events for this remediation")
        events := getAuditEventsByCorrelationID(ctx, "test-remediation-12345")

        By("Verifying complete audit trail exists")
        Expect(events).To(HaveLen(6)) // Phase transitions + key events

        By("Verifying audit events are time-ordered")
        for i := 1; i < len(events); i++ {
            Expect(events[i].EventTimestamp).To(BeTemporally(">=", events[i-1].EventTimestamp))
        }

        By("Verifying critical business context preserved across phases")
        // All events should have same correlation_id
        for _, event := range events {
            Expect(event.CorrelationID).To(Equal("test-remediation-12345"))
        }

        By("Verifying operators can reconstruct decision flow")
        // Phase transitions should reference previous phase
        phaseTransitions := filterEventsByType(events, "aianalysis.phase.transition")
        Expect(phaseTransitions).To(HaveLen(3)) // Pending→Investigating→Analyzing→Completed

        // Business value: Full forensic capability
    })
})
```

**Business Value**: **95% Confidence**
- Operators can answer: "What did the AI do and why?"
- Forensic analysis of problematic recommendations
- Compliance audits require complete trails

**Authoritative Reference**: DD-AUDIT-003, BR-AI-030

---

#### 2. **Audit Trail Survives Error Scenarios** 🔥

**Business Outcome**: Operators can debug failures with audit context

**Example**: "Why did this analysis fail? Show me what happened."

**Current Gap**: Audit integration tests only cover happy path

```go
Context("Audit Trail Completeness on Errors", func() {
    It("should capture audit trail even when HolmesGPT-API fails", func() {
        By("Simulating HAPI permanent failure")
        mockClient.WithError(fmt.Errorf("401 Unauthorized"))
        analysis := createTestAnalysis()
        analysis.Spec.RemediationID = "test-failure-001"

        result, err := handler.Handle(ctx, analysis)

        By("Verifying analysis fails as expected")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.Phase).To(Equal("Failed"))

        By("Verifying audit trail captured the failure")
        events := getAuditEventsByCorrelationID(ctx, "test-failure-001")

        By("Verifying error audit event exists")
        errorEvents := filterEventsByType(events, "aianalysis.error.occurred")
        Expect(errorEvents).To(HaveLen(1))

        By("Verifying error event contains actionable context")
        errorEvent := errorEvents[0]
        Expect(errorEvent.EventOutcome).To(Equal("failure"))
        Expect(errorEvent.ErrorCode).To(Equal("PermanentError"))
        Expect(errorEvent.ErrorMessage).To(ContainSubstring("401 Unauthorized"))

        // Verify event_data has phase context
        eventData := unmarshalEventData(errorEvent.EventData)
        Expect(eventData["phase"]).To(Equal("Investigating"))
        Expect(eventData["error_type"]).To(Equal("PermanentError"))

        By("Verifying operators can understand failure cause")
        // Business value: Debugging capability
    })

    It("should capture audit trail when Rego policy fails", func() {
        By("Simulating Rego evaluation error")
        // Policy file missing/corrupted
        analysis := createTestAnalysis()
        analysis.Spec.RemediationID = "test-policy-failure-001"

        // Simulate policy evaluation failure (degraded mode)
        // ... test that audit captures degraded policy evaluation

        By("Verifying audit shows degraded policy state")
        events := getAuditEventsByCorrelationID(ctx, "test-policy-failure-001")
        regoEvents := filterEventsByType(events, "aianalysis.rego.evaluation")

        Expect(regoEvents[0].EventData).To(ContainSubstring("degraded"))

        // Business value: Policy debugging capability
    })
})
```

**Business Value**: **95% Confidence**
- Operators can debug why analyses fail
- Error context preserved for post-mortems
- Critical for production troubleshooting

**Authoritative Reference**: DD-AUDIT-003, BR-AI-009

---

### Priority 2: Investigation Quality (Operator Trust)

#### 3. **Root Cause Evidence is Actionable** 🔥

**Business Outcome**: Operators can validate AI's root cause claims with evidence

**Example**: "AI says memory leak - show me the evidence"

**Current Gap**: RCA audit exists but evidence quality/actionability not tested

```go
Context("Root Cause Evidence Quality - BR-AI-012", func() {
    It("should provide verifiable evidence for root cause claims", func() {
        By("Simulating HAPI response with detailed RCA")
        rcaMap := testutil.BuildMockRootCauseAnalysis(
            "Container OOMKilled due to memory leak in application",
            "high",
            []string{
                "Memory usage increased 15% per hour over 4 hours",
                "Heap dump shows 2.5GB of unclosed HTTP connections",
                "Pattern matches 3 previous incidents with same deployment",
            },
        )
        mockClient.WithIncidentResponseRCA(rcaMap)

        result, err := handler.Handle(ctx, analysis)

        By("Verifying analysis captures RCA in status")
        Expect(err).NotTo(HaveOccurred())
        rca := analysis.Status.RootCauseAnalysis
        Expect(rca).NotTo(BeEmpty())

        By("Verifying RCA summary is specific, not generic")
        Expect(rca["summary"]).To(ContainSubstring("memory leak"))
        Expect(rca["summary"]).NotTo(Equal("Issue detected")) // Generic = bad

        By("Verifying RCA includes quantifiable evidence")
        factors := rca["contributing_factors"].([]string)
        Expect(factors).To(HaveLen(3))

        // Evidence should have specific metrics
        hasQuantifiableEvidence := false
        for _, factor := range factors {
            if strings.Contains(factor, "%") || strings.Contains(factor, "GB") || strings.Contains(factor, "hours") {
                hasQuantifiableEvidence = true
                break
            }
        }
        Expect(hasQuantifiableEvidence).To(BeTrue(), "Evidence should include specific metrics")

        By("Verifying severity assessment matches evidence")
        Expect(rca["severity"]).To(Equal("high"))

        By("Verifying audit event captures full RCA context")
        events := getAuditEventsByCorrelationID(ctx, analysis.Spec.RemediationID)
        analysisCompleteEvents := filterEventsByType(events, "aianalysis.analysis.completed")
        Expect(analysisCompleteEvents).To(HaveLen(1))

        eventData := unmarshalEventData(analysisCompleteEvents[0].EventData)
        Expect(eventData["root_cause_summary"]).To(ContainSubstring("memory leak"))

        // Business value: Operator can validate AI's claim
    })

    It("should distinguish between strong and weak evidence", func() {
        By("Simulating weak evidence scenario")
        rcaMap := testutil.BuildMockRootCauseAnalysis(
            "Possible network issue",
            "medium",
            []string{
                "Intermittent connection failures observed",
                "No clear pattern identified",
            },
        )
        mockClient.WithIncidentResponseRCA(rcaMap)

        result, err := handler.Handle(ctx, analysis)

        By("Verifying approval is required for weak evidence")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.ApprovalRequired).To(BeTrue())
        Expect(analysis.Status.ApprovalReason).To(ContainSubstring("insufficient evidence"))

        // Business value: Operators don't trust weak AI claims
    })
})
```

**Business Value**: **95% Confidence**
- Operators can verify AI's reasoning
- Builds trust in AI recommendations
- Distinguishes high-quality from low-quality investigations

**Authoritative Reference**: BR-AI-012, BR-AI-010

---

#### 4. **Workflow Selection Rationale Completeness** 🔥

**Business Outcome**: Operators understand WHY AI chose this workflow

**Example**: "Why restart instead of scale?"

**Current Gap**: Rationale field exists but completeness/quality not validated

```go
Context("Workflow Selection Rationale - BR-AI-075, BR-AI-010", func() {
    It("should provide complete rationale for workflow selection", func() {
        By("Simulating HAPI response with workflow selection")
        mockClient.WithFullResponse(
            "Analysis complete",
            0.85,
            []string{},
            "Memory leak detected",
            "high",
            "restart-pod-v1",
            "registry.io/kubernaut/workflows/restart:v1.0.0",
            0.88,
            true,
            "Pod restart will clear memory leak. Memory usage pattern shows 15% hourly increase, heap analysis shows unclosed connections. Restart is lower risk than memory increase as it preserves resource limits.",
            true, // Include alternatives
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying workflow selection is captured")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())

        By("Verifying rationale is substantive, not generic")
        rationale := analysis.Status.SelectedWorkflow.Rationale
        Expect(rationale).NotTo(BeEmpty())
        Expect(rationale).NotTo(Equal("Recommended workflow")) // Generic = bad
        Expect(len(rationale)).To(BeNumerically(">", 50), "Rationale should be detailed")

        By("Verifying rationale explains WHAT problem it solves")
        Expect(rationale).To(ContainSubstring("memory leak"))

        By("Verifying rationale explains WHY this approach")
        Expect(rationale).To(ContainSubstring("restart")) // Action
        Expect(rationale).To(ContainSubstring("clear")) // Expected outcome

        By("Verifying rationale includes evidence reference")
        // Should reference specific evidence from RCA
        Expect(rationale).To(Or(
            ContainSubstring("%"),
            ContainSubstring("pattern"),
            ContainSubstring("analysis"),
        ))

        By("Verifying alternative workflows provide comparison")
        Expect(analysis.Status.AlternativeWorkflows).To(HaveLen(1))
        alt := analysis.Status.AlternativeWorkflows[0]
        Expect(alt.Rationale).To(ContainSubstring("risk")) // Trade-off explanation

        // Business value: Operators can compare options intelligently
    })

    It("should flag when rationale is missing or weak", func() {
        By("Simulating HAPI response with empty rationale")
        mockClient.WithFullResponse(
            "Analysis complete",
            0.65, // Lower confidence
            []string{"Rationale quality low"},
            "Issue detected",
            "medium",
            "generic-fix-v1",
            "registry.io/kubernaut/workflows/generic:v1.0.0",
            0.60,
            true,
            "", // Empty rationale = problem
            false,
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying approval is required for weak rationale")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.ApprovalRequired).To(BeTrue())
        Expect(analysis.Status.ApprovalReason).To(ContainSubstring("weak rationale"))

        // Business value: Protect operators from blind trust
    })
})
```

**Business Value**: **90% Confidence**
- Operators understand AI's reasoning
- Enables comparison of alternatives
- Critical for approval decisions

**Authoritative Reference**: BR-AI-075, BR-AI-010, BR-AI-006

---

### Priority 3: Approval Decision Quality (Operator UX)

#### 5. **Data Quality Issues Are Visible** 🔥

**Business Outcome**: Operators know WHY approval is required (data quality vs policy)

**Example**: "Why does this need approval? Environment is staging."

**Current Gap**: Approval required is tested, but visibility of WHY is not

```go
Context("Data Quality Visibility - BR-AI-076", func() {
    It("should distinguish data quality issues from policy decisions", func() {
        By("Simulating analysis with data quality issue")
        mockClient.WithFullResponse(
            "Analysis complete but target validation failed",
            0.85, // High confidence
            []string{"Target resource not in owner chain"},
            "Resource issue detected",
            "medium",
            "fix-v1",
            "registry.io/kubernaut/workflows/fix:v1.0.0",
            0.85,
            false, // ← targetInOwnerChain = false (data quality issue)
            "Fix recommended",
            false,
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying approval is required")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.ApprovalRequired).To(BeTrue())

        By("Verifying approval reason is SPECIFIC about data quality")
        Expect(analysis.Status.ApprovalReason).To(ContainSubstring("data quality"))
        Expect(analysis.Status.ApprovalReason).To(ContainSubstring("target not in owner chain"))

        By("Verifying approval context shows data quality issues")
        Expect(analysis.Status.ApprovalContext).NotTo(BeNil())
        Expect(analysis.Status.ApprovalContext.DataQualityIssues).To(ContainElement("targetInOwnerChain=false"))

        By("Verifying audit captures data quality as approval driver")
        events := getAuditEventsByCorrelationID(ctx, analysis.Spec.RemediationID)
        approvalEvents := filterEventsByType(events, "aianalysis.approval.decision")

        eventData := unmarshalEventData(approvalEvents[0].EventData)
        Expect(eventData["approval_reason"]).To(ContainSubstring("data quality"))

        // Business value: Operators understand root cause of approval requirement
    })

    It("should distinguish policy-driven approval from quality-driven approval", func() {
        By("Simulating production analysis (policy-driven approval)")
        analysis := createTestAnalysis()
        analysis.Spec.AnalysisRequest.SignalContext.Environment = "production"

        mockClient.WithFullResponse(
            "Analysis complete",
            0.95, // Very high confidence
            []string{}, // No warnings
            "Clear root cause",
            "high",
            "fix-v1",
            "registry.io/kubernaut/workflows/fix:v1.0.0",
            0.95,
            true, // Data quality OK
            "High confidence fix",
            false,
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying approval required due to policy, not quality")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.ApprovalRequired).To(BeTrue())
        Expect(analysis.Status.ApprovalReason).To(ContainSubstring("production environment"))
        Expect(analysis.Status.ApprovalReason).NotTo(ContainSubstring("data quality"))

        By("Verifying approval context shows policy as driver")
        Expect(analysis.Status.ApprovalContext.PolicyEvaluation).NotTo(BeEmpty())
        Expect(analysis.Status.ApprovalContext.DataQualityIssues).To(BeEmpty())

        // Business value: Operators understand policy vs quality issues
    })
})
```

**Business Value**: **95% Confidence**
- Operators understand root cause of approval
- Reduces confusion ("Why does this need approval?")
- Enables targeted fixes (policy vs data quality)

**Authoritative Reference**: BR-AI-076, BR-AI-013

---

#### 6. **Approval Context is Decision-Ready** 🔥

**Business Outcome**: Operators have everything needed to approve/reject

**Example**: "Can I safely approve this without additional investigation?"

**Current Gap**: Approval context fields exist but decision-readiness not validated

```go
Context("Approval Context Decision-Readiness - BR-AI-076", func() {
    It("should provide complete decision-ready context for low confidence", func() {
        By("Simulating low-confidence scenario")
        mockClient.WithFullResponse(
            "Possible solution identified",
            0.72, // Below 80% threshold
            []string{"Limited historical data", "Multiple root cause candidates"},
            "Suspected memory issue",
            "medium",
            "restart-pod-v1",
            "registry.io/kubernaut/workflows/restart:v1.0.0",
            0.75,
            true,
            "Best guess based on available evidence",
            true, // Include alternatives
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying all approval context fields populated")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.ApprovalRequired).To(BeTrue())

        ctx := analysis.Status.ApprovalContext
        Expect(ctx).NotTo(BeNil())

        By("Verifying context has confidence score")
        Expect(ctx.Confidence).To(BeNumerically("==", 0.72))

        By("Verifying context has investigation summary")
        Expect(ctx.InvestigationSummary).NotTo(BeEmpty())
        Expect(len(ctx.InvestigationSummary)).To(BeNumerically(">", 50), "Summary should be detailed")

        By("Verifying context has evidence collected")
        Expect(ctx.EvidenceCollected).NotTo(BeEmpty())
        Expect(ctx.EvidenceCollected).To(HaveLen(2)) // Warnings from HAPI

        By("Verifying context has alternatives for comparison")
        Expect(ctx.AlternativesConsidered).To(HaveLen(1))
        alt := ctx.AlternativesConsidered[0]
        Expect(alt.WorkflowID).NotTo(BeEmpty())
        Expect(alt.Confidence).To(BeNumerically(">", 0))
        Expect(alt.Rationale).NotTo(BeEmpty())

        By("Verifying context explains confidence score")
        Expect(ctx.ConfidenceExplanation).To(ContainSubstring("limited historical data"))

        By("Verifying operators can make decision without additional context")
        // Has: what (recommendation), why (rationale), how confident (score),
        // what else (alternatives), what's the risk (evidence quality)

        // Business value: Fast approval decisions
    })

    It("should provide comparison matrix for alternatives", func() {
        By("Simulating multiple alternatives scenario")
        // ... test that alternatives include trade-off explanations

        By("Verifying alternatives show risk/benefit trade-offs")
        // Alternative 1: Fast but higher risk
        // Alternative 2: Slow but safer
        // Operators can choose based on situation

        // Business value: Contextual decision-making
    })
})
```

**Business Value**: **90% Confidence**
- Reduces approval latency (no back-and-forth)
- Improves approval decision quality
- Meets ADR-018 target (<5% approval miss rate)

**Authoritative Reference**: BR-AI-076, ADR-018

---

### Priority 4: Recovery Effectiveness (Learning from Failures)

#### 7. **Recovery Learns from Failure Patterns** ❌ **NOT YET SUPPORTED**

**Business Outcome**: Recovery attempts don't repeat ineffective strategies

**Example**: "Pod restart failed 3 times - AI should try different approach"

**Current Status**: ❌ **Feature not implemented in V1.0**
- CRD schema has `PreviousExecution` struct defined
- Handler has TODO comment (line 214-215 in `investigating.go`)
- `buildPreviousExecution` is stubbed out
- **HAPI receives NO failure history** - cannot learn from patterns

**Implementation Gap**: Recovery endpoint tested, but `PreviousExecution` context not passed to HAPI

```go
Context("Recovery Learning from Failures - BR-AI-080, BR-AI-081", func() {
    It("should avoid repeating failed workflow types", func() {
        By("Simulating 3rd recovery attempt after 2 restart failures")
        analysis := createTestAnalysis()
        analysis.Spec.IsRecoveryAttempt = true
        analysis.Spec.RecoveryAttemptNumber = 3

        analysis.Spec.PreviousExecution = &aianalysisv1.PreviousExecution{
            WorkflowID: "restart-pod-v2",
            ExecutionID: "exec-002",
            Outcome: "Failed",
            FailureReason: "Pod restarted but OOMKilled again",
            AttemptHistory: []aianalysisv1.RecoveryAttempt{
                {
                    AttemptNumber: 1,
                    WorkflowID: "restart-pod-v1",
                    Outcome: "Failed",
                    FailureReason: "OOMKilled 5 minutes after restart",
                },
                {
                    AttemptNumber: 2,
                    WorkflowID: "restart-pod-v2", // Same type again
                    Outcome: "Failed",
                    FailureReason: "OOMKilled again",
                },
            },
        }

        mockClient.WithRecoverySuccessResponse(
            0.70, // Lower confidence after failures
            "increase-memory-limit-v1", // ← DIFFERENT strategy
            "registry.io/kubernaut/workflows/increase-memory:v1.0.0",
            0.65,
            true, // Recovery analysis shows learning
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying HAPI received failure pattern context")
        Expect(mockClient.LastRecoveryRequest).NotTo(BeNil())
        Expect(mockClient.LastRecoveryRequest.RecoveryAttemptNumber.Value).To(Equal(3))

        By("Verifying AI selects DIFFERENT workflow type")
        Expect(err).NotTo(HaveOccurred())
        selectedWF := analysis.Status.SelectedWorkflow.WorkflowID
        Expect(selectedWF).NotTo(ContainSubstring("restart")) // Avoid failed type
        Expect(selectedWF).To(ContainSubstring("increase-memory")) // New strategy

        By("Verifying rationale explains strategy change")
        rationale := analysis.Status.SelectedWorkflow.Rationale
        Expect(rationale).To(ContainSubstring("previous restart attempts failed"))
        Expect(rationale).To(ContainSubstring("memory limit"))

        By("Verifying recovery analysis captures learning")
        // Check audit event has recovery_analysis showing failure pattern recognition
        events := getAuditEventsByCorrelationID(ctx, analysis.Spec.RemediationID)
        hapiEvents := filterEventsByType(events, "aianalysis.holmesgpt.call")

        eventData := unmarshalEventData(hapiEvents[0].EventData)
        recoveryAnalysis := eventData["recovery_analysis"].(map[string]interface{})
        Expect(recoveryAnalysis["state_changed"]).To(BeFalse()) // Failure pattern detected
        Expect(recoveryAnalysis["failure_understood"]).To(BeTrue())

        // Business value: Avoid wasting time on failed strategies
    })

    It("should escalate to investigation when recovery fails repeatedly", func() {
        By("Simulating 5th recovery attempt (multiple strategies failed)")
        analysis := createTestAnalysis()
        analysis.Spec.IsRecoveryAttempt = true
        analysis.Spec.RecoveryAttemptNumber = 5 // High attempt number

        // Failed: restart, increase-memory, scale-up, restart-with-debug
        analysis.Spec.PreviousExecution = &aianalysisv1.PreviousExecution{
            AttemptHistory: []aianalysisv1.RecoveryAttempt{
                {AttemptNumber: 1, WorkflowID: "restart-pod-v1", Outcome: "Failed"},
                {AttemptNumber: 2, WorkflowID: "increase-memory-v1", Outcome: "Failed"},
                {AttemptNumber: 3, WorkflowID: "scale-up-v1", Outcome: "Failed"},
                {AttemptNumber: 4, WorkflowID: "restart-debug-v1", Outcome: "Failed"},
            },
        }

        mockClient.WithRecoverySuccessResponse(
            0.45, // Very low confidence
            "investigate-root-cause-v1", // ← DIAGNOSTIC workflow
            "registry.io/kubernaut/workflows/investigate:v1.0.0",
            0.40,
            true,
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying AI escalates to diagnostic workflow")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.SelectedWorkflow.WorkflowID).To(ContainSubstring("investigate"))

        By("Verifying approval is required for diagnostic escalation")
        Expect(analysis.Status.ApprovalRequired).To(BeTrue())
        Expect(analysis.Status.ApprovalReason).To(ContainSubstring("multiple recovery failures"))

        // Business value: Recognize when automated recovery exhausted
    })
})
```

**Business Value**: **90% Confidence**
- Avoids wasting time/resources on failed strategies
- Intelligent escalation when stuck
- Critical for persistent failures

**Authoritative Reference**: BR-AI-080, BR-AI-081, DD-RECOVERY-002

---

### Priority 5: Policy Decision Auditability (Compliance)

#### 8. **Policy Decisions Are Fully Auditable** 🔥

**Business Outcome**: Compliance audits can reconstruct policy decisions

**Example**: "Show me all auto-approvals in production last month and their justification"

**Current Gap**: Rego evaluation audit exists but policy decision context not validated

```go
Context("Policy Decision Auditability - BR-AI-030", func() {
    It("should capture complete policy evaluation context", func() {
        By("Simulating production analysis with policy evaluation")
        analysis := createTestAnalysis()
        analysis.Spec.AnalysisRequest.SignalContext.Environment = "production"
        analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = map[string]interface{}{
            "gitOpsManaged": true,
            "pdbProtected":  true,
            "stateful":      false,
        }

        mockClient.WithFullResponse(
            "Analysis complete",
            0.92,
            []string{},
            "Clear issue",
            "high",
            "fix-v1",
            "registry.io/kubernaut/workflows/fix:v1.0.0",
            0.90,
            true,
            "High confidence fix",
            false,
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying policy evaluation is audited")
        Expect(err).NotTo(HaveOccurred())

        events := getAuditEventsByCorrelationID(ctx, analysis.Spec.RemediationID)
        regoEvents := filterEventsByType(events, "aianalysis.rego.evaluation")
        Expect(regoEvents).To(HaveLen(1))

        By("Verifying audit captures policy input")
        eventData := unmarshalEventData(regoEvents[0].EventData)
        Expect(eventData["outcome"]).To(Equal("deny")) // Production requires approval
        Expect(eventData["degraded"]).To(BeFalse())
        Expect(eventData["reason"]).To(ContainSubstring("production environment"))

        By("Verifying audit shows which policy rules triggered")
        // Policy evaluation should explain WHICH rules were evaluated
        Expect(eventData["duration_ms"]).To(BeNumerically(">", 0))

        By("Verifying operators can query by policy outcome")
        // Query: "Find all analyses that were auto-approved"
        autoApprovedEvents := queryAuditEventsByPolicyOutcome(ctx, "allow")
        // Query: "Find all analyses that required approval"
        approvalRequiredEvents := queryAuditEventsByPolicyOutcome(ctx, "deny")

        // Business value: Compliance reporting, policy tuning
    })

    It("should track policy version for audit trail integrity", func() {
        By("Simulating analysis with specific policy version")
        // Policy ConfigMap has version label
        analysis := createTestAnalysis()

        result, err := handler.Handle(ctx, analysis)

        By("Verifying audit captures policy version/hash")
        events := getAuditEventsByCorrelationID(ctx, analysis.Spec.RemediationID)
        regoEvents := filterEventsByType(events, "aianalysis.rego.evaluation")

        eventData := unmarshalEventData(regoEvents[0].EventData)
        Expect(eventData).To(HaveKey("policy_hash"))
        Expect(eventData["policy_hash"]).NotTo(BeEmpty())

        // Business value: Audit trail shows which policy was active
    })
})
```

**Business Value**: **85% Confidence**
- Compliance audits can reconstruct decisions
- Policy tuning based on actual outcomes
- Regulatory requirement for approval decisions

**Authoritative Reference**: BR-AI-030, BR-AI-026

---

### Priority 6: Human Review Specificity (Operator UX)

#### 9. **Human Review Reasons Are Actionable** 🔥

**Business Outcome**: Operators know SPECIFICALLY why human review needed

**Example**: "Don't just say 'needs review' - tell me WHY"

**Current Gap**: 7 enum values tested but actionability not validated

```go
Context("Human Review Reason Specificity - BR-HAPI-197", func() {
    It("should provide actionable context for 'low_confidence' review reason", func() {
        By("Simulating low confidence with specific root causes")
        mockClient.WithHumanReviewReasonEnum(
            "low_confidence",
            []string{
                "Historical success rate for this workflow is only 45%",
                "Similar incidents have 60% failure rate",
                "Insufficient observability data to confirm root cause",
            },
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying human review flag is set")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.HumanReviewRequired).To(BeTrue())
        Expect(analysis.Status.HumanReviewReason).To(Equal("low_confidence"))

        By("Verifying warnings provide SPECIFIC confidence issues")
        warnings := analysis.Status.Warnings
        Expect(warnings).NotTo(BeEmpty())
        Expect(warnings).To(ContainElement(ContainSubstring("success rate")))
        Expect(warnings).NotTo(ContainElement("Low confidence")) // Generic = bad

        By("Verifying approval context explains confidence issues")
        ctx := analysis.Status.ApprovalContext
        Expect(ctx.ConfidenceExplanation).To(ContainSubstring("Historical success rate"))

        // Business value: Operators know what data is missing/weak
    })

    It("should provide actionable context for 'investigation_inconclusive' reason", func() {
        By("Simulating inconclusive investigation")
        mockClient.WithInvestigationInconclusive("investigation_inconclusive")

        result, err := handler.Handle(ctx, analysis)

        By("Verifying operators understand WHY inconclusive")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.HumanReviewReason).To(Equal("investigation_inconclusive"))

        By("Verifying warnings explain WHAT was inconclusive")
        // Not just "investigation inconclusive"
        // Should say: "No clear root cause identified - 3 candidate causes with similar likelihood"
        Expect(analysis.Status.Warnings).NotTo(BeEmpty())
        Expect(analysis.Status.Warnings[0]).To(ContainSubstring("root cause"))

        // Business value: Operators know what additional investigation needed
    })

    It("should track validation attempt failures when 'validation_failed' reason", func() {
        By("Simulating failed LLM self-correction attempts")
        validationAttempts := testutil.NewMockValidationAttempts([]string{
            "Workflow ID not found in catalog",
            "Parameters missing required field",
            "ContainerImage format invalid",
        })
        mockClient.WithHumanReviewAndHistory(
            "validation_failed",
            []string{"LLM could not self-correct after 3 attempts"},
            validationAttempts,
        )

        result, err := handler.Handle(ctx, analysis)

        By("Verifying operators see validation history")
        Expect(err).NotTo(HaveOccurred())
        Expect(analysis.Status.ValidationAttemptsHistory).NotTo(BeEmpty())
        Expect(analysis.Status.ValidationAttemptsHistory).To(HaveLen(3))

        By("Verifying each attempt shows WHAT failed")
        for i, attempt := range analysis.Status.ValidationAttemptsHistory {
            Expect(attempt.Reason).NotTo(Equal("validation failed")) // Generic = bad
            Expect(attempt.Reason).To(ContainSubstring("catalog") ||
                                       ContainSubstring("parameters") ||
                                       ContainSubstring("image"))
        }

        // Business value: Operators understand LLM quality issues
    })
})
```

**Business Value**: **85% Confidence**
- Operators know specific action needed
- Reduces back-and-forth clarification
- Improves human review efficiency

**Authoritative Reference**: BR-HAPI-197, DD-HAPI-002

---

### Priority 7: Confidence Score Calibration (Trust)

#### 10. **Confidence Scores Match Reality**

**Business Outcome**: Operators can trust the confidence numbers

**Example**: "If AI says 85% confident, is it actually right 85% of the time?"

**Current Gap**: Confidence scores tracked but calibration not validated

```go
Context("Confidence Score Calibration - BR-AI-003, BR-AI-022", func() {
    It("should track actual vs predicted confidence for calibration", func() {
        By("Simulating multiple analyses with different confidence levels")
        testCases := []struct {
            confidence float64
            outcome    string // "success" or "failure"
        }{
            {0.90, "success"}, // High confidence, succeeded
            {0.85, "success"},
            {0.80, "success"},
            {0.75, "failure"}, // Medium confidence, failed
            {0.70, "failure"},
            {0.60, "success"}, // Low confidence, but succeeded
            {0.50, "failure"}, // Low confidence, failed
        }

        for i, tc := range testCases {
            By(fmt.Sprintf("Test case %d: confidence=%.2f, outcome=%s", i+1, tc.confidence, tc.outcome))
            analysis := createTestAnalysis()
            analysis.Name = fmt.Sprintf("test-analysis-%d", i+1)

            mockClient.WithFullResponse(
                "Analysis",
                tc.confidence,
                []string{},
                "RCA",
                "medium",
                fmt.Sprintf("workflow-v%d", i+1),
                "registry.io/kubernaut/workflows/fix:v1.0.0",
                tc.confidence,
                true,
                "Rationale",
                false,
            )

            result, err := handler.Handle(ctx, analysis)
            Expect(err).NotTo(HaveOccurred())

            // Later: WorkflowExecution completes with tc.outcome
            // AIAnalysis should track: predicted=tc.confidence, actual=(success|failure)
        }

        By("Verifying confidence calibration can be computed")
        // Query: "For all analyses with confidence 80-90%, what % actually succeeded?"
        // If 85% confidence → 85% success rate, calibration is good
        // If 85% confidence → 50% success rate, calibration is poor

        By("Verifying operators can trust confidence scores")
        // Calibration metric should be exposed for monitoring

        // Business value: Trust in AI recommendations
    })
})
```

**Business Value**: **75% Confidence**
- Operators can trust confidence numbers
- Enables continuous model improvement
- Important for automation thresholds

**Authoritative Reference**: BR-AI-003, BR-AI-022

---

## 📊 **Business Outcome Priority Summary**

### By Confidence Level

| Confidence | Count | Focus Area |
|------------|-------|-----------|
| **95%** | 5 cases | Audit completeness, Investigation quality, Data quality visibility |
| **90%** | 3 cases | Approval UX, Recovery learning, Rationale quality |
| **85%** | 3 cases | Policy auditability, Human review specificity, Investigation summary |
| **75%** | 1 case | Confidence calibration |

### By Business Impact

| Impact | Count | Operator Need |
|--------|-------|---------------|
| **Critical** | 5 cases | "Can I trust this?" "What happened?" "Why approval?" |
| **High** | 4 cases | "What should I do?" "Did it learn?" "What's the evidence?" |
| **Medium** | 3 cases | "Is this specific enough?" "Can I verify claims?" "Are scores accurate?" |

---

## ✅ **V1.0 Implementation Recommendations**

### Must-Have for V1.0 (95% Confidence - 5 cases)
1. ✅ **Cross-phase audit correlation** (forensic capability)
2. ✅ **Audit trail survives errors** (debugging capability)
3. ✅ **Root cause evidence actionability** (operator trust)
4. ✅ **Data quality visibility** (approval UX)
5. ✅ **Workflow rationale completeness** (decision quality)

**Estimated Effort**: 20 hours (2.5 days)
**Business Value**: Core operator experience, trust building

### Should-Have for V1.0 (90%+ Confidence - 2 cases)
6. ✅ **Approval context decision-ready** (approval latency)
7. ✅ **Policy decision auditability** (compliance)

**Estimated Effort**: 8 hours (1 day)
**Total V1.0 Effort**: 28 hours (3.5 days)

### Deferred to V1.1+ (Not Yet Implemented)
8. ❌ **Recovery failure learning** (NOT SUPPORTED - PreviousExecution context not passed to HAPI)

### Deferred to V1.1 (85% and below - 4 cases)
9-12. ✅ Human review specificity, Confidence calibration, Investigation summary, Alternative comparison

**Estimated Effort**: 12-14 hours (1.5-2 days)

---

## 🔗 **Authoritative Documentation References**

### Business Requirements
- `docs/services/crd-controllers/02-aianalysis/BR_MAPPING.md` (v1.3)
- BR-AI-012: Root cause identification
- BR-AI-030: Policy audit trail
- BR-AI-076: Approval context
- BR-AI-080-081: Recovery flow

### Design Decisions
- DD-AUDIT-003: Audit event specification
- DD-RECOVERY-002: Recovery flow architecture
- DD-HAPI-002: Validation and self-correction

### Architectural Decisions
- ADR-018: Approval notification (<5% miss rate target)
- ADR-041: LLM response contract

---

## 📝 **Key Insight: Business vs Non-Functional**

### ✅ Business Outcomes (What Operators Get)
- **Investigation Quality**: "Do I understand what happened?"
- **Recommendation Trust**: "Can I trust this AI decision?"
- **Decision Support**: "Do I have enough context to approve?"
- **Forensic Capability**: "Can I debug what went wrong?"
- **Learning**: "Does the system get smarter over time?"

### ❌ Non-Functional Concerns (Not in Scope)
- Performance tuning (timeouts, payload sizes)
- Infrastructure reliability (retries, circuit breakers)
- Operational automation (hot-reload, resource limits)
- Scalability (large clusters, high cardinality)

**Focus**: Test what operators SEE and USE, not how the system achieves it.

---

**Document Version**: 1.0
**Created**: December 16, 2025
**Author**: AI Assistant
**Status**: ✅ COMPLETE - Ready for V1.0 Implementation
**Confidence**: 90%+ Business Value for All 12 Cases

