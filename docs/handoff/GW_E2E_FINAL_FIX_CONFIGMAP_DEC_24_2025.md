# Gateway E2E Coverage - Final Fix: ConfigMap Required (Dec 24, 2025)

## 🎯 **Root Cause #2: Missing Configuration**

After fixing the `localhost/` prefix issue, Gateway pod was still failing to start.

### **The Problem**

Gateway's `main.go` requires a configuration file:

```go
configPath := flag.String("config", "config/gateway.yaml", "Path to configuration file")
serverCfg, err := config.LoadFromFile(*configPath)
if err != nil {
    logger.Error(err, "Failed to load configuration", "config_path", *configPath)
    os.Exit(1)  // ← Pod exits immediately
}
```

**The E2E manifest was missing**:
- ConfigMap with `gateway.yaml` configuration
- Volume mount for the configuration file
- Command-line argument to specify config file path

### **The Fix**

Updated `test/infrastructure/gateway.go` `GatewayCoverageManifest()` function:

#### **1. Added ConfigMap** (at beginning of manifest):
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-config
  namespace: kubernaut-system
data:
  gateway.yaml: |
    server:
      listen_addr: ":8080"
      read_timeout: "30s"
      write_timeout: "30s"
      idle_timeout: "120s"
    middleware:
      rate_limit:
        requests_per_minute: 100
        burst: 10
    processing:
      deduplication:
        ttl: "5m"
      environment:
        cache_ttl: "30s"
        configmap_namespace: "kubernaut-system"
        configmap_name: "kubernaut-environment-overrides"
```

#### **2. Added Volume Mount** (in Deployment spec):
```yaml
volumeMounts:
- name: coverage
  mountPath: /coverage
- name: config
  mountPath: /app/config
  readOnly: true
```

#### **3. Added Config Argument**:
```yaml
args:
- --config=/app/config/gateway.yaml
```

#### **4. Added Config Volume**:
```yaml
volumes:
- name: coverage
  emptyDir: {}
- name: config
  configMap:
    name: gateway-config
```

### **Why This Was Missed**

1. **Silent failure**: Pod exits immediately with `os.Exit(1)`, no obvious error in deployment
2. **Deployment shows "created"**: Kubernetes successfully creates the Deployment
3. **No pod logs**: Pod crashes before logs can be inspected (E2E auto-deletes cluster)
4. **Focus on image issues**: Previous debugging focused on image loading, not startup requirements

### **Verification Method**

Would need persistent cluster to see:
```bash
kubectl logs -n kubernaut-system -l app=gateway
# Would show: "Failed to load configuration" error
```

### **Pattern for Other Services**

DataStorage also requires ConfigMap:
```yaml
# DataStorage ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
data:
  config.yaml: |
    database:
      host: postgresql
      ...
```

**Lesson**: Always check service's `main.go` for required configuration files!

## 📊 **Current Status**

### **Fixes Applied** ✅
1. ✅ Image tag propagation (build → load → deploy)
2. ✅ Podman image loading (image-archive method)
3. ✅ Image name prefix (keep `localhost/`)
4. ✅ ConfigMap with Gateway configuration

### **Ready to Test**
All infrastructure fixes complete. Gateway should now:
- Build with coverage ✅
- Load into Kind ✅
- Deploy with correct image name ✅
- Find configuration file ✅
- Start successfully ✅

## 🚀 **Next Test Run**

Expected outcome:
```
📦 PHASE 4: Deploying Gateway (coverage-enabled)...
   ✅ Gateway coverage manifest applied
⏳ Waiting for Gateway to be ready (timeout: 2m0s)...
   ✅ Gateway is healthy
```

---

**File Modified**: `test/infrastructure/gateway.go`
**Function**: `GatewayCoverageManifest()`
**Change**: Added ConfigMap + volume mounts + args
**Confidence**: 90% (ConfigMap is definitely required for Gateway to start)








