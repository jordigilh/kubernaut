# DD-LLM-002: Canonical Signal Type Taxonomy (v1.1 - Pending Review)

**Version**: 1.0
**Status**: 🔮 **Proposed for v1.1** (Pending Review)
**Last Updated**: 2025-11-16
**Related**: [ADR-041](./ADR-041-llm-prompt-response-contract.md)

---

## Context & Problem

### Current State (v1.0)

The LLM prompt currently provides a few example signal types and references an external URL:

```markdown
**Note**: These are common examples. Use any canonical Kubernetes event reason that matches your RCA findings.
For complete list, see: https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/event-v1/#Event
```

### Problems

1. **External URL Inaccessible**: LLM cannot access external URLs during investigation
2. **Open-Ended Selection**: "Use any canonical" is too vague, LLM might invent terms
3. **Risk of Wrong Signal Type**: LLM might provide non-existent signal types (e.g., "MemoryExhausted" instead of "OOMKilled")
4. **MCP Search Failure Risk**: Invalid signal types lead to poor workflow matching
5. **No Validation**: No mechanism to catch or normalize LLM errors

### Impact Assessment

| Risk Scenario | Likelihood | Impact | v1.0 Mitigation |
|---|---|---|---|
| LLM uses non-canonical signal_type | MEDIUM | MEDIUM | Semantic search might still match |
| LLM invents completely wrong term | LOW | HIGH | Returns `null` workflow |
| LLM uses close variant (e.g., "OOMKill" vs "OOMKilled") | MEDIUM | LOW | Semantic search handles similarity |
| Taxonomy changes over time | LOW | LOW | Manual prompt updates |

---

## Decision (Deferred to v1.1)

**Defer comprehensive signal type taxonomy to v1.1.**

### Rationale

1. **v1.0 Priority**: Focus on core workflow selection functionality
2. **Semantic Search Resilience**: pgvector semantic search can handle minor variations
3. **Low Risk**: LLM (Claude 4.5 Haiku) is generally good at canonical Kubernetes terms
4. **Complexity vs. Benefit**: Full taxonomy adds prompt length without proven need
5. **Can Monitor**: Track LLM signal_type selections in v1.0 to identify actual issues

### v1.0 Approach (Current)

- Provide example signal types in prompt
- Rely on LLM's knowledge of Kubernetes
- Rely on semantic search to handle variations
- Monitor for issues in production

---

## Proposed Solutions for v1.1

### Option 1: Complete Taxonomy in Prompt (Recommended for v1.1)

**Approach**: Include full list of ~50-100 canonical Kubernetes event reasons in prompt

**Example**:
```markdown
### Valid Signal Types (Canonical Kubernetes Event Reasons)

**CRITICAL**: Use ONLY these signal types for workflow search. Do NOT invent new terms.

#### Pod Lifecycle
- `Scheduled`, `FailedScheduling`, `Preempted`

#### Container Image
- `Pulling`, `Pulled`, `Failed`, `ErrImagePull`, `ImagePullBackOff`

#### Container Issues
- `BackOff`, `CrashLoopBackOff`, `OOMKilled`, `Evicted`

#### Volume
- `FailedMount`, `FailedAttachVolume`, `FailedDetachVolume`

#### Health Checks
- `Unhealthy`, `ProbeSucceeded`, `ProbeFailed`

#### Node Issues
- `NodeNotReady`, `NodeMemoryPressure`, `NodeDiskPressure`

#### Scaling & Replication
- `ScalingReplicaSet`, `SuccessfulCreate`, `FailedCreate`

**Mapping Examples**:
- RCA: "Container exceeded memory limit" → Use `OOMKilled`
- RCA: "Image not found" → Use `ErrImagePull`
- RCA: "Node has insufficient memory" → Use `NodeMemoryPressure`
```

**Pros**:
- ✅ LLM has complete reference (no guessing)
- ✅ No external dependencies
- ✅ Deterministic selection
- ✅ Simple implementation (just update prompt)

**Cons**:
- ⚠️ Longer prompt (~200-300 lines)
- ⚠️ Static list (requires manual updates)

**Confidence**: 85%

---

### Option 2: MCP Signal Type Normalization Tool

**Approach**: Create MCP tool that normalizes LLM's natural language to canonical signal_type

**Tool Specification**:
```json
{
  "name": "normalize_signal_type",
  "description": "Converts natural language RCA finding to canonical Kubernetes signal type",
  "inputSchema": {
    "type": "object",
    "properties": {
      "rca_finding": {
        "type": "string",
        "description": "Natural language description of root cause"
      }
    },
    "required": ["rca_finding"]
  }
}
```

**Example Usage**:
```
LLM RCA: "Container exceeded memory limit and was terminated"
↓
LLM calls: normalize_signal_type(rca_finding="Container exceeded memory limit")
↓
MCP returns: {"signal_type": "OOMKilled", "confidence": 0.95}
↓
LLM uses: "OOMKilled" for workflow search
```

**Implementation**:
- Semantic search against canonical signal type list
- Returns closest match with confidence score
- Handles typos and variations automatically

**Pros**:
- ✅ Handles LLM variations/typos automatically
- ✅ Shorter prompt (no need to list all signal types)
- ✅ Extensible (add new signal types without prompt changes)
- ✅ Confidence score for validation

**Cons**:
- ❌ Requires MCP server implementation
- ❌ Additional tool call (latency)
- ❌ More complex architecture

**Confidence**: 90%

---

### Option 3: Backend Validation (Alternative)

**Approach**: Validate and normalize signal_type in `holmesgpt-api` before MCP search

**Implementation**:
```python
CANONICAL_SIGNAL_TYPES = {
    "OOMKilled", "CrashLoopBackOff", "ImagePullBackOff", "Evicted",
    "FailedScheduling", "NodeNotReady", "Unhealthy", "BackOff",
    # ... complete list
}

def validate_and_normalize_signal_type(llm_signal_type: str) -> tuple[str, float]:
    """Validates and normalizes LLM's signal_type."""
    # Exact match
    if llm_signal_type in CANONICAL_SIGNAL_TYPES:
        return llm_signal_type, 1.0

    # Case-insensitive match
    for canonical in CANONICAL_SIGNAL_TYPES:
        if llm_signal_type.lower() == canonical.lower():
            return canonical, 0.95

    # Fuzzy match (using embeddings or string similarity)
    best_match, confidence = find_closest_match(llm_signal_type, CANONICAL_SIGNAL_TYPES)

    if confidence > 0.7:
        logger.warning(f"Normalized: {llm_signal_type} → {best_match}")
        return best_match, confidence
    else:
        logger.error(f"Invalid signal_type: {llm_signal_type}")
        return llm_signal_type, 0.0
```

**Pros**:
- ✅ Catches LLM errors before MCP search
- ✅ Can normalize minor variations
- ✅ Logs issues for monitoring
- ✅ No MCP changes needed

**Cons**:
- ⚠️ Adds complexity to holmesgpt-api
- ⚠️ Requires embedding/similarity library
- ⚠️ Might "fix" LLM errors incorrectly

**Confidence**: 75%

---

## Recommended Approach for v1.1

**Hybrid: Option 1 (Taxonomy in Prompt) + Option 3 (Backend Validation)**

### Phase 1: Add Complete Taxonomy to Prompt
- Compile complete list from Kubernetes API docs
- Add to ADR-041 prompt as categorized list
- Include mapping examples

### Phase 2: Add Backend Validation (Optional Safety Net)
- Validate signal_type in `holmesgpt-api`
- Log warnings for non-canonical types
- Normalize minor variations (case, typos)

### Phase 3: Monitor and Iterate
- Track LLM signal_type selections
- Identify common errors/variations
- Refine taxonomy and validation rules

---

## Implementation Effort (v1.1)

**Total**: 3-4 hours

**Breakdown**:
1. Compile complete list from K8s docs: 1 hour
2. Update ADR-041 and recovery.py: 1 hour
3. Add backend validation (optional): 1 hour
4. Update tests and documentation: 1 hour

---

## Success Metrics (v1.1)

- **Taxonomy Coverage**: 100% of common Kubernetes event reasons
- **LLM Selection Accuracy**: >95% of signal types are canonical
- **Workflow Match Rate**: Improved by 10-20% vs. v1.0
- **False Positive Rate**: <5% incorrect normalizations

---

## Open Questions for v1.1 Review

1. **Is taxonomy needed?**: Monitor v1.0 to see if LLM actually makes errors
2. **Prompt length concern?**: Does adding ~200 lines impact LLM performance?
3. **MCP tool vs. prompt?**: Which approach is more maintainable long-term?
4. **Validation strictness?**: Should we reject non-canonical types or normalize them?

---

## Related Decisions

- **Builds On**: [ADR-041 v3.3](./ADR-041-llm-prompt-response-contract.md) - LLM Prompt/Response Contract
- **Related**: [DD-LLM-001](./DD-LLM-001-mcp-search-taxonomy.md) - MCP Search Taxonomy
- **Supports**: BR-WORKFLOW-001 - LLM must select appropriate workflows

---

## Review & Evolution

### When to Revisit
- After v1.0 production deployment (monitor LLM signal_type selections)
- If workflow match rate is <80%
- If users report incorrect workflow selections
- If Kubernetes adds significant new event reasons

### Decision Criteria for v1.1
- **Implement if**: v1.0 shows >10% non-canonical signal types
- **Skip if**: v1.0 shows <5% issues and semantic search handles variations well
- **Enhance if**: Users request more deterministic behavior

---

## Changelog

### v1.0 (2025-11-16)
- Initial DD created
- Deferred to v1.1 pending v1.0 production feedback
- Documented three alternative approaches
- Recommended hybrid approach for v1.1


