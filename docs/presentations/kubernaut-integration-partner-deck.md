---
marp: true
theme: default
paginate: true
title: "Kubernaut — Kubernetes Remediation as a Service"
author: Kubernaut Team
---

<!-- _class: lead -->

# Kubernaut

### Kubernetes Remediation as a Service

*How your platform + Kubernaut delivers enterprise-grade incident response*

> **SVG slides for Google Slides import:** `docs/presentations/svg/`

| # | Slide | SVG |
|---|---|---|
| 0 | Title | `00-title.svg` |
| 1 | Complementary strengths | `01-complementary-strengths.svg` |
| 2 | Domain depth (iceberg) | `03a-domain-depth.svg` |
| 3 | Chat UI mockup | `02-chat-ui-mockup.svg` |
| 4 | Sequence diagram | `03-sequence-diagram.svg` |
| 5 | Interactive mode | `04a-interactive-mode.svg` |
| 6 | Ownership split | `05-ownership-split.svg` |
| 7 | Protocols (MCP vs A2A) | `06-protocols.svg` |
| 8 | Natural language intake | `03b-natural-language.svg` |
| 9 | Architecture overview | `04-architecture.svg` |
| 10 | Joint demo flow | `07-demo-flow.svg` |
| 11 | Next steps | `08-next-steps.svg` |
| 12 | Closing | `09-closing.svg` |

---

## We solve the same problem from different angles

| | Your Platform | Kubernaut |
|---|---|---|
| **Strength** | Orchestration, UX, natural language interaction | Kubernetes remediation domain expertise |
| **Users see** | Your chat UI, your dashboards, your brand | Nothing — invisible infrastructure |
| **Builds** | The experience | The engine |
| **Owns** | User intent, presentation, policy | Investigation, execution, verification |

**Neither replaces the other. Together, the user gets something neither can deliver alone.**

---

## What Kubernetes remediation actually requires

Remediation is a deep domain. This is what Kubernaut has built over 3 major releases:

**Signal intake**
- Prometheus AlertManager, Kubernetes Events, natural language
- Fingerprint-based deduplication (50 identical alerts → 1 remediation)
- Resource scope validation (does this alert point to a real K8s resource?)

**Investigation**
- LLM-powered root cause analysis — not rules, actual diagnosis
- Native Go `client-go` bindings (pod inspection, logs, events, resource state)
- Prometheus metric queries, observability tool integration
- Remediation history context (has this happened before? what worked?)

---

## What Kubernetes remediation actually requires (cont.)

**Remediation execution**
- Searchable workflow catalog with semantic matching
- Three executors: Tekton Pipelines, Kubernetes Jobs, Ansible (AWX/AAP)
- Per-workflow ServiceAccount with least-privilege RBAC
- OPA/Rego policy gates (is this remediation safe to run?)
- Optional human approval gates (RemediationApprovalRequest CRD)

**Closing the loop**
- Health checks, alert resolution monitoring, spec-hash drift detection
- Effectiveness scoring fed back into future investigations
- Notifications (Slack, console, extensible)

**Governance**
- 9 CRDs modeling the full lifecycle
- Full audit trail: every LLM call, tool invocation, decision
- Short-lived tokens, inter-pod TLS, admission-time validation

---

## The integration idea

> Your platform is the face.
> Kubernaut is the brain and the hands for Kubernetes remediation.

Your users interact with **your** chat interface, **your** dashboards, **your** brand.

Behind the scenes, your agent delegates to Kubernaut via standard protocols (MCP, A2A).

Kubernaut does the heavy lifting and streams results back.

**Your user never knows Kubernaut exists. Your product gets the credit.**

---

## What your user sees

```
┌──────────────────────────────────────────────────────────┐
│  Your Platform                                            │
│                                                           │
│  [!] Alert: pods crash-looping in namespace payments      │
│                                                           │
│  User: "What's going on with payments?"                   │
│                                                           │
│  Agent: "Investigating the payments namespace..."         │
│  > Checked pod status: payments-api-7d4f -- 12 restarts   │
│  > Pulled logs: OOMKilled at 512Mi limit                  │
│  > Queried metrics: memory trending up since 14:32 deploy │
│                                                           │
│  Agent: "Root cause: memory leak in payments-api caused   │
│  by unbounded cache growth after the 14:32 deploy."      │
│                                                           │
│  User: "Can you fix it?"                                  │
│                                                           │
│  Agent: "I found 2 remediation options:"                  │
│  ┌──────────────────┐  ┌───────────────────────────┐     │
│  │ [R] Rollback      │  │ [M] Increase memory limit │     │
│  │ Deploy to v2.3.1 │  │ 512Mi → 1Gi               │     │
│  │ [Recommended]     │  │                           │     │
│  └──────────────────┘  └───────────────────────────┘     │
│                                                           │
│  User: clicks [Rollback]                                  │
│                                                           │
│  Agent: "Executing rollback..."                           │
│  > Pipeline started ------------------- OK                │
│  > Health check: pods healthy, 0 restarts                 │
│  > Alert resolved                                         │
│                                                           │
│  Agent: "Done. Payments is healthy."                      │
└──────────────────────────────────────────────────────────┘
```

---

## What happens behind the scenes

```
Your UI                Your Agent              Kubernaut
  │                         │                        │
  │ "what's going on        │                        │
  │  with payments?"        │                        │
  │────────────────────────▶│                        │
  │                         │  kubernaut_investigate  │
  │                         │  (MCP tool call)        │
  │                         │───────────────────────▶│
  │                         │                        │── inspect pods
  │                         │                        │── pull logs
  │                         │                        │── query prometheus
  │                         │    SSE stream: findings │
  │                         │◀───────────────────────│
  │  streamed to user       │                        │
  │◀────────────────────────│                        │
  │                         │                        │
  │ "Can you fix it?"       │                        │
  │────────────────────────▶│  kubernaut_enrich      │
  │                         │───────────────────────▶│── gather context
  │                         │  kubernaut_select_workflow   │
  │                         │───────────────────────▶│── query catalog
  │                         │    workflow options     │
  │                         │◀───────────────────────│
  │  rendered as cards      │                        │
  │◀────────────────────────│                        │
  │                         │                        │
  │  [clicks Rollback]      │                        │
  │────────────────────────▶│  kubernaut_select_workflow   │
  │                         │  (execute=true)        │
  │                         │───────────────────────▶│── run pipeline
  │                         │  kubernaut_watch (SSE)  │── verify fix
  │                         │◀───────────────────────│
  │  "Done. Healthy."       │                        │
  │◀────────────────────────│                        │
```

---

## Interactive mode — not a black box

Kubernaut's MCP interactive mode is what makes this feel native in your UI:

**Real-time streaming**
Investigation findings stream token-by-token via SSE.
Your agent renders them however fits your UX — chat bubbles, log panels, progress bars.

**Conversational steering**
Your user asks follow-ups ("what about the other pods?", "show me the logs").
Your agent passes them through. Kubernaut's LLM responds in context.

**Choice presentation**
Workflow options return as structured data (name, description, risk level, parameters).
Your UI renders them as cards, dropdowns, buttons — your design, your brand.

**Join mid-flight**
If Kubernaut's autonomous pipeline already started (from an alert), your user attaches
and sees the current state + live updates going forward. No restart needed.

---

## What each side owns

### Your platform controls

- **UX and branding** — the user interacts with your interface, your design language
- **User intent parsing** — understanding "what's going on" means "investigate"
- **Result formatting** — rendering findings, options, and progress your way
- **Authentication** — who your users are, what they can access
- **Multi-tenant routing** — which cluster, which namespace, which team
- **Policy overlay** — your platform can add approval rules on top of Kubernaut's

### Kubernaut handles (invisible to the user)

- **Kubernetes inspection** — native `client-go`, not shelling out to kubectl
- **LLM-powered RCA** — actual diagnosis, not pattern matching
- **Signal deduplication** — 50 identical alerts become 1 remediation
- **Workflow catalog + execution** — Tekton, K8s Jobs, Ansible
- **Safety controls** — OPA policies, RBAC, approval gates
- **Effectiveness verification** — did the fix actually work?
- **Audit trail** — every decision recorded for compliance

---

## Integration protocols

### MCP (Model Context Protocol) — for interactive collaboration

Your agent calls Kubernaut's MCP tools directly:

| Tool | Purpose |
|---|---|
| `kubernaut_investigate` | Start/continue an interactive RCA session with streaming |
| `kubernaut_enrich` | Gather live cluster context for a root cause |
| `kubernaut_select_workflow` | Browse catalog, select, and execute a remediation |
| `kubernaut_watch` | Attach to an ongoing remediation for live status |

Supporting internal tools (used by the API Frontend, not exposed to external clients):

| Tool | Purpose |
|---|---|
| `submit_signal` | Submit a structured signal to the Gateway for dedup and RR creation |
| `find_remediation` | Query existing remediations by namespace, resource, status |

Best for: **interactive, user-driven flows** where your agent steers the process.

---

## Integration protocols (cont.)

### A2A (Agent-to-Agent) — for autonomous delegation

Your orchestrator delegates a remediation task to Kubernaut's Agent Card:

1. Discover Kubernaut via A2A Agent Card (capabilities, endpoint, auth)
2. Send `tasks/send` with the signal (structured or natural language)
3. Receive streaming status updates at each pipeline phase
4. Get final result: verified fix or escalation with full diagnostic context

Best for: **autonomous, fire-and-forget flows** where your agent detects a problem
and hands off without user involvement.

### When to use which

| Scenario | Protocol |
|---|---|
| User asks "fix payments" in your chat | MCP (interactive) |
| Your monitoring agent detects anomaly at 3am | A2A (autonomous) |
| User wants to review an ongoing remediation | MCP (`kubernaut_watch`) |
| Batch delegation from your incident pipeline | A2A (task per incident) |

---

## Natural language — no schema required

Your agent doesn't need to understand Kubernaut's signal format.

Send natural language:
> *"pods in namespace payments are crash-looping after the last deploy"*

Kubernaut's LLM extracts:
```json
{
  "alert_type": "CrashLoopBackOff",
  "namespace": "payments",
  "resource": "deployment/payments-api",
  "context": "post-deploy regression"
}
```

This feeds into the normal pipeline — deduplication, investigation, remediation.

**Your agent speaks your users' language. Kubernaut translates to Kubernetes.**

---

## What your product gains

### Without building from scratch

- **Credible remediation story** — not "we run a kubectl command," but full closed-loop with RCA, safety gates, and verification
- **Enterprise readiness** — audit trail, OPA policies, RBAC, approval workflows
- **Depth** — 9 CRDs modeling the full incident lifecycle, 3 executor types, effectiveness scoring
- **Kubernetes-native** — CRDs, controllers, OLM operator. Runs as a citizen of the cluster, not bolted on

### While keeping full control

- Your UX, your brand, your user relationships
- Your orchestration logic, your policies, your auth
- Your roadmap — Kubernaut evolves independently; integration stays stable via MCP/A2A

---

## Architecture overview

```
┌─────────────────────────────────────────────────────────┐
│                   Your Platform                          │
│  ┌────────────┐  ┌──────────────┐  ┌────────────────┐  │
│  │  Chat UI   │  │  Your Agent  │  │  Dashboards    │  │
│  └─────┬──────┘  └──────┬───────┘  └───────┬────────┘  │
│        │                │                   │           │
└────────┼────────────────┼───────────────────┼───────────┘
         │          MCP / A2A                 │
         │                │                   │
┌────────┼────────────────┼───────────────────┼───────────┐
│        ▼                ▼                   ▼           │
│  ┌──────────────────────────────────────────────────┐   │
│  │              Kubernaut API Frontend               │   │
│  │         (MCP server + A2A endpoint)               │   │
│  └──────────────────────┬───────────────────────────┘   │
│                         │                               │
│  ┌──────────┐  ┌───────┴─────┐  ┌───────────────────┐  │
│  │ Gateway  │  │  Kubernaut  │  │   Remediation      │  │
│  │ (signal  │  │  Agent (KA) │  │   Orchestrator     │  │
│  │  dedup)  │  │  (LLM RCA)  │  │   (CRD lifecycle) │  │
│  └──────────┘  └─────────────┘  └───────────────────┘  │
│                                                         │
│  ┌──────────┐  ┌─────────────┐  ┌───────────────────┐  │
│  │ Workflow  │  │ Effectivens │  │  Notification     │  │
│  │ Execution │  │ Monitor     │  │  Service          │  │
│  └──────────┘  └─────────────┘  └───────────────────┘  │
│                                                         │
│                    Kubernaut (in-cluster)                │
└─────────────────────────────────────────────────────────┘
```

---

## What a joint demo could look like

**Scenario: CrashLoopBackOff after a bad deploy**

| Step | What the audience sees | Who does it |
|---|---|---|
| 1 | Alert appears in your platform's UI | Your platform |
| 2 | User asks "what's going on?" in chat | Your platform |
| 3 | Investigation streams in real-time: pod status, logs, metrics | Kubernaut (via MCP) |
| 4 | Root cause displayed: "OOM from cache leak after deploy" | Kubernaut → Your UI |
| 5 | Remediation options shown as cards | Kubernaut → Your UI |
| 6 | User clicks "Rollback" | Your platform |
| 7 | Execution progress streams live | Kubernaut (via MCP) |
| 8 | "Fixed. Pods healthy. Alert resolved." | Kubernaut → Your UI |

**Your platform drives the experience. Kubernaut drives the outcome.**

---

## Next steps

1. **Technical deep-dive** — walk through MCP tool contracts and A2A Agent Card schema
2. **Proof of concept** — your agent calls `kubernaut_investigate` against a test cluster
3. **UX alignment** — how your UI renders Kubernaut's streaming responses
4. **Security model** — auth between your agent and Kubernaut's API Frontend

### What we need from you
- Your agent's protocol support (MCP, A2A, or both)
- Your auth model (how your agent authenticates to backend services)
- A test scenario you want to demonstrate

### What we provide
- Kubernaut running in a dev cluster with the MCP/A2A frontend
- Sample alert scenarios (CrashLoopBackOff, OOM, resource quota, failed deploy)
- MCP tool documentation and A2A Agent Card

---

<!-- _class: lead -->

# Your platform + Kubernaut

*Your users get the best experience.*
*Your product gets enterprise-grade Kubernetes remediation.*
*Neither of us builds what the other already has.*
