# Webhook CLI Flag Architecture - Data Storage URL

**Date**: 2026-01-05  
**Status**: ✅ **APPROVED**  
**Related**: WEBHOOK_IMPLEMENTATION_PLAN.md (lines 114-196, 799-897, 1048-1089)  
**Pattern**: Follows Gateway/Notification/AI Analysis services

---

## 🎯 **Overview**

The authentication webhook service requires a CLI flag `--data-storage-url` to connect to the Data Storage service for writing audit events.

**Reason**: Webhooks must write complete audit events (WHO + WHAT + ACTION) per DD-WEBHOOK-003.

---

## 🔧 **CLI Flag Specification**

### **Required Flag**

```bash
--data-storage-url=<URL>
```

**Description**: Data Storage service URL for audit event writes  
**Required**: Yes (webhook crashes if missing per ADR-032)  
**Format**: HTTP/HTTPS URL  
**Example**: `http://datastorage-service:8080`

---

## 📋 **Usage Examples**

### **Production Deployment**

```bash
./webhooks-controller \
  --data-storage-url=http://datastorage-service:8080 \
  --webhook-port=9443 \
  --cert-dir=/tmp/k8s-webhook-server/serving-certs \
  --metrics-bind-address=:8080
```

### **Development (Integration Tests)**

```bash
./webhooks-controller \
  --data-storage-url=http://localhost:18099 \
  --webhook-port=9443
```

### **E2E Tests**

```bash
./webhooks-controller \
  --data-storage-url=http://localhost:28099 \
  --webhook-port=9443
```

---

## 🏗️ **Implementation Pattern**

### **main.go** (Following Gateway Service Pattern)

```go
package main

import (
    "flag"
    "fmt"
    "os"
    "time"
    
    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/pkg/webhooks"
    ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
    var dataStorageURL string
    
    // 1. Define CLI flag
    flag.StringVar(&dataStorageURL, "data-storage-url", "", 
        "Data Storage service URL for audit events (required)")
    flag.Parse()
    
    // 2. Validate required flag
    if dataStorageURL == "" {
        setupLog.Error(fmt.Errorf("data-storage-url is required"), 
            "missing required flag")
        os.Exit(1)  // Per ADR-032: Webhooks are P0
    }
    
    // 3. Initialize OpenAPI audit client
    // Per DD-API-001: Use OpenAPI generated client
    dsClient, err := audit.NewOpenAPIClientAdapter(
        dataStorageURL, 
        5*time.Second,
    )
    if err != nil {
        setupLog.Error(err, 
            "failed to create Data Storage client - audit is MANDATORY")
        os.Exit(1)
    }
    
    // 4. Create buffered audit store
    auditConfig := audit.RecommendedConfig("authwebhook")
    auditStore, err := audit.NewBufferedStore(
        dsClient, 
        auditConfig, 
        "authwebhook", 
        ctrl.Log.WithName("audit"),
    )
    if err != nil {
        setupLog.Error(err, "failed to create audit store")
        os.Exit(1)
    }
    
    setupLog.Info("Audit store initialized", 
        "data_storage_url", dataStorageURL, 
        "buffer_size", auditConfig.BufferSize)
    
    // 5. Pass audit store to webhook handlers
    wfeHandler := webhooks.NewWorkflowExecutionAuthHandler(auditStore)
    rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler(auditStore)
    nrHandler := webhooks.NewNotificationRequestDeleteHandler(auditStore)
    
    // ... register handlers with webhook server ...
}
```

**Reference**: `cmd/gateway/main.go` lines 86-103 (identical pattern)

---

## ☸️ **Kubernetes Configuration**

### **Option A: Hardcoded (Simple)**

```yaml
# deploy/webhooks/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut-auth-webhook
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: webhook
        image: kubernaut/auth-webhook:latest
        args:
        - --webhook-port=9443
        - --metrics-bind-address=:8080
        - --cert-dir=/tmp/k8s-webhook-server/serving-certs
        - --data-storage-url=http://datastorage-service:8080  # Hardcoded
```

**Pros**: ✅ Simple, ✅ No extra resources  
**Cons**: ⚠️ Less flexible, ⚠️ Requires redeployment to change

---

### **Option B: ConfigMap (Recommended)**

```yaml
# config/base/webhook-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: webhook-config
  namespace: kubernaut-system
data:
  data-storage-url: "http://datastorage-service:8080"
---
# deploy/webhooks/deployment.yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: webhook
        args:
        - --data-storage-url=$(DATA_STORAGE_URL)
        env:
        - name: DATA_STORAGE_URL
          valueFrom:
            configMapKeyRef:
              name: webhook-config
              key: data-storage-url
```

**Pros**: ✅ Flexible (no redeployment), ✅ Environment-specific  
**Cons**: ⚠️ Extra resource

---

### **Option C: Secret (For URLs with Auth)**

```yaml
# config/base/webhook-secrets.yaml
apiVersion: v1
kind: Secret
metadata:
  name: webhook-secrets
  namespace: kubernaut-system
stringData:
  data-storage-url: "https://datastorage.prod.internal:8443?token=xyz"
---
# deploy/webhooks/deployment.yaml
env:
- name: DATA_STORAGE_URL
  valueFrom:
    secretKeyRef:
      name: webhook-secrets
      key: data-storage-url
```

**Pros**: ✅ Secure for sensitive URLs  
**Cons**: ⚠️ More complex

---

## 🌐 **Service Discovery**

### **Production** (K8s Service Name)

```
--data-storage-url=http://datastorage-service:8080
```

**Resolution**: Kubernetes DNS (`datastorage-service.kubernaut-system.svc.cluster.local`)

### **Development** (Localhost)

```
--data-storage-url=http://localhost:18099  # Integration tests
--data-storage-url=http://localhost:28099  # E2E tests
```

**Resolution**: Podman containers expose ports to localhost

---

## 🔐 **Security Considerations**

### **Q: Why No Authentication?**

**A**: Data Storage trusts internal pod-to-pod traffic

**Network Policy**: Restricts Data Storage access to authorized pods only

```yaml
# network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: datastorage-allow-webhooks
spec:
  podSelector:
    matchLabels:
      app: datastorage
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: auth-webhook  # Only webhook pod can access
```

### **Q: Future mTLS?**

**A**: If Data Storage adds mTLS, add `--data-storage-cert` flag:

```bash
--data-storage-url=https://datastorage-service:8443
--data-storage-cert=/etc/tls/ca.crt
```

---

## 📊 **Consistency with Other Services**

| Service | Flag Name | Pattern | Reference |
|---------|-----------|---------|-----------|
| **Gateway** | `--data-storage-url` | OpenAPI client + BufferedStore | `cmd/gateway/main.go:86-103` |
| **Notification** | `--datastorage-url` | OpenAPI client + BufferedStore | `cmd/notification/main.go` |
| **AI Analysis** | `--data-storage-url` | OpenAPI client + BufferedStore | `cmd/aianalysis/main.go` |
| **Webhook** (NEW) | `--data-storage-url` | OpenAPI client + BufferedStore | `cmd/webhooks/main.go` |

**Result**: ✅ Consistent pattern across all services

---

## 🚀 **Benefits**

| Aspect | Benefit |
|--------|---------|
| **Simplicity** | Single URL flag (no complex auth config) |
| **Consistency** | Same pattern as Gateway/Notification/AI Analysis |
| **Flexibility** | Easy to point to different Data Storage instances |
| **Type Safety** | OpenAPI client catches schema mismatches |
| **Performance** | BufferedStore enables async writes |
| **Reliability** | Crashes if audit unavailable (P0 service per ADR-032) |

---

## ✅ **Validation Checklist**

Before deploying webhook service:

- [ ] `--data-storage-url` flag configured in deployment YAML
- [ ] Data Storage service accessible from webhook pod
- [ ] Network policy allows webhook → Data Storage traffic
- [ ] Audit events visible in Data Storage API (`GET /audit/events`)
- [ ] Webhook logs show "Audit store initialized" message
- [ ] Integration tests pass with real Data Storage service

---

## 📚 **References**

- **DD-WEBHOOK-003**: Webhook-Complete Audit Pattern (audit events mandatory)
- **DD-API-001**: OpenAPI Client Usage Mandate (type-safe clients)
- **ADR-032**: Audit Requirements for P0 Services (crash if unavailable)
- **Gateway Implementation**: `cmd/gateway/main.go` lines 86-103
- **WEBHOOK_IMPLEMENTATION_PLAN.md**: Lines 114-196, 799-897

---

**Last Updated**: 2026-01-05  
**Status**: Ready for implementation  
**Priority**: HIGH (required for webhook service deployment)  

