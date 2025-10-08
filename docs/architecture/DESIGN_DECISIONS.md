# Architectural Design Decisions

**Purpose**: This document tracks major architectural decisions made during kubernaut development, providing context for "why" certain approaches were chosen over alternatives.

**Format**: Each decision follows the Architecture Decision Record (ADR) pattern, documenting context, alternatives considered, decision rationale, and consequences.

---

## Quick Reference

| ID | Decision | Status | Date | Key Files |
|---|---|---|---|---|
| DD-001 | Recovery Context Enrichment (Alternative 2) | ✅ Approved | 2024-10-08 | [PROPOSED_FAILURE_RECOVERY_SEQUENCE.md](PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) |

---

## DD-001: Recovery Context Enrichment (Alternative 2)

### Status
**✅ Approved Design** (2024-10-08)
**Last Reviewed**: 2024-10-08
**Confidence**: 95% (based on temporal consistency requirements)

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
- ✅ **Immutable audit trail**: Each RemediationProcessing CRD contains complete snapshot
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
3. **Complete Audit Trail**: Each RemediationProcessing CRD is immutable snapshot of attempt
4. **Architectural Consistency**: Follows established self-contained CRD pattern
5. **Simplified AIAnalysis**: No external dependencies = easier testing and maintenance

**Key Insight**: The ~1 minute "penalty" for dual enrichment (RP enriches → RR creates AIAnalysis) is actually a **feature**, not a bug - it ensures fresh monitoring/business contexts for better AI recommendations.

### Implementation

**Primary Implementation Files**:
- [PROPOSED_FAILURE_RECOVERY_SEQUENCE.md](PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) - Authoritative sequence diagram
- [04_WORKFLOW_ENGINE_ORCHESTRATION.md](../requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md) - BR-WF-RECOVERY-011
- [remediationprocessor/controller-implementation.md](../services/crd-controllers/01-remediationprocessor/controller-implementation.md) - RP enrichment logic
- [remediationorchestrator/controller-implementation.md](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md) - Recovery initiation
- [aianalysis/controller-implementation.md](../services/crd-controllers/02-aianalysis/controller-implementation.md) - EnrichmentData consumption

**Data Flow**:
1. **Remediation Orchestrator** creates new RemediationProcessing CRD (recovery=true)
2. **RemediationProcessing Controller** enriches with ALL contexts:
   - Monitoring context (FRESH from monitoring service)
   - Business context (FRESH from business context service)
   - Recovery context (FRESH from Context API)
3. **Remediation Orchestrator** watches RemediationProcessing completion
4. **Remediation Orchestrator** creates AIAnalysis CRD, copying enrichment data to spec
5. **AIAnalysis Controller** reads `spec.enrichmentData` (NO API calls)
6. **HolmesGPT** receives complete context for optimal recovery recommendations

**Graceful Degradation**:
```go
// In RemediationProcessing Controller
recoveryCtx, err := r.ContextAPIClient.GetRemediationContext(ctx, remediationRequestID)
if err != nil {
    // Fallback: Build context from failed workflow reference
    recoveryCtx = r.buildFallbackRecoveryContext(rp)
}
rp.Status.EnrichmentResults.RecoveryContext = recoveryCtx
```

### Consequences

**Positive**:
- ✅ AI receives optimal context for recovery decisions (fresh + historical)
- ✅ Complete audit trail for compliance and debugging
- ✅ Simplified testing (AIAnalysis has no external dependencies)
- ✅ Consistent with established architectural patterns
- ✅ Early failure detection (Context API issues caught during enrichment)

**Negative**:
- ⚠️ Recovery initiation takes ~1 minute longer than Alternative 3
  - **Accepted Trade-off**: Better AI decisions worth the time penalty
- ⚠️ RemediationProcessing Controller has additional responsibility (Context API client)
  - **Mitigation**: Well-encapsulated with graceful degradation

**Neutral**:
- 🔄 Must maintain Context API client in RemediationProcessing package
- 🔄 RemediationOrchestrator copies enrichment data (simple data copy, low risk)

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

## Future Design Decisions

As new architectural decisions are made, add them here following the DD-XXX format:

### Template

```markdown
## DD-XXX: [Decision Title]

### Status
**[Status Emoji] [Status]** (YYYY-MM-DD)
**Last Reviewed**: YYYY-MM-DD
**Confidence**: XX%

### Context & Problem
[What problem are we solving? Why does it matter?]

### Alternatives Considered
[List 2-3 alternatives with pros/cons]

### Decision
**APPROVED: [Alternative Name]**

[Rationale for decision]

### Implementation
[Key files and data flows]

### Consequences
**Positive**: [Benefits]
**Negative**: [Trade-offs]
**Neutral**: [Other impacts]

### Related Decisions
[Links to other DDs]

### Review & Evolution
[When to revisit this decision]
```

---

## References

- [Architecture Decision Records (ADR)](https://adr.github.io/) - Best practices for documenting architectural decisions
- [kubernaut APDC Methodology](../../.cursor/rules/00-core-development-methodology.mdc) - Analysis → Plan → Do → Check framework
- [Conflict Resolution Matrix](../../.cursor/rules/13-conflict-resolution-matrix.mdc) - Priority hierarchy for rule conflicts

