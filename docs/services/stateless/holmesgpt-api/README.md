# HolmesGPT API Service

**Version**: v3.10
**Status**: ✅ **PRODUCTION READY**
**Service Type**: Stateless HTTP API (Python/FastAPI)
**Port**: 8080 (REST API), 9090 (Metrics)
**Namespace**: `kubernaut-system`

---

## 📋 Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| v3.10 | 2025-12-07 | ConfigMap hot-reload implementation (V1.0): FileWatcher + ConfigManager + Metrics, `INVESTIGATION_INCONCLUSIVE` enum, 474 unit tests | [DD-HAPI-004](../../../architecture/decisions/DD-HAPI-004-configmap-hotreload.md), [BR-HAPI-199](../../../requirements/BR-HAPI-199-configmap-hot-reload.md), [BR-HAPI-200](../../../requirements/BR-HAPI-200-resolved-stale-signals.md) |
| v3.9 | 2025-12-06 | ConfigMap hot-reload spec (V1.0): LLM config, toolsets, log_level | [DD-HAPI-004](../../../architecture/decisions/DD-HAPI-004-configmap-hotreload.md), [BR-HAPI-199](../../../requirements/BR-HAPI-199-configmap-hot-reload.md) |
| v3.8 | 2025-12-06 | ADR-034 audit compliance fix, E2E audit pipeline tests passing, 437 unit tests, 557 total tests | [ADR-034](../../../architecture/decisions/ADR-034-unified-audit-table-design.md) |
| v3.7 | 2025-12-06 | Full LLM I/O audit, `validation_attempts_history` field, E2E audit tests (real DB only) | [BR-AUDIT-005](../../../requirements/BR-AUDIT-005.md) |
| v3.6 | 2025-12-06 | LLM self-correction loop (max 3 retries), `needs_human_review` + `human_review_reason`, 429 unit tests | [DD-HAPI-002 v1.2](../../../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md) |
| v3.5 | 2025-12-05 | Added `alternative_workflows[]` for audit/context (ADR-045 v1.2), 500 tests | [ADR-045](../../../architecture/decisions/ADR-045-aianalysis-holmesgpt-api-contract.md) |
| v3.4 | 2025-12-03 | Added Implementation Structure section (100% ADR-039 compliance) | [DOCUMENTATION_STANDARDIZATION_REQUEST](../../../../handoff/DOCUMENTATION_STANDARDIZATION_REQUEST_HOLMESGPT_API.md) |
| v3.3 | 2025-12-03 | Documentation restructured to SERVICE_DOCUMENTATION_GUIDE.md standard | [DOCUMENTATION_MIGRATION_PLAN](./DOCUMENTATION_MIGRATION_PLAN.md) |
| v3.2 | 2025-11-30 | All production blockers resolved (RFC7807, Graceful Shutdown) | [DD-004](../../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md), [DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) |
| v3.1 | 2025-11-01 | Production blockers identified | - |
| v3.0 | 2025-10-17 | Minimal Internal Service (45 BRs, 104 tests) | [DD-HOLMESGPT-012](../../../architecture/decisions/DD-HOLMESGPT-012-Minimal-Internal-Service-Architecture.md) |

---

## 🗂️ Documentation Index

| Document | Purpose | Status |
|----------|---------|--------|
| **[overview.md](./overview.md)** | Service architecture, HolmesGPT integration, design decisions | ✅ Complete |
| **[api-specification.md](./api-specification.md)** | OpenAPI spec, endpoints, request/response models | ✅ Complete |
| **[BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md)** | BR catalog (45 essential BRs) | ✅ Complete |
| **[BR_MAPPING.md](./BR_MAPPING.md)** | Test-BR traceability matrix | ✅ Complete |
| **[implementation-checklist.md](./implementation-checklist.md)** | APDC-TDD phases & validation | ✅ Complete |
| **[security-configuration.md](./security-configuration.md)** | RBAC, network policies, secrets | ✅ Complete |
| **[observability-logging.md](./observability-logging.md)** | Structured logging, tracing, correlation IDs | ✅ Complete |
| **[metrics-slos.md](./metrics-slos.md)** | Prometheus metrics, SLIs/SLOs, Grafana | ✅ Complete |
| **[testing-strategy.md](./testing-strategy.md)** | Unit/Integration/E2E tests, mock patterns | ✅ Complete |
| **[integration-points.md](./integration-points.md)** | Service dependencies, upstream/downstream | ✅ Complete |

### Implementation Documents

| Document | Purpose |
|----------|---------|
| **[implementation/IMPLEMENTATION_PLAN_V3.0.md](./implementation/IMPLEMENTATION_PLAN_V3.0.md)** | Minimal service implementation (45 BRs) |
| **[implementation/IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md](./implementation/IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md)** | Production blockers resolution |
| **[implementation/design/](./implementation/design/)** | Design decisions and rationale |
| **[implementation/archive/](./implementation/archive/)** | Previous implementation versions |

### Cross-Team Handoffs

| Document | Team | Status |
|----------|------|--------|
| **[HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md](./HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md)** | SignalProcessing | ✅ Validated |
| **[BR-HAPI-046-050-DATA-STORAGE-WORKFLOW-TOOL.md](./BR-HAPI-046-050-DATA-STORAGE-WORKFLOW-TOOL.md)** | Data Storage | ✅ Complete |

---

## 📁 File Organization

```
holmesgpt-api/
├── 📄 README.md                           - Service index & navigation (COMMON)
├── 📘 overview.md                         - High-level architecture (COMMON)
├── 🔧 api-specification.md               - API contract (SERVICE-SPECIFIC)
├── 📋 BUSINESS_REQUIREMENTS.md           - BR catalog (COMMON)
├── 📋 BR_MAPPING.md                       - Test-BR traceability (COMMON)
├── ✅ implementation-checklist.md         - APDC-TDD phases (COMMON)
├── 🔒 security-configuration.md          - Security patterns (COMMON)
├── 📊 observability-logging.md           - Logging & tracing (COMMON)
├── 📈 metrics-slos.md                    - Prometheus & SLOs (COMMON)
├── 🧪 testing-strategy.md                - Test patterns (COMMON)
├── 🔗 integration-points.md              - Service coordination (COMMON)
│
├── 📁 observability/                      - Observability assets
│   └── grafana-dashboard.json            - Grafana dashboard
│
├── 📁 implementation/                     - Implementation docs
│   ├── IMPLEMENTATION_PLAN_V3.0.md       - Active implementation
│   ├── IMPLEMENTATION_PLAN_V3.1_*.md     - Production readiness
│   ├── 📁 archive/                       - Previous versions
│   └── 📁 design/                        - Design documents
│
└── 📋 BR subdocuments
    ├── BR-HAPI-046-050-*.md              - Data Storage integration
    └── HANDOFF_*.md                      - Cross-team handoffs
```

---

## 🏗️ Implementation Structure

### **Service Location**
- **Directory**: `holmesgpt-api/`
- **Entry Point**: `holmesgpt-api/src/main.py`
- **Runtime**: Python 3.11+ / FastAPI
- **Run Command**: `uvicorn src.main:create_app --factory --host 0.0.0.0 --port 8080`

### **HTTP API Handlers**
- **Package**: `src/extensions/`
  - `incident.py` - `/api/v1/incident/analyze` endpoint
  - `recovery.py` - `/api/v1/recovery/analyze` endpoint
  - `postexec.py` - `/api/v1/postexec/analyze` endpoint
  - `health.py` - `/health`, `/ready` endpoints

### **Business Logic**
- **Package**: `src/`
  - `models/` - Pydantic request/response models (incident, recovery, postexec)
  - `toolsets/` - Workflow catalog client, MCP search integration
  - `clients/datastorage/` - Data Storage REST API client
  - `middleware/` - Metrics, RFC 7807 errors, authentication
  - `config/` - Configuration management
  - `audit/` - LLM call auditing

### **Tests**
- `tests/unit/` - 433 unit tests (business logic, models, prompts)
- `tests/integration/` - 71 integration tests (Data Storage, mock LLM)
- `tests/e2e/` - 40 E2E tests (workflow selection, container image)
- `tests/smoke/` - 4 smoke tests (real LLM validation, optional)
- `tests/load/` - Locust load testing

**See Also**: [OpenAPI Specification](./api/openapi.json) for complete API documentation.

---

## 🎯 Quick Start

### Service Overview

**Minimal internal service wrapper around HolmesGPT Python SDK.**

**Core Capabilities**:
- AI-powered investigation for Kubernetes issues
- Recovery strategy analysis
- Post-execution effectiveness analysis
- Multi-provider LLM support (OpenAI, Claude, Ollama)

### API Endpoints

| Endpoint | Method | Purpose | Latency Target |
|----------|--------|---------|----------------|
| `/api/v1/incident/analyze` | POST | AI investigation (HolmesGPT SDK) | < 5s |
| `/api/v1/recovery/analyze` | POST | Recovery strategy analysis | < 3s |
| `/api/v1/postexec/analyze` | POST | Post-execution effectiveness | < 2s |
| `/health` | GET | Liveness probe | < 50ms |
| `/ready` | GET | Readiness probe | < 50ms |
| `/metrics` | GET | Prometheus metrics | < 100ms |

### Test Summary

| Test Type | Count | Coverage |
|-----------|-------|----------|
| **Unit Tests** | 437 | Core business logic, ADR-034 audit compliance |
| **Integration Tests** | 71 | Service interactions |
| **E2E Tests** | 45 | End-to-end workflows (requires real Data Storage) |
| **Smoke Tests** | 4 | Real LLM validation (optional) |
| **Total** | **557** | **Full coverage** |

---

## ✅ V1.0 Features (Complete)

| Feature | Status | BR | DD |
|---------|--------|-----|-----|
| **ConfigMap Hot-Reload** | ✅ Complete | [BR-HAPI-199](../../../requirements/BR-HAPI-199-configmap-hot-reload.md) | [DD-HAPI-004](../../../architecture/decisions/DD-HAPI-004-configmap-hotreload.md) |
| **Investigation Inconclusive** | ✅ Complete | [BR-HAPI-200](../../../requirements/BR-HAPI-200-resolved-stale-signals.md) | - |

### Hot-Reload Scope
| Field | Hot-Reload | Business Use Case |
|-------|------------|-------------------|
| `llm.model` | ✅ | Cost/quality switching |
| `llm.provider` | ✅ | Provider failover |
| `llm.endpoint` | ✅ | Endpoint switching |
| `llm.max_retries` | ✅ | Retry tuning |
| `llm.timeout_seconds` | ✅ | Timeout adjustment |
| `llm.temperature` | ✅ | Response tuning |
| `toolsets.*` | ✅ | Feature toggles |
| `log_level` | ✅ | Debug enablement |

---

## 🔗 Related Documents

### Architecture Decisions
- **[DD-HOLMESGPT-012](../../../architecture/decisions/DD-HOLMESGPT-012-Minimal-Internal-Service-Architecture.md)** - Minimal internal service architecture
- **[DD-HAPI-004](../../../architecture/decisions/DD-HAPI-004-configmap-hotreload.md)** - ConfigMap hot-reload ✅
- **[DD-004](../../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md)** - RFC 7807 error responses ✅
- **[DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md)** - Kubernetes-aware graceful shutdown ✅
- **[ADR-045](../../../architecture/decisions/ADR-045-aianalysis-holmesgpt-api-contract.md)** - AIAnalysis ↔ HolmesGPT-API contract

### Parent Documentation
- **[../README.md](../README.md)** - All stateless services
- **[../../SERVICE_DOCUMENTATION_GUIDE.md](../../SERVICE_DOCUMENTATION_GUIDE.md)** - Documentation standard

### Related Services
- **[AIAnalysis](../../crd-controllers/02-aianalysis/)** - Investigation orchestration
- **[Data Storage](../data-storage/)** - Workflow catalog, audit storage
- **[SignalProcessing](../../crd-controllers/01-signalprocessing/)** - DetectedLabels extraction

---

**Document Status**: ✅ Complete
**Standard Compliance**: SERVICE_DOCUMENTATION_GUIDE.md v3.1 + ADR-039 (100%)
**Last Updated**: December 3, 2025
