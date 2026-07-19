# Kubernaut Roadmap

Each milestone builds on the previous: **control** → **external integration** → **fleet scale** → **custom agents & domain expansion**. See [#818](https://github.com/jordigilh/kubernaut/issues/818) for the full vision.

<p align="center">
  <img src="kubernaut-roadmap.svg" alt="Kubernaut Roadmap" width="960"/>
</p>

---

## v1.4 — Operator Overrides and Platform Hardening (released)

- **Prompt injection guardrails** — Shadow agent with a dedicated scanning model to protect the agentic pipeline against prompt injection attacks ([#601](https://github.com/jordigilh/kubernaut/issues/601))
- **Operator workflow/parameter override** — Operators can override workflow selection and parameters during RAR approval, with authwebhook validation ([#594](https://github.com/jordigilh/kubernaut/issues/594))
- **PagerDuty and Microsoft Teams** — New notification delivery channels alongside Slack and console ([#60](https://github.com/jordigilh/kubernaut/issues/60), [#593](https://github.com/jordigilh/kubernaut/issues/593))
- **Unified monitoring config** — Single `monitoring:` Helm block replacing per-component Prometheus/AlertManager keys, with OCP auto-detection ([#463](https://github.com/jordigilh/kubernaut/issues/463))
- **NetworkPolicies** — Default-deny network policies for all services based on the traffic matrix ([#285](https://github.com/jordigilh/kubernaut/issues/285))

Track progress on the [v1.4 milestone](https://github.com/jordigilh/kubernaut/milestone/5).

---

## v1.5 — Live Investigation Control and Native Agent Integration (released)

*See, steer, and connect to your AI investigations in real time.* ([#822](https://github.com/jordigilh/kubernaut/issues/822))

- **Real-time investigation streaming** — Sub-second token-level reasoning updates streamed to the operator via SSE
- **Cancel and takeover** — Interrupt long-running investigations and take manual control, or hand control back to the autonomous agent
- **MCP Interactive Mode** — Investigate, enrich, and select workflows through any MCP-compatible interface — Claude, Cursor, Slack bots, or custom UIs ([#703](https://github.com/jordigilh/kubernaut/issues/703))
- **A2A Protocol** — External AI agents delegate remediation to Kubernaut and track task lifecycle via the [Agent-to-Agent](https://a2aproject.github.io/A2A/latest/specification/) standard ([kubernaut-apifrontend#15](https://github.com/jordigilh/kubernaut-apifrontend/issues/15))
- **Kubernaut Console** — Web-based operator dashboard with chat UI, live remediation streaming, and workflow selection ([shipped in v1.5.1](https://github.com/jordigilh/kubernaut/releases/tag/v1.5.1) — [kubernaut-console](https://github.com/jordigilh/kubernaut-console))
- **Natural language signal intake** — Trigger investigations by describing the problem in plain text; moved to the API Frontend as the external-facing entry point ([kubernaut-apifrontend#53](https://github.com/jordigilh/kubernaut-apifrontend/issues/53))

Track progress on the [v1.5 milestone](https://github.com/jordigilh/kubernaut/milestone/6) (closed).

---

## v1.6 — Fleet Operations (release candidate)

*From one cluster to your entire fleet.* ([#54](https://github.com/jordigilh/kubernaut/issues/54))

Single hub deployment manages remediations across multiple clusters using three purpose-built paths: K8s MCP for investigation, ACM for SA provisioning, and AAP for execution — with RHBK (Keycloak) JWT authentication.

- **Multi-cluster investigation** — KA investigates remote clusters via MCP with RHBK JWT authentication — cluster-agnostic RCA from a single hub ([#1510](https://github.com/jordigilh/kubernaut/issues/1510))
- **Fleet remediation** — Cluster-scoped workflow targeting via SP Rego classification ([#1511](https://github.com/jordigilh/kubernaut/issues/1511))
- **Centralized observability** — Cluster identity threaded through event payloads for console context and audit trails across the fleet ([#1409](https://github.com/jordigilh/kubernaut/issues/1409))
- **ACM Search integration** — Bearer-token authenticated ACM Search adapter for fleet-wide resource discovery ([#1556](https://github.com/jordigilh/kubernaut/issues/1556))

Track progress on the [v1.6 milestone](https://github.com/jordigilh/kubernaut/milestone/7) — `v1.6.0-rc1` tagged, GA pending remaining follow-up work.

---

## v1.7 — Custom Investigation Agents

*Bring your own agent, safely.*

- **Custom investigation agents** — Operators inject SOPs into the investigation pipeline via customer-authored agents packaged as opaque OCI images, executed via the image's own entrypoint. KA acts as a supervised harness — no Kubernaut-known runtime, no LLM calls of its own. Defined by the `AgenticWorkflow` CRD ([#1536](https://github.com/jordigilh/kubernaut/issues/1536))
- **AuthBridge / OpenShell integration** — AuthBridge intercepts every outbound LLM/MCP call from an opaque agent for credential injection and audit relay, extensible to a shadow-evaluator tee for security review; OpenShell provides sandbox isolation as a coexisting sidecar in the same pod ([#1535](https://github.com/jordigilh/kubernaut/issues/1535), [#1681](https://github.com/jordigilh/kubernaut/issues/1681))
- **kube-mcp-server by default** — Standardized Kubernetes access layer for agents, deprecating direct K8s Go bindings ([#1516](https://github.com/jordigilh/kubernaut/issues/1516))

Track progress on the [v1.7 milestone](https://github.com/jordigilh/kubernaut/milestone/8).

---

## Collective Intelligence (unscheduled)

*Multiple AI perspectives, one root cause.*

- **Multi-agent consensus RCA** — Ensemble investigation with independent LLM agents from different model families; a consolidator validates agreement and cross-examines on divergence ([#648](https://github.com/jordigilh/kubernaut/issues/648))
- **Remediation history analysis** — LLM-driven review of past RCA and remediation chains to improve future investigation accuracy ([#842](https://github.com/jordigilh/kubernaut/issues/842))

---

## Operational Expansion (unscheduled)

*New domains, same intelligent approach.*

- **Cost optimization** — LLM-driven FinOps investigation and resource remediation using signals from Red Hat Cost Management (Koku), Kubecost, OpenCost, and VPA ([#555](https://github.com/jordigilh/kubernaut/issues/555))
- **Threat remediation** — LLM-driven investigation and response for security and compliance signals from Red Hat Advanced Cluster Security (RHACS), Falco, Trivy, and OPA ([#554](https://github.com/jordigilh/kubernaut/issues/554))
- **Non-Kubernetes workflows** — `targetSystem` field enables execution against external systems (VMs, cloud APIs, IaC) with EA evolution for unverifiable outcomes ([#739](https://github.com/jordigilh/kubernaut/issues/739))
- **ITSM integration** — Jira/ServiceNow webhook adapter for the Notification service ([#53](https://github.com/jordigilh/kubernaut/issues/53))
