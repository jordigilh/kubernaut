# Gateway Service - Implementation Plan v2.27

✅ **CATEGORIZATION MIGRATION** - Architectural Simplification (DD-CATEGORIZATION-001)

**Service**: Gateway Service (Entry Point for All Signals)
**Phase**: Phase 2, Service #1
**Plan Version**: v2.27 (Categorization Migration to Signal Processing)
**Template Version**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v2.0
**Plan Date**: November 11, 2025
**Current Status**: 📋 V2.27 CATEGORIZATION MIGRATION PLANNED (Not Yet Implemented)
**Business Requirements**: BR-GATEWAY-001 through BR-GATEWAY-115 (~55 BRs, 5 BRs deprecated)
**Scope**: Prometheus AlertManager + Kubernetes Events + HTTP Server + Observability + Network-Level Security + E2E Edge Cases + K8s API Retry Logic + **Categorization Migration**
**Confidence**: 95% ✅ **Architectural Decision Approved - Implementation Deferred to Signal Processing Development**

**Architecture**: Adapter-specific self-registered endpoints (DD-GATEWAY-001)
**Security**: Network Policies + TLS + Rate Limiting + Security Headers + Log Sanitization + Timestamp Validation (DD-GATEWAY-004)
**Optimization**: Lightweight metadata storage (DD-GATEWAY-004 Redis)
**Resilience**: K8s API retry with exponential backoff (DD-GATEWAY-008)
**Simplification**: Categorization moved to Signal Processing (DD-CATEGORIZATION-001) **NEW**

---

## 📋 Version History

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v2.25** | Nov 7, 2025 | **K8s API Retry Logic**: Added comprehensive retry logic for transient K8s API errors (429 rate limiting, 503 service unavailable, timeouts). **Phased Implementation**: Phase 1 (Synchronous Retry with Exponential Backoff, 10h) + Phase 2 (Async Retry Queue, 12h incremental). **New BRs**: BR-GATEWAY-111 (Retry Configuration), BR-GATEWAY-112 (Error Classification), BR-GATEWAY-113 (Exponential Backoff), BR-GATEWAY-114 (Retry Metrics), BR-GATEWAY-115 (Async Retry Queue). | ⚠️ SUPERSEDED |
| **v2.26** | Nov 7, 2025 | K8s API Retry Logic - Gap Resolution from Context API Lessons. Comprehensive analysis with phased approach, performance metrics, rollback plan. Confidence: 87%. | ⚠️ SUPERSEDED |
| **v2.27** | Nov 11, 2025 | **Categorization Migration**: Deprecated 5 BRs (BR-GATEWAY-007, 014, 015, 016, 017) and moved categorization responsibility to Signal Processing Service. **Rationale**: DD-CATEGORIZATION-001 approved - consolidate all categorization (environment classification + priority assignment) into Signal Processing Service for context-driven categorization with full K8s context. **Gateway Changes**: Remove environment classification logic, remove priority assignment logic, set placeholder values (`environment: "pending"`, `priority: "pending"`) in RemediationRequest CRD. **Implementation Status**: NOT YET IMPLEMENTED - deferred until Signal Processing Service development begins. **Migration Target**: Signal Processing Service (BR-SP-051 to BR-SP-053 for environment, BR-SP-070 to BR-SP-072 for priority). **Confidence**: 95% (architectural decision approved, clear migration path). | ✅ **CURRENT** |

---

## 🎯 **v2.27 Feature Overview: Categorization Migration**

### **Architectural Decision**

**Decision**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) - Gateway vs Signal Processing Categorization Split Assessment

**Status**: ✅ **APPROVED** (November 11, 2025)

**Confidence**: 92% (based on industry best practices from Datadog, PagerDuty, Splunk, Google Cloud Operations)

---

### **Business Problem**

**Current State**: Categorization logic is duplicated between Gateway and Signal Processing:
- **Gateway**: Environment classification (BR-GATEWAY-014 to 017) + Priority assignment (BR-GATEWAY-007)
  - Limited context: Only alert payload + namespace labels
  - Fast but simplistic: 2-3ms environment, 5-8ms Rego evaluation
- **Signal Processing**: Environment classification (BR-SP-051 to 053) with business metadata
  - Rich context: Full K8s context (~8KB) after enrichment
  - Sophisticated but slower: 1-2 seconds (acceptable for enrichment phase)

**Problems**:
1. **Duplicate Logic**: Environment classification implemented twice
2. **Limited Context**: Gateway cannot consider deployment criticality, SLA requirements, resource quotas
3. **Maintenance Burden**: Changes must be synchronized across two services
4. **Consistency Risk**: Gateway and Signal Processing might classify differently

**Impact**:
- **Maintenance Overhead**: Duplicate code increases maintenance burden
- **Inconsistency Risk**: Divergent implementations over time
- **Limited Sophistication**: Gateway cannot perform business-aware categorization

---

### **Solution: Consolidate Categorization into Signal Processing**

**Recommended Architecture** (DD-CATEGORIZATION-001):

```
┌─────────────────────────────────────────────────────────────┐
│ Gateway Service (Fast Path: <50ms)                         │
│                                                              │
│ Responsibilities:                                           │
│ 1. Alert ingestion and normalization                       │
│ 2. Fingerprint-based deduplication (Redis)                 │
│ 3. Storm detection (rate + pattern)                        │
│ 4. RemediationRequest CRD creation                         │
│                                                              │
│ ❌ NO environment classification                            │
│ ❌ NO priority assignment                                   │
│ ❌ NO Rego policy evaluation                                │
│                                                              │
│ CRD Fields:                                                 │
│   environment: "pending"  # Placeholder                     │
│   priority: "pending"     # Placeholder                     │
└─────────────────────────────────────────────────────────────┘
                          │
                          v
┌─────────────────────────────────────────────────────────────┐
│ Signal Processing Service (Enrichment Path: ~3s)           │
│                                                              │
│ Responsibilities:                                           │
│ 1. K8s context enrichment (~2s)                            │
│ 2. Environment classification (BR-SP-051 to 053)           │
│ 3. Priority assignment with Rego policies (BR-SP-070+)     │
│ 4. Business categorization (SLA, criticality)              │
│ 5. Recovery context integration (if recovery attempt)      │
│                                                              │
│ ✅ ALL categorization logic centralized                     │
│ ✅ Rich context available for sophisticated decisions      │
│ ✅ Graceful degradation if K8s API unavailable             │
│                                                              │
│ CRD Fields:                                                 │
│   environment: "production"  # Classified                   │
│   priority: "P0"             # Assigned                     │
│   confidence: 0.95           # Classification confidence    │
│   businessPriority: "P0"     # Business-aware priority      │
│   slaRequirement: "5m"       # SLA metadata                 │
└─────────────────────────────────────────────────────────────┘
```

---

### **Deprecated Business Requirements**

| BR ID | Description | Status | Migration Target |
|-------|-------------|--------|------------------|
| **BR-GATEWAY-007** | Signal Priority Classification | ⚠️ **DEPRECATED** | Signal Processing (BR-SP-070 to BR-SP-072) |
| **BR-GATEWAY-014** | Signal Enrichment (Environment Classification) | ⚠️ **DEPRECATED** | Signal Processing (BR-SP-051 to BR-SP-053) |
| **BR-GATEWAY-015** | Environment Classification - Explicit Labels | ⚠️ **DEPRECATED** | Signal Processing (BR-SP-051) |
| **BR-GATEWAY-016** | Environment Classification - Namespace Pattern | ⚠️ **DEPRECATED** | Signal Processing (BR-SP-052) |
| **BR-GATEWAY-017** | Environment Classification - Fallback | ⚠️ **DEPRECATED** | Signal Processing (BR-SP-053) |

**Deprecation Details**:
- **Implementation**: `pkg/gateway/processing/environment_classification.go` (to be removed)
- **Tests**: Unit and integration tests (to be migrated to Signal Processing)
- **Rego Policies**: Priority assignment Rego policies (to be migrated to Signal Processing)
- **Decision Reference**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)

---

### **Implementation Status**

**Current Status**: 📋 **NOT YET IMPLEMENTED**

**Rationale**: Implementation deferred until Signal Processing Service development begins. Gateway will continue to perform categorization until Signal Processing Service is ready to take over.

**Implementation Timeline** (from DD-CATEGORIZATION-001):

#### **Phase 1: Signal Processing Enhancement** (Week 1)
- Add Rego policy engine to Signal Processing Service
- Migrate Gateway's Rego policies to Signal Processing
- Enhance policies with K8s context inputs (replicas, quotas, node health)
- Add business context inputs (SLA, criticality, historical failures)
- **NEW BRs**: BR-SP-070 (Rego priority), BR-SP-071 (Fallback matrix), BR-SP-072 (Hot-reload)

#### **Phase 2: Gateway Simplification** (Week 2)
- Remove environment classification from Gateway Service
- Remove priority assignment from Gateway Service
- Remove Rego policy engine from Gateway Service
- Set placeholder values in RemediationRequest CRD: `environment: "pending"`, `priority: "pending"`
- **DEPRECATE BRs**: BR-GATEWAY-007, BR-GATEWAY-014, BR-GATEWAY-015, BR-GATEWAY-016, BR-GATEWAY-017

#### **Phase 3: Signal Processing Ownership** (Week 3)
- Signal Processing reads `environment: "pending"` and `priority: "pending"`
- Signal Processing performs classification after K8s enrichment
- Signal Processing updates RemediationRequest CRD with classified values
- Signal Processing emits metrics for classification confidence

---

### **Benefits of Migration**

1. **Single Responsibility** (92% Confidence)
   - Gateway: Fast ingestion and triage
   - Signal Processing: Enrichment and categorization
   - Industry alignment: Datadog, PagerDuty, Splunk

2. **Context-Driven Categorization** (95% Confidence)
   - Signal Processing has full K8s context (~8KB) after enrichment
   - Can consider deployment criticality, SLA requirements, resource quotas
   - Industry alignment: Google Cloud Operations, AWS CloudWatch

3. **Eliminate Duplicate Logic** (94% Confidence)
   - Single source of truth for categorization
   - Reduced maintenance burden
   - Consistency guaranteed

4. **Fail-Safe Architecture** (90% Confidence)
   - Gateway remains fail-fast (reject invalid alerts)
   - Signal Processing has graceful degradation
   - Industry alignment: Prometheus Alertmanager, Grafana OnCall

5. **Performance Improvement** (95% Confidence)
   - Gateway saves 10ms per alert (no categorization)
   - Response time improves from 50ms → 40ms
   - Signal Processing 1-2s acceptable for enrichment phase

---

### **Risk Assessment**

| Risk | Severity | Mitigation | Confidence |
|------|----------|------------|------------|
| Gateway Performance Impact | LOW (5%) | Removing categorization saves 10ms per alert | 95% |
| Downstream Dependency | MEDIUM (15%) | Audit all services reading RemediationRequest CRD | 85% |
| Signal Processing Failure | MEDIUM (20%) | Graceful degradation with fallback logic | 80% |
| Migration Complexity | LOW (10%) | Phased rollout with feature flags | 90% |

---

### **Files Affected**

#### **To Be Removed (Phase 2)**:
- `pkg/gateway/processing/environment_classification.go`
- `pkg/gateway/processing/priority_classification.go`
- `test/unit/gateway/processing/environment_classification_test.go`
- `test/integration/gateway/environment_classification_test.go`
- `test/unit/gateway/priority_classification_test.go`
- `test/integration/gateway/priority_classification_test.go`

#### **To Be Modified (Phase 2)**:
- `pkg/gateway/processing/crd_creator.go` - Set placeholder values
- `config/gateway-config.yaml` - Remove categorization configuration

#### **To Be Migrated to Signal Processing (Phase 1)**:
- Rego policy files for priority assignment
- Environment classification logic
- Unit and integration tests

---

### **Configuration Changes**

#### **Current Configuration** (to be removed in Phase 2):
```yaml
processing:
  environment_classification:
    enabled: true
    namespace_label: "environment"
    configmap_name: "kubernaut-environment-overrides"
  priority_assignment:
    enabled: true
    rego_policy_configmap: "kubernaut-priority-policies"
    fallback_matrix:
      critical:
        production: "P0"
        staging: "P1"
      warning:
        production: "P1"
        staging: "P2"
```

#### **New Configuration** (Phase 2):
```yaml
processing:
  # Categorization removed - handled by Signal Processing Service
  # Gateway sets placeholder values: environment="pending", priority="pending"
```

---

### **Testing Strategy**

#### **Phase 1: Signal Processing Enhancement**
- Unit tests for Rego policies with K8s context
- Integration tests for environment classification with full context
- Unit tests for priority assignment with business context

#### **Phase 2: Gateway Simplification**
- Unit tests for placeholder value setting
- Integration tests for RemediationRequest CRD creation with placeholders
- E2E tests for end-to-end flow (Gateway → Signal Processing)

#### **Phase 3: Signal Processing Ownership**
- Integration tests for Signal Processing reading placeholders
- Integration tests for Signal Processing updating CRD with classified values
- E2E tests for complete categorization flow

---

### **Rollback Plan**

If Signal Processing categorization fails:
1. **Immediate**: Signal Processing falls back to Gateway's simple logic
2. **Short-term**: Re-enable Gateway categorization via feature flag
3. **Long-term**: Fix Signal Processing categorization issues

---

## 📊 **Implementation Confidence**

**Overall Confidence**: 95%

**Breakdown**:
- **Architectural Decision**: 92% (DD-CATEGORIZATION-001 approved)
- **Migration Strategy**: 90% (phased rollout with feature flags)
- **Risk Mitigation**: 85% (clear rollback plan)
- **Industry Alignment**: 95% (proven patterns from Datadog, PagerDuty, Splunk)

---

## 🔗 **Related Documentation**

- **[DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)** - Complete analysis and decision rationale
- **[Gateway BRs](../BUSINESS_REQUIREMENTS.md)** - Updated with deprecated BRs
- **[Signal Processing BRs](../../crd-controllers/01-signalprocessing/overview.md)** - Migration target BRs

---

## 📝 **Next Steps**

1. ✅ **Approved**: DD-CATEGORIZATION-001 approved (November 11, 2025)
2. ✅ **Updated**: Gateway BRs updated with deprecation notices
3. 📋 **Pending**: Signal Processing Service development
4. 📋 **Pending**: Phase 1 implementation (Signal Processing enhancement)
5. 📋 **Pending**: Phase 2 implementation (Gateway simplification)
6. 📋 **Pending**: Phase 3 implementation (Signal Processing ownership)

---

**Status**: 📋 **CATEGORIZATION MIGRATION PLANNED - NOT YET IMPLEMENTED**
**Implementation**: Deferred until Signal Processing Service development begins
**Confidence**: 95% (architectural decision approved, clear migration path)
**Author**: AI Assistant (Cursor)
**Date**: November 11, 2025
**Approved By**: User
**Approval Date**: November 11, 2025

