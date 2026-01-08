# Auth Webhook Production Deployment

**Status**: ✅ Production Ready  
**Purpose**: User attribution for CRD status changes (SOC2 CC8.1 compliance)  
**Component**: Kubernetes Admission Webhook  

---

## 📋 **Overview**

The Auth Webhook intercepts CRD status updates to inject authenticated user identity for SOC2 compliance audit trails.

### **SOC2 CC8.1 Requirement**
- ✅ Track who initiated remediation actions
- ✅ Track who approved/rejected remediation requests
- ✅ Track who deleted notification requests
- ✅ Capture authenticated user identity in audit trails

### **How It Works**

```
User/Service Action
    ↓
Kubernetes API Server
    ↓
Auth Webhook (mTLS) ────→ Validate/Mutate
    ↓                          ↓
Inject User Identity    Create Audit Event
    ↓                          ↓
CRD Persisted          DataStorage Service
    ↓
Workflow Controller Processes
```

---

## 🚀 **Deployment**

### **Prerequisites**

1. **cert-manager** installed in cluster:
```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.3/cert-manager.yaml
```

2. **ClusterIssuer** for self-signed certificates:
```bash
kubectl apply -f ../cert-manager/selfsigned-issuer.yaml
```

3. **DataStorage Service** deployed (for audit event creation):
```bash
kubectl apply -k ../data-storage/
```

### **Install Auth Webhook**

```bash
# Deploy with Kustomize
kubectl apply -k deploy/authwebhook/

# Verify deployment
kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=authwebhook
kubectl get certificate -n kubernaut-system authwebhook-tls
kubectl get mutatingwebhookconfiguration authwebhook-mutating
kubectl get validatingwebhookconfiguration authwebhook-validating
```

### **Expected Output**

```
NAME                          READY   STATUS    RESTARTS   AGE
authwebhook-xxxxxxxxxx-xxxxx   1/1     Running   0          30s
authwebhook-xxxxxxxxxx-xxxxx   1/1     Running   0          30s

NAME               READY   SECRET             AGE
authwebhook-tls    True    authwebhook-tls    30s

NAME                      WEBHOOKS   AGE
authwebhook-mutating      2          30s

NAME                       WEBHOOKS   AGE
authwebhook-validating     1          30s
```

---

## 🔧 **Configuration**

### **Environment Variables**

| Variable | Default | Description |
|----------|---------|-------------|
| `WEBHOOK_PORT` | `9443` | HTTPS port for webhook server |
| `LOG_LEVEL` | `info` | Log verbosity (debug/info/warn/error) |
| `DATA_STORAGE_URL` | `http://data-storage-service.kubernaut-system.svc.cluster.local:8080` | DataStorage endpoint |

### **Resource Requirements**

| Resource | Request | Limit |
|----------|---------|-------|
| CPU | 100m | 500m |
| Memory | 128Mi | 256Mi |
| Replicas | 2 | High availability |

### **Namespace Selector**

Webhooks only apply to namespaces labeled with:
```yaml
kubernaut.ai/audit-enabled: "true"
```

**Enable audit for a namespace**:
```bash
kubectl label namespace my-namespace kubernaut.ai/audit-enabled=true
```

---

## 📋 **Managed CRDs**

### **1. WorkflowExecution (Mutating)**
- **Path**: `/mutate-workflowexecution`
- **Operation**: UPDATE status
- **Purpose**: Inject user who initiated workflow execution
- **Injected Fields**: `status.initiatedBy`, `status.approvedBy`

### **2. RemediationApprovalRequest (Mutating)**
- **Path**: `/mutate-remediationapprovalrequest`
- **Operation**: UPDATE status
- **Purpose**: Track approval/rejection decisions
- **Injected Fields**: `status.approvedBy`, `status.rejectedBy`

### **3. NotificationRequest (Validating)**
- **Path**: `/validate-notificationrequest-delete`
- **Operation**: DELETE
- **Purpose**: Audit notification request deletions
- **Action**: Create audit event before deletion

---

## 🔐 **Security**

### **Authentication**
- **mTLS**: Webhook authenticates to Kubernetes API via cert-manager managed certificate
- **Certificate**: Automatically issued and renewed by cert-manager (30 days before expiry)
- **Rotation**: Zero-downtime certificate rotation via Secret updates

### **Authorization (RBAC)**
```yaml
ClusterRole: authwebhook
Permissions:
  - Read CRDs (workflowexecutions, remediationapprovalrequests, notificationrequests)
  - Update CRD status (for mutation)
  - Create TokenReviews (for authentication)
  - Create SubjectAccessReviews (for authorization)
```

### **Failure Policy**
- **Mode**: `Fail` (block operations if webhook unavailable)
- **Rationale**: SOC2 compliance requires audit trail completeness
- **Timeout**: 10 seconds per webhook call

---

## 🧪 **Testing**

### **Health Checks**

```bash
# Liveness probe
kubectl exec -n kubernaut-system deploy/authwebhook -- curl -s http://localhost:8081/healthz

# Readiness probe
kubectl exec -n kubernaut-system deploy/authwebhook -- curl -s http://localhost:8081/readyz
```

### **Webhook Validation**

```bash
# Create test namespace with audit enabled
kubectl create namespace webhook-test
kubectl label namespace webhook-test kubernaut.ai/audit-enabled=true

# Create test WorkflowExecution CRD
kubectl apply -f - <<EOF
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: test-workflow
  namespace: webhook-test
spec:
  workflowID: test-001
EOF

# Update status (webhook will inject user identity)
kubectl patch workflowexecution test-workflow -n webhook-test \
  --type=merge \
  --subresource=status \
  -p '{"status":{"state":"approved"}}'

# Verify user attribution
kubectl get workflowexecution test-workflow -n webhook-test -o jsonpath='{.status.approvedBy}'
# Expected: system:serviceaccount:default:kubectl (or your user)
```

### **E2E Tests**

```bash
# Run complete E2E test suite
ginkgo run -v test/e2e/authwebhook/

# Expected: 2 test scenarios, ~30s execution time
```

---

## 📊 **Monitoring**

### **Metrics**

Webhook exposes Prometheus metrics on port `8081`:

| Metric | Type | Description |
|--------|------|-------------|
| `webhook_admission_requests_total` | Counter | Total admission requests by webhook |
| `webhook_admission_duration_seconds` | Histogram | Request processing duration |
| `webhook_admission_errors_total` | Counter | Failed admission requests |

### **Logs**

```bash
# View webhook logs
kubectl logs -n kubernaut-system -l app.kubernetes.io/name=authwebhook -f

# Filter audit events
kubectl logs -n kubernaut-system -l app.kubernetes.io/name=authwebhook | grep "audit_event"
```

---

## 🔧 **Troubleshooting**

### **Webhook Not Receiving Requests**

```bash
# Check webhook configuration
kubectl get mutatingwebhookconfiguration authwebhook-mutating -o yaml
kubectl get validatingwebhookconfiguration authwebhook-validating -o yaml

# Verify CA bundle injected by cert-manager
kubectl get mutatingwebhookconfiguration authwebhook-mutating \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d

# Check namespace labels
kubectl get namespace my-namespace -o jsonpath='{.metadata.labels}'
```

### **Certificate Issues**

```bash
# Check certificate status
kubectl get certificate -n kubernaut-system authwebhook-tls
kubectl describe certificate -n kubernaut-system authwebhook-tls

# Check secret
kubectl get secret -n kubernaut-system authwebhook-tls
kubectl get secret -n kubernaut-system authwebhook-tls -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text

# Force certificate renewal
kubectl delete secret -n kubernaut-system authwebhook-tls
# cert-manager will recreate automatically
```

### **High Latency or Timeouts**

```bash
# Check pod resource usage
kubectl top pods -n kubernaut-system -l app.kubernetes.io/name=authwebhook

# Check DataStorage connectivity
kubectl exec -n kubernaut-system deploy/authwebhook -- \
  curl -s http://data-storage-service.kubernaut-system.svc.cluster.local:8080/health/ready

# Increase timeout if needed
kubectl patch mutatingwebhookconfiguration authwebhook-mutating \
  --type=json \
  -p='[{"op": "replace", "path": "/webhooks/0/timeoutSeconds", "value": 30}]'
```

---

## 🚀 **Upgrade**

### **Zero-Downtime Deployment**

```bash
# Update image tag in kustomization.yaml
# Then apply
kubectl apply -k deploy/authwebhook/

# Watch rollout
kubectl rollout status deployment/authwebhook -n kubernaut-system

# Verify new pods
kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=authwebhook
```

### **Rollback**

```bash
# Rollback to previous version
kubectl rollout undo deployment/authwebhook -n kubernaut-system

# Verify rollback
kubectl rollout history deployment/authwebhook -n kubernaut-system
```

---

## 📚 **References**

- **SOC2 Plan**: `docs/development/SOC2/SOC2_WEEK2_COMPLETE_PLAN_V1_1_JAN07.md`
- **E2E Tests**: `test/e2e/authwebhook/`
- **Webhook Implementation**: `cmd/webhooks/main.go`
- **cert-manager Docs**: https://cert-manager.io/docs/

---

## ✅ **Production Readiness Checklist**

- [x] High availability (2 replicas)
- [x] Health checks (liveness + readiness)
- [x] Resource limits configured
- [x] Security context (non-root, read-only FS)
- [x] TLS via cert-manager
- [x] Automatic certificate rotation
- [x] Namespace selector (opt-in audit)
- [x] Failure policy: Fail (SOC2 requirement)
- [x] E2E tests passing
- [x] Monitoring metrics exposed
- [x] RBAC least privilege

---

**Document Version**: 1.0  
**Last Updated**: January 7, 2025  
**Component Status**: ✅ Production Ready

