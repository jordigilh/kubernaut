# Gateway Service Deployment

The Gateway Service provides signal ingestion from Prometheus AlertManager and Kubernetes Events, with deduplication, storm detection, and RemediationRequest CRD creation.

## 📋 **Prerequisites**

- Kubernetes cluster (1.24+) or OpenShift (4.12+)
- kubectl or oc CLI configured
- Redis (included in deployment)

## 🚀 **Quick Start**

### **OpenShift Deployment**

```bash
# Deploy Gateway + Redis to OpenShift
oc apply -k deploy/gateway/overlays/openshift/

# Verify deployment
oc get pods -n kubernaut-system -l app.kubernetes.io/component=gateway
oc logs -n kubernaut-system -l app.kubernetes.io/component=gateway --tail=50
```

### **Vanilla Kubernetes Deployment**

```bash
# Deploy Gateway + Redis to Kubernetes
kubectl apply -k deploy/gateway/overlays/kubernetes/

# Verify deployment
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=gateway
kubectl logs -n kubernaut-system -l app.kubernetes.io/component=gateway --tail=50
```

## 📁 **Directory Structure**

```
deploy/gateway/
├── base/                          # Platform-agnostic base manifests
│   ├── kustomization.yaml
│   ├── 00-namespace.yaml          # kubernaut-system namespace
│   ├── 01-rbac.yaml               # ServiceAccount + ClusterRole + Binding
│   ├── 02-configmap.yaml          # Gateway config + Rego policies
│   ├── 03-deployment.yaml         # Gateway deployment
│   ├── 04-service.yaml            # Gateway service (8080, 9090)
│   ├── 05-redis.yaml              # Redis deployment + service
│   └── 06-servicemonitor.yaml     # Prometheus ServiceMonitor
├── overlays/
│   ├── openshift/                 # OpenShift-specific
│   │   ├── kustomization.yaml
│   │   └── patches/
│   │       ├── remove-security-context.yaml        # Gateway SCC fix
│   │       └── remove-redis-security-context.yaml  # Redis SCC fix
│   └── kubernetes/                # Vanilla K8s (uses base as-is)
│       └── kustomization.yaml
└── README.md                      # This file
```

## 🔧 **Configuration**

### **Environment Variables**

Gateway configuration is managed via ConfigMap (`gateway-config`):

- **Redis**: `redis-gateway.kubernaut-system.svc.cluster.local:6379`
- **Deduplication TTL**: 5 minutes
- **Storm Detection**: Rate threshold 10, Pattern threshold 5
- **Priority Policy**: `/config.app/gateway/policies/priority.rego`

### **Customization**

To customize configuration:

1. Edit `base/02-configmap.yaml`
2. Redeploy: `kubectl apply -k deploy/gateway/overlays/[platform]/`

## 🏗️ **Architecture**

### **Components**

| Component | Purpose | Port |
|---|---|---|
| **Gateway** | Signal ingestion + processing | 8080 (HTTP), 9090 (metrics) |
| **Redis** | Deduplication cache | 6379 |

### **Endpoints**

- `POST /api/v1/signals/prometheus` - Prometheus AlertManager webhooks
- `POST /api/v1/signals/kubernetes-event` - Kubernetes Events
- `GET /health` - Health check
- `GET /ready` - Readiness probe
- `GET /metrics` - Prometheus metrics

## 🔍 **Troubleshooting**

### **Gateway Pod Not Starting (OpenShift)**

**Symptom**: `Error: unable to validate against any security context constraint`

**Solution**: Ensure you're using the OpenShift overlay:
```bash
oc apply -k deploy/gateway/overlays/openshift/
```

### **Redis Connection Failures**

**Symptom**: `Failed to connect to Redis`

**Check Redis status**:
```bash
kubectl get pods -n kubernaut-system -l app=redis-gateway
kubectl logs -n kubernaut-system -l app=redis-gateway
```

### **Image Pull Errors**

**Symptom**: `ImagePullBackOff` or `ErrImagePull`

**Solution**: Ensure image repository is public or configure imagePullSecrets:
```bash
# Check image
kubectl describe pod -n kubernaut-system [gateway-pod-name]
```

## 📊 **Monitoring**

### **Prometheus Metrics**

Gateway exposes 17+ metrics at `:9090/metrics`:

- `gateway_signals_received_total` - Total signals received
- `gateway_signals_deduplicated_total` - Deduplicated signals
- `gateway_storm_detected_total` - Storm detections
- `gateway_crds_created_total` - RemediationRequests created
- `gateway_redis_operations_total` - Redis operations

### **Health Checks**

```bash
# Health check
kubectl exec -n kubernaut-system [gateway-pod] -- curl localhost:8080/health

# Readiness check
kubectl exec -n kubernaut-system [gateway-pod] -- curl localhost:8080/ready
```

## 🔄 **Upgrading**

```bash
# Update image tag in base/kustomization.yaml
# Then redeploy
kubectl apply -k deploy/gateway/overlays/[platform]/

# Or use kubectl set image
kubectl set image deployment/gateway gateway=quay.io/jordigilh/kubernaut-gateway:v1.1.0 -n kubernaut-system
```

## 🗑️ **Uninstall**

```bash
# OpenShift
oc delete -k deploy/gateway/overlays/openshift/

# Kubernetes
kubectl delete -k deploy/gateway/overlays/kubernetes/
```

## 📚 **References**

- [Gateway Service Documentation](../../docs/services/stateless/gateway-service/)
- [Implementation Plan v2.23](../../docs/services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.23.md)
- [Completion Summary](../../docs/services/stateless/gateway-service/GATEWAY_V2.23_COMPLETE.md)
