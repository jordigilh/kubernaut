# Kubernaut

**AIOps Platform for Intelligent Kubernetes Remediation**

[![Go Report Card](https://goreportcard.com/badge/github.com/jordigilh/kubernaut)](https://goreportcard.com/report/github.com/jordigilh/kubernaut)
[![Go Version](https://img.shields.io/badge/Go-1.25.3-blue.svg)](https://golang.org/dl/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.30+-blue.svg)](https://kubernetes.io/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/jordigilh/kubernaut/actions/workflows/ci-pipeline.yml/badge.svg)](https://github.com/jordigilh/kubernaut/actions/workflows/ci-pipeline.yml)

Kubernaut is an open-source **AIOps platform** that closes the loop from Kubernetes alert to automated remediation — without a human in the middle. When something goes wrong in your cluster (an OOMKill, a CrashLoopBackOff, node pressure), Kubernaut detects the signal, enriches it with context, sends it to an LLM for live root cause investigation using real `kubectl` access, matches a remediation workflow from a searchable catalog, and executes the fix — or escalates to a human with a full RCA when it can't.

The result: **mean time to resolution drops from 60 minutes to under 5**, while humans stay in control through approval gates, configurable confidence thresholds, and SOC2-compliant audit trails.

---

## 🎯 What is Kubernaut?

Kubernaut automates the entire incident response lifecycle for Kubernetes through a five-stage AIOps pipeline:

1. **Signal Detection** — Receives alerts from Prometheus AlertManager (including predictive `predict_linear()` alerts) and Kubernetes Events, validates resource scope, and creates a `RemediationRequest`.
2. **Signal Processing** — Enriches the signal with Kubernetes context: owner chain, namespace labels, severity classification, deduplication, and signal mode (reactive vs. predictive).
3. **AI Analysis** — An LLM investigates the incident live — checking pod logs, events, and resource limits via `kubectl` — produces a root cause analysis, and searches a workflow catalog for a matching remediation.
4. **Workflow Execution** — Runs the selected remediation (e.g., a Kubernetes Job that patches a Deployment's memory limits) via Tekton Pipelines or Kubernetes Jobs, with optional human approval gates.
5. **Notification** — Keeps the team informed at every stage — whether the fix was applied automatically, is pending approval, or has been flagged for human review.

For SRE teams, the value proposition is: **reduce MTTR on known failure patterns to near-zero, while building a searchable catalog of remediation workflows that encode your team's operational knowledge.** Kubernaut handles the toil; your team focuses on the novel problems.

### Key Capabilities

- **Multi-Source Signal Processing**: Prometheus alerts (reactive and predictive), Kubernetes events with deduplication, signal mode classification, and signal type normalization (ADR-054)
- **AI-Powered Root Cause Analysis**: HolmesGPT integration with LLM providers (Vertex AI, OpenAI, and others via LiteLLM) for intelligent investigation with live `kubectl` access
- **Remediation Workflow Catalog**: Searchable catalog of OCI-containerized workflows with label-based matching (signal type, severity, component, environment), wildcard support, and confidence scoring
- **Flexible Execution**: Tekton Pipelines (multi-step) or Kubernetes Jobs (single-step) with parameterized actions following the Validate-Action-Verify pattern
- **Resource Scope Management**: Label-based opt-in model (`kubernaut.ai/managed=true`) controls which namespaces and resources Kubernaut manages, with metadata-only informer caching (ADR-053)
- **Safety-First Design**: Admission webhook validation, human-in-the-loop approval gates, configurable confidence thresholds, and effectiveness tracking
- **SOC2 Type II Compliance**: Full RemediationRequest reconstruction from audit traces, operator attribution via webhooks, hash chain integrity verification (DD-AUDIT-004, DD-WEBHOOK-001, ADR-034)
- **Continuous Learning**: Multi-dimensional effectiveness tracking (incident type, workflow, action) to improve remediation success rates over time
- **Enterprise Diagnostics**: Must-gather diagnostic collection following OpenShift industry standard (BR-PLATFORM-001)
- **Production-Ready**: 10 of 10 V1.0 services complete with comprehensive CI coverage reporting

---

## 🏗️ Architecture

Kubernaut follows a microservices architecture with 10 production-ready services (5 CRD controllers + 4 stateless services + 1 diagnostic tool):

![Kubernaut Layered Architecture](docs/architecture/diagrams/kubernaut-layered-architecture.svg)

### Architecture Flow

1. **Gateway Service** receives signals (Prometheus alerts, K8s events), validates resource scope via metadata-only informer cache (`kubernaut.ai/managed` label, ADR-053), and creates `RemediationRequest` CRDs
2. **Remediation Orchestrator** (CRD controller) validates resource scope as Check #1 in the routing pipeline, then coordinates remediation lifecycle across 4 other CRD controllers:
   - **Signal Processing Service**: Enriches signals with Kubernetes context, classifies signal mode (reactive/predictive), and normalizes signal types (ADR-054)
   - **AI Analysis Service**: Performs HolmesGPT investigation and generates recommendations
   - **Workflow Execution**: Orchestrates Tekton Pipelines or Kubernetes Jobs for remediation workflows
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

**V1.0 Release** | **Timeline**: Pre-release February 2026
**Production-Ready Services**: 10 of 10 (100%) ✅ | **Full Pipeline E2E**: ✅ | **SOC2 Type II Compliance**: ✅

| Service | Status | Purpose | All Tiers Coverage |
|---------|--------|---------|-------------------|
| **HolmesGPT API** | ✅ v3.10 | AI investigation wrapper | 92.7% |
| **AI Analysis** | ✅ v1.0 | AI-powered analysis & recommendations | 87.4% |
| **Signal Processing** | ✅ v1.0 | Signal enrichment, mode classification, type normalization | 85.6% |
| **Workflow Execution** | ✅ v1.0 | Tekton Pipeline & K8s Job orchestration | 82.8% |
| **Gateway** | ✅ v1.0 | Signal ingestion & deduplication | 81.5% |
| **Remediation Orchestrator** | ✅ v1.0 | Cross-CRD lifecycle coordination | 80.9% |
| **Auth Webhook** | ✅ v1.0 | SOC2 operator attribution (DD-WEBHOOK-001) | 78.9% |
| **Notification** | ✅ v1.0 | Multi-channel delivery | 73.3% |
| **Data Storage** | ✅ v1.0 | REST API for PostgreSQL (ADR-032) | 60.7% |
| **Must-Gather** | ✅ v1.0 | Enterprise diagnostic collection (BR-PLATFORM-001) | N/A (bats) |
| ~~Effectiveness Monitor~~ | ❌ V1.1 | Continuous improvement (DD-017) | — |

> **Coverage** is the merged "All Tiers" metric: line-by-line deduplication across unit, integration, and E2E tiers. See the [Coverage Analysis Report](docs/testing/COVERAGE_ANALYSIS_REPORT.md) for per-tier breakdown. Coverage is reported automatically on every PR via CI.

**V1.0 Completion**:
- ✅ **SOC2 Type II Compliance** (January 2026): RR reconstruction (DD-AUDIT-004), operator attribution (DD-WEBHOOK-001), hash chain integrity (ADR-034)
- ✅ **CI/CD Coverage Pipeline** (February 2026): Per-tier analysis with line-by-line merging, automated PR comments
- ✅ **SAR Authentication**: Middleware-based SubjectAccessReview for all stateless services (DD-AUTH-014)
- ✅ **Resource Scope Management** (February 2026): `kubernaut.ai/managed` label-based opt-in for both Gateway and Remediation Orchestrator (BR-SCOPE-001, BR-SCOPE-010, ADR-053). Namespace fallback deprecated (DD-GATEWAY-007). ScopeChecker interface with mandatory DI, metadata-only informer caching, exponential backoff for unmanaged resources.
- ✅ **Predictive Signal Mode** (February 2026): Signal mode classification and type normalization in SP, predictive prompt strategy in HAPI enabling preemptive remediation for `predict_linear()` alerts (BR-SP-106, BR-AI-084, ADR-054)
- ✅ **Full Pipeline E2E Validation** (February 2026): End-to-end scenario with real LLM (Vertex AI) covering signal detection → AI investigation → workflow matching → execution → notification ([#39](https://github.com/jordigilh/kubernaut/issues/39))

**Next** (V1.0+):
- 🚧 **PagerDuty Delivery Channel** ([#60](https://github.com/jordigilh/kubernaut/issues/60)): PagerDuty Events API v2 integration for the Notification service
- 🚧 **Async AA-HAPI Session Polling** ([#64](https://github.com/jordigilh/kubernaut/issues/64)): Replace synchronous HAPI calls with session-based submit/poll design to eliminate timeout fragility

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.25+** for building services
- **Kubernetes cluster** (Kind recommended for development, v1.30+)
  - **Note**: v1.30+ required for CRD selectableFields support (field selectors on spec fields)
- **PostgreSQL** (for Data Storage service)
- **kubectl** with cluster access

### Build

```bash
# Build all services
make build-all

# Build a specific service (gateway, datastorage, notification, etc.)
make build-gateway
make build-datastorage
```

### Testing

```bash
# Run unit tests for a specific service
make test-unit-gateway
make test-unit-signalprocessing

# Run all unit tests across all services
make test-tier-unit

# Run integration/E2E tests for a specific service
make test-integration-gateway
make test-e2e-gateway

# Run all test tiers for a single service
make test-all-gateway

# Generate coverage report (markdown, table, or JSON)
make coverage-report-markdown
make coverage-report
make coverage-report-json
```

## 🚢 **Deployment**

Kubernaut services use **Kustomize** for Kubernetes deployment.

### **Stateless Services** (Deployment Manifests Available)

| Service | Deployment Path | Notes |
|---|---|---|
| **Gateway** | `deploy/gateway/` | Kustomize with base/overlays |
| **Data Storage** | `deploy/data-storage/` | Includes PostgreSQL infrastructure |
| **HolmesGPT API** | `deploy/holmesgpt-api/` | Full manifest set |
| **Notification** | `deploy/notification/` | Full manifest set |
| **Auth Webhook** | `deploy/authwebhook/` | Includes webhook configurations |

### **CRD Controllers** (Manifests Pending)

CRD controller deployment manifests (Signal Processing, AI Analysis, Workflow Execution, Remediation Orchestrator) will be finalized after the **Segmented E2E Scenarios** task (#39). Resource Scope Management (`kubernaut.ai/managed` label) is now complete — controllers require the managed label on target namespaces/resources (ADR-053).

### **Deployment Guides**

- **[Gateway Service](deploy/gateway/README.md)**: Signal ingestion with K8s-native state management (DD-GATEWAY-012)
- **[Data Storage Service](deploy/data-storage/README.md)**: PostgreSQL-backed unified audit table (ADR-034)
- **[HolmesGPT API](deploy/holmesgpt-api/README.md)**: AI-powered root cause analysis
- **[Auth Webhook](deploy/authwebhook/README.md)**: SOC2 operator attribution (DD-WEBHOOK-001)

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
- Tekton Pipelines (PipelineRuns, TaskRuns, logs) and Kubernetes Jobs
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
- **Deployment** → Kustomize manifests for Kubernetes (v1.30+)

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

### **📖 Documentation Structure Guide** ⭐ **NEW**

**[docs/DOCS_DIRECTORY_STRUCTURE_GUIDE.md](docs/DOCS_DIRECTORY_STRUCTURE_GUIDE.md)** - Complete guide to Kubernaut's documentation organization

**Quick Reference**:
- **Design Decisions (DD-*)**: `docs/architecture/decisions/` - Permanent architectural choices
- **Session Handoffs**: `docs/handoff/` - AI session summaries and implementation status (~2,776 documents)
- **Development Guides**: `docs/development/` - Methodology, testing, standards
- **Test Documentation**: `docs/testing/` - Test plans and strategies
- **Planning**: `docs/plans/` - Implementation plans and roadmaps

**Examples**:
```
Session summary → docs/handoff/DS_E2E_COMPLETE_JAN_26_2026.md
Design decision → docs/architecture/decisions/DD-AUTH-013-http-status-codes.md
Test plan → docs/development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md
```

---

### Architecture Documentation

- **[Approved Microservices Architecture](docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md)**: Service boundaries and V1/V2 roadmap
- **[Multi-CRD Reconciliation Architecture](docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)**: CRD communication patterns
- **[CRD Schemas](docs/architecture/CRD_SCHEMAS.md)**: Authoritative CRD field definitions
- **[Tekton Execution Architecture](docs/architecture/TEKTON_EXECUTION_ARCHITECTURE.md)**: Workflow orchestration with Tekton Pipelines and Kubernetes Jobs
- **[Design Decisions](docs/architecture/decisions/)**: All DD-* and ADR-* architectural decisions

### Service Documentation

- **[CRD Controllers](docs/services/crd-controllers/)**: Remediation Orchestrator, Signal Processing Service, AI Analysis Service, Workflow Execution
- **[Stateless Services](docs/services/stateless/)**: Gateway Service, Data Storage Service, HolmesGPT API, Notification Service, Effectiveness Monitor

### Development Resources

- **[Testing Strategy](.cursor/rules/03-testing-strategy.mdc)**: Defense-in-depth testing pyramid
- **[CRD Controller Templates](docs/templates/crd-controller-gap-remediation/)**: Production-ready scaffolding (saves 40-60% development time)
- **[Development Methodology](docs/development/methodology/)**: APDC framework, TDD workflow

---

## 🧪 Testing Strategy

Kubernaut follows a **defense-in-depth testing pyramid** with coverage tracked automatically via CI:

- **Unit-Testable**: ≥70% target — Pure business logic with external mocks only
- **Integration-Testable**: ≥60% target — Cross-service coordination, CRD-based flows, DB adapters
- **E2E**: Full end-to-end user journeys (Kind cluster)
- **All Tiers**: ≥80% target — Line-by-line deduplication across all tiers

| Service | Unit-Testable | Integration | E2E | All Tiers |
|---------|---------------|-------------|-----|-----------|
| **HolmesGPT API** | 79.1% | 56.5% | 59.2% | 92.7% |
| **AI Analysis** | 80.0% | 73.6% | 53.4% | 87.4% |
| **Signal Processing** | 87.3% | 61.8% | 58.1% | 85.6% |
| **Workflow Execution** | 73.4% | 67.9% | 56.3% | 82.8% |
| **Gateway** | 67.7% | 42.5% | 58.8% | 81.5% |
| **Remediation Orchestrator** | 79.2% | 55.9% | 47.2% | 80.9% |
| **Auth Webhook** | 50.0% | 49.0% | 40.5% | 78.9% |
| **Notification** | 75.5% | 57.7% | 50.8% | 73.3% |
| **Data Storage** | 55.0% | 34.2% | 45.2% | 60.7% |

> Coverage is reported on every PR via GitHub Actions. "All Tiers" uses line-by-line merging — a statement covered by any tier counts once. See [Coverage Analysis Report](docs/testing/COVERAGE_ANALYSIS_REPORT.md) for methodology and patterns.

---

## 🛡️ Security

### RBAC Configuration

Each CRD controller requires specific Kubernetes permissions. See [RBAC documentation](docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md#security-considerations) for details.

### Service-to-Service Authentication

- **Gateway Service**: Network-level security (NetworkPolicies + TLS)
- **CRD Controllers**: Kubernetes ServiceAccount authentication
- **Inter-service**: SubjectAccessReview middleware authentication (DD-AUTH-014, SAR-based)

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

**Kubernaut** - AIOps for Kubernetes: from alert to remediation, intelligently. Building the next evolution of Kubernetes operations through AI-driven, CRD-based microservices that learn and adapt.

**V1.0 Status**: 10 services production-ready ✅ | SOC2 compliance ✅ | CI coverage pipeline ✅ | Scope management ✅ | Full pipeline E2E ✅ | 1 deferred to V1.1 (DD-017) | Pre-release: February 2026

**Next**: PagerDuty delivery channel ([#60](https://github.com/jordigilh/kubernaut/issues/60)) | Async AA-HAPI polling ([#64](https://github.com/jordigilh/kubernaut/issues/64))

