# Kubernaut

**AI-Powered Kubernetes Operations Platform**

Kubernaut is an open-source Kubernetes AIOps platform that combines AI-driven investigation with automated remediation. It analyzes Kubernetes incidents, orchestrates multi-step remediation workflows, and executes validated actions—targeting mean time to resolution reduction from 60 minutes to under 5 minutes while maintaining operational safety.

---

## 🎯 What is Kubernaut?

Kubernaut automates the entire incident response lifecycle for Kubernetes:

1. **Signal Ingestion**: Receives alerts from Prometheus AlertManager and Kubernetes Events
2. **AI Analysis**: Uses HolmesGPT for root cause analysis and remediation recommendations
3. **Workflow Orchestration**: Executes multi-step remediation playbooks via Tekton Pipelines
4. **Continuous Learning**: Tracks effectiveness and improves recommendations over time

### Key Capabilities

- **Multi-Source Signal Processing**: Prometheus alerts, Kubernetes events, with deduplication and storm detection
- **AI-Powered Root Cause Analysis**: HolmesGPT integration for intelligent investigation
- **Remediation Playbooks**: Industry-standard, versioned remediation patterns (PagerDuty/Google SRE-aligned)
- **Safety-First Execution**: Comprehensive validation, dry-run mode, and rollback capabilities
- **Continuous Learning**: Multi-dimensional effectiveness tracking (incident type, playbook, action)
- **Production-Ready**: 289 tests passing, 95% confidence across all services

---

## 🏗️ Architecture

Kubernaut follows a microservices architecture with 11 services (4 CRD controllers + 7 stateless services):

![Kubernaut Layered Architecture](docs/architecture/diagrams/kubernaut-layered-architecture.svg)

### Architecture Flow

1. **Gateway Service** receives signals (Prometheus alerts, K8s events) and creates `RemediationRequest` CRDs
2. **Remediation Orchestrator** coordinates the lifecycle across 4 specialized CRD controllers:
   - **Signal Processing**: Enriches signals with Kubernetes context
   - **AI Analysis**: Performs HolmesGPT investigation and generates recommendations
   - **Remediation Execution**: Orchestrates Tekton Pipelines for multi-step workflows
   - **Notification**: Delivers multi-channel notifications (Slack, Email, etc.)
3. **Data Storage Service** provides centralized PostgreSQL access (ADR-032)
4. **Effectiveness Monitor** tracks outcomes and feeds learning back to AI

### Communication Pattern

Kubernaut uses **Kubernetes Custom Resources (CRDs)** for all inter-service communication, enabling:
- Event-driven, resilient workflows
- Built-in retry and reconciliation
- Complete audit trail
- Horizontal scaling

---

## 📊 Implementation Status

**Current Phase**: Phase 2 Complete - 6 of 11 services production-ready (55%)

| Service | Status | Purpose | BR Coverage |
|---------|--------|---------|-------------|
| **Gateway Service** | ✅ **v1.0 PRODUCTION-READY** | Signal ingestion & deduplication | 62/62 P0/P1 (100%) |
| **Data Storage Service** | ✅ **Phase 1 PRODUCTION-READY** | REST API Gateway for PostgreSQL (ADR-032) | TBD |
| **Context API** | ✅ **v1.0 PRODUCTION-READY** | Historical intelligence REST API | 12/15 Active (80%) |
| **Dynamic Toolset Service** | ✅ **COMPLETE** | HolmesGPT toolset configuration | TBD |
| **Notification Service** | ✅ **COMPLETE** | Multi-channel delivery | TBD |
| **HolmesGPT API** | ✅ **v3.0 PRODUCTION-READY** | AI investigation wrapper | TBD |
| **Signal Processing** | ⏸️ Phase 3 | Signal enrichment | - |
| **AI Analysis** | ⏸️ Phase 4 | AI-powered analysis | - |
| **Remediation Execution** | ⏸️ Phase 3 | Tekton workflow orchestration | - |
| **Remediation Orchestrator** | ⏸️ Phase 5 | Cross-CRD coordination | - |
| **Effectiveness Monitor** | ⏸️ Phase 5 | Outcome assessment & learning | - |

**Timeline**: 13-week development plan (currently in Week 2-3)

**Recent Updates** (November 8, 2025):
- ✅ Gateway Service v1.0: 240/240 tests passing, 62 P0/P1 BRs, production-ready
- ✅ Context API v1.0: 100% P0 2x coverage, 12 active BRs (3 deprecated per ADR-032), 11 BRs migrated to AI/ML Service

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.23.9+** for building services
- **Kubernetes cluster** (Kind recommended for development)
- **Redis** (for Gateway service deduplication)
- **PostgreSQL** (for data persistence)
- **kubectl** with cluster access

### Build and Run

```bash
# Install CRDs
make install

# Build all CRD controllers (single binary for development)
make build
# Creates: bin/manager (includes all CRD controllers)

# Build individual services
go build -o bin/gateway-service ./cmd/gateway
go build -o bin/dynamic-toolset ./cmd/dynamictoolset
go build -o bin/data-storage ./cmd/datastorage
go build -o bin/context-api ./cmd/contextapi
```

### Testing

```bash
# Setup Kind cluster for testing
make test-gateway-setup

# Run tests by tier
make test                      # Unit tests (70%+ coverage)
make test-integration          # Integration tests (>50% coverage)
make test-e2e                  # End-to-end tests (<10% coverage)

# Clean up
make test-gateway-teardown
```

---

## 📚 Documentation

### For New Developers

Start with these essential documents:

1. **[Kubernaut CRD Architecture](docs/architecture/KUBERNAUT_CRD_ARCHITECTURE.md)** ⭐ **PRIMARY REFERENCE**
   - Complete architecture overview
   - System diagrams and service specifications
   - Code examples and operational guide

2. **[V1 Source of Truth Hierarchy](docs/V1_SOURCE_OF_TRUTH_HIERARCHY.md)**
   - Authoritative documentation for V1 implementation
   - 3-tier hierarchy: Architecture → Services → Design

3. **[Service Development Order Strategy](docs/planning/SERVICE_DEVELOPMENT_ORDER_STRATEGY.md)**
   - Implementation timeline & dependencies
   - Phase-by-phase development guide

### Architecture Documentation

- **[Approved Microservices Architecture](docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md)**: Service boundaries and V1/V2 roadmap
- **[Multi-CRD Reconciliation Architecture](docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)**: CRD communication patterns
- **[CRD Schemas](docs/architecture/CRD_SCHEMAS.md)**: Authoritative CRD field definitions
- **[Tekton Execution Architecture](docs/architecture/TEKTON_EXECUTION_ARCHITECTURE.md)**: Workflow orchestration with Tekton

### Service Documentation

- **[CRD Controllers](docs/services/crd-controllers/)**: RemediationOrchestrator, SignalProcessing, AIAnalysis, WorkflowExecution
- **[Stateless Services](docs/services/stateless/)**: Gateway, Dynamic Toolset, Data Storage, Context API, HolmesGPT API

### Development Resources

- **[Testing Strategy](.cursor/rules/03-testing-strategy.mdc)**: Defense-in-depth testing pyramid
- **[CRD Controller Templates](docs/templates/crd-controller-gap-remediation/)**: Production-ready scaffolding (saves 40-60% development time)
- **[Design Decisions](docs/architecture/DESIGN_DECISIONS.md)**: All architectural decisions with alternatives

---

## 🧪 Testing Strategy

Kubernaut follows a **defense-in-depth testing pyramid**:

- **Unit Tests**: **70%+ coverage** - Extensive business logic with external mocks only
- **Integration Tests**: **>50% coverage** - Cross-service coordination, CRD-based flows
- **E2E Tests**: **<10% coverage** - Critical end-to-end user journeys

**Current Test Status**: 373+ tests passing (100% pass rate)

| Service | Unit | Integration | E2E | Total | Confidence |
|---------|------|-------------|-----|-------|------------|
| **Gateway v1.0** | 31 (100%) | 124 (100%) | 3 (100%) | **158** | **100%** |
| **Context API v1.0** | 81% coverage | 53% coverage | 19% coverage | **147+** | **100%** |
| Data Storage | 167 tests | 18 tests | ✅ Complete | 185+ | 98% |
| Dynamic Toolset | 194 tests | 38 tests | ⏸️ V2 | 232+ | 95% |
| Notification Service | 19 tests | 21 tests | ⏸️ Deferred | 40+ | 95% |
| HolmesGPT API v3.0 | 74 tests | 30 tests | ⏸️ Requires LLM | 104+ | 98% |

---

## 🛡️ Security

### RBAC Configuration

Each CRD controller requires specific Kubernetes permissions. See [RBAC documentation](docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md#security-considerations) for details.

### Service-to-Service Authentication

- **Gateway Service**: Network-level security (NetworkPolicies + TLS)
- **CRD Controllers**: Kubernetes ServiceAccount authentication
- **Inter-service**: Service mesh (Istio/Linkerd) with mTLS

---

## 📊 Monitoring & Observability

- **Metrics**: All services expose Prometheus metrics on `:9090/metrics`
- **Health Checks**: `GET /health` and `GET /ready` endpoints on all services
- **Logging**: Structured JSON logging with configurable levels
- **Tracing**: OpenTelemetry support (planned for V1.1)

---

## 🤝 Contributing

### Development Standards

- **Go**: Standard conventions with comprehensive error handling
- **Testing**: Ginkgo/Gomega BDD tests, >70% unit coverage
- **Documentation**: Comprehensive inline documentation
- **CRD Changes**: Update [CRD_SCHEMAS.md](docs/architecture/CRD_SCHEMAS.md)

### Pull Request Process

1. Create feature branch from `main`
2. Implement with comprehensive tests
3. Follow [Service Development Order](docs/planning/SERVICE_DEVELOPMENT_ORDER_STRATEGY.md)
4. Update relevant documentation
5. Code review and merge

---

## 📄 License

Apache License 2.0

---

## 🔗 Support & Community

- **Issues**: [GitHub Issues](https://github.com/jordigilh/kubernaut/issues)
- **Discussions**: [GitHub Discussions](https://github.com/jordigilh/kubernaut/discussions)
- **Documentation**: Comprehensive guides in `docs/` directory

---

**Kubernaut** - Building the next evolution of Kubernetes operations through intelligent, CRD-based microservices that learn and adapt.

**Current Status**: Phase 2 Complete - 6 of 11 services production-ready (55%) | **Target**: Week 13 for V1 completion
