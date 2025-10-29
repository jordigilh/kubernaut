# {{CONTROLLER_NAME}} Controller - Deployment Guide

**Controller**: {{CONTROLLER_NAME}}
**Version**: 1.0.0
**Last Updated**: 2025-10-22

---

## 📋 Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Configuration](#configuration)
4. [Validation](#validation)
5. [Scaling and High Availability](#scaling-and-high-availability)
6. [Upgrade and Rollback](#upgrade-and-rollback)
7. [Uninstallation](#uninstallation)

---

## 🔧 Prerequisites

### Kubernetes Cluster Requirements

| Component | Minimum Version | Recommended |
|---|---|---|
| Kubernetes | 1.28+ | 1.29+ |
| kubectl | 1.28+ | 1.29+ |
| Cluster Resources | 2 CPU, 4GB RAM | 4 CPU, 8GB RAM |

### Required CRDs

```bash
# Install CRD for {{CONTROLLER_NAME}}
kubectl apply -f config/crd/{{CRD_GROUP}}_{{CRD_KIND_LOWER}}.yaml

# Verify CRD installation
kubectl get crd {{CRD_KIND_LOWER}}.{{CRD_GROUP}}
```

### External Dependencies

TODO: List controller-specific dependencies
Example for RemediationProcessor:
- PostgreSQL 15+ (Data Storage)
- Context API Service
- Vector Database (pgvector or Milvus)

Example for WorkflowExecution:
- Kubernetes Executor Service
- Workflow Templates ConfigMaps

Example for AIAnalysis:
- HolmesGPT API Service
- Context API Service

---

## 📦 Installation

### Method 1: Using Kubectl

```bash
# 1. Create namespace
kubectl create namespace kubernaut-system

# 2. Create ConfigMap
kubectl apply -f deploy/{{CONTROLLER_NAME}}/configmap.yaml

# 3. Create Secrets (if needed)
# TODO: Add controller-specific secrets
# kubectl create secret generic {{CONTROLLER_NAME}}-secrets \
#   --from-literal=postgres-password=<password> \
#   -n kubernaut-system

# 4. Create RBAC resources
kubectl apply -f deploy/{{CONTROLLER_NAME}}/rbac.yaml

# 5. Deploy controller
kubectl apply -f deploy/{{CONTROLLER_NAME}}/deployment.yaml

# 6. Verify deployment
kubectl get deployment {{CONTROLLER_NAME}} -n kubernaut-system
kubectl get pods -l app={{CONTROLLER_NAME}} -n kubernaut-system
```

### Method 2: Using Kustomize

```bash
# 1. Review kustomization
cat deploy/{{CONTROLLER_NAME}}/kustomization.yaml

# 2. Deploy with kustomize
kubectl apply -k deploy/{{CONTROLLER_NAME}}/

# 3. Verify deployment
kubectl get all -l app={{CONTROLLER_NAME}} -n kubernaut-system
```

### Method 3: Using Helm (if available)

```bash
# TODO: Add Helm installation if chart exists
# helm install {{CONTROLLER_NAME}} ./charts/{{CONTROLLER_NAME}} \
#   --namespace kubernaut-system \
#   --create-namespace
```

---

## ⚙️ Configuration

### ConfigMap Structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{CONTROLLER_NAME}}-config
  namespace: kubernaut-system
data:
  config.yaml: |
    namespace: kubernaut-system
    metrics_address: :8080
    health_address: :8081
    leader_election: true
    log_level: info
    max_concurrency: 10

    kubernetes:
      qps: 20.0
      burst: 30

    # TODO: Add controller-specific configuration
```

### Environment Variables

```yaml
env:
  # Common environment variables
  - name: CONTROLLER_NAMESPACE
    value: "kubernaut-system"
  - name: LOG_LEVEL
    value: "info"
  - name: MAX_CONCURRENCY
    value: "10"

  # TODO: Add controller-specific environment variables
  # Example:
  # - name: POSTGRES_HOST
  #   valueFrom:
  #     configMapKeyRef:
  #       name: {{CONTROLLER_NAME}}-config
  #       key: postgres_host
  # - name: POSTGRES_PASSWORD
  #   valueFrom:
  #     secretKeyRef:
  #       name: {{CONTROLLER_NAME}}-secrets
  #       key: postgres-password
```

### Secrets Management

```bash
# Create secrets manually
kubectl create secret generic {{CONTROLLER_NAME}}-secrets \
  --from-literal=key1=value1 \
  --from-literal=key2=value2 \
  -n kubernaut-system

# Or from file
kubectl create secret generic {{CONTROLLER_NAME}}-secrets \
  --from-env-file=.env \
  -n kubernaut-system

# Using sealed-secrets (recommended for GitOps)
kubeseal --format=yaml < secrets.yaml > sealed-secrets.yaml
kubectl apply -f sealed-secrets.yaml
```

---

## ✅ Validation

### Deployment Validation Script

```bash
#!/bin/bash
# validate-deployment.sh

set -e

NAMESPACE="kubernaut-system"
CONTROLLER="{{CONTROLLER_NAME}}"

echo "=== Validating {{CONTROLLER_NAME}} Deployment ==="

# 1. Check namespace
echo "✓ Checking namespace..."
kubectl get namespace $NAMESPACE > /dev/null 2>&1 || {
  echo "✗ Namespace $NAMESPACE not found"
  exit 1
}

# 2. Check CRD
echo "✓ Checking CRD..."
kubectl get crd {{CRD_KIND_LOWER}}.{{CRD_GROUP}} > /dev/null 2>&1 || {
  echo "✗ CRD not found"
  exit 1
}

# 3. Check ConfigMap
echo "✓ Checking ConfigMap..."
kubectl get configmap $CONTROLLER-config -n $NAMESPACE > /dev/null 2>&1 || {
  echo "✗ ConfigMap not found"
  exit 1
}

# 4. Check Deployment
echo "✓ Checking Deployment..."
kubectl get deployment $CONTROLLER -n $NAMESPACE > /dev/null 2>&1 || {
  echo "✗ Deployment not found"
  exit 1
}

# 5. Check Pod Status
echo "✓ Checking Pod Status..."
READY_PODS=$(kubectl get pods -l app=$CONTROLLER -n $NAMESPACE -o jsonpath='{.items[*].status.containerStatuses[0].ready}' | grep -o "true" | wc -l)
DESIRED_REPLICAS=$(kubectl get deployment $CONTROLLER -n $NAMESPACE -o jsonpath='{.spec.replicas}')

if [ "$READY_PODS" -eq "$DESIRED_REPLICAS" ]; then
  echo "✓ All pods are ready ($READY_PODS/$DESIRED_REPLICAS)"
else
  echo "✗ Not all pods are ready ($READY_PODS/$DESIRED_REPLICAS)"
  exit 1
fi

# 6. Check Health Endpoints
echo "✓ Checking Health Endpoints..."
POD=$(kubectl get pods -l app=$CONTROLLER -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')

kubectl exec -it $POD -n $NAMESPACE -- curl -sf http://localhost:8081/healthz > /dev/null || {
  echo "✗ Health check failed"
  exit 1
}

kubectl exec -it $POD -n $NAMESPACE -- curl -sf http://localhost:8081/readyz > /dev/null || {
  echo "✗ Readiness check failed"
  exit 1
}

# 7. Check Metrics
echo "✓ Checking Metrics..."
kubectl exec -it $POD -n $NAMESPACE -- curl -sf http://localhost:8080/metrics > /dev/null || {
  echo "✗ Metrics endpoint failed"
  exit 1
}

# TODO: Add controller-specific validations
# Example: Check database connectivity, external service health, etc.

echo ""
echo "✅ All validations passed!"
echo "{{CONTROLLER_NAME}} is deployed and healthy"
```

### Manual Validation Steps

```bash
# 1. Check deployment status
kubectl get deployment {{CONTROLLER_NAME}} -n kubernaut-system

# 2. Check pod status
kubectl get pods -l app={{CONTROLLER_NAME}} -n kubernaut-system

# 3. Check logs for errors
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system --tail=50

# 4. Check recent events
kubectl get events -n kubernaut-system --sort-by='.lastTimestamp' | grep {{CONTROLLER_NAME}}

# 5. Test reconciliation with sample resource
kubectl apply -f config/samples/{{CRD_GROUP}}_v1alpha1_{{CRD_KIND_LOWER}}.yaml

# 6. Verify resource was reconciled
kubectl get {{CRD_KIND}} -A
kubectl describe {{CRD_KIND}} <name> -n <namespace>
```

---

## 📈 Scaling and High Availability

### Horizontal Scaling

```bash
# Scale to multiple replicas
kubectl scale deployment {{CONTROLLER_NAME}} --replicas=3 -n kubernaut-system

# Verify scaling
kubectl get deployment {{CONTROLLER_NAME}} -n kubernaut-system
kubectl get pods -l app={{CONTROLLER_NAME}} -n kubernaut-system
```

### Leader Election

The controller uses leader election to ensure only one instance is actively reconciling at a time.

```yaml
# Leader election is enabled by default
env:
  - name: LEADER_ELECTION
    value: "true"
```

### Pod Disruption Budget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{CONTROLLER_NAME}}-pdb
  namespace: kubernaut-system
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: {{CONTROLLER_NAME}}
```

### Resource Requests and Limits

```yaml
resources:
  requests:
    memory: "256Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

### Anti-Affinity for HA

```yaml
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app
                operator: In
                values:
                  - {{CONTROLLER_NAME}}
          topologyKey: kubernetes.io/hostname
```

---

## 🔄 Upgrade and Rollback

### Upgrade Procedure

```bash
# 1. Review release notes
# Check for breaking changes, new configuration options

# 2. Backup current resources
kubectl get {{CRD_KIND}} -A -o yaml > {{CONTROLLER_NAME}}-resources-backup.yaml
kubectl get configmap {{CONTROLLER_NAME}}-config -n kubernaut-system -o yaml > config-backup.yaml

# 3. Update ConfigMap if needed
kubectl apply -f deploy/{{CONTROLLER_NAME}}/configmap.yaml

# 4. Update deployment with new image
kubectl set image deployment/{{CONTROLLER_NAME}} \
  {{CONTROLLER_NAME}}=quay.io/jordigilh/{{IMAGE_NAME}}:v0.2.0 \
  -n kubernaut-system

# 5. Monitor rollout
kubectl rollout status deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# 6. Verify health
kubectl get pods -l app={{CONTROLLER_NAME}} -n kubernaut-system
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system --tail=50

# 7. Test with sample resource
kubectl apply -f config/samples/{{CRD_GROUP}}_v1alpha1_{{CRD_KIND_LOWER}}.yaml
```

### Rollback Procedure

```bash
# 1. Check rollout history
kubectl rollout history deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# 2. Rollback to previous version
kubectl rollout undo deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# 3. Rollback to specific revision
kubectl rollout undo deployment/{{CONTROLLER_NAME}} --to-revision=2 -n kubernaut-system

# 4. Monitor rollback
kubectl rollout status deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# 5. Verify health
kubectl get pods -l app={{CONTROLLER_NAME}} -n kubernaut-system
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system --tail=50
```

---

## 🗑️ Uninstallation

### Complete Uninstallation

```bash
# 1. Backup resources before deletion
kubectl get {{CRD_KIND}} -A -o yaml > {{CONTROLLER_NAME}}-final-backup.yaml

# 2. Delete controller resources
kubectl delete -f deploy/{{CONTROLLER_NAME}}/

# 3. Delete CRD (this will delete all CRD resources!)
kubectl delete crd {{CRD_KIND_LOWER}}.{{CRD_GROUP}}

# 4. Delete namespace (if no other controllers)
kubectl delete namespace kubernaut-system

# 5. Verify cleanup
kubectl get all -n kubernaut-system
kubectl get crd | grep {{CRD_GROUP}}
```

### Uninstall Without Deleting Resources

```bash
# Delete only the controller, keep CRD and resources
kubectl delete deployment {{CONTROLLER_NAME}} -n kubernaut-system
kubectl delete configmap {{CONTROLLER_NAME}}-config -n kubernaut-system
kubectl delete serviceaccount {{CONTROLLER_NAME}} -n kubernaut-system

# Keep CRD and resources intact
# kubectl get {{CRD_KIND}} -A (resources still exist)
```

---

## 📚 Additional Resources

- [Kubernetes Deployment Best Practices](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
- [Kustomize Documentation](https://kustomize.io/)
- [Helm Documentation](https://helm.sh/docs/)
- [Controller Runtime Deployment Guide](https://book.kubebuilder.io/reference/using-finalizers.html)

---

## 🤝 Support

For deployment issues:

1. Check [OPERATIONS.md](./OPERATIONS.md) for troubleshooting
2. Review controller logs
3. Check Kubernetes events
4. Open GitHub issue with deployment details

---

**Document Status**: ✅ **PRODUCTION-READY**
**Maintained By**: Kubernaut Operations Team
