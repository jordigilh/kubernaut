# Canonical Action Types - Source of Truth

**Version**: 1.0.0
**Last Updated**: 2025-10-07
**Status**: ✅ APPROVED - Single Source of Truth

---

## Purpose

This document defines the **canonical list of 29 predefined action types** supported by Kubernaut. All services MUST use these exact action types.

**Source of Truth**: This list is derived from `pkg/platform/executor/executor.go` which contains the actual registered action handlers.

---

## Canonical Action Types (29 Total)

### Core Actions (P0 - High Frequency - 5 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `scale_deployment` | Scale deployment replicas | P0 | High load, resource optimization |
| `restart_pod` | Restart specific pod | P0 | Memory leaks, stuck processes |
| `increase_resources` | Increase CPU/memory limits | P0 | Resource exhaustion |
| `rollback_deployment` | Rollback to previous version | P0 | Bad deployment, regression |
| `expand_pvc` | Expand PersistentVolumeClaim | P0 | Storage full |

### Infrastructure Actions (P1 - Medium Frequency - 6 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `drain_node` | Drain node for maintenance | P1 | Node maintenance, upgrade |
| `cordon_node` | Mark node unschedulable | P1 | Prevent new pods on node |
| `uncordon_node` | Mark node schedulable | P1 | Re-enable node after maintenance |
| `taint_node` | Apply taints to control pod scheduling | P1 | Node isolation, workload segregation |
| `untaint_node` | Remove taints to allow pod scheduling | P1 | Restore node after issue resolution |
| `quarantine_pod` | Isolate problematic pod | P1 | Security incident, misbehavior |

### Storage & Persistence (P2 - 3 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `cleanup_storage` | Clean up old data | P2 | Disk space management |
| `backup_data` | Create data backup | P2 | Before risky operations |
| `compact_storage` | Compact storage | P2 | Storage optimization |

### Application Lifecycle (P1 - 3 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `update_hpa` | Update HorizontalPodAutoscaler | P1 | Autoscaling adjustment |
| `restart_daemonset` | Restart DaemonSet pods | P1 | DaemonSet issues |
| `scale_statefulset` | Scale StatefulSet replicas | P1 | Stateful workload scaling |

### Security & Compliance (P2 - 3 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `rotate_secrets` | Rotate Kubernetes secrets | P2 | Security compliance |
| `audit_logs` | Collect audit logs | P2 | Security investigation |
| `update_network_policy` | Update NetworkPolicy | P2 | Security policy changes |

### Network & Connectivity (P2 - 2 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `restart_network` | Restart network components | P2 | Network connectivity issues |
| `reset_service_mesh` | Reset service mesh | P2 | Service mesh problems |

### Database & Stateful (P2 - 2 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `failover_database` | Trigger database failover | P2 | Database primary failure |
| `repair_database` | Repair database corruption | P2 | Data corruption issues |

### Monitoring & Observability (P2 - 3 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `enable_debug_mode` | Enable debug logging | P2 | Troubleshooting |
| `create_heap_dump` | Create JVM heap dump | P2 | Memory analysis |
| `collect_diagnostics` | Collect diagnostic data | P2 | Problem investigation |

### Resource Management (P1 - 2 actions)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `optimize_resources` | Optimize resource allocation | P1 | Cost optimization |
| `migrate_workload` | Migrate workload to another node/cluster | P1 | Load balancing, maintenance |

### Fallback (P3 - 1 action)

| Action Type | Description | Priority | Typical Use Case |
|-------------|-------------|----------|------------------|
| `notify_only` | No automated action, notify only | P3 | Unknown issues, manual review required |

---

## Action Type Validation

### Go Implementation

```go
// pkg/shared/types/common.go
var ValidActions = map[string]bool{
    // Core Actions (P0) - 5 actions
    "scale_deployment":      true,
    "restart_pod":           true,
    "increase_resources":    true,
    "rollback_deployment":   true,
    "expand_pvc":            true,

    // Infrastructure Actions (P1) - 6 actions
    "drain_node":            true,
    "cordon_node":           true,
    "uncordon_node":         true,
    "taint_node":            true,
    "untaint_node":          true,
    "quarantine_pod":        true,

    // Storage & Persistence (P2) - 3 actions
    "cleanup_storage":       true,
    "backup_data":           true,
    "compact_storage":       true,

    // Application Lifecycle (P1) - 3 actions
    "update_hpa":            true,
    "restart_daemonset":     true,
    "scale_statefulset":     true,

    // Security & Compliance (P2) - 3 actions
    "rotate_secrets":        true,
    "audit_logs":            true,
    "update_network_policy": true,

    // Network & Connectivity (P2) - 2 actions
    "restart_network":       true,
    "reset_service_mesh":    true,

    // Database & Stateful (P2) - 2 actions
    "failover_database":     true,
    "repair_database":       true,

    // Monitoring & Observability (P2) - 3 actions
    "enable_debug_mode":     true,
    "create_heap_dump":      true,
    "collect_diagnostics":   true,

    // Resource Management (P1) - 2 actions
    "optimize_resources":    true,
    "migrate_workload":      true,

    // Fallback (P3) - 1 action
    "notify_only":           true,
}
```

### Python Implementation (HolmesGPT API)

```python
# Valid action types for HolmesGPT structured responses
VALID_ACTION_TYPES = [
    # Core Actions (P0)
    "scale_deployment",
    "restart_pod",
    "increase_resources",
    "rollback_deployment",
    "expand_pvc",

    # Infrastructure Actions (P1)
    "drain_node",
    "cordon_node",
    "uncordon_node",
    "taint_node",
    "untaint_node",
    "quarantine_pod",

    # Storage & Persistence (P2)
    "cleanup_storage",
    "backup_data",
    "compact_storage",

    # Application Lifecycle (P1)
    "update_hpa",
    "restart_daemonset",
    "scale_statefulset",

    # Security & Compliance (P2)
    "rotate_secrets",
    "audit_logs",
    "update_network_policy",

    # Network & Connectivity (P2)
    "restart_network",
    "reset_service_mesh",

    # Database & Stateful (P2)
    "failover_database",
    "repair_database",

    # Monitoring & Observability (P2)
    "enable_debug_mode",
    "create_heap_dump",
    "collect_diagnostics",

    # Resource Management (P1)
    "optimize_resources",
    "migrate_workload",

    # Fallback (P3)
    "notify_only",
]
```

---

## Deprecation and Extension Policy

### Adding New Actions

1. **Proposal**: Submit design document with business requirement
2. **Implementation**: Add to executor registration first
3. **Documentation**: Update this canonical list
4. **Version**: Increment version number (1.1.0, 1.2.0, etc.)
5. **Rollout**: Use feature flags for gradual adoption

### Deprecating Actions

1. **Announcement**: 6-month deprecation notice
2. **Marking**: Add `deprecated: true` flag with deprecation date
3. **Aliasing**: Support old name as alias for 6 months
4. **Removal**: Remove from canonical list after deprecation period
5. **Version**: Increment major version (2.0.0)

### Version History

| Version | Date | Changes | Action Count |
|---------|------|---------|--------------|
| 1.1.0 | 2025-10-07 | Added taint_node and untaint_node | 29 |
| 1.0.0 | 2025-10-07 | Initial canonical list | 27 |

---

## Compliance Requirements

### All Services MUST

1. ✅ Use ONLY these 29 action types
2. ✅ Validate action types against this list
3. ✅ Reject unknown action types OR use fuzzy matching to map to these
4. ✅ Reference this document as source of truth
5. ✅ Update implementations when this list changes

### Service-Specific Compliance

**HolmesGPT API Service**:
- MUST generate ONLY these 29 action types in structured responses
- MUST validate responses against this list
- MUST use fuzzy matching to map similar actions to canonical names

**AI Analysis Service**:
- MUST validate structured actions against this list
- MUST reject actions not in this list (or use fuzzy matching)
- MUST use these exact type names in Go constants

**Workflow Execution Service**:
- MUST accept ONLY these 29 action types in workflow steps
- MUST validate workflow definitions against this list
- MUST fail workflow creation for unknown action types

**Context API Service**:
- MUST provide success rates for these 29 action types
- MUST enforce environment constraints using these action names
- MUST return historical patterns using these action names

**Kubernetes Executor Service**:
- MUST have handlers registered for all 29 actions
- MUST reject execution requests for unknown actions
- IS the authoritative source for what actions are implemented

---

## Cross-Reference

**Files That MUST Match This List**:
- `pkg/shared/types/common.go` - ValidActions map
- `pkg/platform/executor/executor.go` - registerBuiltinActions()
- `docs/services/stateless/holmesgpt-api/api-specification.md` - Valid Action Types section
- `docs/services/crd-controllers/02-aianalysis/integration-points.md` - ActionType constants
- `docs/services/crd-controllers/03-workflowexecution/integration-points.md` - Action validation
- `docs/services/stateless/context-api/api-specification.md` - Action success rates
- `docs/services/crd-controllers/04-kubernetesexecutor/predefined-actions.md` - Action reference

---

## Audit Checklist

Before any release, verify:
- [ ] All 29 actions registered in executor
- [ ] ValidActions map matches this list exactly
- [ ] All service specs reference these exact action names
- [ ] No services use deprecated or non-canonical action names
- [ ] Integration tests cover all 29 actions
- [ ] Documentation is consistent across all services

---

## Questions & Answers

**Q: Why 29 actions?**
A: These are the actions currently implemented and registered in the executor. This is the reality of what the system can actually do.

**Q: Can I add more actions?**
A: Yes, but follow the extension policy. Add to executor first, then update this canonical list, then update all service specs.

**Q: What about the 40+ actions in ValidActions?**
A: Those are being REMOVED. Only these 29 are actually implemented in the executor.

**Q: Why not support all 40+ actions?**
A: Because they're not implemented. Having them in ValidActions creates false expectations and integration failures.

**Q: What if HolmesGPT generates an unknown action?**
A: Use fuzzy matching to map to a canonical action, or fall back to `notify_only`.

---

**Document Owner**: Platform Team
**Review Frequency**: Quarterly or when actions are added/removed
**Next Review Date**: 2026-01-07
