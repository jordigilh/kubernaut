# Signal Processing Service - Business Requirements

**Version**: 1.2
**Last Updated**: 2025-12-06
**Status**: ✅ APPROVED
**Owner**: SignalProcessing Team
**Related**: [IMPLEMENTATION_PLAN_V1.25.md](IMPLEMENTATION_PLAN_V1.25.md)

---

## Overview

This document defines all business requirements for the SignalProcessing CRD Controller service. These requirements drive the implementation plan and test coverage matrix.

**Requirement Format**: `BR-SP-XXX` where:
- `BR` = Business Requirement
- `SP` = Signal Processing service
- `XXX` = Sequential number (grouped by category)

---

## Requirement Categories

| Range | Category | Description |
|-------|----------|-------------|
| 001-010 | Core Enrichment | K8s context fetching, classification, recovery |
| 011-050 | Reserved | Future core requirements |
| 051-060 | Environment Classification | Namespace-based environment detection |
| 061-069 | Reserved | Future classification requirements |
| 070-079 | Priority Assignment | Rego-based priority engine |
| 080-089 | Business Classification | Confidence scoring, multi-dimensional |
| 090-099 | Audit & Observability | Audit trail, metrics, logging |
| 100-109 | Label Detection | DD-WORKFLOW-001 v2.2 label schema |

---

## Core Enrichment Requirements (BR-SP-001 to BR-SP-012)

### BR-SP-001: Kubernetes Context Enrichment

**Priority**: P0 (Critical)
**Category**: Core Enrichment

**Description**: The SignalProcessing controller MUST fetch and enrich signals with Kubernetes context within 2 seconds P95.

**Acceptance Criteria**:
- [ ] Fetch Pod details (name, phase, labels, annotations, containers, restart count)
- [ ] Fetch Namespace details (name, labels)
- [ ] Fetch Node details (name, labels, capacity, allocatable, conditions)
- [ ] Fetch Owner chain (Pod → ReplicaSet → Deployment/StatefulSet)
- [ ] Cache results with configurable TTL to reduce API load
- [ ] P95 latency < 2 seconds for complete enrichment

**SLO**: <2 seconds P95

**Test Coverage**: `enricher_test.go` (Unit + Integration)

**References**:
- [DD-017: K8s Enrichment Depth Strategy](../../../architecture/decisions/DD-017-k8s-enrichment-depth-strategy.md)
- [ADR-041: Rego Policy Data Fetching Separation](../../../architecture/decisions/ADR-041-rego-policy-data-fetching-separation.md)

---

### BR-SP-002: Business Classification

**Priority**: P0 (Critical)
**Category**: Core Enrichment

**Description**: The SignalProcessing controller MUST classify signals by business criticality using multi-dimensional categorization.

**Acceptance Criteria**:
- [ ] Classify by business unit (from namespace labels or Rego policies)
- [ ] Classify by service owner (from deployment labels or Rego policies)
- [ ] Classify by criticality level (critical, high, medium, low)
- [ ] Classify by SLA tier (platinum, gold, silver, bronze)
- [ ] Provide confidence score (0.0-1.0) for each classification

**Test Coverage**: `business_classifier_test.go` (Unit)

---

### BR-SP-003: Recovery Context Integration

**Priority**: P1 (High)
**Category**: Core Enrichment

**Description**: The SignalProcessing controller MUST embed recovery data from failed WorkflowExecution when processing recovery signals.

**Acceptance Criteria**:
- [ ] Detect recovery signals via `spec.failureData` field presence
- [ ] Extract previous workflow execution reference
- [ ] Extract failure reason and error details
- [ ] Include recovery attempt count
- [ ] Pass recovery context to AIAnalysis for workflow selection

**Test Coverage**: `reconciler_test.go` (Integration)

**References**:
- [DD-001: Recovery Context Enrichment](../../../architecture/decisions/DD-001-recovery-context-enrichment.md)

---

### BR-SP-006: Rule-Based Filtering

**Priority**: P2 (Medium)
**Category**: Core Enrichment

**Description**: The SignalProcessing controller MUST implement rule-based filtering with complex conditions using Rego policies.

**Acceptance Criteria**:
- [ ] Load filtering rules from ConfigMap
- [ ] Support AND/OR conditions on signal fields
- [ ] Support namespace-based filtering
- [ ] Support label-based filtering
- [ ] Hot-reload rules without restart

**Test Coverage**: `rego_engine_test.go` (Unit + Integration)

---

### BR-SP-012: Historical Action Context

**Priority**: P2 (Medium)
**Category**: Core Enrichment

**Description**: The SignalProcessing controller MUST add historical action context to signals via the deduplication system.

**Acceptance Criteria**:
- [ ] Track first occurrence timestamp
- [ ] Track last occurrence timestamp
- [ ] Track occurrence count
- [ ] Reference previous RemediationRequest if duplicate
- [ ] Include correlation ID for related signals

**Test Coverage**: `reconciler_test.go` (Integration)

**References**:
- [Shared DeduplicationInfo Type](../../../../pkg/shared/types/deduplication.go)

---

## Environment Classification Requirements (BR-SP-051 to BR-SP-053)

### BR-SP-051: Environment Classification (Primary)

**Priority**: P0 (Critical)
**Category**: Environment Classification

**Description**: The SignalProcessing controller MUST detect environment from namespace labels as the primary source.

**Acceptance Criteria**:
- [ ] Check `kubernaut.ai/environment` label on namespace (ONLY this label)
- [ ] Return environment value: `production`, `staging`, `development`, `test`
- [ ] Case-insensitive matching
- [ ] Confidence: 0.95 when label is present

**Detection**:
- `metadata.labels["kubernaut.ai/environment"]` (single authoritative source)

**Rationale**: Using only `kubernaut.ai/` prefixed labels prevents accidentally capturing labels from other systems and ensures clear ownership of environment classification.

**Test Coverage**: `environment_classifier_test.go` (Unit)

---

### BR-SP-052: Environment Classification (Fallback)

**Priority**: P1 (High)
**Category**: Environment Classification

**Description**: The SignalProcessing controller MUST fall back to ConfigMap-based environment mapping when namespace labels are absent.

**Acceptance Criteria**:
- [ ] Load environment mapping from `kubernaut-environment-config` ConfigMap
- [ ] Support namespace name → environment mapping
- [ ] Support namespace pattern → environment mapping (regex)
- [ ] Hot-reload mapping without restart
- [ ] Log when fallback is used

**ConfigMap Format**:
```yaml
data:
  mapping: |
    prod-*: production
    staging-*: staging
    dev-*: development
    test-*: test
```

**Test Coverage**: `environment_classifier_test.go` (Unit)

---

### BR-SP-053: Environment Classification (Default)

**Priority**: P1 (High)
**Category**: Environment Classification

**Description**: The SignalProcessing controller MUST use a default environment when all detection methods fail.

**Acceptance Criteria**:
- [ ] Default to `unknown` when no label or ConfigMap mapping found
- [ ] Log warning when default is used
- [ ] Never fail classification - always return a value
- [ ] Confidence: 0.0 for default (indicates no detection was possible)

**Rationale**: Using `unknown` is more accurate than assuming `development` - organizations have varied environment taxonomies and `development` could mean different things to different customers.

**Test Coverage**: `environment_classifier_test.go` (Unit)

---

## Priority Assignment Requirements (BR-SP-070 to BR-SP-072)

### BR-SP-070: Priority Assignment (Rego)

**Priority**: P0 (Critical)
**Category**: Priority Assignment

**Description**: The SignalProcessing controller MUST assign priority using Rego policies that consider K8s context and business classification.

**Acceptance Criteria**:
- [ ] Load priority policy from ConfigMap
- [ ] Evaluate Rego with K8s context as input
- [ ] Return priority: `P0` (critical), `P1` (high), `P2` (medium), `P3` (low)
- [ ] Support custom priority rules per namespace
- [ ] P95 evaluation latency < 100ms

**Rego Input Schema**:
```json
{
  "signal": { "severity": "critical", "source": "prometheus" },
  "environment": "production",
  "namespace_labels": { "tier": "critical" },
  "deployment_labels": { "app": "payment-service" }
}
```

**Test Coverage**: `priority_engine_test.go` (Unit)

---

### BR-SP-071: Priority Fallback Matrix

**Priority**: P1 (High)
**Category**: Priority Assignment

**Description**: The SignalProcessing controller MUST use a severity-based fallback when Rego policy fails or times out.

**Acceptance Criteria**:
- [ ] Fallback triggers on: Rego timeout (>100ms), policy error, missing policy
- [ ] Fallback based on signal severity ONLY (environment is not considered in fallback)
- [ ] Log when fallback is used
- [ ] Never fail - always return a valid priority

**Fallback Matrix** (Severity-Based Only):
| Severity | Priority | Rationale |
|----------|----------|-----------|
| critical | P1 | Conservative - high but not highest without context |
| warning | P2 | Standard priority for warnings |
| info | P3 | Lowest priority for informational |
| unknown | P2 | Default when severity is also unknown |

**Rationale**: When Rego policy fails, we don't have reliable environment classification. Using severity-only fallback is more predictable and avoids compounding uncertainty from potentially incorrect environment detection.

**Test Coverage**: `priority_engine_test.go` (Unit)

---

### BR-SP-072: Rego Hot-Reload

**Priority**: P1 (High)
**Category**: Priority Assignment

**Description**: The SignalProcessing controller MUST support hot-reload of Rego policies from ConfigMap changes without restart.

**Acceptance Criteria**:
- [ ] Watch mounted ConfigMap file for changes (via `fsnotify`)
- [ ] Re-compile policy on ConfigMap update
- [ ] Use mutex to prevent race conditions during reload
- [ ] Log policy version hash after reload
- [ ] Continue using old policy if new policy fails to compile

**Implementation**:
- `fsnotify`-based file watch on mounted ConfigMap (per [DD-INFRA-001](../../../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md))
- Uses shared `pkg/shared/hotreload/FileWatcher` component
- `sync.RWMutex` for policy access
- SHA256 hash for policy version tracking

**Deployment Requirement**:
ConfigMap must be mounted as a volume in the deployment spec:
```yaml
volumes:
- name: rego-policies
  configMap:
    name: kubernaut-rego-policies
volumeMounts:
- name: rego-policies
  mountPath: /etc/kubernaut/policies
```

**Test Coverage**:
- `pkg/shared/hotreload/file_watcher_test.go` (Unit) - 14 tests for FileWatcher
- `test/integration/signalprocessing/hot_reloader_test.go` (Integration) - 4 tests for policy hot-reload

**References**:
- [DD-INFRA-001: ConfigMap Hot-Reload Pattern](../../../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md)
- [NOTICE_SHARED_HOTRELOADER_PACKAGE.md](../../../handoff/NOTICE_SHARED_HOTRELOADER_PACKAGE.md)

---

## Business Classification Requirements (BR-SP-080 to BR-SP-081)

### BR-SP-080: Confidence Scoring

**Priority**: P1 (High)
**Category**: Business Classification

**Description**: The SignalProcessing controller MUST provide confidence scores (0.0-1.0) for all categorization decisions.

**Acceptance Criteria**:
- [ ] Confidence 1.0: Explicit label match (e.g., `environment=production`)
- [ ] Confidence 0.8: Pattern match (e.g., namespace `prod-*`)
- [ ] Confidence 0.6: Rego policy inference
- [ ] Confidence 0.4: Default fallback
- [ ] Include confidence in status for each classification field

**Test Coverage**: All classifier tests (Unit)

---

### BR-SP-081: Multi-dimensional Categorization

**Priority**: P1 (High)
**Category**: Business Classification

**Description**: The SignalProcessing controller MUST support multi-dimensional business categorization.

**Acceptance Criteria**:
- [ ] Dimension 1: Business Unit (from labels or Rego)
- [ ] Dimension 2: Service Owner (from labels or Rego)
- [ ] Dimension 3: Criticality (critical, high, medium, low)
- [ ] Dimension 4: SLA Tier (platinum, gold, silver, bronze)
- [ ] All dimensions optional - use "unknown" if not determinable

**Test Coverage**: `business_classifier_test.go` (Unit)

---

## Audit & Observability Requirements (BR-SP-090)

### BR-SP-090: Categorization Audit Trail

**Priority**: P1 (High)
**Category**: Audit & Observability

**Description**: The SignalProcessing controller MUST create audit events for all categorization decisions.

**Acceptance Criteria**:
- [ ] Audit event on signal processing start
- [ ] Audit event on enrichment complete (with context summary)
- [ ] Audit event on classification complete (with all dimensions)
- [ ] Audit event on processing complete or failed
- [ ] Use async buffered writes (ADR-038) - never block reconciliation
- [ ] Include correlation ID for tracing

**Audit Event Fields**:
```json
{
  "event_type": "signalprocessing.classified",
  "signal_id": "sp-123",
  "correlation_id": "corr-456",
  "environment": "production",
  "priority": "P1",
  "confidence": 0.95,
  "policy_version": "sha256:abc123",
  "duration_ms": 150
}
```

**Test Coverage**: `audit_client_test.go` (Unit + Integration)

**References**:
- [ADR-038: Async Buffered Audit Ingestion](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)

---

## Label Detection Requirements (BR-SP-100 to BR-SP-104)

> **Reference**: [DD-WORKFLOW-001 v2.2](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)

### BR-SP-100: OwnerChain Traversal

**Priority**: P0 (Critical)
**Category**: Label Detection

**Description**: The SignalProcessing controller MUST traverse K8s ownerReferences to build the complete ownership chain.

**Acceptance Criteria**:
- [ ] Start from signal source resource (usually Pod)
- [ ] Follow `metadata.ownerReferences` to parent resources
- [ ] Build chain: Pod → ReplicaSet → Deployment/StatefulSet/DaemonSet
- [ ] Stop at cluster-scoped resources or max depth (5)
- [ ] Include namespace, kind, name for each entry
- [ ] Empty chain for orphan resources (no owners)

**Output Schema**:
```json
{
  "ownerChain": [
    {"namespace": "prod", "kind": "ReplicaSet", "name": "api-7d8f9c6b5"},
    {"namespace": "prod", "kind": "Deployment", "name": "api"}
  ]
}
```

**Test Coverage**: `ownerchain_builder_test.go` (Unit + Integration)

---

### BR-SP-101: DetectedLabels Auto-Detection

**Priority**: P0 (Critical)
**Category**: Label Detection

**Description**: The SignalProcessing controller MUST auto-detect 8 cluster characteristics from K8s resources.

**Acceptance Criteria**:
- [ ] **gitOpsManaged**: Check ArgoCD/Flux annotations on Deployment
- [ ] **gitOpsTool**: Return "argocd" or "flux" based on annotations
- [ ] **pdbProtected**: List PDBs, check if selector matches Pod labels
- [ ] **hpaEnabled**: List HPAs, check if scaleTargetRef matches Deployment
- [ ] **stateful**: Check if owner chain includes StatefulSet
- [ ] **helmManaged**: Check `app.kubernetes.io/managed-by: Helm` label
- [ ] **networkIsolated**: List NetworkPolicies, check if podSelector matches
- [ ] **serviceMesh**: Check for Istio/Linkerd sidecar or annotations

**Detection Methods**:
| Field | API Call | Cache TTL (V1.1) |
|-------|----------|------------------|
| gitOpsManaged | None (existing data) | N/A |
| gitOpsTool | None (existing data) | N/A |
| pdbProtected | List PDBs | 5 min *(deferred)* |
| hpaEnabled | List HPAs | 1 min *(deferred)* |
| stateful | None (owner chain) | N/A |
| helmManaged | None (existing data) | N/A |
| networkIsolated | List NetworkPolicies | 5 min *(deferred)* |
| serviceMesh | None (existing data) | N/A |

> **Note**: Cache TTL is deferred to V1.1. V1.0 performs fresh queries on each reconciliation.
> This is acceptable for P0 release because:
> - SignalProcessing reconciles per-signal (not batch)
> - K8s API is local (controller runs in-cluster)
> - Performance optimization is V1.1 scope

**Test Coverage**: `label_detector_test.go` (Unit + Integration)

---

### BR-SP-102: CustomLabels Rego Extraction

**Priority**: P1 (High)
**Category**: Label Detection

**Description**: The SignalProcessing controller MUST extract custom labels using customer-defined Rego policies.

**Acceptance Criteria**:
- [ ] Load custom label policies from ConfigMap
- [ ] Execute Rego with K8s context as input
- [ ] Output: `map[string][]string` (subdomain → label values)
- [ ] Validate against limits: max 10 keys, 5 values/key, 63 char keys, 100 char values
- [ ] Strip any attempts to override mandatory labels (security wrapper)
- [ ] Sandboxed execution: no network, no filesystem, 5s timeout, 128MB memory

**Example Output**:
```json
{
  "customLabels": {
    "constraint": ["cost-constrained", "stateful-safe"],
    "team": ["name=payments"],
    "region": ["us-east-1"]
  }
}
```

**Test Coverage**: `rego_engine_test.go` (Unit + Integration)

**References**:
- [DD-WORKFLOW-001 v2.2 - CustomLabels](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)

---

### BR-SP-103: FailedDetections Tracking

**Priority**: P1 (High)
**Category**: Label Detection

**Description**: The SignalProcessing controller MUST track which label detections failed due to query errors.

**Acceptance Criteria**:
- [ ] Track failed detections in `FailedDetections []string` array
- [ ] Include field name in array when query fails (RBAC denied, timeout, network error)
- [ ] Do NOT include field when resource simply doesn't exist (valid false)
- [ ] Log ERROR for query failures
- [ ] Do NOT log error for "resource not found" (valid negative)

**Scenario Table**:
| Scenario | Field Value | FailedDetections | Log Level |
|----------|-------------|------------------|-----------|
| PDB exists | `true` | `[]` | Info |
| No PDB | `false` | `[]` | Info |
| RBAC denied | `false` | `["pdbProtected"]` | Error |

**Test Coverage**: `label_detector_test.go` (Unit)

---

### BR-SP-104: Mandatory Label Protection

**Priority**: P0 (Critical)
**Category**: Label Detection

**Description**: The SignalProcessing controller MUST prevent customer Rego policies from overriding mandatory system labels.

**Acceptance Criteria**:
- [ ] Security wrapper strips these keys from Rego output:
  - `environment`
  - `priority`
  - `component`
  - `signal_type`
  - `severity`
- [ ] Log warning when stripped keys are detected
- [ ] Never allow customer code to set mandatory labels

**Test Coverage**: `rego_security_wrapper_test.go` (Unit)

**References**:
- [DD-WORKFLOW-001 v2.2 - Mandatory Labels](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)

---

## Requirements Summary

| Category | Count | Priority Breakdown |
|----------|-------|-------------------|
| Core Enrichment | 5 | P0: 2, P1: 1, P2: 2 |
| Environment Classification | 3 | P0: 1, P1: 2 |
| Priority Assignment | 3 | P0: 1, P1: 2 |
| Business Classification | 2 | P1: 2 |
| Audit & Observability | 1 | P1: 1 |
| Label Detection | 5 | P0: 3, P1: 2 |
| **Total** | **19** | **P0: 7, P1: 9, P2: 3** |

---

## Traceability Matrix

| BR ID | Implementation Plan Section | Test File | Status |
|-------|----------------------------|-----------|--------|
| BR-SP-001 | Day 3: K8s Enricher | `enricher_test.go` | ✅ Planned |
| BR-SP-002 | Day 6: Business Classifier | `business_classifier_test.go` | ✅ Planned |
| BR-SP-003 | Day 10: Reconciler | `reconciler_test.go` | ✅ Planned |
| BR-SP-006 | Day 9: Rego Engine | `rego_engine_test.go` | ✅ Planned |
| BR-SP-012 | Day 10: Reconciler | `reconciler_test.go` | ✅ Planned |
| BR-SP-051 | Day 4: Environment Classifier | `environment_classifier_test.go` | ✅ Planned |
| BR-SP-052 | Day 4: Environment Classifier | `environment_classifier_test.go` | ✅ Planned |
| BR-SP-053 | Day 4: Environment Classifier | `environment_classifier_test.go` | ✅ Planned |
| BR-SP-070 | Day 5: Priority Engine | `priority_engine_test.go` | ✅ Planned |
| BR-SP-071 | Day 5: Priority Engine | `priority_engine_test.go` | ✅ Planned |
| BR-SP-072 | Day 5: Hot Reloader | `hot_reloader_test.go` | ✅ Planned |
| BR-SP-080 | Day 6: Business Classifier | All classifier tests | ✅ Planned |
| BR-SP-081 | Day 6: Business Classifier | `business_classifier_test.go` | ✅ Planned |
| BR-SP-090 | Day 11: Audit Client | `audit_client_test.go` | ✅ Planned |
| BR-SP-100 | Day 7: OwnerChain | `ownerchain_builder_test.go` | ✅ Planned |
| BR-SP-101 | Day 8: DetectedLabels | `label_detector_test.go` | ✅ Planned |
| BR-SP-102 | Day 9: CustomLabels | `rego_engine_test.go` | ✅ Planned |
| BR-SP-103 | Day 8: FailedDetections | `label_detector_test.go` | ✅ Planned |
| BR-SP-104 | Day 9: Security Wrapper | `rego_security_wrapper_test.go` | ✅ Planned |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2 | 2025-12-06 | BR-SP-071: Changed to severity-only fallback (not environment × severity matrix) |
| 1.1 | 2025-12-06 | BR-SP-072: Updated to fsnotify-based hot-reload per DD-INFRA-001, added FileWatcher reference |
| 1.0 | 2025-12-03 | Initial release - 19 BRs defined |

