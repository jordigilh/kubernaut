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

## What is Kubernaut?

Kubernaut automates the entire incident response lifecycle for Kubernetes through a five-stage AIOps pipeline:

1. **Signal Detection** — Receives alerts from Prometheus AlertManager (including predictive `predict_linear()` alerts) and Kubernetes Events, validates resource scope, and creates a `RemediationRequest`.
2. **Signal Processing** — Enriches the signal with Kubernetes context: owner chain, namespace labels, severity classification, deduplication, and signal mode (reactive vs. predictive).
3. **AI Analysis** — An LLM investigates the incident live — checking pod logs, events, and resource limits via `kubectl` — produces a root cause analysis, and searches a workflow catalog for a matching remediation.
4. **Workflow Execution** — Runs the selected remediation (e.g., a Kubernetes Job that patches a Deployment's memory limits) via Tekton Pipelines or Kubernetes Jobs, with optional human approval gates.
5. **Close the Loop** — Two parallel actions after execution completes:
   - **Notification**: Keeps the team informed — whether the fix was applied automatically, is pending approval, or has been flagged for human review.
   - **Effectiveness Monitoring**: Evaluates whether the fix actually worked via spec hash comparison, health checks, metric evaluation, and effectiveness scoring.

For SRE teams, the value proposition is: **reduce MTTR on known failure patterns to near-zero, while building a searchable catalog of remediation workflows that encode your team's operational knowledge.** Kubernaut handles the toil; your team focuses on the novel problems.

### Key Capabilities

- **Multi-Source Signal Processing**: Prometheus alerts (reactive and predictive), Kubernetes events with deduplication, signal mode classification, and signal type normalization
- **AI-Powered Root Cause Analysis**: HolmesGPT integration with LLM providers (Vertex AI, OpenAI, and others via LiteLLM) for intelligent investigation with live `kubectl` access
- **Remediation Workflow Catalog**: Searchable catalog of OCI-containerized workflows with label-based matching (signal type, severity, component, environment), wildcard support, and confidence scoring
- **Flexible Execution**: Tekton Pipelines (multi-step) or Kubernetes Jobs (single-step) with parameterized actions following the Validate-Action-Verify pattern
- **Resource Scope Management**: Label-based opt-in model (`kubernaut.ai/managed=true`) controls which namespaces and resources Kubernaut manages
- **Safety-First Design**: Admission webhook validation, human-in-the-loop approval gates, configurable confidence thresholds, and effectiveness tracking
- **SOC2 Type II Compliance**: Full RemediationRequest reconstruction from audit traces, operator attribution via webhooks, and hash chain integrity verification
- **Continuous Learning**: Multi-dimensional effectiveness tracking (incident type, workflow, action) to improve remediation success rates over time
- **Enterprise Diagnostics**: [Must-gather](cmd/must-gather/README.md) diagnostic collection following the OpenShift industry standard

---

## Architecture

Kubernaut follows a microservices architecture with 10 production-ready services (6 CRD controllers + 4 stateless services):

![Kubernaut Layered Architecture](docs/architecture/diagrams/kubernaut-layered-architecture.svg)

### Architecture Flow

1. **Gateway Service** receives signals (Prometheus alerts, K8s events), validates resource scope via the `kubernaut.ai/managed` label, and creates `RemediationRequest` CRDs
2. **Remediation Orchestrator** (CRD controller) coordinates remediation lifecycle across 5 other CRD controllers:
   - **Signal Processing Service**: Enriches signals with Kubernetes context, classifies signal mode (reactive/predictive), and normalizes signal types
   - **AI Analysis Service**: Performs HolmesGPT investigation and generates recommendations
   - **Workflow Execution**: Orchestrates Tekton Pipelines or Kubernetes Jobs for remediation workflows
   - **Notification Service**: Delivers multi-channel notifications (Slack, Email, etc.)
   - **Effectiveness Monitor**: Assesses post-remediation health via spec hash comparison, health checks, and metric evaluation
3. **Data Storage Service** provides centralized PostgreSQL access for audit trails and workflow schemas

### Communication Pattern

Kubernaut uses **Kubernetes Custom Resources (CRDs)** for all inter-service communication, enabling:
- Event-driven, resilient workflows
- Built-in retry and reconciliation
- Complete audit trail
- Horizontal scaling

---

## Quick Start

### Prerequisites

- **Go 1.25+** for building services
- **Kubernetes cluster** (Kind recommended for development, v1.30+)
- **PostgreSQL** (for Data Storage service)
- **kubectl** with cluster access

### Build

```bash
make build-all

make build-gateway
make build-datastorage
```

### Testing

```bash
make test-unit-gateway
make test-tier-unit

make test-integration-gateway
make test-e2e-gateway

make test-all-gateway
```

---

## Deployment

Kubernaut services use **Kustomize** for Kubernetes deployment. See the [demo deployment guide](docs/demo/README.md) for a complete walkthrough.

| Service | Deployment Path | Notes |
|---|---|---|
| **Gateway** | `deploy/gateway/` | Kustomize with base/overlays |
| **Data Storage** | `deploy/data-storage/` | Includes PostgreSQL infrastructure |
| **HolmesGPT API** | `deploy/holmesgpt-api/` | Full manifest set |
| **Notification** | `deploy/notification/` | Full manifest set |
| **Auth Webhook** | `deploy/authwebhook/` | Includes webhook configurations |

### Deployment Guides

- **[Gateway Service](deploy/gateway/README.md)**: Signal ingestion with K8s-native state management
- **[Data Storage Service](deploy/data-storage/README.md)**: PostgreSQL-backed unified audit table
- **[HolmesGPT API](deploy/holmesgpt-api/README.md)**: AI-powered root cause analysis
- **[Auth Webhook](deploy/authwebhook/README.md)**: SOC2 operator attribution

---

## For Developers

**New to Kubernaut development?** Start with the **[Developer Guide](docs/DEVELOPER_GUIDE.md)**.

| I want to... | Go to... |
|--------------|----------|
| **Implement a new service** | [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](docs/services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) |
| **Extend an existing service** | [FEATURE_EXTENSION_PLAN_TEMPLATE.md](docs/services/FEATURE_EXTENSION_PLAN_TEMPLATE.md) |
| **Document a service** | [SERVICE_DOCUMENTATION_GUIDE.md](docs/services/SERVICE_DOCUMENTATION_GUIDE.md) |
| **Understand architecture** | [Kubernaut CRD Architecture](docs/architecture/KUBERNAUT_CRD_ARCHITECTURE.md) |

---

## Documentation

**[Documentation Structure Guide](docs/DOCS_DIRECTORY_STRUCTURE_GUIDE.md)** — How the `docs/` directory is organized.

### Architecture

- [Approved Microservices Architecture](docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md) — Service boundaries and roadmap
- [Multi-CRD Reconciliation Architecture](docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md) — CRD communication patterns
- [CRD Schemas](docs/architecture/CRD_SCHEMAS.md) — Authoritative CRD field definitions
- [Tekton Execution Architecture](docs/architecture/TEKTON_EXECUTION_ARCHITECTURE.md) — Workflow orchestration with Tekton Pipelines and Kubernetes Jobs
- [Design Decisions](docs/architecture/decisions/) — All architectural decisions (DD-* and ADR-*)

### Services

- [CRD Controllers](docs/services/crd-controllers/) — Remediation Orchestrator, Signal Processing, AI Analysis, Workflow Execution, Notification, Effectiveness Monitor
- [Stateless Services](docs/services/stateless/) — Gateway, Data Storage, HolmesGPT API, Auth Webhook

### Development

- [Developer Guide](docs/DEVELOPER_GUIDE.md) — Onboarding, environment setup, and contribution workflow
- [Testing Coverage Methodology](docs/testing/TESTING_COVERAGE_METHODOLOGY.md) — Defense-in-depth pyramid with per-tier coverage analysis
- [Development Methodology](docs/development/methodology/) — APDC framework and TDD workflow

---

## Security

### RBAC Configuration

Each CRD controller requires specific Kubernetes permissions. See [RBAC documentation](docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md#security-considerations) for details.

### Service-to-Service Authentication

- **Gateway Service**: Network-level security (NetworkPolicies + TLS)
- **CRD Controllers**: Kubernetes ServiceAccount authentication
- **Inter-service**: SubjectAccessReview middleware authentication

---

## Monitoring & Observability

- **Metrics**: All services expose Prometheus metrics on `:9090/metrics`
- **Health Checks**: `GET /health` and `GET /ready` endpoints on all services
- **Logging**: Structured JSON logging with configurable levels

---

## Contributing

### Development Standards

- **Go**: Standard conventions with comprehensive error handling
- **Testing**: Ginkgo/Gomega BDD framework with defense-in-depth coverage (unit, integration, E2E). Coverage is reported automatically on every PR via CI. See the [Testing Coverage Methodology](docs/testing/TESTING_COVERAGE_METHODOLOGY.md) for details.
- **Documentation**: Comprehensive inline documentation
- **CRD Changes**: Update [CRD_SCHEMAS.md](docs/architecture/CRD_SCHEMAS.md)

### Pull Request Process

1. Create feature branch from `main`
2. Implement with comprehensive tests
3. Update relevant documentation
4. Code review and merge

---

## License

Apache License 2.0

---

## Support & Community

- **Issues**: [GitHub Issues](https://github.com/jordigilh/kubernaut/issues)
- **Discussions**: [GitHub Discussions](https://github.com/jordigilh/kubernaut/discussions)
- **Documentation**: Comprehensive guides in `docs/` directory

---

**Kubernaut** — From alert to remediation, intelligently.
