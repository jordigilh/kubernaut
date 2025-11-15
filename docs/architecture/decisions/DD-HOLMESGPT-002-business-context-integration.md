# DD-HOLMESGPT-002: Business Context Integration

**Status**: Approved  
**Version**: 1.0  
**Created**: 2025-11-15  
**Target Release**: v1.1  
**Service**: HolmesGPT API  
**Related**: BR-AI-057, DD-PLAYBOOK-001

---

## Decision

HolmesGPT API SHALL accept and utilize business context fields in the recovery analysis prompt to enable business-aware LLM recommendations.

---

## Context

**Problem**: LLM recommendations are purely technical and ignore organizational business policies.

**Example**:
```json
{
  "context": {
    "namespace": "cost-management",
    "priority": "P1"
  }
}
```

LLM doesn't know that "cost-management" namespace requires cost-optimization priority.

---

## Decision Details

### Input Format

Accept enriched business context from AI Analysis Service:

```json
{
  "incident_id": "inc-001",
  "context": {
    "namespace": "cost-management",
    "priority": "P1",
    "business_category": "cost-management",
    "cost_sensitivity": "HIGH",
    "sla_tier": "STANDARD"
  }
}
```

### Prompt Enhancement

Add business context section to investigation prompt:

```python
## Business Context
- **Business Category**: {business_category}
- **Cost Sensitivity**: {cost_sensitivity}
- **SLA Tier**: {sla_tier}
- **Priority**: {priority}

**Business Constraints**:
When formulating recovery strategies, consider the following organizational policies:
- Cost-sensitive namespaces should prioritize optimization over resource scaling
- Resource increases may require approval based on business category
- SLA requirements vary by tier (CRITICAL > HIGH > STANDARD)
```

### Implementation Location

File: `holmesgpt-api/src/extensions/recovery.py`
Function: `_create_investigation_prompt()`

---

## Consequences

**Positive**:
- LLM recommendations align with business policies
- Reduced manual intervention
- Cost-aware remediation

**Negative**:
- Prompt complexity increases
- Dependency on AI Analysis Service enrichment

---

**Approved**: 2025-11-15  
**Implemented**: v1.1
