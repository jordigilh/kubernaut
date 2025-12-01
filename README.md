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

Kubernaut follows a microservices architecture with 10 services (4 CRD controllers + 6 stateless services):

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

**Current Phase**: Phase 2 In Progress - 4 of 10 services production-ready (40%)

| Service | Status | Purpose | BR Coverage |
|---------|--------|---------|-------------|
| **Gateway Service** | ✅ **v1.0 PRODUCTION-READY** | Signal ingestion & deduplication | 20 BRs (100%) |
| **Data Storage Service** | ✅ **Phase 1 PRODUCTION-READY** | REST API Gateway for PostgreSQL (ADR-032) | 34 BRs (100%) |
| **Dynamic Toolset Service** | ⏸️ **Deferred to V2.0** | Service discovery & toolset generation | 8 BRs (DD-016: V1.x uses static config) |
| **Notification Service** | ✅ **COMPLETE** | Multi-channel delivery | 12 BRs (100%) |
| **HolmesGPT API** | ✅ **v3.2 PRODUCTION-READY** | AI investigation wrapper | 47 BRs (RFC 7807 + Graceful Shutdown + Recovery Prompt, 100%) |
| **Signal Processing** | ⏸️ Phase 3 | Signal enrichment | - |
| **AI Analysis** | ⏸️ Phase 4 | AI-powered analysis | - |
| **Remediation Execution** | ⏸️ Phase 3 | Tekton workflow orchestration | - |
| **Remediation Orchestrator** | ⏸️ Phase 5 | Cross-CRD coordination | - |
| **Effectiveness Monitor** | ⏸️ Phase 5 | Outcome assessment & learning | - |

**Timeline**: 13-week development plan (currently in Week 2-3)

**Recent Updates** (November 30, 2025):
- ✅ **HolmesGPT API v3.2**: Recovery prompt implementation complete (DD-RECOVERY-002/003), DetectedLabels integration, mock LLM server for testing
- ✅ **DEV_MODE Anti-Pattern Removed**: Tests now use mock LLM server with same code path as production
- ⏸️ **Dynamic Toolset Deferred to V2.0**: Per DD-016, deferred to V2.0 (V1.x uses static config, redundant with HolmesGPT-API's built-in Prometheus discovery)
- ✅ **E2E Test Optimization**: Parallel execution enabled, 2m37s runtime (~40% improvement)
- ✅ **Production Deployment Ready**: In-cluster deployment manifests with RBAC, NetworkPolicy, ServiceMonitor
- ✅ **RFC 7807 & Graceful Shutdown**: Implemented for Dynamic Toolset & HolmesGPT API (186 tests, 100% pass rate)
- ✅ **BR Documentation Complete**: 121 BRs documented across 5 services (100% coverage)
- ✅ Gateway Service v1.0: 240/240 tests passing, 20 BRs, production-ready
- ✅ Data Storage Service Phase 1: Unified audit table, PostgreSQL access layer (ADR-032)

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

## 🚢 **Deployment**

Kubernaut services use **Kustomize overlays** for cross-platform deployment (OpenShift + vanilla Kubernetes).

### **Available Services**

| Service | Status | Deployment Path |
|---|---|---|
| **Gateway + Redis** | ✅ Production-Ready | `deploy/gateway/` |
| **HolmesGPT API** | ⏸️ Coming Soon | `deploy/holmesgpt-api/` |
| **PostgreSQL** | ⏸️ Coming Soon | `deploy/postgres/` |

### **Quick Deploy - Gateway Service**

#### **OpenShift**

```bash
# Deploy Gateway + Redis to OpenShift
oc apply -k deploy/gateway/overlays/openshift/

# Verify
oc get pods -n kubernaut-system -l app.kubernetes.io/component=gateway
```

#### **Vanilla Kubernetes**

```bash
# Deploy Gateway + Redis to Kubernetes
kubectl apply -k deploy/gateway/overlays/kubernetes/

# Verify
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=gateway
```

### **Kustomize Structure**

Each service follows this structure:

```
deploy/[service]/
├── base/                          # Platform-agnostic manifests
│   ├── kustomization.yaml
│   └── *.yaml                     # K8s resources
├── overlays/
│   ├── openshift/                 # OpenShift-specific (SCC fixes)
│   │   ├── kustomization.yaml
│   │   └── patches/
│   └── kubernetes/                # Vanilla K8s (uses base)
│       └── kustomization.yaml
└── README.md                      # Service-specific deployment guide
```

**Key Differences**:
- **OpenShift**: Removes hardcoded `runAsUser`/`fsGroup` for SCC compatibility
- **Kubernetes**: Uses base manifests with explicit security contexts

### **Deployment Guides**

- **[Gateway Service](deploy/gateway/README.md)**: Signal ingestion + deduplication + storm detection
- **HolmesGPT API**: Coming soon
- **PostgreSQL**: Coming soon

---

---

## 👨‍💻 **For Developers**

**New to Kubernaut development?** Start here:

### 📘 **[Developer Guide](docs/DEVELOPER_GUIDE.md)** ⭐ **START HERE**

Complete onboarding guide for contributors:
- **Adding a new service** → 12-day implementation plan with APDC-TDD methodology
- **Extending existing services** → Feature implementation patterns
- **Development environment setup** → Prerequisites, tools, IDE configuration
- **Testing strategy** → Defense-in-depth pyramid (Unit 70%+ / Integration >50% / E2E <10%)
- **Deployment** → Kustomize overlays for OpenShift + Kubernetes

### **Quick Links for Developers**

| I want to... | Go to... |
|--------------|----------|
| **Implement a new service** | [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](docs/services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) (11-12 days) |
| **Extend an existing service** | [FEATURE_EXTENSION_PLAN_TEMPLATE.md](docs/services/FEATURE_EXTENSION_PLAN_TEMPLATE.md) (3-12 days) |
| **Document a service** | [SERVICE_DOCUMENTATION_GUIDE.md](docs/services/SERVICE_DOCUMENTATION_GUIDE.md) |
| **Understand architecture** | [Kubernaut CRD Architecture](docs/architecture/KUBERNAUT_CRD_ARCHITECTURE.md) |
| **Learn testing strategy** | [03-testing-strategy.mdc](.cursor/rules/03-testing-strategy.mdc) |
| **Follow Go standards** | [02-go-coding-standards.mdc](.cursor/rules/02-go-coding-standards.mdc) |

---

## 📚 Documentation

### Architecture Documentation

- **[Approved Microservices Architecture](docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md)**: Service boundaries and V1/V2 roadmap
- **[Multi-CRD Reconciliation Architecture](docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)**: CRD communication patterns
- **[CRD Schemas](docs/architecture/CRD_SCHEMAS.md)**: Authoritative CRD field definitions
- **[Tekton Execution Architecture](docs/architecture/TEKTON_EXECUTION_ARCHITECTURE.md)**: Workflow orchestration with Tekton

### Service Documentation

- **[CRD Controllers](docs/services/crd-controllers/)**: RemediationOrchestrator, SignalProcessing, AIAnalysis, WorkflowExecution
- **[Stateless Services](docs/services/stateless/)**: Gateway, Dynamic Toolset, Data Storage, HolmesGPT API, Notification, Effectiveness Monitor

### Development Resources

- **[Testing Strategy](.cursor/rules/03-testing-strategy.mdc)**: Defense-in-depth testing pyramid
- **[CRD Controller Templates](docs/templates/crd-controller-gap-remediation/)**: Production-ready scaffolding (saves 40-60% development time)
- **[Design Decisions](docs/architecture/DESIGN_DECISIONS.md)**: All architectural decisions with alternatives

---

## 🧪 Testing Strategy

Kubernaut follows a **defense-in-depth testing pyramid**:

- **Unit Tests**: **70%+ coverage** - Extensive business logic with external mocks only
- **Integration Tests**: **>50% coverage** - Cross-service coordination, CRD-based flows, microservices architecture
- **E2E Tests**: **<10% coverage** - Critical end-to-end user journeys

**Current Test Status**: ~1,177 tests passing (100% pass rate across all tiers)

| Service | Unit Specs | Integration Specs | E2E Specs | Total | Confidence |
|---------|------------|-------------------|-----------|-------|------------|
| **Gateway v1.0** | 105 | 114 | 2 (+12 deferred to v1.1) | **221** | **100%** |
| **Data Storage** | 475 | ~60 | - | **~535** | **98%** |
| **Dynamic Toolset** | - | - | - | **Deferred to V2.0** | **DD-016** |
| **Notification Service** | 140 | 97 | 12 | **249** | **100%** |
| **HolmesGPT API v3.2** | 151 | 21 | - | **172** | **98%** |

**Total**: ~871 unit specs + ~292 integration specs + 14 E2E specs = **~1,177 test specs**

*Note: Gateway v1.0 has 2 E2E specs (Storm TTL, K8s API Rate Limiting), 12 additional E2E tests deferred to v1.1. Notification Service has 12 E2E specs (Kind-based file delivery + metrics validation). Dynamic Toolset (245 tests) deferred to V2.0 per DD-016. Integration spec counts are estimates.*

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

**Current Status**: Phase 2 In Progress - 4 of 10 services production-ready (40%) | 1 deferred to V2.0 (DD-016) | **Target**: Week 13 for V1 completion

---

## 📋 Changelog

**Version**: 1.1
**Date**: 2025-11-15
**Status**: Updated

### Version 1.1 (2025-11-15)
- **Service Naming Correction**: Replaced all instances of "Workflow Engine" with "Remediation Execution Engine" per ADR-035
- **Terminology Alignment**: Updated to match authoritative naming convention (RemediationExecution CRD, Remediation Execution Engine architectural concept)
- **Documentation Consistency**: Aligned with NAMING_CONVENTION_REMEDIATION_EXECUTION.md reference document

### Version 1.0 (Original)
- Initial document creation
