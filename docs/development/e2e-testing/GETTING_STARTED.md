# Getting Started with Kubernaut E2E Testing

**Status**: Feature Complete (Testing Phase)
**Current Phase**: Unit, Integration, and E2E Test Implementation
**Deployment Model**: Hybrid Architecture (Recommended)

---

## 🎯 **Quick Overview**

Kubernaut E2E testing uses a **hybrid architecture** for optimal security and performance:
- **🖥️ OpenShift Cluster**: Remote dedicated host (e.g., helios08)
- **🤖 HolmesGPT Container**: Custom container image with REST API (deployed in cluster or locally)
- **🔧 Kubernaut**: Local machine (manages remote cluster)
- **🧪 Tests**: Local machine
- **📊 Vector DB**: Local machine (PostgreSQL)

This provides **enterprise-grade security isolation** while maintaining **development efficiency**.

**Note**: HolmesGPT now runs as a **custom container image** with REST API endpoints, replacing the previous CLI sidecar approach.

---

## 🚀 **1. Prerequisites**

### **Remote Host Requirements**
- **RHEL 9.7** bare metal server with root SSH access
- **Minimum**: 64GB RAM, 16 CPU cores, 500GB storage
- **Recommended**: 256GB RAM, 40 CPU cores, 3TB storage
- **Network**: SSH key authentication configured

### **Local Development Machine**
- **Go 1.21+** for building Kubernaut
- **oc/kubectl** CLI tools
- **PostgreSQL** with pgvector extension
- **HolmesGPT REST API** running on localhost:8090
- **SSH access** to remote host

### **HolmesGPT Container Setup**
Ensure HolmesGPT custom container is deployed and accessible:
```bash
# Option 1: Local container deployment
docker run -d -p 8090:8090 kubernaut/holmesgpt-api:latest

# Option 2: Kubernetes deployment (in remote cluster)
oc apply -f deploy/holmesgpt-deployment.yaml

# Verify HolmesGPT REST API is accessible
curl http://localhost:8090/health

# Expected response: {"status": "healthy", ...}
```

---

## 🏗️ **2. Hybrid Deployment (Primary Method)**

### **Step 1: Deploy Remote OpenShift Cluster**
```bash
# Navigate to e2e testing directory
cd docs/development/e2e-testing/

# Configure and validate remote host
make configure-e2e-remote

# Deploy ONLY OpenShift cluster on remote host
make deploy-cluster-remote-only
```

### **Step 2: Setup Local Environment**
```bash
# Setup local Kubernaut to connect to remote cluster
make setup-local-hybrid

# This will:
# - Retrieve kubeconfig from remote cluster
# - Build local Kubernaut
# - Setup PostgreSQL vector database
# - Create monitoring configuration
```

### **Step 3: Validate Hybrid Architecture**
```bash
# Validate network topology and security isolation
make validate-hybrid-topology

# Check all component status
make status-hybrid
```

### **Step 4: Run E2E Tests**
```bash
# Run hybrid E2E test scenarios
make test-e2e-hybrid

# This tests the complete flow:
# - Local Kubernaut managing remote cluster
# - Local HolmesGPT API processing alerts
# - Network isolation validation
# - Business requirement validation
```

---

## 🛠️ **3. Alternative Deployment Options**

### **Local Development (Testing Only)**
For development and unit testing:
```bash
# Setup complete environment locally (requires significant resources)
./setup-complete-e2e-environment.sh

# Run local tests
./run-e2e-tests.sh basic
```

### **Root User Deployment (Single Host)**
For single-host deployment with root access:
```bash
# Deploy everything on one RHEL 9.7 host as root
sudo ./setup-complete-e2e-environment-root.sh

# Run comprehensive tests
sudo ./run-e2e-tests-root.sh all
```

### **Legacy Remote Deployment (Complete Remote)**
For completely remote deployment (all components on remote host):
```bash
# Deploy complete environment on remote host
make setup-e2e-remote

# Run tests on remote host
make test-e2e-remote
```

---

## 🔍 **4. Validation & Monitoring**

### **Health Checks**
```bash
# Check hybrid deployment status
make status-hybrid

# Expected output shows:
# ✅ Remote cluster accessible
# ✅ Local HolmesGPT API responding
# ✅ Local Kubernaut running
# ✅ PostgreSQL vector DB available
```

### **Network Topology Validation**
```bash
# Validate security isolation
make validate-hybrid-topology

# Verifies:
# ✅ Local machine → Remote cluster (should work)
# ✅ Local machine → HolmesGPT API (should work)
# ❌ Remote cluster → HolmesGPT API (should fail - security feature)
```

### **Component Access**
```bash
# SSH to remote cluster for debugging
make ssh-remote-cluster

# View deployment logs
make logs-e2e-remote

# Monitor local Kubernaut
tail -f ./local-config-remote/kubernaut.log
```

---

## 🧪 **5. Running Tests**

### **Test Categories**
```bash
# Business requirement validation
make test-e2e-hybrid           # Complete hybrid test suite

# Specific test categories
./run-use-case-1.sh           # Pod resource exhaustion recovery
./run-use-case-2.sh           # Node failure remediation
./run-use-case-3.sh           # Network policy conflicts
# ... (see TOP_10_E2E_USE_CASES.md for full list)
```

### **Chaos Engineering**
```bash
# Run chaos experiments with LitmusChaos
./run-chaos-tests.sh pod-delete
./run-chaos-tests.sh node-drain
./run-chaos-tests.sh network-partition
```

### **Test Data & Scenarios**
```bash
# Inject realistic test data
./inject-test-scenarios.sh

# Run stress tests
./run-stress-tests.sh ai-model
./run-stress-tests.sh cluster-load
```

---

## 🧹 **6. Cleanup**

### **Hybrid Environment Cleanup**
```bash
# Stop local components and cleanup remote cluster
make cleanup-hybrid

# This will:
# - Stop local Kubernaut and PostgreSQL
# - Remove local configuration files
# - Cleanup remote OpenShift cluster
# - Remove remote deployment files
```

### **Selective Cleanup**
```bash
# Cleanup only remote cluster (preserve local setup)
ssh root@helios08 "cd /root/kubernaut-e2e && ./cleanup-e2e-environment-root.sh"

# Cleanup only local components
pkill kubernaut
rm -rf local-config-remote/ test-config-hybrid/
```

---

## 📊 **7. Troubleshooting**

### **Common Issues**

#### **SSH Connection Issues**
```bash
# Test SSH connectivity
ssh root@helios08 "echo 'Connection OK'"

# Reconfigure if needed
./configure-remote-host.sh helios08 root
```

#### **HolmesGPT API Not Accessible**
```bash
# Check if HolmesGPT REST API is running
curl http://localhost:8090/health

# Start HolmesGPT API if needed
# (Follow HolmesGPT REST API setup documentation)
```

#### **Cluster Connection Issues**
```bash
# Test cluster connectivity
export KUBECONFIG=./kubeconfig-remote
oc get nodes

# Re-retrieve kubeconfig if needed
scp root@helios08:/root/kubernaut-e2e/kubeconfig ./kubeconfig-remote
```

#### **PostgreSQL Issues**
```bash
# Check PostgreSQL service
pgrep postgres

# Start PostgreSQL if needed
brew services start postgresql  # macOS
sudo systemctl start postgresql # Linux
```

### **Debug Commands**
```bash
# Check all component status
make status-hybrid

# View recent logs
make logs-e2e-remote

# SSH for manual debugging
make ssh-remote-cluster

# Test network topology
make validate-hybrid-topology
```

---

## 🎯 **8. Next Steps**

### **For Developers**
1. **Follow hybrid setup** (Steps 1-4 above)
2. **Run test scenarios** to validate functionality
3. **Focus on unit/integration tests** (current milestone)
4. **Contribute to test coverage** for business requirements

### **For QA Teams**
1. **Use hybrid deployment** for realistic testing scenarios
2. **Execute chaos engineering** experiments
3. **Validate business requirements** using automated test suite
4. **Report issues** using test result dashboards

### **For DevOps Teams**
1. **Deploy hybrid infrastructure** for team environments
2. **Setup monitoring** and alerting for test environments
3. **Integrate with CI/CD** pipelines for automated testing
4. **Maintain cluster** and environment health

---

## 📚 **Additional Resources**

- **[HYBRID_ARCHITECTURE_SUMMARY.md](HYBRID_ARCHITECTURE_SUMMARY.md)** - Complete architectural overview
- **[TOP_10_E2E_USE_CASES.md](TOP_10_E2E_USE_CASES.md)** - Business requirement test scenarios
- **[TESTING_TOOLS_AUTOMATION.md](TESTING_TOOLS_AUTOMATION.md)** - Testing framework details
- **[README.md](README.md)** - Complete deployment options and configuration details

---

**🚀 Ready to start? Begin with the hybrid deployment (Steps 1-4) for the optimal balance of security, performance, and development efficiency.**
