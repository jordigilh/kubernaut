# CRD Controllers - Production Deployment Guide

**Version**: 1.0
**Date**: 2025-10-13
**Applies To**: Remediation Processor, Workflow Execution, Kubernetes Executor (DEPRECATED - ADR-025)
**Target Environment**: Production Kubernetes clusters
**High Availability**: Active-Passive with leader election

---

## 📋 Table of Contents

1. [Deployment Overview](#deployment-overview)
2. [Prerequisites](#prerequisites)
3. [Namespace Strategy](#namespace-strategy)
4. [Deployment Manifests](#deployment-manifests)
5. [Configuration Management](#configuration-management)
6. [Secret Management](#secret-management)
7. [RBAC Configuration](#rbac-configuration)
8. [High Availability Setup](#high-availability-setup)
9. [Resource Limits & Requests](#resource-limits--requests)
10. [Monitoring & Alerting](#monitoring--alerting)
11. [Network Policies](#network-policies)
12. [Deployment Validation](#deployment-validation)

---

## 🎯 Deployment Overview

### Service Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ kubernaut-system Namespace                                   │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Remediation Processor Controller                       │ │
│  │ - Replicas: 2 (HA)                                     │ │
│  │ - Leader Election: Yes                                 │ │
│  │ - Port 9090: Metrics                                   │ │
│  │ - Port 8081: Health Probes                             │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Workflow Execution Controller                          │ │
│  │ - Replicas: 2 (HA)                                     │ │
│  │ - Leader Election: Yes                                 │ │
│  │ - Port 9090: Metrics                                   │ │
│  │ - Port 8081: Health Probes                             │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Kubernetes Executor Controller (DEPRECATED - ADR-025)  │ │
│  │ - Replicas: 2 (HA)                                     │ │
│  │ - Leader Election: Yes                                 │ │
│  │ - Port 9090: Metrics                                   │ │
│  │ - Port 8081: Health Probes                             │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### Deployment Strategy

- **Namespace**: `kubernaut-system` (dedicated namespace for all controllers)
- **High Availability**: 2 replicas per controller with leader election
- **Update Strategy**: RollingUpdate with maxUnavailable=1
- **Resource Isolation**: NetworkPolicies for security
- **Configuration**: ConfigMaps for settings, Secrets for credentials
- **Monitoring**: Prometheus metrics + Grafana dashboards

---

## 📋 Prerequisites

### Cluster Requirements

- **Kubernetes Version**: >= 1.25
- **CRD Support**: CustomResourceDefinitions API enabled
- **Leader Election**: Kubernetes lease API available
- **Metrics**: Prometheus operator installed (optional but recommended)
- **Storage**: PostgreSQL for Data Storage Service (Remediation Processor dependency)

### Required CRDs

Ensure all CRDs are installed before deploying controllers:

```bash
# Install CRDs
kubectl apply -f config/crd/bases/remediationprocessing.kubernaut.ai_remediationprocessings.yaml
kubectl apply -f config/crd/bases/kubernaut.ai_workflowexecutions.yaml
kubectl apply -f config/crd/bases/kubernetesexecution.kubernaut.ai_kubernetesexecutions.yaml  # DEPRECATED - ADR-025

# Verify CRDs
kubectl get crds | grep kubernaut.ai
```

---

## 🏗️ Namespace Strategy

### Create kubernaut-system Namespace

**File**: `deploy/crd-controllers/namespace.yaml`

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: crd-controllers
    app.kubernetes.io/part-of: kubernaut-platform
```

**Apply**:
```bash
kubectl apply -f deploy/crd-controllers/namespace.yaml
```

---

## 📦 Deployment Manifests

### Remediation Processor Deployment

**File**: `deploy/crd-controllers/remediation-processor-deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: remediation-processor-controller
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: remediation-processor
    app.kubernetes.io/component: controller
    app.kubernetes.io/part-of: kubernaut
spec:
  replicas: 2  # HA: 2 replicas with leader election
  selector:
    matchLabels:
      app.kubernetes.io/name: remediation-processor
      app.kubernetes.io/component: controller
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: remediation-processor
        app.kubernetes.io/component: controller
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: remediation-processor-sa
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        fsGroup: 65532
      containers:
      - name: controller
        image: quay.io/jordigilh/remediation-processor-controller:v1.0.0
        imagePullPolicy: IfNotPresent
        command:
        - /manager
        args:
        - --metrics-bind-address=:9090
        - --health-probe-bind-address=:8081
        - --leader-elect=true
        - --storage-connection-string=$(STORAGE_CONNECTION_STRING)
        env:
        - name: STORAGE_CONNECTION_STRING
          valueFrom:
            secretKeyRef:
              name: remediation-processor-secrets
              key: storage-connection-string
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        ports:
        - name: metrics
          containerPort: 9090
          protocol: TCP
        - name: health
          containerPort: 8081
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
          initialDelaySeconds: 15
          periodSeconds: 20
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        resources:
          requests:
            cpu: 200m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        volumeMounts:
        - name: config
          mountPath: /etc/config
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: remediation-processor-config
      terminationGracePeriodSeconds: 10
      nodeSelector:
        kubernetes.io/os: linux
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app.kubernetes.io/name
                  operator: In
                  values:
                  - remediation-processor
              topologyKey: kubernetes.io/hostname
```

### Workflow Execution Deployment

**File**: `deploy/crd-controllers/workflow-execution-deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: workflow-execution-controller
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: workflow-execution
    app.kubernetes.io/component: controller
    app.kubernetes.io/part-of: kubernaut
spec:
  replicas: 2  # HA: 2 replicas with leader election
  selector:
    matchLabels:
      app.kubernetes.io/name: workflow-execution
      app.kubernetes.io/component: controller
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: workflow-execution
        app.kubernetes.io/component: controller
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: workflow-execution-sa
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        fsGroup: 65532
      containers:
      - name: controller
        image: quay.io/jordigilh/workflow-execution-controller:v1.0.0
        imagePullPolicy: IfNotPresent
        command:
        - /manager
        args:
        - --metrics-bind-address=:9090
        - --health-probe-bind-address=:8081
        - --leader-elect=true
        env:
        - name: MAX_CONCURRENT_STEPS
          valueFrom:
            configMapKeyRef:
              name: workflow-execution-config
              key: max-concurrent-steps
        - name: WORKFLOW_TIMEOUT_DEFAULT
          valueFrom:
            configMapKeyRef:
              name: workflow-execution-config
              key: workflow-timeout-default
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        ports:
        - name: metrics
          containerPort: 9090
          protocol: TCP
        - name: health
          containerPort: 8081
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
          initialDelaySeconds: 15
          periodSeconds: 20
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        resources:
          requests:
            cpu: 300m
            memory: 384Mi
          limits:
            cpu: 700m
            memory: 768Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        volumeMounts:
        - name: config
          mountPath: /etc/config
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: workflow-execution-config
      terminationGracePeriodSeconds: 10
      nodeSelector:
        kubernetes.io/os: linux
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app.kubernetes.io/name
                  operator: In
                  values:
                  - workflow-execution
              topologyKey: kubernetes.io/hostname
```

### Kubernetes Executor Deployment (DEPRECATED - ADR-025)

**File**: `deploy/crd-controllers/kubernetes-executor-deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetes-executor-controller
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernetes-executor
    app.kubernetes.io/component: controller
    app.kubernetes.io/part-of: kubernaut
spec:
  replicas: 2  # HA: 2 replicas with leader election
  selector:
    matchLabels:
      app.kubernetes.io/name: kubernetes-executor
      app.kubernetes.io/component: controller
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: kubernetes-executor
        app.kubernetes.io/component: controller
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: kubernetes-executor-sa
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        fsGroup: 65532
      containers:
      - name: controller
        image: quay.io/jordigilh/kubernetes-executor-controller:v1.0.0
        imagePullPolicy: IfNotPresent
        command:
        - /manager
        args:
        - --metrics-bind-address=:9090
        - --health-probe-bind-address=:8081
        - --leader-elect=true
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        ports:
        - name: metrics
          containerPort: 9090
          protocol: TCP
        - name: health
          containerPort: 8081
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
          initialDelaySeconds: 15
          periodSeconds: 20
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        resources:
          requests:
            cpu: 150m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 256Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        volumeMounts:
        - name: config
          mountPath: /etc/config
          readOnly: true
        - name: rego-policies
          mountPath: /etc/rego
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: kubernetes-executor-config
      - name: rego-policies
        configMap:
          name: kubernetes-executor-policies
      terminationGracePeriodSeconds: 10
      nodeSelector:
        kubernetes.io/os: linux
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app.kubernetes.io/name
                  operator: In
                  values:
                  - kubernetes-executor
              topologyKey: kubernetes.io/hostname
```

---

## ⚙️ Configuration Management

### Remediation Processor ConfigMap

**File**: `deploy/crd-controllers/remediation-processor-configmap.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: remediation-processor-config
  namespace: kubernaut-system
data:
  # Classification thresholds
  ai-required-threshold: "0.7"
  automated-threshold: "0.8"

  # Similarity search configuration
  similarity-threshold: "0.7"
  max-similar-results: "10"

  # Deduplication configuration
  fingerprint-ttl: "24h"

  # Performance tuning
  enrichment-timeout: "10s"
  classification-timeout: "5s"
```

### Workflow Execution ConfigMap

**File**: `deploy/crd-controllers/workflow-execution-configmap.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: workflow-execution-config
  namespace: kubernaut-system
data:
  # Parallel execution configuration
  max-concurrent-steps: "5"

  # Timeout configuration
  workflow-timeout-default: "30m"
  step-timeout-default: "10m"

  # Rollback configuration
  rollback-enabled-by-default: "true"
  rollback-timeout: "15m"

  # Dependency resolution
  max-dependency-depth: "10"
  circular-dependency-detection: "true"
```

### Kubernetes Executor ConfigMap (DEPRECATED - ADR-025)

**File**: `deploy/crd-controllers/kubernetes-executor-configmap.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernetes-executor-config
  namespace: kubernaut-system
data:
  # Action timeout defaults (can be overridden per action)
  scale-deployment-timeout: "300s"
  restart-deployment-timeout: "600s"
  delete-pod-timeout: "120s"
  patch-configmap-timeout: "60s"
  patch-secret-timeout: "60s"
  update-image-timeout: "600s"
  cordon-node-timeout: "30s"
  drain-node-timeout: "900s"
  uncordon-node-timeout: "30s"
  rollout-status-timeout: "60s"

  # Job cleanup
  job-ttl-seconds-after-finished: "300"

  # RBAC configuration
  rbac-auto-create: "true"
  rbac-cleanup-enabled: "true"
```

### Kubernetes Executor Rego Policies ConfigMap (DEPRECATED - ADR-025)

**File**: `deploy/crd-controllers/kubernetes-executor-policies-configmap.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernetes-executor-policies
  namespace: kubernaut-system
data:
  production-safety.rego: |
    package kubernetesexecution.safety

    # Deny scaling below 1 replica in production
    deny[msg] {
      input.action == "ScaleDeployment"
      input.parameters.replicas < 1
      input.environment == "production"
      msg = "Cannot scale below 1 replica in production"
    }

    # Deny node drain during business hours (9am-5pm UTC)
    deny[msg] {
      input.action == "DrainNode"
      hour := time.clock([time.now_ns()])[0]
      hour >= 9
      hour < 17
      input.environment == "production"
      msg = "Node drain operations not allowed during business hours (9am-5pm UTC)"
    }

    # Deny pod deletion for critical workloads without approval
    deny[msg] {
      input.action == "DeletePod"
      input.target.labels["criticality"] == "high"
      not input.approved
      msg = "High criticality pods require manual approval for deletion"
    }

  namespace-restrictions.rego: |
    package kubernetesexecution.namespace

    # Deny actions on system namespaces
    deny[msg] {
      input.target.namespace == "kube-system"
      msg = "Actions on kube-system namespace are not allowed"
    }

    deny[msg] {
      input.target.namespace == "kube-public"
      msg = "Actions on kube-public namespace are not allowed"
    }
```

---

## 🔐 Secret Management

### Remediation Processor Secrets

**File**: `deploy/crd-controllers/remediation-processor-secrets.yaml`

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: remediation-processor-secrets
  namespace: kubernaut-system
type: Opaque
stringData:
  storage-connection-string: "host=postgresql.kubernaut-system.svc.cluster.local port=5432 user=remediationprocessor password=CHANGEME dbname=kubernaut sslmode=require"
```

**⚠️ Security Best Practices**:
- Use external secret management (HashiCorp Vault, AWS Secrets Manager, etc.)
- Rotate secrets regularly (every 90 days minimum)
- Use Sealed Secrets or SOPS for GitOps workflows
- Never commit plaintext secrets to Git

**Vault Integration Example**:
```yaml
apiVersion: secrets-store.csi.x-k8s.io/v1
kind: SecretProviderClass
metadata:
  name: remediation-processor-vault-secrets
spec:
  provider: vault
  parameters:
    vaultAddress: "https://vault.example.com"
    roleName: "remediation-processor"
    objects: |
      - objectName: "storage-connection-string"
        secretPath: "secret/data/kubernaut/remediation-processor"
        secretKey: "storage-connection-string"
```

---

## 🔒 RBAC Configuration

### Service Accounts

**File**: `deploy/crd-controllers/serviceaccounts.yaml`

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: remediation-processor-sa
  namespace: kubernaut-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: workflow-execution-sa
  namespace: kubernaut-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernetes-executor-sa
  namespace: kubernaut-system
```

### ClusterRole (Shared)

**File**: `deploy/crd-controllers/clusterrole.yaml`

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: crd-controller-role
rules:
# CRD permissions
- apiGroups: ["remediationprocessing.kubernaut.ai"]
  resources: ["remediationprocessings"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["remediationprocessing.kubernaut.ai"]
  resources: ["remediationprocessings/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["remediationprocessing.kubernaut.ai"]
  resources: ["remediationprocessings/finalizers"]
  verbs: ["update"]

- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/finalizers"]
  verbs: ["update"]

- apiGroups: ["kubernetesexecution.kubernaut.ai"]  # DEPRECATED - ADR-025
  resources: ["kubernetesexecutions"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernetesexecution.kubernaut.ai"]  # DEPRECATED - ADR-025
  resources: ["kubernetesexecutions/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernetesexecution.kubernaut.ai"]  # DEPRECATED - ADR-025
  resources: ["kubernetesexecutions/finalizers"]
  verbs: ["update"]

# Kubernetes Job management (for Kubernetes Executor) (DEPRECATED - ADR-025)
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# ServiceAccount management (for per-action RBAC)
- apiGroups: [""]
  resources: ["serviceaccounts"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles", "rolebindings"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Leader election
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Events
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

### ClusterRoleBindings

**File**: `deploy/crd-controllers/clusterrolebindings.yaml`

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: remediation-processor-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: crd-controller-role
subjects:
- kind: ServiceAccount
  name: remediation-processor-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: workflow-execution-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: crd-controller-role
subjects:
- kind: ServiceAccount
  name: workflow-execution-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernetes-executor-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: crd-controller-role
subjects:
- kind: ServiceAccount
  name: kubernetes-executor-sa
  namespace: kubernaut-system
```

---

## 🔄 High Availability Setup

### Leader Election Configuration

All three controllers use leader election to ensure only one replica is active at a time:

**Controller Arguments**:
```yaml
args:
- --leader-elect=true
- --leader-election-id=<controller-name>.kubernaut.ai
- --leader-election-namespace=kubernaut-system
```

**Leader Election Resources**:
- **Resource Type**: Lease (coordination.k8s.io/v1)
- **Lease Duration**: 15s (default)
- **Renew Deadline**: 10s (default)
- **Retry Period**: 2s (default)

### Pod Anti-Affinity

Ensures replicas run on different nodes:

```yaml
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/name
            operator: In
            values:
            - <controller-name>
        topologyKey: kubernetes.io/hostname
```

### Graceful Shutdown

Controllers handle graceful shutdown during rolling updates:

```yaml
terminationGracePeriodSeconds: 10
```

**Shutdown Process**:
1. Receive SIGTERM signal
2. Stop accepting new reconciliation requests
3. Complete in-flight reconciliations (up to 10s)
4. Release leader lease
5. Exit cleanly

---

## 📊 Resource Limits & Requests

### Resource Allocation

| Controller | CPU Request | CPU Limit | Memory Request | Memory Limit |
|------------|-------------|-----------|----------------|--------------|
| **Remediation Processor** | 200m | 500m | 256Mi | 512Mi |
| **Workflow Execution** | 300m | 700m | 384Mi | 768Mi |
| **Kubernetes Executor** (DEPRECATED - ADR-025) | 150m | 500m | 128Mi | 256Mi |

### Resource Tuning Guidelines

**CPU**:
- **Request**: Expected baseline usage (p50)
- **Limit**: Peak usage (p99)
- **Monitoring**: Adjust if CPU throttling occurs

**Memory**:
- **Request**: Minimum required for operation
- **Limit**: Maximum to prevent OOM
- **Monitoring**: Adjust if OOMKilled events occur

**Horizontal Pod Autoscaling** (Optional):

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: workflow-execution-hpa
  namespace: kubernaut-system
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: workflow-execution-controller
  minReplicas: 2
  maxReplicas: 5
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

---

## 📊 Monitoring & Alerting

### Prometheus ServiceMonitor

**File**: `deploy/crd-controllers/servicemonitor.yaml`

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: crd-controllers
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/part-of: kubernaut
spec:
  selector:
    matchLabels:
      app.kubernetes.io/part-of: kubernaut
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

### Key Metrics to Monitor

**Remediation Processor**:
```prometheus
# Enrichment latency
remediation_enrichment_duration_seconds{quantile="0.95"} < 2
remediation_enrichment_duration_seconds{quantile="0.99"} < 5

# Classification accuracy
remediation_classification_decisions_total{result="ai_required"}
remediation_classification_decisions_total{result="automated"}

# Deduplication effectiveness
remediation_duplicate_signals_suppressed_total
```

**Workflow Execution**:
```prometheus
# Workflow completion rate
rate(workflow_execution_completed_total[5m])

# Step execution latency
workflow_step_execution_duration_seconds{quantile="0.95"}

# Parallel execution utilization
workflow_concurrent_steps_active / workflow_max_concurrent_steps

# Rollback frequency
rate(workflow_rollback_triggered_total[5m])
```

**Kubernetes Executor**:
```prometheus
# Action execution latency by action type
kubernetes_action_execution_duration_seconds{action="ScaleDeployment",quantile="0.95"}

# Job success rate
rate(kubernetes_job_completed_total{status="success"}[5m]) / rate(kubernetes_job_completed_total[5m])

# Safety policy violations
rate(kubernetes_safety_policy_violations_total[5m])

# RBAC creation failures
rate(kubernetes_rbac_creation_failed_total[5m])
```

### Alert Rules

**File**: `deploy/crd-controllers/prometheus-rules.yaml`

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: crd-controller-alerts
  namespace: kubernaut-system
spec:
  groups:
  - name: remediation-processor
    interval: 30s
    rules:
    - alert: HighEnrichmentLatency
      expr: remediation_enrichment_duration_seconds{quantile="0.95"} > 5
      for: 5m
      labels:
        severity: warning
        component: remediation-processor
      annotations:
        summary: "High enrichment latency detected"
        description: "95th percentile enrichment latency is {{ $value }}s (threshold: 5s)"

    - alert: ClassificationFailureRate
      expr: rate(remediation_classification_failed_total[5m]) > 0.1
      for: 5m
      labels:
        severity: critical
        component: remediation-processor
      annotations:
        summary: "High classification failure rate"
        description: "Classification failure rate is {{ $value }} per second"

  - name: workflow-execution
    interval: 30s
    rules:
    - alert: HighRollbackRate
      expr: rate(workflow_rollback_triggered_total[5m]) > 0.5
      for: 5m
      labels:
        severity: warning
        component: workflow-execution
      annotations:
        summary: "High workflow rollback rate"
        description: "Rollback rate is {{ $value }} per second (threshold: 0.5)"

    - alert: WorkflowTimeout
      expr: increase(workflow_timeout_exceeded_total[5m]) > 5
      for: 2m
      labels:
        severity: critical
        component: workflow-execution
      annotations:
        summary: "Workflow timeouts detected"
        description: "{{ $value }} workflows exceeded timeout in last 5 minutes"

  - name: kubernetes-executor  # DEPRECATED - ADR-025
    interval: 30s
    rules:
    - alert: HighJobFailureRate
      expr: rate(kubernetes_job_completed_total{status="failed"}[5m]) > 0.2
      for: 5m
      labels:
        severity: warning
        component: kubernetes-executor
      annotations:
        summary: "High Kubernetes Job failure rate"
        description: "Job failure rate is {{ $value }} per second"

    - alert: SafetyPolicyViolations
      expr: increase(kubernetes_safety_policy_violations_total[5m]) > 10
      for: 2m
      labels:
        severity: critical
        component: kubernetes-executor
      annotations:
        summary: "Safety policy violations detected"
        description: "{{ $value }} safety policy violations in last 5 minutes"
```

---

## 🔒 Network Policies

**File**: `deploy/crd-controllers/networkpolicy.yaml`

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: crd-controllers-network-policy
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/part-of: kubernaut
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow Prometheus scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 9090
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
  # Allow Kubernetes API
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow PostgreSQL (for Remediation Processor)
  - to:
    - namespaceSelector:
        matchLabels:
          name: kubernaut-system
      podSelector:
        matchLabels:
          app: postgresql
    ports:
    - protocol: TCP
      port: 5432
```

---

## ✅ Deployment Validation

### Pre-Deployment Checklist

- [ ] CRDs installed and validated
- [ ] Namespace created
- [ ] ServiceAccounts created
- [ ] RBAC roles and bindings applied
- [ ] ConfigMaps created with correct values
- [ ] Secrets created (credentials validated)
- [ ] Network policies applied
- [ ] Prometheus monitoring configured

### Post-Deployment Validation

**1. Verify Pods Running**:
```bash
kubectl get pods -n kubernaut-system
# Expected: 6 pods (2 replicas × 3 controllers) in Running state
```

**2. Check Leader Election**:
```bash
kubectl get leases -n kubernaut-system
# Expected: 3 leases (one per controller)
```

**3. Verify Metrics Endpoint**:
```bash
kubectl port-forward -n kubernaut-system \
  deployment/remediation-processor-controller 9090:9090

curl http://localhost:9090/metrics | grep remediation_
# Expected: Prometheus metrics exposed
```

**4. Test CRD Reconciliation**:
```bash
# Create test SignalProcessing CRD
kubectl apply -f test/integration/testdata/sample-remediationprocessing.yaml

# Watch reconciliation
kubectl get remediationprocessing -w

# Check controller logs
kubectl logs -n kubernaut-system \
  deployment/remediation-processor-controller -f
```

**5. Verify RBAC Permissions**:
```bash
kubectl auth can-i list remediationprocessings \
  --as=system:serviceaccount:kubernaut-system:remediation-processor-sa
# Expected: yes
```

---

## 🚨 Troubleshooting

### Common Issues

**Issue 1: Leader Election Fails**
```bash
# Symptom: Multiple pods show leader election errors

# Check lease status
kubectl describe lease -n kubernaut-system remediation-processor.kubernaut.ai

# Verify RBAC permissions
kubectl auth can-i update leases \
  --as=system:serviceaccount:kubernaut-system:remediation-processor-sa

# Solution: Ensure coordination.k8s.io RBAC permissions are granted
```

**Issue 2: OOMKilled Pods**
```bash
# Symptom: Pods restart with OOMKilled reason

# Check memory usage
kubectl top pods -n kubernaut-system

# Solution: Increase memory limits in deployment
```

**Issue 3: High Enrichment Latency**
```bash
# Symptom: Remediation Processor slow

# Check PostgreSQL connection
kubectl exec -n kubernaut-system deployment/remediation-processor-controller -- \
  curl -v postgresql.kubernaut-system.svc.cluster.local:5432

# Solution: Tune connection pooling in ConfigMap
```

---

## 📚 Additional Resources

- [Kubernetes Controller Runtime Documentation](https://book.kubebuilder.io/)
- [Leader Election Best Practices](https://kubernetes.io/docs/concepts/architecture/leases/)
- [Prometheus Operator Documentation](https://prometheus-operator.dev/)
- [Kubernetes RBAC Documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)

---

**Status**: ✅ **Production Deployment Guide Complete**
**Next Action**: Apply manifests to production cluster
**Validation**: Run post-deployment validation checklist

