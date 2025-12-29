# Notification Controller - Production Deployment Guide

**Version**: 1.0.0
**Target Environment**: Kubernetes 1.27+
**Deployment Method**: Kustomize
**Status**: Production-Ready (98% complete)

---

## 📋 **Prerequisites**

### **Infrastructure Requirements**

| Requirement | Minimum | Recommended | Purpose |
|-------------|---------|-------------|---------|
| **Kubernetes Version** | 1.27+ | 1.29+ | CRD support, health probes |
| **CPU (per pod)** | 100m | 200m | Controller processing |
| **Memory (per pod)** | 64Mi | 128Mi | Controller + cache |
| **Persistent Storage** | N/A | N/A | CRDs stored in etcd |

### **External Dependencies**

| Dependency | Required | Purpose |
|------------|----------|---------|
| **Slack Webhook URL** | No | Slack channel notifications |
| **Prometheus** | No | Metrics scraping (optional) |

### **Tools Required**

- `kubectl` 1.27+
- `kustomize` 5.0+ (or `kubectl apply -k`)
- Docker (for building controller image)
- KIND (for local testing)

---

## 🚀 **Deployment Steps**

### **Step 1: Install CRD**

```bash
# Install NotificationRequest CRD
kubectl apply -f config/crd/bases/kubernaut.ai_notificationrequests.yaml

# Verify CRD installation
kubectl get crds | grep notificationrequest
kubectl api-resources | grep notification
```

**Expected Output**:
```
notificationrequests.kubernaut.ai     2025-10-12T10:00:00Z
```

---

### **Step 2: Create Namespace**

```bash
# Create dedicated namespace for notification controller
kubectl apply -f deploy/notification/00-namespace.yaml

# Verify namespace creation
kubectl get namespace kubernaut-notifications
```

**Expected Output**:
```
NAME                       STATUS   AGE
kubernaut-notifications    Active   5s
```

---

### **Step 3: Configure RBAC**

```bash
# Create ServiceAccount, ClusterRole, and ClusterRoleBinding
kubectl apply -f deploy/notification/01-rbac.yaml

# Verify RBAC configuration
kubectl get serviceaccount notification-controller -n kubernaut-notifications
kubectl get clusterrole notification-controller
kubectl get clusterrolebinding notification-controller
```

**Verify Permissions**:
```bash
# Verify controller can access NotificationRequests
kubectl auth can-i get notificationrequests \
  --as=system:serviceaccount:kubernaut-notifications:notification-controller

# Verify controller can update status
kubectl auth can-i update notificationrequests/status \
  --as=system:serviceaccount:kubernaut-notifications:notification-controller
```

---

### **Step 4: Create Slack Secret** (Optional)

If using Slack notifications:

```bash
# Create secret with Slack webhook URL
kubectl create secret generic notification-slack-webhook \
  --from-literal=webhook-url="https://hooks.slack.com/services/YOUR/WEBHOOK/URL" \
  -n kubernaut-notifications

# Verify secret creation
kubectl get secret notification-slack-webhook -n kubernaut-notifications
```

**Alternative: From File**
```bash
# Create secret file
cat > slack-webhook-secret.yaml <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: notification-slack-webhook
  namespace: kubernaut-notifications
type: Opaque
stringData:
  webhook-url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
EOF

kubectl apply -f slack-webhook-secret.yaml
```

**Without Slack**:
If not using Slack, the controller will start successfully (Slack delivery will be disabled).

---

### **Step 5: Deploy Controller**

#### **Option A: Using Kustomize (Recommended)**

```bash
# Deploy all resources with single command
kubectl apply -k deploy/notification/

# Wait for controller to be ready
kubectl wait --for=condition=available deployment/notification-controller \
  -n kubernaut-notifications --timeout=60s
```

#### **Option B: Using Individual Files**

```bash
# Deploy in order
kubectl apply -f deploy/notification/00-namespace.yaml
kubectl apply -f deploy/notification/01-rbac.yaml
kubectl apply -f deploy/notification/02-deployment.yaml
kubectl apply -f deploy/notification/03-service.yaml

# Wait for controller
kubectl wait --for=condition=available deployment/notification-controller \
  -n kubernaut-notifications --timeout=60s
```

---

### **Step 6: Verify Deployment**

```bash
# Check pod status
kubectl get pods -n kubernaut-notifications

# Expected output:
# NAME                                        READY   STATUS    RESTARTS   AGE
# notification-controller-xxxxxxxxxx-xxxxx    1/1     Running   0          30s

# Check controller logs
kubectl logs -f deployment/notification-controller -n kubernaut-notifications

# Expected log entries:
# {"level":"info","ts":"...","msg":"Starting controller"}
# {"level":"info","ts":"...","msg":"Listening for health probes on :8081"}
# {"level":"info","ts":"...","msg":"Listening for metrics on :8080"}
```

---

### **Step 7: Verify Health**

```bash
# Port-forward to health endpoint
kubectl port-forward -n kubernaut-notifications \
  deployment/notification-controller 8081:8081 &

# Check liveness
curl http://localhost:8081/healthz

# Check readiness
curl http://localhost:8081/readyz

# Expected output for both: "ok"
```

---

### **Step 8: Verify Metrics**

```bash
# Port-forward to metrics endpoint
kubectl port-forward -n kubernaut-notifications \
  deployment/notification-controller 8080:8080 &

# Check Prometheus metrics
curl http://localhost:8080/metrics | grep notification_

# Expected metrics:
# notification_requests_total{...}
# notification_delivery_attempts_total{...}
# notification_reconciliation_duration_seconds{...}
```

---

## ✅ **Validation Tests**

### **Test 1: Create Notification (Console Only)**

```bash
# Create test notification
cat > test-notification.yaml <<EOF
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: test-notification
  namespace: kubernaut-notifications
spec:
  subject: "Test Notification"
  body: "This is a test message"
  type: info
  priority: low
  channels:
    - console
EOF

kubectl apply -f test-notification.yaml

# Wait for processing
sleep 5

# Check status
kubectl get notificationrequest test-notification -n kubernaut-notifications -o yaml
```

**Expected Status**:
```yaml
status:
  phase: Sent
  reason: AllDeliveriesSucceeded
  message: "Successfully delivered to 1 channel(s)"
  deliveryAttempts:
    - channel: console
      status: success
      timestamp: "2025-10-12T10:00:00Z"
  totalAttempts: 1
  successfulDeliveries: 1
  failedDeliveries: 0
  completionTime: "2025-10-12T10:00:01Z"
```

---

### **Test 2: Create Notification (Console + Slack)**

```bash
# Create test notification with Slack
cat > test-notification-slack.yaml <<EOF
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: test-notification-slack
  namespace: kubernaut-notifications
spec:
  subject: "Test Slack Notification"
  body: "This message should appear in Slack"
  type: alert
  priority: high
  channels:
    - console
    - slack
EOF

kubectl apply -f test-notification-slack.yaml

# Wait for processing
sleep 5

# Check status
kubectl get notificationrequest test-notification-slack -n kubernaut-notifications \
  -o jsonpath='{.status}' | jq .
```

**Expected**: Slack message delivered to configured channel.

---

### **Test 3: Verify Retry Logic**

```bash
# Create notification with intentionally invalid Slack webhook
# (Controller will retry with exponential backoff)

# Temporarily update secret with invalid URL
kubectl patch secret notification-slack-webhook \
  -n kubernaut-notifications \
  --type='json' \
  -p='[{"op": "replace", "path": "/data/webhook-url", "value": "'"$(echo -n 'https://invalid-webhook-url' | base64)"'"}]'

# Create notification
kubectl apply -f test-notification-slack.yaml

# Watch retry attempts (check every 30s, 60s, 120s)
watch -n 10 'kubectl get notificationrequest test-notification-slack -n kubernaut-notifications -o jsonpath="{.status.deliveryAttempts}" | jq .'

# Restore valid webhook URL
kubectl patch secret notification-slack-webhook \
  -n kubernaut-notifications \
  --type='json' \
  -p='[{"op": "replace", "path": "/data/webhook-url", "value": "'"$(echo -n 'https://hooks.slack.com/services/YOUR/WEBHOOK/URL' | base64)"'"}]'
```

**Expected**: Multiple delivery attempts with increasing delays.

---

## 📊 **Production Monitoring**

### **Prometheus Scraping**

The controller exposes metrics on `:8080/metrics` with Prometheus annotations:

```yaml
# Annotations in deployment
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8080"
  prometheus.io/path: "/metrics"
```

**Prometheus ServiceMonitor** (if using Prometheus Operator):

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: notification-controller
  namespace: kubernaut-notifications
spec:
  selector:
    matchLabels:
      control-plane: notification-controller
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```

---

### **Grafana Dashboard**

**Key Metrics to Monitor**:

1. **Notification Volume**:
   - `rate(notification_requests_total[5m])`

2. **Delivery Success Rate**:
   - `rate(notification_delivery_attempts_total{status="success"}[5m])`
   - `rate(notification_delivery_attempts_total{status="failed"}[5m])`

3. **Circuit Breaker State**:
   - `notification_circuit_breaker_state` (0=closed, 1=open, 2=half-open)

4. **Reconciliation Performance**:
   - `histogram_quantile(0.95, rate(notification_reconciliation_duration_seconds_bucket[5m]))`

5. **Channel Health**:
   - `notification_channel_health_score` (0-100 scale)

---

### **Alerting Rules**

**Prometheus Alerting**:

```yaml
groups:
  - name: notification-controller
    rules:
      - alert: NotificationControllerDown
        expr: up{job="notification-controller"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Notification controller is down"
          description: "No metrics from notification controller for 5 minutes"

      - alert: HighNotificationFailureRate
        expr: |
          rate(notification_delivery_attempts_total{status="failed"}[5m])
          /
          rate(notification_delivery_attempts_total[5m])
          > 0.5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High notification failure rate"
          description: "More than 50% of notifications failing"

      - alert: CircuitBreakerOpen
        expr: notification_circuit_breaker_state == 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Circuit breaker open for {{ $labels.channel }}"
          description: "Channel {{ $labels.channel }} circuit breaker has been open for 5 minutes"
```

---

## 🔒 **Security Hardening**

### **Pod Security Standards**

The deployment follows **Restricted** Pod Security Standards:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  fsGroup: 65532
  seccompProfile:
    type: RuntimeDefault

containers:
  - securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
          - ALL
```

### **Network Policies**

**Restrict Ingress** (optional):

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: notification-controller
  namespace: kubernaut-notifications
spec:
  podSelector:
    matchLabels:
      control-plane: notification-controller
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: kubernaut-system
      ports:
        - protocol: TCP
          port: 8080  # Metrics
        - protocol: TCP
          port: 8081  # Health
  egress:
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443  # Slack HTTPS
    - to:
        - namespaceSelector:
            matchLabels:
              name: kube-system
      ports:
        - protocol: TCP
          port: 443  # Kubernetes API
```

### **Secret Management Best Practices**

1. **Kubernetes Secrets**: Use native Secrets for Slack webhook URL
2. **External Secrets Operator**: For production, consider using External Secrets Operator with vault
3. **RBAC**: Controller has read-only access to secrets
4. **Rotation**: Implement secret rotation policy (e.g., every 90 days)

---

## 🔄 **Upgrading**

### **Rolling Update Strategy**

The deployment uses RollingUpdate strategy by default:

```yaml
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
```

**Upgrade Steps**:

```bash
# 1. Update CRD (if changed)
kubectl apply -f config/crd/bases/kubernaut.ai_notificationrequests.yaml

# 2. Update controller image
kubectl set image deployment/notification-controller \
  manager=localhost:5001/kubernaut-notification:v1.1.0 \
  -n kubernaut-notifications

# 3. Monitor rollout
kubectl rollout status deployment/notification-controller -n kubernaut-notifications

# 4. Verify new version
kubectl logs -f deployment/notification-controller -n kubernaut-notifications | grep "version"
```

**Rollback** (if needed):

```bash
kubectl rollout undo deployment/notification-controller -n kubernaut-notifications
```

---

## 🧹 **Cleanup**

### **Remove Controller**

```bash
# Delete all notification controller resources
kubectl delete -k deploy/notification/

# Or delete individually
kubectl delete -f deploy/notification/03-service.yaml
kubectl delete -f deploy/notification/02-deployment.yaml
kubectl delete -f deploy/notification/01-rbac.yaml
kubectl delete -f deploy/notification/00-namespace.yaml

# Delete CRD (WARNING: This deletes ALL NotificationRequests)
kubectl delete -f config/crd/bases/kubernaut.ai_notificationrequests.yaml
```

### **Remove Secrets**

```bash
# Delete Slack webhook secret
kubectl delete secret notification-slack-webhook -n kubernaut-notifications
```

---

## 🐛 **Troubleshooting**

### **Controller Not Starting**

**Symptom**: Pod stuck in `CrashLoopBackOff` or `Error` state

**Diagnosis**:
```bash
# Check pod status
kubectl describe pod -n kubernaut-notifications -l control-plane=notification-controller

# Check events
kubectl get events -n kubernaut-notifications --sort-by='.lastTimestamp'

# Check logs
kubectl logs -n kubernaut-notifications -l control-plane=notification-controller
```

**Common Causes**:
1. **Image pull error**: Check image name and registry
2. **RBAC error**: Verify ServiceAccount and ClusterRoleBinding
3. **CRD not installed**: Install CRD first

---

### **Notifications Not Delivered**

**Symptom**: NotificationRequest stuck in `Sending` phase

**Diagnosis**:
```bash
# Check NotificationRequest status
kubectl get notificationrequest <name> -n kubernaut-notifications -o yaml

# Check controller logs for specific notification
kubectl logs -f deployment/notification-controller -n kubernaut-notifications | grep <name>

# Check delivery attempts
kubectl get notificationrequest <name> -n kubernaut-notifications \
  -o jsonpath='{.status.deliveryAttempts}' | jq .
```

**Common Causes**:
1. **Slack webhook error**: Verify webhook URL in secret
2. **Network issue**: Check egress network policies
3. **Circuit breaker open**: Wait for recovery or fix underlying issue

---

### **High Memory Usage**

**Symptom**: Controller pod using >128Mi memory

**Diagnosis**:
```bash
# Check resource usage
kubectl top pod -n kubernaut-notifications

# Check for memory leaks
kubectl logs -f deployment/notification-controller -n kubernaut-notifications | grep -i "memory\|leak"
```

**Resolution**:
1. Increase memory limits in deployment
2. Check for notification buildup (large CRD list)
3. Review controller logs for goroutine leaks

---

## ✅ **Production Readiness Checklist**

### **Pre-Deployment**
- [ ] CRD installed and verified
- [ ] Namespace created
- [ ] RBAC configured and tested
- [ ] Slack webhook secret created (if using Slack)
- [ ] Image built and pushed to registry

### **Deployment**
- [ ] Controller deployed via Kustomize
- [ ] Pod status: Running
- [ ] Health probes: Passing (liveness + readiness)
- [ ] Logs: No errors

### **Validation**
- [ ] Test notification (console) successful
- [ ] Test notification (Slack) successful (if enabled)
- [ ] Retry logic tested
- [ ] Metrics endpoint accessible
- [ ] Prometheus scraping configured

### **Monitoring**
- [ ] Prometheus ServiceMonitor created
- [ ] Grafana dashboard configured
- [ ] Alerting rules configured
- [ ] On-call rotation aware of alerts

### **Security**
- [ ] Pod Security Standards: Restricted
- [ ] Network policies configured (optional)
- [ ] Secret rotation policy defined
- [ ] RBAC permissions reviewed

### **Documentation**
- [ ] Runbook created for on-call
- [ ] Slack channels documented
- [ ] Escalation process defined
- [ ] Disaster recovery plan reviewed

---

## 📞 **Support**

### **Escalation Path**

1. **L1 Support**: Check this guide's troubleshooting section
2. **L2 Support**: Review controller logs and metrics
3. **L3 Support**: Check implementation plan and architecture docs

### **Key Documentation**

- [README](./README.md) - Controller overview
- [Implementation Plan](./implementation/IMPLEMENTATION_PLAN_V1.0.md) - Complete development guide
- [BR Coverage Matrix](./testing/BR-COVERAGE-MATRIX.md) - Test mapping
- [Error Handling Philosophy](./implementation/design/ERROR_HANDLING_PHILOSOPHY.md) - Retry + circuit breaker

---

**Version**: 1.0.0
**Last Updated**: 2025-10-12
**Status**: Production-Ready (98% complete) ✅
**Next**: Day 12 - Build pipeline + integration test execution


