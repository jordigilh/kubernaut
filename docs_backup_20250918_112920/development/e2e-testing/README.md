# OpenShift 4.18 Virtual Deployment on RHEL 9.7

This directory contains comprehensive configurations and guides for deploying OpenShift 4.18 **virtually using KCLI** on a RHEL 9.7 host, optimized for testing environments.

## 🎯 **Optimized for Your Environment**

**Target Host:** RHEL 9.7 with Intel Xeon Gold 5218R, 256GB RAM, 3TB storage
**Deployment Type:** Virtual cluster using KVM/QEMU on single physical host
**Resource Usage:** Conservative allocation leaving plenty of headroom

## 📁 Directory Contents

### 🚀 **Quick Start Files** (Start Here!)
- **`KCLI_QUICK_START.md`** - **Recommended quick start** (5 commands to full cluster)
- **`QUICK_START.md`** - Traditional openshift-installer approach

### 📖 **Comprehensive Guides**
- **`KCLI_BAREMETAL_INSTALLATION_GUIDE.md`** - Complete KCLI guide with RHEL 9.7 specifics
- **`OCP_BAREMETAL_INSTALLATION_GUIDE.md`** - Traditional installer guide

### ⚙️ **Configuration Files**
- **`kcli-baremetal-params.yml`** - **Main configuration** - optimized for your host
- **`install-config-baremetal-4.18.yaml`** - Traditional installer config

### 🔧 **Automation Scripts** (All executable)
- **`deploy-kcli-cluster.sh`** - **Complete automated deployment**
- **`validate-baremetal-setup.sh`** - Pre-deployment validation for RHEL 9.7
- **`setup-storage.sh`** - Automated storage configuration

### 💾 **Storage Configuration**
- **`storage/local-storage-operator.yaml`** - Local Storage Operator config
- **`storage/odf-operator.yaml`** - OpenShift Data Foundation config

## 🚀 **Quick Start**

**New to E2E Testing?** 👉 **[GETTING_STARTED.md](GETTING_STARTED.md)** - Complete setup guide with hybrid architecture

## ⚡ **Super Quick Deployment Options**

### **Option 1: OpenShift Cluster Only (2 Steps)**

#### **Step 1:** Provide Required Files (Only 2!)
```bash
# 1. Download your pull secret from Red Hat
curl -s https://console.redhat.com/openshift/install/pull-secret -o ~/.pull-secret.txt

# 2. Generate SSH key if needed
ssh-keygen -t rsa -b 4096 -C "your-email@kubernaut.io"
```

#### **Step 2:** Deploy Cluster Only
```bash
# Navigate to config directory
cd docs/development/e2e-testing/

# Optionally validate setup first
./validate-baremetal-setup.sh kcli-baremetal-params.yml

# Deploy complete cluster with automated storage
./deploy-kcli-cluster.sh kubernaut-e2e kcli-baremetal-params.yml

# Access cluster (ready in ~80 minutes)
export KUBECONFIG=~/.kcli/clusters/kubernaut-e2e/auth/kubeconfig
oc get nodes
oc get storageclass  # 5 storage classes ready!
```

### **Option 2: Complete E2E Testing Environment (1 Step!)**

#### **Deploy Everything for E2E Testing**
```bash
# Navigate to e2e testing directory
cd docs/development/e2e-testing/

# Deploy complete E2E testing environment
# Includes: OCP cluster + Kubernaut + AI model + Vector DB + Chaos testing + Monitoring
./setup-complete-e2e-environment.sh

# Validate complete environment
./validate-complete-e2e-environment.sh --detailed

# Run E2E tests
./run-e2e-tests.sh all

# Cleanup when done
./cleanup-e2e-environment.sh
```

### **Option 3: Root User Deployment on RHEL 9.7 (Recommended for Production Testing)**

#### **Complete E2E Environment as Root**
```bash
# Navigate to e2e testing directory
cd docs/development/e2e-testing/

# Ensure scripts are executable
sudo ./make-scripts-executable.sh

# Validate RHEL 9.7 system as root
sudo ./validate-baremetal-setup-root.sh kcli-baremetal-params-root.yml

# Deploy complete E2E testing environment as root
# Optimized for RHEL 9.7 with proper permissions and paths
sudo ./setup-complete-e2e-environment-root.sh

# Validate environment as root
sudo ./validate-complete-e2e-environment.sh --detailed

# Run tests as root
sudo ./run-e2e-tests-root.sh basic

# Cleanup when done
sudo ./cleanup-e2e-environment-root.sh
```

### **Option 4: Hybrid Architecture (Recommended for Enterprise)**

#### **Deploy Hybrid: Remote Cluster + Local AI/Tests**

**Architecture Overview:**
- 🖥️ **OpenShift Cluster**: Remote host (helios08)
- 🤖 **HolmesGPT Container**: Custom REST API container (localhost:8090 or in-cluster)
- 🔧 **Kubernaut**: Local machine (manages remote cluster)
- 🧪 **Tests**: Local machine
- 📊 **Vector DB**: Local machine (PostgreSQL)

**Network Topology:**
- Local machine → Remote cluster ✅
- Local machine → HolmesGPT Container ✅
- Remote cluster → HolmesGPT ⚙️ (configurable: local or in-cluster deployment)

```bash
# Navigate to e2e testing directory
cd docs/development/e2e-testing/

# 1. Deploy ONLY OpenShift cluster on remote host
make deploy-cluster-remote-only

# 2. Setup local Kubernaut to connect to remote cluster
make setup-local-hybrid

# 3. Deploy HolmesGPT container
# Option A: Local deployment
docker run -d -p 8090:8090 kubernaut/holmesgpt-api:latest

# Option B: In-cluster deployment
oc apply -f deploy/holmesgpt-deployment.yaml

# 4. Validate hybrid network topology
make validate-hybrid-topology

# 5. Check hybrid deployment status
make status-hybrid

# 6. Run E2E tests in hybrid mode
make test-e2e-hybrid

# 7. SSH to remote cluster for management
make ssh-remote-cluster

# 8. Cleanup hybrid environment
make cleanup-hybrid
```

### **Option 5: Legacy Remote Host Deployment (Complete Remote)**

#### **Deploy Complete Environment on Remote Host (helios08)**
```bash
# Navigate to e2e testing directory
cd docs/development/e2e-testing/

# Configure and validate remote host connection
make configure-e2e-remote

# Validate remote host readiness
make validate-e2e-remote

# Deploy complete E2E environment on remote host
make setup-e2e-remote

# Check environment status
make status-e2e-remote

# Run tests on remote host
make test-e2e-remote

# View deployment logs
make logs-e2e-remote

# SSH to remote host for manual management
make ssh-e2e-remote

# Cleanup remote environment
make cleanup-e2e-remote
```

#### **Custom Remote Host Configuration**
```bash
# Configure different remote host
./configure-remote-host.sh myhost.kubernaut.io admin

# Or skip connection test
TEST_CONNECTION=false ./configure-remote-host.sh 192.168.122.100

# Update Makefile variables manually if needed
# Edit REMOTE_HOST and REMOTE_USER in Makefile
```

## 📊 **Resource Allocation (Conservative for Testing)**

### **Your RHEL 9.7 Host**
- **Available:** Intel Xeon Gold 5218R (40 threads), 256GB RAM, 3TB disk
- **VM Usage:** ~84GB RAM, 24 vCPUs, ~500GB storage
- **Headroom:** **172+ GB RAM, 16+ CPU threads, 2.5+ TB disk free**

### **Virtual OpenShift Cluster**
| Component | Count | Per VM | Total |
|-----------|-------|---------|--------|
| **Masters** | 3 | 16GB RAM, 4 vCPU, 80GB | 48GB, 12 vCPU, 240GB |
| **Workers** | 3 | 12GB RAM, 4 vCPU, 80GB | 36GB, 12 vCPU, 240GB |
| **Bootstrap** | 1 | 16GB RAM, 4 vCPU, 80GB | Temporary during install |
| **Storage** | - | Virtual disks | ~600GB for ODF |

**Peak Usage During Install:** ~100GB RAM, 28 vCPUs, 600GB disk
**Steady State:** ~84GB RAM, 24 vCPUs, 500GB disk

## 💾 **Automated Storage Setup**

### **Storage Operators Installed Automatically**
1. **Local Storage Operator (LSO)** - Manages virtual storage devices
2. **OpenShift Data Foundation (ODF)** - Enterprise Ceph storage (200GB/worker)

### **5 Storage Classes Created Automatically**
1. **`ocs-storagecluster-ceph-rbd`** (default) - High-performance block storage
2. **`ocs-storagecluster-cephfs`** - Shared filesystem (ReadWriteMany)
3. **`ocs-storagecluster-ceph-rgw`** - S3-compatible object storage
4. **`local-block`** - Local block devices for high I/O
5. **`local-filesystem`** - Local filesystem storage

### **Ready to Use Immediately**
```bash
# Create a PVC using default storage
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 10Gi
EOF
```

## ⏱️ **Installation Timeline**

| Phase | Duration | Description | Status |
|-------|----------|-------------|---------|
| Validation | 2 min | RHEL 9.7 host checks | ✅ Automated |
| Bootstrap | 10 min | Bootstrap VM creation | ✅ Automated |
| Masters | 20 min | Control plane (3 VMs) | ✅ Automated |
| Workers | 15 min | Worker nodes (3 VMs) | ✅ Automated |
| Finalization | 10 min | Cluster initialization | ✅ Automated |
| **Storage Setup** | **15-20 min** | **LSO + ODF deployment** | ✅ **Automated** |
| **Total** | **~80 min** | **Complete production-ready cluster** | 🎉 **Ready!** |

## 🎯 **Key Features**

### ✨ **Complete Automation**
- **No Manual Steps:** Everything automated except pull secret and SSH key
- **Resource Optimization:** Conservative allocation for stable testing
- **Storage Ready:** 5 storage classes configured automatically
- **Health Monitoring:** Built-in validation and verification

### 🛡️ **Production-Grade Storage**
- **High Availability:** 3-replica Ceph cluster with automatic failover
- **Backup & Recovery:** Snapshot and clone capabilities
- **Performance:** Optimized for test workloads on single host
- **Monitoring:** Integrated storage metrics and alerting

### 🔧 **RHEL 9.7 Optimized**
- **KVM Acceleration:** Uses host CPU features for better performance
- **Memory Efficiency:** Conservative allocation with ample headroom
- **Storage Efficiency:** Virtual disk allocation optimized for testing
- **Network Optimization:** Internal virtual networking for performance

## 🎛️ **Configuration Customization**

The default configuration is ready to use, but you can customize in `kcli-baremetal-params.yml`:

```yaml
# Only change these if needed:
cluster: ocp418-baremetal          # Cluster name
domain: kubernaut.io                # Your domain
pull_secret: '~/.pull-secret.txt'  # Your pull secret path
ssh_key: '~/.ssh/id_rsa.pub'      # Your SSH key path

# Network (defaults are fine for testing)
api_ip: 192.168.122.100
ingress_ip: 192.168.122.101
cidr: 192.168.122.0/24

# Storage (optimized for testing)
storage_operators: true            # Keep enabled for automated storage
odf_size: "200Gi"                 # Reasonable size for testing

# Resources (optimized for your host - don't increase)
ctlplane_memory: 16384               # 16GB per ctlplane
worker_memory: 12288               # 12GB per worker
```

## 📋 **Prerequisites (Simple!)**

### **What You Have** ✅
- RHEL 9.7 host with libvirtd
- Intel Xeon Gold 5218R, 256GB RAM, 3TB storage
- Internet access

### **What You Need** (2 items)
1. **Red Hat Pull Secret** - Download from [console.redhat.com](https://console.redhat.com/openshift/install/pull-secret)
2. **SSH Key Pair** - Generate with `ssh-keygen -t rsa -b 4096`

### **What Gets Installed Automatically**
- KCLI (if not present)
- OpenShift 4.18 cluster (6 VMs)
- Local Storage Operator
- OpenShift Data Foundation
- All required dependencies

## 🆘 **Troubleshooting**

### **Quick Debug Commands**
```bash
# Check cluster status
kcli list cluster
kcli info cluster ocp418-test

# Check host resources
free -h
df -h /var/lib/libvirt/images
virsh list --all

# Check storage
oc get storageclass
oc get cephcluster -n openshift-storage
oc get pv
```

### **Common Issues**
1. **Insufficient resources:** The configuration is conservative, but if issues arise, check `free -m` and `df -h`
2. **Storage not ready:** Wait 5-10 minutes after cluster is up for storage operators to initialize
3. **Network issues:** Check libvirt networking with `virsh net-list --all`

## 🎉 **What You Get**

After successful deployment:

✅ **Production-ready OpenShift 4.18 cluster**
✅ **5 storage classes with default configured**
✅ **Enterprise Ceph storage with HA**
✅ **Monitoring, logging, and alerting enabled**
✅ **Web console access with admin credentials**
✅ **Ready for application deployment**
✅ **Backup and disaster recovery capabilities**
✅ **All running on single RHEL 9.7 host efficiently**

## 🧪 **Complete E2E Testing Environment Features**

### **Comprehensive Testing Stack**
The complete E2E environment includes everything needed for Kubernaut testing:

#### **Core Infrastructure**
- **OpenShift 4.18 Cluster**: Virtual cluster with ODF storage
- **AI Model Integration**: oss-gpt:20b at localhost:8080
- **Vector Database**: PostgreSQL with pgvector for pattern storage
- **Monitoring Stack**: Prometheus/Grafana for metrics and alerting

### **🔐 Root User Deployment (RHEL 9.7 Optimized)**

#### **Why Root Deployment?**
- **Production-Grade**: Matches typical enterprise deployment patterns
- **Full System Access**: Complete control over libvirt, networking, and storage
- **RHEL 9.7 Optimized**: Tuned specifically for Red Hat Enterprise Linux 9.7
- **Enterprise Security**: Proper permissions and ownership throughout

#### **Root Deployment Features**
- **Automated KCLI Installation**: Installs and configures KCLI for root user
- **libvirt Configuration**: Proper setup of virtualization stack with root permissions
- **Storage Optimization**: Dedicated /var/lib/libvirt/images storage management
- **Network Management**: Full control over virtual networking and VIPs
- **Security Compliance**: SELinux and firewall configuration handling

### **🌐 Hybrid Architecture Deployment (Recommended Enterprise Pattern)**

#### **Why Hybrid Architecture?**
- **Security Isolation**: Cluster cannot reach local development resources
- **Resource Optimization**: Heavy cluster workloads on dedicated hardware, AI/dev tools local
- **Network Realism**: Tests realistic network latency and connectivity patterns
- **Development Efficiency**: Fast local AI model access and test iteration
- **Enterprise Security**: Production-like isolation between cluster and AI services

#### **Hybrid Architecture Features**
- **Remote Cluster Management**: Local Kubernaut manages remote OpenShift cluster
- **Local AI Integration**: Fast access to localhost:8080 AI model with no network latency
- **Network Topology Validation**: Ensures proper isolation between components
- **Kubeconfig Management**: Automatic retrieval and management of remote cluster access
- **Hybrid Monitoring**: Combined local and remote metrics collection
- **Security Testing**: Validates that cluster cannot access development machine resources

### **🖥️ Legacy Remote Deployment (Complete Remote)**

#### **Why Complete Remote Deployment?**
- **Enterprise Infrastructure**: Deploy everything on dedicated bare metal servers
- **Resource Isolation**: Complete separation from development machines
- **Production Realism**: Test on actual enterprise hardware configurations
- **Team Collaboration**: Shared testing environments accessible to multiple developers
- **CI/CD Integration**: Automated deployment to remote test infrastructure

#### **Complete Remote Features**
- **SSH Key Authentication**: Secure passwordless deployment and management
- **Automated File Transfer**: Intelligent copying of only required scripts and configurations
- **Connection Validation**: Comprehensive SSH and host validation before deployment
- **Remote Monitoring**: Real-time status checking and log viewing from local machine
- **Error Handling**: Robust error detection and reporting for remote operations
- **Resource Management**: Remote resource usage monitoring and optimization

#### **Testing Frameworks**
- **Chaos Engineering**: LitmusChaos for controlled failure injection
- **Test Applications**: Pre-deployed apps for testing scenarios
- **Business Validation**: Automated BR-XXX requirement validation
- **Load Testing**: AI model stress testing capabilities

#### **Automation & Scripts**
- **One-Command Setup**: Complete environment in ~30 minutes
- **Comprehensive Validation**: 50+ automated checks
- **Test Execution**: Ready-to-run E2E test suites
- **Auto-Cleanup**: Scheduled environment cleanup

### **Available Test Suites**

#### **Top 10 E2E Use Cases**
1. **AI-Driven Pod Resource Exhaustion Recovery**
2. **Multi-Node Failure with Workload Migration**
3. **HolmesGPT Investigation Pipeline Under Load**
4. **Network Partition Recovery with Service Mesh**
5. **Storage Failure with Vector Database Persistence**
6. **AI Model Timeout Cascade with Fallback Logic**
7. **Cross-Namespace Resource Contention Resolution**
8. **Prometheus Alertmanager Integration Storm**
9. **Security Incident Response with Pod Quarantine**
10. **End-to-End Disaster Recovery Validation**

#### **Test Execution Commands**
```bash
# Run all test suites
./run-e2e-tests.sh all

# Run specific test categories
./run-e2e-tests.sh use-cases    # Business use cases
./run-e2e-tests.sh chaos        # Chaos engineering tests
./run-e2e-tests.sh stress       # AI model stress tests

# Run individual use cases
./run-use-case-1.sh             # Resource exhaustion recovery
./run-use-case-3.sh             # HolmesGPT investigation
./run-use-case-9.sh             # Security incident response
```

### **Environment Management**

#### **Setup Commands**
```bash
# Complete E2E environment setup
./setup-complete-e2e-environment.sh

# Validate environment health
./validate-complete-e2e-environment.sh --detailed

# Check specific components
./validate-baremetal-setup.sh kcli-baremetal-params.yml
```

#### **Hybrid Setup Commands**
```bash
# Deploy OpenShift cluster on remote host only
make deploy-cluster-remote-only

# Setup local Kubernaut to connect to remote cluster
make setup-local-hybrid

# Validate hybrid network topology
make validate-hybrid-topology

# Check hybrid environment status
make status-hybrid
```

#### **Legacy Remote Setup Commands**
```bash
# Configure remote host connection
make configure-e2e-remote

# Complete E2E environment setup on remote host
make setup-e2e-remote

# Validate remote environment health
make validate-e2e-remote

# Check remote environment status
make status-e2e-remote
```

#### **Cleanup Commands**
```bash
# Complete cleanup
./cleanup-e2e-environment.sh

# Preserve cluster but cleanup applications
./cleanup-e2e-environment.sh --preserve-cluster

# Preserve data for analysis
./cleanup-e2e-environment.sh --preserve-data
```

#### **Hybrid Cleanup Commands**
```bash
# Complete hybrid cleanup (local + remote)
make cleanup-hybrid

# SSH to remote cluster for manual management
make ssh-remote-cluster
```

#### **Legacy Remote Cleanup Commands**
```bash
# Complete remote cleanup
make cleanup-e2e-remote

# SSH to remote host for manual cleanup
make ssh-e2e-remote
```

## 📚 **Next Steps**

### **For Cluster-Only Deployment:**
1. **Access web console:** Get URL with `oc get routes console -n openshift-console`
2. **Deploy test applications:** Use any storage class for persistent volumes
3. **Explore operators:** Check OperatorHub for additional capabilities
4. **Monitor resources:** Use `htop` and `oc top nodes` to monitor usage
5. **Scale if needed:** Add more workers with `kcli scale cluster`

### **For Complete E2E Environment:**
1. **Run validation:** `./validate-complete-e2e-environment.sh --detailed`
2. **Execute test suites:** `./run-e2e-tests.sh all`
3. **Monitor testing:** Check Grafana dashboards for metrics
4. **Analyze results:** Review business requirement validation reports
5. **Cleanup environment:** `./cleanup-e2e-environment.sh` when done

### **For Hybrid E2E Deployment (Recommended):**
1. **Deploy remote cluster:** `make deploy-cluster-remote-only` (OpenShift on helios08)
2. **Start local AI model:** Ensure oss-gpt:20b is running on localhost:8080
3. **Setup local environment:** `make setup-local-hybrid` (local Kubernaut + Vector DB)
4. **Validate topology:** `make validate-hybrid-topology` (network isolation tests)
5. **Check status:** `make status-hybrid` (monitor all components)
6. **Execute tests:** `make test-e2e-hybrid` (run hybrid test scenarios)
7. **Manual access:** `make ssh-remote-cluster` (SSH to cluster for debugging)
8. **Cleanup:** `make cleanup-hybrid` (cleanup both local and remote components)

### **For Legacy Remote E2E Deployment:**
1. **Configure remote host:** `make configure-e2e-remote` (validates SSH and host readiness)
2. **Deploy environment:** `make setup-e2e-remote` (complete automated deployment)
3. **Monitor deployment:** `make status-e2e-remote` (real-time status and resource usage)
4. **Execute tests:** `make test-e2e-remote` (run test suites on remote host)
5. **View logs:** `make logs-e2e-remote` (check deployment and test logs)
6. **Manual access:** `make ssh-e2e-remote` (SSH to remote host for debugging)
7. **Cleanup:** `make cleanup-e2e-remote` (complete remote environment cleanup)

## 🔗 **Additional Resources**

- **KCLI Documentation:** https://kcli.readthedocs.io/
- **OpenShift Documentation:** https://docs.openshift.com/container-platform/4.18/
- **OpenShift Data Foundation:** https://access.redhat.com/products/red-hat-openshift-data-foundation
- **Red Hat Support:** https://access.redhat.com/

---

**🚀 Ready to deploy? Start with `KCLI_QUICK_START.md` for the fastest path to a running cluster!**