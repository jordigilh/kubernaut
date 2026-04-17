# PROPOSAL-EXT-001: External Integration Strategy

**Status**: PROPOSAL (under review)
**Date**: April 15, 2026
**Author**: Kubernaut Architecture Team
**Confidence**: 95% (post design gate mitigation)
**Related**: [#703](https://github.com/jordigilh/kubernaut/issues/703) (MCP Interactive Mode), [#705](https://github.com/jordigilh/kubernaut/issues/705) (A2A Protocol), [#708](https://github.com/jordigilh/kubernaut/issues/708) (API Frontend Service), [#711](https://github.com/jordigilh/kubernaut/issues/711) (Investigation Prompt Bundles), [#713](https://github.com/jordigilh/kubernaut/issues/713) (Kubernaut Console), [#714](https://github.com/jordigilh/kubernaut/issues/714) (NL Signal Intake), [#592](https://github.com/jordigilh/kubernaut/issues/592) (Conversational Mode), [DD-INTERACTIVE-001](../decisions/DD-INTERACTIVE-001-interactive-mode-crd-placement-and-timeouts.md)

---

## Purpose

This proposal defines Kubernaut's external integration strategy: **who** interacts with Kubernaut, **through what protocols**, and **where the boundaries are** between Kubernaut and integrating platforms. It is the umbrella document for three enhancement proposals (#703, #705, #592) that share persona definitions, infrastructure, and architectural decisions.

This is a proposal, not a committed design. Once accepted, persona definitions graduate into a formal ADR, and business requirements are extracted per-persona per-capability.

---

## Table of Contents

1. [Persona Catalog and Interaction Boundaries](#1-persona-catalog-and-interaction-boundaries)
2. [Protocol Architecture](#2-protocol-architecture)
3. [API Frontend Service](#3-api-frontend-service)
4. [CRD and Controller Changes](#4-crd-and-controller-changes)
5. [Relationship to #592 (Conversational Mode)](#5-relationship-to-592-conversational-mode)
6. [Open Design Questions](#6-open-design-questions)
7. [Work Estimation](#7-work-estimation)
8. [Evolution Path](#8-evolution-path)

---

## 1. Persona Catalog and Interaction Boundaries

This section defines who interacts with Kubernaut, through what protocol, and what Kubernaut does NOT do. Every subsequent section flows from these definitions.

### 1.1 Existing v1.0 Personas

These personas are already served by Kubernaut today. They are referenced implicitly across BRs, ADRs, and service documentation but have never been formally cataloged.

| Persona | Description | Evidence |
|---------|-------------|----------|
| **Cluster/platform operator** | Installs and configures Kubernaut. Tunes timeouts, routing, Helm values, and RBAC. Manages the deployment lifecycle. | BR-ORCH-043, BR-ORCH-044, BR-ORCH-046, service READMEs |
| **SRE/on-call responder** | Receives alerts, reviews remediation outcomes, uses kubectl to inspect state. Primary consumer of notification channels. | BR-ORCH-044, BR-GATEWAY-058/093/184, README.md |
| **Remediation approver** | Reviews RemediationApprovalRequests, approves or rejects proposed workflows. May be the same person as the SRE or a separate role. | ADR-040, BR-ORCH-001/025-028, BR-AI-059/060, `RAR.status.decidedBy` |
| **Workflow author** | Publishes and maintains workflow content in the catalog. Defines execution bundles, parameters, and action types. | BR-WORKFLOW-001/003/004, WorkflowExecution service docs |
| **Security/compliance auditor** | Reviews audit trails for SOC2 compliance. Needs attribution (who did what, when) and full event reconstruction. | BR-AUDIT-006, `11_SECURITY_ACCESS_CONTROL.md`, authwebhook attribution model |
| **Notification recipient** | Receives notifications about remediation events. May be outside K8s RBAC (vendors, contractors with Slack/PagerDuty access but no cluster credentials). | ADR-014 |

**RBAC archetypes** (BR-RBAC-011): The security requirements define four product-level role buckets -- **viewer**, **operator**, **developer**, **admin** -- that map to ClusterRole templates. These are authorization tiers, not separate personas.

### 1.2 New External Integration Personas

This proposal introduces three new persona categories for external integration:

| Persona | Protocol | Direction | Entry Point | Use Cases |
|---------|----------|-----------|-------------|-----------|
| **Human operator via MCP** | MCP (Streamable HTTP) | Interactive | API Frontend -> KA | Steer investigations, review findings interactively, choose workflows explicitly, watch live remediation status |
| **External AI agent (inbound)** | A2A | Inbound | API Frontend -> GW | Submit alerts/signals, track remediation lifecycle, respond to approval requests, receive structured artifacts (RCA, effectiveness score) |
| **ITSM/approval/comms agent (outbound)** | A2A | Outbound | NT -> external | Receive incident creation/update notifications, accept approval delegation requests, post team communications |

The **autonomous pipeline** persona (Alertmanager, K8s events) is existing and unchanged. It enters through the Gateway via webhooks.

### 1.3 Boundary Definition

What Kubernaut is:
- A **remediation specialist** -- investigate alerts, perform root cause analysis, select and execute remediation workflows, verify effectiveness
- A **policy enforcement engine** -- Rego-based approval gating, confidence thresholds, scope management
- An **audit authority** -- every decision, tool call, and state transition is recorded with attribution

What Kubernaut is NOT:
- NOT an **orchestration platform** -- no multi-agent routing, no UI rendering, no conversation UX management
- NOT a **monitoring platform** -- receives signals, does not generate them
- NOT an **identity provider** -- authenticates via the cluster's native mechanisms (TokenReview, SAR)

Ownership split with integrating platforms:

| Integrating Platform Owns | Kubernaut Owns |
|---------------------------|----------------|
| User experience (chat UI, dashboards) | Investigation depth (LLM + K8s tools) |
| Agent orchestration (multi-agent routing) | Workflow catalog and execution engines |
| Multi-tenant identity management | Policy enforcement (Rego, approval gates) |
| Conversation UX and history display | Audit trail (durable, SOC2-compliant) |
| Signal generation (monitoring, alerting) | Signal processing (enrichment, classification) |

### 1.4 Graduation Path

Once this proposal is accepted:
1. Personas become an ADR (e.g., ADR-060: Kubernaut Interaction Personas) that other BRs reference as context
2. BRs are extracted per-persona per-capability (e.g., BR-MCP-001: "As a human operator, I can steer an investigation via MCP")
3. DDs are created as implementation progresses (DD-INTERACTIVE-001 is the first)

---

## 2. Protocol Architecture

### 2.1 Protocol Selection

| Protocol | Audience | Interaction Model | Standard |
|----------|----------|-------------------|----------|
| **MCP** (Model Context Protocol) | Human operators via LLM clients (IDE copilots, Slack bots, operational consoles) | Tool-based, step-by-step operator control | [modelcontextprotocol.io](https://modelcontextprotocol.io) |
| **A2A** (Agent-to-Agent) | External AI agents (SRE platforms, ITSM, incident commanders) | Task-based, delegate and track | [a2aproject.github.io](https://a2aproject.github.io/A2A/latest/specification) |

These protocols are complementary, not competing:
- MCP gives agents **hands** -- connecting them to tools, APIs, and data sources (vertical integration)
- A2A gives agents **colleagues** -- letting autonomous agents delegate tasks to each other as peers (horizontal integration)

### 2.2 MCP Tools

Four MCP tools exposed through the API Frontend:

| Tool | Scope | Description |
|------|-------|-------------|
| `kubernaut_investigate` | RR | Interactive, streaming investigation chat with the LLM. User drives the RCA conversation. |
| `kubernaut_enrich` | RR | Run enrichment pipeline on current RCA target, return natural language summary. Informative -- invalidated if investigation restarts. |
| `kubernaut_select_workflow` | RR | Trigger workflow selection, present candidates, accept user choice. Auto-triggers enrichment if missing. |
| `kubernaut_watch` | Namespace/resource | Passive monitoring -- stream live phase transitions for matching RRs without intervening. Enables "join mid-flight" UX. |

Supporting Gateway MCP tools (internal, consumed by KA and API Frontend, not exposed externally):

| Tool | Description |
|------|-------------|
| `submit_signal` | Submit a structured signal to the Gateway for dedup and RR creation |
| `find_remediation` | Query active/completed remediations by resource, namespace, or label selector |

### 2.3 A2A Task Lifecycle

A2A tasks map to RR lifecycle phases:

| A2A Task State | RR Phase | Trigger |
|----------------|----------|---------|
| `submitted` | Pending | External agent sends `SendMessage` |
| `working` | Processing | SP enrichment + classification running |
| `working` | Analyzing | AI investigation in progress |
| `input-required` | AwaitingApproval | Approval needed; agent must respond |
| `working` | Executing | Workflow execution running |
| `completed` / `failed` | Completed / Failed | Terminal state with artifacts |

### 2.4 Service Responsibilities

| Service | MCP/A2A Role | Changes |
|---------|-------------|---------|
| **API Frontend** (new) | MCP Server + A2A Server (inbound) | New service (`cmd/apifrontend/`) |
| **Gateway** | MCP Server (internal tools only) | Gains `submit_signal`, `find_remediation` MCP tools for internal use |
| **KA** | Investigation engine + signal extraction endpoint | Gains `/api/v1/signal/extract` for NL signal intake; interactive session support |
| **RO, SP, AA, WFE, EA** | CRD-driven pipeline | AA controller gains interactive mode early branch (DD-INTERACTIVE-001). Rest unchanged. |
| **NT** | A2A Client (outbound delivery channel) | Gains A2A delivery channel alongside Slack/PD/Teams |

### 2.5 Data Flow: MCP Interactive Path

```
Operator (MCP Client)
  --> API Frontend (MCP Server)
    --> KA (investigation, enrichment, workflow selection)
    --> GW (submit_signal, find_remediation -- via internal MCP tools)
    --> CRD Watch (RR, SP, AA, RAR, WFE, EA -- live status via SSE)
```

### 2.6 Data Flow: A2A Inbound Path

```
External Agent (A2A Client)
  --> API Frontend (A2A Server)
    --> KA /api/v1/signal/extract (if natural language)
    --> GW (structured signal submission, dedup, RR creation)
    --> CRD Watch (phase transitions --> A2A task status updates)
```

### 2.7 Data Flow: A2A Outbound Path

```
RO (creates NotificationRequest CRD)
  --> NT (reconciles NR)
    --> A2A Client (SendMessage to configured external agents)
```

---

## 3. API Frontend Service

### 3.1 Purpose

A dedicated microservice (`cmd/apifrontend/`) that serves as the unified external interface for both MCP and A2A protocols. This keeps the Gateway focused on signal ingestion/dedup and avoids polluting it with protocol complexity.

### 3.2 Key Design Decisions

- **Shared CRD watch layer** -- One informer-based watch implementation serves both MCP (SSE streaming to operators) and A2A (task status updates to agents). One watch, two protocol outputs.
- **Gateway stays clean** -- The Gateway gains only internal MCP tool exposure (`submit_signal`, `find_remediation`), consumed by KA and the API Frontend. External clients never hit the Gateway directly for MCP/A2A.
- **Session management for MCP, task lifecycle for A2A** -- Different state machines, but both backed by CRD state as the source of truth.
- **Natural language signal intake** -- When an A2A agent sends freeform text instead of a structured signal, the Frontend calls KA's `/api/v1/signal/extract` endpoint. KA extracts the structured signal (LLM + K8s inspection) and returns it. The Frontend then submits it to the Gateway through the normal path. KA is a pure extraction service in this path -- text in, structure out.

### 3.3 Service Architecture

The API Frontend is a hybrid service:
- **HTTP API** (MCP server, A2A endpoints, Agent Card) -- follows Gateway's Chi router pattern
- **CRD watching** (9 CRDs for live status) -- follows controller-runtime informer pattern

The Notification service (`cmd/notification/main.go`) is the closest template: HTTP endpoints + K8s API interaction.

### 3.4 Agent Card

The API Frontend serves an Agent Card at `/.well-known/agent.json` for A2A discovery:

```json
{
  "name": "Kubernaut",
  "description": "AIOps agent for Kubernetes remediation.",
  "url": "https://kubernaut-apifrontend.namespace.svc:8443",
  "version": "1.4.0",
  "capabilities": {
    "streaming": true,
    "pushNotifications": false
  },
  "skills": [
    {
      "id": "investigate-and-remediate",
      "name": "Kubernetes Alert Investigation and Remediation",
      "description": "Investigate a Kubernetes alert, perform root cause analysis, select and execute a remediation workflow, and verify effectiveness.",
      "tags": ["kubernetes", "rca", "remediation", "observability", "aiops"]
    }
  ],
  "authentication": {
    "schemes": ["bearer"],
    "credentials": "Kubernetes ServiceAccount token or API key"
  }
}
```

---

## 4. CRD and Controller Changes

### 4.1 Design Decision: DD-INTERACTIVE-001 (Approved)

Two design gates were resolved. Implementation is planned for v1.4:

**G1: CRD Placement (Option C -- Hybrid)**

- `AIAnalysisSpec.InteractiveMode bool` -- immutable intent, set by RO at creation time based on `kubernaut.ai/interactive-mode` annotation on the parent RR. Follows existing `self == oldSelf` CEL rule.
- `AIAnalysisStatus.InteractiveSession *InteractiveSessionInfo` -- mutable runtime state (session ID, user, start time, expiry). Only populated when `spec.interactiveMode` is true.
- `kubernaut.ai/interactive-mode` annotation on RR -- set by API Frontend when MCP session starts. Shared constant and helper to be defined in `pkg/shared/annotations/annotations.go`.

**G2: Timeout Policy (Option A -- Elevated Timeouts)**

- When `spec.interactiveMode` is true, the RO passes elevated timeouts:
  - `AIAnalysisSpec.TimeoutConfig.InvestigatingTimeout`: 30m (vs. 60s autonomous default)
  - `RemediationRequest.Status.TimeoutConfig.Analyzing`: 45m (vs. 10m autonomous default)
- Configurable via Helm: `kubernautAgent.interactive.investigatingTimeout`, `analyzingPhaseTimeout`, `sessionTTL`
- AA controller: early branch in `InvestigatingHandler.Handle` skips autonomous KA invocation when `spec.InteractiveMode` is true; requeues every 15s to check for session completion.

### 4.2 Implementation Status

All items below are **planned for v1.4**. None have been implemented yet.

| Change | File | Status |
|--------|------|--------|
| `InteractiveMode` spec field | `api/aianalysis/v1alpha1/aianalysis_types.go` | Planned |
| `InteractiveSessionInfo` type + status field | Same | Planned |
| Annotation constant + `IsInteractiveMode` helper | `pkg/shared/annotations/annotations.go` | Planned |
| RO propagation: annotation -> spec + elevated timeouts | `pkg/remediationorchestrator/creator/aianalysis.go` | Planned |
| RR-level Analyzing timeout override | `internal/controller/remediationorchestrator/reconciler.go` | Planned |
| AA controller early branch | `pkg/aianalysis/handlers/investigating.go` | Planned |
| Helm values | `charts/kubernaut/values.yaml` (`kubernautAgent.interactive`) | Planned |
| Deepcopy + CRD manifests regenerated | `make generate && make manifests` | Planned |

---

## 5. Relationship to #592 (Conversational Mode)

Three enhancements form a layered stack. Understanding the boundaries prevents scope creep and clarifies what infrastructure is shared.

| Enhancement | Scope | Entry Points | Target Version |
|-------------|-------|-------------|----------------|
| **#592** Conversational Mode | **RAR-scoped** -- ask about a pending approval | Slack bot thread replies, `kubectl kubernaut chat` | v1.4 |
| **#703** MCP Interactive Mode | **RR-scoped** -- drive the full investigation lifecycle | Any MCP-compatible client (IDE copilots, Slack bots, consoles) | v1.4 |
| **#705** A2A Protocol | **Signal-scoped** -- delegate remediation, track task lifecycle | External A2A agents | v1.4 |

### What #703 inherits from #592

| Artifact | Built by #592 | Extended by #703 |
|----------|---------------|------------------|
| SSE streaming | TLS + SSE with event IDs, reconnection, proxy-friendly headers | Reused as-is for MCP Streamable HTTP transport |
| Auth middleware | TokenReview + SAR (`can user UPDATE rar/{name}?`) | Extended to MCP-scoped RBAC verbs |
| Session management | In-memory sessions with TTL, shared-session model | Extended with RR-level locking and conversation history |
| Audit-seeded reconstruction | Fetch audit chain by `correlation_id`, map to LLM messages | Reused for "join mid-flight" and session recovery |
| LLM tool access | Read-only toolsets during conversation | Extended to full investigation toolset |

### What #705 shares with #703

| Artifact | Shared | Separate |
|----------|--------|----------|
| CRD watch layer | Yes -- one informer, two protocol outputs | A2A maps to task states; MCP maps to SSE events |
| API Frontend service | Yes -- same binary hosts both MCP and A2A endpoints | Different route handlers, different auth models |
| Session/conversation state | No -- A2A is stateless (task lifecycle via CRDs) | MCP has multi-turn interactive sessions |

---

## 6. Open Design Questions

### 6.1 Partially Resolved (have direction, need DD)

**Post-workflow-selection lifecycle (retry/re-select)**

Both #703 and #705 expose the limitation that the RR lifecycle is strictly forward. If WFE fails, the RR goes to Failed with no retry path. In MCP mode, the user wants to try a different workflow. In A2A mode, the external agent wants to respond to a failure with an alternative. This requires CRD lifecycle changes (AA re-activation, WFE retry, new RR phases) and needs its own design document.

**MCP endpoint security**

Enhancement #592 defines a detailed auth model (TokenReview + SAR) for conversational endpoints that #703 inherits. MCP-specific aspects still need a DD: namespace scoping (limit sessions to namespaces the operator has access to), RBAC verb granularity (dedicated `investigate` verb on `remediationrequests`?), and MCP client authentication (K8s bearer, OAuth2, API key).

### 6.2 Unresolved (need design work)

**A2A signal schema versioning**

How do we version the A2A remediation request schema? Options: embed version in the A2A `DataPart`, use A2A extensions, or version the entire endpoint path. Open question #1 in #705.

**Multi-tenant namespace enforcement**

A2A agents must be scoped to authorized namespaces. High-priority gap identified in #705 design refinements. Needs to align with the existing scope management system (`kubernaut.ai/managed` label, `pkg/shared/scope/`).

**Session continuity across KA restarts**

Enhancement #592 acknowledges that in-memory sessions are lost on pod restart. The session is auto-recreated from the durable audit chain, but conversation turns added during the interactive session are lost. Full session persistence in DataStorage is deferred to v1.5. This is a known v1.4 limitation that carries forward to #703.

---

## 7. Work Estimation

The following estimates are preliminary, derived from a codebase-level preflight analysis. They are subject to revision during sprint planning.

### 7.1 Work Streams

**Tier 0 -- Foundations (must land first): 4.5 weeks**

| Item | What Exists | What's Needed | Estimate |
|------|-------------|---------------|----------|
| MCP SDK evaluation + selection | Nothing | Evaluate `modelcontextprotocol/go-sdk` for Streamable HTTP server support. Decision doc. | 0.5w |
| LLM streaming interface | `llm.Client.Chat` (sync only) | Add `StreamChat` to `llm.Client`. Implement in LangChainGo + Anthropic adapters + instrumented wrapper. | 2w |
| SSE transport helper | Zero SSE code | Lightweight SSE writer for Chi. Reusable across KA and API Frontend. | 0.5w |
| AIAnalysis CRD changes + DD | `InvestigationSession` for async polling | `InteractiveMode` in spec, `InteractiveSessionInfo` in status. DD-INTERACTIVE-001. | 1.5w |

**Tier 1 -- MCP Interactive Mode (depends on Tier 0): 13.5 weeks**

| Item | Estimate | Notes |
|------|----------|-------|
| Interactive session manager | 2w | RR-level locking, TTL, conversation history, user tracking |
| MCP server in KA | 1.5w | Stub + config + Helm exist; SDK wiring |
| `kubernaut_investigate` tool | 3w | Dual-path refactor of investigator core (JSONMode, ephemeral messages) |
| `kubernaut_enrich` tool | 1w | Expose existing enrichment pipeline as MCP tool |
| `kubernaut_select_workflow` tool | 1.5w | Catalog + structured response + auto-enrichment fallback |
| `kubernaut_watch` tool | 1.5w | CRD watching via controller-runtime informers |
| Audit enrichment | 1w | New `interactive.message`, `interactive.checkpoint` event types |
| AA controller update | 1.5w | State machine: idempotency gate, RO timeouts, new condition type |
| Config + security | 0.5w | Feature gate, MCP auth, session timeout config |

**Tier 2 -- A2A Protocol (depends on Tier 0, partially on Tier 1): 10.5 weeks**

| Item | Estimate | Notes |
|------|----------|-------|
| A2A protocol design | 1w | Agent Card schema, task lifecycle mapping, auth model. ADR. |
| API Frontend service | 3.5w | New `cmd/apifrontend/`. Chi router + CRD watchers for 9 CRDs. |
| A2A inbound handler | 2w | `tasks/send` endpoint, Agent Card serving, task state tracking |
| NL signal extraction | 1.5w | New KA endpoint: LLM extracts structured signal from freeform text |
| Gateway MCP tools | 1w | `submit_signal` and `find_remediation` as internal MCP tools |
| A2A status streaming | 1.5w | CRD watch -> A2A task status updates via SSE |

**Tier 3 -- Tests + Polish (throughout, bulk at end): 7.5 weeks**

| Item | Estimate |
|------|----------|
| Unit tests (Ginkgo/Gomega, all components) | 3w |
| Integration tests (MCP calls, A2A lifecycle, SSE) | 2w |
| Mock LLM interactive scenarios (new YAML format) | 1w |
| Helm chart + Dockerfile for API Frontend | 0.5w |
| Documentation (tool contracts, Agent Card, integration guide) | 1w |

### 7.2 Dependency Graph

```
Tier 0 (Foundations)
  0.1 MCP SDK eval
  0.2 LLM streaming interface
  0.3 SSE transport helper
  0.4 AIAnalysis CRD changes (DD-INTERACTIVE-001)
       |
       v
  +----+----+
  |         |
  v         v
Tier 1    Tier 2
(MCP)     (A2A)
  |         |
  |  Tier 2 depends on:
  |    - 0.1, 0.3 (MCP/SSE foundations)
  |    - 1.2 (MCP server in KA)
  |    - 1.6 (CRD watching)
  |         |
  v         v
     Tier 3
  (Tests + Polish)
```

### 7.3 Totals

| Scope | 1 Developer | 2 Developers (parallel) |
|-------|-------------|------------------------|
| **MCP Interactive only** (T0 + T1 + tests) | 22 weeks | 13 weeks |
| **A2A only** (T0 + T2 + tests) | 19 weeks | 11 weeks |
| **Both MCP + A2A** (all tiers) | 36 weeks | 20 weeks |
| **MCP Lite** (foundations + investigate + watch + basic session) | 10 weeks | 6 weeks |

### 7.4 Phased Delivery Option

> **Note**: This phased breakdown is an *integration-first rollout option* for teams that need incremental capability sooner. It does not override the product roadmap (section 8), which targets MCP + A2A + API Frontend together in v1.4. The API Frontend hosting model is still TBD -- the original OCP deployment target is no longer available, and alternatives (Backstage plugin, standalone service, etc.) are under evaluation. Delivery estimates for the API Frontend will be refined once the hosting decision is made.

If an integrating team needs something sooner:

- **Phase 1 -- "MCP Lite" (6-10 weeks)**: Foundations + `kubernaut_investigate` with streaming + `kubernaut_watch` + basic session manager. Enough for a demo and initial integration testing.
- **Phase 2 -- "Full MCP" (+6-7 weeks)**: Remaining Tier 1 (enrich, select_workflow, audit, AA controller, config). Full interactive lifecycle.
- **Phase 3 -- "A2A + API Frontend" (TBD)**: API Frontend service, A2A inbound/outbound, natural language extraction. Estimate pending hosting model decision.

### 7.5 Design Gates

| Gate | Question | Status |
|------|----------|--------|
| **G1: CRD immutability** | Where does `InteractiveMode` live? | **Resolved** -- Option C (hybrid: spec + status). DD-INTERACTIVE-001. |
| **G2: RO timeout policy** | How long can interactive sessions run? | **Resolved** -- Option A (elevated timeouts: 30m/45m). DD-INTERACTIVE-001. |
| **G3: A2A spec stability** | Which A2A spec revision do we target? | **Mitigated** -- pin to a specific spec revision; treat drift as future iteration. |

### 7.6 Risks

1. **LLM streaming** (Tier 0.2) -- interface change touches every LLM adapter and the investigator's core loop. Riskiest foundation item.
2. **API Frontend** (Tier 2.2) -- largest single greenfield item. New service with CRD watchers for 9 CRDs.
3. **A2A spec drift** (G3) -- the A2A protocol is still maturing. Building on it carries spec-drift risk.
4. **Investigator dual-path** (Tier 1.3) -- `runLLMLoop` is tightly coupled to autonomous flow. JSONMode on every turn, ephemeral messages, and dual-path through the investigator are the highest-risk refactor.
5. **Calendar time** -- 20-week "2 devs parallel" estimate does not account for code review cycles, design doc approvals, or competing release stabilization work. Real calendar time is likely 22-26 weeks.

---

## 8. Evolution Path

| Version | Capability | Enhancement |
|---------|-----------|-------------|
| **v1.4** | MCP interactive mode + A2A inbound + API Frontend + Kubernaut Console + NL signal intake + Prompt Bundles | #703, #705, #708, #711, #713, #714 |
| **v1.5** | Multi-agent consensus RCA | #648 |
| **v1.6** | A2A-based multi-cluster federation -- per-cluster Gateway agents | #54 |
| **v1.6+** | A2A outbound on NT -- ITSM/approval/comms agent delegation | #705 Phase 2 |
| **v1.6+** | Post-WFE retry lifecycle -- re-select workflow on failure | Cross-cutting (#703 + #705) |

Within each version, the internal evolution follows this progression:

| Phase | Capability | Complexity |
|-------|-----------|------------|
| V1 (DD-INTERACTIVE-001) | Elevated static timeouts for interactive sessions | Minimal (reuse existing infrastructure) |
| V2 (future) | Heartbeat-based timeout extension | Medium (heartbeat patch loop in KA) |
| V3 (future) | Suspend/resume with session TTL | Higher (session lifecycle management) |

---

## Appendix A: What Does NOT Change

The following are explicitly out of scope for this proposal:

- **Internal CRD-driven pipeline** (RO, SP, AA, KA, WFE, EA) -- untouched except for AA controller interactive mode branch
- **Existing Gateway ingestion** (Alertmanager, K8s events webhooks) -- untouched
- **Existing NT delivery channels** (Slack, PagerDuty, Teams, Console, File) -- untouched
- **KA investigation engine** (autonomous mode) -- untouched
- **Rego policies and approval gates** -- apply to all signals regardless of source
- **Workflow catalog and execution engines** (Tekton, Job, Ansible) -- untouched

## Appendix B: Glossary

| Term | Definition |
|------|-----------|
| **MCP** | Model Context Protocol -- a protocol for connecting AI agents to tools, APIs, and data sources |
| **A2A** | Agent-to-Agent protocol -- an open standard for communication between independent AI agent systems |
| **API Frontend** | Proposed new Kubernaut microservice that unifies MCP and A2A protocol exposure |
| **RR** | RemediationRequest CRD -- the root object for a remediation lifecycle |
| **RAR** | RemediationApprovalRequest CRD -- tracks approval decisions |
| **AA** | AIAnalysis CRD and its controller |
| **KA** | Kubernaut Agent -- the LLM integration service |
| **GW** | Gateway service -- signal ingestion, dedup, RR creation |
| **NT** | Notification service -- delivers notifications via configured channels |
| **RO** | Remediation Orchestrator -- orchestrates the CRD pipeline |
| **SP** | SignalProcessing CRD and its controller |
| **WFE** | WorkflowExecution CRD and its controller |
| **EA** | EffectivenessAssessment CRD and its controller |
| **SSE** | Server-Sent Events -- HTTP-based unidirectional streaming |
