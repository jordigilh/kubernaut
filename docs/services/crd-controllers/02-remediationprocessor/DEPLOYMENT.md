# RemediationProcessor Controller - Deployment Guide

**Version**: 1.0.0
**Last Updated**: October 21, 2025
**Maintainer**: kubernaut-dev@jordigilh.com

---

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Installation](#installation)
4. [Configuration Management](#configuration-management)
5. [Validation](#validation)
6. [Scaling and High Availability](#scaling-and-high-availability)
7. [Upgrade Procedures](#upgrade-procedures)
8. [Rollback Procedures](#rollback-procedures)
9. [Uninstallation](#uninstallation)

---

## Overview

### Deployment Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│ Kubernetes Cluster                                                 │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐ │
│  │ Namespace: kubernaut-system                                  │ │
│  │                                                              │ │
│  │  ┌────────────────────────────────────────────────────────┐ │ │
│  │  │ RemediationProcessor Deployment (2-3 replicas)         │ │ │
│  │  │                                                        │ │ │
│  │  │  ┌──────────┐  ┌──────────┐  ┌──────────┐           │ │ │
│  │  │  │ Leader   │  │ Standby  │  │ Standby  │           │ │ │
│  │  │  │ Pod      │  │ Pod      │  │ Pod      │           │ │ │
│  │  │  │ :8080    │  │ :8080    │  │ :8080    │           │ │ │
│  │  │  │ :8081    │  │ :8081    │  │ :8081    │           │ │ │
│  │  │  └────┬─────┘  └────┬─────┘  └────┬─────┘           │ │ │
│  │  │       │             │             │                 │ │ │
│  │  └───────┼─────────────┼─────────────┼─────────────────┘ │ │
│  │          │             │             │                   │ │
│  │  ┌───────┴─────────────┴─────────────┴────────────────┐  │ │
│  │  │ Service (Metrics & Health)                         │  │ │
│  │  │  - metrics: 8080/TCP                               │  │ │
│  │  │  - health: 8081/TCP                                │  │ │
│  │  └────────────────────────────────────────────────────┘  │ │
│  │                                                           │ │
│  │  ┌────────────────────────────────────────────────────┐  │ │
│  │  │ ConfigMap (remediationprocessor-config)            │  │ │
│  │  │  - Controller settings                             │  │ │
│  │  │  - PostgreSQL connection                           │  │ │
│  │  │  - Context API endpoint                            │  │ │
│  │  │  - Classification thresholds                       │  │ │
│  │  └────────────────────────────────────────────────────┘  │ │
│  │                                                           │ │
│  │  ┌────────────────────────────────────────────────────┐  │ │
│  │  │ Secret (remediationprocessor-secret)               │  │ │
│  │  │  - PostgreSQL password                             │  │ │
│  │  └────────────────────────────────────────────────────┘  │ │
│  │                                                           │ │
│  │  ┌────────────────────────────────────────────────────┐  │ │
│  │  │ ServiceMonitor (Prometheus scraping)               │  │ │
│  │  └────────────────────────────────────────────────────┘  │ │
│  │                                                           │ │
│  │  ┌────────────────────────────────────────────────────┐  │ │
│  │  │ ServiceAccount + RBAC (Permissions)                │  │ │
│  │  └────────────────────────────────────────────────────┘  │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                │
│  External Dependencies:                                        │
│  - PostgreSQL (port 5432) - Remediation history storage       │
│  - Context API (port 8080) - Historical pattern enrichment    │
│  - Kubernetes API - CRD reconciliation                         │
└────────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Type | Purpose |
|-----------|------|---------|
| **Deployment** | Kubernetes Deployment | Controller pods with leader election |
| **Service** | Kubernetes Service | Metrics (8080) and health (8081) |
| **ConfigMap** | Configuration | Controller settings and thresholds |
| **Secret** | Sensitive data | PostgreSQL credentials |
| **ServiceMonitor** | Prometheus CRD | Automatic metrics scraping |
| **ServiceAccount** | RBAC | Kubernetes API permissions |
| **Role/RoleBinding** | RBAC | CRD and lease permissions |

---

## Prerequisites

### Kubernetes Cluster Requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| **Kubernetes Version** | 1.28+ | 1.29+ |
| **Cluster Nodes** | 3 | 5+ |
| **Available CPU** | 500m | 1 core |
| **Available Memory** | 1 GB | 2 GB |

### Required Kubernetes Features

- **CRD Support**: Custom Resource Definitions v1
- **RBAC**: Role-Based Access Control enabled
- **Prometheus Operator**: For ServiceMonitor support (optional but recommended)
- **Persistent Storage**: For PostgreSQL StatefulSet

### Dependencies

| Dependency | Required | Purpose | Port |
|------------|----------|---------|------|
| **Kubernetes API Server** | Yes | CRD reconciliation | 6443 |
| **PostgreSQL** | Yes | Remediation history storage | 5432 |
| **Context API** | Yes | Historical pattern enrichment | 8080 |
| **Prometheus** | Recommended | Metrics collection | 9090 |

### Required CRDs

```bash
# Verify RemediationProcessing CRD is installed
kubectl get crds | grep remediationprocessing

# Expected CRD:
# remediationprocessings.remediation.kubernaut.io
```

If not installed:
```bash
# Install RemediationProcessing CRD
kubectl apply -f config/crd/bases/remediation.kubernaut.io_remediationprocessings.yaml
```

### Tools Required

- `kubectl` 1.28+
- `kustomize` 5.0+ (optional)
- `helm` 3.0+ (optional)
- Access to container registry (quay.io)

---

## Installation

### Method 1: Using Make (Recommended)

```bash
# 1. Clone repository
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut

# 2. Install RemediationProcessing CRD (if not already installed)
kubectl apply -f config/crd/bases/remediation.kubernaut.io_remediationprocessings.yaml

# 3. Verify CRD installation
kubectl get crds remediationprocessings.remediation.kubernaut.io

# 4. Build and push container image (if needed)
make docker-build-remediationprocessor
make docker-push-remediationprocessor

# 5. Deploy controller
make deploy-remediationprocessor

# 6. Verify deployment
kubectl get pods -n kubernaut-system -l app=remediationprocessor
```

### Method 2: Using kubectl (Manual)

#### Step 1: Create Namespace

```bash
kubectl create namespace kubernaut-system
```

#### Step 2: Install RemediationProcessing CRD

```bash
kubectl apply -f config/crd/bases/remediation.kubernaut.io_remediationprocessings.yaml
```

#### Step 3: Create ConfigMap

```bash
kubectl apply -f deploy/remediationprocessor/configmap.yaml
```

**ConfigMap Contents** (`deploy/remediationprocessor/configmap.yaml`):
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: remediationprocessor-config
  namespace: kubernaut-system
  labels:
    app: remediationprocessor
    component: controller
data:
  config.yaml: |
    namespace: kubernaut-system
    metrics_address: ":8080"
    health_address: ":8081"
    leader_election: true
    log_level: info
    max_concurrency: 10

    kubernetes:
      qps: 20.0
      burst: 30

    data_storage:
      postgres_host: postgres-service.kubernaut-system.svc.cluster.local
      postgres_port: 5432
      postgres_user: remediation_user
      postgres_database: kubernaut_remediation
      ssl_mode: require
      max_connections: 25
      max_idle_conns: 5

    context:
      endpoint: http://context-api.kubernaut-system.svc.cluster.local:8080
      timeout: 30
      max_retries: 3
      retry_backoff_ms: 100

    classification:
      semantic_threshold: 0.85
      time_window_minutes: 60
      similarity_engine: cosine
      batch_size: 100
```

#### Step 4: Create Secret

```bash
# Create Secret with PostgreSQL password
kubectl create secret generic remediationprocessor-secret \
  --from-literal=postgres_password='YOUR_SECURE_PASSWORD' \
  -n kubernaut-system

# Label the Secret
kubectl label secret remediationprocessor-secret \
  app=remediationprocessor \
  component=controller \
  -n kubernaut-system
```

#### Step 5: Create ServiceAccount and RBAC

**ServiceAccount** (`deploy/remediationprocessor/serviceaccount.yaml`):
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: remediationprocessor
  namespace: kubernaut-system
  labels:
    app: remediationprocessor
```

**Role** (`deploy/remediationprocessor/role.yaml`):
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: remediationprocessor-role
  namespace: kubernaut-system
rules:
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationprocessings"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationprocessings/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationprocessings/finalizers"]
  verbs: ["update"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
```

**RoleBinding** (`deploy/remediationprocessor/rolebinding.yaml`):
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: remediationprocessor-rolebinding
  namespace: kubernaut-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: remediationprocessor-role
subjects:
- kind: ServiceAccount
  name: remediationprocessor
  namespace: kubernaut-system
```

Apply RBAC resources:
```bash
kubectl apply -f deploy/remediationprocessor/serviceaccount.yaml
kubectl apply -f deploy/remediationprocessor/role.yaml
kubectl apply -f deploy/remediationprocessor/rolebinding.yaml
```

#### Step 6: Create Deployment

**Deployment** (`deploy/remediationprocessor/deployment.yaml`):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: remediationprocessor
  namespace: kubernaut-system
  labels:
    app: remediationprocessor
    component: controller
spec:
  replicas: 2
  selector:
    matchLabels:
      app: remediationprocessor
  template:
    metadata:
      labels:
        app: remediationprocessor
        component: controller
    spec:
      serviceAccountName: remediationprocessor
      containers:
      - name: controller
        image: quay.io/jordigilh/remediationprocessor:v0.1.0
        imagePullPolicy: IfNotPresent
        command:
        - /app/remediationprocessor
        args:
        - --config=/etc/remediationprocessor/config.yaml
        - --leader-elect=true
        - --metrics-bind-address=:8080
        - --health-probe-bind-address=:8081
        ports:
        - name: metrics
          containerPort: 8080
          protocol: TCP
        - name: health
          containerPort: 8081
          protocol: TCP
        env:
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: remediationprocessor-secret
              key: postgres_password
        volumeMounts:
        - name: config
          mountPath: /etc/remediationprocessor
          readOnly: true
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
          timeoutSeconds: 3
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 3
          failureThreshold: 3
        resources:
          requests:
            cpu: 250m
            memory: 512Mi
          limits:
            cpu: 1000m
            memory: 2Gi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 1001
          capabilities:
            drop:
            - ALL
      volumes:
      - name: config
        configMap:
          name: remediationprocessor-config
      securityContext:
        runAsNonRoot: true
        runAsUser: 1001
        fsGroup: 1001
      terminationGracePeriodSeconds: 10
```

Apply deployment:
```bash
kubectl apply -f deploy/remediationprocessor/deployment.yaml
```

#### Step 7: Create Service

**Service** (`deploy/remediationprocessor/service.yaml`):
```yaml
apiVersion: v1
kind: Service
metadata:
  name: remediationprocessor
  namespace: kubernaut-system
  labels:
    app: remediationprocessor
    component: controller
spec:
  selector:
    app: remediationprocessor
  ports:
  - name: metrics
    port: 8080
    targetPort: 8080
    protocol: TCP
  - name: health
    port: 8081
    targetPort: 8081
    protocol: TCP
  type: ClusterIP
```

Apply service:
```bash
kubectl apply -f deploy/remediationprocessor/service.yaml
```

#### Step 8: Create ServiceMonitor (Optional, requires Prometheus Operator)

**ServiceMonitor** (`deploy/remediationprocessor/servicemonitor.yaml`):
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: remediationprocessor
  namespace: kubernaut-system
  labels:
    app: remediationprocessor
spec:
  selector:
    matchLabels:
      app: remediationprocessor
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
    scrapeTimeout: 10s
```

Apply ServiceMonitor:
```bash
kubectl apply -f deploy/remediationprocessor/servicemonitor.yaml
```

---

## Configuration Management

### Environment Variable Overrides

Override any configuration via environment variables in the Deployment:

```bash
# PostgreSQL configuration
kubectl set env deployment/remediationprocessor -n kubernaut-system \
  POSTGRES_HOST=new-postgres-host

# Context API configuration
kubectl set env deployment/remediationprocessor -n kubernaut-system \
  CONTEXT_API_ENDPOINT=http://new-context-api:8080

# Classification tuning
kubectl set env deployment/remediationprocessor -n kubernaut-system \
  SEMANTIC_THRESHOLD=0.80 \
  TIME_WINDOW_MINUTES=120

# Concurrency tuning
kubectl set env deployment/remediationprocessor -n kubernaut-system \
  MAX_CONCURRENCY=20

# Restart to apply
kubectl rollout restart deployment/remediationprocessor -n kubernaut-system
```

### ConfigMap Updates

```bash
# Edit ConfigMap
kubectl edit configmap remediationprocessor-config -n kubernaut-system

# Or apply from file
kubectl apply -f deploy/remediationprocessor/configmap.yaml

# Restart pods to reload configuration
kubectl rollout restart deployment/remediationprocessor -n kubernaut-system
```

### Secret Rotation

```bash
# Update PostgreSQL password
kubectl create secret generic remediationprocessor-secret \
  --from-literal=postgres_password='NEW_SECURE_PASSWORD' \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart pods to use new secret
kubectl rollout restart deployment/remediationprocessor -n kubernaut-system

# Verify new secret is in use
kubectl get secret remediationprocessor-secret -n kubernaut-system -o jsonpath='{.metadata.creationTimestamp}'
```

---

## Validation

### Smoke Tests

#### Test 1: Pod Health

```bash
# Check pod status
kubectl get pods -n kubernaut-system -l app=remediationprocessor

# Expected: All pods in Running state with 1/1 READY

# Check pod events
kubectl get events -n kubernaut-system --field-selector involvedObject.name=remediationprocessor

# Expected: No error events in last 5 minutes
```

#### Test 2: Health Endpoints

```bash
# Port-forward to pod
kubectl port-forward -n kubernaut-system deployment/remediationprocessor 8081:8081 &

# Test liveness
curl http://localhost:8081/healthz
# Expected: ok

# Test readiness
curl http://localhost:8081/readyz
# Expected: ok

# Stop port-forward
kill %1
```

#### Test 3: Metrics Endpoint

```bash
# Port-forward to metrics port
kubectl port-forward -n kubernaut-system deployment/remediationprocessor 8080:8080 &

# Test metrics
curl http://localhost:8080/metrics | grep remediationprocessor

# Expected: remediationprocessor_* metrics present

# Stop port-forward
kill %1
```

#### Test 4: PostgreSQL Connectivity

```bash
# Exec into pod and test connection
kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
  psql -h postgres-service.kubernaut-system.svc.cluster.local \
       -U remediation_user \
       -d kubernaut_remediation \
       -c "SELECT version();"

# Expected: PostgreSQL version output
```

#### Test 5: Context API Connectivity

```bash
# Exec into pod and test Context API
kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
  curl -f http://context-api.kubernaut-system.svc.cluster.local:8080/health

# Expected: HTTP 200 OK
```

#### Test 6: Create RemediationProcessing CR

```bash
# Create test RemediationProcessing
cat <<EOF | kubectl apply -f -
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationProcessing
metadata:
  name: test-remediation
  namespace: kubernaut-system
spec:
  alertName: "HighMemoryUsage"
  severity: "warning"
  targetNamespace: "default"
  targetPod: "test-pod"
EOF

# Wait for reconciliation
sleep 5

# Check status
kubectl get remediationprocessing test-remediation -n kubernaut-system -o yaml

# Expected: Status field populated with processing results

# Cleanup
kubectl delete remediationprocessing test-remediation -n kubernaut-system
```

### Validation Checklist

- [ ] All pods are Running with 1/1 READY
- [ ] No error events in pod events
- [ ] Liveness probe returns 200 OK
- [ ] Readiness probe returns 200 OK
- [ ] Metrics endpoint returns Prometheus metrics
- [ ] PostgreSQL connection successful
- [ ] Context API connection successful
- [ ] Test RemediationProcessing CR reconciles successfully
- [ ] Leader election lease created (`kubectl get lease -n kubernaut-system`)
- [ ] ServiceMonitor scraping metrics (check Prometheus targets)

---

## Scaling and High Availability

### Horizontal Scaling

```bash
# Scale up
kubectl scale deployment/remediationprocessor -n kubernaut-system --replicas=3

# Scale down
kubectl scale deployment/remediationprocessor -n kubernaut-system --replicas=1

# Check scaling status
kubectl get deployment remediationprocessor -n kubernaut-system
```

**Recommendations**:
- **Production**: 2-3 replicas for high availability
- **Development**: 1 replica to save resources
- **High-throughput**: 3-5 replicas with increased resource limits

### Leader Election

Leader election is enabled by default. Only one replica actively reconciles CRDs:

```bash
# Check which pod is leader
kubectl logs -n kubernaut-system -l app=remediationprocessor | grep "leader election"

# Check lease
kubectl get lease -n kubernaut-system | grep remediationprocessor

# Lease holder is shown in HOLDER column
```

### Resource Limits

**Recommended settings by environment**:

**Development**:
```yaml
resources:
  requests:
    cpu: 100m
    memory: 256Mi
  limits:
    cpu: 500m
    memory: 1Gi
```

**Production**:
```yaml
resources:
  requests:
    cpu: 250m
    memory: 512Mi
  limits:
    cpu: 1000m
    memory: 2Gi
```

**High-throughput**:
```yaml
resources:
  requests:
    cpu: 500m
    memory: 1Gi
  limits:
    cpu: 2000m
    memory: 4Gi
```

Apply resource changes:
```bash
kubectl set resources deployment/remediationprocessor -n kubernaut-system \
  --requests=cpu=250m,memory=512Mi \
  --limits=cpu=1000m,memory=2Gi
```

---

## Upgrade Procedures

### Pre-Upgrade Checklist

- [ ] Review release notes for breaking changes
- [ ] Backup ConfigMap and Secret
- [ ] Test new version in staging environment
- [ ] Schedule maintenance window
- [ ] Notify stakeholders

### Upgrade Steps

```bash
# 1. Backup current configuration
kubectl get configmap remediationprocessor-config -n kubernaut-system -o yaml > config_backup_$(date +%Y%m%d).yaml
kubectl get secret remediationprocessor-secret -n kubernaut-system -o yaml > secret_backup_$(date +%Y%m%d).yaml

# 2. Update container image
kubectl set image deployment/remediationprocessor \
  controller=quay.io/jordigilh/remediationprocessor:v0.2.0 \
  -n kubernaut-system

# 3. Monitor rollout
kubectl rollout status deployment/remediationprocessor -n kubernaut-system

# 4. Verify new version
kubectl get pods -n kubernaut-system -l app=remediationprocessor -o jsonpath='{.items[*].spec.containers[*].image}'

# 5. Run smoke tests
# (Run validation tests from Validation section)

# 6. Monitor for errors
kubectl logs -n kubernaut-system -l app=remediationprocessor --tail=100 -f
```

### Post-Upgrade Validation

- [ ] All pods running successfully
- [ ] No increase in error rates
- [ ] Metrics still being scraped
- [ ] RemediationProcessing CRs reconciling correctly
- [ ] Monitor for 30 minutes after upgrade

---

## Rollback Procedures

### Quick Rollback

```bash
# Rollback to previous version
kubectl rollout undo deployment/remediationprocessor -n kubernaut-system

# Monitor rollback
kubectl rollout status deployment/remediationprocessor -n kubernaut-system

# Verify
kubectl get pods -n kubernaut-system -l app=remediationprocessor
```

### Manual Rollback

```bash
# 1. Set to known good image version
kubectl set image deployment/remediationprocessor \
  controller=quay.io/jordigilh/remediationprocessor:v0.1.0 \
  -n kubernaut-system

# 2. Restore backed-up configuration (if needed)
kubectl apply -f config_backup_YYYYMMDD.yaml
kubectl apply -f secret_backup_YYYYMMDD.yaml

# 3. Restart pods
kubectl rollout restart deployment/remediationprocessor -n kubernaut-system

# 4. Validate
kubectl rollout status deployment/remediationprocessor -n kubernaut-system
```

---

## Uninstallation

### Complete Removal

```bash
# 1. Delete RemediationProcessor deployment
make undeploy-remediationprocessor

# Or manually:
kubectl delete -f deploy/remediationprocessor/servicemonitor.yaml
kubectl delete -f deploy/remediationprocessor/service.yaml
kubectl delete -f deploy/remediationprocessor/deployment.yaml
kubectl delete -f deploy/remediationprocessor/rolebinding.yaml
kubectl delete -f deploy/remediationprocessor/role.yaml
kubectl delete -f deploy/remediationprocessor/serviceaccount.yaml
kubectl delete -f deploy/remediationprocessor/configmap.yaml
kubectl delete secret remediationprocessor-secret -n kubernaut-system

# 2. Delete RemediationProcessing CRD (caution: deletes all CR instances)
kubectl delete crd remediationprocessings.remediation.kubernaut.io

# 3. Verify cleanup
kubectl get all -n kubernaut-system -l app=remediationprocessor
# Expected: No resources found
```

### Partial Removal (Keep CRD)

```bash
# Delete only controller, keep CRD and CRs
kubectl delete deployment remediationprocessor -n kubernaut-system
kubectl delete service remediationprocessor -n kubernaut-system
kubectl delete servicemonitor remediationprocessor -n kubernaut-system
kubectl delete configmap remediationprocessor-config -n kubernaut-system
kubectl delete secret remediationprocessor-secret -n kubernaut-system

# CRD and RemediationProcessing CRs remain
kubectl get remediationprocessings -A
```

---

**End of Deployment Guide**










