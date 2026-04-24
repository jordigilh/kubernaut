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

Kubernaut evolves from an automated remediation engine into an intelligent operations platform. Each milestone builds on the previous — control, then integration, then intelligence, then scale, then expansion.

<p align="center">
  <img src="docs/roadmap/kubernaut-roadmap.svg" alt="Kubernaut Roadmap — from autonomous AI to intelligent operations platform" width="100%"/>
</p>

### Delivered

| Release | Theme | Highlights |
|---------|-------|------------|
| **v1.3.0** | Kubernaut Agent | Ground-up Go rewrite of the AI Agent, OLM Operator, inter-pod TLS |
| **v1.4.0** | Platform Hardening | Prompt injection guardrails ([#601](https://github.com/jordigilh/kubernaut/issues/601)), operator overrides ([#594](https://github.com/jordigilh/kubernaut/issues/594)), PagerDuty/Teams ([#60](https://github.com/jordigilh/kubernaut/issues/60), [#593](https://github.com/jordigilh/kubernaut/issues/593)), NetworkPolicies ([#285](https://github.com/jordigilh/kubernaut/issues/285)) |

Track v1.4 progress on the [milestone](https://github.com/jordigilh/kubernaut/milestone/5).

### What's Next

**Live Investigation Control** — See and steer your AI investigations in real time. Real-time investigation streaming, cancel and takeover of long-running investigations, investigation snapshots, and sub-second token-level reasoning updates.

**Agentic Integration** — Your tools talk to Kubernaut natively. MCP Server for any MCP-compatible client ([#703](https://github.com/jordigilh/kubernaut/issues/703)), A2A protocol for agent-to-agent delegation ([#705](https://github.com/jordigilh/kubernaut/issues/705)), API Frontend ([#708](https://github.com/jordigilh/kubernaut/issues/708)), Operator Console ([#713](https://github.com/jordigilh/kubernaut/issues/713)), natural language signal intake ([#714](https://github.com/jordigilh/kubernaut/issues/714)), and investigation prompt bundles ([#711](https://github.com/jordigilh/kubernaut/issues/711)). See [#818](https://github.com/jordigilh/kubernaut/issues/818) for the full vision.

**Collective Intelligence** — Multiple AI perspectives, one root cause. Multi-agent consensus RCA ([#648](https://github.com/jordigilh/kubernaut/issues/648)), OCI-packaged prompt bundles, and forensic replay of past remediations.

**Fleet Operations** — From one cluster to your entire fleet. Multi-cluster federation via A2A, cross-cluster pattern detection, and a skills marketplace for shared playbooks.

**Operational Expansion** — New problem domains, same intelligent approach. Cost optimization, threat remediation, and non-Kubernetes workflows ([#739](https://github.com/jordigilh/kubernaut/issues/739)).

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
