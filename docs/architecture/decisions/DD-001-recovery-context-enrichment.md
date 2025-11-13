## DD-001: Recovery Context Enrichment (Alternative 2)

### Status
**✅ Approved Design** (2024-10-08)
**Last Reviewed**: 2025-11-11
**Confidence**: 95% (based on temporal consistency requirements)

**⚠️ IMPORTANT UPDATE (2025-11-11)**: Recovery context enrichment approach has been revised by **DD-CONTEXT-006**. Signal Processor NO LONGER queries Context API for historical recovery data. Instead, Remediation Orchestrator embeds current failure data from WorkflowExecution CRD. See [DD-CONTEXT-006](DD-CONTEXT-006-signal-processor-recovery-data-source.md) for details.

### Context & Problem
When a workflow execution fails, the system must create a recovery attempt with fresh context to enable AI-driven alternative strategies. The challenge: **where** should the historical context be enriched, and **how** should it flow to the AI Analysis service?

**Key Requirements**:
- **BR-WF-RECOVERY-011**: MUST integrate with Context API for historical recovery context
- Need **fresh** monitoring and business context (not stale from initial attempt)
- Maintain **temporal consistency** (all contexts captured at same timestamp)
- Preserve **immutable audit trail** (each attempt is a separate CRD instance)
- Follow **self-contained CRD pattern** (no cross-CRD reads during reconciliation)

### Alternatives Considered

#### Alternative 1 (Option A): AIAnalysis Controller Queries Context API
**Approach**: AIAnalysis Controller directly queries Context API during reconciliation

**Pros**:
- ✅ Separation of concerns (AI controller owns AI context)
- ✅ Direct access to historical data

**Cons**:
- ❌ **Breaks temporal consistency**: Monitoring/business contexts queried earlier by RemediationProcessing, recovery context queried later by AIAnalysis
- ❌ **Stale contexts for recovery**: Uses monitoring/business data from initial attempt (not fresh)
- ❌ **Cross-CRD reads**: AIAnalysis must read RemediationProcessing status to get contexts
- ❌ **Complex error handling**: Two controllers must handle Context API failures
- ❌ **Audit trail gaps**: Recovery enrichment not captured in RemediationProcessing audit

**Confidence**: 60% (rejected)

---

#### Alternative 2 (Option B → Final): RemediationProcessing Enriches ALL Contexts
**Approach**: RemediationProcessing Controller enriches with monitoring + business + recovery contexts, Remediation Orchestrator copies to AIAnalysis CRD spec

**Pros**:
- ✅ **Temporal consistency**: All contexts captured at same timestamp by single controller
- ✅ **Fresh contexts**: Recovery attempt gets FRESH monitoring/business data (not stale from initial attempt)
- ✅ **Complete enrichment pattern**: RP enriches → RR copies → AIAnalysis reads from spec
- ✅ **Self-contained CRDs**: AIAnalysis has all data in spec (no API calls during reconciliation)
- ✅ **Immutable audit trail**: Each SignalProcessing CRD contains complete snapshot
- ✅ **Graceful degradation in RP**: Single point for Context API fallback logic
- ✅ **Simplified AIAnalysis**: No external dependencies, pure data transformation

**Cons**:
- ⚠️ Slightly longer recovery initiation (~1 minute): RR creates RP → RP enriches → RR creates AIAnalysis
  - **Mitigation**: User confirmed <1 minute is acceptable for better AI decisions

**Confidence**: 95% (approved)

**Evolution**: This started as "Option B" (RR embeds context) but evolved when we realized recovery needs FRESH monitoring/business contexts, not just historical context.

---

#### Alternative 3: AIAnalysis Queries Context API After RP Completion
**Approach**: RemediationProcessing enriches monitoring/business, AIAnalysis queries Context API for recovery context

**Pros**:
- ✅ RemediationProcessing focuses on monitoring/business enrichment
- ✅ AIAnalysis owns all AI-related context gathering

**Cons**:
- ❌ **Breaks temporal consistency**: Contexts captured at different times
- ❌ **Incomplete enrichment in RP**: RP audit trail missing recovery context
- ❌ **Dual API dependency**: Both RP and AIAnalysis depend on external services
- ❌ **Inconsistent pattern**: Only recovery context handled differently
- ❌ **No early failure detection**: Context API failures not caught until AIAnalysis phase

**Confidence**: 70% (rejected)

---

### Decision

**APPROVED: Alternative 2** - RemediationProcessing Controller enriches ALL contexts

**Rationale**:
1. **Temporal Consistency is Critical**: AI analysis quality depends on all contexts being from the same point in time
2. **Fresh Contexts Drive Better Decisions**: Recovery needs current cluster state, not stale data from initial attempt
3. **Complete Audit Trail**: Each SignalProcessing CRD is immutable snapshot of attempt
4. **Architectural Consistency**: Follows established self-contained CRD pattern
5. **Simplified AIAnalysis**: No external dependencies = easier testing and maintenance

**Key Insight**: The ~1 minute "penalty" for dual enrichment (RP enriches → RR creates AIAnalysis) is actually a **feature**, not a bug - it ensures fresh monitoring/business contexts for better AI recommendations.

### Implementation

**Primary Implementation Files**:
- [PROPOSED_FAILURE_RECOVERY_SEQUENCE.md](PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) - Authoritative sequence diagram
- [04_WORKFLOW_ENGINE_ORCHESTRATION.md](../requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md) - BR-WF-RECOVERY-011
- [remediationprocessor/controller-implementation.md](../services/crd-controllers/01-signalprocessing/controller-implementation.md) - RP enrichment logic
- [remediationorchestrator/controller-implementation.md](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md) - Recovery initiation
- [aianalysis/controller-implementation.md](../services/crd-controllers/02-aianalysis/controller-implementation.md) - EnrichmentData consumption

**Data Flow** (REVISED per DD-CONTEXT-006):
1. **Remediation Orchestrator** extracts failure data from WorkflowExecution CRD
2. **Remediation Orchestrator** creates new SignalProcessing CRD with embedded failure data
3. **RemediationProcessing Controller** enriches with:
   - Monitoring context (FRESH from monitoring service)
   - Business context (FRESH from business context service)
   - ~~Recovery context (FRESH from Context API)~~ **REMOVED** - See DD-CONTEXT-006
4. **Remediation Orchestrator** watches RemediationProcessing completion
5. **Remediation Orchestrator** creates AIAnalysis CRD, copying enrichment data to spec
6. **AIAnalysis Controller** reads `spec.enrichmentData` (NO API calls)
7. **HolmesGPT API** receives current failure context
8. **LLM** queries Context API for playbooks via tool call (semantic search) if needed

**Graceful Degradation** (REVISED per DD-CONTEXT-006):
```go
// In RemediationProcessing Controller
// NO Context API call for recovery context
// Failure data already embedded in spec by Remediation Orchestrator

if rp.Spec.IsRecoveryAttempt {
    log.Info("Recovery attempt detected - using embedded failure data",
        "attemptNumber", rp.Spec.RecoveryAttemptNumber,
        "failedStep", rp.Spec.FailureData.FailedStep,
        "errorType", rp.Spec.FailureData.ErrorType)

    // Failure data already in spec - no external call needed
    rp.Status.EnrichmentResults.FailureContext = rp.Spec.FailureData
}
```

### Consequences

**Positive** (REVISED per DD-CONTEXT-006):
- ✅ AI receives optimal context for recovery decisions (fresh monitoring + business + current failure data)
- ✅ Complete audit trail for compliance and debugging
- ✅ Simplified testing (AIAnalysis has no external dependencies)
- ✅ Consistent with established architectural patterns
- ✅ No circumstantial historical data (LLM reasons about current situation)
- ✅ LLM can query Context API for playbooks via tool call if needed

**Negative** (REVISED per DD-CONTEXT-006):
- ⚠️ Recovery initiation takes ~1 minute longer than Alternative 3
  - **Accepted Trade-off**: Better AI decisions worth the time penalty
- ~~⚠️ RemediationProcessing Controller has additional responsibility (Context API client)~~ **REMOVED** - No longer calls Context API

**Neutral** (REVISED per DD-CONTEXT-006):
- ~~🔄 Must maintain Context API client in RemediationProcessing package~~ **REMOVED** - No longer needed
- 🔄 RemediationOrchestrator copies enrichment data (simple data copy, low risk)
- 🔄 RemediationOrchestrator extracts failure data from WorkflowExecution CRD

### Validation Results

**Confidence Assessment Progression**:
- Initial Option B assessment: 90% confidence
- After Alternative 2 analysis: 92% confidence
- After implementation review: 95% confidence

**Key Validation Points**:
- ✅ Temporal consistency verified through reconciliation phase design
- ✅ Fresh context delivery confirmed through dual enrichment pattern
- ✅ Graceful degradation tested with Context API unavailability scenarios
- ✅ Audit trail completeness validated through CRD schema review
- ✅ Self-contained CRD pattern maintained across all controllers

### Related Decisions
- **Supersedes**: Option A (AIAnalysis queries Context API)
- **Supersedes**: Option B (RR embeds static context)
- **Builds On**: Self-contained CRD pattern (established in core architecture)
- **Supports**: BR-WF-RECOVERY-001 to BR-WF-RECOVERY-011 (recovery requirements)

### Review & Evolution

**When to Revisit**:
- If Context API latency exceeds 2 seconds consistently
- If recovery time requirements become more stringent (<30 seconds)
- If V2 introduces multi-provider AI requiring different context patterns
- If audit requirements change to require real-time context streaming

**Success Metrics**:
- Recovery success rate: Target >80% for first recovery attempt
- Context freshness: All contexts <1 minute old at AI analysis time
- Temporal consistency: All contexts within 10-second window
- Audit completeness: 100% of attempts have complete enrichment data

---
