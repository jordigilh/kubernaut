# DD-GATEWAY-002: OpenTelemetry Adapter for Gateway Service

## Status
**📋 PLANNING** (2025-10-21)
**Last Reviewed**: 2025-10-21
**Confidence**: 78%
**Target Version**: Kubernaut V1.1 (Q1 2026)
**Current Gateway Version**: V1.0 (Prometheus + Kubernetes Events only)

---

## Context & Problem

### Problem Statement
Gateway Service V1.0 currently supports only **Prometheus alerts** and **Kubernetes events** as signal sources. Many organizations have adopted **OpenTelemetry** as their observability standard, collecting distributed traces that contain valuable error and performance signals. These traces could trigger automated remediation, but Gateway cannot currently ingest them.

**Business Need**:
- **BR-GATEWAY-024 to BR-GATEWAY-040**: Organizations using OpenTelemetry want automatic remediation triggered by trace errors and high-latency spans
- **Industry Trend**: OpenTelemetry is becoming the de facto standard for observability (CNCF incubating project with broad adoption)
- **Complementary Signals**: Traces provide deeper context than metrics-based alerts (full request path, span attributes, error details)

### Key Requirements

1. **Trace-Based Signal Detection**: Identify error spans and high-latency traces that warrant remediation
2. **OTLP Protocol Support**: Parse OpenTelemetry Protocol (OTLP) in both HTTP and gRPC formats
3. **Resource Mapping**: Map OpenTelemetry service names to Kubernetes resources (deployments, pods)
4. **Signal Criteria Configuration**: Allow operators to define what constitutes a "signal" (error types, latency thresholds)
5. **Architecture Consistency**: Follow existing `SignalAdapter` pattern (no Gateway refactoring)
6. **Mitigation Integration**: Seamless integration with existing remediation pipeline (CRD-based)

---

## Alternatives Considered

### Alternative 1: Error Span-Based Signals (Recommended)

**Approach**: Trigger signals when OpenTelemetry spans have `status.code == ERROR`

**Implementation**:
```go
// OpenTelemetryAdapter extracts error spans from OTLP traces
type OpenTelemetryAdapter struct{}

func (a *OpenTelemetryAdapter) Parse(ctx context.Context, rawData []byte) (*types.NormalizedSignal, error) {
    // 1. Unmarshal OTLP format (JSON or protobuf)
    var traces TracesData
    if err := unmarshalOTLP(rawData, &traces); err != nil {
        return nil, err
    }

    // 2. Extract error spans (status.code == ERROR)
    errorSpan := findFirstErrorSpan(traces)
    if errorSpan == nil {
        return nil, errors.New("no error spans found")
    }

    // 3. Map service name to Kubernetes resource
    resource := types.ResourceIdentifier{
        Kind:      extractKindFromAttributes(errorSpan),
        Name:      extractNameFromAttributes(errorSpan),
        Namespace: extractNamespaceFromAttributes(errorSpan),
    }

    // 4. Generate fingerprint: SHA256(service:operation:error_type)
    fingerprint := calculateFingerprint(
        errorSpan.ServiceName,
        errorSpan.OperationName,
        errorSpan.Status.Message,
    )

    // 5. Convert to NormalizedSignal
    return &types.NormalizedSignal{
        Fingerprint:  fingerprint,
        AlertName:    fmt.Sprintf("TraceError_%s", errorSpan.ServiceName),
        Severity:     "critical", // Error spans are critical
        Namespace:    resource.Namespace,
        Resource:     resource,
        SourceType:   "opentelemetry-trace",
        Source:       "opentelemetry-adapter",
        FiringTime:   errorSpan.StartTime,
        ReceivedTime: time.Now(),
        RawPayload:   rawData,
    }, nil
}
```

**Pros**:
- ✅ **Simple Logic**: Clear criterion (span.status.code == ERROR)
- ✅ **Low False Positives**: Errors explicitly indicate problems
- ✅ **Fast Implementation**: ~40 hours for MVP
- ✅ **Industry Standard**: OpenTelemetry spec defines error semantics
- ✅ **Easy Testing**: Straightforward error injection in tests

**Cons**:
- ❌ **Misses Performance Issues**: High latency without errors not detected
- ❌ **Application Dependency**: Requires apps to set error status correctly
- ❌ **Single Signal Type**: Only errors, no latency-based signals

**Confidence**: 85% (approved for MVP)

---

### Alternative 2: Multi-Criteria Signal Detection (Latency + Errors)

**Approach**: Trigger signals based on **error spans OR high-latency spans** (configurable thresholds)

**Implementation**:
```go
type SignalCriteria struct {
    EnableErrorSpans    bool          `yaml:"enableErrorSpans"`
    EnableLatencySpans  bool          `yaml:"enableLatencySpans"`
    LatencyThreshold    time.Duration `yaml:"latencyThreshold"` // e.g., 5s
}

func (a *OpenTelemetryAdapter) Parse(ctx context.Context, rawData []byte) (*types.NormalizedSignal, error) {
    traces := unmarshalOTLP(rawData)

    // Check error spans
    if a.criteria.EnableErrorSpans {
        if span := findFirstErrorSpan(traces); span != nil {
            return createSignalFromSpan(span, "error")
        }
    }

    // Check high-latency spans
    if a.criteria.EnableLatencySpans {
        if span := findHighLatencySpan(traces, a.criteria.LatencyThreshold); span != nil {
            return createSignalFromSpan(span, "latency")
        }
    }

    return nil, errors.New("no spans match signal criteria")
}
```

**Pros**:
- ✅ **Comprehensive**: Detects both errors and performance issues
- ✅ **Flexible**: Configurable criteria per environment
- ✅ **Better Coverage**: Catches slow operations without explicit errors
- ✅ **Operator Control**: SREs define what triggers remediation

**Cons**:
- ❌ **Complex Logic**: Multiple criteria to validate
- ❌ **False Positives**: Latency spikes may not need remediation
- ❌ **Threshold Tuning**: Requires per-service latency baselines
- ❌ **Longer Implementation**: +20-30 hours vs Alternative 1

**Confidence**: 75% (approved for production-ready version)

---

### Alternative 3: Rate-Based Signal Aggregation (Advanced)

**Approach**: Aggregate multiple error spans from the same service into a **single signal** when error rate exceeds threshold

**Implementation**:
```go
type RateAggregator struct {
    window     time.Duration // e.g., 1 minute
    threshold  int           // e.g., 100 errors/minute
    errorCache map[string]*ErrorBucket
}

func (a *RateAggregator) ShouldCreateSignal(span *Span) bool {
    serviceKey := span.ServiceName
    bucket := a.errorCache[serviceKey]

    // Increment error count
    bucket.ErrorCount++

    // Check if threshold exceeded
    if bucket.ErrorCount > a.threshold {
        // Create aggregated signal
        return true
    }

    return false
}
```

**Pros**:
- ✅ **Reduces Signal Volume**: 1000 error spans → 1 signal
- ✅ **Better Signal-to-Noise**: Only high-error-rate services trigger remediation
- ✅ **Prevents Storm**: Similar to existing alert storm detection
- ✅ **Scaling**: Handles high-volume trace data

**Cons**:
- ❌ **Delayed Detection**: Waits for rate threshold (slower remediation)
- ❌ **Stateful**: Requires Redis or in-memory cache for error counts
- ❌ **Complexity**: Aggregation logic, cache eviction, edge cases
- ❌ **Risk**: May miss single critical errors (e.g., payment failure)

**Confidence**: 60% (deferred to Kubernaut V1.2+)

---

## Decision

**APPROVED: Alternative 1 + Alternative 2** (Phased Approach)

**Phase 1 (Kubernaut V1.1 MVP)**: Alternative 1 - Error Span-Based Signals
**Phase 2 (Kubernaut V1.1 Production)**: Alternative 2 - Add Latency-Based Signals
**Phase 3 (Kubernaut V1.2+)**: Alternative 3 - Rate-Based Aggregation (optional)

**Rationale**:
1. **MVP Speed**: Alternative 1 provides immediate value (40 hours implementation)
2. **Incremental Improvement**: Add latency detection after validating error-based approach
3. **Risk Mitigation**: Phased rollout reduces complexity and validates assumptions
4. **Operator Feedback**: Learn from V1.1 MVP before adding advanced features

**Key Insight**: OpenTelemetry traces are **complementary** to Prometheus alerts, not replacements. Traces provide deeper context (full request path, span attributes) while alerts provide aggregated metrics. Both signal types enhance remediation accuracy.

---

## Implementation

### Primary Implementation Files

**New Files (to be created)**:
- `pkg/gateway/adapters/opentelemetry_adapter.go` - OpenTelemetry adapter implementation
- `pkg/gateway/adapters/opentelemetry_types.go` - OTLP data structures
- `test/unit/gateway/adapters/opentelemetry_adapter_test.go` - Unit tests
- `test/integration/gateway/opentelemetry_integration_test.go` - Integration tests

**Modified Files**:
- `pkg/gateway/server.go` - Register OpenTelemetry adapter (add 3 lines)
- `config/development.yaml` - Add OpenTelemetry configuration section
- `docs/architecture/CRD_SCHEMAS.md` - Document OpenTelemetry providerData format

**No Changes Required**:
- `api/remediation/v1/remediationrequest_types.go` - Already supports multi-signal via `providerData`
- Downstream services (RemediationOrchestrator, AIAnalysis, WorkflowExecution) - All CRD-based, signal-agnostic

---

### Data Flow

**1. OpenTelemetry Collector Sends Trace**:
```
OTel Collector → POST /api/v1/signals/opentelemetry
Content-Type: application/json (OTLP/HTTP)
Body: { "resourceSpans": [...] }
```

**2. Gateway Processes Signal**:
```
OpenTelemetryAdapter.Parse()
  → Extract error span
  → Map service name to K8s resource (from span attributes)
  → Generate fingerprint: SHA256(service:operation:error_type)
  → Return NormalizedSignal
```

**3. Gateway Creates RemediationRequest CRD**:
```yaml
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
spec:
  signalType: "opentelemetry-trace"
  targetType: "kubernetes"
  priority: "P0"
  providerData: |
    {
      "traceId": "abc123...",
      "spanId": "def456...",
      "serviceName": "payment-api",
      "operation": "POST /api/v1/payments",
      "duration": "8.5s",
      "errorType": "DeadlineExceeded",
      "errorMessage": "deadline exceeded",
      "spanAttributes": { ... }
    }
```

**4. Existing Remediation Pipeline Executes**:
```
RemediationRequest → RemediationOrchestrator → RemediationProcessing
  → AIAnalysis (HolmesGPT investigates with trace context)
  → WorkflowExecution (creates Tekton pipeline)
  → Kubernetes applies remediation (e.g., scale deployment)
```

---

### Graceful Degradation

**If OpenTelemetry Adapter Unavailable**:
- Gateway continues processing Prometheus and Kubernetes Event signals
- Configuration: `adapters.opentelemetry.enabled: false`
- No impact on existing signal sources

**If Span Attributes Missing**:
```go
// Fallback: Query K8s API for matching deployments
if resource.Name == "" {
    deployments, err := k8sClient.ListDeployments(ctx, namespace)
    // Match by label: app=service-name
    resource = findMatchingDeployment(deployments, serviceName)
}
```

**If Trace Context Invalid**:
- Return HTTP 400 Bad Request (same as Prometheus/K8s Event parse errors)
- Log parse error for debugging
- No CRD created (prevent invalid remediation)

---

## Consequences

### Positive

- ✅ **Industry Standard Integration**: OpenTelemetry is CNCF incubating project with broad adoption
- ✅ **Deeper Signal Context**: Traces provide full request path, span attributes, error details
- ✅ **Complementary to Alerts**: Traces + Metrics = comprehensive observability
- ✅ **Architecture Consistency**: Follows existing `SignalAdapter` pattern (70% code reuse)
- ✅ **Zero Downstream Changes**: CRD-based architecture is signal-agnostic
- ✅ **Operator Flexibility**: Configurable signal criteria per environment
- ✅ **Phased Rollout**: MVP → Production → Advanced features (risk mitigation)

### Negative

- ⚠️ **Application Dependency** - **Mitigation**: Require OTel SDK with Kubernetes resource detector in app deployment guide
  - Applications must instrument with OpenTelemetry SDK
  - Span attributes must include `k8s.namespace.name`, `k8s.pod.name` for resource mapping
  - Without K8s attributes, fallback to service name → K8s API lookup (slower, less accurate)

- ⚠️ **Signal Volume Risk** - **Mitigation**: Implement span sampling at OTel Collector level
  - High-throughput services generate 1000s of spans/second
  - Every error span → potential signal → potential CRD creation
  - Could overwhelm Gateway and Kubernetes API server
  - **Solution**: Span sampling (10% sampling = 90% volume reduction), rate-based aggregation (Phase 3)

- ⚠️ **OTLP Protocol Complexity** - **Mitigation**: Use `go.opentelemetry.io/collector` libraries (battle-tested)
  - OTLP has gRPC and HTTP variants
  - Protobuf encoding adds parsing complexity
  - **Solution**: Use official OTel Go libraries, start with JSON-encoded OTLP (simpler), add protobuf in Phase 2

- ⚠️ **Resource Mapping Accuracy** - **Mitigation**: Document required span attributes in deployment guide
  - OTel service name may not match K8s deployment name
  - Example: service = "payment-api-v2", deployment = "payment-api"
  - **Solution**: Require span attributes (`k8s.deployment.name`), fallback to label matching (`app=service-name`)

### Neutral

- 🔄 **Additional Testing Required**: +40 tests following defense-in-depth pyramid per [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)
  - Unit: +25 tests (70%+ BR coverage = extensive business logic validation)
  - Integration: +10 tests (>50% BR coverage = cross-service flows, CRD coordination)
  - E2E: +5 tests (10-15% BR coverage = complete remediation workflows)
  - **Defense-in-depth**: Some BRs tested at multiple levels (e.g., BR-GATEWAY-024 in unit, integration, AND e2e for comprehensive validation)
- 🔄 **Documentation Updates**: Deployment guide, architecture docs, CRD schema docs, testing strategy docs
- 🔄 **Operational Complexity**: Operators must deploy OTel Collector, configure exporters, instrument applications with OTel SDK

---

## Validation Results

### Confidence Assessment Progression

- **Initial assessment**: 70% confidence (feasibility study)
- **After architecture analysis**: 78% confidence (CRD schema supports multi-signal)
- **After adapter pattern review**: 85% confidence (proven pattern, 70% code reuse)
- **After risk mitigation design**: 78% confidence (span volume risk identified, mitigations planned)

### Key Validation Points

- ✅ **Architecture Readiness**: CRD `providerData` field supports arbitrary JSON (no schema changes)
- ✅ **Adapter Pattern Proven**: Prometheus and K8s Event adapters follow same interface
- ✅ **Downstream Service Compatibility**: RemediationOrchestrator, AIAnalysis, WorkflowExecution are signal-agnostic
- ✅ **Industry Precedent**: Other AIOps tools support OTel traces (AWS X-Ray, Datadog)
- ✅ **Library Availability**: `go.opentelemetry.io/proto/otlp` provides OTLP parsing

---

## Related Decisions

- **Builds On**: [DD-GATEWAY-001](mdc:DD-GATEWAY-001-adapter-specific-endpoints.md) - Adapter-specific endpoints architecture
- **Supports**: [ADR-015](mdc:ADR-015-alert-to-signal-naming-migration.md) - Alert → Signal naming migration
- **Relates To**: [BR-GATEWAY-024 to BR-GATEWAY-040](../../services/stateless/gateway-service/IMPLEMENTATION_PLAN_V1.0.md#br-gateway-024-to-040-opentelemetry-adapter-kubernaut-v11---future)

---

## Review & Evolution

### When to Revisit

- **After MVP Deployment**: Evaluate error-based signals effectiveness (Q1 2026)
- **If Signal Volume Issues**: Implement rate-based aggregation (Alternative 3)
- **If Resource Mapping Failures**: Enhance K8s API fallback logic
- **If False Positives High**: Add custom signal rules (BR-GATEWAY-039)

### Success Metrics

- **Signal Accuracy**: >90% of OpenTelemetry signals map to correct Kubernetes resource
- **Parse Performance**: <10ms to parse OTLP trace and extract error span
- **False Positive Rate**: <10% of OpenTelemetry signals result in unnecessary remediation
- **Deduplication Effectiveness**: >80% of duplicate error spans deduplicated
- **Integration Success**: Zero changes required to downstream services

**Testing Metrics** (per [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)):
- **Unit Test Coverage**: 70%+ of BR-GATEWAY-024 to 040 (12+ of 17 BRs)
- **Integration Test Coverage**: >50% of BR-GATEWAY-024 to 040 (9+ of 17 BRs)
- **E2E Test Coverage**: 10-15% of BR-GATEWAY-024 to 040 (2-3 of 17 BRs)
- **Defense-in-Depth**: Key BRs tested at multiple levels (unit + integration + e2e)
- **Overall Confidence**: 90% through pyramid testing strategy with defense-in-depth

---

## Implementation Roadmap

### Phase 1: MVP (40-50 hours, Kubernaut V1.1)

**Goal**: Error span-based signals (Alternative 1)

**Testing Strategy**: Defense-in-depth pyramid approach per [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)
- **Unit Tests (70%+ BR coverage)**: EXTENSIVE business logic validation with real adapter logic
- **Integration Tests (>50% BR coverage)**: Cross-service flows, CRD creation, Gateway processing pipeline
- **E2E Tests (10-15% BR coverage)**: Complete OTel trace → Kubernetes remediation workflow

**Deliverables**:
1. `OpenTelemetryAdapter` implementation (20h)
   - Parse OTLP/HTTP JSON format
   - Extract error spans (`status.code == ERROR`)
   - Map service → K8s resource via span attributes
   - Generate fingerprint: SHA256(service:operation:error_type)

2. Unit Tests - 70%+ BR Coverage (15h)
   - **BR-GATEWAY-024**: Parse OTLP/HTTP traces with valid error spans
   - **BR-GATEWAY-026**: Parse JSON-encoded OTLP format
   - **BR-GATEWAY-028**: Extract error spans (status.code == ERROR)
   - **BR-GATEWAY-030**: Generate fingerprints from trace attributes
   - **BR-GATEWAY-031**: Map OTel service names to K8s resources
   - **BR-GATEWAY-032**: Extract K8s resource from span attributes (k8s.pod.name, k8s.namespace.name)
   - **BR-GATEWAY-033**: Fallback to K8s API lookup when attributes missing
   - **BR-GATEWAY-040**: Deduplicate OpenTelemetry signals
   - Error scenarios: invalid OTLP, missing attributes, malformed traces
   - Boundary values: min/max span durations, attribute limits
   - Target: 70%+ of BR-GATEWAY-024 to 040 covered with real adapter logic
   - Mock strategy: Mock ONLY external dependencies (K8s API), use REAL adapter parsing logic

3. Integration Tests - >50% BR Coverage (10h)
   - **BR-GATEWAY-024**: End-to-end OTLP/HTTP → Gateway → RemediationRequest CRD
   - **BR-GATEWAY-038**: Verify trace context stored in CRD providerData
   - **BR-GATEWAY-040**: Verify deduplication via Redis fingerprints
   - **Multi-signal integration**: OTel + Prometheus signals in same Gateway instance
   - **Resource mapping validation**: K8s API lookup fallback behavior
   - Target: >50% of BR-GATEWAY-024 to 040 validated in realistic Gateway processing pipeline
   - Testing: Real Gateway server, real Redis, fake K8s client or Kind cluster

4. E2E Tests - 10-15% BR Coverage (5h)
   - **BR-GATEWAY-024 + downstream**: Complete OTel trace → RemediationRequest → AIAnalysis → WorkflowExecution
   - **Critical journey**: High-latency payment service trace → scale deployment remediation
   - Target: 10-15% of BR-GATEWAY-024 to 040 validated in full Kubernaut workflow
   - Testing: Real OTel Collector, real Gateway, real downstream services, Kind/OCP cluster

5. Documentation (5h)
   - Update CRD_SCHEMAS.md with OpenTelemetry providerData schema
   - Update Gateway IMPLEMENTATION_PLAN with test results
   - Create OTel Collector integration guide

**Exit Criteria**:
- **Unit**: 25+ tests passing (70%+ BR coverage: 12+ of 17 BRs)
- **Integration**: 10+ tests passing (>50% BR coverage: 9+ of 17 BRs)
- **E2E**: 2-3 tests passing (10-15% BR coverage: 2-3 of 17 BRs)
- **Defense-in-depth**: Key BRs tested at multiple levels (e.g., BR-GATEWAY-024 in unit, integration, AND e2e)
- **Confidence**: 85-90% (unit), 80-85% (integration), 90-95% (e2e)
- Manual validation: OTel Collector → Gateway → Kubernetes remediation workflow

---

### Phase 2: Production-Ready (40 hours, Kubernaut V1.1)

**Goal**: Add latency-based signals (Alternative 2)

**Testing Strategy**: Expand defense-in-depth coverage for latency-based signals

**Deliverables**:
1. Latency Signal Detection (15h)
   - Add configurable latency threshold
   - Extract high-latency spans
   - Severity mapping (latency → priority)

2. OTLP/gRPC Support (10h)
   - Parse protobuf-encoded OTLP
   - Test gRPC endpoint

3. Advanced Testing - Defense-in-Depth Expansion (10h)
   - **Unit Tests (70%+ BR coverage)**:
     - BR-GATEWAY-029: Extract high-latency spans (duration > threshold)
     - BR-GATEWAY-027: Parse protobuf-encoded OTLP
     - BR-GATEWAY-034: Configure error span criteria
     - BR-GATEWAY-035: Configure latency thresholds
     - BR-GATEWAY-036: Span sampling/filtering logic
   - **Integration Tests (>50% BR coverage)**:
     - BR-GATEWAY-025: OTLP/gRPC end-to-end flow
     - BR-GATEWAY-036: High-volume signal filtering (1000 traces/sec)
     - Multi-criteria signals: Error + latency detection in same trace
   - **E2E Tests (10-15% BR coverage)**:
     - Complete latency-based remediation journey
   - **Performance tests**: 1000 traces/sec sustained load
   - **Signal volume tests**: Deduplication under high load
   - **Error injection tests**: OTLP/gRPC failure scenarios

4. Production Hardening (5h)
   - Rate limiting per signal type
   - Circuit breaker for K8s API lookups
   - Metrics and observability (Prometheus)

**Exit Criteria**:
- **Unit**: 35+ tests passing (70%+ of 17 BRs = 12+ BRs)
- **Integration**: 20+ tests passing (>50% of 17 BRs = 9+ BRs)
- **E2E**: 5+ tests passing (10-15% of 17 BRs = 2-3 BRs)
- **Defense-in-depth validation**: BR-GATEWAY-029 tested at unit (algorithm), integration (pipeline), and e2e (full workflow) levels
- **Performance**: 1000 traces/sec sustained, <50ms p95 latency
- **Confidence**: 85-90% overall system confidence through defense-in-depth pyramid

---

### Phase 3: Advanced Features (30+ hours, Kubernaut V1.2+)

**Optional enhancements** (deferred):

1. **Rate-Based Aggregation** (Alternative 3)
   - Aggregate error spans into single signal
   - Redis-based error rate tracking
   - Configurable thresholds

2. **Custom Signal Rules**
   - User-defined span attribute filters
   - Example: `http.status_code >= 500 AND service.name == "payment-api"`

3. **OTel Metrics Support**
   - Complement traces with metrics
   - Example: `http.server.request.duration > 5s`

---

## Appendix: Example Scenarios

### Scenario 1: Payment Service Error Span

**Input** (OTLP/HTTP JSON):
```json
{
  "resourceSpans": [{
    "resource": {
      "attributes": [
        {"key": "service.name", "value": {"stringValue": "payment-api"}},
        {"key": "k8s.namespace.name", "value": {"stringValue": "production"}},
        {"key": "k8s.pod.name", "value": {"stringValue": "payment-api-xyz-789"}},
        {"key": "k8s.deployment.name", "value": {"stringValue": "payment-api"}}
      ]
    },
    "scopeSpans": [{
      "spans": [{
        "traceId": "abc123def456",
        "spanId": "span789",
        "name": "POST /api/v1/payments",
        "startTimeUnixNano": "1696435200000000000",
        "endTimeUnixNano": "1696435212500000000",
        "status": {
          "code": "STATUS_CODE_ERROR",
          "message": "deadline exceeded"
        },
        "attributes": [
          {"key": "http.method", "value": {"stringValue": "POST"}},
          {"key": "http.route", "value": {"stringValue": "/api/v1/payments"}},
          {"key": "error.type", "value": {"stringValue": "DeadlineExceeded"}}
        ]
      }]
    }]
  }]
}
```

**Output** (NormalizedSignal):
```go
&types.NormalizedSignal{
    Fingerprint:  "sha256(payment-api:POST /api/v1/payments:DeadlineExceeded)",
    AlertName:    "TraceError_payment-api",
    Severity:     "critical",
    Namespace:    "production",
    Resource: types.ResourceIdentifier{
        Kind:      "Deployment",
        Name:      "payment-api",
        Namespace: "production",
    },
    SourceType:   "opentelemetry-trace",
    Source:       "opentelemetry-adapter",
    FiringTime:   time.Unix(0, 1696435200000000000),
    ReceivedTime: time.Now(),
    RawPayload:   []byte(/* OTLP JSON */),
}
```

**RemediationRequest CRD**:
```yaml
apiVersion: remediation.kubernaut.io/v1
kind: RemediationRequest
metadata:
  name: rr-sha256abc...
  namespace: production
spec:
  signalType: "opentelemetry-trace"
  targetType: "kubernetes"
  priority: "P0"
  environment: "production"
  severity: "critical"
  providerData: |
    {
      "traceId": "abc123def456",
      "spanId": "span789",
      "serviceName": "payment-api",
      "operation": "POST /api/v1/payments",
      "duration": "12.5s",
      "errorType": "DeadlineExceeded",
      "errorMessage": "deadline exceeded",
      "spanAttributes": {
        "http.method": "POST",
        "http.route": "/api/v1/payments"
      }
    }
```

---

**Document Status**: ✅ Complete - Feasibility Study
**Confidence**: 78% (High - Architecture validated, risks identified with mitigations)
**Next Steps**:
1. Await Kubernaut V1.0 completion
2. Begin OpenTelemetry adapter development (Q1 2026)
3. Deploy MVP to staging for validation

