# Gateway Service - Business Requirements

**Version**: v1.7
**Last Updated**: 2026-01-29
**Status**: ✅ APPROVED
**Owner**: Gateway Team
**Total BRs**: 77 identified BRs (BR-GATEWAY-001 through BR-GATEWAY-183)

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.7 | 2026-01-29 | **NEW BR-GATEWAY-182, BR-GATEWAY-183**: ServiceAccount Authentication and SAR Authorization. Gateway MUST authenticate webhook requests using Kubernetes TokenReview and authorize using SubjectAccessReview for defense-in-depth security and SOC2 compliance. Supersedes DD-GATEWAY-006 (network-only security). | [DD-AUTH-014 V2.0](../../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) |
> | v1.6 | 2026-01-09 | **NEW BR-GATEWAY-181**: Signal Pass-Through Architecture. Gateway MUST preserve external severity/environment/priority values WITHOUT transformation. Removes hardcoded severity mappings. Enables customer extensibility (Sev1-4, P0-P4 schemes). | [DD-SEVERITY-001](../../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md), [TRIAGE-SEVERITY-EXTENSIBILITY](../../../architecture/decisions/TRIAGE-SEVERITY-EXTENSIBILITY.md) |
> | v1.5 | 2025-12-07 | BR-GATEWAY-038: Rate limiting code REMOVED (middleware + tests). Proxy delegation complete. | [ADR-048](../../../architecture/decisions/ADR-048-rate-limiting-proxy-delegation.md) |
> | v1.4 | 2025-12-07 | BR-GATEWAY-038 (Rate Limiting): Delegated to Ingress/Route proxy. Gateway middleware DEPRECATED. | [ADR-048](../../../architecture/decisions/ADR-048-rate-limiting-proxy-delegation.md) |
> | v1.3 | 2025-12-06 | Classification code REMOVED from Gateway (not placeholder). Updated BR-007, BR-014-017 to reflect file deletions. | [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md) |
> | v1.2 | 2025-12-03 | Added BR-GATEWAY-TARGET-RESOURCE-VALIDATION for resource info validation | [DD-GATEWAY-NON-K8S-SIGNALS](../../../architecture/decisions/DD-GATEWAY-NON-K8S-SIGNALS.md) |
> | v1.1 | 2025-11-11 | 5 BRs deprecated (007, 014-017) - moved to Signal Processing | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
> | v1.0 | 2025-10-04 | Initial business requirements | - |

---

## 📋 **BR Overview**

This document provides a comprehensive list of all business requirements for the Gateway Service. Each BR is mapped to its implementation, test coverage, and priority.

**BR Numbering**: Not all numbers are used (gaps indicate deprecated or future BRs)

**Test Coverage Status**:
- ✅ **Covered**: BR has test coverage (unit, integration, or E2E)
- ⏳ **Planned**: BR planned for future implementation
- ❌ **Missing**: BR has no test coverage

---

## 🎯 **Core Signal Ingestion** (BR-GATEWAY-001 to BR-GATEWAY-025)

### **BR-GATEWAY-001: Prometheus AlertManager Webhook Ingestion**
**Description**: Gateway must accept and process Prometheus AlertManager webhook payloads
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + Integration + E2E
**Implementation**: `pkg/gateway/adapters/prometheus/adapter.go`
**Tests**: `test/unit/gateway/adapters/prometheus_adapter_test.go`

### **BR-GATEWAY-002: Kubernetes Event Ingestion**
**Description**: Gateway must accept and process Kubernetes Event payloads
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/adapters/k8s_event/adapter.go`
**Tests**: `test/unit/gateway/k8s_event_adapter_test.go`

### **BR-GATEWAY-003: Signal Validation**
**Description**: Gateway must validate incoming signal payloads for required fields
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/adapters/*/validation.go`
**Tests**: `test/unit/gateway/adapters/validation_test.go`, `test/integration/gateway/signal_validation_test.go`

### **BR-GATEWAY-004: Signal Fingerprinting**
**Description**: Gateway must generate deterministic fingerprints for signal deduplication
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/processing/deduplication.go`
**Tests**: `test/unit/gateway/deduplication_test.go`

### **BR-GATEWAY-005: Signal Metadata Extraction**
**Description**: Gateway must extract severity, namespace, and resource metadata from external signals **without transformation or interpretation**
**Priority**: P0 (Critical)
**Status**: ✅ Complete (Updated 2026-01-16 - Pass-through architecture per DD-SEVERITY-001)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/adapters/prometheus_adapter.go`, `pkg/gateway/adapters/kubernetes_event_adapter.go`
**Tests**: `test/integration/gateway/custom_severity_test.go`, `test/unit/gateway/adapters/*_test.go`

**Clarification** (2026-01-16 per DD-SEVERITY-001): Gateway acts as a "dumb pipe" - extracts and preserves values, never determines policy-based classifications. Severity determination is owned by SignalProcessing via Rego policy (BR-SP-105).

**Examples**:
- Prometheus alert with `labels.severity="Sev1"` → `RR.Spec.Severity="Sev1"` (preserved)
- K8s event with `Type="Warning"` → `RR.Spec.Severity="Warning"` (preserved)
- Missing severity → `RR.Spec.Severity="unknown"` (default, not policy)

**Authority**: [DD-SEVERITY-001](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md), BR-GATEWAY-111

### **BR-GATEWAY-006: Signal Timestamp Validation**
**Description**: Gateway must validate signal timestamps and reject stale signals
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/middleware/timestamp_validation.go`
**Tests**: `test/unit/gateway/middleware/timestamp_validation_test.go`

### **BR-GATEWAY-007: Signal Priority Classification** ⛔ **DEPRECATED (2026-01-16)**
**Status**: ⛔ **DEPRECATED** (2026-01-16 per DD-SEVERITY-001)
**Reason**: Priority determination moved to SignalProcessing Rego (BR-SP-070)
**Replacement**: Gateway passes through raw priority hints (if present in labels), SignalProcessing determines final priority
**Migration**: Removed priority determination logic from Gateway adapters (2025-12-06)
**Authority**: [DD-SEVERITY-001](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)

**Description**: ~~Gateway must classify signals into P0/P1/P2/P3 priorities based on severity~~ **REMOVED**: Priority classification completely removed from Gateway. Signal Processing Service now owns this functionality via Rego policy (BR-SP-070).
**Priority**: P0 (Critical)
**Test Coverage**: ❌ N/A - Code removed from Gateway
**Implementation**: ~~`pkg/gateway/processing/priority_classification.go`~~ **DELETED** (2025-12-06)
**Tests**: ~~`test/unit/gateway/priority_classification_test.go`~~ **DELETED** (2025-12-06)
**Migration Target**: Signal Processing Service (BR-SP-070 to BR-SP-072)
**Decision Reference**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
**Removal Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

### **BR-GATEWAY-008: Storm Detection** ❌ **REMOVED**
**Status**: ❌ **REMOVED** (December 13, 2025)
**Reason**: Redundant with deduplication (`occurrenceCount`), no downstream consumers, no added business value
**Removal Reference**: [DD-GATEWAY-015](../../../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)
**Original Description**: Gateway must detect alert storms (>10 alerts/minute) and aggregate them

### **BR-GATEWAY-009: Concurrent Storm Detection** ❌ **REMOVED**
**Status**: ❌ **REMOVED** (December 13, 2025)
**Reason**: Part of storm detection feature removal
**Removal Reference**: [DD-GATEWAY-015](../../../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)
**Original Description**: Gateway must handle concurrent alert bursts without race conditions

### **BR-GATEWAY-010: Storm State Recovery** ❌ **REMOVED**
**Status**: ❌ **REMOVED** (December 13, 2025)
**Reason**: Part of storm detection feature removal (Redis deprecated per DD-GATEWAY-011)
**Removal Reference**: [DD-GATEWAY-015](../../../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)
**Original Description**: Gateway must recover storm state from Redis after restart

### **BR-GATEWAY-011: Deduplication**
**Description**: Gateway must deduplicate identical signals within TTL window
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/processing/deduplication.go`
**Tests**: `test/unit/gateway/deduplication_test.go`, `test/integration/gateway/deduplication_test.go`

### **BR-GATEWAY-012: Deduplication TTL**
**Description**: Gateway must expire deduplicated signals after configurable TTL (default: 5 minutes)
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/processing/deduplication.go`
**Tests**: `test/unit/gateway/deduplication_test.go`

### **BR-GATEWAY-013: Deduplication Count Tracking**
**Description**: Gateway must track count of deduplicated signals for observability
**Priority**: P2 (Medium)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/processing/deduplication.go`
**Tests**: `test/unit/gateway/deduplication_test.go`

### **BR-GATEWAY-014: Signal Enrichment** ⚠️ **DEPRECATED - REMOVED (2025-12-06)**
**Description**: ~~Gateway must enrich signals with environment classification (prod/staging/dev)~~ **REMOVED**: Environment classification completely removed from Gateway (2025-12-06). Signal Processing Service now owns this functionality.
**Priority**: P1 (High)
**Test Coverage**: ❌ N/A - Code removed from Gateway
**Implementation**: ~~`pkg/gateway/processing/environment_classification.go`~~ **DELETED** (2025-12-06)
**Tests**: N/A - Implemented in Signal Processing
**Migration Target**: Signal Processing Service (BR-SP-051 to BR-SP-053)
**Decision Reference**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
**Removal Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

### **BR-GATEWAY-015: Environment Classification - Explicit Labels** ⚠️ **DEPRECATED - REMOVED (2025-12-06)**
**Description**: ~~Gateway must classify environment from explicit `environment` label~~ **REMOVED**: Environment classification completely removed from Gateway (2025-12-06).
**Priority**: P1 (High)
**Test Coverage**: ❌ N/A - Code removed from Gateway
**Implementation**: ~~`pkg/gateway/processing/environment_classification.go`~~ **DELETED** (2025-12-06)
**Tests**: ~~`test/unit/gateway/processing/environment_classification_test.go`~~ **DELETED** (2025-12-06)
**Migration Target**: Signal Processing Service (BR-SP-051)
**Decision Reference**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
**Removal Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

### **BR-GATEWAY-016: Environment Classification - Namespace Pattern** ⚠️ **DEPRECATED - REMOVED (2025-12-06)**
**Description**: ~~Gateway must classify environment from namespace patterns (prod-*, staging-*, dev-*)~~ **REMOVED**: Environment classification completely removed from Gateway (2025-12-06).
**Priority**: P1 (High)
**Test Coverage**: ❌ N/A - Code removed from Gateway
**Implementation**: ~~`pkg/gateway/processing/environment_classification.go`~~ **DELETED** (2025-12-06)
**Tests**: ~~`test/unit/gateway/processing/environment_classification_test.go`~~ **DELETED** (2025-12-06)
**Migration Target**: Signal Processing Service (BR-SP-052)
**Decision Reference**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
**Removal Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

### **BR-GATEWAY-017: Environment Classification - Fallback** ⚠️ **DEPRECATED - REMOVED (2025-12-06)**
**Description**: ~~Gateway must use fallback environment (unknown) when classification fails~~ **REMOVED**: Environment classification completely removed from Gateway (2025-12-06).
**Priority**: P2 (Medium)
**Test Coverage**: ❌ N/A - Code removed from Gateway
**Implementation**: ~~`pkg/gateway/processing/environment_classification.go`~~ **DELETED** (2025-12-06)
**Tests**: ~~`test/unit/gateway/processing/environment_classification_test.go`~~ **DELETED** (2025-12-06)
**Migration Target**: Signal Processing Service (BR-SP-053)
**Decision Reference**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
**Removal Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

### **BR-GATEWAY-018: CRD Metadata Generation**
**Description**: Gateway must generate RemediationRequest CRD metadata (labels, annotations)
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/processing/crd_creator.go`
**Tests**: `test/unit/gateway/crd_metadata_test.go`, `test/integration/gateway/crd_metadata_test.go`

### **BR-GATEWAY-019: CRD Name Generation**
**Description**: Gateway must generate valid CRD names (DNS subdomain, ≤253 chars)
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + E2E
**Implementation**: `pkg/gateway/processing/crd_creator.go`
**Tests**: `test/unit/gateway/crd_metadata_test.go`, `test/e2e/gateway/05_crd_name_length_test.go`

### **BR-GATEWAY-020: CRD Namespace Handling**
**Description**: Gateway must create CRDs in target namespace or fallback namespace
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/processing/crd_creator.go`
**Tests**: `test/unit/gateway/crd_metadata_test.go`, `test/integration/gateway/crd_creation_test.go`

### **BR-GATEWAY-021: CRD Creation**
**Description**: Gateway must create RemediationRequest CRDs in Kubernetes
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/processing/crd_creator.go`
**Tests**: `test/unit/gateway/crd_metadata_test.go`, `test/integration/gateway/crd_creation_test.go`

### **BR-GATEWAY-022: Signal Adapter Registration**
**Description**: Gateway must support dynamic adapter registration for new signal sources
**Priority**: P2 (Medium)
**Test Coverage**: ⏳ Deferred (v2.0)
**Implementation**: `pkg/gateway/adapters/registry.go` (exists but unused)
**Tests**: None (intentional - plugin system deferred to v2.0)
**Rationale**: v1.0 ships with 2 static adapters (Prometheus, K8s Events). Dynamic registration not needed until custom adapter support required.

### **BR-GATEWAY-023: Signal Adapter Validation**
**Description**: Gateway must validate adapter implementations at registration time
**Priority**: P2 (Medium)
**Test Coverage**: ⏳ Deferred (v2.0)
**Implementation**: `pkg/gateway/adapters/registry.go` (exists but unused)
**Tests**: None (intentional - depends on BR-022)
**Rationale**: Adapter validation only needed when dynamic adapter registration (BR-022) is implemented.

### **BR-GATEWAY-024: HTTP Request Logging**
**Description**: Gateway must log all incoming HTTP requests with sanitized data
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/shared/sanitization/sanitizer.go` (shared library)
**Tests**: `test/unit/shared/sanitization/sanitizer_test.go`

### **BR-GATEWAY-025: HTTP Response Logging**
**Description**: Gateway must log all HTTP responses with status codes and duration
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/shared/sanitization/sanitizer.go` (shared library)
**Tests**: `test/unit/shared/sanitization/sanitizer_test.go`

### **BR-GATEWAY-027: Signal Source Service Identification**
**Description**: Gateway adapters must provide monitoring system name (not adapter name) for LLM tool selection
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/adapters/adapter.go`, `pkg/gateway/adapters/prometheus_adapter.go`, `pkg/gateway/adapters/kubernetes_event_adapter.go`
**Tests**: `test/unit/gateway/adapters/prometheus_adapter_test.go`, `test/unit/gateway/k8s_event_adapter_test.go`

**Business Context**: The LLM uses the `signal_source` field to determine which investigation tools to use:
- `signal_source="prometheus"` → LLM uses Prometheus queries for investigation
- `signal_source="kubernetes-events"` → LLM uses kubectl for investigation

**Technical Details**:
- `GetSourceService()` returns monitoring system name (e.g., "prometheus", "kubernetes-events")
- `GetSourceType()` returns signal type identifier (e.g., "prometheus-alert", "kubernetes-event")
- Adapter names (e.g., "prometheus-adapter") are internal implementation details, not useful for LLM
- Both methods are part of the `SignalAdapter` interface

---

### **BR-GATEWAY-028: Unique CRD Names for Signal Occurrences**
**Description**: Each signal occurrence MUST create a unique RemediationRequest CRD, even if the same problem reoccurs
**Priority**: P0 (Critical)
**Test Coverage**: ⏳ Planned (Unit + Integration)
**Implementation**: `pkg/gateway/processing/crd_creator.go`
**Tests**: `test/unit/gateway/processing/crd_creator_test.go` (planned)
**Related DD**: [DD-015: Timestamp-Based CRD Naming](../../../architecture/decisions/DD-015-timestamp-based-crd-naming.md)

**Business Context**:
- **Remediation Retry**: If first remediation fails/completes, subsequent occurrences need new remediation attempts
- **Audit Trail**: Each occurrence must be independently tracked for compliance
- **Historical Analysis**: ML models need complete occurrence history, not just latest

**Acceptance Criteria**:
- ✅ Same signal occurring twice creates 2 unique CRDs
- ✅ CRD names never collide, even for identical signals
- ✅ Fingerprint remains stable across all occurrences
- ✅ Can query all occurrences via field selector on `spec.signalFingerprint`

**Technical Details**:
- CRD name format: `rr-<fingerprint-prefix-12-chars>-<unix-timestamp>`
- Example: `rr-bd773c9f25ac-1731868032`
- Fingerprint stored in `spec.signalFingerprint` for querying

---

### **BR-GATEWAY-029: Immutable Signal Fingerprint**
**Description**: The `spec.signalFingerprint` field MUST be immutable after CRD creation
**Priority**: P0 (Critical)
**Test Coverage**: ⏳ Planned (Unit + E2E)
**Implementation**: `api/remediation/v1alpha1/remediationrequest_types.go`
**Tests**: `test/unit/api/remediation/immutability_test.go` (planned), `test/e2e/crd_immutability_test.go` (planned)
**Related DD**: [DD-015: Timestamp-Based CRD Naming](../../../architecture/decisions/DD-015-timestamp-based-crd-naming.md)

**Business Context**:
- **Data Integrity**: Fingerprint is used for deduplication and tracking - must not change
- **Query Stability**: Field selector queries depend on stable fingerprint values
- **Audit Compliance**: Immutability ensures fingerprint cannot be tampered with

**Acceptance Criteria**:
- ✅ CRD creation with fingerprint succeeds
- ✅ CRD update attempting to change fingerprint fails with validation error
- ✅ Validation error message: "signalFingerprint is immutable"

**Technical Details**:
- Uses Kubernetes CEL validation: `+kubebuilder:validation:XValidation:rule="self == oldSelf",message="signalFingerprint is immutable"`
- Field selector query: `kubectl get remediationrequest --field-selector spec.signalFingerprint=<full-64-char-fingerprint>`

---

## 🔐 **Security & Authentication** (BR-GATEWAY-036 to BR-GATEWAY-054)

### **BR-GATEWAY-036: Kubernetes TokenReviewer Authentication**
**Description**: Gateway must authenticate API requests using Kubernetes TokenReviewer
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/middleware/auth.go`
**Tests**: None

### **BR-GATEWAY-037: ServiceAccount RBAC Validation**
**Description**: Gateway must validate ServiceAccount has required RBAC permissions
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/middleware/auth.go`
**Tests**: None

### **BR-GATEWAY-038: Rate Limiting** ✅ DELEGATED TO PROXY
**Description**: Gateway must enforce rate limits (1000 req/min per client)
**Priority**: P1 (High)
**Test Coverage**: ✅ Delegated to Ingress/Route Proxy (no Gateway tests needed)
**Implementation**: ❌ REMOVED - `pkg/gateway/middleware/ratelimit.go` deleted (ADR-048)
**Tests**: ❌ REMOVED - `test/unit/gateway/middleware/ratelimit_test.go` deleted
**Related BRs**: Covered via VULN-GATEWAY-003
**Architectural Decision**: [ADR-048 - Rate Limiting Proxy Delegation](../../../architecture/decisions/ADR-048-rate-limiting-proxy-delegation.md)

> **✅ IMPLEMENTATION COMPLETE (2025-12-07)**
>
> Rate limiting is now delegated to the infrastructure layer:
> - **Kubernetes**: Nginx Ingress annotations (`nginx.ingress.kubernetes.io/limit-rps`)
> - **OpenShift**: HAProxy Router annotations (`haproxy.router.openshift.io/rate-limit-*`)
>
> **Files Deleted**:
> - `pkg/gateway/middleware/ratelimit.go`
> - `test/unit/gateway/middleware/ratelimit_test.go`
> - `test/e2e/gateway/15_rate_limiting_under_load_test.go`
>
> **Rationale**: Global cluster-wide enforcement, zero Redis dependency, crash-proof.
> See [ADR-048](../../../architecture/decisions/ADR-048-rate-limiting-proxy-delegation.md) for details.

### **BR-GATEWAY-039: Security Headers**
**Description**: Gateway must add security headers (X-Content-Type-Options, X-Frame-Options, etc.)
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/middleware/security_headers.go`
**Tests**: `test/unit/gateway/middleware/security_headers_test.go`
**Related BRs**: Covered via BR-GATEWAY-073 (19 refs), BR-GATEWAY-074 (1 ref)
**Note**: Tests reference sub-BRs for granular coverage tracking

### **BR-GATEWAY-040: TLS Support**
**Description**: Gateway must support TLS for HTTPS endpoints
**Priority**: P1 (High)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/server.go`
**Tests**: None

### **BR-GATEWAY-041: Mutual TLS (mTLS)**
**Description**: Gateway must support mutual TLS for client authentication
**Priority**: P2 (Medium)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/server.go`
**Tests**: None

### **BR-GATEWAY-042: Log Sanitization**
**Description**: Gateway must sanitize sensitive data (tokens, passwords) from logs
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit (shared library)
**Implementation**: `pkg/shared/sanitization/sanitizer.go` (DD-005 compliant)
**Tests**: `test/unit/shared/sanitization/sanitizer_test.go`

### **BR-GATEWAY-043: Input Validation**
**Description**: Gateway must validate all input payloads against schema
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/adapters/*/validation.go`
**Tests**: `test/unit/gateway/adapters/validation_test.go`

### **BR-GATEWAY-044: SQL Injection Prevention**
**Description**: Gateway must prevent SQL injection attacks (N/A - no SQL)
**Priority**: N/A
**Test Coverage**: N/A
**Implementation**: N/A
**Tests**: N/A

### **BR-GATEWAY-045: XSS Prevention**
**Description**: Gateway must prevent XSS attacks (N/A - API only)
**Priority**: N/A
**Test Coverage**: N/A
**Implementation**: N/A
**Tests**: N/A

### **BR-GATEWAY-050: Network Policy Enforcement**
**Description**: Gateway must enforce Kubernetes Network Policies for ingress/egress
**Priority**: P1 (High)
**Test Coverage**: ❌ Missing
**Implementation**: Kubernetes manifests
**Tests**: None

### **BR-GATEWAY-051: Pod Security Standards**
**Description**: Gateway must comply with Kubernetes Pod Security Standards (restricted)
**Priority**: P1 (High)
**Test Coverage**: ✅ Integration
**Implementation**: Kubernetes manifests
**Tests**: `test/integration/gateway/webhook_security_test.go`

### **BR-GATEWAY-052: Secret Management**
**Description**: Gateway must load secrets from Kubernetes Secrets (not environment variables)
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/config/config.go`
**Tests**: None

### **BR-GATEWAY-053: RBAC Permissions**
**Description**: Gateway must have minimal RBAC permissions (create RemediationRequest CRDs only)
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: Kubernetes manifests
**Tests**: None

### **BR-GATEWAY-054: Audit Logging**
**Description**: Gateway must log all CRD creation events for audit trail
**Priority**: P1 (High)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/processing/crd_creator.go`
**Tests**: None

---

## 📊 **Observability** (BR-GATEWAY-066 to BR-GATEWAY-079)

### **BR-GATEWAY-066: Prometheus Metrics Endpoint**
**Description**: Gateway must expose Prometheus metrics at `/metrics` endpoint
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Integration (13 refs)
**Implementation**: `pkg/gateway/metrics/metrics.go`
**Tests**: `test/integration/gateway/observability_test.go` (Context: "BR-101: Prometheus Metrics Endpoint")
**Related BRs**: Covered via BR-GATEWAY-101 in observability tests
**Note**: Test uses BR-101 numbering for observability suite consistency

### **BR-GATEWAY-067: HTTP Request Metrics**
**Description**: Gateway must expose HTTP request count, duration, and status code metrics
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/middleware/http_metrics.go`
**Tests**: `test/unit/gateway/middleware/http_metrics_test.go`, `test/integration/gateway/observability_test.go` (Context: "BR-104: HTTP Request Duration Metrics")
**Related BRs**: Covered via BR-GATEWAY-104 in observability tests
**Note**: Test uses BR-104 numbering for observability suite consistency

### **BR-GATEWAY-068: CRD Creation Metrics**
**Description**: Gateway must expose CRD creation count and duration metrics
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Integration (1 ref)
**Implementation**: `pkg/gateway/metrics/metrics.go`
**Tests**: `test/integration/gateway/observability_test.go` (Context: "BR-103: CRD Creation Metrics")
**Related BRs**: Covered via BR-GATEWAY-103 in observability tests
**Note**: Test uses BR-103 numbering for observability suite consistency

### **BR-GATEWAY-069: Deduplication Metrics**
**Description**: Gateway must expose deduplication hit/miss rate metrics
**Priority**: P1 (High)
**Test Coverage**: ✅ Integration (1 ref)
**Implementation**: `pkg/gateway/metrics/metrics.go`
**Tests**: `test/integration/gateway/observability_test.go` (Context: "BR-102: Alert Ingestion Metrics" - includes deduplication metrics)
**Related BRs**: Covered via BR-GATEWAY-102 in observability tests
**Note**: Test uses BR-102 numbering for observability suite consistency

### **BR-GATEWAY-070: Storm Detection Metrics** ❌ **REMOVED**
**Status**: ❌ **REMOVED** (December 13, 2025)
**Reason**: Part of storm detection feature removal
**Removal Reference**: [DD-GATEWAY-015](../../../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md)
**Migration**: Use `occurrenceCount >= 5` in Prometheus queries to identify persistent signals
**Original Description**: Gateway must expose storm detection count and aggregation metrics

### **BR-GATEWAY-071: Health Check Endpoint**
**Description**: Gateway must expose `/health` endpoint for liveness probe
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Integration
**Implementation**: `pkg/gateway/server.go`
**Tests**: `test/integration/gateway/http_server_test.go`

### **BR-GATEWAY-072: Readiness Check Endpoint**
**Description**: Gateway must expose `/ready` endpoint for readiness probe
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Integration
**Implementation**: `pkg/gateway/server.go`
**Tests**: `test/integration/gateway/http_server_test.go`

### **BR-GATEWAY-073: Redis Health Check**
**Description**: Gateway must check Redis connectivity in readiness probe
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Integration
**Implementation**: `pkg/gateway/server.go`
**Tests**: `test/integration/gateway/redis_connection_test.go`

### **BR-GATEWAY-074: Kubernetes API Health Check**
**Description**: Gateway must check Kubernetes API connectivity in readiness probe
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Integration
**Implementation**: `pkg/gateway/server.go`
**Tests**: `test/integration/gateway/http_server_test.go`

### **BR-GATEWAY-075: Structured Logging**
**Description**: Gateway must use structured logging (JSON format) with zap logger
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/server.go`, `pkg/shared/sanitization/sanitizer.go`
**Tests**: `test/unit/shared/sanitization/sanitizer_test.go`

### **BR-GATEWAY-076: Log Levels**
**Description**: Gateway must support configurable log levels (debug, info, warn, error)
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/config/config.go`
**Tests**: None

### **BR-GATEWAY-077: Distributed Tracing**
**Description**: Gateway must support OpenTelemetry distributed tracing
**Priority**: P2 (Medium)
**Test Coverage**: ✅ Integration
**Implementation**: `pkg/gateway/middleware/tracing.go`
**Tests**: `test/integration/gateway/observability_test.go`

### **BR-GATEWAY-078: Error Tracking**
**Description**: Gateway must track and expose error rates by type
**Priority**: P1 (High)
**Test Coverage**: ✅ Integration
**Implementation**: `pkg/gateway/metrics/metrics.go`
**Tests**: `test/integration/gateway/observability_test.go` (multiple contexts test error metrics)
**Note**: Error tracking validated across multiple observability test contexts

### **BR-GATEWAY-079: Performance Metrics**
**Description**: Gateway must expose P50/P95/P99 latency metrics
**Priority**: P1 (High)
**Test Coverage**: ✅ Integration
**Implementation**: `pkg/gateway/metrics/metrics.go`
**Tests**: `test/integration/gateway/observability_test.go` (Context: "BR-104: HTTP Request Duration Metrics" - includes histogram for P50/P95/P99)
**Related BRs**: Covered via BR-GATEWAY-104 in observability tests
**Note**: Latency percentiles validated through HTTP request duration histogram

---

## 🔄 **Reliability & Resilience** (BR-GATEWAY-090 to BR-GATEWAY-115)

### **BR-GATEWAY-090: Redis Connection Pooling**
**Description**: Gateway must use connection pooling for Redis connections
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/config/config.go`
**Tests**: None

### **BR-GATEWAY-091: Redis HA Support**
**Description**: Gateway must support Redis Sentinel for high availability
**Priority**: P1 (High)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/config/config.go`
**Tests**: None

### **BR-GATEWAY-092: Graceful Shutdown**
**Description**: Gateway must implement graceful shutdown (drain requests, close connections)
**Priority**: P0 (Critical)
**Test Coverage**: ✅ Integration
**Implementation**: `pkg/gateway/server.go`
**Tests**: `test/integration/gateway/graceful_shutdown_test.go`

### **BR-GATEWAY-093: Circuit Breaker for K8s API**
**Description**: Gateway must implement circuit breaker for critical dependencies (Kubernetes API)
**Priority**: P1 (High)
**Test Coverage**: ✅ Implemented (2026-01-03)
**Implementation**: `pkg/gateway/k8s/client_with_circuit_breaker.go`
**Tests**: `test/integration/gateway/k8s_api_failure_test.go` (BR-GATEWAY-093)
**Design Decision**: DD-GATEWAY-014 (Circuit Breaker for K8s API)
**Library**: `github.com/sony/gobreaker` (industry-standard, production-grade)
**Rationale**: Kubernetes API is a critical dependency for Gateway operations. Circuit breaker prevents cascading failures when K8s API is degraded (high latency, rate limiting, or unavailability), enabling fail-fast behavior and protecting Gateway availability. Complements existing retry logic (BR-111-114) by preventing repeated attempts when K8s API is known to be degraded.

**Sub-Requirements**:
- **BR-GATEWAY-093-A**: Fail-fast when K8s API unavailable (prevent request queue buildup)
- **BR-GATEWAY-093-B**: Prevent cascade failures during K8s API overload (protect cluster control plane)
- **BR-GATEWAY-093-C**: Observable metrics for circuit breaker state and operations (enable SRE response)

**Circuit Breaker Configuration**:
- **Threshold**: 50% failure rate over 10 requests
- **Timeout**: 30 seconds (half-open state attempt)
- **Interval**: 60 seconds (success/failure counter reset)
- **Max Requests**: 3 (half-open state test requests)

**Metrics Exposed**:
- `gateway_circuit_breaker_state{name="k8s-api"}` - State gauge (0=closed, 1=half-open, 2=open)
- `gateway_circuit_breaker_operations_total{name="k8s-api",result="success|failure"}` - Operations counter

**Alert Rules**:
- Alert when circuit breaker is open (K8s API degraded)
- Alert when failure rate exceeds 20% (pre-trip warning)

### **BR-GATEWAY-101: Error Handling**
**Description**: Gateway must handle all errors gracefully and return RFC7807 problem details
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/middleware/error_handler.go`
**Tests**: `test/integration/gateway/rfc7807_compliance_test.go`

### **BR-GATEWAY-102: Timeout Handling**
**Description**: Gateway must enforce timeouts for all external operations
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/config/config.go`
**Tests**: `test/integration/gateway/timeout_handling_test.go`

### **BR-GATEWAY-103: Retry Logic - Redis**
**Description**: Gateway must retry transient Redis errors with exponential backoff
**Priority**: P1 (High)
**Test Coverage**: ❌ Missing
**Implementation**: None
**Tests**: None

### **BR-GATEWAY-104: Retry Logic - Kubernetes API**
**Description**: Gateway must retry transient Kubernetes API errors with exponential backoff
**Priority**: P1 (High)
**Test Coverage**: ❌ Missing
**Implementation**: None
**Tests**: None

### **BR-GATEWAY-105: Backpressure Handling**
**Description**: Gateway must handle backpressure when downstream services are slow
**Priority**: P2 (Medium)
**Test Coverage**: ⏳ Deferred (v2.0)
**Implementation**: None (intentionally not implemented for v1.0)
**Tests**: None (intentional - feature deferred to v2.0)
**Rationale**: Gateway is stateless with minimal processing. No queues or buffering. Synchronous request processing. K8s API backpressure handled by retry logic (BR-111-114). Backpressure handling will be added if async processing or queuing is introduced in v2.0.

### **BR-GATEWAY-106: Resource Limits**
**Description**: Gateway must enforce resource limits (CPU, memory, connections)
**Priority**: P1 (High)
**Test Coverage**: ❌ Missing
**Implementation**: Kubernetes manifests
**Tests**: None

### **BR-GATEWAY-107: Memory Management**
**Description**: Gateway must prevent memory leaks and OOM errors
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: All code
**Tests**: None

### **BR-GATEWAY-108: Goroutine Management**
**Description**: Gateway must prevent goroutine leaks
**Priority**: P0 (Critical)
**Test Coverage**: ❌ Missing
**Implementation**: All code
**Tests**: None

### **BR-GATEWAY-109: Connection Pooling**
**Description**: Gateway must use connection pooling for HTTP clients
**Priority**: P1 (High)
**Test Coverage**: ❌ Missing
**Implementation**: `pkg/gateway/config/config.go`
**Tests**: None

### **BR-GATEWAY-110: Load Shedding**
**Description**: Gateway must implement load shedding when overloaded
**Priority**: P2 (Medium)
**Test Coverage**: ⏳ Deferred (v2.0)
**Implementation**: None (intentionally not implemented for v1.0)
**Tests**: None (intentional - feature deferred to v2.0)
**Rationale**: Rate limiting (BR-038 via proxy - ADR-048) provides sufficient protection. Cluster-wide rate limiting prevents overload. No Redis dependency. No need for additional load shedding. Will be added if Gateway becomes a bottleneck in production.

### **BR-GATEWAY-111: K8s API Retry Configuration**
**Description**: Gateway must support configurable retry behavior for K8s API errors
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit + Integration + E2E
**Implementation**: `pkg/gateway/config/config.go`, `pkg/gateway/processing/crd_creator.go`
**Tests**: `test/unit/gateway/processing/crd_creator_retry_test.go`, `test/integration/gateway/k8s_api_failure_test.go`, `test/e2e/gateway/03_k8s_api_rate_limit_test.go`

### **BR-GATEWAY-112: K8s API Error Classification**
**Description**: Gateway must classify K8s API errors as retryable or non-retryable
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/processing/errors.go`
**Tests**: `test/unit/gateway/processing/crd_creator_retry_test.go`

### **BR-GATEWAY-113: K8s API Exponential Backoff**
**Description**: Gateway must implement exponential backoff for K8s API retries
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit
**Implementation**: `pkg/gateway/processing/crd_creator.go`
**Tests**: `test/unit/gateway/processing/crd_creator_retry_test.go`

### **BR-GATEWAY-114: K8s API Retry Metrics**
**Description**: Gateway must expose metrics for K8s API retry attempts and success rates
**Priority**: P1 (High)
**Test Coverage**: ✅ Unit + Integration
**Implementation**: `pkg/gateway/metrics/metrics.go`
**Tests**: `test/unit/gateway/processing/crd_creator_retry_test.go`, `test/integration/gateway/k8s_api_failure_test.go`

### **BR-GATEWAY-115: K8s API Async Retry Queue**
**Description**: Gateway must support async retry queue for K8s API errors (Phase 2)
**Priority**: P2 (Medium)
**Test Coverage**: ⏳ Planned (Day 15)
**Implementation**: None (planned)
**Tests**: None (planned)

---

## 🔮 **Future Enhancements** (BR-GATEWAY-180+)

### **BR-GATEWAY-180: OpenTelemetry Integration**
**Description**: Gateway must support full OpenTelemetry integration (traces, metrics, logs)
**Priority**: P3 (Low)
**Test Coverage**: ❌ Missing
**Implementation**: None
**Tests**: None

### **BR-GATEWAY-181: Signal Pass-Through Architecture** 🆕
**Description**: Gateway MUST normalize external signals to CRD format WITHOUT interpreting or transforming semantic values (severity, environment, priority). Gateway acts as a "dumb pipe" that extracts and preserves values, never determines policy-based classifications.

**Priority**: P0 (Critical - Blocks customer onboarding)
**Status**: ✅ **COMPLETE** (2026-01-16 - Week 3 from DD-SEVERITY-001)
**Category**: Signal Normalization
**Test Coverage**: 🟡 **Partial** (Adapter refactoring complete, tests pending)

**Acceptance Criteria**:
- [x] Extract severity label from external source → `Spec.Severity` (preserve EXACT value, no transformation) ✅ **COMPLETE**
- [ ] Extract environment label from external source → `Spec.Environment` (preserve EXACT value or empty string, no default) ⏳ **Pending Week 1 CRD changes**
- [ ] Extract priority label from external source → `Spec.Priority` (preserve EXACT value or empty string, no default) ⏳ **Pending Week 1 CRD changes**
- [x] NO hardcoded severity mappings (e.g., `"Sev1"` → `"warning"`) ✅ **COMPLETE**
- [x] NO default fallback values for non-empty strings (e.g., unknown severity → `"warning"`) ✅ **COMPLETE**
- [x] NO transformation logic based on business rules ✅ **COMPLETE**
- [ ] CRD validation MUST accept any string value (not enum-restricted) ⏳ **Waiting on Week 1 CRD schema changes**
- [ ] Audit trail MUST log external→CRD field mappings for debugging ⏳ **Planned**

**Rationale**:
- **Separation of Concerns**: Policy logic (severity/environment/priority determination) belongs in SignalProcessing where full Kubernetes context is available
- **Operator Control**: Severity/environment mappings are operator-defined via SignalProcessing Rego policies, not hardcoded in Gateway
- **Customer Extensibility**: Customers can use ANY severity scheme (Sev1-4, P0-P4, Critical/High/Medium/Low) without Gateway code changes
- **Architectural Consistency**: Matches DD-CATEGORIZATION-001 pattern where Gateway ingests, SignalProcessing categorizes

**Implementation**:
- ✅ `pkg/gateway/adapters/prometheus_adapter.go`: Removed `determineSeverity()` hardcoded switch (**COMPLETE 2026-01-16**)
- ✅ `pkg/gateway/adapters/kubernetes_event_adapter.go`: Removed `mapSeverity()` hardcoded logic (**COMPLETE 2026-01-16**)
- ⏳ `api/remediation/v1alpha1/remediationrequest_types.go`: Remove `+kubebuilder:validation:Enum` from `Spec.Severity` (**Waiting on Week 1**)

**Tests**:
- ⏳ `test/unit/gateway/adapters/prometheus_adapter_test.go`: Verify pass-through (input "Sev1" → output "Sev1") (**Pending creation**)
- ⏳ `test/integration/gateway/custom_severity_test.go`: End-to-end with non-standard severity values (**Pending creation**)

**Related BRs**:
- BR-SP-105 (SignalProcessing Severity Determination via Rego) - **Unblocked by this BR**
- BR-GATEWAY-005 (Signal Metadata Extraction - updated to clarify pass-through)
- BR-GATEWAY-007 (Priority Assignment - deprecated per DD-SEVERITY-001)

**Decision Reference**:
- [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) (Environment/Priority consolidation)
- [DD-SEVERITY-001](../../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) v1.1 (Severity refactoring plan)

**Authority**: [DD-SEVERITY-001](../../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) v1.1, Week 3

---

## 🔐 **Authentication & Authorization** (BR-GATEWAY-182 to BR-GATEWAY-183)

### **BR-GATEWAY-182: ServiceAccount Authentication (TokenReview)**
**Description**: Gateway MUST authenticate incoming webhook requests using Kubernetes TokenReview API to validate ServiceAccount tokens
**Priority**: P0 (Critical)
**Status**: 🚧 In Progress (January 2026)
**Test Coverage**: ⏳ Planned (Unit + Integration + E2E)
**Implementation**: `pkg/gateway/middleware/auth.go` (pending)
**Tests**: 
- `test/unit/gateway/middleware/auth_test.go` (pending)
- `test/integration/gateway/auth_integration_test.go` (pending)
- `test/e2e/gateway/auth_e2e_test.go` (pending)

**Rationale**:
1. **Security**: Gateway is external-facing entry point (Prometheus AlertManager, K8s Event forwarders)
2. **Zero-Trust**: Network Policies alone insufficient for defense-in-depth
3. **SOC2 Compliance**: Operator attribution for signal injection (CC8.1 requirement)
4. **Webhook Compatibility**: AlertManager + K8s Events support Bearer tokens natively

**Authentication Flow**:
```
1. Extract Bearer token from Authorization header
2. Call Kubernetes TokenReview API to validate token
3. Extract user identity (e.g., "system:serviceaccount:monitoring:prometheus-sa")
4. Inject user into request context for audit logging
5. Return 401 Unauthorized if token invalid
```

**User Identity Format**:
- ServiceAccount: `system:serviceaccount:<namespace>:<sa-name>`
- User: `<username>@<domain>`
- System: `system:<component-name>`

**Related Requirements**:
- REQ-3 (DD-AUTH-014): Extract user identity for audit logging
- BR-GATEWAY-054: Audit logging (must capture ActorID)

**Decision Reference**: [DD-AUTH-014 V2.0](../../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md)

**Supersedes**: [DD-GATEWAY-006](../../../architecture/decisions/DD-GATEWAY-006-authentication-strategy.md) (Network Policies only - now obsolete)

---

### **BR-GATEWAY-183: SubjectAccessReview Authorization**
**Description**: Gateway MUST authorize incoming webhook requests using Kubernetes SubjectAccessReview (SAR) to validate RBAC permissions
**Priority**: P0 (Critical)
**Status**: 🚧 In Progress (January 2026)
**Test Coverage**: ⏳ Planned (Unit + Integration + E2E)
**Implementation**: `pkg/gateway/middleware/auth.go` (pending)
**Tests**: 
- `test/unit/gateway/middleware/auth_test.go` (pending)
- `test/integration/gateway/auth_integration_test.go` (pending)
- `test/e2e/gateway/auth_e2e_test.go` (pending)**Authorization Check**:
```
Can <ServiceAccount> CREATE remediationrequests.kubernaut.ai IN <namespace>?
```**SAR Parameters**:
- **User**: Authenticated ServiceAccount name (from BR-GATEWAY-182)
- **Resource**: `remediationrequests.kubernaut.ai` (CRD API group)
- **Verb**: `create` (Gateway creates RemediationRequest CRDs)
- **Namespace**: Target namespace from signal payload (e.g., `monitoring`, `prod`)**Authorization Outcomes**:
- ✅ **Allowed**: SAR returns `allowed=true` → Process signal, create RemediationRequest
- ❌ **Denied**: SAR returns `allowed=false` → Return 403 Forbidden with explanation
- ⚠️ **Error**: SAR API fails → Return 500 Internal Server Error (fail-closed)**RBAC Example** (Prometheus AlertManager):
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-signal-sender
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationrequests"]
  verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prometheus-to-gateway
subjects:
- kind: ServiceAccount
  name: prometheus-sa
  namespace: monitoring
roleRef:
  kind: ClusterRole
  name: gateway-signal-sender
  apiGroup: rbac.authorization.k8s.io
```**Error Handling**:
- **401 Unauthorized**: Token validation fails (BR-GATEWAY-182)
- **403 Forbidden**: SAR denies access (this BR)
- **500 Internal Server Error**: TokenReview/SAR API failure (fail-closed for security)**Performance Considerations** (per DD-AUTH-014 V2.0):
- ✅ No caching: Gateway throughput <100 signals/min (low load)
- ✅ Network Policies: Reduce unauthorized traffic before SAR check
- ✅ Fail-closed: API server unavailability blocks requests (secure default)**Related Requirements**:
- BR-GATEWAY-182: ServiceAccount Authentication (prerequisite)
- BR-GATEWAY-053: RBAC Permissions (general RBAC requirement)**Decision Reference**: [DD-AUTH-014 V2.0](../../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md)**Authority**: DD-AUTH-014 V2.0 (January 29, 2026)
