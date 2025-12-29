# DataStorage Team: Gateway E2E DataStorage Deployment Issue

**Date**: December 13, 2025
**From**: Gateway Team
**To**: DataStorage Team
**Priority**: 🔴 **BLOCKING** - Gateway E2E tests cannot run
**Status**: ⏸️ AWAITING DS TEAM GUIDANCE

---

## 📋 TL;DR

Gateway E2E tests are using a **simplified inline YAML** to deploy DataStorage, but the pod times out and never becomes ready. AIAnalysis E2E successfully deploys DataStorage using a more complete approach. **We need DS team guidance on the correct deployment pattern for E2E tests.**

---

## 🔴 Current Problem

**What**: DataStorage pod times out during readiness check in Gateway E2E tests
**Where**: `test/infrastructure/gateway_e2e.go:232` (`deployDataStorageToCluster` function)
**Error**: `error: timed out waiting for the condition on pods/datastorage-7bbb549d8f-2xqrf`
**Timeout**: 120 seconds

---

## 🔍 Root Cause Analysis

### Gateway's Current Approach (FAILING ❌)

**File**: `test/infrastructure/gateway_e2e.go:232-310`

**What it does**:
```go
// Simplified inline YAML deployment
deploymentYAML := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: datastorage
  template:
    metadata:
      labels:
        app: datastorage
    spec:
      containers:
      - name: datastorage
        image: datastorage:e2e-test  // ❌ Image doesn't exist!
        ports:
        - containerPort: 8080
        env:
        - name: POSTGRES_HOST
          value: postgres
        - name: POSTGRES_PORT
          value: "5432"
        - name: POSTGRES_USER
          value: testuser
        - name: POSTGRES_PASSWORD
          value: testpass
        - name: POSTGRES_DB
          value: testdb
        - name: REDIS_ADDR
          value: redis:6379
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  namespace: %s
spec:
  type: NodePort
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: %d
  selector:
    app: datastorage
`, namespace, namespace, GatewayDataStoragePort)
```

**What's MISSING**:
1. ❌ **No database migrations** (critical tables don't exist!)
2. ❌ **No image building/loading** (assumes `datastorage:e2e-test` exists)
3. ❌ **No ConfigMap** (DataStorage needs `/etc/datastorage/config.yaml`)
4. ❌ **No Secret** (DB/Redis credentials)
5. ❌ **No volume mounts** (can't access config/secrets)
6. ❌ **No readiness/liveness probes**
7. ❌ **Wrong environment variable format** (DataStorage uses config file, not env vars)

---

### AIAnalysis's Approach (WORKING ✅)

**File**: `test/infrastructure/aianalysis.go:421-580`

**What it does**:
```go
func deployDataStorage(clusterName, kubeconfigPath string, writer io.Writer) error {
    // 1. ✅ Apply database migrations FIRST
    fmt.Fprintln(writer, "  📋 Applying database migrations (shared library)...")
    ctx := context.Background()
    config := DefaultMigrationConfig("kubernaut-system", kubeconfigPath)
    config.Tables = []string{"audit_events", "remediation_workflow_catalog"}
    if err := ApplyMigrationsWithConfig(ctx, config, writer); err != nil {
        fmt.Fprintf(writer, "  ⚠️  Migration warning (may already be applied): %v\n", err)
    }

    // 2. ✅ Build DataStorage image
    fmt.Fprintln(writer, "  Building Data Storage image...")
    buildCmd := exec.Command("podman", "build", "-t", "kubernaut-datastorage:latest",
        "-f", "docker/data-storage.Dockerfile", ".")
    buildCmd.Dir = projectRoot
    if err := buildCmd.Run(); err != nil {
        return fmt.Errorf("failed to build Data Storage with podman: %w", err)
    }

    // 3. ✅ Load image into Kind
    fmt.Fprintln(writer, "  Loading Data Storage image into Kind...")
    if err := loadImageToKind(clusterName, "kubernaut-datastorage:latest", writer); err != nil {
        return fmt.Errorf("failed to load image: %w", err)
    }

    // 4. ✅ Deploy complete manifest with ConfigMap, Secret, Deployment, Service
    manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: kubernaut-system
data:
  config.yaml: |
    shutdownTimeout: 30s
    server:
      port: 8080
      host: "0.0.0.0"
      read_timeout: 30s
      write_timeout: 30s
    database:
      host: postgresql
      port: 5432
      name: action_history
      user: slm_user
      ssl_mode: disable
      max_open_conns: 25
      max_idle_conns: 5
      conn_max_lifetime: 5m
      conn_max_idle_time: 10m
      secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
      usernameKey: "username"
      passwordKey: "password"
    redis:
      addr: redis:6379
      db: 0
      dlq_stream_name: dlq-stream
      dlq_max_len: 1000
      dlq_consumer_group: dlq-group
      secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
      passwordKey: "password"
    logging:
      level: debug
      format: json
---
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secret
  namespace: kubernaut-system
stringData:
  db-secrets.yaml: |
    username: slm_user
    password: test_password
  redis-secrets.yaml: |
    password: ""
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: datastorage
  template:
    metadata:
      labels:
        app: datastorage
    spec:
      containers:
      - name: datastorage
        image: kubernaut-datastorage:latest  // ✅ Proper image tag
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        args:
        - "--config=/etc/datastorage/config.yaml"  // ✅ Config file
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secret
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  type: NodePort
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30081  // ✅ Valid NodePort range
    name: http
  selector:
    app: datastorage
`

    // 5. ✅ Apply and wait for readiness
    // ... (kubectl apply + wait logic)
}
```

**Why it WORKS**:
1. ✅ **Database migrations applied first** → critical tables exist
2. ✅ **Image built and loaded** → container can start
3. ✅ **ConfigMap provided** → DataStorage reads config correctly
4. ✅ **Secret provided** → DB/Redis authentication works
5. ✅ **Volume mounts configured** → config/secrets accessible
6. ✅ **Readiness/liveness probes** → K8s knows when pod is ready
7. ✅ **Proper args** → DataStorage starts with `--config` flag

---

## 🎯 Questions for DS Team

### Question 1: Shared Deployment Function?
**Q**: Should Gateway use the existing `deployDataStorage` function from `aianalysis.go` instead of reinventing the wheel?

**Current situation**:
- AIAnalysis: Uses `deployDataStorage(clusterName, kubeconfigPath, writer)` ✅
- Gateway: Custom inline YAML ❌
- SignalProcessing: Uses similar pattern to AIAnalysis ✅
- Notification: Uses `deployDataStorageServiceForNotification` ⚠️ (custom)

**Proposed**:
```go
// In test/infrastructure/gateway_e2e.go

// Replace deployDataStorageToCluster with:
func deployDataStorageToCluster(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    // Use AIAnalysis's proven deployment pattern
    return deployDataStorage(clusterName, kubeconfigPath, writer)
}
```

**Benefits**:
- Eliminates code duplication
- Ensures consistent DataStorage deployment across all services
- Automatically includes migrations, proper config, secrets

**Concerns**:
- Is `deployDataStorage` specific to AIAnalysis needs?
- Should it be moved to `datastorage.go` as a shared function?

---

### Question 2: Minimal DataStorage Configuration?
**Q**: What's the **minimal viable configuration** for DataStorage in E2E tests?

Gateway only needs DataStorage for:
- ✅ Audit event storage (Gateway writes audit events)
- ❌ NO workflow catalog needed
- ❌ NO DLQ processing needed

**Can we simplify**:
1. Which database tables are **required** vs optional?
   - `audit_events`: Required ✅
   - `remediation_workflow_catalog`: Required? ⚠️
   - Others?

2. Which config sections are **required** vs optional?
   - `server`: Required ✅
   - `database`: Required ✅
   - `redis`: Required? ⚠️ (Gateway uses it for deduplication)
   - `logging`: Required ✅
   - `dlq`: Required? ⚠️

---

### Question 3: DataStorage Image Naming?
**Q**: Should all E2E tests use the same DataStorage image tag?

**Current**:
- AIAnalysis: `kubernaut-datastorage:latest`
- Gateway (broken): `datastorage:e2e-test`
- SignalProcessing: (need to check)

**Proposed standard**: `kubernaut-datastorage:e2e` or `localhost/kubernaut-datastorage:e2e-test`

---

### Question 4: Migration Strategy?
**Q**: Should migrations be applied **before** or **after** DataStorage deployment?

**AIAnalysis pattern** (current):
```
1. Apply migrations → 2. Deploy DataStorage
```

**Alternative** (DataStorage self-heals):
```
1. Deploy DataStorage → 2. DataStorage applies migrations on startup
```

**Which is authoritative?**

---

### Question 5: Shared E2E Deployment Function?
**Q**: Should we create a **canonical** `DeployDataStorageForE2E` function in `datastorage.go`?

**Proposed**:
```go
// In test/infrastructure/datastorage.go

// DeployDataStorageForE2E is the authoritative way to deploy DataStorage in E2E tests
// Used by: AIAnalysis, Gateway, SignalProcessing, WorkflowExecution, Notification
func DeployDataStorageForE2E(ctx context.Context, clusterName, namespace, kubeconfigPath string, tables []string, writer io.Writer) error {
    // 1. Apply required migrations
    if err := applyRequiredMigrations(ctx, namespace, kubeconfigPath, tables, writer); err != nil {
        return err
    }

    // 2. Build and load DataStorage image
    if err := buildAndLoadDataStorageImage(clusterName, writer); err != nil {
        return err
    }

    // 3. Deploy DataStorage with full config
    if err := deployDataStorageManifest(namespace, kubeconfigPath, writer); err != nil {
        return err
    }

    // 4. Wait for readiness
    return waitForDataStorageReady(ctx, namespace, kubeconfigPath, writer)
}
```

**Benefits**:
- Single source of truth
- Consistent across all services
- Easy to maintain

---

## 🚀 Proposed Fix (Needs DS Team Approval)

### Option A: Reuse AIAnalysis's `deployDataStorage` (RECOMMENDED)
```go
// In test/infrastructure/gateway_e2e.go

func DeployTestServices(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    // ... PostgreSQL + Redis deployment ...

    // Deploy Data Storage service (reuse AIAnalysis pattern)
    fmt.Fprintln(writer, "💾 Deploying Data Storage service...")
    if err := deployDataStorage("gateway-e2e", kubeconfigPath, writer); err != nil {
        return fmt.Errorf("failed to deploy Data Storage: %w", err)
    }

    // ... Gateway deployment ...
}
```

**Pros**: ✅ Proven to work, minimal code change
**Cons**: ⚠️ May apply unnecessary migrations (workflow_catalog)

---

### Option B: Create Shared Function in `datastorage.go`
```go
// In test/infrastructure/datastorage.go

func DeployDataStorageForE2E(ctx context.Context, clusterName, namespace, kubeconfigPath string, opts DeployOptions, writer io.Writer) error {
    // Shared, configurable DataStorage deployment
}
```

**Pros**: ✅ Canonical, configurable, maintainable
**Cons**: ⚠️ Requires refactoring AIAnalysis, SignalProcessing, etc.

---

### Option C: Fix Gateway's Inline YAML (NOT RECOMMENDED)
Add missing ConfigMap, Secret, migrations, image building to Gateway's inline YAML.

**Pros**: ✅ Self-contained
**Cons**: ❌ Code duplication, maintenance burden, error-prone

---

## ⏰ Urgency

**Blocking**:
- ❌ Gateway E2E tests cannot run
- ❌ Gateway parallel optimization cannot proceed (needs baseline timing)
- ❌ Gateway V1.0 production readiness checklist incomplete

**Timeline**:
- **Today**: DS team guidance on approach
- **Next session**: Implement fix
- **Goal**: Gateway E2E tests passing, baseline established, parallel optimization implemented

---

## 📊 Similar Issues in Other Services?

| Service | DataStorage Deployment | Status | Notes |
|---------|----------------------|--------|-------|
| AIAnalysis | `deployDataStorage` (lines 421-580) | ✅ WORKING | Full config, migrations, proper setup |
| SignalProcessing | Similar to AIAnalysis | ✅ WORKING | Confirmed in codebase search |
| WorkflowExecution | `deployDataStorageWithConfig` | ✅ WORKING | Custom but complete |
| Notification | `deployDataStorageServiceForNotification` | ✅ WORKING | Custom variant |
| **Gateway** | **Inline YAML (broken)** | ❌ FAILING | Missing migrations, config, secrets |

**Pattern**: Gateway is the only service NOT using a complete deployment function.

---

## 🙋 Action Items

### For DS Team:
- [ ] Review this document
- [ ] Answer Questions 1-5
- [ ] Recommend Option A, B, or C (or suggest alternative)
- [ ] Provide guidance on minimal config for Gateway's use case

### For Gateway Team (after DS guidance):
- [ ] Implement approved fix
- [ ] Verify DataStorage pod startup
- [ ] Complete baseline E2E timing measurement
- [ ] Proceed with parallel optimization

---

## 📚 References

**Code**:
- Gateway (failing): `test/infrastructure/gateway_e2e.go:232-310`
- AIAnalysis (working): `test/infrastructure/aianalysis.go:421-580`
- SignalProcessing: `test/infrastructure/signalprocessing.go` (similar to AIAnalysis)
- DataStorage E2E: `test/infrastructure/datastorage.go`

**Documentation**:
- Parallel optimization: `docs/handoff/E2E_PARALLEL_INFRASTRUCTURE_OPTIMIZATION.md`
- Gateway fixes: `docs/handoff/GATEWAY_E2E_INFRASTRUCTURE_FIXES.md`
- Gateway implementation plan: `docs/handoff/GATEWAY_E2E_PARALLEL_IMPLEMENTATION.md`

---

**Contact**: Gateway Team (via this document)
**Status**: ⏸️ AWAITING DS TEAM RESPONSE


