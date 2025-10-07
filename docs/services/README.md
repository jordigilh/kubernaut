# Kubernaut Services Documentation

**Version**: 1.0
**Last Updated**: 2025-10-06
**Purpose**: Navigation hub for all Kubernaut V1 service specifications

---

## 📋 Quick Navigation

### **🎯 CRD Controllers** (5 services)
Kubernetes controllers that reconcile Custom Resource Definitions:

1. **[Remediation Processor](./crd-controllers/01-remediationprocessor/)** - Signal enrichment and context gathering
2. **[AI Analysis](./crd-controllers/02-aianalysis/)** - AI-powered root cause analysis
3. **[Workflow Execution](./crd-controllers/03-workflowexecution/)** - Multi-step workflow orchestration
4. **[Kubernetes Executor](./crd-controllers/04-kubernetesexecutor/)** - Kubernetes action execution with safety
5. **[Remediation Orchestrator](./crd-controllers/05-remediationorchestrator/)** - Orchestrates entire remediation workflow

### **🌐 HTTP Services** (6 services)
Stateless HTTP API services:

6. **[Gateway Service](./stateless/gateway-service/)** - Signal ingestion and triage
7. **[Context API](./stateless/context-api/)** - Historical intelligence and pattern matching
8. **[Data Storage](./stateless/data-storage/)** - Audit trail persistence and embeddings
9. **[HolmesGPT API](./stateless/holmesgpt-api/)** - AI investigation engine
10. **[Notification Service](./stateless/notification-service/)** - Escalation and notification routing
11. **[Dynamic Toolset](./stateless/dynamic-toolset/)** - HolmesGPT toolset configuration management

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

| # | Service | Type | Status | Priority | Docs Complete |
|---|---------|------|--------|----------|---------------|
| 1 | Remediation Processor | CRD | ⏸️ Design | P0 | ✅ 100% |
| 2 | AI Analysis | CRD | ⏸️ Design | P0 | ✅ 100% |
| 3 | Workflow Execution | CRD | ⏸️ Design | P0 | ✅ 100% |
| 4 | Kubernetes Executor | CRD | ⏸️ Design | P0 | ✅ 100% |
| 5 | Remediation Orchestrator | CRD | ⏸️ Design | P0 | ✅ 100% |
| 6 | Gateway Service | HTTP | ⏸️ Design | P0 | ✅ 100% |
| 7 | Context API | HTTP | ⏸️ Design | P1 | ✅ 100% |
| 8 | Data Storage | HTTP | ⏸️ Design | P1 | ✅ 100% |
| 9 | HolmesGPT API | HTTP | ⏸️ Design | P0 | ✅ 100% |
| 10 | Notification Service | HTTP | ⏸️ Design | P0 | ✅ 100% |
| 11 | Dynamic Toolset | HTTP | ⏸️ Design | P1 | ✅ 100% |

**Overall**: ✅ **11/11 services** (100%) documentation complete, ready for implementation

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
├── 01-remediationprocessor/
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
- Deploy Data Storage + Context API

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
| **Services Documented** | 11/11 | ✅ 100% |
| **CRD Controllers** | 5/5 | ✅ Complete |
| **HTTP Services** | 6/6 | ✅ Complete |
| **Architecture Docs** | 12+ | ✅ Complete |
| **Total Documents** | 50+ | ✅ Complete |
| **Total Lines** | 11,700+ | ✅ Complete |

---

## 🔄 Documentation Versions

| Version | Date | Changes |
|---------|------|---------|
| **1.0** | 2025-10-06 | Initial top-level navigation hub created |

---

## 📞 Quick Links

- **Templates**: [CRD Service Specification Template](../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md)
- **Maintenance**: [MAINTENANCE_GUIDE.md](./crd-controllers/MAINTENANCE_GUIDE.md)
- **Triage Reports**: [COMPREHENSIVE_SERVICES_TRIAGE_REPORT.md](./COMPREHENSIVE_SERVICES_TRIAGE_REPORT.md)
- **Completion Summary**: [ALL_SERVICE_SPECIFICATIONS_COMPLETE.md](./ALL_SERVICE_SPECIFICATIONS_COMPLETE.md)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete Navigation Hub
