# 🚀 KUBERNAUT V1 CRD-BASED ARCHITECTURE - IMPLEMENTATION GUIDE

**Document Version**: 4.0 - Service Module Organization
**Date**: January 2025
**Status**: Production-Ready Implementation Specifications
**Architecture Authority**: [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)

---

## 📋 OVERVIEW

This guide provides the **complete implementation roadmap** for Kubernaut's V1 CRD-based architecture with **declarative manifest reconciliation** for cloud-native alert remediation.

**Core Principle**: "Only services that interact with alerts or perform actions because of alerts require CRDs"

**Architecture**:
- **5 CRDs**: 1 central (AlertRemediation) + 4 service-specific
- **12 Services**: 5 with CRD controllers + 7 stateless support
- **Timeline**: 8 weeks to production
- **Confidence**: 98% (approved)

**Critical Benefits**:
- ✅ State persistence across service restarts
- ✅ Automatic failure recovery via reconciliation
- ✅ Network partition resilience
- ✅ Kubernetes-native operational model

---

## 🏗️ V1 SERVICE ARCHITECTURE

### **CATEGORY 1: SERVICES WITH CRD + RECONCILIATION CONTROLLERS (5)**

#### 📊 1. [Alert Processor Service](services/crd-controllers/01-alert-processor.md)
**Port**: 8081 | **CRD**: AlertProcessing | **Status**: ⚠️ **NEEDS CRD IMPLEMENTATION**

Alert enrichment, environment classification, and routing with reconciliation phases: `enriching → classifying → routing → completed`

**Business Requirements**: BR-SP-001 to BR-SP-050, BR-ENV-001 to BR-ENV-050
**Priority**: **P0 - HIGH** | **Effort**: 1 week

---

#### 🔍 2. [AI Analysis Service](services/crd-controllers/02-ai-analysis.md)
**Port**: 8082 | **CRD**: AIAnalysis | **Status**: ⚠️ **NEEDS CRD IMPLEMENTATION**

HolmesGPT investigation, AI analysis, and recommendation generation with phases: `investigating → analyzing → recommending → completed`

**Business Requirements**: BR-AI-001 to BR-AI-050, BR-WF-HOLMESGPT-001 to BR-WF-HOLMESGPT-005
**Priority**: **P0 - HIGH** | **Effort**: 1 week

---

#### 🎯 3. [Workflow Service](services/crd-controllers/03-workflow.md)
**Port**: 8083 | **CRD**: WorkflowExecution | **Status**: ⚠️ **NEEDS CRD IMPLEMENTATION**

Multi-step workflow orchestration and execution with phases: `planning → validating → executing → monitoring → completed`

**Business Requirements**: BR-WF-001 to BR-WF-165
**Priority**: **P0 - HIGH** | **Effort**: 1 week

---

#### ⚡ 4. [Kubernetes Executor Service](services/crd-controllers/04-kubernetes-executor.md)
**Port**: 8084 | **CRD**: KubernetesExecution (DEPRECATED - ADR-025) | **Status**: ⚠️ **NEEDS CRD IMPLEMENTATION**

Kubernetes operations execution with safety validation and phases: `validating → executing → verifying → completed`

**Business Requirements**: BR-EX-001 to BR-EX-155
**Priority**: **P0 - HIGH** | **Effort**: 1 week

---

#### 📊 5. [AlertRemediation Central Controller](services/crd-controllers/05-central-controller.md)
**CRD**: AlertRemediation | **Status**: ❌ **NEW - MUST IMPLEMENT FIRST**

Central state aggregation with:
- **Watch-based status updates** from all 4 service CRDs (event-driven)
- **Timeout management** (BR-ALERT-006) - Configurable timeout (default: 1h) with escalation
- **24-hour retention + cleanup** - Review window with automatic cleanup

**Note**: Duplicate alert detection is handled by Gateway Service (BR-WH-008), not Central Controller.

**Priority**: **P0 - CRITICAL** | **Effort**: 2 weeks
**Must implement BEFORE service CRDs**

---

### **CATEGORY 2: STATELESS SUPPORT SERVICES (7)**

Services without CRDs that provide infrastructure support:

#### 🔗 6. [Gateway Service](services/stateless/06-gateway.md)
**Port**: 8080 | **Status**: ✅ **PRODUCTION READY** (Enhancement: CRD creation)

HTTP webhook reception, **duplicate alert detection (BR-WH-008)**, **alert storm escalation (BR-ALERT-003, BR-ALERT-006)**, and CRD creation (AlertRemediation + AlertProcessing)

**Critical Responsibility**: Gateway is the **ONLY** service that performs duplicate detection. All downstream services receive only non-duplicate alerts.

**Business Requirements**: BR-WH-001 to BR-WH-026, BR-ALERT-003, BR-ALERT-005, BR-ALERT-006
**Priority**: **P1 - MEDIUM** | **Effort**: 3-4 hours

---

#### 📊 7. [Data Storage Service](services/stateless/07-data-storage.md)
**Port**: 8085 | **Status**: ✅ **FOUNDATION EXISTS** (HTTP wrapper needed)

PostgreSQL persistence and local vector database for permanent audit trail

**Business Requirements**: BR-STOR-001 to BR-STOR-135, BR-HIST-001 to BR-HIST-020
**Priority**: **P2 - LOW** | **Effort**: 3-4 hours

---

#### 🔍 8. [HolmesGPT-API Service](services/stateless/08-holmesgpt-api.md)
**Port**: 8090 | **Status**: ❌ **NEW SERVICE** (Python REST API)

Python REST API wrapper for HolmesGPT SDK with investigation endpoints

**Business Requirements**: BR-HAPI-001 to BR-HAPI-185
**Priority**: **P1 - HIGH** | **Effort**: 6-8 hours

---

#### 🌐 9. [Context Service](services/stateless/09-context.md)
**Port**: 8091 | **Status**: ❌ **NEW SERVICE**

Context retrieval and serving for enrichment

**Business Requirements**: BR-CTX-001 to BR-CTX-180
**Priority**: **P2 - LOW** | **Effort**: 2-3 hours

---

#### 🔍 10. [Intelligence Service](services/stateless/10-intelligence.md)
**Port**: 8086 | **Status**: ❌ **NEW SERVICE**

Pattern analysis and discovery on historical data

**Business Requirements**: BR-INT-001 to BR-INT-150
**Priority**: **P2 - LOW** | **Effort**: 3-4 hours

---

#### 📈 11. [Infrastructure Monitoring Service](services/stateless/11-infrastructure-monitoring.md)
**Port**: 8094 | **Status**: ❌ **NEW SERVICE**

System metrics collection and CRD monitoring

**Business Requirements**: BR-MET-001 to BR-MET-050, BR-OSC-001 to BR-OSC-020
**Priority**: **P2 - LOW** | **Effort**: 2-3 hours

---

#### 📢 12. [Notification Service](services/stateless/12-notification.md)
**Port**: 8089 | **Status**: ✅ **FOUNDATION EXISTS** (HTTP wrapper needed)

Multi-channel notification delivery

**Business Requirements**: BR-NOTIF-001 to BR-NOTIF-120
**Priority**: **P2 - LOW** | **Effort**: 2-3 hours

---

## 🚀 IMPLEMENTATION ROADMAP

### **PHASE 0: CRD Framework Setup (Week 1)**

**Objective**: Establish Kubebuilder framework and development environment

**Tasks**:
1. Install Kubebuilder v3.12+ and controller-runtime v0.16+
2. Setup KIND cluster for CRD testing
3. Configure Prometheus for controller metrics
4. Generate initial CRD scaffolds

**Success Criteria**: ✅ Working CRD development environment

---

### **PHASE 1: Central Coordination (Weeks 2-3)**

**Objective**: Implement AlertRemediation CRD and central controller

**Week 2**: Define schema, implement basic reconciliation, add CRD reference management
**Week 3**: Watch configuration, duplicate handling, timeout management, 24h cleanup

**Deliverables**: Fully functional central coordination controller
**Reference**: [Central Controller Specification](services/crd-controllers/05-central-controller.md)

---

### **PHASE 2: Service CRDs - Alert Processing (Weeks 4-5)**

**Objective**: Implement AlertProcessing and AIAnalysis CRDs

**Week 4**: [Alert Processor](services/crd-controllers/01-alert-processor.md) - Enrichment & classification
**Week 5**: [AI Analysis](services/crd-controllers/02-ai-analysis.md) - HolmesGPT integration & recommendations

**Deliverables**: Alert enrichment and AI analysis reconciliation working

---

### **PHASE 3: Service CRDs - Workflow & Execution (Weeks 6-7)**

**Objective**: Implement WorkflowExecution and KubernetesExecution (DEPRECATED - ADR-025) CRDs

**Week 6**: [Workflow Service](services/crd-controllers/03-workflow.md) - Orchestration logic
**Week 7**: [Kubernetes Executor](services/crd-controllers/04-kubernetes-executor.md) - Infrastructure operations

**Deliverables**: Complete end-to-end CRD-based remediation pipeline

---

### **PHASE 4: Production Readiness (Week 8)**

**Objective**: End-to-end validation and production deployment

**Tasks**:
- End-to-end CRD flow validation
- Performance testing (2500 alerts/min)
- Duplicate alert storm testing
- Timeout and escalation testing
- Security hardening and RBAC
- Production deployment

**Success Criteria**: ✅ Production-ready CRD-based architecture

---

## 📊 SUCCESS CRITERIA

### **Technical Validation**
- ✅ All 5 CRDs deployed and reconciling
- ✅ Watch-based status updates: <1s latency
- ✅ CRD reconciliation success rate: >99.9%
- ✅ Automatic failure recovery: 100% coverage
- ✅ 24-hour cleanup execution: 100% success

### **Business Value**
- ✅ Duplicate alert suppression: >95% noise reduction
- ✅ Timeout escalation: 100% detection and notification
- ✅ State persistence: Zero state loss on restarts
- ✅ Network resilience: Tolerates partitions
- ✅ Production readiness: 99.9% availability

### **Performance Targets**
- Alert processing: <5s end-to-end
- CRD reconciliation: <100ms per loop
- Status aggregation: <1s latency
- Cleanup execution: <30s per CRD set

---

## 📚 DETAILED SERVICE SPECIFICATIONS

For complete implementation details, see the [Service Specifications Directory](services/):

### CRD Controllers
1. [Alert Processor](services/crd-controllers/01-alert-processor.md) - Complete CRD schema, controller implementation, testing
2. [AI Analysis](services/crd-controllers/02-ai-analysis.md) - HolmesGPT integration patterns
3. [Workflow](services/crd-controllers/03-workflow.md) - Orchestration and dependency resolution
4. [Kubernetes Executor](services/crd-controllers/04-kubernetes-executor.md) - Safety validation and rollback
5. [Central Controller](services/crd-controllers/05-central-controller.md) - Watch configuration and aggregation

### Stateless Services
6. [Gateway](services/stateless/06-gateway.md) - CRD creation and duplicate detection
7. [Data Storage](services/stateless/07-data-storage.md) - Audit trail and vector DB
8. [HolmesGPT-API](services/stateless/08-holmesgpt-api.md) - Python SDK wrapper
9. [Context](services/stateless/09-context.md) - Context serving
10. [Intelligence](services/stateless/10-intelligence.md) - Pattern analysis
11. [Infrastructure Monitoring](services/stateless/11-infrastructure-monitoring.md) - Metrics collection
12. [Notification](services/stateless/12-notification.md) - Multi-channel delivery

---

## 🎯 QUICK START GUIDE

### **1. Install Prerequisites**
```bash
# Install Kubebuilder
curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/

# Install KIND
go install sigs.k8s.io/kind@latest

# Create development cluster
kind create cluster --name kubernaut-dev
```

### **2. Initialize Project**
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
kubebuilder init --domain kubernaut.io --repo github.com/jordigilh/kubernaut
```

### **3. Generate First CRD (AlertRemediation)**
```bash
kubebuilder create api --group kubernaut --version v1 --kind AlertRemediation
```

### **4. Deploy CRD to Cluster**
```bash
make manifests
make install
kubectl get crds
```

### **5. Run Controller Locally**
```bash
make run
```

---

## 📚 REFERENCE DOCUMENTS

**Primary Architecture Authority**:
- [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md) - **AUTHORITATIVE** CRD specification
- [APPROVED_MICROSERVICES_ARCHITECTURE.md](../architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md) - V1 service specifications
- [KUBERNAUT_ARCHITECTURE_OVERVIEW.md](../architecture/KUBERNAUT_ARCHITECTURE_OVERVIEW.md) - High-level system design

**Business Requirements**:
- Alert Processing: BR-SP-001 to BR-SP-050
- Environment Classification: BR-ENV-001 to BR-ENV-050
- AI Analysis: BR-AI-001 to BR-AI-050
- Workflow: BR-WF-001 to BR-WF-165
- Kubernetes Execution: BR-EX-001 to BR-EX-155
- Duplicate Handling: BR-WH-008, BR-ALERT-003
- Timeout Management: BR-ALERT-006

**Technical Documentation**:
- [Kubebuilder Book](https://book.kubebuilder.io/) - CRD development guide
- [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime) - Reconciliation framework

---

## 🚨 CRITICAL SUCCESS FACTORS

### **1. CRD-Based Architecture (Non-Negotiable)**
**DO**: Implement reconciliation loops, use Kubernetes watches (event-driven), maintain 24-hour CRD retention
**DON'T**: Use HTTP/REST for alert-processing coordination, poll for status, keep CRDs indefinitely

### **2. Service Responsibility Boundaries**
**Services WITH CRDs (5)**: Alert Processor, AI Analysis, Workflow, Executor, Central Controller
**Services WITHOUT CRDs (7)**: Gateway, Storage, HolmesGPT-API, Context, Intelligence, Monitor, Notification

### **3. Dual Audit System**
**CRD System (Temporary - 24h)**: Real-time execution + review window
**Database System (Permanent)**: Complete audit trail for compliance

### **4. Production Resilience**
**Required**: State persistence, automatic recovery, network partition tolerance, duplicate suppression, timeout detection

---

## 🔍 GAP ANALYSIS

### Current Implementation vs CRD Architecture

**Completed**:
- ✅ Business logic for all services (pkg/ directory)
- ✅ HTTP-based alert processing foundation
- ✅ Environment classification logic
- ✅ HolmesGPT integration patterns
- ✅ ActionExecutor infrastructure
- ✅ Safety validation framework

**Missing - CRITICAL**:
- ❌ All 5 CRD schemas
- ❌ All 5 reconciliation controllers
- ❌ Watch-based status aggregation
- ❌ Duplicate alert handling in Gateway
- ❌ Timeout detection and escalation
- ❌ 24-hour CRD lifecycle management
- ❌ Kubebuilder framework setup

**Gap Impact**: **CRITICAL** - Current HTTP-based approach lacks production resilience features. CRD implementation is **mandatory** for production deployment per MULTI_CRD_RECONCILIATION_ARCHITECTURE.md.

---

**🚀 Ready to implement V1 CRD-based architecture! Start with Phase 0: Kubebuilder framework setup, then proceed to Phase 1: Central Controller implementation.**

**For detailed service-by-service specifications, see the [services/ directory](services/).**
