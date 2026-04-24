# Kubernaut

**AIOps Platform for Intelligent Kubernetes Remediation**

[![Go Report Card](https://goreportcard.com/badge/github.com/jordigilh/kubernaut)](https://goreportcard.com/report/github.com/jordigilh/kubernaut)
[![Go Version](https://img.shields.io/badge/Go-1.25.6-blue.svg)](https://golang.org/dl/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.32+-blue.svg)](https://kubernetes.io/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/jordigilh/kubernaut/actions/workflows/ci-pipeline.yml/badge.svg)](https://github.com/jordigilh/kubernaut/actions/workflows/ci-pipeline.yml)

Kubernaut closes the loop from Kubernetes alert to automated remediation. When something goes wrong in your cluster, Kubernaut detects the signal, sends it to an LLM-powered agent that investigates the root cause using native Go client-go bindings against the Kubernetes API, log, and Prometheus endpoints, selects a remediation workflow, and executes the fix — or escalates to a human with a full RCA when it can't.

<p align="center">
  <img src="https://raw.githubusercontent.com/jordigilh/kubernaut-demo-scenarios/main/scenarios/crashloop/crashloop-lite.gif" alt="CrashLoopBackOff demo — from alert to automated fix in under 5 minutes" width="800"/>
</p>

<p align="center">
  <a href="https://jordigilh.github.io/kubernaut-docs/"><strong>Full Documentation</strong></a> &nbsp;·&nbsp;
  <a href="https://github.com/jordigilh/kubernaut-demo-scenarios"><strong>Demo Scenarios</strong></a> &nbsp;·&nbsp;
  <a href="https://github.com/jordigilh/kubernaut/releases/tag/v1.3.0"><strong>Latest Release</strong></a>
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
- **Investigates** — Performs live root cause analysis using Kubernetes inspection tools, configurable observability toolsets (Prometheus, etc.), and remediation history
- **Remediates** — Selects and executes a workflow from a searchable catalog via Tekton Pipelines, Kubernetes Jobs, or Ansible (AWX/AAP), with optional human approval gates
- **Closes the loop** — Notifies the team (Slack, console), evaluates whether the fix worked via health checks, alert resolution, and spec hash drift detection, and feeds effectiveness scores back into future investigations

<details>
<summary>Architecture</summary>

![Kubernaut Layered Architecture](docs/architecture/diagrams/kubernaut-layered-architecture.svg)

</details>

---

## Roadmap

### v1.4 — Operator Overrides and Platform Hardening (current)

- **Prompt injection guardrails** — Shadow agent with a dedicated scanning model to protect the agentic pipeline against prompt injection attacks ([#601](https://github.com/jordigilh/kubernaut/issues/601))
- **Operator workflow/parameter override** — Operators can override workflow selection and parameters during RAR approval, with authwebhook validation ([#594](https://github.com/jordigilh/kubernaut/issues/594))
- **PagerDuty and Microsoft Teams** — New notification delivery channels alongside Slack and console ([#60](https://github.com/jordigilh/kubernaut/issues/60), [#593](https://github.com/jordigilh/kubernaut/issues/593))
- **Unified monitoring config** — Single `monitoring:` Helm block replacing per-component Prometheus/AlertManager keys, with OCP auto-detection ([#463](https://github.com/jordigilh/kubernaut/issues/463))
- **NetworkPolicies** — Default-deny network policies for all services based on the traffic matrix ([#285](https://github.com/jordigilh/kubernaut/issues/285))

Track progress on the [v1.4 milestone](https://github.com/jordigilh/kubernaut/milestone/5).

### v2.0 — Intelligent Operations Platform (next)

Kubernaut evolves from an automated remediation engine into an extensible intelligent operations platform. The core pipeline (SP classification, AA policy gates, audited execution) remains the safety architecture; the intelligence layer becomes externalized and pluggable. See [#818](https://github.com/jordigilh/kubernaut/issues/818) for the full vision.

- **MCP interactive mode** — Investigate, enrich, and select workflows through any MCP-compatible interface — IDE copilots, Slack bots, or custom UIs ([#703](https://github.com/jordigilh/kubernaut/issues/703))
- **A2A protocol support** — External AI agents delegate remediation to Kubernaut and track task lifecycle via the [Agent-to-Agent](https://a2aproject.github.io/A2A/latest/specification/) standard ([#705](https://github.com/jordigilh/kubernaut/issues/705))
- **API Frontend service** — Unified external protocol layer hosting MCP and inbound A2A endpoints, with shared CRD watching for live status streaming ([#708](https://github.com/jordigilh/kubernaut/issues/708))
- **Investigation Prompt Bundles** — Operators inject SOPs into the investigation pipeline via OCI-packaged prompts and skills with five hook phases (pre-investigation, investigation, post-investigation, rca-resolution, workflow-selection) ([#711](https://github.com/jordigilh/kubernaut/issues/711))
- **Natural language signal intake** — Trigger investigations by describing the problem in plain text; Kubernaut extracts a structured signal and runs the full pipeline ([#714](https://github.com/jordigilh/kubernaut/issues/714))
- **Kubernaut Console** — Web-based operator dashboard with chat UI, live remediation streaming, and workflow selection ([#713](https://github.com/jordigilh/kubernaut/issues/713))

<p align="center">
  <img src="docs/architecture/diagrams/kubernaut-interactive-console-mockup.png" alt="Kubernaut Console — interactive investigation and remediation" width="800"/>
</p>

### Future

- **Non-K8s workflow support** — `targetSystem` field enables execution against external systems (ServiceNow, Jira, cloud APIs) with EA evolution for unverifiable outcomes ([#739](https://github.com/jordigilh/kubernaut/issues/739))
- **Multi-agent consensus RCA** — Ensemble investigation with independent LLM agents from different model families; a consolidator validates agreement and cross-examines on divergence ([#648](https://github.com/jordigilh/kubernaut/issues/648))
- **Multi-cluster federation** — A2A-based fleet-wide remediation with centralized signal ingestion, cross-cluster RCA, and federated workflow execution
- **Multi-adapter Gateway** — Signal intake from ServiceNow, OTEL, CloudWatch, Jira, and custom webhooks beyond AlertManager
- **Goose runtime integration** — KA as a supervised harness for Goose-executed prompt bundles with MCP tool discovery
- **EA domain-specific assessors** — Pluggable verification strategies per execution target (K8s, ServiceNow, cloud APIs)
- **Skills marketplace** — OCI artifact registry for community and enterprise MCP tool packages

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
make build-all          # Build all services
make test-tier-unit     # Run unit tests
make test-all-gateway   # Run all test tiers for a service
```

We use **Ginkgo/Gomega BDD** for testing and follow a TDD workflow. See the [Developer Guide](docs/DEVELOPER_GUIDE.md) for environment setup, build targets, and test commands.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. In short: create a feature branch, implement with tests, update docs, and open a PR.

---

## License

Apache License 2.0 — see [LICENSE](LICENSE).

---

**Issues**: [GitHub Issues](https://github.com/jordigilh/kubernaut/issues) · **Discussions**: [GitHub Discussions](https://github.com/jordigilh/kubernaut/discussions)

**Kubernaut** — From alert to remediation, intelligently.
