# DD-SIGNAL-PROCESSING-001: Service Rename from RemediationProcessing to SignalProcessing

**Date**: 2025-11-03
**Status**: ✅ Approved
**Confidence**: 95%
**Decision Type**: Terminology & Architecture

---

## Context

The service currently named "RemediationProcessing" (formerly "AlertProcessor") processes signals from the Gateway service. The Gateway service established "signal" as the standard terminology for remediation triggers (alerts, events, future sources).

### Current State

- **Service Name**: RemediationProcessing
- **CRD API Group**: `remediationprocessing.kubernaut.io/v1alpha1`
- **Business Requirements**: BR-AP-* (Alert Processing)
- **Directory**: `docs/services/crd-controllers/01-remediationprocessor/`
- **Status**: Not yet implemented (documentation phase only)

---

## Problem

### Terminology Inconsistency

1. **Gateway Service**: Uses "signal" terminology
   - `NormalizedSignal` struct
   - `/api/v1/signals/prometheus` endpoint
   - `/api/v1/signals/kubernetes-event` endpoint
   - BR-GATEWAY-SIGNAL-TERMINOLOGY.md recommends "signal" over "alert"

2. **RemediationProcessing**: Uses "alert" and "remediation" terminology
   - Inconsistent with Gateway's established patterns
   - "RemediationProcessing" is too generic
   - Doesn't emphasize position in data flow

3. **Architecture Diagram**: Already shows "🔍 SIGNAL PROCESSING"
   - Diagram uses correct terminology
   - Documentation lags behind

### Confusion Points

- Current name doesn't clearly indicate it's the next stage after Gateway's signal ingestion
- "RemediationProcessing" is generic and doesn't emphasize the data flow: Signal Ingestion → Signal Processing → AI Analysis
- Inconsistent with established Gateway terminology creates cognitive overhead

---

## Decision

**RENAME** "RemediationProcessing" to "SignalProcessing"

### Scope of Changes

#### 1. Service Identity
- **Service Name**: RemediationProcessing → **SignalProcessing**
- **CRD Name**: `RemediationProcessing` → **`SignalProcessing`**
- **CRD API Group**: `remediationprocessing.kubernaut.io/v1alpha1` → **`signalprocessing.kubernaut.io/v1alpha1`**
- **Controller**: `RemediationProcessingReconciler` → **`SignalProcessingReconciler`**

#### 2. Business Requirements
- **Prefix**: BR-AP-* (Alert Processing) → **BR-SP-*** (Signal Processing)
- **Range**: BR-SP-001 to BR-SP-180 (reserved for Signal Processing)
- **Mapping**: Maintain 1:1 mapping with old BR-AP-* numbers for traceability

#### 3. Documentation
- **Directory**: `docs/services/crd-controllers/01-remediationprocessor/` → **`docs/services/crd-controllers/01-signalprocessing/`**
- **All Files**: Update references to SignalProcessing
- **Architecture Docs**: Update KUBERNAUT_CRD_ARCHITECTURE.md, APPROVED_MICROSERVICES_ARCHITECTURE.md

#### 4. What Stays Unchanged

✅ **Core Responsibilities**: All functionality remains identical
- K8s context enrichment
- Recovery context integration
- Business-aware environment classification
- Alert validation
- Status updates

✅ **Integration Points**: No changes to service interactions
✅ **BR Coverage**: Same business requirements, just renumbered
✅ **Architecture**: Same position in service flow

---

## Rationale

### Benefits

1. **Terminology Consistency** (Weight: 40%)
   - Aligns with Gateway's "signal" terminology
   - Single vocabulary across the platform
   - Reduces cognitive overhead for developers

2. **Architectural Clarity** (Weight: 30%)
   - Clear data flow: **Signal Ingestion (Gateway)** → **Signal Processing** → **AI Analysis**
   - Service name immediately communicates its role
   - Position in workflow is self-documenting

3. **Generic Applicability** (Weight: 20%)
   - "Signal" is more generic than "alert" (supports alerts, events, future sources)
   - Future-proof for additional signal types
   - Aligns with industry best practices (observability signals)

4. **Zero Risk** (Weight: 10%)
   - No implementation yet → No code refactoring
   - No backwards compatibility concerns
   - Pure documentation change
   - Perfect timing (pre-v1.0.0)

### Comparison: Service Responsibilities

| Service | Name | Primary Function | Environment Classification |
|---------|------|------------------|----------------------------|
| **Gateway** | Gateway | Signal Ingestion | **Quick Lookup** (2-3ms): Simple string for routing |
| **This Service** | **SignalProcessing** | **Signal Enrichment** | **Business Classification**: Rich metadata (confidence, priority, SLA) |
| **Next Service** | AIAnalysis | AI Investigation | Uses enriched business classification for risk-aware decisions |

---

## Implementation Plan

### Phase 1: Design Decision (✅ Complete)
- [x] Create DD-SIGNAL-PROCESSING-001 document
- [x] Gain stakeholder approval

### Phase 2: Documentation Updates (In Progress)

#### High Priority (Core Architecture)
- [ ] Update `KUBERNAUT_CRD_ARCHITECTURE.md`
  - Section: "2. RemediationProcessing" → "2. SignalProcessing - Signal Processing & Enrichment"
  - All references to RemediationProcessing → SignalProcessing
  - Update BR references: BR-AP-* → BR-SP-*

- [ ] Update `APPROVED_MICROSERVICES_ARCHITECTURE.md`
  - Service catalog entry
  - CRD specifications

- [ ] Rename directory: `01-remediationprocessor/` → `01-signalprocessing/`

- [ ] Update Excalidraw diagram: `kubernaut-layered-architecture.excalidraw`
  - Change "Environment Classification" → "Business Classification"
  - Confirm "SIGNAL PROCESSING" name is correct (already done ✅)

#### Medium Priority (Service Documentation)
- [ ] `docs/services/crd-controllers/01-signalprocessing/overview.md`
- [ ] `docs/services/crd-controllers/01-signalprocessing/controller-implementation.md`
- [ ] `docs/services/crd-controllers/01-signalprocessing/reconciliation-phases.md`
- [ ] `docs/services/crd-controllers/01-signalprocessing/crd-schema.md`
- [ ] `docs/services/crd-controllers/01-signalprocessing/testing-strategy.md`
- [ ] `docs/services/crd-controllers/01-signalprocessing/integration-points.md`

#### Low Priority (Supporting Documentation)
- [ ] Update `README.md` service catalog
- [ ] Update BR coverage matrices
- [ ] Update test documentation references

### Phase 3: CRD Specification (Before Implementation)
- [ ] Define `SignalProcessing` CRD schema
- [ ] API group: `signalprocessing.kubernaut.io/v1alpha1`
- [ ] Update controller scaffolding templates
- [ ] Update RBAC definitions

### Phase 4: Test Specifications (Before Implementation)
- [ ] Update test strategy documents
- [ ] Update test file naming: `*_remediationprocessing_test.go` → `*_signalprocessing_test.go`
- [ ] Update mock factory names

---

## Alternatives Considered

### Alternative 1: Keep "RemediationProcessing"
**Rejected**: Maintains terminology inconsistency with Gateway
- **Pros**: No documentation changes
- **Cons**: Perpetuates confusion, inconsistent with Gateway, missed opportunity to fix before implementation

### Alternative 2: "AlertProcessing"
**Rejected**: "Alert" is more narrow than "signal"
- **Pros**: Clear purpose
- **Cons**: Doesn't cover events or future signal sources, inconsistent with Gateway

### Alternative 3: "SignalEnrichment"
**Considered**: More specific but "Processing" emphasizes multi-step nature
- **Pros**: Emphasizes primary function
- **Cons**: "Processing" better captures enrichment + classification + validation pipeline

### Alternative 4: Defer to v2.0.0
**Rejected**: Perfect time to fix is NOW (pre-implementation)
- **Pros**: Defer decision
- **Cons**: Creates technical debt, requires migration later, missed opportunity

---

## Business Requirement Mapping

### Old → New Prefix
- BR-AP-001 → BR-SP-001 (K8s Context Enrichment)
- BR-AP-020 → BR-SP-020 (Recovery Context Integration)
- BR-AP-031 → BR-SP-031 (Business-Aware Environment Classification)
- BR-AP-040 → BR-SP-040 (Alert Validation)
- BR-AP-051 → BR-SP-051 (Status Updates)
- ... (maintains 1:1 mapping for traceability)

### Traceability
Maintain cross-reference table in service documentation:
```
| Old BR | New BR | Description |
|--------|--------|-------------|
| BR-AP-001 | BR-SP-001 | Enrich signals with K8s context |
| BR-AP-031 | BR-SP-031 | Business-aware environment classification |
```

---

## Related Decisions

- **BR-GATEWAY-SIGNAL-TERMINOLOGY.md**: Recommends "signal" terminology
- **Gateway Service Design B**: Established "signal" as standard term
- **DD-001**: Recovery Context Enrichment (Alternative 2)
- **ADR-032**: Data Access Layer Isolation

---

## Success Metrics

### Documentation Consistency
- ✅ All documentation uses consistent "signal" terminology
- ✅ No orphaned references to "RemediationProcessing" or "AlertProcessor"
- ✅ Clear data flow: Signal Ingestion → Signal Processing → AI Analysis

### Developer Experience
- ✅ New developers immediately understand service relationships
- ✅ No confusion between Gateway and SignalProcessing responsibilities
- ✅ Service name self-documents its position in the workflow

### Architecture Clarity
- ✅ Diagram terminology matches documentation
- ✅ API group names are consistent with service names
- ✅ BR prefixes clearly identify service ownership

---

## Timeline

- **DD Creation**: 2025-11-03 (✅ Complete)
- **Documentation Updates**: 2025-11-03 (2-4 hours)
- **CRD Specification**: Before implementation start
- **Implementation**: TBD (using SignalProcessing from day 1)

---

## Approval

**Approved by**: Architecture Team
**Date**: 2025-11-03
**Confidence**: 95%

**Risk Assessment**: MINIMAL (documentation only, no code refactoring required)
**Value Assessment**: HIGH (establishes correct terminology from the start, prevents future technical debt)

---

## Implementation Notes

### For Implementers
When beginning implementation:
1. Use `SignalProcessing` as the CRD name
2. Use `signalprocessing.kubernaut.io/v1alpha1` as the API group
3. Use BR-SP-* prefix for business requirements
4. Reference this DD for terminology decisions

### For Reviewers
When reviewing PRs:
1. Verify no references to "RemediationProcessing" or "AlertProcessor"
2. Confirm use of "SignalProcessing" terminology
3. Check BR references use BR-SP-* prefix

---

**Document Status**: ✅ Approved
**Last Updated**: 2025-11-03
**Version**: 1.0.0

