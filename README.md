# Kubernaut

**AI-Powered Kubernetes Operations Platform**

[![Go Report Card](https://goreportcard.com/badge/github.com/jordigilh/kubernaut)](https://goreportcard.com/report/github.com/jordigilh/kubernaut)
[![Go Version](https://img.shields.io/badge/Go-1.25.3-blue.svg)](https://golang.org/dl/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.34-blue.svg)](https://kubernetes.io/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/jordigilh/kubernaut/actions/workflows/ci-pipeline.yml/badge.svg)](https://github.com/jordigilh/kubernaut/actions/workflows/ci-pipeline.yml)

Kubernaut is an open-source Kubernetes AIOps platform that combines AI-driven investigation with automated remediation. It analyzes Kubernetes incidents, orchestrates multi-step remediation workflows, and executes validated actions—targeting mean time to resolution reduction from 60 minutes to under 5 minutes while maintaining operational safety.

---

## 🎯 What is Kubernaut?

Kubernaut automates the entire incident response lifecycle for Kubernetes:

1. **Signal Ingestion**: Receives alerts from Prometheus AlertManager and Kubernetes Events
2. **AI Analysis**: Uses HolmesGPT for root cause analysis and remediation recommendations
3. **Workflow Orchestration**: Executes OCI-containerized remediation workflows via Tekton Pipelines
4. **Continuous Learning**: Tracks effectiveness of worfklow executions and successful remediations over time

### Key Capabilities

- **Multi-Source Signal Processing**: Prometheus alerts, Kubernetes events, with deduplication and storm detection
- **AI-Powered Root Cause Analysis**: HolmesGPT integration for intelligent investigation
- **Remediation Workflows**: OCI-containerized Tekton workflows with flexible single or multi-step execution
- **Safety-First Execution**: Comprehensive validation, dry-run mode, and rollback capabilities
- **Continuous Learning**: Multi-dimensional effectiveness tracking (incident type, workflow, action)
- **Production-Ready**: 3,562+ tests passing, SOC2-compliant audit traces, 8 of 8 V1.0 services ready
- **Enterprise Diagnostics**: Must-gather diagnostic collection following OpenShift industry standard (BR-PLATFORM-001)

---

## 🏗️ Architecture

Kubernaut follows a microservices architecture with 8 production-ready services (5 CRD controllers + 3 stateless services):

![Kubernaut Layered Architecture](docs/architecture/diagrams/kubernaut-layered-architecture.svg)

### Architecture Flow

1. **Gateway Service** receives signals (Prometheus alerts, K8s events) and creates `RemediationRequest` CRDs
2. **Remediation Orchestrator** (CRD controller) coordinates remediation lifecycle across 4 other CRD controllers:
   - **Signal Processing Service**: Enriches signals with Kubernetes context
   - **AI Analysis Service**: Performs HolmesGPT investigation and generates recommendations
   - **Workflow Execution**: Orchestrates Tekton Pipelines for multi-step workflows
   - **Notification Service**: Delivers multi-channel notifications (Slack, Email, etc.)
3. **Data Storage Service** provides centralized PostgreSQL access (ADR-032)
4. **Effectiveness Monitor** tracks workflow remediation outcomes (deferred to V1.1)

### Communication Pattern

Kubernaut uses **Kubernetes Custom Resources (CRDs)** for all inter-service communication, enabling:
- Event-driven, resilient workflows
- Built-in retry and reconciliation
- Complete audit trail
- Horizontal scaling

---

## 📊 Implementation Status

**V1.0 Release** (In Development): All Core Services + Must-Gather Production-Ready
**Production-Ready Services**: 9 of 9 services (100%) ✅
**SOC2 Compliance**: In active development (Week 1 of 3-week sprint - see [SOC2_AUDIT_IMPLEMENTATION_PLAN.md](docs/development/SOC2/SOC2_AUDIT_IMPLEMENTATION_PLAN.md))
**Timeline**: V1.0 Pre-release: January 2026

| Service | Status | Purpose | BR Coverage |
|---------|--------|---------|-------------|
| **Gateway Service** | ✅ **v1.0 PRODUCTION-READY** | Signal ingestion & deduplication | 20 BRs (395 tests: 240U+118I+37E2E) **100% P0** |
| **Data Storage Service** | ✅ **v1.0 PRODUCTION-READY** | REST API Gateway for PostgreSQL (ADR-032) | 34 BRs (727 tests: 551U+163I+13E2E) E2E Cov: 70.8% main, 78.2% middleware |
| **HolmesGPT API** | ✅ **v3.10 PRODUCTION-READY** | AI investigation wrapper | 48 BRs (601 tests: 474U+77I+45E2E+5smoke) |
| **Notification Service** | ✅ **v1.0 PRODUCTION-READY** | Multi-channel delivery | 17 BRs (358 tests: 225U+112I+21E2E) **100% pass** |
| **Signal Processing Service** | ✅ **v1.0 PRODUCTION-READY** | Signal enrichment with K8s context | 456 tests (336U+96I+24E2E) |
| **AI Analysis Service** | ✅ **v1.0 PRODUCTION-READY** | AI-powered analysis & recommendations | 309 tests (222U+53I+34E2E) |
| **Workflow Execution** | ✅ **v1.0 PRODUCTION-READY** | Tekton workflow orchestration | 12 BRs (314 tests: 229U+70I+15E2E) |
| **Remediation Orchestrator** | ✅ **v1.0 PRODUCTION-READY** | Cross-CRD lifecycle coordination | 490 tests (432U+39I+19E2E) SOC2-compliant audit traces |
| **Must-Gather Diagnostic Tool** | ✅ **v1.0 PRODUCTION-READY** | Enterprise diagnostic collection | BR-PLATFORM-001 (45 containerized tests, multi-arch, CI pipeline) |
| **~~Effectiveness Monitor~~** | ❌ **Deferred to V1.1** | Continuous improvement (DD-017) | 10 BRs (requires 8+ weeks of data) |

**V1.0 Completion** (In Development):
- 🚧 **SOC2 Compliance**: In active development - targeting 100% SOC2 Type II compliance (enterprise-ready)
  - **Scope**: RR reconstruction + operator attribution (16 days, 128-129 hours)
  - **Week 1**: RR reconstruction from audit traces (Days 1-6, 48-49 hours)
  - **Week 2-3**: Operator action auditing with webhooks (Days 7-16, 80 hours)
  - **Plan**: [SOC2_AUDIT_IMPLEMENTATION_PLAN.md](docs/development/SOC2/SOC2_AUDIT_IMPLEMENTATION_PLAN.md) (comprehensive work breakdown)
  - **Test Plan**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) (Week 1)
  - **Extension**: [TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md](docs/development/SOC2/TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md) (Week 2-3)
  - **Authority**: DD-AUDIT-004 (field mapping), ADR-034 (unified audit table), DD-WEBHOOK-001 (operator attribution)
- ✅ **All Services Production-Ready**: 8 of 8 core services (100%)
- ⏳ **Next Phase**: Segmented E2E tests + Full system validation (Week 2-3 of sprint)
- ⏳ **Pre-Release**: January 2026 (feedback solicitation)

**Recent Updates** (January 4, 2026):
- 📋 **SOC2 Implementation Plans**: Created comprehensive 3-week implementation plan for 100% SOC2 Type II compliance
  - **Week 1**: [SOC2_AUDIT_IMPLEMENTATION_PLAN.md](docs/development/SOC2/SOC2_AUDIT_IMPLEMENTATION_PLAN.md) - RR reconstruction (6 days, 48-49 hours)
  - **Week 1**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) - DD-TESTING-001 compliant tests
  - **Week 2-3**: [TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md](docs/development/SOC2/TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md) - Operator attribution (10 days, 80 hours)
  - **Total**: 16 days, 128-129 hours across 3 weeks (quality prioritized over speed)
  - Plans in authoritative location: `docs/development/SOC2/`
- ✅ **DD-AUDIT-003 v1.3**: Extended with SOC2-specific event types (`aianalysis.analysis.completed`, `workflow.selection.completed`)
- 🎉 **V1.0 Core Services Complete**: All 8 services production-ready (December 28, 2025)
- ✅ **Gateway v1.0**: 339 tests (221U+22I+96E2E), 20 BRs, 100% P0 compliance, K8s-native deduplication (DD-GATEWAY-012), HTTP anti-pattern refactoring complete (Jan 10, 2026)
- ✅ **Signal Processing v1.0**: SOC2-compliant audit traces (original_payload, signal_labels, signal_annotations)
- ✅ **AI Analysis v1.0**: SOC2-compliant audit traces (provider_data for RR reconstruction)
- ✅ **Workflow Execution v1.0**: 314 tests (229U+70I+15E2E), DD-TEST-002 infrastructure migration, Redis port 16388 (DD-TEST-001 v1.9)
- ✅ **Remediation Orchestrator v1.0**: 497 tests (439U+39I+19E2E), SOC2-compliant audit traces (timeout_config), cross-CRD coordination
- ✅ **Data Storage v1.0**: 727 tests (551U+163I+13E2E), unified audit table (ADR-034), PostgreSQL access layer (ADR-032)
- ✅ **HolmesGPT API v3.10**: 601 tests (474U+77I+45E2E+5smoke), ConfigMap hot-reload, LLM self-correction loop
- ✅ **Notification Service v1.0**: 358 tests (225U+112I+21E2E), 100% pass rate, Kind-based E2E, multi-channel delivery, OpenAPI audit client integration
- ✅ **Code Quality**: Go Report Card integration, gofmt compliance (297 files), comprehensive badge suite
- ✅ **Must-Gather v1.0**: 45 containerized bats tests, multi-arch support (amd64/arm64), GDPR/CCPA sanitization, GitHub Actions CI, OpenShift-compatible (BR-PLATFORM-001)
- 📊 **V1.0 Service Count**: 9 production-ready services (8 core + must-gather diagnostic tool)

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.24.6+** for building services
- **Kubernetes cluster** (Kind recommended for development, v1.24+)
- **PostgreSQL** (for Data Storage service)
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

Kubernaut services use **Kustomize** for Kubernetes deployment.

### **Available Services** (All V1.0 Production-Ready)

| Service | Status | Deployment Path |
|---|---|---|
| **Gateway Service** | ✅ V1.0 Production-Ready | `deploy/gateway/` |
| **Data Storage Service** | ✅ V1.0 Production-Ready | `deploy/data-storage/` |
| **HolmesGPT API** | ✅ V3.10 Production-Ready | `deploy/holmesgpt-api/` |
| **Notification Service** | ✅ V1.0 Production-Ready | `deploy/notification/` |
| **Signal Processing** | ✅ V1.0 Production-Ready | CRD Controller (see `cmd/signalprocessing/`) |
| **AI Analysis** | ✅ V1.0 Production-Ready | CRD Controller (see `cmd/aianalysis/`) |
| **Workflow Execution** | ✅ V1.0 Production-Ready | CRD Controller (see `cmd/workflowexecution/`) |
| **Remediation Orchestrator** | ✅ V1.0 Production-Ready | CRD Controller (see `cmd/remediationorchestrator/`) |

**Note**: Gateway uses Kubernetes-native state management (DD-GATEWAY-012). Redis was removed - deduplication and storm tracking now use RemediationRequest status fields.

### **Quick Deploy - Gateway Service**

```bash
# Deploy Gateway service to Kubernetes
kubectl apply -k deploy/gateway/base/

# Verify deployment
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=gateway
kubectl logs -n kubernaut-system -l app.kubernetes.io/component=gateway --tail=50

# Check readiness
kubectl exec -n kubernaut-system $(kubectl get pod -n kubernaut-system -l app.kubernetes.io/component=gateway -o jsonpath='{.items[0].metadata.name}') -- curl localhost:8080/ready
```

### **Quick Deploy - Data Storage Service**

```bash
# Deploy PostgreSQL + Data Storage service
kubectl apply -k deploy/data-storage/

# Verify deployment
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=data-storage
```

### **Kustomize Structure**

Each service follows this structure:

```
deploy/[service]/
├── kustomization.yaml           # Service deployment manifest
├── 00-namespace.yaml            # kubernaut-system namespace
├── 01-rbac.yaml                 # ServiceAccount, Role, RoleBinding
├── 02-configmap.yaml            # Service configuration
├── 03-deployment.yaml           # Deployment spec
├── 04-service.yaml              # Service (ClusterIP)
├── 05-servicemonitor.yaml       # Prometheus metrics (if applicable)
└── README.md                    # Service-specific deployment guide
```

**Note**: For V1.0, we provide standard Kubernetes manifests. Platform-specific overlays (OpenShift SCC, etc.) will be added in V1.1 based on user feedback.

### **Deployment Guides**

- **[Gateway Service](deploy/gateway/README.md)**: Signal ingestion with K8s-native state management (DD-GATEWAY-012)
- **[Data Storage Service](deploy/data-storage/README.md)**: PostgreSQL-backed unified audit table (ADR-034)
- **[HolmesGPT API](deploy/holmesgpt-api/README.md)**: AI-powered root cause analysis
- **[Notification Service](deploy/notification/README.md)**: Multi-channel notifications (Slack, Email)

---

## 🔍 **Diagnostic Collection - Must-Gather**

Kubernaut provides industry-standard diagnostic collection following the OpenShift must-gather pattern:

```bash
# OpenShift-style (oc adm must-gather)
oc adm must-gather --image=quay.io/jordigilh/must-gather:latest

# Kubernetes-style (kubectl debug)
kubectl debug node/<node-name> \
  --image=quay.io/jordigilh/must-gather:latest \
  --image-pull-policy=Always -- /usr/bin/gather

# Direct pod execution (fallback)
kubectl run kubernaut-must-gather \
  --image=quay.io/jordigilh/must-gather:latest \
  --rm --attach -- /usr/bin/gather
```

**Collects**:
- All Kubernaut CRDs (RemediationRequests, SignalProcessings, AIAnalyses, etc.)
- Service logs (Gateway, Data Storage, HolmesGPT API, CRD controllers)
- Configurations (ConfigMaps, Secrets sanitized)
- Tekton Pipelines (PipelineRuns, TaskRuns, logs)
- Database infrastructure (PostgreSQL, Redis)
- Metrics snapshots and audit event samples

**Output**: Compressed tarball (`kubernaut-must-gather-<cluster>-<timestamp>.tar.gz`)

**Status**: ✅ **v1.0 PRODUCTION-READY** - 45 containerized tests, multi-arch support, GDPR/CCPA sanitization, CI pipeline
**Documentation**: See [BR-PLATFORM-001](docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md) and [Must-Gather README](cmd/must-gather/README.md)

---

## 👨‍💻 **For Developers**

**New to Kubernaut development?** Start here:

### 📘 **[Developer Guide](docs/DEVELOPER_GUIDE.md)** ⭐ **START HERE**

Complete onboarding guide for contributors:
- **Adding a new service** → 12-day implementation plan with APDC-TDD methodology
- **Extending existing services** → Feature implementation patterns
- **Development environment setup** → Prerequisites, tools, IDE configuration
- **Testing strategy** → Defense-in-depth pyramid (Unit 70%+ / Integration >50% / E2E <10%)
- **Deployment** → Kustomize manifests for Kubernetes (v1.24+)

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

- **[CRD Controllers](docs/services/crd-controllers/)**: Remediation Orchestrator, Signal Processing Service, AI Analysis Service, Workflow Execution
- **[Stateless Services](docs/services/stateless/)**: Gateway Service, Data Storage Service, HolmesGPT API, Notification Service, Effectiveness Monitor

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

**Current Test Status** (as of Jan 9, 2026): ~3,589 tests passing across all tiers

| Service | Unit Specs | Integration Specs | E2E Specs | Total |
|---------|------------|-------------------|-----------|-------|
| **Gateway v1.0** | 221 | 22 | 96 | **339** |
| **Data Storage** | 434 | 153 | 84 | **671** |
| **Signal Processing** | 336 | 96 | 24 | **456** |
| **AI Analysis** | 222 | 53 | 34 | **309** |
| **Notification Service** | 225 | 112 | 21 | **358** |
| **HolmesGPT API v3.10** | 474 | 77 | 45 | **601** |
| **Workflow Execution v1.0** | 229 | 70 | 15 | **314** |
| **Remediation Orchestrator v1.0** | 432 | 39 | 19 | **490** |

**Total**: ~2,573 unit specs + ~622 integration specs + ~338 E2E specs = **~3,533 test specs**

*Note: Gateway v1.0 has 339 tests (221U+22I+96E2E) with 100% pass rate verified January 14, 2026 (HTTP anti-pattern refactoring complete Jan 10, 2026 - moved 74 integration tests to E2E tier, deleted 2 obsolete E2E tests per DD-GATEWAY-011/015). DataStorage v1.0 has 671 tests (434U+153I+84E2E) with E2E coverage 70.8% main/78.2% middleware. SignalProcessing v1.0 has 456 tests (336U+96I+24E2E). AI Analysis v1.0 has 309 tests (222U+53I+34E2E). Notification Service v1.0 has 358 tests (225U+112I+21E2E) with 100% pass rate verified December 28, 2025 (Kind-based E2E, OpenAPI audit client integration, ActorId filtering per DD-E2E-002). Workflow Execution v1.0 has 314 tests (229U+70I+15E2E) with 100% pass rate (311/311 passing, 3 pending for V1.1), DD-TEST-002 infrastructure migration complete, and parallel testing enabled (port 16388 per DD-TEST-001 v1.9). Remediation Orchestrator v1.0 has 490 tests (432U+39I+19E2E) with SOC2-compliant audit traces, cross-CRD coordination, and comprehensive timeout/blocking management (100% NULL-TESTING compliance per TESTING_GUIDELINES.md, verified December 28, 2025). See WE_FINAL_VALIDATION_DEC_25_2025.md for details.*

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

**V1.0 Complete** (Post-Merge): All 9 services production-ready (100%) ✅ | 8 core services + Must-Gather diagnostic tool | SOC2-compliant audit traces | 1 deferred to V1.1 (DD-017) | **V1.0 Pre-release**: January 2026

**Current Sprint (Week 1-3)**: ✅ Must-Gather v1.0 Complete (BR-PLATFORM-001, 45 tests, CI pipeline) → Segmented E2E tests with Remediation Orchestrator (Week 2) → Full system E2E with OOMKill scenario + Claude 4.5 Haiku (Week 3) → Pre-release + feedback solicitation.

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
