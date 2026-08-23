# Stateless HTTP Services Documentation

**Version**: 1.0
**Last Updated**: 2025-10-06
**Service Type**: HTTP REST APIs
**Purpose**: Navigation hub for all HTTP-based stateless services

---

## 📋 Quick Navigation

> **Kubernaut Agent (KA) has moved**: KA is no longer a pure stateless HTTP service — per
> [DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md), AIAnalysis
> now dispatches KA via the `AgentSession` CRD (watch + Lease), not HTTP submit/poll. Its docs have
> moved to the top-level [`docs/services/kubernaut-agent/`](../kubernaut-agent/) alongside the
> project's other hybrid/platform services. It is intentionally **not** listed below.

### **HTTP Services** (5 services)

1. **[Gateway Service](./gateway-service/)** - Signal ingestion and triage (Design B: adapter endpoints)
2. **[Context API](./context-api/)** - Historical intelligence and pattern matching (read-only)
3. **[Data Storage](./data-storage/)** - Audit trail persistence and embeddings (write-only)
4. **~~[Notification Service](./notification-service/)~~** - ⚠️ DEPRECATED (migrated to CRD Controller, see [06-notification](../crd-controllers/06-notification/))
5. **[Dynamic Toolset](./dynamic-toolset/)** - Kubernaut Agent toolset configuration management

---

## 🏗️ Common Architecture Patterns

### **All HTTP Services Share**:
- **Port 8080**: REST API, `/health`, `/ready` endpoints
- **Port 9090**: Prometheus `/metrics` (with TokenReviewer auth)
- **Authentication**: Kubernetes TokenReviewer for API endpoints
- **Logging**: `go.uber.org/zap` (structured logging)
- **Tracing**: X-Correlation-ID for distributed tracing
- **Error Handling**: Standardized error responses
- **No State**: Stateless (data in PostgreSQL/Redis/Vector DB)

---

## 📊 Service Overview

| Service | Purpose | Port | Status | Docs |
|---------|---------|------|--------|------|
| **Gateway** | Signal ingestion | 8080 | ⏸️ Design | ✅ 100% |
| **Context API** | Historical queries (read) | 8080 | ⏸️ Design | ✅ 100% |
| **Data Storage** | Audit persistence (write) | 8080 | ⏸️ Design | ✅ 100% |
| **Notification** | Escalation delivery | 8080 | ⏸️ Design | ✅ 100% |
| **Dynamic Toolset** | Toolset config | 8080 | ⏸️ Design | ✅ 100% |

> **Kubernaut Agent** moved to [`docs/services/kubernaut-agent/`](../kubernaut-agent/) (hybrid: `AgentSession` CRD-dispatch + MCP) — see note above.

---

## 🎯 Service Responsibilities

### **Gateway Service** (Entry Point)
- **Receives**: Prometheus alerts, Kubernetes events
- **Normalizes**: All signals to `NormalizedSignal` format
- **Creates**: `RemediationRequest` CRD
- **Features**: Deduplication, storm detection, priority assignment

### **Context API** (Read Layer)
- **Provides**: Historical intelligence for decision-making
- **Queries**: Environment context, pattern matching, success rates
- **Caching**: Multi-tier (in-memory + Redis)
- **Clients**: Remediation Processor, AI Analysis, Workflow Execution

### **Data Storage** (Write Layer)
- **Persists**: Audit trail for all remediation activities
- **Generates**: Vector embeddings for semantic search
- **Storage**: Dual-write (PostgreSQL + Vector DB)
- **Clients**: All CRD controllers write audit data here

### **Notification Service** (Output)
- ⚠️ **DEPRECATED**: Service migrated to CRD Controller (2025-10-12)
- **See**: [06-notification](../crd-controllers/06-notification/) for current implementation
- **Reason**: Zero data loss, complete audit trail, automatic retry with CRD-based persistence
- **Formats**: Channel-specific adapters

### **Dynamic Toolset** (Configuration)
- **Discovers**: Available Kubernetes resources
- **Generates**: Kubernaut Agent toolset configurations
- **Updates**: ConfigMaps dynamically
- **Validates**: Toolset compatibility

---

## 🔗 Service Dependencies

```
Signal Sources (Prometheus, K8s Events)
    ↓
Gateway Service (adapter endpoints)
    ↓
RemediationRequest CRD (Kubernetes)
    ↓
CRD Controllers
    ├─→ Read: Context API
    └─→ Write: Data Storage
        ↓
    PostgreSQL + Vector DB
        ↓
    Context API (reads) → Controllers
```

---

## 📁 Directory Structure

```
stateless/
├── README.md                          ← You are here
│                                       (kubernaut-agent/ moved to ../kubernaut-agent/, see note above)
├── gateway-service/                   ✅ 20+ documents
│   ├── README.md
│   ├── overview.md
│   ├── api-specification.md
│   └── [17+ more documents]
├── context-api/                       ✅ 3 documents
│   ├── README.md                      ✅ NEW
│   ├── overview.md
│   ├── api-specification.md
│   └── database-schema.md
├── data-storage/                      ✅ 2 documents
│   ├── README.md                      ✅ NEW
│   ├── overview.md
│   └── api-specification.md
├── notification-service/              ✅ Directory + subdirs
│   ├── README.md                      ✅ NEW
│   ├── triage/                        (2 files)
│   ├── solutions/                     (3 files)
│   ├── revisions/                     (1 file)
│   └── summaries/                     (6 files)
└── dynamic-toolset/                   ✅ 2 documents
    ├── README.md                      ✅ NEW
    ├── overview.md
    └── api-specification.md
```

---

## 🎯 Getting Started

### **For Implementation**
1. Read service `README.md` for quick overview
2. Review `overview.md` for architecture
3. Study `api-specification.md` for endpoints
4. Follow APDC-TDD methodology

**Total Time**: 30-60 minutes per service

### **For Integration**
1. Check service `README.md` for dependencies
2. Review `api-specification.md` for API contracts
3. Understand authentication (TokenReviewer)
4. Implement correlation ID propagation

---

## 🔒 Security Standards

### **All HTTP Services Use**:
- **Authentication**: Kubernetes TokenReviewer API
- **Authorization**: RBAC via ServiceAccount
- **TLS**: Between services (Istio/service mesh)
- **Secrets**: Kubernetes Secrets (mounted, not env vars)
- **Sanitization**: Remove sensitive data before logging

**See**: [KUBERNETES_TOKENREVIEWER_AUTH.md](../../architecture/KUBERNETES_TOKENREVIEWER_AUTH.md)

---

## 📊 Performance Targets

| Service | Latency (p95) | Throughput | Scaling |
|---------|---------------|------------|---------|
| **Gateway** | < 200ms | 100 req/s | 2-4 replicas |
| **Context API** | < 200ms | 50 req/s | 2-4 replicas |
| **Data Storage** | < 250ms | 50 req/s | 2-3 replicas |
| **Notification** | < 1s | 20 req/s | 2-3 replicas |
| **Dynamic Toolset** | < 300ms | 5 req/s | 1-2 replicas |

> Kubernaut Agent's performance targets now live in [`docs/services/kubernaut-agent/metrics-slos.md`](../kubernaut-agent/metrics-slos.md) (no longer a request/s-shaped HTTP SLO, since dispatch is CRD-based).

---

## 🔄 Documentation Standards

### **Each Service Directory Contains**:
- ✅ `README.md` - Navigation hub
- ✅ `overview.md` - Architecture and design
- ✅ `api-specification.md` - REST API contracts
- ✅ `security-configuration.md` (optional)
- ✅ `observability-logging.md` (optional)
- ✅ `testing-strategy.md` (optional)

**Consistency**: All services follow same structure for easy navigation

---

## 📞 Quick Links

- **Architecture**: [../architecture/](../../architecture/)
- **CRD Controllers**: [../crd-controllers/](../crd-controllers/)
- **Triage Reports**: [STATELESS_SERVICES_TRIAGE.md](./STATELESS_SERVICES_TRIAGE.md)
- **Completion Summary**: [FINAL_COMPLETION_SUMMARY.md](./FINAL_COMPLETION_SUMMARY.md)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2025-10-06
**Status**: ✅ Complete Navigation Hub

