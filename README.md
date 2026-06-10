# Kubernaut

**AIOps Platform for Intelligent Kubernetes Remediation**

[![Go Report Card](https://goreportcard.com/badge/github.com/jordigilh/kubernaut)](https://goreportcard.com/report/github.com/jordigilh/kubernaut)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue.svg)](https://golang.org/dl/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.32+-blue.svg)](https://kubernetes.io/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/jordigilh/kubernaut/actions/workflows/ci-pipeline.yml/badge.svg)](https://github.com/jordigilh/kubernaut/actions/workflows/ci-pipeline.yml)

Kubernaut closes the loop from Kubernetes alert to automated remediation. It operates in two modes: **autonomously** — detecting signals, investigating root causes, and executing fixes end-to-end without human involvement — and **interactively** — letting operators join an in-progress investigation via MCP or A2A, guide the agent, and approve remediations in real time. The LLM-powered agent uses native Go client-go bindings against the Kubernetes API, Prometheus, and log endpoints to investigate, select a remediation workflow, and execute the fix — or escalate to a human with a full RCA when it can't.

<p align="center">
  <img src="https://raw.githubusercontent.com/jordigilh/kubernaut-demo-scenarios/main/scenarios/crashloop/crashloop-lite.gif" alt="CrashLoopBackOff demo — from alert to automated fix in under 5 minutes" width="800"/>
</p>

<p align="center">
  <a href="https://jordigilh.github.io/kubernaut-docs/"><strong>Full Documentation</strong></a> &nbsp;·&nbsp;
  <a href="https://github.com/jordigilh/kubernaut-demo-scenarios"><strong>Demo Scenarios</strong></a> &nbsp;·&nbsp;
  <a href="https://github.com/jordigilh/kubernaut/releases/tag/v1.5.0"><strong>Latest Release (v1.5.0)</strong></a>
</p>

---

## Why

Kubernetes operators spend hours manually triaging alerts, diagnosing root causes from scattered logs and metrics, and executing remediation steps from runbooks that drift out of date. The response depends on tribal knowledge, human availability, and often happens at 3am.

Rule-based remediation tools help with known, deterministic problems — "if X, do Y." But when the same symptom has multiple root causes, or the right fix depends on context the rule can't see, they fall short.

Kubernaut bridges that gap. It uses an LLM agent that investigates the actual root cause through native Go bindings against the Kubernetes API and observability stack, selects the right remediation from a workflow catalog, executes it, and verifies the fix worked — escalating to humans only when it should. Rule-based tools are thermostats. Kubernaut is a diagnostician that also adjusts the thermostat.

**[Why Kubernaut? — full comparison with rule-based tools](https://jordigilh.github.io/kubernaut-docs/latest/getting-started/why-kubernaut/)**

---

## What It Does

- **Detects** — Ingests Prometheus AlertManager alerts and Kubernetes Events, validates resource scope, and deduplicates by fingerprint
- **Triages** — Resolves signal severity through a multi-tier pipeline (firing alerts, Prometheus rule evaluation, LLM-based triage) and derives grounded signal names from infrastructure context
- **Investigates** — Performs live root cause analysis using Kubernetes inspection tools, configurable observability toolsets (Prometheus, etc.), and remediation history. Runs **autonomously** end-to-end, or **interactively** with an operator guiding the investigation in real time via MCP tools or A2A sessions
- **Integrates** — Exposes MCP and A2A (Agent-to-Agent) protocol endpoints through the API Frontend, enabling external agents, UIs, and automation to interact with Kubernaut via OIDC-authenticated sessions. Operators can take over autonomous sessions mid-flight, review findings, and approve next steps
- **Remediates** — Selects and executes a workflow from a searchable catalog via Tekton Pipelines, Kubernetes Jobs, or Ansible (AWX/AAP), with optional human approval gates
- **Closes the loop** — Notifies the team (Slack, webhook), evaluates whether the fix worked via health checks, alert resolution, and spec hash drift detection, and feeds effectiveness scores back into future investigations

<details>
<summary>Architecture</summary>

![Kubernaut Layered Architecture](docs/architecture/diagrams/kubernaut-layered-architecture.svg)

</details>

<details>
<summary>Services</summary>

| Service | Path | Description |
|---|---|---|
| **Gateway** | `cmd/gateway` | Signal ingestion — AlertManager webhooks and Kubernetes Events |
| **Signal Processing** | `cmd/signalprocessing` | Signal enrichment, deduplication, and routing |
| **Remediation Orchestrator** | `cmd/remediationorchestrator` | CRD lifecycle orchestration across the pipeline |
| **AI Analysis** | `cmd/aianalysis` | Investigation controller — dispatches to Kubernaut Agent |
| **Kubernaut Agent** | `cmd/kubernautagent` | LLM-powered RCA, workflow selection, and MCP tool execution |
| **API Frontend** | `cmd/apifrontend` | External protocol layer — MCP, A2A, OIDC auth, severity triage |
| **Workflow Execution** | `cmd/workflowexecution` | Tekton Pipeline / Job / Ansible execution engine |
| **Data Storage** | `cmd/datastorage` | Workflow catalog, audit trail, and persistence (PostgreSQL) |
| **Notification** | `cmd/notification` | Slack, webhook, and console notification delivery |
| **Effectiveness Monitor** | `cmd/effectivenessmonitor` | Post-remediation health checks and effectiveness scoring |
| **Auth Webhook** | `cmd/authwebhook` | Kubernetes authentication webhook for service identity |

</details>

---

## Roadmap

### v1.5 — Agentic Integration ([released](https://github.com/jordigilh/kubernaut/releases/tag/v1.5.0))

- **Dual-mode investigation** — Kubernaut operates **autonomously** (alert-to-fix with zero human involvement) and **interactively** (operator joins via MCP/A2A, guides the agent, approves actions). Operators can take over an autonomous session mid-flight without restarting the investigation ([#703](https://github.com/jordigilh/kubernaut/issues/703), [#823](https://github.com/jordigilh/kubernaut/issues/823))
- **API Frontend service** — Unified external protocol layer (MCP + A2A) with OIDC authentication, severity triage pipeline, and natural language signal intake
- **Severity triage pipeline** — Multi-tier severity resolution (Prometheus alerts, rule evaluation, LLM-based triage) with pod correlation and signal name derivation
- **A2A protocol** — Agent-to-Agent integration enabling external AI agents and automation platforms to trigger investigations and remediations via JSON-RPC

### v1.5.x — ITSM Integration (upcoming)

- **ServiceNow incident triage** — Bi-directional integration with ServiceNow for incident creation, enrichment, and remediation status sync

### v1.6 — Fleet Management (next)

- **Fleet operations** — Multi-cluster remediation orchestration via ACM/AAP, enabling policy-driven remediation across fleet-scale Kubernetes environments
- **Kubernaut Console** — Web UI for interactive investigation, remediation monitoring, and workflow management

<p align="center">
  <img src="docs/architecture/diagrams/kubernaut-console-animated.gif" alt="Kubernaut Console — interactive investigation and remediation" width="800"/>
  <br/>
  <em>Coming in v1.6 — Kubernaut Console: investigate, chat, and remediate from a single UI</em>
</p>

**[Full roadmap](docs/roadmap/ROADMAP.md)** — Fleet Operations (ACM/AAP), Collective Intelligence, and Operational Expansion (cost, security, non-K8s). For past releases, see the [CHANGELOG](CHANGELOG.md).

---

## Installation

See the [Installation Guide](https://jordigilh.github.io/kubernaut-docs/latest/getting-started/installation/) for prerequisites, configuration, and deployment instructions.

---

## Documentation

| Resource | Link |
|---|---|
| **User & Operator Guide** | [jordigilh.github.io/kubernaut-docs](https://jordigilh.github.io/kubernaut-docs/) |
| **Architecture Overview** | [Architecture](https://jordigilh.github.io/kubernaut-docs/latest/getting-started/architecture-overview/) |
| **Developer Guide** | [docs/DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md) |
| **Must-Gather Diagnostics** | [cmd/must-gather/README.md](cmd/must-gather/README.md) |

---

## Related Repositories

| Repository | Description |
|---|---|
| [kubernaut-docs](https://github.com/jordigilh/kubernaut-docs) | Documentation website (MkDocs Material) |
| [kubernaut-demo-scenarios](https://github.com/jordigilh/kubernaut-demo-scenarios) | Demo scenarios, scripts, and recordings |

---

## Development

```bash
make build-all                    # Build all services
make test-tier-unit               # Run unit tests (all services)
make test-integration-apifrontend # Run integration tests for a service
make test-e2e-apifrontend         # Run E2E tests for a service (Kind cluster)
make test-all-gateway             # Run all test tiers for a service
make lint                         # Run golangci-lint across the monorepo
```

We use **Ginkgo/Gomega BDD** for testing and follow a strict TDD workflow with a defense-in-depth testing pyramid (unit, integration, E2E). See the [Developer Guide](docs/DEVELOPER_GUIDE.md) for environment setup, build targets, and test commands.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. In short: create a feature branch, implement with tests, update docs, and open a PR.

---

## License

Apache License 2.0 — see [LICENSE](LICENSE).

---

**Issues**: [GitHub Issues](https://github.com/jordigilh/kubernaut/issues) · **Discussions**: [GitHub Discussions](https://github.com/jordigilh/kubernaut/discussions)

**Kubernaut** — From alert to remediation, intelligently.
