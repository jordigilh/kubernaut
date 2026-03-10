# DD-GATEWAY-013 → ADR-052 Conversion: Distributed Locking Pattern

**Date**: December 30, 2025
**Type**: Design Decision → Architecture Decision Record Conversion
**Reason**: Cross-service architectural pattern
**Status**: ✅ **COMPLETE** - ADR-052 created, DD-GATEWAY-013 deleted

---

## 🎯 **Why Convert DD to ADR?**

**Current State**: DD-GATEWAY-013 documents distributed locking for Gateway service only

**New Reality**: Distributed locking is now a **cross-service architectural pattern** used by:
1. ✅ Gateway Service (signal deduplication)
2. ✅ RemediationOrchestrator Service (resource lock safety)
3. 🔮 Future: Any service with multi-replica race conditions

**ADR Criteria Met**:
- ✅ **Cross-Service Impact**: Affects multiple services
- ✅ **Shared Infrastructure**: Shared lock manager (`pkg/shared/locking/`)
- ✅ **Architectural Pattern**: K8s Lease-based mutual exclusion
- ✅ **Long-Term Decision**: Not easily reversible
- ✅ **RBAC Implications**: Requires coordination.k8s.io/leases permissions

**Conclusion**: This is an **architectural decision**, not a service-specific design decision.

---

## 📋 **Conversion Plan**

### **New ADR**

**Number**: ADR-052
**Title**: K8s Lease-Based Distributed Locking for Multi-Replica Race Protection
**Status**: Approved
**Date**: December 30, 2025

### **Content Structure**

```markdown
# ADR-052: K8s Lease-Based Distributed Locking Pattern

## Status
**Status**: ✅ APPROVED
**Date**: December 30, 2025
**Supersedes**: DD-GATEWAY-013 (converted to ADR for cross-service applicability)

## Context

Multiple Kubernaut services run with horizontal scaling (2+ replicas for HA).
This creates race conditions when concurrent requests target the same resource
across different pod instances.

**Affected Services**:
- Gateway: Signal deduplication (cross-second race on same fingerprint)
- RemediationOrchestrator: Resource locking (duplicate WFE for same target)

## Decision

Use Kubernetes Lease resources for distributed mutual exclusion across service replicas.

**Implementation**: Each service implements the pattern independently (copy and adapt)

**Pattern**:
1. Acquire K8s Lease before critical section
2. Execute protected operation (check-then-create)
3. Release Lease after operation

## Consequences

**Positive**:
- ✅ Eliminates multi-replica race conditions
- ✅ K8s-native (no external dependencies)
- ✅ Fault-tolerant (lease expires on pod crash)
- ✅ Scales safely (1 to 100+ replicas)
- ✅ Service-specific customization (lock keys, metrics, lease duration)
- ✅ Independent teams (no cross-team coordination overhead)

**Negative**:
- ⚠️ +10-20ms latency per operation
- ⚠️ Additional K8s API load (2 API calls per operation)
- ⚠️ RBAC requirement (coordination.k8s.io/leases permissions)
- ⚠️ Code duplication (~200 lines per service - acceptable for independence)

## Implementation

**Pattern-Based Approach** (NOT shared library):
- Each service implements its own lock manager
- Gateway's implementation serves as reference
- Services adapt for their specific needs

**Why Not Shared Library**:
- Metrics coupling complexity (each service has different metrics)
- Service-specific lock key generation logic
- YAGNI principle (2 services don't justify abstraction)
- Faster implementation, simpler testing

**Services Using Pattern**:
| Service | Implementation | Lock Key | Use Case | BR Reference |
|---------|---------------|----------|----------|--------------|
| Gateway | `pkg/gateway/processing/distributed_lock.go` | Signal fingerprint | Deduplication | BR-GATEWAY-190 |
| RemediationOrchestrator | `pkg/remediationorchestrator/locking/distributed_lock.go` | Target resource | Resource lock | BR-ORCH-050 |

**RBAC Requirement**:
```yaml
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update", "delete"]
```
```

---

## ✅ **Conversion Complete**

### **What Was Done** (December 30, 2025)

1. ✅ **Created ADR-052**: [ADR-052-distributed-locking-pattern.md](../architecture/decisions/ADR-052-distributed-locking-pattern.md)
   - **Size**: Comprehensive pattern documentation
   - **Approach**: Hybrid (pattern + when-to-use guidelines)
   - **Implementation**: Independent (copy-adapt, not shared library)
   - **Business Requirements**: BR-GATEWAY-190, BR-ORCH-050 referenced
   - **Code Examples**: Skeleton with links to real implementations

2. ✅ **Deleted DD-GATEWAY-013**: Fully migrated content to ADR-052 (user decision: Q1-B)
   - No deprecation notice (clean migration)
   - All content preserved in ADR-052

3. ✅ **Updated Cross-References**:
   - [CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md](../shared/CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md) → Points to ADR-052
   - Gateway Implementation Plan → References ADR-052
   - RO Implementation Plan → References ADR-052

4. ✅ **Key Decisions Documented**:
   - **Why not shared library**: YAGNI principle, metrics coupling complexity
   - **When to use**: Multi-replica + check-then-create + business impact
   - **How to adapt**: Copy Gateway's implementation, customize for service

## 🔄 **Migration Checklist** (Complete)

- [x] Create ADR-052 document with full context (pattern documentation)
- [x] Delete DD-GATEWAY-013 (full migration, no deprecation)
- [x] Update Gateway implementation plan to reference ADR-052
- [x] Update RO implementation plan to reference ADR-052
- [x] Update cross-team coordination document to reference ADR-052
- [ ] Add ADR-052 to architecture decisions README (deferred - can be done anytime)
- [ ] Update Gateway's lock manager code comments to reference ADR-052 (during implementation)
- [ ] Update RO's lock manager code comments to reference ADR-052 (during implementation)

---

## 📚 **Related Documentation**

### **Source Documents**
- `docs/architecture/decisions/DD-GATEWAY-013-multi-replica-deduplication.md` (to be deprecated)
- CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md (cross-team recommendation; document removed)
### **Implementation Plans**
- `docs/services/stateless/gateway-service/implementation/IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md`
- `docs/services/crd-controllers/05-remediationorchestrator/implementation/IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md` (to be created)

### **Test Plans**
- `docs/services/stateless/gateway-service/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md`
- `docs/services/crd-controllers/05-remediationorchestrator/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md` (to be created)

---

## ✅ **Next Steps**

1. **Create ADR-052** with full architectural context
2. **Update DD-GATEWAY-013** with deprecation notice pointing to ADR-052
3. **Update implementation plans** to reference ADR-052 instead of DD-GATEWAY-013
4. **Create shared lock manager** in `pkg/shared/locking/`
5. **Refactor Gateway** to use shared lock manager
6. **Implement RO** distributed locking using shared lock manager

---

**Status**: 📋 **Conversion Pending** - Will be completed in next branch
**Timeline**: Alongside RO and Gateway distributed locking implementation

