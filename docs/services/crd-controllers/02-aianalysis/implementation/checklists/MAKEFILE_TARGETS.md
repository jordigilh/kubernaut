# AI Analysis Service - Makefile Targets

**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)
**Version**: 1.0

---

## 🔧 **Build Targets**

### **Build Controller Binary**
```bash
# Build the aianalysis controller
make build-aianalysis

# Or manually
go build -o bin/aianalysis ./cmd/aianalysis/...
```

### **Build with Version Info**
```bash
# Build with ldflags for version info
VERSION=$(git describe --tags --always --dirty)
COMMIT=$(git rev-parse HEAD)
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

go build -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildDate=${DATE}" \
  -o bin/aianalysis ./cmd/aianalysis/...
```

---

## 🧪 **Test Targets**

### **Unit Tests**
```bash
# Run all unit tests
make test-unit-aianalysis

# Or manually
go test ./test/unit/aianalysis/... -v

# With coverage
go test ./test/unit/aianalysis/... -coverprofile=coverage.out -covermode=atomic
go tool cover -html=coverage.out -o coverage.html
```

### **Integration Tests**
```bash
# Run integration tests (requires KIND)
make test-integration-aianalysis

# Or manually
INTEGRATION_TEST=true go test ./test/integration/aianalysis/... -v -timeout 10m
```

### **E2E Tests**
```bash
# Run E2E tests (requires full stack)
make test-e2e-aianalysis

# Or manually
E2E_TEST=true go test ./test/e2e/aianalysis/... -v -timeout 15m
```

### **All Tests**
```bash
# Run all tests
make test-aianalysis

# Parallel execution
go test ./test/unit/aianalysis/... -v -p 4
```

---

## 📦 **Code Generation**

### **Generate CRD Manifests**
```bash
# Generate CRD YAML
make manifests

# Regenerate after type changes
make generate
```

### **Generate DeepCopy Methods**
```bash
# Generate zz_generated.deepcopy.go
controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./api/..."
```

---

## 🔍 **Lint Targets**

### **Run Linter**
```bash
# Run golangci-lint
make lint

# For specific packages
golangci-lint run ./internal/controller/aianalysis/... ./pkg/aianalysis/...
```

### **Fix Lint Issues**
```bash
# Auto-fix where possible
golangci-lint run --fix ./internal/controller/aianalysis/...
```

---

## 🐳 **Docker Targets**

### **Build Docker Image**
```bash
# Build image
make docker-build-aianalysis IMG=aianalysis:dev

# Or manually
docker build -t aianalysis:dev -f Dockerfile.aianalysis .
```

### **Push Docker Image**
```bash
# Push to registry
make docker-push-aianalysis IMG=aianalysis:dev
```

---

## ☸️ **Kubernetes Targets**

### **Install CRDs**
```bash
# Install CRDs to cluster
make install

# Or manually
kubectl apply -f config/crd/bases/aianalysis.kubernaut.io_aianalyses.yaml
```

### **Deploy Controller**
```bash
# Deploy to cluster
make deploy-aianalysis

# With custom image
make deploy-aianalysis IMG=aianalysis:dev
```

### **Undeploy Controller**
```bash
# Remove from cluster
make undeploy-aianalysis
```

---

## 📊 **Metrics Targets**

### **Validate Metrics**
```bash
# Check metrics endpoint
curl -s localhost:9090/metrics | grep -E "^aianalysis_"

# Count metrics
curl -s localhost:9090/metrics | grep -c "^aianalysis_"
```

### **Expected Metrics**
```
aianalysis_reconciliation_total
aianalysis_reconciliation_duration_seconds
aianalysis_phase_transitions_total
aianalysis_phase_duration_seconds
aianalysis_holmesgpt_requests_total
aianalysis_holmesgpt_latency_seconds
aianalysis_rego_policy_evaluations_total
aianalysis_approval_decisions_total
aianalysis_detected_labels_failures_total
```

---

## 🧹 **Clean Targets**

### **Clean Build Artifacts**
```bash
# Clean all build artifacts
make clean

# Clean specific
rm -rf bin/aianalysis
rm -f coverage.out coverage.html
```

### **Clean Test Cache**
```bash
# Clear Go test cache
go clean -testcache
```

---

## 📋 **Quick Reference**

| Target | Command | Purpose |
|--------|---------|---------|
| Build | `make build-aianalysis` | Compile controller |
| Test Unit | `make test-unit-aianalysis` | Run unit tests |
| Test Integration | `make test-integration-aianalysis` | Run integration tests |
| Test E2E | `make test-e2e-aianalysis` | Run E2E tests |
| Lint | `make lint` | Run linter |
| Generate | `make generate` | Generate code |
| Manifests | `make manifests` | Generate CRD YAML |
| Docker Build | `make docker-build-aianalysis` | Build Docker image |
| Deploy | `make deploy-aianalysis` | Deploy to cluster |
| Clean | `make clean` | Clean artifacts |

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [Makefile](../../../../../Makefile) | Main project Makefile |
| [DD-014: Version Logging](../../../../architecture/decisions/DD-014-binary-version-logging.md) | Build flags |

