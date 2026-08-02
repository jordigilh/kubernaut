# DD-EFFECTIVENESS-001: Hybrid Automated + AI Analysis Approach for Post-Execution Effectiveness

**Date**: October 16, 2025
**Status**: ✅ APPROVED (directional decision) — the two-level split (Level 1 automated / Level 2 AI-powered) remains the current approved direction; see [DD-017](DD-017-effectiveness-monitor-v1.1-deferral.md) v2.6 for current phasing authority. **Level 1's specific Go architecture described below was NOT how V1.0 was actually built** — see [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md) for the real, implemented V1.0 architecture (CRD-watch controller, 4 deterministic scorers, audit-event emission to DataStorage). **Level 2 (AI-powered analysis) has never been built** — everything below describing it (code sketch, OOM example, cost figures) is a design **proposal** retained as input for future V1.1 planning, not a spec of current or historical behavior.
**Confidence**: 95% (original assessment of the two-level *split* decision; not a confidence claim about the specific architecture sketched below, which was superseded)

**Note**: Per [DD-017](DD-017-effectiveness-monitor-v1.1-deferral.md) v2.6 (current, authoritative), Level 1 (Automated Assessment) is in V1.0 scope and has shipped — via the CRD-controller architecture in [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md), not the service sketch below. Level 2 (AI-powered analysis via a **(planned)** Kubernaut Agent (KA) PostExec endpoint) remains an unbuilt V1.1 proposal.
**Decision Makers**: Architecture Team, Product Owner
**Replaces**: N/A (New decision)

---

## 📋 Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 2.0 | 2026-08-02 | Architecture Team (#1806 correction) | **Correction pass**: Updated the status banner to disambiguate the still-valid two-level *split* decision from the specific Level 1 Go architecture sketched here (superseded by the real CRD-controller implementation, ADR-EM-001) and the never-built Level 2 AI analysis. Removed the fictional Go/Python code blocks (replaced with pointers to ADR-EM-001 and the EM overview doc). Removed the "Context API" data-flow references (that service was deprecated one month after this document was written — see [DD-CONTEXT-006](DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md), 2025-11-13). Relabeled the OOM example as an explicitly hypothetical illustration. Flagged `BR-INS-*` and `BR-HAPI-POSTEXEC-*` as never-formalized, code-unreferenced IDs (not renumbered). Renamed "HolmesGPT API" to "(planned) Kubernaut Agent (KA) PostExec endpoint" throughout. Added a blanket caveat on the dollar-cost figures and Week 1-6 timeline as illustrative Oct-2025 estimates, not current budget. Preserved the decision-trigger taxonomy and alternatives-considered analysis as genuine forward-looking V1.1 design input. | [#1806](https://github.com/jordigilh/kubernaut/issues/1806) |
| 1.0 | 2025-10-16 | Architecture Team | Original decision: hybrid two-level architecture (Level 1 automated, always-on; Level 2 AI-powered, selective). Included a Go/Python code sketch for Level 1/Level 2, a Context API-based lessons-learned sink, and Week 1-6 implementation phasing. Never implemented as described — see v2.0 correction above. | — |

---

## ⚠️ Reading This Document (2026-08-02 Correction Notice)

This document was written **before** the Effectiveness Monitor (EM) existed in any implemented form. Treat it accordingly:

- **What is still current**: the *directional* decision — split effectiveness assessment into an always-on deterministic Level 1 and a selectively-triggered AI-powered Level 2 — per [DD-017](DD-017-effectiveness-monitor-v1.1-deferral.md) v2.6. The decision-trigger taxonomy, the alternatives-considered cost/benefit reasoning, and the illustrative OOM example below retain genuine value as **proposed design input for future V1.1 planning**.
- **What is superseded**: the specific Level 1 Go architecture (`EffectivenessMonitor.AssessExecution()`, `shouldCallAI()`, `stateComparator`, `holmesgptClient` — confirmed via repo-wide grep to not exist anywhere in the codebase). The real, implemented V1.0 Level 1 architecture is a Kubernetes CRD controller documented in [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md) and [the EM overview](../../services/crd-controllers/07-effectivenessmonitor/overview.md).
- **What was never built**: Level 2 AI-powered analysis in its entirety. There is no HolmesGPT/Kubernaut Agent (KA) client anywhere in `pkg/effectivenessmonitor/` or `internal/controller/effectivenessmonitor/` (confirmed via grep). The `POST /api/v1/postexec/analyze` endpoint referenced below does not exist in KA's `openapi.json` or anywhere else in the repo — it is tracked as a genuinely planned V1.1 feature in DD-017.
- **What is stale and removed**: the "Context API" learning sink (deprecated one month after this document was written, per [DD-CONTEXT-006](DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md), 2025-11-13) and the specific dollar-cost figures / Week 1-6 timeline (illustrative Oct-2025 estimates never executed as such — treat all `$` figures and week-numbered phasing below as historical illustration, not a current budget or plan).

---

## 🎯 **CONTEXT**

The Effectiveness Monitor service needs to assess whether remediation actions achieved their intended goals. The question arose: **Can automated checks (e.g., "pod not dying for OOM") suffice, or do we need AI/LLM analysis?**

### **Business Requirements Affected**
- BR-INS-001 to BR-INS-010: Effectiveness assessment, pattern analysis, oscillation detection
- BR-WF-INVESTIGATION-004: Analyze execution results for pattern learning
- BR-HAPI-POSTEXEC-001 to 005: Post-execution analysis endpoint

> ⚠️ **BR ID NOTE (2026-08-02, #1806)**: `BR-INS-001` through `BR-INS-010` and `BR-HAPI-POSTEXEC-001` through `BR-HAPI-POSTEXEC-005` were never formalized as requirement documents under `docs/requirements/` and have **zero code references** anywhere in the repository (verified via repo-wide grep). Per the "do not rename or renumber IDs" convention, they are preserved here unrenumbered and flagged for a future requirements-mapping pass. The real, current, code-backed Level 1 requirements are **`BR-EM-001` through `BR-EM-012`** — see the [Effectiveness Monitor Overview](../../services/crd-controllers/07-effectivenessmonitor/overview.md) or [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md).

### **Target Metrics**
- Remediation effectiveness: 70% (current) → 85-90% (target)
- Cascade failure rate: 30% (current) → <10% (target)
- MTTR (failed remediation): 15 min (current) → 8 min (target)

> These were the original Oct-2025 proposal's target metrics. They are illustrative estimates, not validated against current production data — Level 2 (the mechanism that would move these numbers) has never been built.

---

## 📋 **DECISION**

**Approved Approach**: **Hybrid Architecture** (Automated Checks + Selective AI Analysis)

### **Two-Level Assessment**

#### **Level 1: Automated Assessment** — original proposal (superseded by the real V1.0 implementation)
- **Always executed**: Every workflow completion
- **Purpose**: Technical validation, immediate feedback
- **Scope** (as originally proposed):
  - Pod/resource health checks
  - Metric comparisons (latency, error rates, resource usage)
  - Basic effectiveness scoring (formula-based)
  - Anomaly detection (metric changes > thresholds)
- **Cost**: Negligible (computational only)

> **What actually shipped**: the "always executed" principle held, but the mechanism did not. The real V1.0 Level 1 is a controller-runtime watch on the `EffectivenessAssessment` CRD (created by the Remediation Orchestrator), running 4 independent deterministic scorers (health, alert, metrics, hash) and emitting per-component audit events — no `EffectivenessMonitor.AssessExecution()` call, no basic-effectiveness "formula in Go" computed by EM itself (DataStorage computes the weighted score on demand). See [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md).

#### **Level 2: AI-Powered Analysis** — proposed, **not yet built** (planned V1.1)
- **Selectively executed**: High-value cases only
- **Purpose**: Pattern learning, root cause validation, causation analysis
- **Scope** (as originally proposed):
  - Root cause validation ("problem solved" vs "problem masked")
  - Oscillation detection ("fix A caused problem B")
  - Pattern learning (context-aware effectiveness)
  - Lesson extraction (for future investigations)
- **Cost**: ~$10K/year (illustrative Oct-2025 LLM API cost estimate — not a current budget figure; no Level 2 code exists to incur this cost)

---

## 🔄 **ALTERNATIVES CONSIDERED**

> The reasoning below (why not automated-only, why not full-AI) remains valid input for future V1.1 planning, since the underlying trade-off (cost/latency of AI vs. missed nuance from purely deterministic checks) hasn't changed. All dollar figures are illustrative Oct-2025 estimates, not current costs.

### **Alternative 1: Automated Checks Only** ❌ REJECTED
**Description**: Effectiveness Monitor performs all assessments using automated checks only

**Pros**:
- ✅ Zero LLM API costs
- ✅ Fast execution (no AI latency)
- ✅ Simple implementation

**Cons**:
- ❌ Cannot detect "problem masked" vs "problem solved"
  - Example: Memory increase masks leak, OOM will recur
- ❌ Cannot explain oscillations
  - Example: Fix OOM → CPU throttling (causation unclear)
- ❌ No pattern learning
  - Example: Cannot learn "gradual scaling works better for stateful apps"
- ❌ Miss target: 70% → ~75% effectiveness (insufficient)

**Verdict**: Insufficient for 85-90% effectiveness target

---

### **Alternative 2: AI Analysis for Everything** ❌ REJECTED
**Description**: Call a HolmesGPT-era-equivalent AI analysis API for every workflow completion

**Pros**:
- ✅ Maximum intelligence
- ✅ Complete pattern learning

**Cons**:
- ❌ High cost: ~$50/day for 10K executions = $18K/year (illustrative, wasteful)
- ❌ High latency: AI analysis adds 2-5s per workflow
- ❌ Most executions are routine (no learning value)

**Verdict**: Cost inefficient, adds latency unnecessarily

---

### **Alternative 3: Hybrid Approach** ✅ APPROVED (directional decision; still current per DD-017)
**Description**: Automated checks always + AI analysis selectively

**Pros**:
- ✅ Cost-effective: ~$10K/year illustrative estimate (55% savings vs Alternative 2)
- ✅ Fast feedback: Automated checks provide immediate results
- ✅ Intelligent learning: AI analyzes high-value cases
- ✅ Achieves target: 70% → 85-90% effectiveness (illustrative target)

**Cons**:
- ⚠️ More complex: Two-level architecture
- ⚠️ Decision logic: When to trigger AI analysis

**Verdict**: Best balance of cost, performance, and effectiveness. The *split* itself is what DD-017 v2.6 reaffirms as current; the specific architecture and numbers below are the original proposal, largely superseded (Level 1) or still unbuilt (Level 2).

---

## 📊 **COST/BENEFIT ANALYSIS** (Illustrative — Oct 2025 estimates, not a current budget)

> ⚠️ Level 2 has never been built, so none of these costs have been incurred. These figures are preserved as the original economic reasoning behind approving the hybrid split, not as current or projected Kubernaut spend.

### **Annual Costs**

| Execution Type | Volume/Year | Automated Only | + AI Analysis | Hybrid Approach |
|----------------|-------------|----------------|---------------|-----------------|
| Routine success | 3.65M | $0 | $18,250 | $0 |
| P0 failures | 3,650 | $0 | $1,825 | $1,825 ✅ |
| New action types | 2,600 | $0 | $1,300 | $1,300 ✅ |
| Suspected oscillations | 1,825 | $0 | $912 | $912 ✅ |
| Periodic batch | ~10,000 | $0 | $5,000 | $5,000 ✅ |
| **TOTAL** | **3.67M** | **$0** | **$27,287** | **$9,037** ✅ |

**Cost Savings**: 67% reduction vs full AI analysis (illustrative)

### **Business Value** (illustrative)

| Metric | Automated Only | Hybrid Approach | Delta |
|--------|---------------|------------------|-------|
| Remediation effectiveness | 75% | 85-90% | +10-15% |
| Cascade failure rate | 25% | <10% | -15% |
| MTTR (failed remediation) | 12 min | 8 min | -33% |
| Prevented failures/year | 27% | 90% | +63% |
| **Annual value** | $50K | $150K | +$100K |

**ROI**: 11x return on $9K investment (illustrative)

---

## 🔑 **DECISION TRIGGERS (When to Call AI Analysis)** — Proposed V1.1 design input, not yet built

> This taxonomy is the part of the original proposal with the most durable design value: it defines *when* a future Level 2 should fire, independent of the (superseded) Level 1 implementation details above. Preserved as-is for V1.1 planning per DD-017.

### **1. High-Priority Failures** (BR-INS-013)
**Trigger**: `priority == "P0" && execution_success == false`
**Rationale**: Learn from critical failures to prevent recurrence
**Volume**: ~10/day = 3,650/year (illustrative)
**Value**: Root cause validation, prevent repeated failures

### **2. First-Time Action Types** (BR-INS-004)
**Trigger**: Action type not in historical database
**Rationale**: Build pattern library for new actions
**Volume**: ~50/week = 2,600/year (illustrative)
**Value**: Establish effectiveness baselines

### **3. Suspected Oscillations** (BR-INS-005)
**Trigger**: Metric anomaly detected (CPU +20%, latency +50%, new alerts)
**Rationale**: Detect "fix A caused problem B" scenarios
**Volume**: ~5/day = 1,825/year (illustrative)
**Value**: Prevent remediation loops, critical for stability

### **4. Periodic Batch Analysis** (BR-INS-006-010)
**Trigger**: Daily/weekly batch processing of successes
**Rationale**: Pattern learning from aggregated data
**Volume**: ~30/day = 10,000/year (illustrative)
**Value**: Long-term trend analysis, predictive insights

### **5. Routine Successes** ❌ NO AI ANALYSIS
**Trigger**: Routine low-priority successes
**Rationale**: No learning value, wastes cost
**Volume**: ~10,000/day = 3.65M/year (illustrative)
**Action**: Automated checks only, store metrics for batch analysis

---

## 🏗️ **ARCHITECTURE**

> **The Go/Python code sketches from the original Oct-2025 proposal have been removed here** — `EffectivenessMonitor.AssessExecution()`, `shouldCallAI()`, `stateComparator`, `holmesgptClient`, and the `POST /api/v1/postexec/analyze` Python handler never existed in the codebase and are not a useful reference for either how V1.0 was built or how V1.1 should be built.
>
> **For the actual V1.0 Level-1 implementation**, see [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md) and the [Effectiveness Monitor Overview](../../services/crd-controllers/07-effectivenessmonitor/overview.md) — a Kubernetes CRD controller that watches `EffectivenessAssessment` CRDs, runs 4 deterministic scorers, and emits typed audit events to DataStorage (which computes the final weighted score on demand).
>
> **For Level 2**, no implementation exists to reference. If/when V1.1 planning begins, the decision-trigger taxonomy above should inform the trigger logic, but the storage/integration design should follow the real V1.0 audit-event pattern (extending the same DataStorage-bound audit events with AI-derived fields, per DD-017 §"V1.1 Integration with DD-KA-016") rather than the "Context API" sink described in the original proposal's data flow — that service was deprecated in November 2025, one month after this document was written (see [DD-CONTEXT-006](DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md)). There is no live "HolmesGPT API" to call; any future Level 2 call target is the **(planned) Kubernaut Agent (KA) PostExec endpoint** referenced in DD-017.

---

## 🎓 **ILLUSTRATIVE EXAMPLE (Hypothetical)** — OOM Remediation

> ⚠️ **This is a hypothetical illustration of what a future Level 2 AI analysis *could* reveal, not a real system output.** Level 2 has never been built, so no system has ever produced the JSON below. The field names and exact values are illustrative; they do not match the real V1.0 `EAComponents` schema (see [CRD Schema](../../services/crd-controllers/07-effectivenessmonitor/crd-schema.md)) or any implemented audit event payload (see the Audit Event Catalog).

### **Scenario**: Pod OOMKilled → Memory increased 512Mi → 2Gi

### **Step 1: Hypothetical Automated Assessment**
```json
{
  "technical_success": true,
  "health_checks": {
    "pod_running": true,
    "no_oom_errors_24h": true,
    "memory_utilization": 0.6,
    "restart_count": 0
  },
  "metrics_improved": {
    "latency": "same (no change)",
    "throughput": "same (1000 req/s)",
    "memory_usage": "increased (490Mi → 1.2Gi)"
  },
  "basic_effectiveness": 1.0,
  "anomalies": [
    "memory_usage higher than expected (1.2Gi vs 800Mi expected)"
  ]
}
```

**Decision**: Anomaly detected → Call AI analysis ✅ (hypothetical)

### **Step 2: Hypothetical Level 2 AI Analysis** (illustrative — no such response has ever been produced)
```json
{
  "execution_success": true,
  "effectiveness_score": 0.7,
  "root_cause_resolved": false,
  "lessons_learned": [
    {
      "category": "root_cause",
      "lesson": "Memory increase masked a memory leak. Expected stabilization at ~800Mi, but usage is 1.2Gi after 24h. Throughput unchanged confirms leak persists."
    }
  ],
  "side_effects": [
    "Increased cost without throughput improvement",
    "Memory leak has more headroom but will cause OOM again"
  ],
  "follow_up_actions": [
    {
      "action_type": "investigation",
      "description": "Profile application memory to identify leak",
      "priority": "high"
    }
  ]
}
```

**Key Insight** (the durable point of this illustration): a purely deterministic check can report "100% success" (pod healthy, no restarts) while masking a real problem (a memory leak) that only pattern-aware analysis would catch — this is the core argument for why Level 2 has forward-looking value, independent of the fictional numbers above.

---

## 📚 **IMPLEMENTATION GUIDANCE** (Historical — Oct 2025 proposal, not executed as described)

> ⚠️ This phased timeline was never executed as written. V1.0 Level 1 was actually delivered via the CRD-controller architecture in [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md) (see DD-017 v2.0–v2.6 for the real delivery timeline). Phase 2/3 (Level 2 AI integration, optimization) have not been executed at all — Level 2 remains an unbuilt V1.1 proposal per DD-017. Retained below only as historical record of the original plan.

### **Phase 1: Automated Foundation** (Week 1-2, as originally proposed)
1. Implement Effectiveness Monitor service
2. Automated health checks (BR-EXEC-027-030)
3. Metric collection and comparison
4. Basic effectiveness scoring
5. Anomaly detection logic

### **Phase 2: AI Integration** (Week 3-4, as originally proposed)
1. HolmesGPT-era post-execution endpoint (superseded concept — any future equivalent would be the planned Kubernaut Agent (KA) PostExec endpoint)
2. Decision logic (when to call AI)
3. Result combination and storage
4. Pattern learning infrastructure

### **Phase 3: Optimization** (Week 5-6, as originally proposed)
1. Tune decision thresholds
2. Optimize LLM prompts
3. Batch processing for cost efficiency
4. Performance monitoring

---

## ✅ **SUCCESS CRITERIA** (Illustrative — original Oct 2025 targets, not validated against current metrics)

### **Technical Success**
- Automated checks execute <100ms per workflow (illustrative target; real V1.0 latency is dominated by the configured stabilization window, default 5m, not computation time — see [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md))
- AI analysis completes <5s when triggered (illustrative — Level 2 not built)
- Combined approach processes 10K workflows/day (illustrative)
- Zero data loss in storage pipeline (directionally consistent with the real V1.0 audit-event pipeline design)

### **Business Success**
- Remediation effectiveness: 85-90% (illustrative target)
- Cascade failure rate: <10% (illustrative target)
- MTTR: 8 minutes (illustrative target)
- LLM API costs: <$12K/year (illustrative — not incurred, Level 2 not built)

### **Learning Success**
- Pattern library grows 10% week-over-week (first 3 months) (illustrative)
- Context-aware recommendations improve 5% month-over-month (illustrative)
- Oscillation detection prevents 100% of remediation loops (illustrative)

---

## 🔄 **RISKS & MITIGATIONS** (Proposed — relevant if/when Level 2 is built)

### **Risk 1: Decision Logic Complexity**
**Risk**: When to trigger AI analysis may be complex
**Mitigation**: Start with simple rules (P0 failures, new actions), iterate based on data
**Monitoring**: Track AI analysis trigger rate, cost per month

### **Risk 2: LLM API Costs Exceed Budget**
**Risk**: AI analysis volume higher than expected
**Mitigation**: Implement cost caps, monthly spending alerts, batch processing
**Fallback**: Reduce batch analysis frequency

### **Risk 3: AI Analysis Latency**
**Risk**: 5s AI latency delays workflow completion updates
**Mitigation**: Asynchronous AI analysis (don't block workflow status update)
**Implementation**: Store automated results immediately, update with AI results when ready

### **Risk 4: False Positive Anomalies**
**Risk**: Automated anomaly detection triggers unnecessary AI calls
**Mitigation**: Tune anomaly thresholds based on historical data
**Monitoring**: Track false positive rate, adjust thresholds monthly

---

## 📈 **MONITORING & METRICS** (Proposed metric names — not implemented; real V1.0 metrics differ)

> None of the metric names below exist in the current implementation. Real V1.0 Prometheus metrics are documented in [Observability & Logging](../../services/crd-controllers/07-effectivenessmonitor/observability-logging.md). These are preserved only as illustrative naming for a future, unbuilt Level 2.

### **Effectiveness Metrics** (proposed)
- `effectiveness_automated_score` (gauge): Automated assessment score
- `effectiveness_ai_adjusted_score` (gauge): AI-adjusted score (when available)
- `effectiveness_assessment_duration_seconds` (histogram): Assessment latency
- `effectiveness_ai_analysis_triggered_total` (counter): AI calls by reason

### **Cost Metrics** (proposed)
- `effectiveness_llm_api_calls_total` (counter): LLM API calls
- `effectiveness_llm_api_cost_dollars` (counter): Cumulative LLM costs
- `effectiveness_cost_per_workflow_cents` (gauge): Average cost per workflow

### **Learning Metrics** (proposed)
- `effectiveness_patterns_learned_total` (counter): New patterns discovered
- `effectiveness_oscillations_detected_total` (counter): Oscillations prevented
- `effectiveness_improvement_percentage` (gauge): Week-over-week effectiveness trend

---

## 🎯 **APPROVAL RATIONALE**

**Why Approved** (the two-level *split*, as reaffirmed by DD-017 v2.6):
1. ✅ Achieves 85-90% effectiveness target (vs 75% automated-only) — illustrative target
2. ✅ Cost-effective: $9K/year illustrative estimate with $100K+ illustrative value
3. ✅ Prevents remediation loops (BR-INS-005)
4. ✅ Enables pattern learning (BR-INS-003-004)
5. ✅ Fast feedback for routine cases (automated checks) — validated by the real V1.0 implementation
6. ✅ Intelligent analysis for high-value cases (AI) — still a V1.1 proposal, not yet built

**Decision Confidence**: 95% (in the directional split; the specific architecture and cost figures below it were an early, since-superseded/unbuilt proposal)

**Remaining 5% Risk**: Decision logic tuning may require iteration (once Level 2 is actually built)

---

**Status**: ✅ APPROVED (directional decision only) — Level 1 architecture **superseded** by the real V1.0 implementation (see [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md)); Level 2 remains an **unbuilt V1.1 design proposal** (see [DD-017](DD-017-effectiveness-monitor-v1.1-deferral.md) v2.6 for current phasing authority).
**Next Steps**: None from this document directly — V1.0 Level 1 has already shipped differently than described here; any Level 2 work should start from DD-017's V1.1 scope, using this document's decision-trigger taxonomy as design input.
