# Kubernaut Developer Guide

**Version**: 2.0
**Date**: 2026-03-21
**Status**: Active

---

## Purpose

This guide is the single entry point for anyone contributing to Kubernaut — whether you are adding a service, extending an existing one, fixing a bug, or reviewing a pull request. It covers environment setup, repository layout, build and test commands, deployment options, and the development methodology the project follows.

**Audience**: Internal team members and external open-source contributors.

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| **Go** | 1.25.6+ | Service development (toolchain 1.25.7) |
| **Python** | 3.12+ | HolmesGPT API service |
| **Kubernetes** | 1.32+ | Runtime platform |
| **kubectl** | 1.32+ | Cluster management |
| **Kind** | 0.30+ | Local development clusters |
| **Podman** or **Docker** | Latest | Container image builds |
| **Helm** | 3.14+ | Chart packaging and deployment |
| **Ginkgo** | v2+ | BDD testing framework (Go services) |
| **golangci-lint** | Latest | Go linter |

---

## Repository Layout

```
kubernaut/
├── api/                        # CRD type definitions (9 groups)
│   ├── actiontype/
│   ├── aianalysis/
│   ├── effectivenessassessment/
│   ├── notification/
│   ├── openapi/
│   ├── remediation/
│   ├── remediationworkflow/
│   ├── signalprocessing/
│   └── workflowexecution/
├── cmd/                        # Service entry points (10 services)
│   ├── aianalysis/
│   ├── authwebhook/
│   ├── datastorage/
│   ├── effectivenessmonitor/
│   ├── gateway/
│   ├── must-gather/
│   ├── notification/
│   ├── remediationorchestrator/
│   ├── signalprocessing/
│   └── workflowexecution/
├── pkg/                        # Service business logic
├── internal/                   # Shared internal packages (errors, controller helpers)
├── test/                       # All test suites
│   ├── unit/                   #   Per-service unit tests
│   ├── integration/            #   Per-service integration tests
│   ├── e2e/                    #   Per-service E2E tests
│   ├── testutil/               #   Shared test helpers
│   ├── fixtures/               #   Test data (workflow schemas, CRD samples)
│   └── ...                     #   Infrastructure, load, chaos, etc.
├── charts/kubernaut/           # Helm chart (production deployment)
├── deploy/                     # Kustomize overlays (individual service development)
├── holmesgpt-api/              # Python service (HolmesGPT API)
│   ├── src/
│   └── tests/
├── docs/                       # Project documentation
│   ├── architecture/           #   CRD architecture, schemas, design decisions
│   ├── development/            #   Methodology (APDC), guidelines
│   ├── services/               #   Per-service docs and templates
│   └── tests/                  #   Test plans (per issue)
├── dependencies/               # Git submodules (holmesgpt SDK)
├── config/                     # Controller-gen and CRD output
├── .github/                    # CI workflows and CODEOWNERS
└── .cursor/rules/              # AI-enforced development standards
```

---

## Setup

```bash
git clone --recurse-submodules https://github.com/jordigilh/kubernaut.git
cd kubernaut

make install        # Install CRDs into current cluster context
make build-all      # Build all Go services
make test-tier-unit # Run unit tests to verify setup
```

If you already cloned without `--recurse-submodules`, initialize the submodule separately:
- ✅ **Production readiness checklist** (109-point assessment)

**Timeline Overview**:
| Phase | Days | Focus | Deliverables |
|-------|------|-------|--------------|
| **Foundation** | 1 | Types, interfaces, K8s client | Package structure, interfaces |
| **Core Logic** | 2-6 | Business logic components | All components implemented |
| **Integration** | 7 | Server, API, metrics | Complete service |
| **Testing** | 8-10 | Integration + Unit tests | 70%+ coverage |
| **Finalization** | 11-12 | E2E, docs, production readiness | Ready for deployment |

### **Step 2: Follow APDC-TDD Methodology**

**APDC Phases** (per feature/component):
1. **Analysis** (5-15 min): Comprehensive context understanding
2. **Plan** (10-20 min): Detailed implementation strategy
3. **Do** (Variable): RED → GREEN → REFACTOR with integration
4. **Check** (5-10 min): Comprehensive validation

**Key Principle**: Write tests FIRST, then implementation.

### **Step 3: Create Service Documentation**

After implementation (Day 11-12), use:

📘 **[SERVICE_DOCUMENTATION_GUIDE.md](services/SERVICE_DOCUMENTATION_GUIDE.md)**

**Required Documents** (7 minimum):
1. `overview.md` - Service purpose, CRD schema, architecture
2. `security-configuration.md` - RBAC, NetworkPolicy, Secrets
3. `observability-logging.md` - Structured logging, tracing
4. `metrics-slos.md` - Prometheus metrics, SLI/SLO
5. `testing-strategy.md` - Unit/Integration/E2E tests
6. `finalizers-lifecycle.md` - Cleanup coordination (CRD controllers)
7. `controller-implementation.md` - Reconciliation loop (CRD controllers)

**Reference Services** (copy and adapt):
- **Service 1 (Signal Processing)**: Data processing patterns
- **Service 2 (AI Analysis)**: AI/ML integration patterns
- **Service 3 (Workflow Execution)**: Multi-step orchestration
- **Service 4 (Kubernetes Executor)**: Action execution patterns
- **Service 5 (Remediation Orchestrator)**: Central orchestration

---

## 🔧 **Extending Existing Services**

### **When to Extend vs. Create New**

**Extend existing service** when:
- ✅ Feature fits within service's bounded context
- ✅ No new CRD required
- ✅ Shares same data model and dependencies

**Create new service** when:
- ❌ Feature requires new CRD
- ❌ Different scaling/deployment requirements
- ❌ Distinct bounded context

### **Feature Extension Process**

📘 **[FEATURE_EXTENSION_PLAN_TEMPLATE.md](services/FEATURE_EXTENSION_PLAN_TEMPLATE.md)** ⭐ **USE THIS TEMPLATE**

**What it provides**:
- ✅ **Flexible timeline** (3-12 days based on complexity)
- ✅ **APDC-TDD methodology** (Analysis → Plan → Do → Check)
- ✅ **Day-by-day breakdown** with customizable phases
- ✅ **Complete test examples** (Unit, Integration, E2E)
- ✅ **Documentation timeline** (what gets created when)
- ✅ **TDD Do's and Don'ts** (strict discipline)
- ✅ **BR Coverage Matrix**
- ✅ **Rollback Plan**

**Timeline Guide**:
| Feature Complexity | Duration | When to Use |
|--------------------|----------|-------------|
| **Simple** | 3-5 days | 1-2 files, minimal integration |
| **Medium** | 5-8 days | 3-5 files, moderate integration |
| **Complex** | 8-12 days | 5+ files, significant integration |

**Reference Examples**:
- **[DD-GATEWAY-008](architecture/decisions/DD-GATEWAY-008-storm-aggregation-windows.md)** — historical storm aggregation DD (**superseded**; removal in [DD-GATEWAY-015](architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md))
- **[DD-GATEWAY-009](architecture/decisions/DD-GATEWAY-009-state-based-deduplication.md)** — state-based (fingerprint) deduplication at the Gateway

**Key Differences from New Service**:
- ✅ Shorter timeline (no foundation setup)
- ✅ Focus on enhancing existing code (not creating new)
- ✅ Regression testing emphasis
- ✅ Integration impact assessment

### **Documentation During Feature Extension**

**📊 What Gets Created When**:

```
Day 1-[N] (Implementation):
    ├── Code Documentation (inline GoDoc, BR references)
    ├── Daily EOD Reports (progress checkpoints)
    └── Configuration Comments (YAML inline docs)

Days [N+1]-[N+M] (Testing):
    ├── Test Documentation (test descriptions, BR mapping)
    ├── Test Helper Documentation
    └── Edge Case Documentation

Day [N+M+1] (Documentation Day):
    ├── Finalize Service Docs (update existing files)
    │   ├── overview.md (add feature, update version)
    │   ├── BUSINESS_REQUIREMENTS.md (add BRs, links)
    │   ├── testing-strategy.md (add test examples)
    │   └── metrics-slos.md (add new metrics)
    │
    └── Create Operational Docs (new files if needed)
        ├── Runbook (if feature affects operations)
        └── Migration Guide (if breaking changes)

Day [N+M+P] (Production Readiness):
    └── Handoff Summary (executive summary, lessons learned)
```

**Key Point**: Most documentation is created **DURING** implementation (inline), not at the end. The documentation day is for **finalizing** and **consolidating**.

---

## 🧪 **Testing**

### **Testing Strategy**

Kubernaut follows **defense-in-depth testing pyramid**:

- **Unit Tests**: **70%+ coverage** - Business logic with external mocks only
- **Integration Tests**: **>50% coverage** - Component interactions, real K8s API
- **E2E Tests**: **<10% coverage** - Critical user journeys

**Reference**: [03-testing-strategy.mdc](../.cursor/rules/03-testing-strategy.mdc)

```bash
git submodule update --init --recursive
```

For the Python service (HolmesGPT API):

```bash
cd holmesgpt-api
pip install -r requirements.txt
pytest tests/unit/ -v
```

---

## Services

Kubernaut is composed of 10 Go services (under `cmd/`) and 1 Python service. All services communicate through Kubernetes Custom Resources (CRDs).

| Service | Type | Location | Description |
|---------|------|----------|-------------|
| **gateway** | HTTP Server | `cmd/gateway` | Ingests AlertManager webhooks and Kubernetes Events, deduplicates by fingerprint, resolves owner chains, creates RemediationRequest CRDs |
| **remediationorchestrator** | CRD Controller | `cmd/remediationorchestrator` | Orchestrates the full remediation pipeline: creates child CRDs (SignalProcessing, AIAnalysis, WorkflowExecution, EffectivenessAssessment, Notification), manages approval gates and timeouts |
| **signalprocessing** | CRD Controller | `cmd/signalprocessing` | Enriches K8s context, classifies environment/severity/priority, traverses owner chains, detects custom labels |
| **aianalysis** | CRD Controller | `cmd/aianalysis` | Triggers LLM-based root cause analysis via HolmesGPT API and manages workflow selection lifecycle |
| **workflowexecution** | CRD Controller | `cmd/workflowexecution` | Executes remediations via Kubernetes Jobs, Tekton Pipelines, or Ansible (AWX/AAP) |
| **effectivenessmonitor** | CRD Controller | `cmd/effectivenessmonitor` | Evaluates whether remediations worked (health checks, alert resolution, spec drift) |
| **datastorage** | HTTP Server | `cmd/datastorage` | Persistence layer (PostgreSQL), workflow catalog, audit trail, OpenAPI |
| **notification** | CRD Controller | `cmd/notification` | Delivers Slack and console notifications with remediation context |
| **authwebhook** | Webhook Server | `cmd/authwebhook` | Admission webhooks for CRD validation, registers workflows with DataStorage |
| **must-gather** | CLI Tool | `cmd/must-gather` | Diagnostics collection script (not included in `SERVICES` build var) |
| **holmesgpt-api** | Python | `holmesgpt-api/` | REST wrapper around the HolmesGPT SDK for LLM investigations |

---

## Building

### Go services

```bash
make build-all              # Build every Go service
make build-gateway          # Build a single service
make build-aianalysis
```

The `SERVICES` variable is auto-discovered from `cmd/` (excluding `must-gather` and `README.md`). You can override it:

```bash
make build-all SERVICES="gateway datastorage"
```

### Container images

```bash
make docker-build IMG=quay.io/kubernaut-ai/gateway:dev
make docker-push  IMG=quay.io/kubernaut-ai/gateway:dev
```

The `CONTAINER_TOOL` variable auto-detects Podman or Docker.

### HolmesGPT API (Python)

```bash
cd holmesgpt-api
podman build -t quay.io/kubernaut-ai/holmesgpt-api:dev .
```

---

## Testing

Kubernaut uses **Ginkgo/Gomega BDD** for all Go tests. Standard `testing.T` tests are not permitted. Python tests use **pytest**.

### Coverage targets

Every tier must reach **>=80% coverage** of the code subset it is responsible for:

| Tier | Scope | Target |
|------|-------|--------|
| **Unit** | Pure logic: config, validators, scoring, builders, formatters | >=80% of unit-testable code |
| **Integration** | I/O-dependent: reconcilers, K8s clients, HTTP handlers, DB adapters | >=80% of integration-testable code |
| **E2E** | Full-stack execution in Kind | >=80% of full service code |

### Commands

**Per-tier (all services)**:

```bash
make test-tier-unit
make test-tier-integration
make test-tier-e2e
```

**Per-service (all tiers)**:

```bash
make test-all-gateway
make test-all-aianalysis
```

**Per-service, per-tier**:

```bash
make test-unit-gateway
make test-integration-gateway
make test-e2e-gateway
```

**HolmesGPT API (Python)**:

```bash
make test-unit-holmesgpt-api
make test-integration-holmesgpt-api
make test-e2e-holmesgpt-api
```

### Linting

```bash
golangci-lint run --timeout=5m   # Go lint
make lint-rules                  # Workspace rule compliance
make lint-test-patterns          # Test anti-pattern detection
make lint-business-integration   # Business code integration check
make lint-tdd-compliance         # TDD methodology compliance
```

### Testing principles

- **Behavior over implementation**: Test what the system does through its public API, not how it does it internally.
- **Business requirement mapping**: Every test must reference a business requirement (`BR-[CATEGORY]-[NUMBER]`) or a test scenario ID (`UT-WF-197-001`).
- **Test plans**: Create a formal test plan before implementation using the [Test Plan Template](testing/TEST_PLAN_TEMPLATE.md). See the [test plan policy](architecture/decisions/DD-TEST-006-test-plan-policy.md) for when a plan is required.
- **No pending tests**: Never use `XIt` or `Skip()`. Either implement the test or remove it.

### Mock strategy per tier

| Tier | Kubernetes API | PostgreSQL / Redis | LLM (HolmesGPT) | `pkg/` business logic |
|------|---------------|-------------------|-----------------|----------------------|
| **Unit** | `fake.NewClientBuilder()` | Mocked | Mocked | Real |
| **Integration** | `envtest` (in-memory API server) | Real containers | Mocked | Real |
| **E2E** | Real Kind cluster | Real containers | Mock LLM | Real |

All `pkg/` business logic must always use real implementations — never mock internal code.

---

## Deployment

### Production — Helm chart

```bash
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system --create-namespace
```

The chart lives in `charts/kubernaut/` and supports value files for different environments:

| Values file | Purpose |
|-------------|---------|
| `values.yaml` | Default (Kind / vanilla Kubernetes) |
| `values-ocp.yaml` | OpenShift-specific overrides |
| `values-airgap.yaml` | Air-gapped / disconnected environments |

### Development — Local checkout

```bash
helm install kubernaut ./charts/kubernaut \
  --namespace kubernaut-system --create-namespace \
  -f charts/kubernaut/values.yaml
```

### Individual services — Kustomize

For developing or debugging a single service, Kustomize overlays are available under `deploy/`:

```bash
kubectl apply -k deploy/gateway/overlays/kubernetes/   # Vanilla K8s
oc apply -k deploy/gateway/overlays/openshift/          # OpenShift
```

### CRD management

```bash
make manifests   # Regenerate CRD YAML from Go types
make install     # Apply CRDs to the current cluster context
```

---

## Development Workflow

### APDC methodology

Complex tasks follow four phases:

1. **Analysis** (5-15 min) — Understand context, map business requirements, assess risks.
2. **Plan** (10-20 min) — Design strategy, define TDD test scenarios, get user approval.
3. **Do** (Variable) — RED (failing test) -> GREEN (minimal passing implementation) -> REFACTOR (improve quality).
4. **Check** (5-10 min) — Validate coverage, run lints, provide a confidence assessment (60-100%).

Full guide: [APDC Framework](development/methodology/APDC_FRAMEWORK.md)

### TDD RED-GREEN-REFACTOR

All development follows strict TDD. Before writing code, create a test plan using the [Test Plan Template](testing/TEST_PLAN_TEMPLATE.md) to define the test scenarios up front.

1. **RED** — Write a failing test that defines the expected behavior.
2. **GREEN** — Write the minimal code to make the test pass. Integrate with `cmd/` in this phase.
3. **REFACTOR** — Improve code quality without changing behavior. No new types in this phase.

### Business requirements

Every code change must map to at least one business requirement:

**Format**: `BR-[CATEGORY]-[NUMBER]` (e.g., `BR-GATEWAY-016`, `BR-AI-056`)

**Categories**: `WORKFLOW`, `AI`, `INTEGRATION`, `SECURITY`, `PLATFORM`, `API`, `STORAGE`, `MONITORING`, `SAFETY`, `PERFORMANCE`

### Pull request checklist

- [ ] All tests pass (`make test-tier-unit`, integration, E2E as applicable)
- [ ] No new lint errors (`golangci-lint run --timeout=5m`)
- [ ] Business requirement mapped (BR-[CATEGORY]-[NUMBER])
- [ ] New business code wired into `cmd/` entry point
- [ ] Documentation updated (if public-facing behavior changed)
- [ ] Confidence assessment provided (60-100% with justification)

---

## Extending the Platform

### Adding a new service

Use the implementation plan template, which provides a 12-day timeline with APDC-TDD phases:

[SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)

After implementation, create service documentation following:

[SERVICE_DOCUMENTATION_GUIDE.md](services/SERVICE_DOCUMENTATION_GUIDE.md)

### Extending an existing service

For features that fit within a service's bounded context and do not require a new CRD:

[FEATURE_EXTENSION_PLAN_TEMPLATE.md](services/FEATURE_EXTENSION_PLAN_TEMPLATE.md)

### Adding a new CRD

1. Create the Go types under `api/<group>/v1alpha1/`.
2. Run `make manifests` to generate the CRD YAML.
3. Run `make install` to apply to your dev cluster.
4. Update [CRD_SCHEMAS.md](architecture/CRD_SCHEMAS.md) with the new field definitions.

---

## Architecture References

| Document | Description |
|----------|-------------|
| [Kubernaut CRD Architecture](architecture/KUBERNAUT_CRD_ARCHITECTURE.md) | System overview, service specs, CRD communication patterns |
| [Multi-CRD Reconciliation Architecture](architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md) | Watch-based coordination, owner references, cascade deletion |
| [CRD Schemas](architecture/CRD_SCHEMAS.md) | Authoritative field definitions and validation rules |
| [V1 Source of Truth Hierarchy](V1_SOURCE_OF_TRUTH_HIERARCHY.md) | Documentation authority: Architecture > Services > Design |
| [Architecture Decision Records](architecture/decisions/) | ADR directory with rationale for key decisions |

---

## Getting Help

- **Issues**: [GitHub Issues](https://github.com/jordigilh/kubernaut/issues)
- **Discussions**: [GitHub Discussions](https://github.com/jordigilh/kubernaut/discussions)
- **Documentation site**: [jordigilh.github.io/kubernaut-docs](https://jordigilh.github.io/kubernaut-docs/)
- **Demo scenarios**: [kubernaut-demo-scenarios](https://github.com/jordigilh/kubernaut-demo-scenarios)
