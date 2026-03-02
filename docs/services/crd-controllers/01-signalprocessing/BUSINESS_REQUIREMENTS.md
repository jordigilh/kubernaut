# Signal Processing Service - Business Requirements

**Version**: 1.3
**Last Updated**: 2026-01-09
**Status**: ✅ APPROVED
**Owner**: SignalProcessing Team
**Related**: [IMPLEMENTATION_PLAN_V1.25.md](IMPLEMENTATION_PLAN_V1.25.md), [DD-SEVERITY-001](../../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)

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
| 080-089 | Business Classification | Source tracking, multi-dimensional |
| 090-099 | Audit & Observability | Audit trail, metrics, logging |
| 100-109 | Label Detection | DD-WORKFLOW-001 v2.2 label schema |
| 110-119 | Observability | K8s Conditions (110), Shared Backoff (111) |

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
**Version**: 2.0 (Updated 2025-12-14 per DD-SP-001)

**Description**: The SignalProcessing controller MUST classify signals by business criticality using multi-dimensional categorization.

**Acceptance Criteria**:
- [ ] Classify by business unit (from namespace labels or Rego policies)
- [ ] Classify by service owner (from deployment labels or Rego policies)
- [ ] Classify by criticality level (critical, high, medium, low)
- [ ] Classify by SLA tier (platinum, gold, silver, bronze)
- [ ] ~~Provide confidence score (0.0-1.0) for each classification~~ **[REMOVED per DD-SP-001 V1.1]**

**Breaking Change**: Removed `OverallConfidence` field from `BusinessClassification` (pre-release, no backwards compatibility impact).

**Test Coverage**: `business_classifier_test.go` (Unit)

**References**:
- [DD-SP-001: Remove Classification Confidence Scores](../../../architecture/decisions/DD-SP-001-remove-classification-confidence-scores.md)

**Changelog**:
- **V2.0** (2025-12-14): Removed confidence score requirement per DD-SP-001 V1.1
- **V1.0** (Initial): Confidence-based approach (deprecated)

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
**Status**: 🔴 **DEPRECATED** (2025-12-19)

**Description**: ~~The SignalProcessing controller MUST implement rule-based filtering with complex conditions using Rego policies.~~

**Deprecation Rationale**:
Per SP BR Coverage Triage ([TRIAGE_SP_BR_COVERAGE_DEC_19_2025.md](../../../handoff/TRIAGE_SP_BR_COVERAGE_DEC_19_2025.md)):

1. **Wrong Layer**: Gateway is the filtering layer - decides whether to create CRDs at all
2. **No Terminal Phase**: SP controller has no "Filtered" phase - all signals go through full pipeline
3. **Rego Already Used**: BR-SP-070 uses Rego for priority assignment (can assign P3 to "unimportant" signals)
4. **Architectural Mismatch**: Filtering should prevent CRD creation, not process then discard

**Original Acceptance Criteria** (NOT IMPLEMENTED):
- ~~Load filtering rules from ConfigMap~~
- ~~Support AND/OR conditions on signal fields~~
- ~~Support namespace-based filtering~~
- ~~Support label-based filtering~~
- ~~Hot-reload rules without restart~~

**Alternative**: If filtering is needed, implement at Gateway level before SignalProcessing CRD creation.

**Test Coverage**: N/A (deprecated)

---

### BR-SP-012: Historical Action Context

**Priority**: P2 (Medium)
**Category**: Core Enrichment
**Status**: 🔴 **DEPRECATED** (2025-12-19)

**Description**: ~~The SignalProcessing controller MUST add historical action context to signals via the deduplication system.~~

**Deprecation Rationale**:
Per Gateway team review ([REQUEST_DEDUPLICATION_CRD_VISIBILITY_V1.1_DEC_19_2025.md](../../../handoff/REQUEST_DEDUPLICATION_CRD_VISIBILITY_V1.1_DEC_19_2025.md)):

1. **Wrong Layer**: Gateway owns deduplication via DD-GATEWAY-011; data lives in `RemediationRequest.status.deduplication`
2. **No Functional Use**: SP controller doesn't use deduplication for classification/categorization
3. **Operator Access**: Deduplication already visible in RR (source of truth)
4. **Architectural Violation**: SP cannot cross-reference parent CRD status per K8s best practices

**Original Acceptance Criteria** (NOT IMPLEMENTED):
- ~~Track first occurrence timestamp~~
- ~~Track last occurrence timestamp~~
- ~~Track occurrence count~~
- ~~Reference previous RemediationRequest if duplicate~~
- ~~Include correlation ID for related signals~~

**Alternative**: Operators can access deduplication via:
```bash
kubectl get rr <name> -o jsonpath='{.status.deduplication}' | jq
```

**References**:
- [Deprecation Decision](../../../handoff/REQUEST_DEDUPLICATION_CRD_VISIBILITY_V1.1_DEC_19_2025.md)
- [DD-GATEWAY-011: Deduplication Ownership](../../../architecture/decisions/DD-GATEWAY-011-status-deduplication-refactoring.md)

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

### BR-SP-052: Environment Classification (Fallback) ⚠️ DEPRECATED

**Priority**: P1 (High)
**Category**: Environment Classification
**Status**: ⚠️ **DEPRECATED** (2025-12-20)

> **Deprecation Notice**: Go-level ConfigMap fallback has been removed. Operators can implement namespace pattern matching directly in their Rego policies if needed. This gives operators full control over fallback behavior.
>
> **Migration**: Implement pattern matching in your `environment.rego` policy using Rego functions.
> See: `deploy/signalprocessing/policies/environment.rego` for examples.

**Original Description**: The SignalProcessing controller MUST fall back to ConfigMap-based environment mapping when namespace labels are absent.

**Original Acceptance Criteria** (Superseded):
- [x] ~~Load environment mapping from ConfigMap~~ → Implement in Rego if needed
- [x] ~~Support namespace name → environment mapping~~ → Implement in Rego
- [x] ~~Support namespace pattern → environment mapping~~ → Use Rego `startswith()`, `endswith()`
- [x] Hot-reload mapping without restart → Rego hot-reload via BR-SP-072
- [x] Log when fallback is used → Logged via Rego policy `source` field

**Rego Migration Example**:
```rego
# Namespace pattern matching in Rego (replaces ConfigMap fallback)
result := {"environment": "production", "source": "namespace-pattern"} if {
    startswith(input.namespace.name, "prod-")
}
result := {"environment": "staging", "source": "namespace-pattern"} if {
    startswith(input.namespace.name, "staging-")
}
```

**Test Coverage**: `environment_classifier_test.go` (Unit) - Updated for Rego-only classification

---

### BR-SP-053: Environment Classification (Default) ⚠️ DEPRECATED

**Priority**: P1 (High)
**Category**: Environment Classification
**Status**: ⚠️ **DEPRECATED** (2025-12-20)

> **Deprecation Notice**: Go-level hardcoded defaults have been removed. Operators now define their own default values using the Rego `default` keyword in their environment.rego policy. This gives operators full control over what "default" means for their organization.
>
> **Migration**: Add a `default result := {...}` rule to your `environment.rego` policy.
> See: `deploy/signalprocessing/policies/environment.rego`

**Original Description**: The SignalProcessing controller MUST use a default environment when all detection methods fail.

**Original Acceptance Criteria** (Superseded):
- [x] ~~Default to `unknown` when no label or ConfigMap mapping found~~ → Operators define via Rego `default`
- [x] Log warning when default is used → Logged as "unclassified" from Rego
- [x] Never fail classification - always return a value → Rego `default` guarantees a result
- [x] ~~Confidence: 0.0 for default~~ → Confidence field removed per DD-SP-001

**Rationale** (Updated): Hardcoded Go defaults create silent behavior mismatch when operator Rego policies don't match. Operators have varied environment taxonomies, so they should define their own defaults.

**Test Coverage**: `environment_classifier_test.go` (Unit) - Updated for Rego-only classification

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

> 📋 **Standalone Document**: [BR-SP-071-priority-fallback-matrix.md](../../../requirements/BR-SP-071-priority-fallback-matrix.md)
>
> This section provides a summary. See the standalone document for complete specification.

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

### BR-SP-080: Classification Source Tracking (Updated)

**Priority**: P1 (High)
**Category**: Business Classification
**Version**: 2.0 (Updated 2025-12-14 per DD-SP-001)

**Description**: The SignalProcessing controller MUST track the source of all categorization decisions to enable observability and understanding of how classifications were determined.

**Acceptance Criteria**:
- [ ] Source `"namespace-labels"`: Explicit label from namespace (operator-defined via `kubernaut.ai/environment`)
- [ ] Source `"rego-inference"`: Pattern matching by Rego policy (e.g., namespace name patterns like `prod-*`, `staging-*`)
- [ ] Source `"default"`: No detection method succeeded (fallback to "unknown")
- [ ] Include `source` field in status for each classification (environment, priority, business)
- [ ] **SECURITY**: Do NOT trust signal labels from external sources (Prometheus, K8s events)

**Rationale**: The `source` field indicates **which detection method was used**, enabling operators to understand classification decisions. This replaces confidence scores which are redundant for deterministic processes (labels, pattern matching).

**Security Rationale**: Signal labels originate from untrusted external sources (Prometheus alerts, K8s events) and MUST NOT be used for environment classification. An attacker could inject labels into Prometheus alerts to trigger privilege escalation (staging alert → labeled "production" → production workflow execution).

**Priority Order** (Security-Hardened):
1. Namespace labels (operator-controlled via RBAC, checked by Rego policy) ✅
2. Rego pattern matching (deterministic logic on namespace name) ✅
3. Default fallback ("unknown") ✅
4. ~~Signal labels~~ ❌ **REMOVED** - Security risk (untrusted external source)

**Breaking Change**: Removed `confidence` field from V1.0 (pre-release). No backwards compatibility impact.

**References**:
- [DD-SP-001: Remove Classification Confidence Scores](../../../architecture/decisions/DD-SP-001-remove-classification-confidence-scores.md)

**Test Coverage**: All classifier tests (Unit)

**Changelog**:
- **V2.0** (2025-12-14): Replaced confidence scoring with source tracking per DD-SP-001
- **V1.0** (Initial): Confidence-based approach (deprecated)

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

## Observability Requirements (BR-SP-110 to BR-SP-111)

### BR-SP-110: Kubernetes Conditions for Operator Visibility

**Priority**: P1 (High)
**Category**: Observability
**Mandate**: [DD-CRD-002](../../../architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md) (V1.0 MANDATORY)

**Description**: The SignalProcessing controller MUST implement Kubernetes Conditions to provide detailed status information for operators and automation.

**Acceptance Criteria**:
- [ ] `EnrichmentComplete` condition set after K8s context enrichment (BR-SP-001)
- [ ] `ClassificationComplete` condition set after environment/priority classification (BR-SP-051-072)
- [ ] `CategorizationComplete` condition set after business categorization (BR-SP-080-081)
- [ ] `ProcessingComplete` condition set on terminal state (Completed/Failed)
- [ ] All failure reasons documented and implemented (16 total)
- [ ] Conditions visible in `kubectl describe signalprocessing`
- [ ] `kubectl wait --for=condition=ProcessingComplete` works correctly

**Condition Types**:
| Condition | Phase Alignment | BR Reference |
|-----------|-----------------|--------------|
| `EnrichmentComplete` | Enriching → Classifying | BR-SP-001 |
| `ClassificationComplete` | Classifying → Categorizing | BR-SP-051-072 |
| `CategorizationComplete` | Categorizing → Completed | BR-SP-080-081 |
| `ProcessingComplete` | Terminal state | BR-SP-090 |

**Failure Reasons**:
- **Enrichment**: EnrichmentSucceeded, EnrichmentFailed, K8sAPITimeout, ResourceNotFound, RBACDenied, DegradedMode
- **Classification**: ClassificationSucceeded, ClassificationFailed, RegoEvaluationError, PolicyNotFound, InvalidNamespaceLabels
- **Categorization**: CategorizationSucceeded, CategorizationFailed, InvalidBusinessUnit, InvalidSLATier
- **Processing**: ProcessingSucceeded, ProcessingFailed, AuditWriteFailed, ValidationFailed

**Test Coverage**: `conditions_test.go` (Unit), Integration tests

**References**:
- [DD-SP-002: Kubernetes Conditions Specification](../../../architecture/decisions/DD-SP-002-kubernetes-conditions-specification.md)
- [DD-CRD-002: Kubernetes Conditions Standard](../../../architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md)
- [Implementation Plan](./IMPLEMENTATION_PLAN_CONDITIONS_V1.0.md)

---

### BR-SP-111: Shared Exponential Backoff Integration

**Priority**: P1 (High)
**Category**: Observability
**Mandate**: [DD-SHARED-001](../../../architecture/decisions/DD-SHARED-001-shared-backoff-library.md) (V1.0 MANDATORY)
**Status**: ✅ **IMPLEMENTED** (2025-12-16)

**Description**: The SignalProcessing controller MUST use the shared exponential backoff library for transient error handling to ensure consistent retry behavior and anti-thundering herd protection.

**Acceptance Criteria**:
- [x] Import `pkg/shared/backoff` in controller
- [x] Add `ConsecutiveFailures` field to SignalProcessingStatus
- [x] Add `LastFailureTime` field to SignalProcessingStatus
- [x] Use `backoff.CalculateWithDefaults()` for retry delays
- [x] Implement `isTransientError()` detection function
- [x] Use `RequeueAfter` with backoff delay for transient errors
- [x] Reset failure counter on successful phase transition
- [x] Include ±10% jitter for anti-thundering herd

**Transient Error Types**:
| Error Type | K8s API Error | Backoff Applied |
|------------|---------------|-----------------|
| Timeout | `apierrors.IsTimeout()` | ✅ Yes |
| Server Timeout | `apierrors.IsServerTimeout()` | ✅ Yes |
| Too Many Requests | `apierrors.IsTooManyRequests()` | ✅ Yes |
| Service Unavailable | `apierrors.IsServiceUnavailable()` | ✅ Yes |
| Context Deadline | `context.DeadlineExceeded` | ✅ Yes |
| Context Canceled | `context.Canceled` | ✅ Yes |
| Not Found | `apierrors.IsNotFound()` | ❌ No (fatal) |
| Forbidden | `apierrors.IsForbidden()` | ❌ No (fatal) |

**Backoff Progression** (with ±10% jitter):
| Attempt | Base Delay | With Jitter |
|---------|------------|-------------|
| 1 | 30s | 27-33s |
| 2 | 1m | 54-66s |
| 3 | 2m | 108-132s |
| 4 | 4m | 216-264s |
| 5+ | 5m (capped) | 270-330s |

**Test Coverage**: `backoff_test.go` (21 Unit tests)

**References**:
- [DD-SHARED-001: Shared Backoff Library](../../../architecture/decisions/DD-SHARED-001-shared-backoff-library.md)
- [Team Announcement](../../../handoff/TEAM_ANNOUNCEMENT_SHARED_BACKOFF.md)

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
**Status**: **RELOCATED to HAPI per ADR-056** — See BR-HAPI-253 through BR-HAPI-256

> **ADR-056 Relocation Note**: DetectedLabels computation was moved from SP to HAPI post-RCA.
> Detection MUST run against the **RCA target resource** (identified by the LLM), not the signal source,
> because the signal and root cause may be different resources with different infrastructure characteristics.
> SP still captures raw K8s metadata (annotations, labels) via K8sEnricher into `KubernetesContext`,
> but this is used only for business classification and custom labels — not for DetectedLabels.
>
> **Authoritative specification**: DD-HAPI-018 v1.3
> **Authoritative implementation**: `holmesgpt-api/src/detection/labels.py`

**Original Description** *(retained for traceability)*: The SignalProcessing controller MUST auto-detect 8 cluster characteristics from K8s resources.

**Test Coverage**: Relocated to `holmesgpt-api/tests/unit/test_label_detector.py`

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

## Severity Determination (BR-SP-105 to BR-SP-109)

### BR-SP-105: Severity Determination via Rego Policy

**Priority**: P0 (Critical)
**Category**: Classification
**Status**: 🆕 NEW (January 2026)

**Description**: SignalProcessing MUST determine normalized severity from external signal severity using operator-configurable Rego policies. This enables customers to use ANY severity naming scheme (Sev1-4, P0-P4, Critical/High/Medium/Low, etc.) without code changes.

**Acceptance Criteria**:
- [ ] Rego policy file: `severity.rego` (operator-provided ConfigMap)
- [ ] Policy input: `input.signal.severity` (external value from Gateway, e.g., "Sev1", "P0", "HIGH")
- [ ] Policy output: `result.severity` (normalized to `critical`, `warning`, or `info`)
- [ ] Status field: `Status.SeverityClassification` (struct similar to `EnvironmentClassification`)
- [ ] Fallback: If Rego evaluation fails or severity unmapped → `"unknown"` (NOT `"warning"`)
- [ ] Audit trail: Log severity determination (external → normalized, source: rego/fallback)
- [ ] Observability: Emit event/log when Rego policy fails to map severity
- [ ] Hot-reload: Support ConfigMap updates without pod restart (per BR-SP-072 pattern)

**SeverityClassification Status Field**:
```go
type SeverityClassification struct {
    Severity      string      `json:"severity"`              // Normalized: critical, warning, info, or unknown
    Source        string      `json:"source"`                // rego-policy, fallback
    ClassifiedAt  metav1.Time `json:"classifiedAt"`
    ExternalValue string      `json:"externalValue"`         // Original value from Gateway (e.g., "Sev1")
}
```

**Default Rego Policy** (backward compatibility):
```rego
package signalprocessing.severity

import rego.v1

# 1:1 mapping for standard severity values
result := {"severity": "critical", "source": "rego-policy"} if {
    lower(input.signal.severity) == "critical"
}

result := {"severity": "warning", "source": "rego-policy"} if {
    lower(input.signal.severity) == "warning"
}

result := {"severity": "info", "source": "rego-policy"} if {
    lower(input.signal.severity) == "info"
}

# Fallback: unmapped severity → unknown
default result := {"severity": "unknown", "source": "fallback"}
```

**Operator Customization Example**:
```rego
package signalprocessing.severity

import rego.v1

# Enterprise "Sev" scheme
result := {"severity": "critical", "source": "rego-policy"} if {
    input.signal.severity in ["Sev1", "SEV1", "sev1"]
}

result := {"severity": "warning", "source": "rego-policy"} if {
    input.signal.severity in ["Sev2", "SEV2", "sev2"]
}

result := {"severity": "info", "source": "rego-policy"} if {
    input.signal.severity in ["Sev3", "SEV3", "sev3", "Sev4", "SEV4", "sev4"]
}

# PagerDuty "P" scheme
result := {"severity": "critical", "source": "rego-policy"} if {
    input.signal.severity in ["P0", "P1"]
}

result := {"severity": "warning", "source": "rego-policy"} if {
    input.signal.severity in ["P2", "P3"]
}

result := {"severity": "info", "source": "rego-policy"} if {
    input.signal.severity in ["P4"]
}

# Fallback
default result := {"severity": "unknown", "source": "fallback"}
```

**Rationale**:
- **Customer Extensibility**: Operators define severity mappings based on their organization's standards
- **Separation of Concerns**: Gateway extracts, SignalProcessing determines
- **Architectural Consistency**: Matches environment/priority/business classification pattern
- **Observability**: Operators can monitor unmapped severities and adjust policies

**Implementation**:
- `pkg/signalprocessing/classifier/severity.go`: Severity classifier with Rego evaluation
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Add severity classification in `reconcileClassifying` phase
- `api/signalprocessing/v1alpha1/signalprocessing_types.go`: Add `Status.SeverityClassification` field
- `deploy/signalprocessing/policies/severity.rego`: Default policy ConfigMap

**Tests**:
- `test/unit/signalprocessing/severity_classifier_test.go`: Unit tests for classifier
- `test/integration/signalprocessing/severity_policy_test.go`: Custom severity mapping E2E
- `test/unit/signalprocessing/controller_test.go`: Status field population

**Related BRs**:
- BR-GATEWAY-111 (Gateway Pass-Through Architecture)
- BR-SP-051 to BR-SP-053 (Environment Classification - same pattern)
- BR-SP-070 to BR-SP-072 (Priority Assignment - same pattern)
- BR-SP-090 (Audit Trail)

**Consumer Impact**:
- BR-AI-XXX: AIAnalysis MUST read `sp.Status.SeverityClassification.Severity` (NOT `sp.Spec.Severity`)
- BR-RO-XXX: RemediationOrchestrator MUST read `sp.Status.SeverityClassification.Severity` (NOT `sp.Spec.Severity`)

**Decision Reference**:
- [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
- [DD-SEVERITY-001](../../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)
- [TRIAGE-SEVERITY-EXTENSIBILITY](../../../architecture/decisions/TRIAGE-SEVERITY-EXTENSIBILITY.md)

**Migration Plan**: See DD-SEVERITY-001 Phase 3 (Week 2)

---

## Requirements Summary

| Category | Count | Priority Breakdown | Notes |
|----------|-------|-------------------|-------|
| Core Enrichment | 5 | P0: 2, P1: 1, P2: 2 | 2 deprecated (BR-SP-006, BR-SP-012) |
| Environment Classification | 3 | P0: 1, P1: 2 | |
| Priority Assignment | 3 | P0: 1, P1: 2 | |
| **Severity Determination** | **1** | **P0: 1** | **🆕 NEW: BR-SP-105** |
| Business Classification | 2 | P1: 2 | |
| Audit & Observability | 1 | P1: 1 | |
| Observability | 2 | P1: 2 | BR-SP-110, BR-SP-111 |
| Label Detection | 5 | P0: 3, P1: 2 | |
| **Total** | **22** | **P0: 8, P1: 11, P2: 3** | **2 deprecated** |

### BR-SP-106: Proactive Signal Mode Classification

**Priority**: P1 (High)
**Category**: Classification
**Status**: 🆕 NEW (February 2026)
**GitHub Issue**: [#55](https://github.com/jordigilh/kubernaut/issues/55)
**Full Document**: [docs/requirements/BR-SP-106-proactive-signal-mode-classification.md](../../../requirements/BR-SP-106-proactive-signal-mode-classification.md)

**Description**: SignalProcessing MUST classify incoming signals as `proactive` or `reactive` based on signal type, and normalize proactive signal types to their base type for workflow catalog matching. This enables Kubernaut to handle preemptive remediation for predicted incidents (e.g., Prometheus `predict_linear()` alerts) using the existing pipeline.

**Acceptance Criteria**: See [dedicated BR document](../../../requirements/BR-SP-106-proactive-signal-mode-classification.md#acceptance-criteria).

**Related**: BR-AI-084, DD-WORKFLOW-001 (label-based workflow matching)

---

### Deprecated Requirements

| BR ID | Reason | Date |
|-------|--------|------|
| BR-SP-006 | Wrong layer - Gateway owns filtering | 2025-12-19 |
| BR-SP-012 | Wrong layer - Gateway owns deduplication | 2025-12-19 |

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
| **BR-SP-105** | **Severity Classifier** | **`severity_classifier_test.go`** | **⏳ NEW** |
| BR-SP-080 | Day 6: Business Classifier | All classifier tests | ✅ Planned |
| BR-SP-081 | Day 6: Business Classifier | `business_classifier_test.go` | ✅ Planned |
| BR-SP-090 | Day 11: Audit Client | `audit_client_test.go` | ✅ Planned |
| BR-SP-100 | Day 7: OwnerChain | `ownerchain_builder_test.go` | ✅ Planned |
| BR-SP-101 | Day 8: DetectedLabels | `label_detector_test.go` | ✅ Planned |
| BR-SP-102 | Day 9: CustomLabels | `rego_engine_test.go` | ✅ Planned |
| BR-SP-103 | Day 8: FailedDetections | `label_detector_test.go` | ✅ Planned |
| BR-SP-104 | Day 9: Security Wrapper | `rego_security_wrapper_test.go` | ✅ Planned |
| **BR-SP-106** | **Proactive Signal Mode** | **`signal_mode_classifier_test.go`** | **⏳ NEW** |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.4 | 2026-02-08 | **NEW BR-SP-106**: Proactive Signal Mode Classification. Enables preemptive remediation via Prometheus `predict_linear()` alerts. Signal type normalization + `SignalMode` status field. [Issue #55](https://github.com/jordigilh/kubernaut/issues/55). |
| 1.3 | 2026-01-09 | **NEW BR-SP-105**: Severity Determination via Rego Policy. Enables customer extensibility (Sev1-4, P0-P4 schemes). Operator-configurable severity mappings. Fallback to "unknown" (not "warning"). Observability for unmapped severities. |
| 1.2 | 2025-12-06 | BR-SP-071: Changed to severity-only fallback (not environment × severity matrix) |
| 1.1 | 2025-12-06 | BR-SP-072: Updated to fsnotify-based hot-reload per DD-INFRA-001, added FileWatcher reference |
| 1.0 | 2025-12-03 | Initial release - 19 BRs defined |

