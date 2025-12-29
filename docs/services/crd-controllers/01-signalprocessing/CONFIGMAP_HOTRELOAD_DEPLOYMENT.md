# SignalProcessing ConfigMap Hot-Reload Deployment Guide

**BR**: [BR-SP-072](BUSINESS_REQUIREMENTS.md#br-sp-072-rego-hot-reload)
**DD**: [DD-INFRA-001](../../../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md)
**Status**: ✅ V1.0 REQUIRED
**Created**: 2025-12-13

---

## Overview

SignalProcessing controller supports hot-reload of Rego policies from ConfigMaps without pod restart. This enables operational agility for policy updates (priority, environment, custom labels) within ~60 seconds.

---

## Required ConfigMap

### Rego Policies ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-rego-policies
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: signalprocessing
    app.kubernetes.io/component: rego-policies
data:
  # BR-SP-070: Priority assignment policy
  priority.rego: |
    package priority

    # Default priority assignment based on severity and environment
    priority := "P0" {
        input.severity == "critical"
        input.environment == "production"
    }

    priority := "P1" {
        input.severity == "critical"
        input.environment != "production"
    }

    priority := "P1" {
        input.severity == "error"
        input.environment == "production"
    }

    priority := "P2" {
        input.severity == "error"
        input.environment != "production"
    }

    priority := "P2" {
        input.severity == "warning"
    }

    priority := "P3" {
        input.severity == "info"
    }

    # Default fallback
    priority := "P2"

  # BR-SP-051: Environment classification policy
  environment.rego: |
    package environment

    # Classify environment from namespace labels
    environment := "production" {
        input.namespace.labels["environment"] == "production"
    }

    environment := "production" {
        startswith(input.namespace.name, "prod-")
    }

    environment := "staging" {
        input.namespace.labels["environment"] == "staging"
    }

    environment := "staging" {
        startswith(input.namespace.name, "staging-")
    }

    environment := "development" {
        input.namespace.labels["environment"] == "development"
    }

    environment := "development" {
        startswith(input.namespace.name, "dev-")
    }

    # Default fallback
    environment := "unknown"

  # BR-SP-102: Custom labels extraction policy
  labels.rego: |
    package labels

    # Extract custom labels from pod annotations
    custom_labels := labels {
        labels := {k: v |
            some k
            v := input.pod.annotations[k]
            startswith(k, "kubernaut.ai/label-")
        }
    }

    # Default empty labels
    custom_labels := {}
```

---

## Deployment Configuration

### SignalProcessing Controller Deployment

Add the following to your SignalProcessing controller deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: signalprocessing-controller
  template:
    metadata:
      labels:
        app: signalprocessing-controller
    spec:
      serviceAccountName: signalprocessing-controller
      containers:
      - name: manager
        image: signalprocessing-controller:latest
        args:
        - --config=/etc/signalprocessing/config.yaml
        volumeMounts:
        # ConfigMap hot-reload volume mount (BR-SP-072)
        - name: rego-policies
          mountPath: /etc/kubernaut/policies
          readOnly: true
        # Controller config volume mount
        - name: controller-config
          mountPath: /etc/signalprocessing
          readOnly: true
      volumes:
      # BR-SP-072: Rego policies ConfigMap for hot-reload
      - name: rego-policies
        configMap:
          name: kubernaut-rego-policies
          optional: false
      # Controller configuration
      - name: controller-config
        configMap:
          name: signalprocessing-config
          optional: true
```

---

## Controller Configuration

### SignalProcessing Config ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-config
  namespace: kubernaut-system
data:
  config.yaml: |
    # Enrichment configuration
    enrichment:
      timeout: 2s
      cache_ttl: 5m

    # Rego policy paths (mounted from ConfigMap)
    rego:
      policy_dir: /etc/kubernaut/policies
      priority_policy: priority.rego
      environment_policy: environment.rego
      labels_policy: labels.rego

    # Audit configuration (ADR-032)
    audit:
      datastorage_url: http://datastorage-service:8080
      buffer_size: 1000
      flush_interval: 5s
```

---

## Hot-Reload Behavior

### How It Works

1. **ConfigMap Mount**: Kubernetes mounts the ConfigMap as files in `/etc/kubernaut/policies/`
2. **File Watch**: SignalProcessing uses `fsnotify` to watch for file changes
3. **Policy Reload**: When a file changes, the controller:
   - Re-compiles the Rego policy
   - Validates the new policy
   - If valid: Atomically swaps to new policy (with `sync.RWMutex`)
   - If invalid: Keeps old policy and logs error
4. **Audit Trail**: Logs policy version hash (SHA256) after reload

### Update Latency

| Action | Latency | Notes |
|--------|---------|-------|
| ConfigMap update | ~0s | `kubectl apply -f configmap.yaml` |
| Kubelet sync | ~60s | Default kubelet sync period |
| Policy reload | <1s | fsnotify + OPA compile |
| **Total** | **~60s** | P99 latency |

---

## Operational Procedures

### Update Priority Policy

```bash
# 1. Edit the ConfigMap
kubectl edit configmap kubernaut-rego-policies -n kubernaut-system

# 2. Wait for hot-reload (~60s)
kubectl logs -f -n kubernaut-system deployment/signalprocessing-controller | grep "policy reloaded"

# 3. Verify new policy is active
kubectl logs -n kubernaut-system deployment/signalprocessing-controller | grep "policy hash"
```

### Rollback Policy

```bash
# Apply previous ConfigMap version
kubectl apply -f configmap-v1.yaml

# Wait for hot-reload
kubectl logs -f -n kubernaut-system deployment/signalprocessing-controller | grep "policy reloaded"
```

### Validate Policy Before Apply

```bash
# Test Rego policy locally
opa test priority.rego

# Dry-run ConfigMap update
kubectl apply -f configmap.yaml --dry-run=client
```

---

## Monitoring

### Metrics

SignalProcessing exposes the following metrics for hot-reload:

| Metric | Type | Description |
|--------|------|-------------|
| `signalprocessing_policy_reload_total` | Counter | Total policy reloads |
| `signalprocessing_policy_reload_errors_total` | Counter | Failed policy reloads |
| `signalprocessing_policy_reload_duration_seconds` | Histogram | Policy reload latency |

### Logs

Watch for these log messages:

```
INFO  policy reloaded  {"policy": "priority.rego", "hash": "abc123...", "duration": "45ms"}
ERROR policy reload failed  {"policy": "priority.rego", "error": "rego_parse_error: ..."}
```

---

## Testing

### Integration Tests

Hot-reload behavior is tested in:
- `test/integration/signalprocessing/hot_reloader_test.go` (5 tests)
- `test/integration/signalprocessing/rego_integration_test.go` (15 tests)

### Manual Testing

```bash
# 1. Start integration test infrastructure
make test-integration-signalprocessing-setup

# 2. Run hot-reload tests
ginkgo --focus="Hot-Reload" ./test/integration/signalprocessing/

# 3. Cleanup
make test-integration-signalprocessing-cleanup
```

---

## Troubleshooting

### Policy Not Reloading

**Symptom**: ConfigMap updated but old policy still active

**Causes**:
1. Kubelet sync delay (wait 60s)
2. Invalid Rego syntax (check logs)
3. File permissions issue

**Resolution**:
```bash
# Check kubelet sync
kubectl get configmap kubernaut-rego-policies -n kubernaut-system -o yaml | grep resourceVersion

# Check controller logs
kubectl logs -n kubernaut-system deployment/signalprocessing-controller | grep -A5 "policy reload"

# Force pod restart (last resort)
kubectl rollout restart deployment/signalprocessing-controller -n kubernaut-system
```

### Invalid Policy Syntax

**Symptom**: `policy reload failed` in logs

**Resolution**:
```bash
# Validate Rego locally
opa test priority.rego

# Check OPA parse errors
kubectl logs -n kubernaut-system deployment/signalprocessing-controller | grep "rego_parse_error"

# Rollback to previous ConfigMap
kubectl rollout undo configmap/kubernaut-rego-policies -n kubernaut-system
```

---

## References

- **BR-SP-072**: [Rego Hot-Reload](BUSINESS_REQUIREMENTS.md#br-sp-072-rego-hot-reload)
- **DD-INFRA-001**: [ConfigMap Hot-Reload Pattern](../../../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md)
- **Shared Package**: `pkg/shared/hotreload/FileWatcher`
- **HolmesGPT API Example**: `docs/services/stateless/holmesgpt-api/implementation/IMPLEMENTATION_PLAN_HOTRELOAD.md`

---

**Last Updated**: 2025-12-13
**Status**: ✅ V1.0 Required - Infrastructure exists, tests enabled


