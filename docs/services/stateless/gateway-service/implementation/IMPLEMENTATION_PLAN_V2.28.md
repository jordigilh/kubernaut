# Gateway Service - Implementation Plan v2.28

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../../../../../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

✅ **CATEGORIZATION MIGRATION COMPLETE** - Classification Removed from Gateway (DD-CATEGORIZATION-001)

**Service**: Gateway Service (Entry Point for All Signals)
**Phase**: Phase 2, Service #1
**Plan Version**: v2.28 (Categorization Migration Complete)
**Template Version**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v2.0
**Plan Date**: December 6, 2025
**Current Status**: ✅ **V2.28 CATEGORIZATION MIGRATION COMPLETE** (2025-12-06)
**Business Requirements**: BR-GATEWAY-001 through BR-GATEWAY-115 (~50 BRs active, 5 BRs REMOVED)
**Scope**: Prometheus AlertManager + Kubernetes Events + HTTP Server + Observability + Network-Level Security + E2E Edge Cases + K8s API Retry Logic + **Categorization Removed**
**Confidence**: 95% ✅ **Implementation Complete - Classification Owned by Signal Processing**

**Architecture**: Adapter-specific self-registered endpoints (DD-GATEWAY-001)
**Security**: Network Policies + TLS + Rate Limiting + Security Headers + Log Sanitization + Timestamp Validation (DD-GATEWAY-004)
**Optimization**: Lightweight metadata storage (DD-GATEWAY-004 Redis)
**Resilience**: K8s API retry with exponential backoff (DD-GATEWAY-008)
**Simplification**: Categorization **REMOVED** from Gateway - now owned by Signal Processing (DD-CATEGORIZATION-001) ✅ **COMPLETE**

---

## 📋 Version History

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v2.25** | Nov 7, 2025 | **K8s API Retry Logic**: Added comprehensive retry logic for transient K8s API errors (429 rate limiting, 503 service unavailable, timeouts). **Phased Implementation**: Phase 1 (Synchronous Retry with Exponential Backoff, 10h) + Phase 2 (Async Retry Queue, 12h incremental). **New BRs**: BR-GATEWAY-111 (Retry Configuration), BR-GATEWAY-112 (Error Classification), BR-GATEWAY-113 (Exponential Backoff), BR-GATEWAY-114 (Retry Metrics), BR-GATEWAY-115 (Async Retry Queue). | ⚠️ SUPERSEDED |
| **v2.26** | Nov 7, 2025 | K8s API Retry Logic - Gap Resolution from Context API Lessons. Comprehensive analysis with phased approach, performance metrics, rollback plan. Confidence: 87%. | ⚠️ SUPERSEDED |
| **v2.27** | Nov 11, 2025 | **Categorization Migration Planned**: Deprecated 5 BRs (BR-GATEWAY-007, 014, 015, 016, 017) and planned categorization responsibility move to Signal Processing Service. **DD-CATEGORIZATION-001 approved**. | ⚠️ SUPERSEDED |
| **v2.28** | Dec 6, 2025 | **Categorization Migration COMPLETE**: Classification code **completely removed** from Gateway (not placeholder values). **Files Deleted**: `classification.go`, `priority.go`, `environment_classification_test.go`, `priority_classification_test.go`, Rego policy files. **CRD Changes**: `environment` and `priority` labels/fields no longer set by Gateway. **HTTP Response Changes**: `environment` and `priority` removed from response. **Config Changes**: `EnvironmentSettings` and `PrioritySettings` removed. **5 BRs REMOVED**: BR-GATEWAY-007, 014, 015, 016, 017. **Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md). **Confidence**: 95% (implementation verified). | ✅ **CURRENT** |

---

## 🎯 **v2.28 Feature Overview: Categorization Migration COMPLETE**

### **Architectural Decision**

**Decision**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) - Gateway vs Signal Processing Categorization Split Assessment

**Status**: ✅ **IMPLEMENTED** (December 6, 2025)

**Confidence**: 95% (implementation verified, code removed)

**Implementation Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

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

**Implemented Architecture** (DD-CATEGORIZATION-001) - **COMPLETE (2025-12-06)**:

```
┌─────────────────────────────────────────────────────────────┐
│ Gateway Service (Fast Path: <50ms) ✅ IMPLEMENTED          │
│                                                              │
│ Responsibilities:                                           │
│ 1. Alert ingestion and normalization                       │
│ 2. Fingerprint-based deduplication (Redis)                 │
│ 3. Storm detection (rate + pattern)                        │
│ 4. RemediationRequest CRD creation                         │
│                                                              │
│ ❌ NO environment classification (CODE DELETED)            │
│ ❌ NO priority assignment (CODE DELETED)                   │
│ ❌ NO Rego policy evaluation (CODE DELETED)                │
│                                                              │
│ CRD Labels Set:                                             │
│   kubernaut.ai/alert-name: <alertname>                      │
│   kubernaut.ai/severity: <severity>                         │
│   (environment/priority labels NOT set by Gateway)          │
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
│ CRD Labels/Fields Added:                                    │
│   kubernaut.ai/environment: "production"  # Classified      │
│   kubernaut.ai/priority: "P0"             # Assigned        │
│   confidence: 0.95           # Classification confidence    │
│   businessPriority: "P0"     # Business-aware priority      │
│   slaRequirement: "5m"       # SLA metadata                 │
└─────────────────────────────────────────────────────────────┘
```

---

### **Removed Business Requirements** ✅ **COMPLETE (2025-12-06)**

| BR ID | Description | Status | Migration Target |
|-------|-------------|--------|------------------|
| **BR-GATEWAY-007** | Signal Priority Classification | ❌ **REMOVED** (2025-12-06) | Signal Processing (BR-SP-070 to BR-SP-072) |
| **BR-GATEWAY-014** | Signal Enrichment (Environment Classification) | ❌ **REMOVED** (2025-12-06) | Signal Processing (BR-SP-051 to BR-SP-053) |
| **BR-GATEWAY-015** | Environment Classification - Explicit Labels | ❌ **REMOVED** (2025-12-06) | Signal Processing (BR-SP-051) |
| **BR-GATEWAY-016** | Environment Classification - Namespace Pattern | ❌ **REMOVED** (2025-12-06) | Signal Processing (BR-SP-052) |
| **BR-GATEWAY-017** | Environment Classification - Fallback | ❌ **REMOVED** (2025-12-06) | Signal Processing (BR-SP-053) |

**Removal Details** (2025-12-06):
- **Implementation Files DELETED**:
  - ~~`pkg/gateway/processing/classification.go`~~ **DELETED**
  - ~~`pkg/gateway/processing/priority.go`~~ **DELETED**
  - ~~`config.app/gateway/policies/priority.rego`~~ **DELETED**
- **Test Files DELETED**:
  - ~~`test/unit/gateway/processing/environment_classification_test.go`~~ **DELETED**
  - ~~`test/unit/gateway/priority_classification_test.go`~~ **DELETED**
- **Config Structs DELETED**:
  - ~~`EnvironmentSettings`~~ **DELETED** from `config.go`
  - ~~`PrioritySettings`~~ **DELETED** from `config.go`
- **Decision Reference**: [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
- **Removal Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

---

### **Implementation Status**

**Current Status**: ✅ **IMPLEMENTED** (December 6, 2025)

**Implementation Summary**:
- Gateway classification code **completely removed** (not placeholder values)
- Signal Processing service now owns all environment/priority classification
- Coordination completed via [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

**Completion Timeline**:

#### **Phase 1: Signal Processing Enhancement** ✅ **COMPLETE**
- SP team completed Day 5 Priority Engine ahead of schedule (2025-12-06)
- Rego policy engine operational in Signal Processing Service
- Environment classification (BR-SP-051 to BR-SP-053) implemented
- Priority assignment (BR-SP-070 to BR-SP-072) implemented

#### **Phase 2: Gateway Simplification** ✅ **COMPLETE** (2025-12-06)
- ✅ Removed environment classification from Gateway Service
- ✅ Removed priority assignment from Gateway Service
- ✅ Removed Rego policy engine from Gateway Service
- ✅ Removed `environment` and `priority` from CRD labels (not placeholder - completely removed)
- ✅ Removed `environment` and `priority` from HTTP response
- ✅ **REMOVED BRs**: BR-GATEWAY-007, BR-GATEWAY-014, BR-GATEWAY-015, BR-GATEWAY-016, BR-GATEWAY-017

#### **Phase 3: Signal Processing Ownership** ✅ **COMPLETE** (SP Day 5)
- ✅ Signal Processing adds `kubernaut.ai/environment` and `kubernaut.ai/priority` labels
- ✅ Signal Processing updates CRD with classified values after K8s enrichment
- ✅ Signal Processing emits metrics for classification confidence
- **Reference**: SP Day 5 Priority Engine completed ahead of schedule (2025-12-06)

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

### **Files Affected** ✅ **COMPLETE (2025-12-06)**

#### **Removed (Phase 2)** ✅:
- ~~`pkg/gateway/processing/classification.go`~~ **DELETED**
- ~~`pkg/gateway/processing/priority.go`~~ **DELETED**
- ~~`pkg/gateway/processing/remediation_path.go`~~ **DELETED**
- ~~`test/unit/gateway/processing/environment_classification_test.go`~~ **DELETED**
- ~~`test/unit/gateway/priority_classification_test.go`~~ **DELETED**
- ~~`config.app/gateway/policies/priority.rego`~~ **DELETED**

#### **Modified (Phase 2)** ✅:
- `pkg/gateway/processing/crd_creator.go` - Environment/priority fields **completely removed** (not placeholders)
- `pkg/gateway/server.go` - Classification and path decision logic removed
- `pkg/gateway/config/config.go` - `EnvironmentSettings` and `PrioritySettings` structs removed

#### **Migrated to Signal Processing (Phase 1)** ✅:
- ✅ Rego policy engine for priority assignment (BR-SP-070 to BR-SP-072)
- ✅ Environment classification logic (BR-SP-051 to BR-SP-053)
- ✅ Unit and integration tests

---

### **Configuration Changes** ✅ **COMPLETE (2025-12-06)**

#### **Removed Configuration** (from `pkg/gateway/config/config.go`):
```go
// DELETED: EnvironmentSettings struct
// DELETED: PrioritySettings struct
// DELETED: Environment and Priority fields from ProcessingSettings
```

#### **Current Configuration** (Phase 2 Complete):
```yaml
processing:
  deduplication:
    ttl: 300s
    redis_key_prefix: "alert:fingerprint:"
  storm:
    rate_threshold: 10
    pattern_threshold: 5
    window: 60s
  crd:
    namespace: "kubernaut-system"
  retry:
    max_attempts: 3
    initial_delay: 100ms
  # NOTE: environment_classification and priority_assignment REMOVED
  # Classification now owned by Signal Processing Service (DD-CATEGORIZATION-001)
```

---

### **Testing Strategy** ✅ **COMPLETE (2025-12-06)**

#### **Phase 1: Signal Processing Enhancement** ✅
- ✅ Unit tests for Rego policies with K8s context (BR-SP-070 to BR-SP-072)
- ✅ Integration tests for environment classification with full context (BR-SP-051 to BR-SP-053)
- ✅ Unit tests for priority assignment with business context

#### **Phase 2: Gateway Simplification** ✅
- ✅ Unit tests verify environment/priority fields **not set** (removed, not placeholders)
- ✅ Integration tests for RemediationRequest CRD creation without classification fields
- ✅ E2E tests for end-to-end flow (Gateway → Signal Processing)

#### **Phase 3: Signal Processing Ownership** ✅
- ✅ Integration tests for Signal Processing adding classification labels to CRD
- ✅ Integration tests for Signal Processing updating CRD with classified values
- ✅ E2E tests for complete categorization flow

---

### **Rollback Plan** ⚠️ **NOT APPLICABLE**

Classification code has been **completely removed** from Gateway (not feature-flagged):
- Gateway cannot fall back to classification logic (code deleted)
- Signal Processing is the **single source of truth** for classification
- If SP fails, alerts are still processed but without classification until SP recovers

**Recovery Strategy** (if SP categorization fails):
1. **Immediate**: Alerts processed without classification (RemediationRequest CRD created)
2. **Short-term**: SP graceful degradation assigns default values per BR-SP-053
3. **Long-term**: Fix Signal Processing categorization issues

---

## 📊 **Implementation Confidence**

**Overall Confidence**: 95%

**Breakdown**:
- **Architectural Decision**: 95% (DD-CATEGORIZATION-001 implemented)
- **Migration Execution**: 95% (all phases complete)
- **Code Removal**: 95% (classification code deleted, not placeholders)
- **Industry Alignment**: 95% (proven patterns from Datadog, PagerDuty, Splunk)

---

## 🔗 **Related Documentation**

- **[DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)** - Complete analysis and decision rationale
- **[Gateway BRs](../BUSINESS_REQUIREMENTS.md)** - Updated with deprecated BRs
- **[Signal Processing BRs](../../crd-controllers/01-signalprocessing/overview.md)** - Migration target BRs

---

## 📝 **Completion Summary**

1. ✅ **Approved**: DD-CATEGORIZATION-001 approved (November 11, 2025)
2. ✅ **Updated**: Gateway BRs updated with deprecation notices (November 11, 2025)
3. ✅ **Complete**: Signal Processing Service Day 5 Priority Engine (December 6, 2025)
4. ✅ **Complete**: Phase 1 - Signal Processing enhancement (BR-SP-051 to BR-SP-072)
5. ✅ **Complete**: Phase 2 - Gateway simplification (classification code removed)
6. ✅ **Complete**: Phase 3 - Signal Processing ownership (SP adds labels to CRD)

---

**Status**: ✅ **CATEGORIZATION MIGRATION COMPLETE**
**Implementation**: Classification code removed from Gateway (December 6, 2025)
**Confidence**: 95% (implementation verified, all phases complete)
**Author**: AI Assistant (Cursor)
**Date**: December 6, 2025
**Approved By**: User
**Approval Date**: December 6, 2025
**Implementation Reference**: [NOTICE_GATEWAY_CLASSIFICATION_REMOVAL](../../../../handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md)

