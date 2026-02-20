# CRD Controllers Documentation

**Last Updated**: 2026-02-16
**Structure Version**: 1.2
**Documentation Status**: ✅ Active

---

## 📋 Overview

This directory contains comprehensive documentation for all **7 CRD controller services** in Kubernaut V1. Each service has its own self-contained directory with 13-15 focused documents following a consistent structure.

---

## 🚀 Service Implementations

### **Core CRD Controllers**

| Service | Directory | Status | Description |
|---------|-----------|--------|-------------|
| **Signal Processing** | [01-signalprocessing/](./01-signalprocessing/) | ✅ Active | Signal ingestion and initial processing |
| **AI Analysis** | [02-aianalysis/](./02-aianalysis/) | ✅ Active | Root cause analysis and recommendations |
| **Workflow Execution** | [03-workflowexecution/](./03-workflowexecution/) | ✅ Active | Workflow planning and orchestration |
| **Kubernetes Executor** | [04-kubernetesexecutor/](./04-kubernetesexecutor/) | ✅ Active | Safe Kubernetes action execution |
| **Remediation Orchestrator** | [05-remediationorchestrator/](./05-remediationorchestrator/) | ✅ Active | Multi-CRD lifecycle coordination |
| **Notification** | [06-notification/](./06-notification/) | 🚧 In Development | Multi-channel notification delivery with CRD persistence |
| **Effectiveness Monitor** | [07-effectivenessmonitor/](./07-effectivenessmonitor/) | ✅ Active (V1.0) | Post-remediation effectiveness assessment (health, alerts, metrics, spec drift) |

### **Code Coverage (PR #90, 2026-02-16)**

| Service | Unit | Integration | E2E | All Tiers |
|---------|------|-------------|-----|-----------|
| Signal Processing | 87.3% | 61.4% | 58.2% | 85.4% |
| AI Analysis | 80.0% | 73.6% | 53.8% | 87.6% |
| Workflow Execution | 74.0% | 67.9% | 56.0% | 82.9% |
| Kubernetes Executor | — | — | — | — |
| Remediation Orchestrator | 79.9% | 59.8% | 49.1% | 82.1% |
| Notification | 75.5% | 57.6% | 49.5% | 73.3% |
| Effectiveness Monitor | 72.1% | 64.9% | 68.8% | 81.9% |

> **Targets**: Unit ≥80%, Integration ≥80%, All Tiers ≥80%.
> Coverage is per-tier against the tier-specific code subset (see `scripts/coverage/coverage_report.py`).

---

## 📖 How to Navigate Service Documentation

Each service directory follows a **consistent 14-file structure** for easy navigation:

### **Standard Files in Each Service** (Common Pattern)

1. **README.md** - Service navigation hub (start here!)
2. **overview.md** - Architecture, key decisions, and business requirements
3. **crd-schema.md** - CRD type definitions and field specifications
4. **controller-implementation.md** - Reconciler logic and core implementation
5. **reconciliation-phases.md** - Phase transitions and state machine
6. **finalizers-lifecycle.md** - Cleanup logic and lifecycle management
7. **testing-strategy.md** - Unit/Integration/E2E test patterns
8. **security-configuration.md** - RBAC, network policies, secrets
9. **observability-logging.md** - Structured logging and distributed tracing
10. **metrics-slos.md** - Prometheus metrics and Grafana dashboards
11. **database-integration.md** - Audit storage and persistence patterns
12. **integration-points.md** - Service coordination and dependencies
13. **migration-current-state.md** - Current code analysis
14. **implementation-checklist.md** - APDC-TDD implementation phases

### **Quick Start**

```bash
# Navigate to a service
cd 01-signalprocessing/

# Read README first (provides navigation)
cat README.md

# Read overview for high-level architecture
cat overview.md

# Deep dive into specific aspects as needed
cat controller-implementation.md
cat testing-strategy.md
```

---

## 🚀 Phase 3 Implementation Planning

### **Quick Start**

- **[Phase 3 Final Summary](./CATEGORY1_FINAL_SUMMARY.md)** - Complete achievement summary (97% avg confidence)
- **[Implementation Quick Reference](./QUICK_REFERENCE_NEXT_STEPS.md)** - Next steps, make targets, timelines

**Status**: ✅ All 3 Phase 3 services ready for implementation (Signal Processing, Workflow Execution, Kubernetes Executor)

---

## 📚 Meta-Documentation

### **Planning Records** → [`planning/`](./planning/)

Historical planning session records and expansion summaries:
- Session 2: Workflow Execution expansion (Part 1)
- Session 3: Workflow Execution expansion (Part 2)
- Session 4: Kubernetes Executor expansion
- Session wrap-up and ROI analysis
- Expansion plans summary

### **Testing Documentation** → [`testing/`](./testing/)

Testing architecture, strategies, and infrastructure:
- Integration test architecture (Hybrid Envtest/Kind approach)
- BR coverage correction (Defense-in-depth strategy)
- Envtest vs Kind assessment
- Infrastructure assessment and setup

### **Implementation Standards** → [`standards/`](./standards/)

Coding standards, patterns, and best practices:
- Go code standards for implementation plans
- Edge cases and error handling philosophy
- Precondition/postcondition validation framework
- Make targets and infrastructure setup
- Gap closure implementation (anti-flaky patterns, etc.)

### **Operational Guides** → [`operations/`](./operations/)

Production deployment and maintenance procedures:
- Production deployment guide (manifests, RBAC, monitoring)
- Maintenance guide for CRD controllers

### **Administrative** → [`admin/`](./admin/)

Meta-documentation and administrative records:
- Document cleanup summary (48 ephemeral docs removed)
- Document organization proposal (approved Option A)

### **Analysis & Triage Reports**

Analysis documents that assess all 7 CRD controllers collectively:

- **[CRD Controllers Triage Report](../../analysis/CRD_CONTROLLERS_TRIAGE_REPORT.md)**
  Identifies inconsistencies, risks, pitfalls, and gaps across all controllers
  _Status: 2 CRITICAL + 5 HIGH + 4 MEDIUM + 3 LOW issues identified_

- **[Critical Decisions Recommendations](../../analysis/CRD_CRITICAL_DECISIONS_RECOMMENDATIONS.md)**
  Data-driven recommendations for critical business requirement decisions
  _Status: Options analysis for WorkflowExecution BR approaches_

### **Development Resources**

Resources for developing and maintaining CRD controller documentation:

- **[CRD Service Specification Template](../../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md)**
  Standard template for creating new CRD controller service specifications
  _Use this when adding new CRD controller services_

### **Historical Reference**

- **[Archive](./archive/)**
  Superseded monolithic documents (5,000+ lines each)
  _For historical reference only - use structured directories instead_

---

## 🎯 Common Tasks

### **Understanding a Specific Service**

1. Navigate to service directory: `cd 01-signalprocessing/`
2. Read `README.md` for navigation and quick start
3. Read `overview.md` for architecture and key decisions
4. Deep dive into specific documents as needed

**Estimated Time**: 30 minutes to understand a service (vs 2+ hours with monolithic docs)

---

### **Adding a New CRD Controller Service**

1. Read [Maintenance Guide](./MAINTENANCE_GUIDE.md) → "Adding New Service" section
2. Copy directory structure from existing service (e.g., `01-signalprocessing/`)
3. Use [CRD Service Template](../../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md) for content guidance
4. Follow the 14-file structure for consistency

**Estimated Effort**: 3-5 days for comprehensive documentation

---

### **Updating Common Patterns**

Common patterns (testing, security, observability, metrics) are **duplicated across services** with clear markers:

```markdown
<!-- COMMON-PATTERN: This file is duplicated across all CRD services -->
<!-- LAST-UPDATED: 2025-01-15 -->
```

**Process**:
1. Update pattern in pilot service (`01-signalprocessing/`)
2. Propagate to other services using guidance in [Maintenance Guide](./MAINTENANCE_GUIDE.md)
3. Update `LAST-UPDATED` header in all files

---

## 📊 Documentation Metrics

| Metric | Before (Archive) | After (Structured) | Improvement |
|--------|------------------|-------------------|-------------|
| **Max Document Size** | 5,249 lines | 916 lines | 82% reduction |
| **Avg Document Size** | 3,862 lines | 735 lines | 81% reduction |
| **Time to Understand** | 2+ hours | 30 min | 75% faster |
| **Merge Conflicts** | Frequent | Rare | ⭐⭐⭐⭐⭐ |
| **Files per Service** | 1 monolithic | 14 focused | ⭐⭐⭐⭐⭐ |

---

## 🔍 Finding Specific Information

### **By Topic**

| What You Need | Where to Look |
|---------------|---------------|
| **Architecture decisions** | `overview.md` in each service |
| **CRD field definitions** | `crd-schema.md` in each service |
| **Reconciliation logic** | `controller-implementation.md` in each service |
| **Testing patterns** | `testing-strategy.md` in each service |
| **Security configuration** | `security-configuration.md` in each service |
| **Metrics & monitoring** | `metrics-slos.md` in each service |
| **Service dependencies** | `integration-points.md` in each service |
| **Cross-service issues** | [Triage Report](../../analysis/CRD_CONTROLLERS_TRIAGE_REPORT.md) |
| **BR decision recommendations** | [Critical Decisions](../../analysis/CRD_CRITICAL_DECISIONS_RECOMMENDATIONS.md) |

### **By Service**

Each service README provides a **navigation hub** with links to all documents. Start there!

Example: [01-signalprocessing/README.md](./01-signalprocessing/README.md)

---

## 🔗 Related Documentation

### **Project-Level Documentation**
- [Architecture Overview](../../architecture/KUBERNAUT_ARCHITECTURE_OVERVIEW.md)
- [Service Catalog](../../architecture/KUBERNAUT_SERVICE_CATALOG.md)
- [Testing Strategy](../../testing/)

### **Implementation Guides**
- [Development Workflow](../../development/)
- [Deployment Guide](../../deployment/)
- [Troubleshooting](../../architecture/TROUBLESHOOTING_GUIDE.md)

---

## 💡 Key Principles

### **1. Self-Containment**
Everything for a service lives in one directory - no need to hunt across multiple locations.

### **2. Progressive Disclosure**
- **README** → Quick start and navigation
- **overview.md** → Architecture and decisions
- **Detailed files** → Deep dive into specific aspects

### **3. Controlled Duplication**
Common patterns duplicated with clear markers for easy maintenance.

### **4. Parallel-Friendly**
14+ files per service = zero merge conflicts when multiple developers work simultaneously.

---

## 📞 Questions or Issues?

### **Documentation Structure**
- See: [Maintenance Guide](./MAINTENANCE_GUIDE.md)
- Contact: Kubernaut Documentation Team

### **Service Implementation**
- See service-specific README (e.g., [01-signalprocessing/README.md](./01-signalprocessing/README.md))
- Refer to: [Implementation Checklist](./01-signalprocessing/implementation-checklist.md)

### **Cross-Service Issues**
- See: [Triage Report](../../analysis/CRD_CONTROLLERS_TRIAGE_REPORT.md)
- Refer to: [Critical Decisions](../../analysis/CRD_CRITICAL_DECISIONS_RECOMMENDATIONS.md)

---

## 🎯 Success Metrics

**Documentation is successful when**:
- ✅ New developers understand a service in **30 minutes** (not 2+ hours)
- ✅ Zero merge conflicts in documentation (parallel work enabled)
- ✅ Focused documents (**<1,000 lines** each, not 5,000+)
- ✅ Easy navigation (README hub + clear structure)

---

**Maintainer**: Kubernaut Documentation Team
**Last Restructure**: 2025-01-15
**Relocation Update**: 2025-10-12
**Document Reorganization**: 2025-10-14 (85% reduction in root-level files)
**EM V1.0 Integration**: 2026-02-12 (7th service: Effectiveness Monitor)
**Coverage Update**: 2026-02-16 (PR #90: added per-service coverage table)
**Status**: ✅ Active and maintained

