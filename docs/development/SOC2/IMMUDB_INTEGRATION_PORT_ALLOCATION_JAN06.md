# Immudb Integration - Port Allocation Refactoring

**Date**: 2026-01-06
**Author**: AI Assistant
**Status**: 🚧 In Progress
**Related**: DD-TEST-001, SOC2 Gap #9 (Tamper Detection)

---

## 🎯 **Objective**

Integrate Immudb for SOC2-compliant immutable audit trails across all services, with systematic port allocation to enable parallel testing.

---

## 📋 **Immudb Port Allocation Matrix**

### **Integration Tests** (Podman)

| Service | PostgreSQL Port | Redis Port | **Immudb Port** | DS Dependency Port |
|---------|----------------|------------|-----------------|-------------------|
| **DataStorage** (primary) | 15433 | 16379 | **13322** | N/A (self) |
| **Gateway** | 15437 | 16380 | **13323** | 18091 |
| **SignalProcessing** | 15436 | 16382 | **13324** | 18094 |
| **RemediationOrchestrator** | 15435 | 16381 | **13325** | 18140 |
| **AIAnalysis** | 15438 | 16384 | **13326** | 18095 |
| **WorkflowExecution** | 15441 | 16388 | **13327** | 18097 |
| **Notification** | 15440 | 16385 | **13328** | 18096 |
| **HolmesGPT API** | 15439 | 16387 | **13329** | 18098 |
| **Auth Webhook** | 15442 | 16386 | **13330** | 18099 |

**Port Range**: 13322-13330 (9 services)
**Reserved**: 13322-13331 (10 ports for future growth)

### **E2E Tests** (Kind Clusters)

| Service | Immudb Deployment | Connection |
|---------|------------------|------------|
| **All Services** | `immudb-service.default.svc.cluster.local:3322` | In-cluster Service (no host mapping) |

**E2E Pattern**: Immudb deployed as Kubernetes Service + Deployment in Kind cluster
**Port**: Default 3322 (no conflicts in dedicated clusters)

---

## 🔄 **Refactoring Tasks**

### **Phase 1: DD-TEST-001 Documentation** ✅ In Progress
- [x] Add Immudb to main port allocation table
- [x] Update DataStorage service section
- [x] Update Gateway service section
- [ ] Update remaining 7 services
- [ ] Update Port Collision Matrix
- [ ] Add revision history entry

### **Phase 2: Code Configuration** (Pending)
- [ ] Update `pkg/datastorage/config/config.go` (ImmudbConfig struct)
- [ ] Update `test/infrastructure/datastorage_bootstrap.go` (constants + helpers)
- [ ] Create Immudb startup functions (following PostgreSQL pattern)

### **Phase 3: Integration Test Suites** (Pending)
- [ ] DataStorage (`test/integration/datastorage/`) - Port 13322
- [ ] Gateway (`test/integration/gateway/`) - Port 13323
- [ ] SignalProcessing (`test/integration/signalprocessing/`) - Port 13324
- [ ] RemediationOrchestrator (`test/integration/remediationorchestrator/`) - Port 13325
- [ ] AIAnalysis (`test/integration/aianalysis/`) - Port 13326
- [ ] WorkflowExecution (`test/integration/workflowexecution/`) - Port 13327
- [ ] Notification (`test/integration/notification/`) - Port 13328
- [ ] HolmesGPT API (`holmesgpt-api/tests/integration/`) - Port 13329
- [ ] Auth Webhook (`test/integration/authwebhook/`) - Port 13330

### **Phase 4: E2E Manifests** (Pending)
- [ ] Create `test/e2e/datastorage/manifests/immudb-deployment.yaml`
- [ ] Create `test/e2e/datastorage/manifests/immudb-service.yaml`
- [ ] Create `test/e2e/datastorage/manifests/immudb-secret.yaml`
- [ ] Update E2E test infrastructure to deploy Immudb

---

## 📝 **Implementation Pattern**

### **Integration Tests** (Programmatic Podman)

```go
// Constants (test/infrastructure/datastorage_bootstrap.go)
const (
	defaultImmudbUser     = "immudb"
	defaultImmudbPassword = "immudb_test_password"
	defaultImmudbDB       = "kubernaut_audit"
)

// Config update
type DSBootstrapConfig struct {
	// ... existing fields ...
	ImmudbPort int // Service-specific port from DD-TEST-001
}

// BeforeSuite (e.g., test/integration/gateway/suite_test.go)
dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
	ServiceName:     "gateway",
	PostgresPort:    15437,
	RedisPort:       16380,
	ImmudbPort:      13323, // Gateway-specific Immudb port
	DataStoragePort: 18091,
	MetricsPort:     19091,
	ConfigDir:       "test/integration/gateway/config",
}, GinkgoWriter)

// Config YAML (test/integration/gateway/config/config.yaml)
immudb:
  host: gateway_immudb_test  # Container name in test network
  port: 3322                 # Container internal port
  database: kubernaut_audit
  username: immudb
  tls_enabled: false
  secretsFile: "/etc/datastorage/secrets/immudb-secrets.yaml"
  passwordKey: "password"

// Secret file creation (test/integration/gateway/suite_test.go)
immudbSecretsYAML := `password: immudb_test_password`
immudbSecretsPath := filepath.Join(configDir, "immudb-secrets.yaml")
os.WriteFile(immudbSecretsPath, []byte(immudbSecretsYAML), 0666)
```

### **E2E Tests** (Kubernetes Manifests)

```yaml
# test/e2e/datastorage/manifests/immudb-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: immudb
spec:
  replicas: 1
  selector:
    matchLabels:
      app: immudb
  template:
    metadata:
      labels:
        app: immudb
    spec:
      containers:
      - name: immudb
        image: quay.io/jordigilh/immudb:latest
        ports:
        - containerPort: 3322
          name: grpc
        env:
        - name: IMMUDB_ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: immudb-secret
              key: admin-password
        - name: IMMUDB_DATABASE
          value: "kubernaut_audit"

---
# test/e2e/datastorage/manifests/immudb-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: immudb-service
spec:
  selector:
    app: immudb
  ports:
  - port: 3322
    targetPort: 3322
    name: grpc

---
# test/e2e/datastorage/manifests/immudb-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: immudb-secret
type: Opaque
stringData:
  admin-password: "immudb_e2e_password"
```

---

## 🎯 **Success Criteria**

- ✅ All 9 services have unique Immudb ports in DD-TEST-001
- ✅ All integration tests use service-specific Immudb ports
- ✅ E2E tests use default port 3322 with Kubernetes Service
- ✅ No port conflicts when running all integration tests in parallel
- ✅ Immudb deployed and accessible in E2E Kind clusters

---

## 📚 **References**

- **DD-TEST-001**: Port Allocation Strategy (authoritative)
- **SOC2 Gap #9**: Event Hashing (Tamper-Evidence)
- **PostgreSQL Pattern**: Existing reference for secret handling and bootstrap

---

**Status**: Phase 1 in progress (DD-TEST-001 documentation)
**Next**: Complete DD-TEST-001 updates, then proceed to Phase 2 (code configuration)

