# ADR-049: RemediationRequest CRD Ownership

## Status
**✅ APPROVED** (2025-12-07)
**Last Reviewed**: 2025-12-07
**Confidence**: 95%

---

## Context & Problem

The `RemediationRequest` CRD is the central artifact in the Kubernaut remediation workflow. Currently, there is inconsistency in documentation about who "owns" this CRD:

- `CRD_SCHEMAS.md` states: "Owner: Central Controller Service" and "Created By: Gateway Service"
- Various implementation docs show Gateway defining and creating RR instances
- RO (Remediation Orchestrator) reconciles RR but doesn't own the schema

This split responsibility contradicts Kubernetes controller patterns where the controller that reconciles a CRD should own its schema definition.

### Problem Statement

> Who should own the RemediationRequest CRD schema definition?

### Key Requirements

1. **Clean Architecture**: Clear ownership boundaries between services
2. **K8s Pattern Alignment**: Follow established controller patterns
3. **Dependency Direction**: Avoid circular or inverted dependencies
4. **Schema Evolution**: Owner should be able to evolve schema based on domain needs
5. **Operational Clarity**: Clear responsibility for CRD lifecycle

---

## Decision

**Remediation Orchestrator (RO) owns the RemediationRequest CRD schema definition.**

### Rationale

| Factor | Analysis |
|--------|----------|
| **Domain Ownership** | RR represents the remediation lifecycle, which is RO's domain |
| **Controller Pattern** | K8s pattern: controller that reconciles a CRD should define its schema |
| **Schema Evolution** | RO knows what fields it needs for orchestration |
| **Dependency Direction** | Gateway → RO types is clean; reverse would be problematic |
| **Single Responsibility** | RO orchestrates remediation; RR is the orchestration artifact |

### What This Means

| Aspect | Before | After |
|--------|--------|-------|
| **Schema Definition** | Ambiguous (Gateway docs) | RO owns `api/remediation/v1alpha1/` |
| **Schema Authority** | `CRD_SCHEMAS.md` + Gateway docs | RO service docs |
| **Instance Creation** | Gateway creates RR | Gateway creates RR (unchanged) |
| **Reconciliation** | RO reconciles RR | RO reconciles RR (unchanged) |
| **Type Imports** | RO imports Gateway types | Gateway imports RO types |

---

## Alternatives Considered

### Alternative 1: Gateway Owns RR Schema (Current State)

**Description**: Gateway defines RR schema since it creates instances.

**Pros**:
- Gateway has full control over what it creates
- No import dependency from Gateway to RO

**Cons**:
- ❌ Violates K8s controller pattern (reconciler should own schema)
- ❌ Gateway is ingestion layer, shouldn't define orchestration artifacts
- ❌ RO can't evolve RR schema without Gateway changes
- ❌ Inverted dependency (orchestrator depends on ingestion layer for types)

**Decision**: ❌ **REJECTED**

---

### Alternative 2: Shared Ownership (Split Schema)

**Description**: Gateway owns `spec`, RO owns `status`.

**Pros**:
- Both services control their relevant parts

**Cons**:
- ❌ Confusing ownership model
- ❌ Schema evolution requires coordination
- ❌ Not a standard K8s pattern
- ❌ Difficult to maintain consistency

**Decision**: ❌ **REJECTED**

---

### Alternative 3: RO Owns RR Schema (Selected)

**Description**: RO defines complete RR schema in `api/remediation/v1alpha1/`. Gateway imports these types to create instances.

**Pros**:
- ✅ Aligns with K8s controller pattern
- ✅ Clean dependency direction (Gateway → RO)
- ✅ RO can evolve schema based on orchestration needs
- ✅ Clear single owner for the CRD
- ✅ Matches how other K8s ecosystems work (e.g., Ingress controller owns Ingress)

**Cons**:
- Gateway must import RO types (minor, acceptable)
- Documentation update required

**Decision**: ✅ **SELECTED**

---

## Implementation

### Phase 1: Documentation Update (Immediate)

1. Update `CRD_SCHEMAS.md`:
   - Change "Owner: Central Controller Service" to "Owner: Remediation Orchestrator"
   - Remove "Created By: Gateway Service" (creation is an action, not ownership)
   - Add "Instances Created By: Gateway Service"

2. Update RO service documentation:
   - Mark as authoritative source for RR schema
   - Document all RR fields with business requirements

3. Update Gateway service documentation:
   - Remove RR schema definitions
   - Reference RO as authoritative source
   - Document that Gateway imports RO types

### Phase 2: Code Organization (If Needed)

If types are currently in Gateway package:
```
# Move from
api/gateway/v1alpha1/remediationrequest_types.go

# To
api/remediation/v1alpha1/remediationrequest_types.go
```

Gateway imports:
```go
import (
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)
```

### Phase 3: Validation

- [ ] All RR schema references point to RO documentation
- [ ] Gateway successfully imports and uses RO types
- [ ] No circular dependencies
- [ ] CRD manifests generated from RO-owned types

---

## Consequences

### Positive

1. **Clear Ownership**: Single owner (RO) for RR CRD schema
2. **K8s Alignment**: Follows established controller patterns
3. **Clean Dependencies**: Gateway → RO (not reverse)
4. **Schema Evolution**: RO can evolve RR based on orchestration needs
5. **Documentation Clarity**: Single source of truth for RR schema

### Negative

1. **Documentation Update**: Significant docs update required
2. **Potential Code Move**: May need to relocate type definitions
3. **Gateway Change**: Gateway must import RO types (minor)

### Neutral

1. **Instance Creation**: Gateway still creates RR instances (unchanged behavior)
2. **Shared Status**: DD-GATEWAY-011 pattern still valid (Gateway updates its status sections)
3. **Reconciliation**: RO still reconciles RR (unchanged)

---

## Compatibility with DD-GATEWAY-011

This ADR is **fully compatible** with DD-GATEWAY-011 (Shared Status Ownership):

| Aspect | DD-GATEWAY-011 | This ADR |
|--------|----------------|----------|
| **Schema Owner** | Not specified | RO |
| **Instance Creator** | Gateway | Gateway (unchanged) |
| **Status Ownership** | Gateway: deduplication, storm; RO: lifecycle | Unchanged |
| **Reconciler** | RO | RO (unchanged) |

The shared status pattern works regardless of who owns the schema definition. Gateway can still update `status.deduplication` and `status.stormAggregation` even though RO owns the overall schema.

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| [DD-GATEWAY-011](DD-GATEWAY-011-shared-status-deduplication.md) | Shared status ownership (compatible) |
| [005-owner-reference-architecture.md](005-owner-reference-architecture.md) | CRD hierarchy (RR is root) |
| [CRD_SCHEMAS.md](../CRD_SCHEMAS.md) | Needs update per this ADR |
| [ADR-001](ADR-001-gateway-ro-deduplication-communication.md) | Deduplication communication |

---

## Acknowledgments

| Team | Acknowledged | Date | Notes |
|------|--------------|------|-------|
| Gateway Service | ✅ **ACKNOWLEDGED** | 2025-12-07 | No code changes required - already imports from `api/remediation/v1alpha1/`. Updated `crd-integration.md` to reference RO as schema owner. |
| Remediation Orchestrator | ✅ **ACKNOWLEDGED** | 2025-12-07 | Accepts schema ownership. Types in `api/remediation/v1alpha1/` |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-07 | Initial decision: RO owns RR CRD schema |
| 1.1 | 2025-12-07 | Gateway team acknowledgment added. Updated `crd-integration.md` to reference RO as schema owner. |


