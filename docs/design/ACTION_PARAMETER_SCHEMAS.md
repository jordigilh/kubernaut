# Action Parameter Schemas

**Version**: 1.0.0
**Last Updated**: 2025-10-07
**Status**: ✅ APPROVED - Parameter Validation Specification

---

## Purpose

This document defines the **required and optional parameters** for each of the 29 canonical action types. All services MUST validate action parameters against these schemas.

**Source of Truth**: This schema complements `docs/design/CANONICAL_ACTION_TYPES.md` by providing detailed parameter specifications.

---

## Schema Format

Each action schema includes:
- **Required Parameters**: MUST be present
- **Optional Parameters**: MAY be present
- **Parameter Types**: Data types (string, integer, boolean, etc.)
- **Validation Rules**: Constraints (min/max, patterns, enums)
- **Examples**: Sample parameter sets

---

## Core Actions (P0)

### 1. scale_deployment

**Description**: Scale deployment replicas

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
  pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"

resource_name: string
  description: "Deployment name"
  pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"

replicas: integer
  description: "Target number of replicas"
  minimum: 0
  maximum: 100
```

**Optional Parameters**:
```yaml
reason: string
  description: "Reason for scaling"

rollback_on_failure: boolean
  description: "Rollback to previous replica count if scaling fails"
  default: true
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "api-server",
  "replicas": 5,
  "reason": "high_load",
  "rollback_on_failure": true
}
```

---

### 2. restart_pod

**Description**: Restart specific pod

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
  pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"

resource_name: string
  description: "Pod name or deployment name"
  pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
```

**Optional Parameters**:
```yaml
resource_type: string
  description: "Resource type (pod or deployment)"
  enum: ["pod", "deployment"]
  default: "pod"

grace_period_seconds: integer
  description: "Grace period before force termination"
  minimum: 0
  maximum: 300
  default: 30

reason: string
  description: "Reason for restart"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "api-server-abc123",
  "resource_type": "pod",
  "grace_period_seconds": 30,
  "reason": "high_memory_usage"
}
```

---

### 3. increase_resources

**Description**: Increase CPU/memory limits

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Deployment, StatefulSet, or DaemonSet name"

resource_type: string
  description: "Resource type"
  enum: ["deployment", "statefulset", "daemonset"]
```

**Optional Parameters**:
```yaml
memory: string
  description: "Memory limit (e.g., '4Gi', '512Mi')"
  pattern: "^[0-9]+(Mi|Gi|M|G)$"

cpu: string
  description: "CPU limit (e.g., '2', '500m')"
  pattern: "^[0-9]+(m)?$"

container_name: string
  description: "Specific container to update (default: all)"

reason: string
  description: "Reason for resource increase"
```

**Validation Rules**:
- At least one of `memory` or `cpu` MUST be specified

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "api-server",
  "resource_type": "deployment",
  "memory": "4Gi",
  "cpu": "2",
  "reason": "resource_exhaustion"
}
```

---

### 4. rollback_deployment

**Description**: Rollback to previous deployment version

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Deployment name"
```

**Optional Parameters**:
```yaml
revision: integer
  description: "Specific revision to rollback to (default: previous)"
  minimum: 1

reason: string
  description: "Reason for rollback"

wait_for_ready: boolean
  description: "Wait for rollback to complete"
  default: true

timeout: string
  description: "Rollback timeout"
  pattern: "^[0-9]+(s|m|h)$"
  default: "5m"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "api-server",
  "revision": 5,
  "reason": "bad_deployment",
  "wait_for_ready": true
}
```

---

### 5. expand_pvc

**Description**: Expand PersistentVolumeClaim

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "PVC name"

size: string
  description: "New size (e.g., '100Gi')"
  pattern: "^[0-9]+(Mi|Gi|Ti|M|G|T)$"
```

**Optional Parameters**:
```yaml
reason: string
  description: "Reason for expansion"

wait_for_expansion: boolean
  description: "Wait for expansion to complete"
  default: true
```

**Validation Rules**:
- New size MUST be larger than current size

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "data-volume",
  "size": "100Gi",
  "reason": "storage_full"
}
```

---

## Infrastructure Actions (P1)

### 6. drain_node

**Description**: Drain node for maintenance

**Required Parameters**:
```yaml
node_name: string
  description: "Node name"
```

**Optional Parameters**:
```yaml
grace_period_seconds: integer
  description: "Grace period for pod eviction"
  minimum: 0
  maximum: 3600
  default: 300

ignore_daemonsets: boolean
  description: "Ignore DaemonSet pods"
  default: true

delete_local_data: boolean
  description: "Delete pods with local data"
  default: false

force: boolean
  description: "Force drain even if PDBs are violated"
  default: false

reason: string
  description: "Reason for drain"
```

**Example**:
```json
{
  "node_name": "worker-node-01",
  "grace_period_seconds": 300,
  "ignore_daemonsets": true,
  "reason": "node_maintenance"
}
```

---

### 7. cordon_node

**Description**: Mark node unschedulable

**Required Parameters**:
```yaml
node_name: string
  description: "Node name"
```

**Optional Parameters**:
```yaml
reason: string
  description: "Reason for cordoning"
```

**Example**:
```json
{
  "node_name": "worker-node-01",
  "reason": "preparing_for_maintenance"
}
```

---

### 8. uncordon_node

**Description**: Mark node schedulable

**Required Parameters**:
```yaml
node_name: string
  description: "Node name"
```

**Optional Parameters**:
```yaml
reason: string
  description: "Reason for uncordoning"

verify_health: boolean
  description: "Verify node health before uncordoning"
  default: true
```

**Example**:
```json
{
  "node_name": "worker-node-01",
  "reason": "maintenance_complete",
  "verify_health": true
}
```

---

### 9. taint_node

**Description**: Apply taints to control pod scheduling and eviction behavior

**Required Parameters**:
```yaml
resource_name: string
  description: "Node name"
  pattern: "^[a-z0-9.-]+$"

key: string
  description: "Taint key (e.g., 'maintenance', 'disk-issue')"
  pattern: "^[a-zA-Z0-9/_.-]+$"

effect: string
  description: "Taint effect controlling scheduling behavior"
  enum: ["NoSchedule", "PreferNoSchedule", "NoExecute"]
```

**Optional Parameters**:
```yaml
value: string
  description: "Optional taint value"
  pattern: "^[a-zA-Z0-9/_.-]*$"
  default: ""

overwrite: boolean
  description: "If true, overwrite existing taint with same key"
  default: false

reason: string
  description: "Reason for applying the taint"
  default: "automated_remediation"
```

**Example**:
```json
{
  "resource_name": "node-1.example.com",
  "key": "disk-issue",
  "value": "intermittent",
  "effect": "NoExecute",
  "reason": "disk_errors_detected"
}
```

---

### 10. untaint_node

**Description**: Remove taints from a node to allow normal pod scheduling

**Required Parameters**:
```yaml
resource_name: string
  description: "Node name"
  pattern: "^[a-z0-9.-]+$"

key: string
  description: "Taint key to remove (e.g., 'maintenance', 'disk-issue')"
  pattern: "^[a-zA-Z0-9/_.-]+$"
```

**Optional Parameters**:
```yaml
effect: string
  description: "Optional: Specific taint effect to remove. If omitted, removes all taints with matching key."
  enum: ["NoSchedule", "PreferNoSchedule", "NoExecute", ""]

verify_health: boolean
  description: "If true, verify node health before untainting"
  default: true

reason: string
  description: "Reason for removing the taint"
  default: "issue_resolved"
```

**Example**:
```json
{
  "resource_name": "node-1.example.com",
  "key": "disk-issue",
  "effect": "NoExecute",
  "verify_health": true,
  "reason": "issue_resolved"
}
```

---

### 11. quarantine_pod

**Description**: Isolate problematic pod

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Pod name"
```

**Optional Parameters**:
```yaml
quarantine_label: string
  description: "Label to apply for quarantine"
  default: "quarantined=true"

remove_from_service: boolean
  description: "Remove from service endpoints"
  default: true

reason: string
  description: "Reason for quarantine"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "suspicious-pod-xyz",
  "remove_from_service": true,
  "reason": "security_incident"
}
```

---

## Storage & Persistence (P2)

### 12. cleanup_storage

**Description**: Clean up old data

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
```

**Optional Parameters**:
```yaml
path: string
  description: "Specific path to clean"

age_threshold: string
  description: "Delete files older than this (e.g., '7d', '30d')"
  pattern: "^[0-9]+(d|h)$"
  default: "7d"

dry_run: boolean
  description: "Simulate cleanup without deleting"
  default: false

reason: string
  description: "Reason for cleanup"
```

**Example**:
```json
{
  "namespace": "production",
  "path": "/var/log/old",
  "age_threshold": "30d",
  "dry_run": false
}
```

---

### 13. backup_data

**Description**: Create data backup

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Resource to backup (PVC, StatefulSet, etc.)"
```

**Optional Parameters**:
```yaml
resource_type: string
  description: "Resource type"
  enum: ["pvc", "statefulset", "database"]
  default: "pvc"

destination: string
  description: "Backup destination (S3, GCS, etc.)"

retention_days: integer
  description: "Backup retention period"
  minimum: 1
  maximum: 365
  default: 30

reason: string
  description: "Reason for backup"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "database-pvc",
  "resource_type": "pvc",
  "destination": "s3://backups/prod",
  "retention_days": 30
}
```

---

### 14. compact_storage

**Description**: Compact storage

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
```

**Optional Parameters**:
```yaml
volume_name: string
  description: "Specific volume to compact"

reason: string
  description: "Reason for compaction"
```

**Example**:
```json
{
  "namespace": "production",
  "volume_name": "data-volume",
  "reason": "storage_optimization"
}
```

---

## Application Lifecycle (P1)

### 15. update_hpa

**Description**: Update HorizontalPodAutoscaler

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "HPA name"
```

**Optional Parameters**:
```yaml
min_replicas: integer
  description: "Minimum replicas"
  minimum: 1
  maximum: 100

max_replicas: integer
  description: "Maximum replicas"
  minimum: 1
  maximum: 1000

target_cpu_utilization: integer
  description: "Target CPU utilization percentage"
  minimum: 1
  maximum: 100

target_memory_utilization: integer
  description: "Target memory utilization percentage"
  minimum: 1
  maximum: 100

reason: string
  description: "Reason for HPA update"
```

**Validation Rules**:
- If both specified: `min_replicas` <= `max_replicas`

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "api-server-hpa",
  "min_replicas": 2,
  "max_replicas": 10,
  "target_cpu_utilization": 80
}
```

---

### 16. restart_daemonset

**Description**: Restart DaemonSet pods

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "DaemonSet name"
```

**Optional Parameters**:
```yaml
reason: string
  description: "Reason for restart"

rolling_restart: boolean
  description: "Restart pods one at a time"
  default: true
```

**Example**:
```json
{
  "namespace": "kube-system",
  "resource_name": "fluentd",
  "rolling_restart": true
}
```

---

### 17. scale_statefulset

**Description**: Scale StatefulSet replicas

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "StatefulSet name"

replicas: integer
  description: "Target number of replicas"
  minimum: 0
  maximum: 100
```

**Optional Parameters**:
```yaml
reason: string
  description: "Reason for scaling"

wait_for_ready: boolean
  description: "Wait for all replicas to be ready"
  default: true
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "cassandra",
  "replicas": 5,
  "wait_for_ready": true
}
```

---

## Security & Compliance (P2)

### 18. rotate_secrets

**Description**: Rotate Kubernetes secrets

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Secret name"
```

**Optional Parameters**:
```yaml
restart_pods: boolean
  description: "Restart pods using this secret"
  default: true

backup_old_secret: boolean
  description: "Backup old secret before rotation"
  default: true

reason: string
  description: "Reason for rotation"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "database-credentials",
  "restart_pods": true,
  "backup_old_secret": true
}
```

---

### 19. audit_logs

**Description**: Collect audit logs

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
```

**Optional Parameters**:
```yaml
time_range: string
  description: "Time range for logs (e.g., '1h', '24h')"
  pattern: "^[0-9]+(h|d)$"
  default: "24h"

resource_name: string
  description: "Specific resource to audit"

output_format: string
  description: "Output format"
  enum: ["json", "text"]
  default: "json"
```

**Example**:
```json
{
  "namespace": "production",
  "time_range": "24h",
  "output_format": "json"
}
```

---

### 20. update_network_policy

**Description**: Update NetworkPolicy

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "NetworkPolicy name"
```

**Optional Parameters**:
```yaml
rules: object
  description: "NetworkPolicy rules (ingress/egress)"

reason: string
  description: "Reason for policy update"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "default-deny",
  "rules": {
    "ingress": [{"from": [{"podSelector": {}}]}]
  }
}
```

---

## Network & Connectivity (P2)

### 21. restart_network

**Description**: Restart network components

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
```

**Optional Parameters**:
```yaml
component: string
  description: "Specific component to restart (e.g., 'coredns', 'calico')"

reason: string
  description: "Reason for restart"
```

**Example**:
```json
{
  "namespace": "kube-system",
  "component": "coredns",
  "reason": "dns_resolution_issues"
}
```

---

### 22. reset_service_mesh

**Description**: Reset service mesh

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
```

**Optional Parameters**:
```yaml
mesh_type: string
  description: "Service mesh type"
  enum: ["istio", "linkerd", "consul"]
  default: "istio"

reason: string
  description: "Reason for reset"
```

**Example**:
```json
{
  "namespace": "istio-system",
  "mesh_type": "istio",
  "reason": "service_mesh_misconfiguration"
}
```

---

## Database & Stateful (P2)

### 23. failover_database

**Description**: Trigger database failover

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Database StatefulSet or deployment name"
```

**Optional Parameters**:
```yaml
target_replica: string
  description: "Target replica to promote to primary"

force: boolean
  description: "Force failover even if primary is healthy"
  default: false

reason: string
  description: "Reason for failover"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "postgresql",
  "target_replica": "postgresql-1",
  "force": false
}
```

---

### 24. repair_database

**Description**: Repair database corruption

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Database name"
```

**Optional Parameters**:
```yaml
repair_type: string
  description: "Type of repair"
  enum: ["index", "full", "corruption_check"]
  default: "corruption_check"

backup_before_repair: boolean
  description: "Create backup before repair"
  default: true

reason: string
  description: "Reason for repair"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "mongodb",
  "repair_type": "index",
  "backup_before_repair": true
}
```

---

## Monitoring & Observability (P2)

### 25. enable_debug_mode

**Description**: Enable debug logging

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Deployment or pod name"
```

**Optional Parameters**:
```yaml
resource_type: string
  description: "Resource type"
  enum: ["deployment", "pod"]
  default: "deployment"

duration: string
  description: "How long to enable debug mode"
  pattern: "^[0-9]+(m|h)$"
  default: "1h"

auto_revert: boolean
  description: "Automatically revert after duration"
  default: true
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "api-server",
  "duration": "30m",
  "auto_revert": true
}
```

---

### 26. create_heap_dump

**Description**: Create JVM heap dump

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Pod name"
```

**Optional Parameters**:
```yaml
output_path: string
  description: "Path to save heap dump"
  default: "/tmp/heap-dump.hprof"

live_objects_only: boolean
  description: "Dump only live objects"
  default: true
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "java-app-xyz",
  "output_path": "/tmp/heap-dump.hprof",
  "live_objects_only": true
}
```

---

### 27. collect_diagnostics

**Description**: Collect diagnostic data

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
```

**Optional Parameters**:
```yaml
resource_name: string
  description: "Specific resource to collect diagnostics from"

include_logs: boolean
  description: "Include container logs"
  default: true

include_metrics: boolean
  description: "Include metrics"
  default: true

include_events: boolean
  description: "Include Kubernetes events"
  default: true

time_range: string
  description: "Time range for diagnostics"
  pattern: "^[0-9]+(h|d)$"
  default: "1h"
```

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "api-server",
  "include_logs": true,
  "include_metrics": true,
  "time_range": "1h"
}
```

---

## Resource Management (P1)

### 28. optimize_resources

**Description**: Optimize resource allocation

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"
```

**Optional Parameters**:
```yaml
resource_type: string
  description: "Resource type to optimize"
  enum: ["deployment", "statefulset", "daemonset", "all"]
  default: "all"

optimization_strategy: string
  description: "Optimization strategy"
  enum: ["cost", "performance", "balanced"]
  default: "balanced"

dry_run: boolean
  description: "Simulate optimization without applying"
  default: false
```

**Example**:
```json
{
  "namespace": "production",
  "resource_type": "deployment",
  "optimization_strategy": "cost",
  "dry_run": false
}
```

---

### 29. migrate_workload

**Description**: Migrate workload to another node/cluster

**Required Parameters**:
```yaml
namespace: string
  description: "Kubernetes namespace"

resource_name: string
  description: "Workload name"

resource_type: string
  description: "Resource type"
  enum: ["deployment", "statefulset", "daemonset"]
```

**Optional Parameters**:
```yaml
target_node: string
  description: "Target node name (for same-cluster migration)"

target_cluster: string
  description: "Target cluster name (for cross-cluster migration)"

drain_source: boolean
  description: "Drain source after migration"
  default: false

reason: string
  description: "Reason for migration"
```

**Validation Rules**:
- Either `target_node` OR `target_cluster` MUST be specified, not both

**Example**:
```json
{
  "namespace": "production",
  "resource_name": "api-server",
  "resource_type": "deployment",
  "target_node": "worker-node-05",
  "drain_source": false
}
```

---

## Fallback (P3)

### 30. notify_only

**Description**: No automated action, notify only

**Required Parameters**:
```yaml
message: string
  description: "Notification message"
```

**Optional Parameters**:
```yaml
namespace: string
  description: "Related namespace"

severity: string
  description: "Notification severity"
  enum: ["info", "warning", "critical"]
  default: "info"
```

**Example**:
```json
{
  "message": "Manual review required for complex issue",
  "namespace": "production",
  "severity": "warning"
}
```

---

## Validation Implementation

### Go Validation Example

```go
package validation

import (
    "fmt"
    "regexp"
)

type ParameterValidator struct {
    schemas map[string]ActionSchema
}

type ActionSchema struct {
    Required map[string]ParameterSpec
    Optional map[string]ParameterSpec
}

type ParameterSpec struct {
    Type        string
    Pattern     *regexp.Regexp
    Minimum     *int
    Maximum     *int
    Enum        []string
    Description string
}

func (v *ParameterValidator) Validate(actionType string, params map[string]interface{}) error {
    schema, ok := v.schemas[actionType]
    if !ok {
        return fmt.Errorf("unknown action type: %s", actionType)
    }

    // Check required parameters
    for paramName, spec := range schema.Required {
        value, exists := params[paramName]
        if !exists {
            return fmt.Errorf("missing required parameter: %s", paramName)
        }

        if err := v.validateParameter(paramName, value, spec); err != nil {
            return err
        }
    }

    // Validate optional parameters if present
    for paramName, value := range params {
        if spec, ok := schema.Optional[paramName]; ok {
            if err := v.validateParameter(paramName, value, spec); err != nil {
                return err
            }
        }
    }

    return nil
}
```

---

## Cross-References

- **Canonical Action Types**: `docs/design/CANONICAL_ACTION_TYPES.md`
- **Service Specifications**: All service specification documents
- **Executor Implementation**: `pkg/platform/executor/executor.go`

---

**Document Owner**: Platform Team
**Review Frequency**: When actions are added/modified
**Next Review Date**: 2026-01-07
