# REQUEST: DataStorage Config File Mount Fix (Blocking BR-SP-090)

**From**: SignalProcessing Team
**To**: DataStorage Team
**Date**: 2025-12-11
**Priority**: 🔴 **HIGH** - Blocking V1.0 audit testing
**Type**: Bug Report & Fix Request

---

## 📋 **Summary**

DataStorage container is crash-looping on startup due to missing config file at `/app/config.yaml`. This blocks **ALL audit testing** (integration + E2E) across multiple services.

**Impact**:
- ❌ BR-SP-090 (SignalProcessing audit trail) - E2E test blocked
- ❌ SP integration tests - failing in BeforeSuite
- ❌ RO audit tests - likely affected
- ❌ Gateway audit tests - likely affected
- ❌ Any service depending on DataStorage for audit

---

## 🔴 **Root Cause**

**DataStorage Container Error**:
```
2025-12-12T00:44:55.539Z ERROR datastorage datastorage/main.go:78
Failed to load configuration file (ADR-030)
  config_path: /app/config.yaml
  error: "failed to read config file: open /app/config.yaml: no such file or directory"
```

**Container**: `sp-datastorage-integration`
**Expected Path**: `/app/config.yaml`
**Actual**: File not mounted/missing

---

## 🔍 **Evidence & Reproduction**

### **Integration Test Failure**
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v -timeout=10m ./test/integration/signalprocessing/...

# Result:
# [FAILED] Timed out after 30.001s.
# Data Storage should become healthy
# Expected <bool>: false to be true
```

### **Container Logs**
```bash
podman logs sp-datastorage-integration

# Output:
# INFO Loading configuration from YAML file (ADR-030) {"config_path": "/app/config.yaml"}
# ERROR Failed to load configuration file (ADR-030)
#   error: "open /app/config.yaml: no such file or directory"
```

### **Container Status**
```bash
podman ps -a | grep datastorage
# sp-datastorage-integration - Exited (1) - keeps restarting
```

---

## 🎯 **Locations to Check**

### **1. Integration Test Infrastructure**
**File**: `test/integration/signalprocessing/helpers_infrastructure.go`
**Function**: `startDataStorageContainer()` (around line 250-350)
**Issue**: Config file volume mount likely missing or incorrect path

**Expected**:
```go
// Config volume mount should be:
"-v", configPath + ":/app/config.yaml:ro,z",
// OR
"-v", configDir + ":/etc/datastorage:ro,z",
```

**Check**:
- Is config file being created/written before container start?
- Is volume mount path correct (`/app/config.yaml` vs `/etc/datastorage/config.yaml`)?
- Is podman volume mount syntax correct (`:ro,z` flags)?

---

### **2. E2E Test Infrastructure**
**File**: `test/infrastructure/datastorage.go`
**Function**: `deployDataStorageServiceInNamespace()` (around line 544-700)
**Issue**: ConfigMap mount in Kubernetes Deployment manifest

**Expected**:
```yaml
volumes:
  - name: config
    configMap:
      name: datastorage-config
volumeMounts:
  - name: config
    mountPath: /app/config.yaml    # OR /etc/datastorage/config.yaml
    subPath: config.yaml
    readOnly: true
```

**Check**:
- Is ConfigMap being created with correct name (`datastorage-config`)?
- Is volume mount path matching what DataStorage main.go expects?
- Is `subPath` specified (required for file-level mounts)?

---

### **3. DataStorage Main.go**
**File**: `cmd/datastorage/main.go:72-78`
**Current Code**:
```go
log.Info("Loading configuration from YAML file (ADR-030)", "config_path", "/app/config.yaml")
cfg, err := config.LoadFromYAML("/app/config.yaml")
if err != nil {
    log.Error(err, "Failed to load configuration file (ADR-030)", "config_path", "/app/config.yaml")
    os.Exit(1)  // <-- CRASH HERE
}
```

**Question**: Was the expected config path recently changed from `/etc/datastorage/config.yaml` to `/app/config.yaml`?

**Check Git History**:
```bash
git log --oneline --all -20 -- cmd/datastorage/main.go
git diff HEAD~5:cmd/datastorage/main.go HEAD:cmd/datastorage/main.go | grep config
```

---

## 📊 **Timeline Context**

| Event | Date | Details |
|-------|------|---------|
| **Embedding Removal** | 2025-12-11 | Major refactor removing `Query`, `Embedding`, `MinSimilarity` fields |
| **DS Build Fixed** | 2025-12-11 | DS team fixed compilation errors from embedding removal |
| **Config Issue Discovered** | 2025-12-11 | During BR-SP-090 audit testing |
| **First Failed Test** | 2025-12-11 19:45 | Integration test: DataStorage health check timeout |

**Hypothesis**: Config file path or mount logic changed during embedding removal refactor but test infrastructure wasn't updated.

---

## 🔧 **Recommended Fix**

### **Option A: Update Test Infrastructure** ⭐ **RECOMMENDED**
If main.go is correct and test infrastructure is outdated:

**Integration Tests** (`helpers_infrastructure.go`):
```go
// Create config file
configPath := filepath.Join(tmpDir, "config.yaml")
configContent := fmt.Sprintf(`service:
  name: data-storage
  port: %d
database:
  host: localhost
  port: %d
  name: kubernaut_audit
  // ... rest of config
`, apiPort, pgPort)
err := os.WriteFile(configPath, []byte(configContent), 0644)

// Mount config file
cmd := exec.Command("podman", "run", "-d",
    "--name", containerName,
    "-v", configPath+":/app/config.yaml:ro,z",  // <-- CRITICAL: Match main.go path
    // ... rest of flags
)
```

**E2E Tests** (`datastorage.go` - deployDataStorageServiceInNamespace):
```yaml
# In Deployment spec:
volumes:
  - name: config
    configMap:
      name: datastorage-config
containers:
  - name: datastorage
    volumeMounts:
      - name: config
        mountPath: /app/config.yaml   # <-- CRITICAL: Match main.go path
        subPath: config.yaml
        readOnly: true
```

---

### **Option B: Revert main.go Path**
If `/etc/datastorage/config.yaml` was the original path:

```go
// cmd/datastorage/main.go
configPath := "/etc/datastorage/config.yaml"  // Revert to original
log.Info("Loading configuration from YAML file (ADR-030)", "config_path", configPath)
cfg, err := config.LoadFromYAML(configPath)
```

---

## ✅ **Verification Steps**

After fix is applied:

### **1. Integration Tests**
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v -timeout=10m ./test/integration/signalprocessing/...

# Expected:
# ✅ PostgreSQL ready (port: XXXXX)
# ✅ Data Storage ready (port: XXXXX)
# [PASSED] 71/71 tests
```

### **2. Container Health Check**
```bash
# After running integration test:
podman ps | grep datastorage
# Should show: Up X minutes (healthy)

podman logs sp-datastorage-integration | tail -10
# Should show: "Starting HTTP server" - NO errors
```

### **3. E2E Tests**
```bash
make test-e2e-signalprocessing

# Expected:
# ✅ DataStorage infrastructure ready for BR-SP-090 audit testing
# [PASSED] 11/11 tests (including BR-SP-090)
```

---

## 📚 **Related Documents**

- **Triage Assessment**: `docs/handoff/TRIAGE_ASSESSMENT_SP_E2E_BR-SP-090.md`
- **Integration Test Fix**: `docs/handoff/FIX_SP_INTEGRATION_TEST_AUDIT_BUG.md`
- **API Impact Doc**: `docs/handoff/API_IMPACT_REMOVE_EMBEDDINGS.md`
- **DS Integration Tests**: `docs/handoff/TRIAGE_DS_INTEGRATION_TESTS_EMBEDDING_REFS.md`

---

## 🎯 **Success Criteria**

DataStorage config issue is resolved when:
1. ✅ DataStorage container starts without errors
2. ✅ Health check endpoint returns 200 OK within 10 seconds
3. ✅ Integration tests: BeforeSuite passes (DataStorage ready)
4. ✅ E2E tests: BR-SP-090 can query audit API successfully
5. ✅ Container logs show: `"Starting HTTP server"` (no config errors)

---

## 💡 **Additional Context**

### **Why This Blocks BR-SP-090**
BR-SP-090 validates that SignalProcessing writes audit events to DataStorage:
1. SP controller calls `RecordSignalProcessed()` → BufferedStore
2. BufferedStore flushes to DataStorage HTTP API
3. DataStorage writes to PostgreSQL `audit_events` table
4. Test queries DataStorage API to verify events persisted

**Current State**:
- ✅ Step 1: SP audit code works (unit tests pass)
- ❌ Step 2: BufferedStore cannot reach DataStorage (container crashed)
- ❌ Step 3: No DataStorage = no PostgreSQL writes
- ❌ Step 4: Test queries return empty (DataStorage not running)

### **Not an Audit Code Issue**
The audit implementation is correct:
- ✅ SP controller has AuditClient wired up
- ✅ RecordSignalProcessed() is called on completion
- ✅ RecordClassificationDecision() is called
- ✅ BufferedStore uses 1-second FlushInterval
- ✅ Integration test proves audit pipeline works (when DataStorage is healthy)

**Problem**: DataStorage deployment configuration, not audit logic.

---

## 📞 **Contact**

**Reporter**: SignalProcessing Team
**Date Discovered**: 2025-12-11
**Blocking**: BR-SP-090 (V1.0 requirement)
**Urgency**: HIGH - Audit is V1.0 critical per user requirement

**Questions?** Check container logs:
```bash
podman logs sp-datastorage-integration 2>&1 | grep -A 5 -B 5 config
```

---

**Document Status**: 🔴 **ACTIVE REQUEST**
**Created**: 2025-12-11
**Priority**: HIGH (Blocking V1.0)
**Assignee**: DataStorage Team
**Blocked Items**: BR-SP-090 E2E test, SP integration tests, cross-service audit testing

