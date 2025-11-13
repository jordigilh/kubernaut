# DD-CONTEXT-005: Rejected Field Analysis

**Parent Decision**: [DD-CONTEXT-005: Minimal LLM Response Schema](../DD-CONTEXT-005-minimal-llm-response-schema.md)
**Date**: November 11, 2025
**Status**: Supporting Analysis - All Fields REJECTED
**Purpose**: Document detailed analysis of proposed response fields and why each was rejected

---

## 🎯 Purpose

This document captures the detailed analysis of proposed additional fields for the Context API LLM response schema. **All fields analyzed here were REJECTED** in favor of the minimal 4-field schema.

**Final Decision**: Keep minimal schema (playbook_id, version, description, confidence) and perform all filtering via query parameters.

---

## 📋 Rejected Fields Summary

| Field | Initial Confidence | Final Status | Rejection Reason |
|-------|-------------------|--------------|------------------|
| `incident_types` | 93% | ❌ REJECTED | Redundant with query filtering |
| `risk_level` | 90% | ❌ REJECTED | Should be query parameter (signal categorization) |
| `actions` | 91% → 65% | ❌ REJECTED | Playbooks are containers, actions not visible |
| `estimated_duration` | 80% | ❌ REJECTED | Not useful without prompt context, creates speed bias |
| `prerequisites` | 85% | ❌ REJECTED | Already filtered by label matching |
| `rollback_available` | 75% | ❌ REJECTED | Part of risk categorization |

---

## 🔍 Detailed Field Analysis

### Field 1: `incident_types` (REJECTED)

**Initial Proposal** (93% Confidence):
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "incident_types": ["pod-oom-killer", "container-memory-pressure"],
  "confidence": 0.92
}
```

**Proposed Benefits**:
- ✅ Explicit incident type matching (no inference needed)
- ✅ Handles multi-incident-type playbooks clearly
- ✅ Reduces LLM hallucination risk (explicit vs inferred)

**Why REJECTED**:

**Fatal Flaw**: Redundant with query filtering
- Context API already filters by `incident_type` query parameter
- Semantic search ensures only matching playbooks are returned
- If a playbook is in the response, it **already matches** the incident
- LLM doesn't need to verify incident type - the query already did that

**The Logic Error**:
```
❌ WRONG: LLM queries → gets all playbooks → filters by incident_types
✅ RIGHT: LLM queries with incident → Context API filters → returns only matching playbooks
```

**Token Cost**: +15 tokens/playbook for redundant information

**Verdict**: ❌ **REJECTED** - Fails redundancy test

---

### Field 2: `risk_level` (REJECTED)

**Initial Proposal** (90% Confidence):
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "risk_level": "low",
  "confidence": 0.92
}
```

**Risk Levels**:
- `low`: Non-destructive, reversible (scale, restart)
- `medium`: Potentially disruptive (drain node, reschedule)
- `high`: Destructive or irreversible (delete, force-terminate)

**Proposed Benefits**:
- ✅ LLM can avoid high-risk actions for critical environments
- ✅ Enables risk-aware reasoning
- ✅ Reduces accidental destructive actions

**Why REJECTED**:

**Fatal Flaw #1**: Prompt dependency
- Risk level is only useful if HolmesGPT API prompt explains it
- Without prompt instructions, LLM sees `"risk_level": "high"` with no context
- Adds token cost for information LLM may not understand

**Fatal Flaw #2**: Should be deterministic filtering, not LLM reasoning
- Risk tolerance is determined by Gateway (environment categorization)
- Production → low-risk only (deterministic rule)
- LLM shouldn't decide risk trade-offs (safety-critical)

**Fatal Flaw #3**: Inconsistent with architecture
- Environment, priority, business_category are all query parameters
- Risk is another signal category, should follow same pattern
- Exposing risk but not other categories is inconsistent

**Correct Approach**:
```
Gateway Rego Policy:
- namespace="production" → risk_tolerance=low

Context API Query:
- labels=kubernaut.io/risk-tolerance:low

Playbook Labels:
- kubernaut.io/max-risk-level: low

Result: Only low-risk playbooks returned (LLM never sees high-risk options)
```

**Token Cost**: +5 tokens/playbook

**Verdict**: ❌ **REJECTED** - Should be query parameter (signal categorization), not response field

---

### Field 3: `actions` (REJECTED)

**Initial Proposal** (91% Confidence → 65% after analysis):
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "actions": ["scale_memory", "restart_pod"],
  "confidence": 0.92
}
```

**Proposed Benefits**:
- ✅ LLM knows exactly what playbook does
- ✅ Clearer than free-form description
- ✅ Structured data for LLM reasoning

**Why REJECTED**:

**Fatal Flaw**: Playbooks are Tekton Tasks (container images)
- Playbook contents are NOT visible outside the container
- Cannot extract actions from container image
- Would require manual annotation during playbook registration
- High maintenance burden (manual field for every playbook)
- Risk of actions field becoming stale/incorrect

**From ADR-035** (Remediation Execution Engine):
> Playbooks are Tekton Task YAML definitions that reference container images. The actual remediation logic is encapsulated in the container.

**Alternative**: Use `description` field (already provided during registration)
- Playbook authors write human-readable descriptions
- LLM can understand natural language descriptions
- No maintenance burden
- Already part of playbook metadata

**Token Cost**: +20 tokens/playbook (if implemented)

**Confidence Downgrade**: 91% → 65% (not feasible without manual maintenance)

**Verdict**: ❌ **REJECTED** - Not technically feasible, high maintenance burden

---

### Field 4: `estimated_duration` (REJECTED)

**Initial Proposal** (80% Confidence):
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "estimated_duration": "45s",
  "confidence": 0.92
}
```

**Proposed Benefits**:
- ✅ LLM can prefer faster playbooks for urgent incidents
- ✅ Time-aware decision making

**Why REJECTED**:

**Fatal Flaw #1**: Prompt dependency without clear value
- Without prompt instructions: LLM sees "45s" but doesn't know what to do with it
- With prompt instructions: "Prefer faster playbooks for P0" creates speed bias
- LLM may prefer fast-but-incorrect over slow-but-correct

**Fatal Flaw #2**: Duration is highly variable
- Same playbook: 30s in dev, 2min in production
- Depends on cluster size, resource availability, network latency
- Estimated duration may mislead LLM

**Fatal Flaw #3**: Speed preference is context-dependent
- P0 production outage → Speed matters
- P3 dev environment → Thoroughness matters
- This is a **filtering decision**, not an LLM reasoning task

**Fatal Flaw #4**: Not critical for playbook selection
- LLM should pick **most confident** playbook
- Confidence already encodes quality/effectiveness
- Adding duration creates multi-objective optimization (confidence vs speed)

**If speed matters**: Should be encoded in confidence score or handled by query parameter, not exposed to LLM

**Token Cost**: +8 tokens/playbook

**Verdict**: ❌ **REJECTED** - Not useful without prompt context, creates speed bias

---

### Field 5: `prerequisites` (REJECTED)

**Initial Proposal** (85% Confidence):
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "prerequisites": ["kubernetes.io/os: linux", "kubernaut.io/environment: production"],
  "confidence": 0.92
}
```

**Proposed Benefits**:
- ✅ LLM knows if playbook can run in environment
- ✅ Prevents recommending incompatible playbooks

**Why REJECTED**:

**Fatal Flaw**: Redundant with label matching
- Prerequisites ARE playbook labels
- Already filtered by Context API before LLM sees playbooks
- If a playbook is in the response, prerequisites are already met
- Same issue as `incident_types` - redundant with filtering

**The Logic**:
```
Query: labels=kubernaut.io/environment:production
Playbook Labels: kubernaut.io/environment: production
Result: Playbook matches, returned in response

If playbook is in response → prerequisites already met
```

**Fatal Flaw #2**: LLM shouldn't reason about prerequisites
- Prerequisites are deterministic (either met or not)
- Should be filtered before LLM sees playbooks
- LLM's job is to pick highest confidence, not validate prerequisites

**Token Cost**: +25 tokens/playbook (for typical prerequisite list)

**Verdict**: ❌ **REJECTED** - Redundant with label matching, fails redundancy test

---

### Field 6: `rollback_available` (REJECTED)

**Initial Proposal** (75% Confidence):
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "rollback_available": true,
  "confidence": 0.92
}
```

**Proposed Benefits**:
- ✅ LLM can prefer reversible actions
- ✅ Safety signal for critical environments

**Why REJECTED**:

**Fatal Flaw #1**: Reversibility is a risk dimension
- Reversible actions = low risk
- Irreversible actions = high risk
- This is part of `risk_level` categorization, not a separate field

**Fatal Flaw #2**: Should be implicit in playbook design
- All playbooks SHOULD be designed with rollback capability
- If a playbook can't be rolled back, it's high-risk
- This is a playbook design principle, not LLM decision factor

**Fatal Flaw #3**: LLM shouldn't decide based on rollback
- If rollback matters, filter by risk level
- Don't expose rollback as separate reasoning factor
- Adds complexity without clear benefit

**Correct Approach**: Encode in risk categorization
```
Playbook Labels:
- kubernaut.io/max-risk-level: low (implies reversible)
- kubernaut.io/max-risk-level: high (implies irreversible)

Query: labels=kubernaut.io/risk-tolerance:low
Result: Only reversible playbooks returned
```

**Token Cost**: +5 tokens/playbook

**Verdict**: ❌ **REJECTED** - Part of risk categorization, not separate field

---

## 🧪 The Four Tests

Every proposed field was evaluated against these four tests:

### Test 1: Redundancy Test
**Question**: "If this field is in the response, what does it tell the LLM that the query didn't already determine?"

**Results**:
- ❌ `incident_types`: Nothing (already filtered by query)
- ❌ `prerequisites`: Nothing (already filtered by label matching)
- ✅ `confidence`: Something (LLM needs this to pick best playbook)

### Test 2: Prompt Dependency Test
**Question**: "Does this field require prompt instructions to be useful?"

**Results**:
- ❌ `risk_level`: YES (needs "prefer low-risk for production")
- ❌ `estimated_duration`: YES (needs "prefer fast for P0")
- ✅ `description`: NO (LLM understands natural language)

### Test 3: Safety Test
**Question**: "Should the LLM be allowed to make trade-off decisions about this field?"

**Results**:
- ❌ `risk_level`: NO (risk decisions should be deterministic)
- ❌ `rollback_available`: NO (safety decisions should be deterministic)
- ✅ `confidence`: YES (LLM should pick highest confidence)

### Test 4: Feasibility Test
**Question**: "Can this field be reliably populated without manual maintenance?"

**Results**:
- ❌ `actions`: NO (playbooks are containers, contents not visible)
- ✅ `confidence`: YES (calculated from semantic + label matching)

---

## 📊 Token Cost Analysis

| Schema | Fields | Tokens/Playbook | 10 Playbooks | Monthly Cost (1K queries/day) |
|--------|--------|-----------------|--------------|-------------------------------|
| **Minimal** (approved) | 4 | 60 | 600 | $18/month |
| **With incident_types** | 5 | 75 | 750 | $22.50/month |
| **With risk_level** | 5 | 65 | 650 | $19.50/month |
| **With actions** | 5 | 80 | 800 | $24/month |
| **Full (all rejected fields)** | 10 | 120 | 1200 | $36/month |

**Savings**: Minimal schema saves $18/month (50%) compared to full schema with all rejected fields.

---

## 💡 Key Insights from Analysis

### Insight 1: Filtering vs Reasoning Pattern

**Discovery**: Most proposed fields were actually **filtering criteria**, not **reasoning factors**.

**The Pattern**:
- **Filtering** (deterministic, pre-LLM): incident_type, environment, risk, prerequisites
- **Reasoning** (probabilistic, LLM task): Pick highest confidence from pre-filtered list

**Rule**: If it's deterministic, filter it. If it's probabilistic, let LLM reason about it.

---

### Insight 2: Signal Categorization Architecture

**Discovery**: Risk, environment, priority, business_category are all **signal categories** from Gateway/Signal Processing.

**The Pattern**:
```
Gateway/Signal Processing → Categorizes signal
Context API → Filters by categories (query parameters)
LLM → Reasons about pre-filtered results
```

**Rule**: Signal categories are query parameters, not response fields.

---

### Insight 3: Prompt Dependency Anti-Pattern

**Discovery**: Fields that require prompt instructions are usually filtering criteria in disguise.

**Examples**:
- `risk_level`: Needs prompt to say "prefer low-risk for production" → Should be query parameter
- `estimated_duration`: Needs prompt to say "prefer fast for P0" → Should be query parameter

**Rule**: If a field needs prompt instructions to be useful, it should be filtered via query, not in response.

---

### Insight 4: Container Encapsulation Reality

**Discovery**: Playbooks are Tekton Tasks (container images). Contents are not visible outside the container.

**Implication**: Cannot extract structured data (actions, steps) from playbooks without manual annotation.

**Rule**: Only use metadata that's provided during playbook registration (description, labels, version).

---

## 🎯 Final Recommendation

**Keep the minimal 4-field schema**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "confidence": 0.92
}
```

**Confidence**: **95%** that this is optimal

**Rationale**:
1. All filtering happens via query parameters (deterministic)
2. LLM task is simple: pick highest confidence (probabilistic)
3. Token efficient: 60 tokens/playbook
4. Safe: Critical decisions made by system, not LLM
5. Consistent: All filtering uses same pattern (label matching)

---

## 🔗 Related Documents

- **Parent Decision**: [DD-CONTEXT-005: Minimal LLM Response Schema](../DD-CONTEXT-005-minimal-llm-response-schema.md)
- **Implementation Spec**: [PLAYBOOK_QUERY_FOR_HOLMESGPT.md](../../../services/stateless/context-api/PLAYBOOK_QUERY_FOR_HOLMESGPT.md)
- **Playbook Catalog**: [ADR-033: Remediation Playbook Catalog](../ADR-033-remediation-playbook-catalog.md)
- **Execution Engine**: [ADR-035: Remediation Execution Engine](../ADR-035-remediation-execution-engine.md)

---

**Document Version**: 1.0
**Last Updated**: November 11, 2025
**Status**: ✅ **COMPLETE** - All fields analyzed and rejected

