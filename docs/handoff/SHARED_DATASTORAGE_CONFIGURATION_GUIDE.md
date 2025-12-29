# Shared DataStorage Service - Configuration Guide for All Teams

**Date**: 2025-12-12
**Status**: ✅ **AUTHORITATIVE REFERENCE** - Use this for all DataStorage integrations
**Applies To**: Gateway, AIAnalysis, WorkflowExecution, RemediationOrchestrator, SignalProcessing
**Authority**: ADR-030 + `test/infrastructure/datastorage.go` (proven working)

---

## 🚨 **CRITICAL: Common Integration Issues & Fixes**

If you're struggling with DataStorage integration, **you likely have one of these issues**:

| ❌ **Common Mistake** | ✅ **Correct Pattern** | **Symptom** |
|---------------------|---------------------|------------|
| `CONFIG_PATH=/dev/null` | `CONFIG_PATH=/etc/datastorage/config.yaml` | "CONFIG_PATH required (ADR-030)" |
| Using environment variables | Using ConfigMap with YAML file | "database secretsFile required" |
| `host: postgres` | `host: postgresql` | "lookup postgres: no such host" |
| `secrets_file:` (snake_case) | `secretsFile:` (camelCase) | "secretsFile required" |
| No volumeMounts | Mount config + secrets volumes | Service can't read config |
| Incomplete config fields | All required fields present | Various connection errors |

---

## 📋 **Quick Start: Copy-Paste Template**

### **Step 1: Create ConfigMap**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: <your-namespace>
data:
  config.yaml: |
    shutdownTimeout: 30s
    server:
      port: 8080
      host: "0.0.0.0"
      read_timeout: 30s
      write_timeout: 30s
    database:
      host: postgresql                    # ← Service name from shared deployment
      port: 5432
      name: action_history
      user: slm_user
      ssl_mode: disable
      max_open_conns: 25
      max_idle_conns: 5
      conn_max_lifetime: 5m
      conn_max_idle_time: 10m
      secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"  # ← camelCase!
      usernameKey: "username"
      passwordKey: "password"
    redis:
      addr: redis:6379                    # ← Service name from shared deployment
      db: 0
      dlq_stream_name: dlq-stream
      dlq_max_len: 1000
      dlq_consumer_group: dlq-group
      secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"  # ← camelCase!
      passwordKey: "password"
    logging:
      level: debug
      format: json
```

### **Step 2: Create Secret**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secret
  namespace: <your-namespace>
stringData:
  db-secrets.yaml: |
    username: slm_user
    password: test_password
  redis-secrets.yaml: |
    password: ""
```

### **Step 3: Deploy DataStorage with Volume Mounts**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: <your-namespace>
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
        image: localhost/kubernaut-datastorage:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml    # ← Points to mounted file!
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
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
  namespace: <your-namespace>
spec:
  selector:
    app: datastorage
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  - port: 9090
    targetPort: 9090
    name: metrics
```

---

## 🔍 **Detailed Explanation: Why This Pattern**

### **Why ConfigMap + volumeMounts?**

**ADR-030 Mandate**: All services MUST use YAML configuration files loaded as Kubernetes ConfigMaps.

❌ **WRONG** (Environment Variables):
```yaml
env:
- name: POSTGRES_HOST
  value: postgresql
- name: POSTGRES_PORT
  value: "5432"
- name: POSTGRES_USER
  value: slm_user
# ... 20+ more environment variables
```

✅ **CORRECT** (ConfigMap):
```yaml
env:
- name: CONFIG_PATH
  value: /etc/datastorage/config.yaml
volumeMounts:
- name: config
  mountPath: /etc/datastorage
volumes:
- name: config
  configMap:
    name: datastorage-config
```

**Benefits**:
- ✅ Configuration versioned in Git
- ✅ Easy to review entire config at once
- ✅ Can validate YAML before deployment
- ✅ Follows project-wide standard (ADR-030)
- ✅ Secrets separated from config

---

### **Why "postgresql" not "postgres"?**

**Service Name Mismatch**: The shared PostgreSQL deployment creates a service named **`postgresql`**, not `postgres`.

❌ **WRONG**:
```yaml
database:
  host: postgres    # ← Service doesn't exist!
```
```
Error: lookup postgres on 10.96.0.10:53: no such host
```

✅ **CORRECT**:
```yaml
database:
  host: postgresql  # ← Matches actual service name
```

**How to Verify**:
```bash
kubectl get svc -n <namespace> | grep postgres
# Output: postgresql    ClusterIP   10.96.x.x   <none>   5432/TCP
```

---

### **Why camelCase field names?**

**Go Struct Mapping**: DataStorage config parser uses Go struct tags that expect camelCase.

❌ **WRONG** (snake_case):
```yaml
database:
  secrets_file: "/etc/datastorage/secrets/db-secrets.yaml"
```
```
Error: database secretsFile required (ADR-030 Section 6)
```

✅ **CORRECT** (camelCase):
```yaml
database:
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
```

**All camelCase fields**:
- `secretsFile` (not `secrets_file`)
- `usernameKey` (not `username_key`)
- `passwordKey` (not `password_key`)
- `shutdownTimeout` (not `shutdown_timeout`)

---

### **Why all these database fields?**

**Complete Configuration**: DataStorage expects ALL fields to be present, not just a subset.

❌ **INCOMPLETE**:
```yaml
database:
  host: postgresql
  port: 5432
  name: action_history
```

✅ **COMPLETE**:
```yaml
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
```

**Required Fields**:
- Connection: `host`, `port`, `name`, `user`, `ssl_mode`
- Connection pool: `max_open_conns`, `max_idle_conns`, `conn_max_lifetime`, `conn_max_idle_time`
- Secrets: `secretsFile`, `usernameKey`, `passwordKey`

---

## 🐛 **Troubleshooting Guide**

### **Error: "CONFIG_PATH environment variable required (ADR-030)"**

**Symptom**:
```
ERROR: CONFIG_PATH environment variable required (ADR-030)
```

**Cause**: `CONFIG_PATH` is either:
- Not set
- Set to `/dev/null`
- Set to non-existent file

**Fix**:
```yaml
env:
- name: CONFIG_PATH
  value: /etc/datastorage/config.yaml    # ← Must be real file!
volumeMounts:
- name: config
  mountPath: /etc/datastorage
```

---

### **Error: "database secretsFile required (ADR-030 Section 6)"**

**Symptom**:
```
ERROR: Failed to load secrets (ADR-030 Section 6): database secretsFile required
```

**Cause**: Config file has wrong field name or missing field.

**Fix**: Use camelCase `secretsFile`:
```yaml
database:
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"  # ← camelCase!
```

---

### **Error: "lookup postgres: no such host"**

**Symptom**:
```
ERROR: failed to ping PostgreSQL: hostname resolving error: lookup postgres on 10.96.0.10:53: no such host
```

**Cause**: Service name mismatch.

**Fix**: Use correct service name `postgresql`:
```yaml
database:
  host: postgresql    # ← Not "postgres"!
```

**Verify**:
```bash
kubectl get svc -n <namespace> | grep postgres
```

---

### **Error: "Container image not present with pull policy Never"**

**Symptom**:
```
ErrImageNeverPull: Container image "kubernaut-datastorage:latest" is not present
```

**Cause**: Podman-built images need `localhost/` prefix.

**Fix**:
```yaml
containers:
- image: localhost/kubernaut-datastorage:latest    # ← Add localhost/ prefix!
  imagePullPolicy: Never
```

---

### **Error: CrashLoopBackOff**

**Symptom**:
```
datastorage-xxx   0/1   CrashLoopBackOff
```

**Debug Steps**:
```bash
# 1. Check logs
kubectl logs -n <namespace> deployment/datastorage

# 2. Check config mounted correctly
kubectl exec -n <namespace> deployment/datastorage -- cat /etc/datastorage/config.yaml

# 3. Check secrets mounted correctly
kubectl exec -n <namespace> deployment/datastorage -- ls -la /etc/datastorage/secrets/

# 4. Check service exists
kubectl get svc -n <namespace> postgresql redis
```

---

## 📚 **Using Shared Infrastructure Functions**

### **In E2E/Integration Tests**

Instead of writing custom deployment code, **use the shared functions**:

```go
import (
    "github.com/jordigilh/kubernaut/test/infrastructure"
)

// Deploy PostgreSQL (creates "postgresql" service)
err := infrastructure.DeployPostgreSQLInNamespace(
    ctx,
    namespace,
    kubeconfigPath,
    writer,
)

// Deploy Redis (creates "redis" service)
err = infrastructure.DeployRedisInNamespace(
    ctx,
    namespace,
    kubeconfigPath,
    writer,
)

// Wait for services to be ready
err = infrastructure.WaitForDataStorageServicesReady(
    ctx,
    namespace,
    kubeconfigPath,
    writer,
)
```

**Benefits**:
- ✅ Consistent service names across all tests
- ✅ Proven working configuration
- ✅ No duplicate code
- ✅ Automatic wait logic

**Location**: `test/infrastructure/datastorage.go`

---

## 🎯 **Team-Specific Examples**

### **Gateway Team**

```go
// In test/infrastructure/gateway.go or test/e2e/gateway/suite_test.go
func CreateGatewayCluster(clusterName, kubeconfigPath string) error {
    // 1. Create Kind cluster
    // ...

    // 2. Deploy shared PostgreSQL + Redis
    ctx := context.Background()
    err := infrastructure.DeployPostgreSQLInNamespace(ctx, "kubernaut-system", kubeconfigPath, os.Stdout)
    if err != nil {
        return err
    }
    err = infrastructure.DeployRedisInNamespace(ctx, "kubernaut-system", kubeconfigPath, os.Stdout)
    if err != nil {
        return err
    }

    // 3. Deploy DataStorage with correct config
    deployDataStorageWithConfigMap(clusterName, kubeconfigPath)

    // 4. Wait for everything to be ready
    err = infrastructure.WaitForDataStorageServicesReady(ctx, "kubernaut-system", kubeconfigPath, os.Stdout)
    if err != nil {
        return err
    }

    // 5. Deploy Gateway
    // ...
}
```

---

### **WorkflowExecution Team**

WorkflowExecution already updated to use shared functions (commit: 1760c2f9):

```go
// test/infrastructure/workflowexecution.go
func SetupWorkflowExecutionInfrastructure(ctx context.Context, namespace, kubeconfigPath string) error {
    // Use shared functions
    err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, os.Stdout)
    err = deployRedisInNamespace(ctx, namespace, kubeconfigPath, os.Stdout)
    err = waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, os.Stdout)

    // Then deploy DataStorage with ConfigMap pattern
    // ...
}
```

---

### **AIAnalysis Team**

AIAnalysis fully working with all fixes (commits: 1760c2f9, d0789f14, 5efcef3f):

```go
// test/infrastructure/aianalysis.go
func CreateAIAnalysisCluster(clusterName, kubeconfigPath string) error {
    // Use shared functions for PostgreSQL + Redis
    err := infrastructure.DeployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)
    err = infrastructure.DeployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)

    // Deploy DataStorage with ConfigMap (ADR-030)
    deployDataStorage(clusterName, kubeconfigPath, writer)

    // Wait with explicit checks
    waitForAIAnalysisInfraReady(ctx, namespace, kubeconfigPath, writer)
}
```

**Result**: ✅ All 5 pods running successfully

---

## 📖 **Authoritative References**

| Topic | Authority | Location |
|-------|-----------|----------|
| Configuration Standard | ADR-030 | `docs/architecture/decisions/ADR-030-service-configuration-management.md` |
| Working Config Example | DataStorage E2E | `test/infrastructure/datastorage.go` lines 555-767 |
| Shared Functions | DataStorage Infra | `test/infrastructure/datastorage.go` |
| ConfigMap Pattern | Context API | `pkg/contextapi/config/config.go` |

---

## ✅ **Validation Checklist**

Before asking for help, verify these:

### **1. ConfigMap Exists**
```bash
kubectl get configmap -n <namespace> datastorage-config
```

### **2. Secret Exists**
```bash
kubectl get secret -n <namespace> datastorage-secret
```

### **3. Config Has Correct Field Names**
```bash
kubectl get configmap -n <namespace> datastorage-config -o yaml | grep secretsFile
# Should show: secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
```

### **4. Service Names Are Correct**
```bash
kubectl get configmap -n <namespace> datastorage-config -o yaml | grep "host:"
# Should show: host: postgresql (not postgres)
```

### **5. Volumes Are Mounted**
```bash
kubectl get deployment -n <namespace> datastorage -o yaml | grep -A5 volumeMounts
# Should show both config and secrets mounts
```

### **6. DataStorage Logs Show Success**
```bash
kubectl logs -n <namespace> deployment/datastorage | grep "Configuration loaded"
# Should show: Configuration loaded successfully (ADR-030)
```

---

## 🚀 **Quick Fix Script**

If you have an existing broken DataStorage deployment:

```bash
#!/bin/bash
# fix-datastorage-config.sh

NAMESPACE="kubernaut-system"

# 1. Delete existing broken resources
kubectl delete deployment -n $NAMESPACE datastorage 2>/dev/null
kubectl delete configmap -n $NAMESPACE datastorage-config 2>/dev/null
kubectl delete secret -n $NAMESPACE datastorage-secret 2>/dev/null

# 2. Apply correct ConfigMap + Secret + Deployment
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: $NAMESPACE
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
  namespace: $NAMESPACE
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
  namespace: $NAMESPACE
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
        image: localhost/kubernaut-datastorage:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secret
EOF

# 3. Wait for ready
kubectl wait --for=condition=ready pod -l app=datastorage -n $NAMESPACE --timeout=60s

# 4. Verify
kubectl logs -n $NAMESPACE deployment/datastorage | grep "Configuration loaded"
```

---

## 🤝 **Getting Help**

### **If You're Still Stuck**

1. ✅ **Verify checklist above** - Most issues are simple config mistakes
2. 📋 **Check logs**: `kubectl logs -n <namespace> deployment/datastorage`
3. 🔍 **Compare your config** to the template in this doc
4. 📖 **Read ADR-030**: Full configuration management standard
5. 💬 **Ask with details**: Include logs, config YAML, and what you've tried

### **Recent Fixes (2025-12-12)**

- ✅ AIAnalysis team: All pods running with these patterns
- ✅ WorkflowExecution team: Shared functions adopted
- 🔜 Gateway team: Use this guide to fix your integration

---

## 📊 **Success Metrics**

Using this guide, you should achieve:

| Metric | Target | Evidence |
|--------|--------|----------|
| DataStorage pod status | Running | `kubectl get pods` shows 1/1 Running |
| Config load time | < 1s | Logs show "Configuration loaded successfully" |
| PostgreSQL connection | < 5s | No "lookup postgres" errors |
| Test infrastructure setup | < 5min | All shared services ready |

---

**Document Status**: ✅ **AUTHORITATIVE** - All patterns proven working in AIAnalysis E2E
**Last Updated**: 2025-12-12
**Maintained By**: Infrastructure team
**Questions**: Check validation checklist, then ask in #infrastructure channel
