# ADR-055 Addendum 001: AffectedResource → RemediationTarget Rename

**Status**: Accepted  
**Date**: 2026-03-04  
**Issue**: #542  
**Parent ADR**: ADR-055 (LLM-Driven Context Enrichment)

## Context

Issue #542 identified that the field name `affectedResource` was semantically ambiguous.
The LLM consistently interpreted "affected" as "the resource experiencing the symptom"
(e.g., the Deployment whose Pods are failing) rather than "the resource whose spec must
change to fix the problem" (e.g., the Node that needs a cordon/drain).

## Decision

Rename `affectedResource` to `remediationTarget` across the entire stack for end-to-end
consistency and to provide clearer semantic guidance to the LLM.

## Blast Radius

### Layer 1: HAPI (Python) — PR #543
| Component | Change |
|-----------|--------|
| `prompt_builder.py` | All prompt examples and JSON blocks: `affectedResource` → `remediationTarget` |
| `result_parser.py` | Function rename: `_parse_affected_resource` → `_parse_remediation_target` |
| `llm_integration.py` | Variables, dict keys, imports updated |
| `enrichment_service.py` | Parameter rename: `affected_resource` → `remediation_target` |
| Mock LLM `server.py` | Output JSON key: `affectedResource` → `remediationTarget` |
| 12 Python test files | Test data, assertions, imports updated |

### Layer 2: Go CRD & Consumers — This PR
| Component | Change | Risk |
|-----------|--------|------|
| **CRD API type** (`api/aianalysis/v1alpha1/aianalysis_types.go`) | `type AffectedResource struct` → `type RemediationTarget struct` | **CRD schema migration** |
| **CRD field** | `AffectedResource *AffectedResource` → `RemediationTarget *RemediationTarget` | All Go consumers updated via `gopls rename` |
| **CRD JSON tag** | `json:"affectedResource,omitempty"` → `json:"remediationTarget,omitempty"` | **Breaking change**: existing `AIAnalysis` CRDs in etcd use the old key |
| **Rego input type** (`pkg/aianalysis/rego/evaluator.go`) | `AffectedResourceInput` → `RemediationTargetInput` | Type-safe rename |
| **Rego PolicyInput field** | `AffectedResource` → `RemediationTarget`, JSON tag `affected_resource` → `remediation_target` | |
| **Rego map key** | `"affected_resource"` → `"remediation_target"` | **Breaking change** for custom Rego policies |
| **Handler function** | `HandleAffectedResourceMissing` → `HandleRemediationTargetMissing` | |
| **Reason codes** | `"AffectedResourceMissing"` → `"RemediationTargetMissing"` | May affect external monitoring |
| **Response processor** | Reads `rcaMap["remediationTarget"]` (was already updated in Layer 1) | |
| **CRD manifests** | Regenerated via `make manifests` | Field name change in YAML spec |
| **Deepcopy** | Regenerated via `make generate` | |
| **~30 Go test files** | Comments, test descriptions, assertion messages updated | |

### Layer 3: Rego Policies
| File | Change |
|------|--------|
| `test/unit/aianalysis/testdata/policies/approval.rego` | `input.affected_resource` → `input.remediation_target`, `has_affected_resource` → `has_remediation_target` |
| `test/integration/aianalysis/testdata/policies/approval.rego` | Same as above, plus `is_sensitive_resource` rules updated |
| Risk factor message | `"Missing affected resource..."` → `"Missing remediation target..."` |

### Not Changed (Different Concepts)
| Component | Reason |
|-----------|--------|
| `AffectedResources []string` on `RemediationRequest` | Different concept: storm aggregation field (plural) |
| `AffectedResourceKind/Name/Namespace` in data-storage OAS client | Generated code from OpenAPI spec; audit event payload fields |
| `affected_resources` in HolmesGPT OAS client | HolmesGPT API field for incident context |

## Upgrade Notes

### CRD Schema Migration
Existing `AIAnalysis` CRDs stored in etcd will have the old `affectedResource` JSON key.
When the new CRD manifest is applied, Kubernetes will:
1. Accept the new schema (the old field simply won't be populated on read)
2. New `AIAnalysis` objects will use `remediationTarget`
3. Existing objects will lose the `affectedResource` data on the next reconcile write

**Mitigation**: This is acceptable because `AIAnalysis` objects are ephemeral (created per
incident, garbage-collected after remediation). No long-lived data is lost.

### Custom Rego Policies
Operators with custom approval policies referencing `input.affected_resource` must update
to `input.remediation_target`. The bundled policies are updated in this change.

### Monitoring / Alerting
The reason code `AffectedResourceMissing` has been renamed to `RemediationTargetMissing`.
Any alerts or dashboards filtering on this reason code must be updated.

## Consequences

- Clearer semantic guidance to the LLM about which resource to select
- End-to-end consistency from HAPI prompts through CRD storage to Rego evaluation
- One-time upgrade cost for custom Rego policies and monitoring rules
