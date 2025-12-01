# Kubernaut Services Documentation

**Version**: 2.1
**Last Updated**: 2025-12-01
**Purpose**: Navigation hub for all Kubernaut V1 service specifications

---

## 📋 Quick Navigation

### **🎯 CRD Controllers** (5 services)
Kubernetes controllers that reconcile Custom Resource Definitions:

1. **[Remediation Processor](./crd-controllers/01-signalprocessing/)** - Signal enrichment and context gathering
2. **[AI Analysis](./crd-controllers/02-aianalysis/)** - AI-powered root cause analysis
3. **[Workflow Execution](./crd-controllers/03-workflowexecution/)** - Multi-step workflow orchestration
4. **[Kubernetes Executor](./crd-controllers/04-kubernetesexecutor/)** - Kubernetes action execution with safety
5. **[Remediation Orchestrator](./crd-controllers/05-remediationorchestrator/)** - Orchestrates entire remediation workflow

### **🌐 HTTP Services** (5 services)
Stateless HTTP API services:

6. **[Gateway Service](./stateless/gateway-service/)** - ✅ **v1.0 PRODUCTION-READY** - Signal ingestion and triage (221 tests)
7. **[Data Storage](./stateless/data-storage/)** - ✅ **Phase 1 PRODUCTION-READY** - REST API Gateway for PostgreSQL with unified audit table (~535 tests)
8. **[HolmesGPT API](./stateless/holmesgpt-api/)** - ✅ **v3.2 PRODUCTION-READY** - AI investigation wrapper (172 tests)
9. **[Dynamic Toolset](./stateless/dynamic-toolset/)** - ⏸️ **Deferred to V2.0** - HolmesGPT toolset configuration (DD-016: V1.x uses static config)
10. **[Notification Controller](./crd-controllers/06-notification/)** - ✅ **PRODUCTION-READY** - CRD-based multi-channel delivery (249 tests: 140 unit + 97 integration + 12 E2E)

---

## 🏗️ Service Architecture

### **CRD Controllers** (Port 8080 health, Port 9090 metrics)
- Watch Kubernetes Custom Resources
- Event-driven reconciliation loops
- No REST APIs (use Kubernetes API)
- Metrics exposed on port 9090

**Common Pattern**: `controller-runtime` based reconcilers

### **HTTP Services** (Port 8080 API/health, Port 9090 metrics)
- Stateless HTTP REST APIs
- Kubernetes TokenReviewer authentication
- Correlation IDs for distributed tracing
- Prometheus metrics on port 9090

**Common Pattern**: `go.uber.org/zap` logging, standardized error handling

---

## 📊 Service Status Overview

| # | Service | Type | Status | Tests | BR Coverage |
|---|---------|------|--------|-------|-------------|
| 1 | Signal Processing | CRD | ⏸️ Phase 3 | - | - |
| 2 | AI Analysis | CRD | ⏸️ Phase 4 | - | - |
| 3 | Remediation Execution | CRD | ⏸️ Phase 3 | - | - |
| 4 | Remediation Orchestrator | CRD | ⏸️ Phase 5 | - | - |
| 5 | Gateway Service | HTTP | ✅ **v1.0 PRODUCTION-READY** | 221 (105U+114I+2E2E) | 20 BRs (100%) |
| 6 | Data Storage | HTTP | ✅ **Phase 1 PRODUCTION-READY** | ~535 (475U+60I) | 34 BRs (100%) |
| 7 | HolmesGPT API | HTTP | ✅ **v3.2 PRODUCTION-READY** | 172 (151U+21I) | 47 BRs (100%) |
| 8 | Dynamic Toolset | HTTP | ⏸️ **Deferred to V2.0** | - | 8 BRs (DD-016: static config in V1.x) |
| 9 | Notification Controller | CRD | ✅ **PRODUCTION-READY** | 249 (140U+97I+12E2E) | 12 BRs (100%) |
| 10 | ~~Context API~~ | HTTP | ❌ **DEPRECATED** | - | Replaced by Data Storage (DD-CONTEXT-006) |

**Overall**: ✅ **4/10 services** (40%) production-ready | **~1,177 tests** passing (100% pass rate)

---

## 🎯 Getting Started

### **For New Developers**
1. Start with [CRD Controllers README](./crd-controllers/README.md) (if applicable)
2. Or start with [Stateless Services README](./stateless/README.md) (if applicable)
3. Read individual service README.md for quick overview
4. Deep dive into specific documents as needed

**Total Time**: 15-30 minutes to understand any service

### **For Implementation**
1. Review service `overview.md` for architecture
2. Read `api-specification.md` for endpoints/schemas
3. Follow `implementation-checklist.md` for APDC-TDD approach
4. Check `testing-strategy.md` for test patterns

### **For Integration**
1. Read `integration-points.md` for dependencies
2. Review `api-specification.md` for endpoints
3. Check `security-configuration.md` for auth requirements

---

## 📁 Documentation Structure

### **CRD Controllers** (Directory per service)
```
crd-controllers/
├── 01-signalprocessing/
│   ├── README.md
│   ├── overview.md
│   ├── crd-schema.md
│   ├── controller-implementation.md
│   └── [10+ more focused documents]
└── [4 more services...]
```

### **HTTP Services** (Directory per service)
```
stateless/
├── gateway-service/
│   ├── README.md
│   ├── overview.md
│   ├── api-specification.md
│   └── [5+ more focused documents]
└── [5 more services...]
```

---

## 🔗 Cross-Cutting Documentation

### **Architecture**
- [CRD Schemas](../architecture/CRD_SCHEMAS.md) - Authoritative CRD definitions
- [Service Dependency Map](../architecture/SERVICE_DEPENDENCY_MAP.md) - Visual service interactions
- [Kubernetes TokenReviewer Auth](../architecture/KUBERNETES_TOKENREVIEWER_AUTH.md) - Authentication standard
- [Notification Payload Schema](../architecture/specifications/notification-payload-schema.md) - Escalation format
- [Logging Standard](../architecture/LOGGING_STANDARD.md) - Zap split strategy

### **Standards**
- [Prometheus ServiceMonitor Pattern](../architecture/PROMETHEUS_SERVICEMONITOR_PATTERN.md)
- [Prometheus AlertRules](../architecture/PROMETHEUS_ALERTRULES.md) - 58 production alerts
- [Log Correlation ID Standard](../architecture/LOG_CORRELATION_ID_STANDARD.md)
- [CRD Field Naming Convention](../architecture/CRD_FIELD_NAMING_CONVENTION.md)

---

## 🎯 Implementation Priorities

### **Phase 1: Foundation** (Weeks 1-2)
- Deploy PostgreSQL + Vector DB
- Deploy Redis
- Deploy Data Storage Service

### **Phase 2: Core Services** (Weeks 3-4)
- Deploy Gateway Service
- Deploy Remediation Orchestrator + Processor
- Deploy HolmesGPT API

### **Phase 3: Workflow & Execution** (Weeks 5-6)
- Deploy AI Analysis + Workflow Execution
- Deploy Kubernetes Executor
- Deploy Notification Service

### **Phase 4: Observability** (Week 7)
- ServiceMonitors + AlertRules
- Grafana dashboards
- Correlation ID validation

**Total**: 7-8 weeks to full production deployment

---

## 📊 Documentation Metrics

| Category | Count | Status |
|----------|-------|--------|
| **Services Active** | 10/10 | ✅ 100% (1 deprecated, 1 deferred to V2.0) |
| **Production-Ready** | 4/10 | 🟡 40% |
| **CRD Controllers** | 5 (1 ready) | 🟡 In Progress |
| **HTTP Services** | 5 (3 ready, 1 deferred) | 🟡 60% Complete |
| **Total Tests** | ~1,177 | ✅ 100% Pass Rate |
| **Total Test Coverage** | Unit 70%+ / Integration >50% / E2E <10% | ✅ Meeting Standards |

---

## 🔄 Documentation Versions

| Version | Date | Changes |
|---------|------|---------|
| **2.1** | 2025-12-01 | **DD-016 Integration**: Dynamic Toolset deferred to V2.0 (static config in V1.x); Updated service count to 4/10 production-ready (40%); Updated test count to ~1,177 (removed Dynamic Toolset's 245 tests); Corrected HTTP Services to 3 ready (60%); Fixed Implementation Priorities (removed Context API reference) |
| **2.0** | 2025-12-01 | Updated to reflect production-ready services: Gateway v1.0, Data Storage Phase 1, HolmesGPT API v3.2, Notification Controller (249 tests); Context API deprecated per DD-CONTEXT-006; Updated service count to 10 active services |
| **1.0** | 2025-10-06 | Initial top-level navigation hub created |

---

## 📞 Quick Links

- **Templates**: [CRD Service Specification Template](../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md)
- **Maintenance**: [MAINTENANCE_GUIDE.md](./crd-controllers/MAINTENANCE_GUIDE.md)
- **Triage Reports**: [COMPREHENSIVE_SERVICES_TRIAGE_REPORT.md](./COMPREHENSIVE_SERVICES_TRIAGE_REPORT.md)
- **Completion Summary**: [ALL_SERVICE_SPECIFICATIONS_COMPLETE.md](./ALL_SERVICE_SPECIFICATIONS_COMPLETE.md)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-12-01
**Status**: 🟡 Phase 2 In Progress - 4/10 Services Production-Ready (40%) | 1 Deferred to V2.0 (DD-016)
