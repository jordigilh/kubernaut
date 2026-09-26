# DD-LLM-001: MCP Workflow Search Parameter Taxonomy

**Status**: Proposed
**Date**: 2025-11-16
**Deciders**: Architecture Team
**Related**: ADR-041, DD-WORKFLOW-001, DD-STORAGE-008
**Version**: 1.2

> **Severity authority correction**: The v1.1 clauses below that tell the LLM to assess or override severity are superseded by BR-SP-105 and DD-SEVERITY-001. SignalProcessing's Rego classification is authoritative; the LLM must pass it through unchanged in RCA and workflow-search fields.

## Changelog

### Version 1.1 (2025-11-16)
- Clarified that `signal_type` in search query comes from LLM's RCA assessment, not input signal
- (Superseded) Clarified that `severity` in search query comes from LLM's RCA assessment, not input signal
- Emphasized that LLM determines technical fields based on investigation; severity is now explicitly excluded from this responsibility by BR-SP-105

### Version 1.2 (2026-09-25)
- Corrected severity authority: the SP Rego classification is passed through unchanged; RCA evidence must not cause the LLM to reclassify severity


### Version 1.0 (2025-11-16)
- Initial version defining MCP search parameter taxonomy for LLM
- Establishes query format: `<signal_type> <severity> [optional_keywords]`
- Defines canonical signal type taxonomy
- Defines RCA severity assessment criteria
- Clarifies business/policy field pass-through requirement
- Recommends signal_type and severity as workflow labels for exact matching

---

## Context

### Problem Statement

The LLM prompt (ADR-041) instructs the LLM to search for workflows via MCP, but does NOT explain:

1. **What format the query should use** (natural language vs structured)
2. **What values are valid** for signal_type and severity
3. **Which fields are search parameters** vs workflow parameters
4. **How to optimize for high confidence scores**

**Current Gap**:
```
LLM Prompt: "Search for appropriate workflows"
LLM Response: ??? (No guidance on query format or valid values)
MCP Search: Low confidence scores due to poor semantic matching
```

**Impact**:
- LLM doesn't know how to construct the search query
- Low confidence scores (60-70%) due to poor keyword matching
- Inconsistent query formats lead to unpredictable results
- No guidance on which fields to pass through vs determine

### Requirements

**BR-WORKFLOW-001**: Workflow catalog must support semantic search by signal type, severity, and business context
**BR-WORKFLOW-003**: LLM must select appropriate workflows based on RCA findings
**BR-AI-001**: LLM must perform independent RCA without contamination

---

## Decision

**Define a structured query format and taxonomy for MCP workflow search that optimizes for high confidence scores through exact label matching combined with semantic ranking.**

This taxonomy will:
- **Define query format**: `<signal_type> <severity> [optional_keywords]`
- **Establish canonical values**: For signal_type and severity
- **Clarify field roles**: Query vs label filters vs workflow parameters
- **Optimize confidence**: Through exact matching + semantic ranking
- **Versioned**: Tracked in this DD with changelog

---

## Design

### 1. MCP Search Architecture

#### 1.1 How Semantic Search Works

**Two-Phase Filtering**:

1. **Phase 1: Exact Label Matching** (SQL WHERE clause)
   - Filters workflows by exact label matches
   - Eliminates false positives
   - Fast (indexed lookups)

2. **Phase 2: Semantic Ranking** (pgvector similarity)
   - Ranks filtered workflows by query similarity
   - Uses embedding cosine distance
   - Returns top N results with confidence scores

**SQL Query Pattern**:
```sql
SELECT
    playbook_id,
    version,
    description,
    parameters,
    1 - (embedding <=> $query_embedding) AS confidence
FROM workflow_catalog
WHERE status = 'active'
  AND is_latest_version = true
  -- EXACT LABEL MATCHING (Phase 1)
  AND labels->>'kubernaut.io/signal-type' = 'OOMKilled'
  AND labels->>'kubernaut.io/severity' = 'critical'
  AND labels->>'kubernaut.io/environment' = 'production'
  AND labels->>'kubernaut.io/priority' = 'P0'
  AND labels->>'kubernaut.io/risk-tolerance' = 'low'
  AND labels->>'kubernaut.io/business-category' = 'revenue-critical'
  -- SEMANTIC RANKING (Phase 2)
  AND 1 - (embedding <=> $query_embedding) >= 0.7
ORDER BY embedding <=> $query_embedding
LIMIT 10;
```

#### 1.2 Confidence Score Optimization

**Strategy**: Combine exact label matching with semantic ranking

**Before Optimization**:
- Query: "OOMKilled critical"
- Workflow description: "Increases memory limits and restarts pod on OOM"
- Semantic similarity: ~70% (limited keyword overlap)

**After Optimization**:
- Exact match on labels: `signal-type=OOMKilled`, `severity=critical`
- Workflow description: "OOMKilled critical: Increases memory limits and restarts pod on OOM"
- Semantic similarity: ~95% (direct keyword matching)

**Result**: Confidence scores increase from 60-70% to 90-95%

---

### 2. Query Format Specification

#### 2.1 Query Structure

**Format**: `<signal_type> <severity> [optional_keywords]`

**Required Components**:
1. `signal_type`: Canonical Kubernetes event reason (first word)
2. `severity`: SignalProcessing's normalized severity (second word; pass through unchanged)

**Optional Components**:
3. Additional context keywords for semantic matching

**Examples**:
```
OOMKilled critical
CrashLoopBackOff high
ImagePullBackOff medium
OOMKilled critical deployment memory
NodeNotReady critical infrastructure
```

#### 2.2 Query Construction Guidelines

**DO**:
- ✅ Use canonical signal_type (exact Kubernetes event reason)
- ✅ Use the SignalProcessing severity unchanged (do not reassess it from RCA findings)
- ✅ Put signal_type first, severity second (consistent order)
- ✅ Add optional context keywords for better semantic matching
- ✅ Use space-separated format (for embedding compatibility)

**DO NOT**:
- ❌ Use natural language descriptions ("memory issue", "pod crashing")
- ❌ Include resource-specific details (namespace, pod name)
- ❌ Use JSON or structured formats (breaks embedding)
- ❌ Include business/policy fields in query (use labels instead)

---

### 3. MCP Search Parameters

#### 3.1 Complete Parameter List

| Parameter | Type | Source | LLM Role | Purpose |
|-----------|------|--------|----------|---------|
| `query` | string | LLM constructs using SP context | **Construct** from signal type + supplied severity + keywords | Semantic search (signal_type + severity + keywords) |
| `label.signal-type` | string | LLM's RCA | **Determine** from investigation | Exact match filter (canonical event) |
| `label.severity` | string | SignalProcessing classification | **Pass-through unchanged** | Exact match filter (SP severity) |
| `label.environment` | string | Input | **Pass-through** | Exact match filter (production/staging/etc) |
| `label.priority` | string | Input | **Pass-through** | Exact match filter (P0/P1/P2/P3) |
| `label.risk-tolerance` | string | Input | **Pass-through** | Exact match filter (low/medium/high) |
| `label.business-category` | string | Input | **Pass-through** | Exact match filter (revenue-critical/etc) |
| `min_confidence` | float | System default | N/A | Minimum similarity threshold (default: 0.7) |
| `max_results` | int | System default | N/A | Maximum results to return (default: 10) |

#### 3.2 Field Roles

**LLM Determines (Technical Assessment)**:
- `query`: Constructed from RCA findings, while retaining the supplied SP severity
- `label.signal-type`: Canonical Kubernetes event from investigation

**LLM Pass-Through (Business/Policy)**:
- `label.severity`: SignalProcessing's Rego classification (must not be recalculated)
- `label.environment`: Deployment classification (not technical)
- `label.priority`: Business priority (not technical severity)
- `label.risk-tolerance`: Organizational policy (not technical decision)
- `label.business-category`: Business classification (not technical)

**Key Principle**: The LLM may determine RCA findings and signal type, but must pass through SignalProcessing's severity and the other policy fields unchanged.

---

### 4. Signal Type Taxonomy (Canonical Values)

**Source**: DD-WORKFLOW-001 (Signal Type Classification)
**Format**: Exact Kubernetes event reason (case-sensitive)

| Canonical Value | Description | Investigation Indicators |
|----------------|-------------|-------------------------|
| `OOMKilled` | Container exceeded memory limit and was killed | Event: OOMKilled, Exit code: 137, Memory limit exceeded |
| `CrashLoopBackOff` | Container repeatedly crashing | Multiple restarts, Exit code: 1/2, Application panic |
| `ImagePullBackOff` | Cannot pull container image | ImagePullBackOff event, Registry auth failure, Image not found |
| `Evicted` | Pod evicted due to resource pressure | Eviction event, Node pressure (disk/memory), Threshold exceeded |
| `NodeNotReady` | Node is not ready | Node condition: NotReady, Kubelet not responding |
| `PodPending` | Pod stuck in pending state | Pod phase: Pending, Insufficient resources, Unschedulable |
| `FailedScheduling` | Scheduler cannot place pod | FailedScheduling event, No nodes match constraints |
| `BackoffLimitExceeded` | Job exceeded retry limit | Job status: Failed, Backoff limit reached |
| `DeadlineExceeded` | Job exceeded active deadline | Job status: Failed, Active deadline exceeded |
| `FailedMount` | Volume mount failed | FailedMount event, PVC not bound, CSI driver error |

**Usage Guidelines**:

**DO**:
- ✅ Use exact canonical value from table above
- ✅ Investigate Kubernetes events to identify canonical type
- ✅ Override input signal_type if investigation reveals different event

**DO NOT**:
- ❌ Use natural language descriptions ("memory exhaustion", "out of memory")
- ❌ Use generic terms ("crash", "failure", "error")
- ❌ Invent new signal types not in canonical list

**Example**:
```
Input: signal_type="HighMemoryUsage"
Investigation: kubectl describe pod shows "OOMKilled" event, exit code 137
Query: "OOMKilled critical"
Label: label.signal-type=OOMKilled ✅
```

---

### 5. Severity Pass-Through (BR-SP-105)

SignalProcessing determines normalized severity from the external signal using the operator-configured Rego policy. The LLM receives that classification as input and MUST NOT reassess, infer, or replace it based on investigation evidence, environment, business impact, or RCA findings.

**Rules**:
- Copy the exact input severity to `root_cause_analysis.severity`.
- Use the exact input severity for `label.severity` and in the workflow-search query.
- If the input classification is `unknown`, preserve `unknown`; do not upgrade it based on the investigation.
- The LLM may report impact and uncertainty in the RCA narrative, but severity classification remains owned by SignalProcessing.

**Example**:
```
SignalProcessing input: severity="high"
Investigation: confirms a production outage affecting users
RCA: may explain the outage and its impact, but must retain severity="high"
Query: "OOMKilled high"
Label: label.severity=high
```

---

### 6. Business/Policy Field Taxonomy (Pass-Through)

#### 6.1 Environment

**CRITICAL**: Environment is a deployment classification, NOT a technical assessment. The LLM must ALWAYS pass through the input value.

| Value | Meaning |
|-------|---------|
| `production` | Production environment |
| `staging` | Staging/pre-production environment |
| `development` | Development environment |
| `test` | Test environment |

**Usage**: Copy from input to `label.environment` parameter (ALWAYS pass through)

---

#### 6.2 Priority

**CRITICAL**: Priority is a BUSINESS decision, NOT a technical assessment. The LLM must ALWAYS pass through the input value.

| Level | Business Meaning |
|-------|------------------|
| `P0` | Critical business impact, revenue-affecting, executive escalation |
| `P1` | High business impact, major feature degraded, customer escalation |
| `P2` | Medium business impact, minor feature affected, internal escalation |
| `P3` | Low business impact, cosmetic or minor issue, no escalation |

**Usage**: Copy from input to `label.priority` parameter (ALWAYS pass through)

**Rationale**: Priority reflects business policy and organizational response, not technical severity.

---

#### 6.3 Risk Tolerance

**CRITICAL**: Risk tolerance is a POLICY decision, NOT a technical assessment. The LLM must ALWAYS pass through the input value.

| Level | Remediation Approach |
|-------|---------------------|
| `low` | Conservative, minimal changes, no service disruption |
| `medium` | Balanced, moderate changes, rolling restarts acceptable |
| `high` | Aggressive, immediate fixes, downtime acceptable |

**Usage**: Copy from input to `label.risk-tolerance` parameter (ALWAYS pass through)

**Rationale**: Risk tolerance reflects organizational SLA requirements and change policies.

---

#### 6.4 Business Category

**CRITICAL**: Business category is a business classification. The LLM must ALWAYS pass through the input value.

**Examples**: `revenue-critical`, `customer-facing`, `internal`, `analytics`, `infrastructure`

**Usage**: Copy from input to `label.business-category` parameter (ALWAYS pass through)

---

### 7. Complete MCP Search Example

#### 7.1 Scenario: OOMKilled in Production Payment Service

**Input to LLM**:
```
Signal Information:
- Source: prometheus-adapter
- Type: OOMKilled
- Severity: critical
- Namespace: production
- Resource: deployment/payment-service

Business Context:
- Environment: production
- Priority: P0
- Business Category: revenue-critical
- Risk Tolerance: low

Error Details:
- Error: OOMKilled
- Error Message: Container exceeded memory limit of 512Mi
```

**LLM's RCA Investigation**:
```
1. Analyzed Signal: OOMKilled event confirmed
2. Examined Resource: deployment/payment-service in production namespace
3. Reviewed Metrics: Memory usage exceeded 512Mi limit
4. Root Cause: Insufficient memory allocation for current load
5. SignalProcessing severity: critical (authoritative input classification; copied unchanged)
```

**LLM's MCP Search Call**:
```
GET /api/v1/playbooks/search?
  query=OOMKilled critical
  &label.signal-type=OOMKilled
  &label.severity=critical
  &label.environment=production
  &label.priority=P0
  &label.risk-tolerance=low
  &label.business-category=revenue-critical
  &min_confidence=0.7
  &max_results=10
```

**MCP Response**:
```json
{
  "playbooks": [
    {
      "playbook_id": "increase-memory-conservative-oom",
      "version": "v1.2",
      "description": "OOMKilled critical: Increases memory limits conservatively without restart",
      "confidence": 0.95,
      "parameters": {
        "NAMESPACE": {
          "type": "string",
          "required": true,
          "description": "Kubernetes namespace"
        },
        "DEPLOYMENT_NAME": {
          "type": "string",
          "required": true,
          "description": "Deployment name"
        },
        "MEMORY_INCREMENT_MB": {
          "type": "integer",
          "required": true,
          "default": 256,
          "description": "Memory increment in MB"
        }
      }
    },
    {
      "playbook_id": "scale-horizontal-oom-recovery",
      "version": "v2.0",
      "description": "OOMKilled critical: Add replicas to distribute load before increasing memory",
      "confidence": 0.88,
      "parameters": {
        "NAMESPACE": {"type": "string", "required": true},
        "DEPLOYMENT_NAME": {"type": "string", "required": true},
        "REPLICA_INCREMENT": {"type": "integer", "required": true, "default": 2}
      }
    }
  ],
  "total_results": 2
}
```

**LLM's JSON Response**:
```json
{
  "analysis_summary": "OOMKilled event confirmed in production payment service due to insufficient memory allocation under current load",
  "root_cause_assessment": "Container exceeded 512Mi memory limit (actual usage ~600Mi). Root cause is insufficient memory allocation for current traffic levels. No memory leak detected.",
  "rca_severity": "critical",
  "selected_workflow": {
    "workflow_id": "increase-memory-conservative-oom",
    "confidence": 0.95,
    "rationale": "MCP search with signal_type=OOMKilled, severity=critical, risk_tolerance=low returned this conservative memory increase workflow with highest confidence. Workflow respects low risk tolerance by avoiding service restart.",
    "parameters": {
      "NAMESPACE": "production",
      "DEPLOYMENT_NAME": "payment-service",
      "MEMORY_INCREMENT_MB": 256
    }
  },
  "alternative_workflows": [
    {
      "workflow_id": "scale-horizontal-oom-recovery",
      "confidence": 0.88,
      "rationale": "Alternative approach: Add replicas to distribute load before increasing memory"
    }
  ],
  "warnings": [
    "Conservative memory increase (256MB) may require multiple iterations if load continues"
  ]
}
```

#### 7.2 Field Justification

**Query Construction**:
- `query="OOMKilled critical"`: ✅ Canonical signal_type + SignalProcessing severity
- Simple format for semantic matching with workflow descriptions

**Label Parameters (Exact Matching)**:
- `label.signal-type=OOMKilled`: ✅ LLM determined from investigation
- `label.severity=critical`: ✅ Passed through unchanged from SignalProcessing
- `label.environment=production`: ✅ Pass-through from input
- `label.priority=P0`: ✅ Pass-through from input
- `label.risk-tolerance=low`: ✅ Pass-through from input
- `label.business-category=revenue-critical`: ✅ Pass-through from input

**Workflow Parameters (Populated by LLM)**:
- `NAMESPACE=production`: From input resource identification
- `DEPLOYMENT_NAME=payment-service`: From input resource identification
- `MEMORY_INCREMENT_MB=256`: From workflow parameter default

---

## Implementation

### 8.1 LLM Prompt Integration

This taxonomy must be included in the LLM prompt (ADR-041) as a dedicated section.

**Prompt Section: "MCP Workflow Search Guidance"**

**Location**: After the signal context, before "Output Format"

**Content**: Summary of this DD with:
- Query format specification: `<signal_type> <severity> [optional_keywords]`
- Signal type canonical values (top 10 most common)
- SignalProcessing severity pass-through requirement (no LLM reassessment)
- Business/policy field pass-through requirement
- Complete MCP search example

**Prompt Length**: ~500-600 words (acceptable for Claude 4.5 Haiku)

### 8.2 DD-WORKFLOW-001 v1.8 Alignment

**STATUS**: ✅ DD-WORKFLOW-001 v1.8 is now the authoritative source (snake_case API fields).

**Current (v1.4)**:
- **5 mandatory labels**: `signal_type`, `severity`, `component`, `environment`, `priority`
- **DetectedLabels**: Auto-detected (GitOps, PDB, HPA, etc.) - no config required
- **CustomLabels**: Customer-defined via Rego (pass-through, Kubernaut doesn't validate)

**Label Usage**:
- `signal_type` and `severity` are used for BOTH:
  1. Exact label filtering in MCP search
  2. Workflow description keywords for semantic matching
- DetectedLabels/CustomLabels passed through to Data Storage for workflow filtering

**Workflow Description Format**:
```
Format: "<signal_type> <severity>: <description>"
Example: "OOMKilled critical: Increases memory limits and restarts pod on OOM"
```

### 8.3 Validation

**Unit Tests** (`test_prompt_generation_adr041.py`):
- [ ] Test MCP search guidance section is present in prompt
- [ ] Test query format is explained
- [ ] Test signal type canonical values are listed
- [ ] Test SignalProcessing severity pass-through is explained
- [ ] Test business/policy pass-through is clarified
- [ ] Test complete example is included

**Integration Tests** (Future - with real LLM):
- [ ] Test LLM constructs query correctly (`<signal_type> <severity>`)
- [ ] Test LLM uses canonical signal types (not natural language)
- [ ] Test LLM preserves the SP severity regardless of RCA findings
- [ ] Test LLM passes through business/policy fields unchanged
- [ ] Test LLM populates workflow parameters correctly
- [ ] Test confidence scores are 90%+ for exact matches

---

## Consequences

### Positive

✅ **Clear query format** for LLM (`<signal_type> <severity>`)
✅ **High confidence scores** (90-95%) through exact label matching
✅ **Canonical taxonomy** ensures consistent workflow search results
✅ **Clear field roles** (LLM determines vs pass-through)
✅ **Optimized semantic search** through structured query + exact filtering
✅ **Versioned in DD** allows taxonomy evolution without breaking changes

### Negative

⚠️ **Increased prompt length** (~500-600 words) - acceptable for Claude 4.5 Haiku
⚠️ **Taxonomy maintenance** required as new signal types are added
⚠️ **Workflow description format** must include signal_type and severity keywords
⚠️ **DD-WORKFLOW-001 updates** required to add signal_type and severity as search labels

### Neutral

ℹ️ **Query format is prescriptive** - limits LLM flexibility (intentional design)
ℹ️ **Business fields are immutable** - LLM cannot adapt to context (policy requirement)
ℹ️ **Requires workflow catalog updates** - descriptions must follow format

---

## Alternatives Considered

### Alternative 1: Natural Language Query Only

**Approach**: Allow LLM to use natural language for query, rely on semantic search alone

**Example**: `query="pod keeps crashing due to memory issues"`

**Pros**:
- Simpler prompt (no format specification needed)
- LLM has more flexibility

**Cons**:
- ❌ Low confidence scores (60-70%) due to poor keyword matching
- ❌ Inconsistent query formats lead to unpredictable results
- ❌ No exact matching on signal_type and severity
- ❌ Harder to debug (why did search fail?)

**Decision**: Rejected - too unreliable for production system

---

### Alternative 2: JSON Query Structure

**Approach**: Use JSON for structured query

**Example**: `query={"signal_type":"OOMKilled","severity":"critical"}`

**Pros**:
- Fully structured
- Type-safe parsing

**Cons**:
- ❌ Doesn't work with semantic embeddings (embeddings are for text, not JSON)
- ❌ URL encoding makes it ugly
- ❌ Defeats the purpose of semantic search

**Decision**: Rejected - incompatible with semantic embeddings

---

### Alternative 3: Separate Signal Type and Severity Parameters

**Approach**: Pass signal_type and severity as separate query parameters, not in query string

**Example**: `query=&signal_type=OOMKilled&severity=critical`

**Pros**:
- Clean parameter separation
- Easy to parse

**Cons**:
- ❌ Doesn't leverage semantic search for signal_type and severity
- ❌ Requires exact string matching only
- ❌ Loses benefit of embedding similarity

**Decision**: Rejected - we use BOTH (query string for semantic + labels for exact)

---

## References

- **ADR-041**: LLM Prompt and Response Contract (parent document)
- **[DD-WORKFLOW-002](../DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md)**: MCP Workflow Catalog Architecture (updated to v2.0 to align with this DD)
- **DD-WORKFLOW-001**: Signal Type Classification and Mandatory Label Schema
- **DD-STORAGE-008**: Workflow Catalog Schema (semantic search implementation)
- **BR-WORKFLOW-001**: Workflow catalog semantic search requirements
- **BR-AI-001**: LLM independent RCA requirements

---

## Confidence Assessment

**95% confidence** this taxonomy optimizes MCP search for high confidence scores:

**Evidence**:
- ✅ Combines exact label matching with semantic ranking
- ✅ Structured query format (`<signal_type> <severity>`) is simple and effective
- ✅ Canonical taxonomy ensures consistency
- ✅ Clear field roles prevent LLM from overriding business policy
- ✅ Workflow description format includes keywords for semantic matching
- ✅ Expected confidence scores: 90-95% for exact matches

**Risks**:
- ⚠️ Requires DD-WORKFLOW-001 updates - mitigated by clear specification
- ⚠️ Workflow descriptions must follow format - mitigated by validation
- ⚠️ LLM might still make mistakes - mitigated by clear examples and validation

**Mitigation**:
- Comprehensive unit tests for prompt generation
- Integration tests with real LLM (future)
- Versioned taxonomy allows evolution without breaking changes
- Clear examples reduce ambiguity and LLM errors
