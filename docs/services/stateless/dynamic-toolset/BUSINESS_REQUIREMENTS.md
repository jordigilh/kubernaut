# Dynamic Toolset Service - Business Requirements

**Service**: Dynamic Toolset Service
**Service Type**: Stateless HTTP API + Kubernetes Controller
**Version**: V2.0 DEFERRED
**Last Updated**: December 7, 2025
**Status**: 🚫 **DEFERRED TO V2.0** - See [DD-016](../../../architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md)

---

## ⚠️ **V2.0 DEFERRAL NOTICE**

All business requirements for this service are **DEFERRED to V2.0** per Design Decision [DD-016](../../../architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md).

**Rationale**: V1.x only requires Prometheus integration, which HolmesGPT-API already handles with built-in service discovery logic. When V2.0 expands HolmesGPT-API to identify multiple observability services (Grafana, Jaeger, Elasticsearch, custom services), this service will provide clear architectural value for centralized multi-service discovery.

**Status**:
- ✅ Implementation **COMPLETE** and code **PRESERVED** for V2.0
- ❌ Business requirements **NOT ACTIVE** in V1.x scope
- ✅ Will **RETURN TO ACTIVE** status in V2.0 planning

---

## 📋 Overview

The **Dynamic Toolset Service** is a stateless HTTP API with Kubernetes controller capabilities that automatically discovers services in a Kubernetes cluster (Prometheus, Grafana, Jaeger, Elasticsearch, custom services) and generates HolmesGPT-compatible toolset ConfigMaps. It provides intelligent service discovery, health validation, and continuous reconciliation to ensure HolmesGPT investigations have access to the latest observability tools.

### Architecture

**Service Type**: Hybrid (Stateless HTTP API + Kubernetes Controller)

**Key Characteristics**:
- Exposes REST API for manual toolset retrieval and discovery triggers
- Watches Kubernetes services across all namespaces for automatic discovery
- Generates HolmesGPT-compatible toolset JSON from discovered services
- Creates and reconciles ConfigMaps with toolset data
- Preserves manual overrides in ConfigMap annotations and labels
- Provides health checks and Prometheus metrics

**Relationship with Other Services**:
- **HolmesGPT API Service**: Consumes toolset ConfigMap for AI investigations
- **Kubernetes API**: Watches services, creates/updates ConfigMaps
- **Discovered Services**: Prometheus, Grafana, Jaeger, Elasticsearch, custom services

### Service Responsibilities

1. **Service Discovery**: Automatically detect observability services using labels and annotations
2. **Toolset Generation**: Generate HolmesGPT-compatible JSON from discovered services
3. **ConfigMap Management**: Create and reconcile ConfigMaps with toolset data
4. **Health Validation**: Validate discovered services are healthy before including
5. **Continuous Updates**: Watch for service changes and update toolsets automatically
6. **Manual Override Preservation**: Preserve admin-configured overrides in ConfigMaps
7. **Observability**: Expose Prometheus metrics for discovery and generation operations

---

## 🎯 Business Requirements

### 📊 Summary

**⚠️ V2.0 SCOPE**: All business requirements below are **DEFERRED to V2.0** per [DD-016](../../../architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md).

**Total Business Requirements**: 11 umbrella BRs (26 granular sub-BRs) - **ALL V2.0 SCOPE**
**Categories**: 7
**V2.0 Priority Breakdown**:
- P0 (Critical - V2.0): 8 BRs (BR-TOOLSET-021, 022, 025, 026, 027, 028, 031, 040)
- P1 (High - V2.0): 3 BRs (BR-TOOLSET-033, 039, 043)

**Test Coverage**:
- Unit: 202 test specs (95% confidence) - includes 8 Content-Type validation tests
- Integration: 56 test scenarios (92% confidence) - includes 6 RFC 7807 + 8 graceful shutdown + 4 Content-Type validation tests
- E2E: 13 test scenarios (90% confidence) - service discovery lifecycle, ConfigMap updates, namespace filtering

**BR Hierarchy**: This service uses **umbrella BRs** (BR-TOOLSET-021, 022, etc.) that map to multiple **granular sub-BRs** (BR-TOOLSET-010 to BR-TOOLSET-035) referenced in test files. See "Sub-BR Mapping" section below for complete traceability.

---

### 🔗 Sub-BR Mapping (Umbrella → Granular)

This section maps high-level umbrella BRs to granular sub-BRs referenced in test files for complete traceability.

#### BR-TOOLSET-021: Automatic Service Discovery
**Maps to 15 granular sub-BRs**:
- **BR-TOOLSET-010**: Prometheus Detector (`test/unit/toolset/prometheus_detector_test.go:18`)
- **BR-TOOLSET-011**: Prometheus Endpoint URL Construction (`test/unit/toolset/prometheus_detector_test.go:98`)
- **BR-TOOLSET-012**: Prometheus Health Check (`test/unit/toolset/prometheus_detector_test.go:195`)
- **BR-TOOLSET-013**: Grafana Detector (`test/unit/toolset/grafana_detector_test.go:18`)
- **BR-TOOLSET-014**: Grafana Endpoint URL Construction (`test/unit/toolset/grafana_detector_test.go:98`)
- **BR-TOOLSET-015**: Grafana Health Check (`test/unit/toolset/grafana_detector_test.go:195`)
- **BR-TOOLSET-016**: Jaeger Detector (`test/unit/toolset/jaeger_detector_test.go:18`)
- **BR-TOOLSET-017**: Jaeger Endpoint URL Construction (`test/unit/toolset/jaeger_detector_test.go:98`)
- **BR-TOOLSET-018**: Jaeger Health Check (`test/unit/toolset/jaeger_detector_test.go:195`)
- **BR-TOOLSET-019**: Elasticsearch Detector (`test/unit/toolset/elasticsearch_detector_test.go:18`)
- **BR-TOOLSET-020**: Elasticsearch Endpoint URL Construction (`test/unit/toolset/elasticsearch_detector_test.go:98`)
- **BR-TOOLSET-021**: Elasticsearch Health Check (`test/unit/toolset/elasticsearch_detector_test.go:195`)
- **BR-TOOLSET-022**: Custom Detector (`test/unit/toolset/custom_detector_test.go:17`)
- **BR-TOOLSET-023**: Custom Endpoint URL Construction (`test/unit/toolset/custom_detector_test.go:103`)
- **BR-TOOLSET-024**: Custom Health Check (`test/unit/toolset/custom_detector_test.go:288`)

**Note**: BR-TOOLSET-021 appears twice (umbrella BR and granular sub-BR for Elasticsearch Health Check). This is intentional for backward compatibility with existing test references.

#### BR-TOOLSET-022: Multi-Detector Orchestration
**Maps to 3 granular sub-BRs**:
- **BR-TOOLSET-025**: Service Discoverer (`test/unit/toolset/service_discoverer_test.go:43`)
- **BR-TOOLSET-026**: Start/Stop Discovery Loop (`test/unit/toolset/service_discoverer_test.go:299`)

**Note**: BR-TOOLSET-022 also appears as a granular sub-BR (Custom Detector) under BR-TOOLSET-021. This dual mapping reflects the service's hybrid architecture (individual detectors + orchestration).

#### BR-TOOLSET-025: Toolset Generation (Umbrella)
**Maps to 2 granular sub-BRs**:
- **BR-TOOLSET-027**: Toolset Generator (`test/unit/toolset/generator_test.go:15`)
- **BR-TOOLSET-028**: HolmesGPT Format Requirements (`test/unit/toolset/generator_test.go:145`)

**Note**: BR-TOOLSET-025 also appears as a granular sub-BR (Service Discoverer) under BR-TOOLSET-022. This dual mapping reflects the end-to-end pipeline (discovery → generation).

#### BR-TOOLSET-026: Continuous Reconciliation (Umbrella)
**Maps to 3 granular sub-BRs**:
- **BR-TOOLSET-029**: ConfigMap Builder (`test/unit/toolset/configmap_builder_test.go:14`)
- **BR-TOOLSET-030**: ConfigMap Overrides Preservation (`test/unit/toolset/configmap_builder_test.go:108`)
- **BR-TOOLSET-031**: ConfigMap Drift Detection (`test/unit/toolset/configmap_builder_test.go:237`)

**Note**: BR-TOOLSET-026 also appears as a granular sub-BR (Start/Stop Discovery Loop) under BR-TOOLSET-022. This dual mapping reflects the continuous reconciliation loop mechanics.

#### BR-TOOLSET-027: Toolset Generation (Umbrella)
**Already mapped above** (see BR-TOOLSET-025 umbrella mapping).

#### BR-TOOLSET-028: HolmesGPT Format Compliance (Umbrella)
**Already mapped above** (see BR-TOOLSET-025 umbrella mapping).

#### BR-TOOLSET-031: ConfigMap Drift Detection (Umbrella)
**Already mapped above** (see BR-TOOLSET-026 umbrella mapping).

#### BR-TOOLSET-033: End-to-End Pipeline (Umbrella)
**Maps to 3 granular sub-BRs**:
- **BR-TOOLSET-032**: Authentication Middleware (`test/unit/toolset/auth_middleware_test.go:18`)
- **BR-TOOLSET-033**: HTTP Server (`test/unit/toolset/server_test.go:22`)
- **BR-TOOLSET-034**: Protected API Endpoints (`test/unit/toolset/server_test.go:110`)
- **BR-TOOLSET-035**: Prometheus Metrics - 9 metrics (45% of original 20) (`test/unit/toolset/metrics_test.go:14`)
  - **DD-TOOLSET-001**: Removed 11 low-value metrics (55%) - REST API disabled, shutdown metrics never scraped
  - **Remaining Metrics**: Service Discovery (4), ConfigMap (3), Toolset Generation (2)

**Note**: BR-TOOLSET-033 appears twice (umbrella BR and granular sub-BR for HTTP Server). This is intentional for backward compatibility with existing test references.

---

### Category 1: Service Discovery

#### BR-TOOLSET-021: Automatic Service Discovery

**Description**: The Dynamic Toolset Service MUST automatically discover services in the Kubernetes cluster by matching labels and annotations for supported service types (Prometheus, Grafana, Jaeger, Elasticsearch, custom services).

**Priority**: P0 (CRITICAL)

**Rationale**: Manual toolset configuration is error-prone and doesn't scale. Automatic discovery ensures HolmesGPT always has access to the latest observability tools without operator intervention.

**Implementation**:
- **5 Service Detectors**:
  1. **Prometheus Detector**: Label-based (`app=prometheus` or `prometheus.io/scrape=true`)
  2. **Grafana Detector**: Label-based (`app=grafana`)
  3. **Jaeger Detector**: Annotation-based (`jaeger.io/enabled=true`)
  4. **Elasticsearch Detector**: Label-based (`app=elasticsearch`)
  5. **Custom Detector**: Annotation-based (`kubernaut.io/toolset=true`)

- **Endpoint Construction**: Kubernetes DNS format (`http://<service-name>.<namespace>:<port>`)
- **Health Check Integration**: Validate service endpoints before including in toolset
- **Multi-Namespace Support**: Discover services across all namespaces

**Acceptance Criteria**:
- ✅ Prometheus services detected via labels
- ✅ Grafana services detected via labels
- ✅ Jaeger services detected via annotations
- ✅ Elasticsearch services detected via labels
- ✅ Custom services detected via annotations
- ✅ Endpoints constructed with correct Kubernetes DNS format
- ✅ Health checks validate service availability
- ✅ Multi-namespace discovery supported

**Test Coverage**:
- Unit: `test/unit/toolset/prometheus_detector_test.go`, `grafana_detector_test.go`, `jaeger_detector_test.go`, `elasticsearch_detector_test.go`, `custom_detector_test.go` (104 scenarios)
- Integration: `test/integration/toolset/service_discovery_test.go` (6 scenarios)
- E2E: Deferred to V2 (in-cluster deployment)

**Related BRs**: BR-TOOLSET-022 (Multi-Detector Orchestration), BR-TOOLSET-027 (Toolset Generation)

---

#### BR-TOOLSET-022: Multi-Detector Discovery Orchestration

**Description**: The Dynamic Toolset Service MUST orchestrate multiple service detectors in parallel, deduplicate discovered services, and handle detector failures gracefully without blocking other detectors.

**Priority**: P0 (CRITICAL)

**Rationale**: Multiple detectors may discover the same service (e.g., Prometheus detected by both label and annotation). Parallel execution improves performance, and graceful error handling ensures partial discovery success.

**Implementation**:
- **Detector Registration**: Register multiple detectors at service startup
- **Parallel Execution**: Run all detectors concurrently using goroutines
- **Deduplication**: Remove duplicate services by name+namespace
- **Error Handling**: Collect errors from individual detectors without blocking others
- **Context Cancellation**: Propagate context cancellation to all detectors
- **Concurrent Safety**: Thread-safe service collection

**Acceptance Criteria**:
- ✅ Multiple detectors execute in parallel
- ✅ Duplicate services deduplicated by name+namespace
- ✅ Detector errors collected without blocking other detectors
- ✅ Context cancellation propagates to all detectors
- ✅ Concurrent safety validated with race detector
- ✅ Empty detector list handled gracefully

**Test Coverage**:
- Unit: `test/unit/toolset/service_discoverer_test.go`, `detector_registration_test.go` (60+ scenarios)
- Integration: `test/integration/toolset/service_discovery_test.go` (5 scenarios)
- E2E: Deferred to V2

**Related BRs**: BR-TOOLSET-021 (Service Discovery), BR-TOOLSET-025 (Advanced Orchestration)

---

### Category 2: Discovery Lifecycle

#### BR-TOOLSET-025: Advanced Multi-Detector Orchestration

**Description**: The Dynamic Toolset Service MUST handle edge cases in multi-detector scenarios including duplicate detection, error aggregation, empty clusters, and partial success scenarios.

**Priority**: P1 (HIGH)

**Rationale**: Production environments have edge cases (empty clusters, detector failures, overlapping patterns). Robust handling ensures reliable discovery in all scenarios.

**Implementation**:
- **Duplicate Detection**: Advanced deduplication when multiple detectors match the same service
- **Error Aggregation**: Collect and report errors from all detectors
- **Empty Cluster Handling**: Return empty list without errors when no services found
- **Partial Success**: Continue discovery when some detectors fail
- **Graceful Degradation**: Provide best-effort results with partial failures

**Acceptance Criteria**:
- ✅ Duplicate services removed even with overlapping detector patterns
- ✅ Errors aggregated from all detectors
- ✅ Empty cluster returns empty list without errors
- ✅ Partial detector failures don't block successful detectors
- ✅ Graceful degradation with partial results

**Test Coverage**:
- Unit: `test/unit/toolset/deduplication_test.go`, `error_handling_test.go` (40+ scenarios)
- Integration: `test/integration/toolset/service_discovery_flow_test.go` (2 scenarios)
- E2E: Deferred to V2

**Related BRs**: BR-TOOLSET-022 (Multi-Detector Orchestration), BR-TOOLSET-026 (Discovery Loop)

---

#### BR-TOOLSET-026: Discovery Loop Lifecycle Management

**Description**: The Dynamic Toolset Service MUST manage continuous discovery with periodic updates, detecting service additions, deletions, and updates, while supporting graceful shutdown via context cancellation.

**Priority**: P1 (HIGH)

**Rationale**: Kubernetes clusters are dynamic. Continuous discovery ensures toolsets stay up-to-date as services are added, removed, or updated. Graceful shutdown prevents data loss during restarts.

**Implementation**:
- **Periodic Discovery**: Configurable discovery interval (default: 5 minutes)
- **Service Addition Detection**: Detect new services in subsequent discovery runs
- **Service Deletion Detection**: Remove deleted services from toolset
- **Service Update Detection**: Update service metadata when services change
- **Context Cancellation**: Stop discovery loop gracefully on context cancel
- **Concurrent Update Handling**: Handle concurrent service updates safely

**Acceptance Criteria**:
- ✅ Discovery loop starts and stops cleanly
- ✅ Service additions detected in subsequent runs
- ✅ Service deletions detected and removed from toolset
- ✅ Service updates reflected in toolset
- ✅ Context cancellation stops loop gracefully
- ✅ Concurrent updates handled safely

**Test Coverage**:
- Unit: `test/unit/toolset/discovery_loop_test.go`, `service_watcher_test.go` (30+ scenarios)
- Integration: `test/integration/toolset/service_discovery_flow_test.go` (5 scenarios)
- E2E: Deferred to V2

**Related BRs**: BR-TOOLSET-025 (Advanced Orchestration), BR-TOOLSET-031 (ConfigMap Management)

---

#### BR-TOOLSET-038: Namespace Requirement

**Description**: The Dynamic Toolset Service MUST write ConfigMaps to the `kubernaut-system` namespace for architectural consistency and RBAC security.

**Priority**: P1 (HIGH)

**Status**: ✅ Active

**Rationale**:
1. **Architectural Consistency**: All kubernaut system components use `kubernaut-system`
2. **RBAC Security**: Least-privilege principle - service only needs access to one namespace
3. **HolmesGPT Integration**: HolmesGPT expects ConfigMap in known namespace

**Acceptance Criteria**:
- **AC-038-01**: ConfigMaps MUST be created in `kubernaut-system` namespace
- **AC-038-02**: Service MUST reject ConfigMap writes to other namespaces
- **AC-038-03**: RBAC permissions MUST be scoped to `kubernaut-system` only
- **AC-038-04**: API specification MUST document namespace requirement

**Test Coverage**:
- Unit: `test/unit/toolset/configmap_builder_test.go:25`

**Implementation**: `pkg/toolset/configmap/builder.go`

**Related BRs**: BR-TOOLSET-021 (ConfigMap Generation)

---

### Category 3: Toolset Generation

#### BR-TOOLSET-027: HolmesGPT Toolset Generation

**Description**: The Dynamic Toolset Service MUST generate HolmesGPT-compatible toolset JSON from discovered services, including all required fields (name, type, endpoint, description, namespace, metadata) with valid JSON schema.

**Priority**: P0 (CRITICAL)

**Rationale**: HolmesGPT requires specific JSON schema for toolset configuration. Invalid JSON or missing fields break AI investigations. Schema compliance is mandatory.

**Implementation**:
- **JSON Generation**: Convert discovered services to HolmesGPT JSON format
- **Schema Validation**: Validate generated JSON against HolmesGPT schema
- **Required Fields**: Include name, type, endpoint, description, namespace
- **Optional Fields**: Include metadata (labels, annotations)
- **Metadata Preservation**: Preserve service metadata in generated JSON
- **Error Handling**: Handle malformed services gracefully

**Acceptance Criteria**:
- ✅ Valid JSON generated from discovered services
- ✅ Schema validation passes for HolmesGPT compatibility
- ✅ All required fields present (name, type, endpoint, description, namespace)
- ✅ Metadata preserved in generated JSON
- ✅ Deduplication in generator
- ✅ Error handling for malformed services

**Test Coverage**:
- Unit: `test/unit/toolset/generator_test.go`, `json_validation_test.go` (50+ scenarios)
- Integration: `test/integration/toolset/generator_integration_test.go` (4 scenarios)
- E2E: Deferred to V2

**Related BRs**: BR-TOOLSET-028 (Tool Structure), BR-TOOLSET-031 (ConfigMap Management)

---

#### BR-TOOLSET-028: HolmesGPT Tool Structure Requirements

**Description**: The Dynamic Toolset Service MUST ensure generated tools include all required fields with correct types and formats, including name (string), type (string), endpoint (URL), description (string), namespace (string), and metadata (object).

**Priority**: P0 (CRITICAL)

**Rationale**: HolmesGPT SDK expects specific field types and formats. Missing or incorrectly typed fields cause investigation failures. Strict validation prevents runtime errors.

**Implementation**:
- **Required Field Validation**: Validate presence of name, type, endpoint, description, namespace
- **Optional Field Handling**: Handle metadata field gracefully (may be empty)
- **Field Type Validation**: Ensure correct types (strings, URLs, objects)
- **Endpoint URL Format**: Validate endpoint is valid URL
- **Description Generation**: Auto-generate descriptions for services without explicit descriptions
- **Metadata Structure**: Ensure metadata is valid JSON object

**Acceptance Criteria**:
- ✅ Required fields validated (name, type, endpoint, description, namespace)
- ✅ Optional fields handled (metadata)
- ✅ Field types validated (strings, URLs, objects)
- ✅ Endpoint URL format validated
- ✅ Descriptions auto-generated when missing
- ✅ Metadata structure validated

**Test Coverage**:
- Unit: `test/unit/toolset/tool_structure_test.go`, `field_validation_test.go` (50+ scenarios)
- Integration: `test/integration/toolset/generator_integration_test.go` (4 scenarios)
- E2E: Deferred to V2

**Related BRs**: BR-TOOLSET-027 (Toolset Generation), BR-TOOLSET-031 (ConfigMap Management)

---

### Category 4: ConfigMap Management

#### BR-TOOLSET-031: ConfigMap Creation and Reconciliation

**Description**: The Dynamic Toolset Service MUST create and reconcile ConfigMaps with toolset data, preserving manual overrides in labels and annotations, and handling ConfigMap not found scenarios gracefully.

**Priority**: P1 (HIGH)

**Rationale**: ConfigMaps are the primary storage mechanism for toolsets. Reconciliation prevents drift and accidental deletion. Manual overrides allow admins to customize toolsets without service restarts.

**Implementation**:
- **ConfigMap Creation**: Create ConfigMap with correct structure and labels
- **Label Management**: Apply `app=kubernaut` label (not `kubernaut-dynamic-toolset`)
- **Annotation Management**: Apply service-managed annotations
- **Override Preservation**: Preserve manual overrides in labels and annotations
- **Reconciliation Strategy**: Update ConfigMap data while preserving overrides
- **Error Handling**: Handle ConfigMap not found, conflicts, and API errors
- **Validation**: Validate ConfigMap data before creation/update

**Acceptance Criteria**:
- ✅ ConfigMap created with correct structure
- ✅ Labels applied correctly (`app=kubernaut`)
- ✅ Annotations applied correctly
- ✅ Manual overrides preserved in labels
- ✅ Manual overrides preserved in annotations
- ✅ Reconciliation updates data without losing overrides
- ✅ ConfigMap not found handled gracefully
- ✅ Conflicts resolved with retry

**Test Coverage**:
- Unit: `test/unit/toolset/configmap_builder_test.go`, `configmap_reconciliation_test.go` (40+ scenarios)
- Integration: `test/integration/toolset/configmap_test.go` (5 scenarios)
- E2E: Deferred to V2

**Related BRs**: BR-TOOLSET-027 (Toolset Generation), BR-TOOLSET-033 (End-to-End Flow)

---

### Category 5: End-to-End Workflows

#### BR-TOOLSET-033: Complete Discovery-to-ConfigMap Pipeline

**Description**: The Dynamic Toolset Service MUST provide a complete end-to-end workflow from service discovery through ConfigMap creation and continuous updates, including error recovery, state management, and reconciliation.

**Priority**: P0 (CRITICAL)

**Rationale**: End-to-end workflow validation ensures all components work together correctly. Error recovery and state management are critical for production reliability.

**Implementation**:
- **Complete Pipeline**: Discovery → Generation → ConfigMap Creation → Reconciliation
- **Continuous Reconciliation**: Watch services and update ConfigMaps automatically
- **Error Recovery**: Retry failed operations with exponential backoff
- **State Management**: Track discovery state across components
- **Service Watching**: Watch Kubernetes services for changes
- **Custom Annotation Preservation**: Preserve custom annotations during reconciliation
- **Namespace Lifecycle**: Handle namespace creation and deletion

**Acceptance Criteria**:
- ✅ Complete pipeline from discovery to ConfigMap creation
- ✅ Continuous reconciliation with service watching
- ✅ Error recovery with retry
- ✅ State management across components
- ✅ Service updates reflected in ConfigMap
- ✅ Custom annotations preserved during reconciliation
- ✅ Namespace creation and deletion handled

**Test Coverage**:
- Unit: `test/unit/toolset/end_to_end_flow_test.go` (30+ scenarios)
- Integration: `test/integration/toolset/service_discovery_flow_test.go` (5 scenarios)
- E2E: Deferred to V2

**Related BRs**: BR-TOOLSET-021 (Service Discovery), BR-TOOLSET-027 (Toolset Generation), BR-TOOLSET-031 (ConfigMap Management)

---

#### BR-TOOLSET-039: RFC 7807 Error Response Standard

**Priority**: P1 (Production Readiness)
**Status**: ✅ Implemented
**Category**: HTTP API & Error Handling

**Description**: HTTP API returns RFC 7807 Problem Details for all error responses

**Acceptance Criteria**:
- All HTTP error responses use `application/problem+json` Content-Type
- Error responses include all required RFC 7807 fields: `type`, `title`, `detail`, `status`, `instance`
- Error type URIs follow convention: `https://kubernaut.io/errors/{error-type}`
- Request ID included in error responses for tracing
- Supports standard HTTP error codes: 400 (Bad Request), 405 (Method Not Allowed), 500 (Internal Error), 503 (Service Unavailable)

**Test Coverage**:
- **Integration**: `test/integration/toolset/rfc7807_compliance_test.go` (6 tests)
  - Test 1: Bad Request (400) with invalid JSON
  - Test 2: Method Not Allowed (405)
  - Test 3: Service Unavailable (503) during shutdown
  - Test 4: All required RFC 7807 fields validation
  - Test 5: Error type URI format validation
  - Test 6: Request ID inclusion and propagation

**Implementation**:
- `pkg/toolset/errors/rfc7807.go`: RFC7807Error struct and helper functions
- `pkg/toolset/server/middleware/request_id.go`: Request ID middleware for tracing
- `pkg/toolset/server/server.go`: writeJSONError() helper for all error responses
- `pkg/toolset/metrics/metrics.go`: ErrorResponsesTotal metric for error tracking

**Related BRs**: BR-TOOLSET-033 (HTTP Server), BR-TOOLSET-040 (Graceful Shutdown)

**Related Design Decisions**:
- **ADR-036**: Authentication and Authorization Strategy (auth handled by sidecars/network policies)

---

#### BR-TOOLSET-040: Graceful Shutdown with In-Flight Request Completion

**Priority**: P0 (Production Safety)
**Status**: ✅ Implemented
**Category**: Kubernetes Operations & Reliability

**Description**: Service implements DD-007 4-step Kubernetes-aware graceful shutdown pattern for zero-downtime deployments

**Acceptance Criteria**:
- Readiness probe returns 503 during shutdown to signal Kubernetes endpoint removal
- Liveness probe remains healthy during shutdown (prevents premature pod termination)
- Waits 5 seconds for Kubernetes endpoint removal propagation across all nodes
- Completes all in-flight HTTP requests before terminating
- Closes external resources (Kubernetes client, discovery loop, metrics server) cleanly
- Respects shutdown context timeout
- Handles concurrent shutdown calls safely
- Logs all shutdown steps with DD-007 tags for observability

**Test Coverage**:
- **Integration**: `test/integration/toolset/graceful_shutdown_test.go` (8 tests)
  - Test 1: Readiness probe coordination (P0) - Returns 503 during shutdown
  - Test 2: Liveness probe during shutdown (P0) - Remains healthy
  - Test 3: In-flight request completion (P0) - Completes active requests
  - Test 4: Resource cleanup (P1) - Closes Kubernetes client connections
  - Test 5: Shutdown timing (P1) - Waits 5 seconds for endpoint propagation
  - Test 6: Shutdown timeout respect (P1) - Respects context timeout
  - Test 7: Concurrent shutdown safety (P2) - Handles concurrent calls
  - Test 8: Shutdown logging (P2) - Logs all DD-007 steps

**Implementation**:
- `pkg/toolset/server/server.go`:
  - `Shutdown()`: 4-step DD-007 pattern implementation
  - `shutdownStep1SetFlag()`: Set shutdown flag for readiness probe
  - `shutdownStep2WaitForPropagation()`: Wait 5s for endpoint removal
  - `shutdownStep3DrainConnections()`: Drain in-flight HTTP connections
  - `shutdownStep4CloseResources()`: Close external resources
  - `handleReady()`: Check shutdown flag and return 503 during shutdown
  - `isShuttingDown atomic.Bool`: Thread-safe shutdown coordination flag

**Business Impact**:
- **Zero request failures** during rolling updates (vs 5-10% baseline without DD-007)
- Production-proven pattern from Gateway and Context API services
- Industry best practice: 5-second wait for Kubernetes endpoint propagation

**Related BRs**: BR-TOOLSET-033 (HTTP Server), BR-TOOLSET-039 (RFC 7807 Error Responses)

**Related Design Decisions**:
- **DD-007**: Kubernetes-Aware Graceful Shutdown Pattern

---

#### BR-TOOLSET-043: Content-Type Validation Middleware

**Priority**: P1 (Security & API Standards)
**Status**: ✅ Implemented
**Category**: API Security & Standards Compliance

**Description**: All POST, PUT, and PATCH endpoints validate the `Content-Type` header and reject requests with invalid or missing Content-Type with a 415 Unsupported Media Type error in RFC 7807 format

**Business Value**:
- **Security**: Prevents MIME confusion attacks and malformed request processing
- **API Contract**: Enforces strict Content-Type requirements per RFC 7231
- **Client Clarity**: Clear error messages when Content-Type is incorrect
- **Standards Compliance**: Follows REST API best practices

**Acceptance Criteria**:
- POST, PUT, PATCH endpoints validate `Content-Type` header
- Accept `application/json` (with or without `charset` parameter)
- Reject requests with missing or invalid Content-Type (415 error)
- Return RFC 7807 error response for invalid Content-Type
- GET, DELETE, HEAD, OPTIONS requests are not affected
- Prometheus metrics track Content-Type validation failures
- Structured logging for all validation failures

**Test Coverage**:
- **Unit**: `pkg/toolset/server/middleware/content_type_test.go` (8 tests)
  - Test 1: Valid `application/json` - processed successfully
  - Test 2: Valid `application/json; charset=utf-8` - processed successfully
  - Test 3: Invalid `text/plain` - 415 with RFC 7807 error
  - Test 4: Invalid `text/html` - 415 with RFC 7807 error
  - Test 5: Invalid `application/xml` - 415 with RFC 7807 error
  - Test 6: Invalid `multipart/form-data` - 415 with RFC 7807 error
  - Test 7: Missing Content-Type - 415 with RFC 7807 error
  - Test 8: GET request - not validated (200 OK)
- **Integration**: `test/integration/toolset/content_type_validation_test.go` (4 tests)
  - Test 1: POST with valid `application/json` - processed successfully
  - Test 2: POST with invalid `text/plain` - 415 with RFC 7807 error
  - Test 3: POST with missing Content-Type - 415 with RFC 7807 error
  - Test 4: GET without Content-Type - not validated (200 OK)

**Implementation**:
- `pkg/toolset/server/middleware/content_type.go`: ValidateContentType middleware
- `pkg/toolset/metrics/metrics.go`: ContentTypeValidationErrors metric
- `pkg/toolset/server/server.go`: Middleware registration in chain

**Endpoints Affected**:
- POST `/api/v1/toolsets/validate` - Toolset validation endpoint
- POST `/api/v1/toolsets/generate` - Toolset generation endpoint
- (Future: Any POST/PUT/PATCH endpoints added to the service)

**Error Response Format** (RFC 7807):
```json
{
  "type": "https://kubernaut.io/errors/unsupported-media-type",
  "title": "Unsupported Media Type",
  "detail": "Content-Type must be 'application/json', got 'text/plain'",
  "status": 415,
  "instance": "/api/v1/toolsets/validate",
  "request_id": "req-123"
}
```

**Related BRs**: BR-TOOLSET-039 (RFC 7807 Error Response Standard), BR-TOOLSET-033 (HTTP Server)

**Related Design Decisions**:
- **DD-004**: RFC 7807 Problem Details for HTTP APIs

**Reference Implementations**:
- Gateway Service: `pkg/gateway/middleware/content_type.go`
- Context API Service: `pkg/contextapi/middleware/content_type.go`

---

## 📊 Test Coverage Summary

### Unit Tests
- **Total**: 194 test specs
- **Coverage**: 100% pass rate
- **Files**: 13 test files
- **Confidence**: 95%

### Integration Tests
- **Total**: 52 test specs (38 existing + 6 RFC 7807 + 8 graceful shutdown)
- **Coverage**: 100% pass rate
- **Files**: 6 test files
- **Confidence**: 92%

### E2E Tests
- **Status**: Implemented in V1.1
- **Test Files**: `test/e2e/toolset/01_discovery_lifecycle_test.go`, `test/e2e/toolset/02_configmap_updates_test.go`, `test/e2e/toolset/03_namespace_filtering_test.go`
- **Infrastructure**: Kind cluster with per-namespace Dynamic Toolset deployment
- **Test Scenarios**: 13 scenarios across 3 test files
  - **Discovery Lifecycle** (6 scenarios): Service discovery, priority ordering, service deletion, annotation handling, custom types, rapid churn
  - **ConfigMap Updates** (4 scenarios): Annotation changes, concurrent updates, ConfigMap recreation, JSON validation
  - **Namespace Filtering** (3 scenarios): Namespace-scoped discovery, RBAC validation, ConfigMap isolation
- **Confidence**: 90%
- **Advanced Scenarios**: Deferred to V2 (multi-cluster, RBAC restrictions, large-scale discovery)

### BR Coverage
- **Total BRs**: 10
- **Unit Test Coverage**: 80% (8/10 BRs) - BR-TOOLSET-039 and BR-TOOLSET-040 tested at integration level
- **Integration Test Coverage**: 100% (10/10 BRs)
- **E2E Test Coverage**: 60% (6/10 BRs) - BR-TOOLSET-016, 017, 018, 019, 020, 021 (discovery, updates, namespace filtering)
- **Overall Coverage**: 100%

---

## 🔗 Related Documentation

- [Dynamic Toolset Service README](./README.md) - Service overview and quick navigation
- [BR Coverage Matrix](./BR_COVERAGE_MATRIX.md) - Detailed test coverage mapping
- [Production Readiness Report](./implementation/PRODUCTION_READINESS_REPORT.md) - 101/109 points (92.7%)
- [Handoff Summary](./implementation/00-HANDOFF-SUMMARY.md) - Complete implementation summary
- [Implementation Plan](./implementation/IMPLEMENTATION_PLAN_ENHANCED.md) - 12-day implementation timeline
- [Testing Strategy](./implementation/testing/TESTING_STRATEGY.md) - Comprehensive test approach

---

## 📝 Version History

### Version 1.1.0 (2025-11-09)
- Added E2E test suite with 13 test scenarios
- Implemented service discovery lifecycle tests (6 scenarios)
- Implemented ConfigMap update tests (4 scenarios)
- Implemented namespace filtering tests (3 scenarios)
- E2E infrastructure: Kind cluster, per-namespace deployment, mock services
- 259 total test specs passing (194 unit + 52 integration + 13 E2E)
- 100% pass rate across all test tiers

### Version 1.0.1 (2025-11-09)
- Added RFC 7807 error response standard (BR-TOOLSET-039)
- Added DD-007 graceful shutdown pattern (BR-TOOLSET-040)
- 10 business requirements implemented
- 100% BR coverage (integration tests)
- 246 test specs passing (100% pass rate)
- Production readiness enhanced with zero-downtime deployments

### Version 1.0 (2025-10-13)
- Initial production-ready release
- 8 business requirements implemented
- 100% BR coverage (unit + integration tests)
- 232 test specs passing (100% pass rate)
- 101/109 production readiness points (92.7%)

---

**Document Version**: 1.1.0
**Last Updated**: November 9, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: Production-Ready



### Version 1.0 (2025-10-13)
- Initial production-ready release
- 8 business requirements implemented
- 100% BR coverage (unit + integration tests)
- 232 test specs passing (100% pass rate)
- 101/109 production readiness points (92.7%)

---

**Document Version**: 1.1.0
**Last Updated**: November 9, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: Production-Ready

