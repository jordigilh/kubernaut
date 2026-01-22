# Webhook CLI Flag Architecture - Data Storage URL

**Date**: 2026-01-05
**Status**: ✅ **APPROVED**
**Related**: WEBHOOK_IMPLEMENTATION_PLAN.md (lines 114-196, 799-897, 1048-1089)
**Pattern**: Follows Gateway/Notification/AI Analysis services

---

## 🎯 **Overview**

The authentication webhook service uses a CLI flag `--data-storage-url` to connect to the Data Storage service for writing audit events.

**Reason**: Webhooks must write complete audit events (WHO + WHAT + ACTION) per DD-WEBHOOK-003.

### **Design Principle: Sensible Defaults**

✅ **Default**: `http://datastorage-service:8080` (K8s service name - works in production)
✅ **Override**: Via environment variable or CLI flag (for dev/staging/test)
✅ **Priority**: CLI flag > env var > default

**Result**: Zero configuration needed for standard production deployments.

---

## 🔧 **CLI Flag Specification**

### **All Webhook Service Flags**

| Flag | Default | Env Variable | Purpose |
|------|---------|--------------|---------|
| `--webhook-port` | `9443` | `WEBHOOK_PORT` | Webhook HTTPS port (standard K8s webhook port) |
| `--data-storage-url` | `http://datastorage-service:8080` | `WEBHOOK_DATA_STORAGE_URL` | Audit event API endpoint |
| `--cert-dir` | `/tmp/k8s-webhook-server/serving-certs` | `WEBHOOK_CERT_DIR` | TLS certificate location |

**Note**: ❌ **No metrics flag** - Audit traces capture 100% of operations; Kubernetes API server already exposes webhook metrics.

**Authority**: DD-WEBHOOK-001 (Service Configuration section), WEBHOOK_METRICS_TRIAGE.md

### **Primary Flag: Data Storage URL**

```bash
--data-storage-url=<URL>
```

**Description**: Data Storage service URL for audit event writes
**Default**: `http://datastorage-service:8080` (K8s service name)
**Required**: No (uses default if not specified)
**Format**: HTTP/HTTPS URL
**Override**: Via environment variable `WEBHOOK_DATA_STORAGE_URL` or CLI flag

**Rationale**: Default to K8s service name for production, override for dev/test environments

### **Webhook Port (Standard K8s Convention)**

```bash
--webhook-port=<PORT>
```

**Description**: HTTPS port for webhook admission endpoint
**Default**: `9443` (standard Kubernetes webhook port)
**Required**: No (uses default if not specified)
**Format**: Integer port number
**Override**: Via environment variable `WEBHOOK_PORT` or CLI flag

**Rationale**: Port 9443 is the de facto standard for K8s admission webhooks (cert-manager, OPA, Istio)

---

## 📋 **Usage Examples**

### **Priority Order**

1. **CLI flag** (highest priority)
2. **Environment variable** `WEBHOOK_DATA_STORAGE_URL`
3. **Default value** `http://datastorage-service:8080`

### **Production Deployment** (Uses All Defaults)

```bash
# All defaults work - no flags needed!
./webhooks-controller

# Equivalent to:
./webhooks-controller \
  --webhook-port=9443 \
  --data-storage-url=http://datastorage-service:8080 \
  --metrics-bind-address=:8080 \
  --cert-dir=/tmp/k8s-webhook-server/serving-certs
```

### **Development** (Override via Env Var)

```bash
# Override via environment variable
export WEBHOOK_DATA_STORAGE_URL=http://localhost:18099
./webhooks-controller --webhook-port=9443
```

### **Development** (Override via CLI Flag)

```bash
# Override Data Storage URL only (webhook port uses default 9443)
./webhooks-controller \
  --data-storage-url=http://localhost:18099
```

### **E2E Tests** (Override via Env Var)

```bash
export WEBHOOK_DATA_STORAGE_URL=http://localhost:28099
./webhooks-controller
# Webhook port 9443 uses default
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
    "github.com/jordigilh/kubernaut/pkg/authwebhook"
    ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
    var dataStorageURL string
    var webhookPort int
    var certDir string

    // 1. Define CLI flags with production defaults
    flag.StringVar(&dataStorageURL, "data-storage-url",
        "http://datastorage-service:8080",  // DEFAULT
        "Data Storage service URL for audit events")
    flag.IntVar(&webhookPort, "webhook-port",
        9443,  // DEFAULT (standard K8s webhook port)
        "Webhook HTTPS port")
    flag.StringVar(&certDir, "cert-dir",
        "/tmp/k8s-webhook-server/serving-certs",  // DEFAULT
        "TLS certificate directory")
    flag.Parse()

    // 2. Allow environment variable overrides (higher priority than defaults)
    if envURL := os.Getenv("WEBHOOK_DATA_STORAGE_URL"); envURL != "" {
        dataStorageURL = envURL
    }
    if envPort := os.Getenv("WEBHOOK_PORT"); envPort != "" {
        if port, err := strconv.Atoi(envPort); err == nil {
            webhookPort = port
        }
    }

    setupLog.Info("Webhook configuration",
        "webhook_port", webhookPort,
        "data_storage_url", dataStorageURL,
        "cert_dir", certDir)
    // No metrics configuration - audit traces sufficient

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

### **Option A: Use All Defaults (Recommended for Production)**

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
        # No args needed - all defaults work!
        # Defaults:
        #   --webhook-port=9443
        #   --data-storage-url=http://datastorage-service:8080
        #   --cert-dir=/tmp/k8s-webhook-server/serving-certs
        ports:
        - containerPort: 9443
          name: webhook
        # No metrics port - audit traces sufficient
```

**Pros**: ✅ Zero configuration, ✅ Production-ready, ✅ Simplest possible deployment, ✅ No metrics overhead
**Cons**: None for standard Kubernetes environments

---

### **Option B: Override via Environment Variable**

```yaml
# deploy/webhooks/deployment.yaml - Staging environment
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: webhook
        image: kubernaut/auth-webhook:latest
        env:
        - name: WEBHOOK_DATA_STORAGE_URL
          value: "http://datastorage-staging:8080"  # Override default
        args:
        - --webhook-port=9443
        - --metrics-bind-address=:8080
```

**Pros**: ✅ Simple override, ✅ No CLI flag needed
**Cons**: ⚠️ Hardcoded in deployment YAML

---

### **Option C: ConfigMap (Recommended for Multi-Environment)**

```yaml
# config/staging/webhook-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: webhook-config
  namespace: kubernaut-system
data:
  data-storage-url: "http://datastorage-staging:8080"
---
# deploy/webhooks/deployment.yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: webhook
        env:
        - name: WEBHOOK_DATA_STORAGE_URL
          valueFrom:
            configMapKeyRef:
              name: webhook-config
              key: data-storage-url
              optional: true  # Falls back to default if ConfigMap missing
        args:
        - --webhook-port=9443
```

**Pros**: ✅ Flexible per environment, ✅ No redeployment, ✅ Falls back to default
**Cons**: ⚠️ Extra resource (ConfigMap)

---

### **Option D: CLI Flag Override (Least Common)**

```yaml
# deploy/webhooks/deployment.yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: webhook
        args:
        - --webhook-port=9443
        - --data-storage-url=http://datastorage-custom:8080  # Explicit override
```

**Pros**: ✅ Explicit, ✅ No env var needed
**Cons**: ⚠️ Requires redeployment to change

---

## 🌐 **Service Discovery**

### **Production** (Uses Default)

**Default**: `http://datastorage-service:8080`
**Resolution**: Kubernetes DNS (`datastorage-service.kubernaut-system.svc.cluster.local`)
**Configuration**: None needed - works out of the box

```yaml
# No configuration needed - uses default
containers:
- name: webhook
  args:
  - --webhook-port=9443
```

### **Staging** (Override via Env Var)

**Override**: `http://datastorage-staging:8080`
**Configuration**: Environment variable

```yaml
env:
- name: WEBHOOK_DATA_STORAGE_URL
  value: "http://datastorage-staging:8080"
```

### **Development** (Override for Localhost)

**Override**: `http://localhost:18099` (integration) or `http://localhost:28099` (E2E)
**Resolution**: Podman containers expose ports to localhost
**Configuration**: Environment variable

```bash
# Integration tests
export WEBHOOK_DATA_STORAGE_URL=http://localhost:18099
./webhooks-controller

# E2E tests
export WEBHOOK_DATA_STORAGE_URL=http://localhost:28099
./webhooks-controller
```

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
| **Webhook** (NEW) | `--data-storage-url` | OpenAPI client + BufferedStore | `cmd/authwebhook/main.go` |

**Result**: ✅ Consistent pattern across all services

---

## 🚀 **Benefits**

| Aspect | Benefit |
|--------|---------|
| **Zero Config Production** | Works out-of-box with sensible K8s service default |
| **Flexible Override** | 3-tier priority: CLI flag > env var > default |
| **Simplicity** | Single URL (no complex auth config) |
| **Consistency** | Same pattern as Gateway/Notification/AI Analysis |
| **Environment-Specific** | Easy ConfigMap per environment (dev/staging/prod) |
| **Type Safety** | OpenAPI client catches schema mismatches |
| **Performance** | BufferedStore enables async writes |
| **Reliability** | Fails fast if Data Storage unavailable (P0 service) |

---

## ✅ **Validation Checklist**

### **Production Deployment**

- [ ] Data Storage service running (`datastorage-service:8080`)
- [ ] Network policy allows webhook → Data Storage traffic
- [ ] Webhook logs show "Using Data Storage URL: http://datastorage-service:8080"
- [ ] Webhook logs show "Audit store initialized"
- [ ] Audit events visible in Data Storage API (`GET /audit/events`)
- [ ] No configuration needed (uses default)

### **Non-Production Environments**

- [ ] `WEBHOOK_DATA_STORAGE_URL` env var set (if override needed)
- [ ] OR `--data-storage-url` CLI flag specified
- [ ] Webhook logs show correct URL being used
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

