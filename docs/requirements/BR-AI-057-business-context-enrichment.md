# BR-AI-057: Business Context Enrichment for Recovery Analysis

**Status**: Approved
**Version**: 1.0
**Created**: 2025-11-15
**Target Release**: v1.1
**Service**: AI Analysis Service
**Related**: DD-HOLMESGPT-002, BR-HAPI-001, BR-PLAYBOOK-001

---

## Business Requirement

The AI Analysis Service SHALL enrich incident context with organizational business policies and constraints to enable business-aware recovery recommendations from the HolmesGPT API.

---

## Rationale

**Problem**:
- Current recovery analysis lacks organizational business context
- LLM recommendations are purely technical, ignoring business policies
- Cost-sensitive namespaces need different remediation approaches than critical services
- Approval requirements and risk tolerance vary by business category

**Example**:
- Incident in `cost-management` namespace should prioritize optimization over resource increases
- Incident in `payment-service` namespace should prioritize availability over cost
- Current system treats both identically

**Business Impact**:
- Inappropriate recommendations violate organizational policies
- Cost overruns from unnecessary resource scaling
- SLA violations from overly conservative approaches
- Manual intervention required to apply business context

---

## Functional Requirements

### FR-AI-057-001: Business Policy Retrieval
The AI Analysis Service SHALL retrieve business policies for the affected namespace/application when analyzing an incident.

**Policy Sources**:
1. Namespace labels (immediate - v1.1)
2. ConfigMap/CRD (future - v1.2)
3. External policy service (future - v2.0)

### FR-AI-057-002: Business Context Fields
The AI Analysis Service SHALL provide the following business context fields to HolmesGPT API:

| Field | Type | Source | Example Values |
|-------|------|--------|----------------|
| `business_category` | string | Namespace label: `kubernaut.ai/business-category` | `cost-management`, `payment-service`, `analytics` |
| `cost_sensitivity` | enum | Namespace label: `kubernaut.ai/cost-sensitivity` | `HIGH`, `MEDIUM`, `LOW` |
| `approval_required_for` | array | Policy ConfigMap | `["resource_increases", "scaling_up", "node_changes"]` |
| `preferred_approaches` | array | Policy ConfigMap | `["optimization", "scale_down", "resource_increase"]` |
| `sla_tier` | enum | Namespace label: `kubernaut.ai/sla-tier` | `CRITICAL`, `HIGH`, `STANDARD` |

### FR-AI-057-003: Context Enrichment
The AI Analysis Service SHALL enrich the recovery request context before sending to HolmesGPT API:

**Input** (from incident):
```json
{
  "incident_id": "inc-001",
  "failed_action": {...},
  "context": {
    "namespace": "cost-management",
    "priority": "P1"
  }
}
```

**Output** (enriched):
```json
{
  "incident_id": "inc-001",
  "failed_action": {...},
  "context": {
    "namespace": "cost-management",
    "priority": "P1",
    "business_category": "cost-management",
    "cost_sensitivity": "HIGH",
    "approval_required_for": ["resource_increases", "scaling_up"],
    "preferred_approaches": ["optimization", "scale_down", "resource_increase"],
    "sla_tier": "STANDARD"
  }
}
```

### FR-AI-057-004: Fallback Behavior
The AI Analysis Service SHALL provide default business context when namespace-specific policies are not available:

**Defaults**:
```yaml
business_category: "general"
cost_sensitivity: "MEDIUM"
approval_required_for: []
preferred_approaches: ["investigate", "remediate", "escalate"]
sla_tier: "STANDARD"
```

---

## Non-Functional Requirements

### NFR-AI-057-001: Performance
- Business policy retrieval SHALL complete within 100ms
- SHALL NOT block incident analysis pipeline
- SHALL cache namespace policies (TTL: 5 minutes)

### NFR-AI-057-002: Reliability
- Policy retrieval failure SHALL NOT prevent incident analysis
- SHALL fall back to defaults on policy service unavailability
- SHALL log policy retrieval errors for monitoring

### NFR-AI-057-003: Observability
- SHALL emit metrics: `business_context_enrichment_duration_ms`
- SHALL emit metrics: `business_context_cache_hit_ratio`
- SHALL log business context applied to each incident

---

## Implementation Phases

### Phase 1: v1.1 (Namespace Labels)
**Scope**: Read business context from namespace labels
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: cost-management
  labels:
    kubernaut.ai/business-category: "cost-management"
    kubernaut.ai/cost-sensitivity: "HIGH"
    kubernaut.ai/sla-tier: "STANDARD"
```

**Implementation**:
- Add namespace label reader to AI Analysis Service
- Enrich context before HolmesGPT API call
- Use defaults for unlabeled namespaces

### Phase 2: v1.2 (Policy ConfigMap)
**Scope**: Read detailed policies from ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: business-policies
  namespace: kubernaut-system
data:
  cost-management.yaml: |
    business_category: cost-management
    cost_sensitivity: HIGH
    approval_required_for:
      - resource_increases
      - scaling_up
    preferred_approaches:
      - optimization
      - scale_down
      - resource_increase
```

### Phase 3: v2.0 (External Policy Service)
**Scope**: Integration with enterprise policy management system
- Query external API for business policies
- Support dynamic policy updates
- Multi-cluster policy consistency

---

## Acceptance Criteria

### AC-AI-057-001: Namespace Label Reading
```gherkin
Given a namespace "cost-management" with label "kubernaut.ai/business-category=cost-management"
When an incident occurs in that namespace
Then the recovery request context SHALL include "business_category": "cost-management"
```

### AC-AI-057-002: Default Fallback
```gherkin
Given a namespace "test-app" without business labels
When an incident occurs in that namespace
Then the recovery request context SHALL include default business context
And the incident analysis SHALL proceed normally
```

### AC-AI-057-003: Performance
```gherkin
Given 100 concurrent incidents across different namespaces
When business context enrichment is performed
Then 95th percentile enrichment time SHALL be < 100ms
```

### AC-AI-057-004: Cache Effectiveness
```gherkin
Given a namespace policy has been retrieved
When another incident occurs in the same namespace within 5 minutes
Then the cached policy SHALL be used
And no additional Kubernetes API call SHALL be made
```

---

## Dependencies

**Upstream**:
- Kubernetes API access for namespace label reading
- RBAC permissions: `get`, `list` on `namespaces`

**Downstream**:
- HolmesGPT API must accept enriched business context (DD-HOLMESGPT-002)
- Playbook catalog must support business context matching (BR-PLAYBOOK-001)

---

## Testing Strategy

### Unit Tests
- Namespace label parsing
- Default value application
- Cache behavior
- Error handling

### Integration Tests
- End-to-end context enrichment
- HolmesGPT API integration
- Policy ConfigMap reading (Phase 2)

### Performance Tests
- Context enrichment latency
- Cache hit ratio under load
- Concurrent request handling

---

## Monitoring & Alerts

**Metrics**:
```
business_context_enrichment_duration_ms{namespace, result}
business_context_cache_hit_ratio
business_context_enrichment_errors_total{error_type}
business_context_default_used_total{namespace}
```

**Alerts**:
- `BusinessContextEnrichmentSlow`: p95 > 200ms for 5 minutes
- `BusinessContextEnrichmentFailureRate`: error rate > 5% for 5 minutes
- `BusinessContextCacheMissRate`: cache miss rate > 50% for 10 minutes

---

## Documentation Requirements

- Update AI Analysis Service API documentation
- Document namespace labeling conventions
- Provide policy ConfigMap examples
- Update runbook for policy management

---

## Related Documents

- [DD-HOLMESGPT-002](../architecture/decisions/DD-HOLMESGPT-002-business-context-integration.md) - HolmesGPT API integration
- [BR-PLAYBOOK-001](./BR-PLAYBOOK-001-playbook-catalog-integration.md) - Playbook catalog integration
- [DD-PLAYBOOK-001](../architecture/decisions/DD-PLAYBOOK-001-mandatory-label-schema.md) - Label schema

---

**Approval**: Required
**Approver**: Product Owner, Technical Lead
**Review Date**: 2025-11-15


