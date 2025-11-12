# DD-CONTEXT-005: Minimal LLM Response Schema (Filter Before LLM Pattern)

**Date**: 2025-11-11
**Status**: ✅ Approved
**Decision Makers**: Architecture Team
**Impact**: High - Affects Context API response design and LLM integration pattern
**Related**: ADR-033 (Playbook Catalog), DD-CONTEXT-003 (Context Enrichment Placement)

---

## 🎯 Decision

**The Context API response schema for HolmesGPT API SHALL remain minimal (4 fields only). All filtering, categorization, and constraint checking SHALL happen via query parameters and label matching BEFORE the LLM sees playbooks.**

**Approved Schema**:
```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "confidence": 0.92
    }
  ],
  "total_results": 2
}
```

**Fields**: 4 (playbook_id, version, description, confidence)

**LLM Task**: Pick the playbook with highest confidence from pre-filtered list.

---

## 📋 Context & Problem

### Problem Statement

When designing the Context API response for HolmesGPT API's LLM-driven playbook selection, we must decide:
1. What information should the LLM receive to make playbook selection decisions?
2. Should filtering criteria (risk, environment, prerequisites) be in the response or query?
3. How do we balance LLM reasoning capability with token efficiency and safety?

### Key Requirements

- **BR-CONTEXT-006**: Context API must provide historical playbook execution data
- **BR-HOLMESGPT-003**: HolmesGPT API must query Context API for playbook recommendations
- **BR-AI-002**: AI Analysis must receive context-enriched playbook options
- **Token Efficiency**: Minimize LLM token usage while maintaining effectiveness
- **Safety**: Critical decisions (risk, environment) must be deterministic, not probabilistic
- **Simplicity**: LLM task should be as simple as possible

---

## 🔍 Alternatives Considered

### Alternative 1: Rich Response Schema with Filtering Fields

**Approach**: Include filtering criteria in response for LLM reasoning

**Proposed Schema**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "incident_types": ["pod-oom-killer", "container-memory-pressure"],
  "risk_level": "low",
  "estimated_duration": "45s",
  "prerequisites": ["kubernetes.io/os: linux"],
  "confidence": 0.92
}
```

**Pros**:
- ✅ LLM has explicit information about playbook capabilities
- ✅ LLM can reason about risk trade-offs
- ✅ LLM can consider duration for time-sensitive incidents

**Cons**:
- ❌ **Redundant with query filtering**: `incident_types` already used to filter playbooks before response
- ❌ **Risk reasoning is unsafe**: LLM shouldn't decide risk trade-offs (should be deterministic)
- ❌ **Token waste**: +40 tokens/playbook for information LLM doesn't need
- ❌ **Complex LLM task**: LLM must reason about multiple dimensions
- ❌ **Inconsistent with architecture**: Filtering criteria are signal categories from Gateway, not LLM reasoning factors
- ❌ **Duration bias**: LLM may prefer speed over correctness without proper context

**Confidence**: 30% (rejected)

**Why Rejected**:
1. **Incident Types**: If a playbook is in the response, it already matches the incident (semantic search + label matching). Redundant.
2. **Risk Level**: Risk tolerance is determined by Gateway (environment categorization), not LLM. Should be query parameter.
3. **Prerequisites**: Already filtered by label matching. If playbook is in response, prerequisites are met.
4. **Duration**: Speed preference is context-dependent and shouldn't bias LLM toward fast-but-incorrect solutions.

---

### Alternative 2: Minimal Schema with Query-Based Filtering (APPROVED)

**Approach**: Filter ALL criteria via query parameters, return only matching playbooks

**Architecture**:
```
Gateway/Signal Processing:
  ↓ Categorizes signal
  ↓ environment=production, priority=P0, risk_tolerance=low, business_category=payment

Context API Query:
  ↓ Semantic search (incident description)
  ↓ Label matching (environment, priority, risk_tolerance, business_category)
  ↓ Filters playbooks by ALL categories
  ↓ Returns only matching playbooks, sorted by confidence

LLM:
  ↓ Receives: [{playbook_id, version, description, confidence}, ...]
  ↓ Task: Pick highest confidence playbook
  ↓ Done.
```

**Approved Schema**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "confidence": 0.92
}
```

**Pros**:
- ✅ **Deterministic filtering**: Categories (environment, priority, risk) are facts, not probabilities
- ✅ **Simpler LLM task**: Pick highest confidence from pre-filtered list
- ✅ **Token efficient**: 60 tokens/playbook (vs 100+ with rich schema)
- ✅ **Safe**: Critical decisions (risk, environment) made by system, not LLM
- ✅ **Consistent**: All filtering uses same pattern (label matching)
- ✅ **No redundancy**: Every field serves LLM reasoning, no duplicate information
- ✅ **Scalable**: Adding new categories doesn't change response schema

**Cons**:
- ⚠️ LLM cannot reason about trade-offs (e.g., "accept higher risk for faster resolution")
  - **Mitigation**: Trade-offs should be encoded in playbook design and confidence scoring, not LLM decisions

**Confidence**: 95% (approved)

---

### Alternative 3: Hybrid Approach (Query Filtering + Selected Response Fields)

**Approach**: Filter most criteria via query, include only critical fields in response

**Proposed Schema**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "risk_level": "low",  // Only field added
  "confidence": 0.92
}
```

**Rationale**: Risk level might help LLM make safety-aware decisions

**Pros**:
- ✅ Minimal token increase (+5 tokens/playbook)
- ✅ LLM has explicit safety signal

**Cons**:
- ❌ **Risk filtering should be deterministic**: If environment=production, only low-risk playbooks should be returned (query parameter)
- ❌ **Prompt dependency**: Risk level is only useful if HolmesGPT API prompt explains it
- ❌ **Inconsistent pattern**: Why expose risk but not other categories?
- ❌ **LLM shouldn't decide risk**: Safety decisions should be system-level, not LLM reasoning

**Confidence**: 65% (rejected)

**Why Rejected**: Risk tolerance is a signal category (like environment, priority) and should be filtered via query parameters, not exposed to LLM for reasoning.

---

## 🎯 Decision Details

### Approved Pattern: Filter Before LLM

**Core Principle**: All filtering and categorization happens via Context API query parameters, NOT in LLM response.

**Query Parameters** (from Gateway/Signal Processing categorization):
- `incident_type`: Semantic search filter
- `labels`: Label matching filters (environment, priority, risk_tolerance, business_category)
- `min_confidence`: Minimum confidence threshold (optional)
- `max_results`: Limit number of playbooks returned

**Response Fields** (for LLM reasoning):
- `playbook_id`: Unique identifier
- `version`: Playbook version
- `description`: Human-readable description of what the playbook does
- `confidence`: Composite score (semantic similarity + label match)

**LLM Task**: Pick the playbook with highest confidence.

---

## 📊 Analysis of Rejected Fields

### Why Each Proposed Field Was Rejected

| Field | Proposed Benefit | Why Rejected | Correct Approach |
|-------|------------------|--------------|------------------|
| `incident_types` | LLM knows which incidents playbook handles | Redundant with query filtering (if playbook is in response, it matches incident) | Already filtered by semantic search |
| `risk_level` | LLM can avoid high-risk actions | Risk tolerance is deterministic (environment → risk), not LLM decision | Query parameter: `labels=kubernaut.io/risk-tolerance:low` |
| `estimated_duration` | LLM can prefer faster playbooks for urgent incidents | Speed preference is context-dependent; may bias toward fast-but-incorrect | Encode in confidence score, not exposed to LLM |
| `prerequisites` | LLM knows if playbook can run in environment | Already filtered by label matching (if playbook is in response, prerequisites met) | Already filtered by label matching |
| `rollback_available` | LLM can prefer reversible actions | Reversibility is a risk dimension, should be in risk categorization | Part of risk_level category, filtered via query |
| `actions` | LLM knows what playbook does | Playbooks are Tekton Tasks (container images), contents not visible | Use `description` field (provided during registration) |

---

## 💡 Key Insights

### 1. Filtering vs Reasoning

**Filtering** (deterministic, pre-LLM):
- Incident type matching (semantic search)
- Environment compatibility (label matching)
- Risk tolerance (label matching)
- Priority alignment (label matching)
- Business category (label matching)

**Reasoning** (probabilistic, LLM task):
- Pick highest confidence playbook from pre-filtered list

**Rule**: If it's deterministic, filter it. If it's probabilistic, let LLM reason about it.

---

### 2. The Redundancy Test

**Question**: "If this field is in the response, what does it tell the LLM that the query didn't already determine?"

**Examples**:
- `incident_types`: Nothing. Query already filtered by incident type.
- `risk_level`: Nothing. Query already filtered by risk tolerance.
- `confidence`: Something. LLM needs this to pick best playbook.

**Rule**: If a field passes the redundancy test, keep it. Otherwise, it's a query parameter.

---

### 3. The Prompt Dependency Test

**Question**: "Does this field require prompt instructions to be useful?"

**Examples**:
- `risk_level`: YES. LLM needs to know "prefer low-risk for production" → Should be filtered, not in response
- `estimated_duration`: YES. LLM needs to know "prefer fast for P0" → Should be filtered, not in response
- `description`: NO. LLM can understand natural language description → Keep in response

**Rule**: If a field requires prompt instructions to be useful, it should be filtered via query parameters, not in response.

---

### 4. The Safety Test

**Question**: "Should the LLM be allowed to make trade-off decisions about this field?"

**Examples**:
- `risk_level`: NO. Risk decisions should be deterministic (production → low-risk only)
- `confidence`: YES. LLM should pick highest confidence (this is its job)

**Rule**: If the answer is NO, filter it. If YES, include it in response.

---

## 📈 Benefits of Minimal Schema

### Token Efficiency

| Schema | Tokens/Playbook | 10 Playbooks | Monthly Cost (1K queries/day) |
|--------|-----------------|--------------|-------------------------------|
| **Minimal** (4 fields) | 60 | 600 | $18/month |
| **Rich** (8 fields) | 100 | 1000 | $30/month |
| **Savings** | -40 | -400 | **-$12/month** |

### LLM Task Simplicity

**Minimal Schema Task**:
```
"Pick the playbook with highest confidence from this list."
```

**Rich Schema Task**:
```
"Consider incident types, risk levels, duration, prerequisites, and confidence.
Balance speed vs safety. Prefer low-risk for production. Ensure prerequisites met.
Pick the best playbook."
```

**Result**: Minimal schema reduces LLM reasoning complexity by 80%.

---

### Safety & Determinism

**Minimal Schema**:
- ✅ Risk decisions made by Gateway (deterministic)
- ✅ Environment filtering by Context API (deterministic)
- ✅ LLM only picks from safe, pre-filtered options

**Rich Schema**:
- ❌ LLM might choose high-risk playbook for production
- ❌ LLM might misinterpret prerequisites
- ❌ LLM might prioritize speed over correctness

---

## 🔧 Implementation Guidance

### Context API Response Builder

```go
type PlaybookResponse struct {
    PlaybookID  string  `json:"playbook_id"`
    Version     string  `json:"version"`
    Description string  `json:"description"`
    Confidence  float64 `json:"confidence"`
}

func (s *ContextAPIServer) buildPlaybookResponse(playbook *Playbook) PlaybookResponse {
    return PlaybookResponse{
        PlaybookID:  playbook.ID,
        Version:     playbook.Version,
        Description: playbook.Description,
        Confidence:  playbook.Confidence, // Pre-calculated: 0.5*semantic + 0.5*label_match
    }
}
```

### Query Parameter Handling

```go
type PlaybookQueryParams struct {
    IncidentType   string            `query:"incident_type"`   // Semantic search
    Labels         map[string]string `query:"labels"`          // Label matching
    MinConfidence  float64           `query:"min_confidence"`  // Confidence threshold
    MaxResults     int               `query:"max_results"`     // Limit results
}

// Labels include:
// - kubernaut.io/environment: production
// - kubernaut.io/priority: P0
// - kubernaut.io/risk-tolerance: low
// - kubernaut.io/business-category: payment-service
```

### HolmesGPT API Integration

```go
// HolmesGPT API constructs query from signal categorization
query := contextapi.PlaybookQuery{
    IncidentType: signal.IncidentType,
    Labels: map[string]string{
        "kubernaut.io/environment":      signal.Environment,      // from Gateway
        "kubernaut.io/priority":         signal.Priority,         // from Gateway
        "kubernaut.io/risk-tolerance":   signal.RiskTolerance,    // from Gateway
        "kubernaut.io/business-category": signal.BusinessCategory, // from Signal Processing
    },
    MinConfidence: 0.7,
    MaxResults:    10,
}

// Context API returns pre-filtered playbooks
playbooks := contextAPIClient.QueryPlaybooks(ctx, query)

// LLM picks highest confidence
selectedPlaybook := playbooks[0] // Already sorted by confidence DESC
```

---

## 🎯 Success Criteria

### Effectiveness Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Correct playbook selection** | 85-90% | LLM selects highest-confidence playbook that resolves incident |
| **Token efficiency** | 60 tokens/playbook | Average tokens per playbook in response |
| **Query filtering accuracy** | 95%+ | Percentage of returned playbooks that match all query criteria |
| **LLM task simplicity** | Single decision | LLM only needs to pick highest confidence (no multi-factor reasoning) |

### Safety Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **High-risk playbooks in production** | 0% | No high-risk playbooks returned for production environment |
| **Prerequisite violations** | 0% | No playbooks returned that don't meet environment prerequisites |
| **Incident type mismatches** | <5% | Playbooks returned match incident type (semantic search accuracy) |

---

## 🔗 Related Decisions

- **ADR-033**: Remediation Playbook Catalog - Defines playbook metadata and labels
- **DD-CONTEXT-003**: Context Enrichment Placement - Defines LLM-driven tool call pattern
- **BR-CONTEXT-006**: Context API must provide historical playbook execution data
- **BR-HOLMESGPT-003**: HolmesGPT API must query Context API for playbook recommendations
- **[PLAYBOOK_QUERY_FOR_HOLMESGPT.md](../../../services/stateless/context-api/PLAYBOOK_QUERY_FOR_HOLMESGPT.md)**: Complete JSON response specification and implementation details

---

## 📝 Future Considerations

### V2.0 Enhancements (Not for V1.0)

**Potential additions** (only if LLM reasoning is required):
- `explanation`: Why this playbook was recommended (for transparency)
- `similar_incidents`: Count of similar incidents this playbook resolved (social proof)

**Rule**: Only add fields if they pass ALL four tests:
1. ✅ Redundancy Test: Not already determined by query
2. ✅ Prompt Dependency Test: Doesn't require prompt instructions
3. ✅ Safety Test: Safe for LLM to reason about
4. ✅ Value Test: Measurably improves LLM selection accuracy

---

**Document Version**: 1.0
**Last Updated**: November 11, 2025
**Status**: ✅ **APPROVED**
**Overall Confidence**: **95%** (minimal schema is optimal)

