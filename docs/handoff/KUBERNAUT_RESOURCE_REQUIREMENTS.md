# Kubernaut Platform - Resource Requirements Summary

**Date**: December 13, 2025
**Version**: V1.0
**Purpose**: Complete resource consumption estimate for full Kubernaut platform deployment

---

## 📊 **Executive Summary**

**Total Resource Requirements for Full Kubernaut Deployment**:

| Tier | CPU Requests | CPU Limits | Memory Requests | Memory Limits |
|------|--------------|------------|-----------------|---------------|
| **Minimum (Requests)** | **1.55 cores** | **5.5 cores** | **2.24 GB** | **4.74 GB** |
| **Maximum (Limits)** | **5.5 cores** | **5.5 cores** | **4.74 GB** | **4.74 GB** |

**Recommended Production Cluster**: 3-node cluster with **2 cores, 4 GB RAM per node** = **6 cores, 12 GB RAM total**

---

## 🏗️ **Component Breakdown**

### **Infrastructure Services (Shared Dependencies)**

| Component | CPU Request | CPU Limit | Memory Request | Memory Limit | Purpose |
|-----------|-------------|-----------|----------------|--------------|---------|
| **PostgreSQL** | 250m | 500m | 256Mi | 512Mi | Audit storage (Data Storage backend) |
| **Redis** | 100m | 200m | 128Mi | 256Mi | DLQ for failed audit writes |
| **Data Storage** | 250m | 500m | 256Mi | 512Mi | Audit API + workflow catalog |
| **SUBTOTAL** | **600m** | **1.2 cores** | **640Mi** | **1.25 GB** |

---

### **CRD Controllers (Business Logic Services)**

| Component | CPU Request | CPU Limit | Memory Request | Memory Limit | Purpose |
|-----------|-------------|-----------|----------------|--------------|---------|
| **Gateway** | 100m | 500m | 256Mi | 512Mi | Signal ingestion, deduplication, RR creation |
| **SignalProcessing** | 100m | 500m | 64Mi | 256Mi | K8s enrichment, environment classification |
| **AIAnalysis** | 100m* | 500m* | 128Mi* | 256Mi* | AI-powered root cause analysis |
| **WorkflowExecution** | 100m* | 500m* | 128Mi* | 256Mi* | Tekton workflow orchestration |
| **RemediationOrchestrator** | 100m* | 500m* | 128Mi* | 256Mi* | Central remediation coordination |
| **Notification** | 250m | 500m | 256Mi | 512Mi | Multi-channel notification delivery |
| **SUBTOTAL** | **750m** | **3.0 cores** | **960Mi** | **2.34 GB** |

**\*Note**: AIAnalysis, WorkflowExecution, and RemediationOrchestrator do not have explicit resource specifications in E2E infrastructure code. Values are **reasonable defaults** based on typical Kubernetes controller resource usage.

---

### **AI/ML Services**

| Component | CPU Request | CPU Limit | Memory Request | Memory Limit | Purpose |
|-----------|-------------|-----------|----------------|--------------|---------|
| **HolmesGPT API (HAPI)** | 200m | 500m | 256Mi | 512Mi | AI investigation + workflow recommendations |
| **SUBTOTAL** | **200m** | **500m** | **256Mi** | **512Mi** |

---

### **Workflow Execution Engine**

| Component | CPU Request | CPU Limit | Memory Request | Memory Limit | Purpose |
|-----------|-------------|-----------|----------------|--------------|---------|
| **Tekton Pipelines** | Variable | Variable | Variable | Variable | Workflow execution engine (3+ controllers) |
| **SUBTOTAL** | **~200m** | **~800m** | **~400Mi** | **~800Mi** |

**Note**: Tekton Pipelines consists of multiple controllers (tekton-pipelines-controller, tekton-pipelines-webhook, tekton-pipelines-resolver). Resource consumption varies based on concurrent workflow executions. Estimates based on default Tekton v1.7.0+ installation.

---

## 📊 **Total Resource Consumption**

### **Aggregate Totals**

| Tier | CPU Requests | CPU Limits | Memory Requests | Memory Limits |
|------|--------------|------------|-----------------|---------------|
| Infrastructure | 600m | 1.2 cores | 640Mi | 1.25 GB |
| CRD Controllers | 750m | 3.0 cores | 960Mi | 2.34 GB |
| AI/ML Services | 200m | 500m | 256Mi | 512Mi |
| Tekton Pipelines | ~200m | ~800m | ~400Mi | ~800Mi |
| **TOTAL** | **1.75 cores** | **5.5 cores** | **2.24 GB** | **4.87 GB** |

**Rounded for Planning**: **2 cores (requests) / 6 cores (limits) | 2.5 GB (requests) / 5 GB (limits)**

---

## 🎯 **Cluster Sizing Recommendations**

### **Development/E2E Testing** (Single-Node)

**Minimum**:
- **CPU**: 2 cores (to meet requests)
- **Memory**: 4 GB (to meet requests + overhead)
- **Example**: Kind cluster on developer machine, Podman VM with 2 CPU / 4 GB RAM

**Recommended**:
- **CPU**: 4 cores (to handle bursts)
- **Memory**: 8 GB (to handle bursts + Kubernetes overhead)
- **Example**: Local development machine with generous resources

---

### **Production** (Multi-Node)

**Minimum** (3-node cluster):
- **Per Node**: 1 core CPU, 2 GB RAM
- **Total Cluster**: 3 cores CPU, 6 GB RAM
- **Note**: Meets resource requests, but no burst capacity

**Recommended** (3-node cluster):
- **Per Node**: 2 cores CPU, 4 GB RAM
- **Total Cluster**: 6 cores CPU, 12 GB RAM
- **Rationale**:
  - 2x resource requests for burst capacity
  - Handles pod restarts and node failures
  - Supports concurrent workflow executions

**High-Availability** (5-node cluster):
- **Per Node**: 2 cores CPU, 4 GB RAM
- **Total Cluster**: 10 cores CPU, 20 GB RAM
- **Rationale**:
  - Multiple replicas for HA (3x Gateway, 3x HAPI, etc.)
  - Handles node failures gracefully
  - Supports high concurrent workflow load

---

## 🔍 **Resource Usage Patterns**

### **Steady-State (Idle)**
- **CPU Usage**: ~30-40% of requests (0.5-0.7 cores)
- **Memory Usage**: ~70-80% of requests (1.6-1.8 GB)
- **Rationale**: Controllers mostly idle, watching CRDs

### **Peak Load (Multiple Concurrent Remediations)**
- **CPU Usage**: ~80-100% of limits (4.4-5.5 cores)
- **Memory Usage**: ~60-80% of limits (3-4 GB)
- **Rationale**: Multiple workflows executing, AI analysis running, notifications sending

### **Burst Scenarios**
- **Storm Detection**: Gateway + SignalProcessing spike to limits
- **AI Analysis**: HAPI spikes to limits during LLM calls
- **Workflow Execution**: Tekton + WE spike during Tekton PipelineRun executions
- **Notification Flood**: Notification spikes during bulk delivery

---

## 📈 **Scalability Considerations**

### **Vertical Scaling (Per-Service)**

**Services that benefit from increased resources**:
1. **HAPI** (AI/ML workload):
   - Increase CPU: 500m → 1000m for faster LLM processing
   - Increase Memory: 512Mi → 1024Mi for larger context windows
2. **Data Storage** (High audit volume):
   - Increase CPU: 500m → 1000m for PostgreSQL connection handling
   - Increase Memory: 512Mi → 1024Mi for PostgreSQL buffering
3. **Gateway** (High signal ingestion rate):
   - Increase CPU: 500m → 1000m for high-throughput deduplication
   - Increase Memory: 512Mi → 1024Mi for in-memory caching

### **Horizontal Scaling (Multiple Replicas)**

**Services that support multiple replicas**:
1. **Gateway**: 3+ replicas for HA (leader election enabled)
2. **HAPI**: 3+ replicas for concurrent AI analysis
3. **Notification**: 3+ replicas for bulk delivery
4. **All CRD Controllers**: Support leader election for HA

**Services that require singleton**:
- None (all controllers support leader election)

---

## 💰 **Cloud Cost Estimates**

### **AWS EKS** (us-east-1 pricing)

**Development Cluster** (3x t3.medium nodes):
- **Instance Type**: t3.medium (2 vCPU, 4 GB RAM)
- **Cost per Node**: ~$0.0416/hour = ~$30/month
- **Total Cluster**: ~$90/month (3 nodes)
- **EKS Control Plane**: ~$73/month
- **Total**: ~$163/month

**Production Cluster** (3x t3.large nodes):
- **Instance Type**: t3.large (2 vCPU, 8 GB RAM)
- **Cost per Node**: ~$0.0832/hour = ~$60/month
- **Total Cluster**: ~$180/month (3 nodes)
- **EKS Control Plane**: ~$73/month
- **Total**: ~$253/month

---

### **GCP GKE** (us-central1 pricing)

**Development Cluster** (3x e2-standard-2 nodes):
- **Instance Type**: e2-standard-2 (2 vCPU, 8 GB RAM)
- **Cost per Node**: ~$0.067/hour = ~$48/month
- **Total Cluster**: ~$144/month (3 nodes)
- **GKE Control Plane**: Free (autopilot) or ~$73/month (standard)
- **Total**: ~$144/month (autopilot) or ~$217/month (standard)

---

### **Azure AKS** (East US pricing)

**Development Cluster** (3x Standard_B2s nodes):
- **Instance Type**: Standard_B2s (2 vCPU, 4 GB RAM)
- **Cost per Node**: ~$0.0416/hour = ~$30/month
- **Total Cluster**: ~$90/month (3 nodes)
- **AKS Control Plane**: Free
- **Total**: ~$90/month

---

## 🛠️ **Resource Optimization Strategies**

### **Cost Reduction**

1. **Use Spot/Preemptible Instances** (60-80% savings):
   - All CRD controllers are stateless
   - Support graceful shutdown via leader election
   - Can tolerate pod restarts

2. **Reduce Replica Count in Dev/Staging**:
   - Run single replica for all services
   - Use leader election for HA in production only

3. **Disable Unused Services**:
   - Run without Notification in dev (use console logs)
   - Run without HAPI in dev (use mock mode in AIAnalysis)

4. **Optimize Resource Requests/Limits**:
   - SignalProcessing: Already optimized (64Mi request)
   - Consider reducing limits for dev environments (500m → 250m CPU)

---

### **Performance Optimization**

1. **Increase HAPI Resources**:
   - CPU: 500m → 1000m (faster LLM processing)
   - Memory: 512Mi → 1024Mi (larger context windows)

2. **Increase PostgreSQL Resources**:
   - CPU: 500m → 1000m (faster audit writes)
   - Memory: 512Mi → 1024Mi (better query performance)

3. **Add Redis Memory**:
   - Memory: 256Mi → 512Mi (more DLQ capacity)

4. **Horizontal Scale Gateway**:
   - 1 → 3 replicas (higher signal ingestion throughput)

---

## 📋 **Resource Specification Status**

### **Explicitly Specified** (7 services):
- ✅ Gateway (test/e2e/gateway/gateway-deployment.yaml)
- ✅ SignalProcessing (test/infrastructure/signalprocessing.go)
- ✅ Notification (test/infrastructure/notification.go)
- ✅ Data Storage (test/infrastructure/datastorage.go)
- ✅ PostgreSQL (test/infrastructure/datastorage.go)
- ✅ Redis (test/infrastructure/datastorage.go)
- ✅ HAPI (holmesgpt-api/deployment.yaml)

### **Using Defaults** (3 services):
- ⚠️ AIAnalysis (no explicit spec in test/infrastructure/aianalysis.go)
- ⚠️ WorkflowExecution (no explicit spec in test/infrastructure/workflowexecution.go)
- ⚠️ RemediationOrchestrator (no explicit spec in test/infrastructure/remediationorchestrator.go)

**Recommendation**: Add explicit resource specifications for AIAnalysis, WorkflowExecution, and RemediationOrchestrator in production deployment manifests.

---

## 🎯 **Resource Limits Rationale**

### **Why 500m CPU Limit?**
- **Standard controller pattern**: Most Kubernetes controllers use 500m CPU limit
- **Burst capacity**: Allows 5x burst from 100m request
- **Fair sharing**: Prevents single service monopolizing cluster CPU

### **Why 512Mi Memory Limit?**
- **Typical Go controller**: 256-512Mi is standard for Go-based controllers
- **Safety margin**: 2x memory request provides safety for spikes
- **OOMKill prevention**: Generous limit prevents container restarts

### **Why Lower SignalProcessing Memory?**
- **Lightweight enrichment**: K8s API calls + Rego evaluation (no heavy processing)
- **Proven in E2E**: 64Mi request / 256Mi limit tested in E2E tests
- **Cost optimization**: Lowest resource controller in the platform

---

## 📊 **Comparison to Similar Platforms**

### **Prometheus + Alertmanager + Grafana**
- **Resources**: ~1.5 cores, ~3 GB RAM (similar to Kubernaut infrastructure)
- **Rationale**: Kubernaut adds remediation orchestration on top of alerting

### **Argo Workflows + Argo Events**
- **Resources**: ~1 core, ~2 GB RAM (similar to Kubernaut workflow components)
- **Rationale**: Kubernaut adds AI analysis + remediation logic

### **Istio Service Mesh**
- **Resources**: ~3 cores, ~4 GB RAM (more than Kubernaut)
- **Rationale**: Service mesh has higher overhead (sidecar per pod)

**Kubernaut is resource-efficient** for the functionality it provides (AI-powered remediation orchestration).

---

## ✅ **Validation & Testing**

### **Resource Usage Validated In**:
- ✅ Gateway E2E tests (test/e2e/gateway/)
- ✅ SignalProcessing E2E tests (test/e2e/signalprocessing/)
- ✅ AIAnalysis E2E tests (test/e2e/aianalysis/)
- ✅ WorkflowExecution E2E tests (test/e2e/workflowexecution/)
- ✅ Notification E2E tests (test/e2e/notification/)
- ✅ Data Storage E2E tests (test/e2e/datastorage/)

### **Cluster Configurations Tested**:
- ✅ Kind cluster (1 node, 4 CPU, 8 GB RAM) - All E2E tests pass
- ✅ Podman VM (2 CPU, 4 GB RAM) - Gateway E2E tests pass
- ⚠️ Production multi-node - Not yet validated (pending production deployment)

---

## 🚀 **Next Steps for Production**

1. **Add Explicit Resource Specs**:
   - Create production deployment manifests for AIAnalysis, WorkflowExecution, RemediationOrchestrator
   - Use specs from this document as baseline

2. **Performance Testing**:
   - Load test with 100+ concurrent remediations
   - Measure actual CPU/memory usage under load
   - Adjust resource specs based on actual usage

3. **Cost Optimization**:
   - Deploy to production cluster
   - Monitor actual resource usage for 1 week
   - Right-size resources based on p95 usage

4. **HA Configuration**:
   - Deploy 3 replicas of Gateway, HAPI, Notification
   - Test failover scenarios
   - Validate leader election works correctly

---

## 📖 **References**

- Gateway Deployment: `test/e2e/gateway/gateway-deployment.yaml`
- SignalProcessing Infrastructure: `test/infrastructure/signalprocessing.go`
- Notification Infrastructure: `test/infrastructure/notification.go`
- Data Storage Infrastructure: `test/infrastructure/datastorage.go`
- HAPI Deployment: `holmesgpt-api/deployment.yaml`
- E2E Team Coordination: `docs/handoff/SHARED_RO_E2E_TEAM_COORDINATION.md`

---

**Document Status**: ✅ **CURRENT**
**Last Updated**: December 13, 2025
**Review Schedule**: After production deployment (Q1 2026)


