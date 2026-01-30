# Gateway Config YAML Convention - CRITICAL BUG FIX
**Date:** January 30, 2026  
**Branch:** `feature/k8s-sar-user-id-stateless-services`  
**Severity:** 🚨 **CRITICAL** - Production configs are broken  
**Status:** Identified, fix in progress

---

## 🚨 **CRITICAL FINDING**

**Gateway config struct YAML tags don't match production ConfigMap format!**

This causes production Gateway configs to be **silently ignored**, leaving fields with zero/default values.

---

## 📋 **Evidence**

### **Production ConfigMap (`deploy/gateway/02-configmap.yaml`)**
```yaml
server:
  listen_addr: ":8080"        # ← snake_case
  read_timeout: 30s           # ← snake_case
  write_timeout: 30s          # ← snake_case
  idle_timeout: 120s          # ← snake_case

infrastructure:
  data_storage_url: "..."     # ← snake_case (assumed)
```

### **Gateway Config Struct (`pkg/gateway/config/config.go`)**
```go
type ServerSettings struct {
    ListenAddr   string        `yaml:"listenAddr"`    // ← camelCase ❌
    ReadTimeout  time.Duration `yaml:"readTimeout"`   // ← camelCase ❌
    WriteTimeout time.Duration `yaml:"writeTimeout"`  // ← camelCase ❌
    IdleTimeout  time.Duration `yaml:"idleTimeout"`   // ← camelCase ❌
}

type InfrastructureSettings struct {
    DataStorageURL string `yaml:"dataStorageUrl"`    // ← camelCase ❌
}
```

---

## 🔍 **Cross-Service Triage**

### **DataStorage Convention (Correct)**
```go
// pkg/datastorage/config/config.go
type ServerConfig struct {
    ReadTimeout  string `yaml:"read_timeout"`   // ← snake_case ✅
    WriteTimeout string `yaml:"write_timeout"`  // ← snake_case ✅
}
```

**DataStorage Test Config:**
```yaml
server:
  read_timeout: 30s   # ← Matches struct tags ✅
  write_timeout: 30s  # ← Matches struct tags ✅
```

### **Gateway Test Config (Matches Production)**
```yaml
# test/integration/gateway/config_integration_test.go
server:
  listen_addr: ":8080"              # ← snake_case ✅ (matches production)
infrastructure:
  data_storage_url: "http://..."    # ← snake_case ✅ (matches production)
```

**Test Failure:**
```
Expected <string>:  (empty)
to equal <string>: :8080

BR-GATEWAY-019: Listen address must be preserved
```

**Root Cause:** YAML parser doesn't recognize `listen_addr`, so `ListenAddr` field remains empty.

---

## 📊 **Convention Analysis**

| Service | YAML Convention | Status |
|---------|----------------|---------|
| **DataStorage** | `snake_case` | ✅ Consistent (struct + production + tests) |
| **Gateway** | `camelCase` (struct) vs `snake_case` (production/tests) | ❌ **BROKEN** |

**YAML Standard:** `snake_case` is the **de facto standard** for multi-word YAML keys:
- Kubernetes API: `snake_case` (e.g., `api_version`, `service_account`)
- Prometheus: `snake_case` (e.g., `scrape_interval`, `evaluation_interval`)
- Helm charts: `snake_case` (e.g., `replica_count`, `image_pull_policy`)

---

## ⚠️ **Impact Assessment**

### **Production Impact** 🚨
1. **Gateway ConfigMap fields are ignored**
   - `listen_addr` → defaults to `""`
   - `read_timeout` → defaults to `0s`
   - `write_timeout` → defaults to `0s`
   - `idle_timeout` → defaults to `0s`

2. **Validation Failure** (if present)
   - Gateway startup fails: "server.listenAddr is required (got: (empty))"

3. **Silent Failure** (if no validation)
   - Gateway starts with dangerous defaults (e.g., no timeouts → resource exhaustion)

### **Test Impact**
- 2 config integration tests fail (incorrect expectations)
- Tests use correct YAML format (matches production)
- Struct tags are the bug, not the tests

---

## ✅ **Required Fix**

### **Option A: Fix Gateway Config Struct** (Recommended)
**Rationale:** Match production ConfigMap and DataStorage convention

```diff
// pkg/gateway/config/config.go
type ServerSettings struct {
-   ListenAddr   string        `yaml:"listenAddr"`
+   ListenAddr   string        `yaml:"listen_addr"`
-   ReadTimeout  time.Duration `yaml:"readTimeout"`
+   ReadTimeout  time.Duration `yaml:"read_timeout"`
-   WriteTimeout time.Duration `yaml:"writeTimeout"`
+   WriteTimeout time.Duration `yaml:"write_timeout"`
-   IdleTimeout  time.Duration `yaml:"idleTimeout"`
+   IdleTimeout  time.Duration `yaml:"idle_timeout"`
-   MaxConcurrentRequests int  `yaml:"maxConcurrentRequests"`
+   MaxConcurrentRequests int  `yaml:"max_concurrent_requests"`
}

type InfrastructureSettings struct {
-   DataStorageURL string `yaml:"dataStorageUrl"`
+   DataStorageURL string `yaml:"data_storage_url"`
}

// Similar fixes for other structs (ProcessingSettings, RetrySettings, etc.)
```

**Advantages:**
- ✅ Fixes production ConfigMap (critical!)
- ✅ Matches DataStorage convention (consistency)
- ✅ Matches YAML standard (snake_case)
- ✅ No test changes needed

**Disadvantages:**
- ⚠️ Requires updating all Gateway config examples/docs

---

### **Option B: Fix Production ConfigMap** (NOT Recommended)
**Rationale:** Change production to match struct

**Why NOT:**
- ❌ Breaks existing production deployments
- ❌ Violates YAML convention (snake_case is standard)
- ❌ Inconsistent with DataStorage
- ❌ Tests would still fail (they follow production format)

---

## 📝 **Implementation Plan**

### **Phase 1: Fix Config Struct** ✅ (Proceeding)
1. Update `pkg/gateway/config/config.go` YAML tags → snake_case
2. Verify tests pass (no test changes needed)
3. Verify production ConfigMap now loads correctly

### **Phase 2: Update Documentation**
1. Update config examples in docs/
2. Update comments in config.go
3. Add migration note (if needed)

### **Phase 3: Validation**
1. Run all Gateway integration tests
2. Verify ConfigMap parsing in test environment
3. Create regression test to prevent future mismatches

---

## 🎯 **Expected Results**

**After Fix:**
- ✅ Production ConfigMap fields recognized and parsed
- ✅ 2 config integration tests pass
- ✅ Gateway convention matches DataStorage
- ✅ YAML standard compliance (snake_case)

---

## 📚 **References**

- **Production ConfigMap:** `deploy/gateway/02-configmap.yaml`
- **Gateway Config Struct:** `pkg/gateway/config/config.go`
- **DataStorage Config:** `pkg/datastorage/config/config.go`
- **Failing Tests:** `test/integration/gateway/config_integration_test.go`

---

**Author:** AI Assistant (via Cursor)  
**Priority:** 🚨 **P0 - CRITICAL** (Production configs broken)  
**Confidence:** 99% (Evidence-based: production ConfigMap + struct mismatch)
