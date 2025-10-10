# Dynamic Toolset Service - ConfigMap Reconciliation Deep Dive

**Version**: v1.0
**Last Updated**: October 10, 2025
**Status**: ✅ Design Complete

---

## Overview

The ConfigMap Reconciliation Controller ensures that the `kubernaut-toolset-config` ConfigMap always matches the desired state based on discovered services, while preserving admin-configured overrides.

---

## Reconciliation Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│           ConfigMap Reconciler Controller                   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Reconciliation Loop (30-second interval)            │  │
│  │  - Watch ConfigMap for changes                       │  │
│  │  - Detect drift from desired state                   │  │
│  │  - Reconcile back to desired state                   │  │
│  │  - Preserve admin overrides                          │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Drift Detection Engine                              │  │
│  │  - Compare current vs desired state                  │  │
│  │  - Identify missing keys                             │  │
│  │  - Identify modified values                          │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Override Preservation                               │  │
│  │  - Extract admin overrides.yaml                      │  │
│  │  - Merge with desired state                          │  │
│  │  - Write back complete ConfigMap                     │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                │
                │ Reads/Writes ConfigMap
                ▼
        ┌───────────────────┐
        │ Kubernetes API    │
        │ (ConfigMaps)      │
        └───────────────────┘
```

---

## Reconciliation Flow

### Step-by-Step Process

#### 1. **Watch ConfigMap**
```go
currentCM, err := k8sClient.CoreV1().ConfigMaps("kubernaut-system").
    Get(ctx, "kubernaut-toolset-config", metav1.GetOptions{})
```

**What**: Retrieve current ConfigMap from Kubernetes
**Why**: Need current state to detect drift
**Performance**: Single K8s API call (<50ms typical)

---

#### 2. **Detect Drift**
```go
hasDrift, driftKeys := r.detectDrift(currentCM, desiredCM)
```

**What**: Compare current vs desired ConfigMap data
**Why**: Only reconcile if changes detected (avoid unnecessary writes)
**Performance**: In-memory map comparison (<1ms)

**Drift Types**:
- **Missing Key**: Desired key not in current ConfigMap
- **Modified Value**: Key exists but value changed
- **Extra Key**: Current has key not in desired (admin override or old service removed)

---

#### 3. **Merge Admin Overrides**
```go
merged := r.mergeOverrides(currentCM, desiredCM)
```

**What**: Preserve `overrides.yaml` from current ConfigMap
**Why**: Admin manual edits should not be overwritten
**Performance**: In-memory map merge (<1ms)

---

#### 4. **Write Back ConfigMap**
```go
_, err := k8sClient.CoreV1().ConfigMaps("kubernaut-system").
    Update(ctx, merged, metav1.UpdateOptions{})
```

**What**: Update ConfigMap in Kubernetes
**Why**: Restore desired state
**Performance**: Single K8s API call (<100ms)

---

## Drift Detection Algorithm

### Implementation

```go
import (
    "fmt"

    corev1 "k8s.io/api/core/v1"
)

func (r *ConfigMapReconciler) detectDrift(current, desired *corev1.ConfigMap) (bool, []string) {
    var driftKeys []string

    // Check for missing keys in current
    for key := range desired.Data {
        if key == "overrides.yaml" {
            continue // Skip overrides, they're admin-managed
        }

        currentValue, ok := current.Data[key]
        if !ok {
            driftKeys = append(driftKeys, fmt.Sprintf("missing:%s", key))
            continue
        }

        // Check for modified values
        if currentValue != desired.Data[key] {
            driftKeys = append(driftKeys, fmt.Sprintf("modified:%s", key))
        }
    }

    // Check for extra keys in current (deleted services)
    for key := range current.Data {
        if key == "overrides.yaml" {
            continue
        }
        if _, ok := desired.Data[key]; !ok {
            driftKeys = append(driftKeys, fmt.Sprintf("extra:%s", key))
        }
    }

    return len(driftKeys) > 0, driftKeys
}
```

### Drift Examples

#### Missing Key Drift

**Scenario**: New service discovered, toolset not in ConfigMap

**Current ConfigMap**:
```yaml
data:
  kubernetes-toolset.yaml: |
    toolset: kubernetes
    enabled: true
```

**Desired ConfigMap**:
```yaml
data:
  kubernetes-toolset.yaml: |
    toolset: kubernetes
    enabled: true
  prometheus-toolset.yaml: |
    toolset: prometheus
    enabled: true
    config:
      url: "http://prometheus-server.monitoring:9090"
```

**Drift Detection**: `missing:prometheus-toolset.yaml`

**Action**: Update ConfigMap to add Prometheus toolset

---

#### Modified Value Drift

**Scenario**: Service endpoint changed (e.g., namespace renamed)

**Current ConfigMap**:
```yaml
data:
  prometheus-toolset.yaml: |
    toolset: prometheus
    config:
      url: "http://prometheus-server.old-namespace:9090"
```

**Desired ConfigMap**:
```yaml
data:
  prometheus-toolset.yaml: |
    toolset: prometheus
    config:
      url: "http://prometheus-server.new-namespace:9090"
```

**Drift Detection**: `modified:prometheus-toolset.yaml`

**Action**: Update ConfigMap to new endpoint

---

#### Extra Key Drift

**Scenario**: Service removed from cluster, toolset still in ConfigMap

**Current ConfigMap**:
```yaml
data:
  kubernetes-toolset.yaml: |
    ...
  prometheus-toolset.yaml: |
    ...
  grafana-toolset.yaml: |
    ...  # Grafana service no longer exists
```

**Desired ConfigMap**:
```yaml
data:
  kubernetes-toolset.yaml: |
    ...
  prometheus-toolset.yaml: |
    ...
```

**Drift Detection**: `extra:grafana-toolset.yaml`

**Action**: Remove Grafana toolset from ConfigMap

---

## Override Preservation

### Admin Override Pattern

**Purpose**: Allow admins to manually configure custom toolsets that won't be overwritten by reconciliation

**Mechanism**: Special `overrides.yaml` key in ConfigMap

### Override Section Example

```yaml
# ConfigMap: kubernaut-toolset-config
data:
  kubernetes-toolset.yaml: |
    # Auto-generated by Dynamic Toolset Service
    toolset: kubernetes
    enabled: true

  prometheus-toolset.yaml: |
    # Auto-generated by Dynamic Toolset Service
    toolset: prometheus
    enabled: true
    config:
      url: "http://prometheus-server.monitoring:9090"

  overrides.yaml: |
    # Admin-managed custom toolsets
    # This section is NEVER overwritten by reconciliation

    custom-elasticsearch:
      enabled: true
      config:
        url: "http://elasticsearch.logging:9200"
        index: "logs-*"

    custom-datadog:
      enabled: false  # Temporarily disable
      config:
        api_key: "${DATADOG_API_KEY}"
```

### Merge Algorithm

```go
import (
    corev1 "k8s.io/api/core/v1"
    "go.uber.org/zap"
)

func (r *ConfigMapReconciler) mergeOverrides(current, desired *corev1.ConfigMap) *corev1.ConfigMap {
    merged := desired.DeepCopy()

    // Preserve admin overrides from current ConfigMap
    if overrides, ok := current.Data["overrides.yaml"]; ok {
        merged.Data["overrides.yaml"] = overrides
        r.logger.Debug("Preserved admin overrides")
    }

    return merged
}
```

**Result**: Auto-generated toolsets are reconciled, admin overrides preserved

---

## Reconciliation Loop

### Timing Configuration

**Reconciliation Interval**: 30 seconds
**Rationale**: Quick detection of manual ConfigMap edits or deletions

**Reconciliation Duration**: < 5 seconds (p95)
**Breakdown**:
- Get ConfigMap: 50ms
- Detect drift: 1ms
- Merge overrides: 1ms
- Update ConfigMap: 100ms

### Loop Implementation

```go
import (
    "context"
    "time"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "go.uber.org/zap"
)

func (r *ConfigMapReconciler) Start(ctx context.Context, desiredState *corev1.ConfigMap) error {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    // Run reconciliation immediately on start
    if err := r.Reconcile(ctx, desiredState); err != nil {
        logger.Error("Initial reconciliation failed", zap.Error(err))
        return err
    }

    for {
        select {
        case <-ticker.C:
            if err := r.Reconcile(ctx, desiredState); err != nil {
                logger.Error("Reconciliation failed", zap.Error(err))
                // Continue running even if reconciliation fails
            }
        case <-r.stopCh:
            logger.Info("Stopping reconciliation loop")
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

---

## ConfigMap Deletion Recovery

### Scenario: ConfigMap Accidentally Deleted

**Problem**: Admin runs `kubectl delete configmap kubernaut-toolset-config`

**Detection**: ConfigMap not found error

```go
currentCM, err := k8sClient.CoreV1().ConfigMaps("kubernaut-system").
    Get(ctx, "kubernaut-toolset-config", metav1.GetOptions{})

if errors.IsNotFound(err) {
    logger.Warn("ConfigMap not found, recreating")
    return r.createConfigMap(ctx, desiredState)
}
```

**Recovery**: Recreate ConfigMap from desired state

```go
func (r *ConfigMapReconciler) createConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
    logger.Info("Creating ConfigMap", zap.String("configmap", cm.Name))

    _, err := k8sClient.CoreV1().ConfigMaps(r.namespace).
        Create(ctx, cm, metav1.CreateOptions{})

    if err != nil {
        return fmt.Errorf("failed to create ConfigMap: %w", err)
    }

    logger.Info("ConfigMap created successfully")
    return nil
}
```

**Result**: ConfigMap recreated in 30 seconds (next reconciliation loop)

---

## Error Handling

### ConfigMap Update Conflicts

**Scenario**: Concurrent writes to ConfigMap (race condition)

**Error**: `the object has been modified; please apply your changes to the latest version`

**Strategy**: Retry with exponential backoff

```go
func (r *ConfigMapReconciler) updateConfigMapWithRetry(ctx context.Context, cm *corev1.ConfigMap) error {
    backoff := time.Second
    maxRetries := 3

    for i := 0; i < maxRetries; i++ {
        _, err := k8sClient.CoreV1().ConfigMaps(r.namespace).
            Update(ctx, cm, metav1.UpdateOptions{})

        if err == nil {
            return nil
        }

        if !errors.IsConflict(err) {
            return err
        }

        logger.Warn("ConfigMap update conflict, retrying",
            zap.Int("attempt", i+1),
            zap.Duration("backoff", backoff))

        time.Sleep(backoff)
        backoff *= 2
    }

    return fmt.Errorf("failed to update ConfigMap after %d retries", maxRetries)
}
```

---

## Monitoring & Metrics

### Reconciliation Metrics

```go
// Reconciliation attempts
configmapReconcileTotal.WithLabelValues("update", "success").Inc()

// Drift detection events
configmapDriftDetectedTotal.WithLabelValues("missing_key").Inc()

// Reconciliation duration
reconciliationDuration.Observe(duration.Seconds())
```

### Logging

```go
logger.Info("ConfigMap reconciliation",
    zap.String("operation", "update"),
    zap.String("status", "success"),
    zap.Strings("drift_keys", driftKeys),
    zap.Duration("duration", duration))
```

---

## Integration with Service Discovery

### Coordinated Updates

```
┌─────────────────────────────────────────────────────────┐
│ Every 5 minutes: Service Discovery                     │
└─────────────────────────────────────────────────────────┘
         │
         │ Discovers new Prometheus service
         ▼
┌─────────────────────────────────────────────────────────┐
│ Generate Desired ConfigMap                              │
│ - Kubernetes toolset (always)                           │
│ - Prometheus toolset (newly discovered)                 │
│ - Grafana toolset (existing)                            │
└─────────────────────────────────────────────────────────┘
         │
         │ Update desired state
         ▼
┌─────────────────────────────────────────────────────────┐
│ Every 30 seconds: Reconciliation                        │
│ - Detect drift (missing prometheus-toolset.yaml)       │
│ - Merge overrides                                       │
│ - Update ConfigMap                                      │
└─────────────────────────────────────────────────────────┘
         │
         │ ConfigMap updated
         ▼
┌─────────────────────────────────────────────────────────┐
│ HolmesGPT API polls ConfigMap (every 60 seconds)       │
│ - Detects new prometheus-toolset.yaml                  │
│ - Reloads toolsets                                      │
│ - Prometheus now available for investigations           │
└─────────────────────────────────────────────────────────┘
```

**Total Latency**: 5-6 minutes (5 min discovery + 30s reconciliation + 60s HolmesGPT poll)

---

## Advanced Scenarios

### Scenario 1: Conflicting Admin Override

**Problem**: Admin override contradicts auto-discovery

**Example**:
```yaml
# Auto-generated (desired)
prometheus-toolset.yaml: |
  url: "http://prometheus-server.monitoring:9090"

# Admin override
overrides.yaml: |
  prometheus:
    url: "http://prometheus-server.custom-namespace:9090"  # Different!
```

**Resolution Strategy**: Admin override wins (in `overrides.yaml` section)

**Implementation**:
- Auto-generated toolsets in main section
- Admin overrides in `overrides.yaml` section
- HolmesGPT API merges both, with overrides taking precedence

---

### Scenario 2: Large ConfigMap (approaching 1MB limit)

**Problem**: Many discovered services may exceed ConfigMap 1MB limit

**Detection**:
```go
func (r *ConfigMapReconciler) validateConfigMapSize(cm *corev1.ConfigMap) error {
    size := 0
    for _, value := range cm.Data {
        size += len(value)
    }

    if size > 900000 { // 900KB (safety margin)
        return fmt.Errorf("ConfigMap size %d bytes exceeds safe limit", size)
    }

    return nil
}
```

**Mitigation** (V2):
- Split ConfigMap into multiple: `kubernaut-toolset-config-1`, `kubernaut-toolset-config-2`
- HolmesGPT API reads all ConfigMaps

---

**Document Status**: ✅ ConfigMap Reconciliation Deep Dive Complete
**Last Updated**: October 10, 2025
**Related**: [implementation.md](./implementation.md), [service-discovery.md](./service-discovery.md)

