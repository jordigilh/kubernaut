# Predefined Actions - Kubernetes Executor

**Version**: 1.0.0
**Last Updated**: 2025-10-07
**Source of Truth**: `docs/design/CANONICAL_ACTION_TYPES.md`

---

## Overview

This document describes the 27 canonical predefined actions that can be executed by the Kubernetes Executor service. These actions are the **only** actions that can be referenced in structured action formats from HolmesGPT API responses.

**IMPORTANT**: This list MUST match:
- `pkg/shared/types/common.go` - ValidActions map
- `pkg/platform/executor/executor.go` - registerBuiltinActions()
- All service specifications

---

## Action Type Reference

### Core Actions (P0 - High Frequency)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P0** | `scale_deployment` | 25% | deployment, namespace, replicas | `scale-deployment-sa` | 10-30s | Scale deployment replicas |
| **P0** | `restart_pod` | 20% | pod, namespace | `restart-pod-sa` | 5-15s | Restart specific pod |
| **P0** | `increase_resources` | 15% | deployment, namespace, memory, cpu | `increase-resources-sa` | 30s-2m | Increase CPU/memory limits |
| **P0** | `rollback_deployment` | 10% | deployment, namespace, revision | `rollback-deployment-sa` | 30s-2m | Rollback to previous version |
| **P0** | `expand_pvc` | 5% | pvc, namespace, size | `expand-pvc-sa` | 1-5m | Expand PersistentVolumeClaim |

### Infrastructure Actions (P1 - Medium Frequency)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P1** | `drain_node` | 5% | node, gracePeriod | `drain-node-sa` | 1-5m | Drain node for maintenance |
| **P1** | `cordon_node` | 5% | node | `cordon-node-sa` | 2-5s | Mark node unschedulable |
| **P1** | `uncordon_node` | 5% | node | `uncordon-node-sa` | 2-5s | Mark node schedulable |
| **P1** | `taint_node` | 4% | node, key, effect, value | `taint-node-sa` | 5-10s | Apply node taints |
| **P1** | `untaint_node` | 4% | node, key, effect | `untaint-node-sa` | 2-5s | Remove node taints |
| **P1** | `quarantine_pod` | 3% | pod, namespace | `quarantine-pod-sa` | 10-20s | Isolate problematic pod |

### Storage & Persistence (P2)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P2** | `cleanup_storage` | 3% | namespace, path | `cleanup-storage-sa` | 30s-5m | Clean up old data |
| **P2** | `backup_data` | 2% | namespace, resource, destination | `backup-data-sa` | 1-10m | Create data backup |
| **P2** | `compact_storage` | 2% | namespace, volume | `compact-storage-sa` | 5-30m | Compact storage |

### Application Lifecycle (P1)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P1** | `update_hpa` | 3% | hpa, namespace, minReplicas, maxReplicas | `update-hpa-sa` | 5-10s | Update HorizontalPodAutoscaler |
| **P1** | `restart_daemonset` | 2% | daemonset, namespace | `restart-daemonset-sa` | 30s-2m | Restart DaemonSet pods |
| **P1** | `scale_statefulset` | 3% | statefulset, namespace, replicas | `scale-statefulset-sa` | 30s-5m | Scale StatefulSet replicas |

### Security & Compliance (P2)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P2** | `rotate_secrets` | 2% | secret, namespace | `rotate-secrets-sa` | 10-30s | Rotate Kubernetes secrets |
| **P2** | `audit_logs` | 1% | namespace, timeRange | `audit-logs-sa` | 30s-2m | Collect audit logs |
| **P2** | `update_network_policy` | 2% | networkpolicy, namespace, rules | `update-network-policy-sa` | 5-15s | Update NetworkPolicy |

### Network & Connectivity (P2)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P2** | `restart_network` | 2% | namespace, component | `restart-network-sa` | 30s-2m | Restart network components |
| **P2** | `reset_service_mesh` | 1% | namespace | `reset-service-mesh-sa` | 1-5m | Reset service mesh |

### Database & Stateful (P2)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P2** | `failover_database` | 1% | database, namespace | `failover-database-sa` | 30s-5m | Trigger database failover |
| **P2** | `repair_database` | 1% | database, namespace | `repair-database-sa` | 5-30m | Repair database corruption |

### Monitoring & Observability (P2)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P2** | `enable_debug_mode` | 2% | deployment, namespace | `enable-debug-mode-sa` | 5-15s | Enable debug logging |
| **P2** | `create_heap_dump` | 1% | pod, namespace | `create-heap-dump-sa` | 30s-2m | Create JVM heap dump |
| **P2** | `collect_diagnostics` | 3% | namespace, resource | `collect-diagnostics-sa` | 1-5m | Collect diagnostic data |

### Resource Management (P1)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P1** | `optimize_resources` | 3% | namespace, resourceType | `optimize-resources-sa` | 30s-5m | Optimize resource allocation |
| **P1** | `migrate_workload` | 2% | workload, namespace, targetNode | `migrate-workload-sa` | 1-10m | Migrate workload to another node/cluster |

### Fallback (P3)

| Priority | Action Type | Coverage | Parameters | ServiceAccount | Typical Duration | Description |
|----------|------------|----------|------------|----------------|------------------|-------------|
| **P3** | `notify_only` | N/A | message | N/A | <1s | No automated action, notify only |

**Total Coverage**: ~100% of common remediation actions
**Total Actions**: 27 canonical action types

---

## Action Handler Implementation

### Example: Scale Deployment Handler

```go
// pkg/platform/executor/actions/scale_deployment.go
package actions

import (
    "context"
    "fmt"

    appsv1 "k8s.io/api/apps/v1"
    "sigs.k8s.io/controller-runtime/client"
)

type ScaleDeploymentHandler struct {
    client client.Client
}

func (h *ScaleDeploymentHandler) Execute(ctx context.Context, params map[string]interface{}) error {
    namespace := params["namespace"].(string)
    deploymentName := params["resource_name"].(string)
    replicas := int32(params["replicas"].(float64))

    // Get deployment
    deployment := &appsv1.Deployment{}
    if err := h.client.Get(ctx, client.ObjectKey{
        Namespace: namespace,
        Name:      deploymentName,
    }, deployment); err != nil {
        return fmt.Errorf("failed to get deployment: %w", err)
    }

    // Update replicas
    deployment.Spec.Replicas = &replicas
    if err := h.client.Update(ctx, deployment); err != nil {
        return fmt.Errorf("failed to scale deployment: %w", err)
    }

    return nil
}

func (h *ScaleDeploymentHandler) GetServiceAccount() string {
    return "scale-deployment-sa"
}

func (h *ScaleDeploymentHandler) GetTimeout() time.Duration {
    return 2 * time.Minute
}
```

---

## Action Registration

All actions MUST be registered in `pkg/platform/executor/executor.go`:

```go
func (e *executor) registerBuiltinActions() error {
    // Core Actions (P0)
    e.registry.Register("scale_deployment", e.executeScaleDeployment)
    e.registry.Register("restart_pod", e.executeRestartPod)
    e.registry.Register("increase_resources", e.executeIncreaseResources)
    e.registry.Register("rollback_deployment", e.executeRollbackDeployment)
    e.registry.Register("expand_pvc", e.executeExpandPVC)

    // Infrastructure Actions (P1)
    e.registry.Register("drain_node", e.executeDrainNode)
    e.registry.Register("cordon_node", e.executeCordonNode)
    e.registry.Register("uncordon_node", e.executeUncordonNode)
    e.registry.Register("taint_node", e.executeTaintNode)
    e.registry.Register("untaint_node", e.executeUntaintNode)
    e.registry.Register("quarantine_pod", e.executeQuarantinePod)

    // Storage & Persistence (P2)
    e.registry.Register("cleanup_storage", e.executeCleanupStorage)
    e.registry.Register("backup_data", e.executeBackupData)
    e.registry.Register("compact_storage", e.executeCompactStorage)

    // Application Lifecycle (P1)
    e.registry.Register("update_hpa", e.executeUpdateHPA)
    e.registry.Register("restart_daemonset", e.executeRestartDaemonSet)
    e.registry.Register("scale_statefulset", e.executeScaleStatefulSet)

    // Security & Compliance (P2)
    e.registry.Register("rotate_secrets", e.executeRotateSecrets)
    e.registry.Register("audit_logs", e.executeAuditLogs)
    e.registry.Register("update_network_policy", e.executeUpdateNetworkPolicy)

    // Network & Connectivity (P2)
    e.registry.Register("restart_network", e.executeRestartNetwork)
    e.registry.Register("reset_service_mesh", e.executeResetServiceMesh)

    // Database & Stateful (P2)
    e.registry.Register("failover_database", e.executeFailoverDatabase)
    e.registry.Register("repair_database", e.executeRepairDatabase)

    // Monitoring & Observability (P2)
    e.registry.Register("enable_debug_mode", e.executeEnableDebugMode)
    e.registry.Register("create_heap_dump", e.executeCreateHeapDump)
    e.registry.Register("collect_diagnostics", e.executeCollectDiagnostics)

    // Resource Management (P1)
    e.registry.Register("optimize_resources", e.executeOptimizeResources)
    e.registry.Register("migrate_workload", e.executeMigrateWorkload)

    // Fallback (P3)
    e.registry.Register("notify_only", e.executeNotifyOnly)

    return nil
}
```

---

## Validation

All services MUST validate action types against this list:

```go
import "github.com/jordigilh/kubernaut/pkg/shared/types"

func ValidateAction(actionType string) error {
    if !types.IsValidAction(actionType) {
        return fmt.Errorf("invalid action type: %s", actionType)
    }
    return nil
}
```

---

## Adding New Actions

To add a new action:

1. ✅ Implement handler in `pkg/platform/executor/`
2. ✅ Register in `registerBuiltinActions()`
3. ✅ Add to `ValidActions` map in `pkg/shared/types/common.go`
4. ✅ Update `docs/design/CANONICAL_ACTION_TYPES.md`
5. ✅ Update all service specifications
6. ✅ Add tests for new action
7. ✅ Update this documentation

---

## Cross-References

- **Canonical List**: `docs/design/CANONICAL_ACTION_TYPES.md`
- **Executor Implementation**: `pkg/platform/executor/executor.go`
- **Type Validation**: `pkg/shared/types/common.go`
- **HolmesGPT API Spec**: `docs/services/stateless/holmesgpt-api/api-specification.md`
- **AI Analysis Spec**: `docs/services/crd-controllers/02-aianalysis/integration-points.md`
- **Workflow Spec**: `docs/services/crd-controllers/03-workflowexecution/integration-points.md`

---

**Document Owner**: Platform Team
**Review Frequency**: When actions are added/removed
**Next Review Date**: 2026-01-07