# SignalProcessing Service - Deployment Guide

**Version**: 1.0
**Last Updated**: December 9, 2025
**Related**: [IMPLEMENTATION_PLAN_V1.31](IMPLEMENTATION_PLAN_V1.31.md)

---

## Overview

This document describes how to deploy the SignalProcessing CRD controller to a Kubernetes cluster.

---

## Prerequisites

### Cluster Requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| Kubernetes | 1.27+ | 1.28+ |
| Nodes | 1 | 3 (HA) |
| CPU | 100m | 500m |
| Memory | 128Mi | 256Mi |

### Required Components

- **CRD Registration**: Must be installed before controller deployment
- **RBAC**: ClusterRole and bindings for K8s API access
- **ConfigMap**: Rego policies and configuration

---

## Quick Deploy

```bash
# Install CRDs
kubectl apply -f config/crd/bases/signalprocessing.kubernaut.ai_signalprocessings.yaml

# Install RBAC
kubectl apply -f config/rbac/signalprocessing/

# Install ConfigMaps
kubectl apply -f config/configmaps/signalprocessing/

# Install Controller
kubectl apply -f config/manager/signalprocessing/
```

---

## Step-by-Step Deployment

### 1. Create Namespace

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: signalprocessing
```

### 2. Install CRD

```yaml
# config/crd/bases/signalprocessing.kubernaut.ai_signalprocessings.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: signalprocessings.signalprocessing.kubernaut.ai
spec:
  group: signalprocessing.kubernaut.ai
  names:
    kind: SignalProcessing
    listKind: SignalProcessingList
    plural: signalprocessings
    singular: signalprocessing
    shortNames:
      - sp
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      # ... (see api/signalprocessing/v1alpha1/ for full schema)
```

### 3. Create RBAC

#### ServiceAccount

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
```

#### ClusterRole

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: signalprocessing-controller
rules:
  # SignalProcessing CRD access
  - apiGroups: ["signalprocessing.kubernaut.ai"]
    resources: ["signalprocessings"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["signalprocessing.kubernaut.ai"]
    resources: ["signalprocessings/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: ["signalprocessing.kubernaut.ai"]
    resources: ["signalprocessings/finalizers"]
    verbs: ["update"]

  # K8s Context Enrichment (BR-SP-001)
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch"]

  # OwnerChain Traversal (BR-SP-100)
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]

  # DetectedLabels (BR-SP-101)
  - apiGroups: ["policy"]
    resources: ["poddisruptionbudgets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["autoscaling"]
    resources: ["horizontalpodautoscalers"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list", "watch"]

  # ConfigMap Hot-Reload (BR-SP-072)
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch"]

  # Events
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]

  # Leader Election
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

#### ClusterRoleBinding

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: signalprocessing-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: signalprocessing-controller
subjects:
  - kind: ServiceAccount
    name: signalprocessing-controller
    namespace: kubernaut-system
```

### 4. Create ConfigMaps

#### Controller Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-config
  namespace: kubernaut-system
data:
  config.yaml: |
    enrichment:
      timeout: 2s
      cacheEnabled: true
      cacheTTL: 5m
    classification:
      environmentLabel: kubernaut.ai/environment
      timeout: 1s
    priority:
      fallbackEnabled: true
      defaultPriority: P2
    metrics:
      port: 9090
    health:
      port: 8081
    leaderElection:
      enabled: true
      leaseDuration: 15s
      renewDeadline: 10s
      retryPeriod: 2s
```

#### Rego Policies

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-rego-policies
  namespace: kubernaut-system
data:
  priority.rego: |
    package signalprocessing.priority

    import rego.v1

    # Default priority
    default priority := "P2"

    # P0: Production + Critical
    priority := "P0" if {
      input.environment == "production"
      input.signal.severity == "critical"
    }

    # P1: Production + Warning OR Staging + Critical
    priority := "P1" if {
      input.environment == "production"
      input.signal.severity == "warning"
    }
    priority := "P1" if {
      input.environment == "staging"
      input.signal.severity == "critical"
    }

    # P3: Development/Test
    priority := "P3" if {
      input.environment in {"development", "test"}
    }

  custom_labels.rego: |
    package signalprocessing.labels

    import rego.v1

    # Extract team from namespace annotations
    labels["team"] := [concat("name=", ns_team)] if {
      ns_team := input.namespace.annotations["kubernaut.ai/team"]
    }

    # Extract cost center from deployment labels
    labels["cost_center"] := [input.deployment.labels["cost-center"]] if {
      input.deployment.labels["cost-center"]
    }
```

#### Environment Mapping (Fallback)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-environment-config
  namespace: kubernaut-system
data:
  mapping.yaml: |
    patterns:
      - pattern: "^prod-.*"
        environment: production
      - pattern: "^staging-.*"
        environment: staging
      - pattern: "^dev-.*"
        environment: development
      - pattern: "^test-.*"
        environment: test
    defaults:
      environment: unknown
```

### 5. Deploy Controller

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: signalprocessing-controller
    app.kubernetes.io/component: controller
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: signalprocessing-controller
  template:
    metadata:
      labels:
        app.kubernetes.io/name: signalprocessing-controller
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: signalprocessing-controller
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: controller
          image: ghcr.io/jordigilh/kubernaut/signalprocessing:latest
          imagePullPolicy: IfNotPresent
          args:
            - --leader-elect=true
            - --metrics-bind-address=:9090
            - --health-probe-bind-address=:8081
          ports:
            - name: metrics
              containerPort: 9090
              protocol: TCP
            - name: health
              containerPort: 8081
              protocol: TCP
          env:
            - name: LOG_LEVEL
              value: info
            - name: CONFIG_PATH
              value: /etc/kubernaut/config/config.yaml
          volumeMounts:
            - name: config
              mountPath: /etc/kubernaut/config
              readOnly: true
            - name: rego-policies
              mountPath: /etc/kubernaut/policies
              readOnly: true
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /health
              port: 8081
            initialDelaySeconds: 15
            periodSeconds: 20
          readinessProbe:
            httpGet:
              path: /ready
              port: 8081
            initialDelaySeconds: 5
            periodSeconds: 10
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
      volumes:
        - name: config
          configMap:
            name: signalprocessing-config
        - name: rego-policies
          configMap:
            name: kubernaut-rego-policies
      terminationGracePeriodSeconds: 30
```

### 6. Create Service (for metrics scraping)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: signalprocessing-controller
spec:
  selector:
    app.kubernetes.io/name: signalprocessing-controller
  ports:
    - name: metrics
      port: 9090
      targetPort: 9090
    - name: health
      port: 8081
      targetPort: 8081
```

---

## Verification

### Check CRD Installation

```bash
kubectl get crd signalprocessings.signalprocessing.kubernaut.ai
# Expected: NAME should be listed

kubectl api-resources | grep signalprocessing
# Expected: signalprocessings   sp   signalprocessing.kubernaut.ai/v1alpha1
```

### Check Controller Running

```bash
kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=signalprocessing-controller
# Expected: 1/1 Running

kubectl logs -n kubernaut-system deploy/signalprocessing-controller --tail=20
# Expected: "Starting controller" messages, no errors
```

### Test Signal Processing

```bash
# Create test signal
kubectl apply -f - <<EOF
apiVersion: signalprocessing.kubernaut.ai/v1alpha1
kind: SignalProcessing
metadata:
  name: test-signal
  namespace: default
spec:
  remediationRequestRef:
    name: test-remediation
    namespace: default
  signal:
    fingerprint: "test123456789012345678901234567890123456789012345"
    name: "TestSignal"
    severity: "warning"
    type: "prometheus"
    targetType: "kubernetes"
    targetResource:
      kind: "Pod"
      name: "test-pod"
      namespace: "default"
EOF

# Check status
kubectl get sp test-signal -o yaml
# Expected: status.phase should progress through Pending → Enriching → Classifying → Completed

# Cleanup
kubectl delete sp test-signal
```

---

## High Availability

### Multi-Replica Deployment

```yaml
spec:
  replicas: 3  # Only 1 active (leader), 2 standby
```

Leader election ensures only one controller processes CRDs at a time.

### Pod Disruption Budget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: signalprocessing-controller
```

---

## Upgrades

### Rolling Update

```bash
# Update image
kubectl set image deployment/signalprocessing-controller \
  -n kubernaut-system \
  controller=ghcr.io/jordigilh/kubernaut/signalprocessing:v1.1.0

# Monitor rollout
kubectl rollout status deployment/signalprocessing-controller -n kubernaut-system
```

### CRD Updates

```bash
# CRD updates must be applied before controller update
kubectl apply -f config/crd/bases/signalprocessing.kubernaut.ai_signalprocessings.yaml

# Then update controller
kubectl apply -f config/manager/signalprocessing/
```

---

## Uninstallation

```bash
# Remove controller
kubectl delete deployment signalprocessing-controller -n kubernaut-system

# Remove RBAC
kubectl delete clusterrolebinding signalprocessing-controller
kubectl delete clusterrole signalprocessing-controller
kubectl delete serviceaccount signalprocessing-controller -n kubernaut-system

# Remove ConfigMaps
kubectl delete configmap signalprocessing-config -n kubernaut-system
kubectl delete configmap kubernaut-rego-policies -n kubernaut-system

# Remove CRD (WARNING: Deletes all SignalProcessing resources!)
kubectl delete crd signalprocessings.signalprocessing.kubernaut.ai
```

---

## Network Policies

### Restrict Controller Egress

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: signalprocessing-controller
  policyTypes:
    - Egress
  egress:
    # Allow DNS
    - to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - protocol: UDP
          port: 53
    # Allow K8s API
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - protocol: TCP
          port: 443
```

---

## References

- [BUILD.md](BUILD.md) - Build instructions
- [OPERATIONS.md](OPERATIONS.md) - Operational procedures
- [Security Configuration](security-configuration.md) - Detailed security setup
- [DD-006: Controller Scaffolding](../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md)





