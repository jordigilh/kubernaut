# Kubernaut Developer Guide

**Version**: 2.1
**Date**: 2026-08-02
**Status**: Active

> **2026-08-02 (Issue #1806)**: Corrected stale HolmesGPT-API/HAPI-era content. There is no
> Python "HolmesGPT API" service, no git submodules, and no `dependencies/` or top-level
> `kubernaut-agent/` directory in this repository. Kubernaut Agent (KA) is a native Go service
> (`cmd/kubernautagent`), built, tested, and containerized exactly like every other service.
> Added the 3 services this guide previously omitted: `apifrontend`, `fleetmetadatacache`, and
> `kubernautagent` itself.

---

## Purpose

This guide is the single entry point for anyone contributing to Kubernaut — whether you are adding a service, extending an existing one, fixing a bug, or reviewing a pull request. It covers environment setup, repository layout, build and test commands, deployment options, and the development methodology the project follows.

**Audience**: Internal team members and external open-source contributors.

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| **Go** | 1.25.6+ | Service development (toolchain 1.25.7) — including Kubernaut Agent, a native Go service |
| **Python** | 3.12+ | Coverage reporting tooling (`scripts/coverage/coverage_report.py`) |
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
├── api/                        # CRD type definitions (11 groups)
│   ├── actiontype/
│   ├── aianalysis/
│   ├── apifrontend/
│   ├── effectivenessassessment/
│   ├── investigationsession/
│   ├── notification/
│   ├── openapi/
│   ├── remediation/
│   ├── remediationworkflow/
│   ├── signalprocessing/
│   └── workflowexecution/
├── cmd/                        # Service entry points (12 services + must-gather CLI tool)
│   ├── aianalysis/
│   ├── apifrontend/
│   ├── authwebhook/
│   ├── datastorage/
│   ├── effectivenessmonitor/
│   ├── fleetmetadatacache/
│   ├── gateway/
│   ├── kubernautagent/         #   Native Go AI investigation engine (replaces the old Python HolmesGPT API)
│   ├── must-gather/            #   Diagnostics CLI tool, not a long-running service
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
├── docs/                       # Project documentation
│   ├── architecture/           #   CRD architecture, schemas, design decisions
│   ├── development/            #   Methodology (APDC), guidelines
│   ├── services/               #   Per-service docs and templates
│   └── tests/                  #   Test plans (per issue)
├── config/                     # Controller-gen and CRD output
├── .github/                    # CI workflows and CODEOWNERS
└── .cursor/rules/              # AI-enforced development standards
```

---

## Setup

```bash
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut

make install        # Install CRDs into current cluster context
make build-all      # Build all Go services
make test-tier-unit # Run unit tests to verify setup
```

There are no git submodules to initialize — Kubernaut Agent (the AI investigation engine) is a
native Go service under `cmd/kubernautagent`, built and tested exactly like every other service
in this repository (see [Building](#building) and [Testing](#testing) below).

> **Adding a new service or extending an existing one?** See
> [Extending the Platform](#extending-the-platform) for the implementation plan templates,
> timelines, and reference services.

---

## Services

Kubernaut is composed of **12 Go services** (under `cmd/`). All services communicate through Kubernetes Custom Resources (CRDs).

| Service | Type | Location | Description |
|---------|------|----------|-------------|
| **gateway** | HTTP Server | `cmd/gateway` | Ingests AlertManager webhooks and Kubernetes Events, deduplicates by fingerprint, resolves owner chains, creates RemediationRequest CRDs |
| **remediationorchestrator** | CRD Controller | `cmd/remediationorchestrator` | Orchestrates the full remediation pipeline: creates child CRDs (SignalProcessing, AIAnalysis, WorkflowExecution, EffectivenessAssessment, Notification), manages approval gates and timeouts |
| **signalprocessing** | CRD Controller | `cmd/signalprocessing` | Enriches K8s context, classifies environment/severity/priority, traverses owner chains, detects custom labels |
| **aianalysis** | CRD Controller | `cmd/aianalysis` | Triggers root cause analysis via an async submit/poll call to Kubernaut Agent and manages workflow selection lifecycle, gated by Rego policy |
| **workflowexecution** | CRD Controller | `cmd/workflowexecution` | Executes remediations via Kubernetes Jobs, Tekton Pipelines, or Ansible (AWX/AAP) — one engine per workflow, Strategy pattern |
| **kubernautagent** | HTTPS Server (native Go) | `cmd/kubernautagent` | AI investigation engine — async session API (submit, then poll); multi-provider LLM support. **Not** a Python/HolmesGPT SDK wrapper; there is no separate Python service to build or run |
| **effectivenessmonitor** | CRD Controller | `cmd/effectivenessmonitor` | Evaluates whether remediations worked (health checks, alert resolution, spec drift) — Level 1 deterministic scoring only; no AI/LLM dependency |
| **datastorage** | HTTP Server | `cmd/datastorage` | Persistence layer (PostgreSQL), workflow catalog, unified audit sink, OpenAPI |
| **notification** | CRD Controller | `cmd/notification` | Delivers Slack and console notifications with remediation context |
| **authwebhook** | Webhook Server | `cmd/authwebhook` | Admission webhooks for CRD validation, registers workflows with DataStorage |
| **apifrontend** | HTTP Server + mini CRD controller | `cmd/apifrontend` | External-facing A2A/MCP natural-language gateway; creates RemediationRequest CRDs directly and calls Kubernaut Agent independently for deep investigation |
| **fleetmetadatacache** | HTTP Server | `cmd/fleetmetadatacache` | Caches multi-cluster ("fleet") metadata for cross-cluster workflow targeting |
| **must-gather** | CLI Tool | `cmd/must-gather` | Diagnostics collection script (not included in `SERVICES` build var) |

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
make docker-build-gateway IMG=quay.io/kubernaut-ai/gateway:dev
make docker-push-gateway  IMG=quay.io/kubernaut-ai/gateway:dev

# Same pattern for every service, including Kubernaut Agent:
make docker-build-kubernautagent IMG=quay.io/kubernaut-ai/kubernautagent:dev
```

The `CONTAINER_TOOL` variable auto-detects Podman or Docker. All 12 services (including
`kubernautagent`) use the same `docker-build-<service>` / `docker-push-<service>` pattern targets
— there is no separate Python build process.

---

## Testing

Kubernaut uses **Ginkgo/Gomega BDD** for all Go tests, across all 12 services (including
`kubernautagent`, a native Go service — there is no separate Python test suite). Standard
`testing.T` tests are not permitted (native Go fuzz tests, `FuzzXxx(f *testing.F)`, are the sole
exception — see [AGENTS.md](../AGENTS.md#exception-go-native-fuzz-tests)).

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

**Kubernaut Agent** (native Go, same pattern as every other service):

```bash
make test-unit-kubernautagent
make test-integration-kubernautagent
make test-e2e-kubernautagent
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

| Tier | Kubernetes API | PostgreSQL / Redis | LLM (via Kubernaut Agent) | `pkg/` business logic |
|------|---------------|-------------------|-----------------|----------------------|
| **Unit** | `fake.NewClientBuilder()` | Mocked | Mocked | Real |
| **Integration** | `envtest` (in-memory API server) | Real containers | Mocked | Real |
| **E2E** | Real Kind cluster | Real containers | Mock LLM | Real |

All `pkg/` business logic must always use real implementations — never mock internal code.

---

## Deployment

### Production — Helm chart

The chart lives in `charts/kubernaut/`. `values.yaml` in that directory is the chart's own
baseline (schema defaults + mandatory-field placeholders) — you don't pass it with `-f`; Helm
loads it automatically. Instead, supply your own minimal values file with the chart's 7
mandatory fields (Rego policies + one LLM profile):

```yaml
# my-values.yaml
global:
  llmProfiles:
    primary:
      provider: openai
      model: gpt-4o
      endpoint: https://api.openai.com/v1
      credentialsSecretName: llm-credentials

signalprocessing:
  policies:
    content: |
      # your Rego classification policy

aianalysis:
  policies:
    content: |
      # your Rego approval policy
```

```bash
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system --create-namespace \
  -f my-values.yaml
```

For air-gapped/disconnected environments, see `charts/kubernaut/values-airgap.yaml`'s header
comment for the pull-and-extract sequence (no live OCI reference at install time).

For OpenShift, use the [Kubernaut Operator](https://jordigilh.github.io/kubernaut-docs/operations/operator/) instead of this Helm chart.

### Development — Local checkout

```bash
helm install kubernaut ./charts/kubernaut \
  --namespace kubernaut-system --create-namespace \
  -f my-values.yaml
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

Full guide: [AGENTS.md](../AGENTS.md#pre-implementation-workflow)

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
| [Architecture Overview](architecture/KUBERNAUT_ARCHITECTURE_OVERVIEW.md) | High-level system design, all 12 active services |
| [Service Catalog](architecture/KUBERNAUT_SERVICE_CATALOG.md) | Per-service specifications, ports, dependencies |
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
