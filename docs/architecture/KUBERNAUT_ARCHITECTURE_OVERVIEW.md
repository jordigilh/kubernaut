# Kubernaut Architecture Overview

**Document Version**: 4.0
**Date**: August 2026
**Status**: Current Production Architecture — 12 active services (full architectural-accuracy rewrite, Issue #1806)

## 📋 Version History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 4.0 | Aug 2026 | **Full architectural-accuracy rewrite** (Issue #1806): replaced all HolmesGPT/HAPI and Context API references with the current system (Kubernaut Agent async submit/poll, Rego approval gating, `RemediationApprovalRequest`); corrected the "8 services, 2 deferred" framing to the real 12 active production services; replaced the single "Action Executor" concept with WorkflowExecution's 3-engine dispatch (Tekton PipelineRun \| native `batchv1.Job` \| Ansible/AWX); added Gateway, Data Storage, Kubernaut Agent, apifrontend, Auth Webhook, and Fleet Metadata Cache to the service inventory; fixed the Effectiveness Monitor doc link to `07-effectivenessmonitor/`; removed dangling links to non-existent documents. | AI Assistant |
| 3.3 | Dec 1, 2025 | DD-016 & DD-017 Integration: Updated V1.0 service count from 11 to 8. Dynamic Toolset deferred to V2.0 (DD-016). Effectiveness Monitor deferred to V1.1 (DD-017). | AI Assistant |
| 3.2 | Nov 15, 2025 | Corrected service naming: "Workflow Engine" → "Remediation Execution Engine" (per ADR-035) | AI Assistant |
| 3.1 | Oct 31, 2025 | Updated End-to-End Traceability diagram: Executor → Tekton Pipelines (per ADR-023, ADR-025) | AI Assistant |
| 3.0 | Jan 2025 | V1 Implementation Focus (10 Services) | - |

---

## 🎯 **Executive Summary**

Kubernaut is an intelligent Kubernetes remediation platform built on a microservices architecture that separates **AI investigation** from **infrastructure execution** for maximum safety and reliability. As of August 2026, the production system comprises **12 active services** (see [Service Catalog](KUBERNAUT_SERVICE_CATALOG.md) for full detail). This is a currently-running system, not a forward-looking plan.

### **Current System Composition**
- **🎯 Core remediation pipeline** (5 services): Gateway, SignalProcessing, AIAnalysis, WorkflowExecution, RemediationOrchestrator
- **🧠 AI engine** (1 service): Kubernaut Agent (KA) — invoked asynchronously, never a single synchronous call
- **🔧 Support services** (3 services): Data Storage (unified audit sink), Notification, Effectiveness Monitor (Level 1)
- **🚪 External-facing & platform services** (3 services): apifrontend, Auth Webhook, Fleet Metadata Cache

### **Key Principles**
- **🔍 Investigation vs ⚡ Execution Separation**: Kubernaut Agent investigates and recommends; WorkflowExecution's execution engines apply infrastructure changes
- **📋 Single Responsibility**: Each service has one clear business purpose
- **🔄 Signal Tracking**: End-to-end traceability from signal to resolution via `RemediationRequest` and the unified audit table (ADR-034)
- **🛡️ Safety First**: Rego-gated approval workflow (`RemediationApprovalRequest`) before any infrastructure change that needs it

---

## 🏗️ **High-Level System Architecture**

### **Core System Flow**
```mermaid
flowchart LR
    SIGNAL[📊 Signal<br/><small>Alerts, K8s Events</small>] --> GATEWAY[🔗 Gateway]
    GATEWAY -->|creates RemediationRequest| ORCH[🎛️ Remediation<br/>Orchestrator]
    ORCH --> SP[🔍 Signal Processing<br/><small>+ Environment Classification</small>]
    SP --> AI[🤖 AI Analysis]
    AI <-.->|async submit/poll| KA[🧠 Kubernaut<br/>Agent]
    AI -->|approval gate<br/>Rego-mandated| RAR[(RemediationApprovalRequest)]
    RAR -.-> AI
    AI --> WF[🎯 Workflow<br/>Execution]
    WF --> ENGINES[⚙️ Tekton \| Job \| Ansible-AWX]
    ENGINES --> K8S[☸️ Kubernetes]

    classDef core fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef investigation fill:#e3f2fd,stroke:#0d47a1,stroke-width:3px
    classDef execution fill:#e8f5e8,stroke:#2e7d32,stroke-width:3px

    class GATEWAY,ORCH,SP,WF core
    class AI,KA investigation
    class ENGINES execution
```

> **Note**: apifrontend is a second, independent entry point for natural-language (A2A/MCP) queries. It creates `RemediationRequest` CRDs directly and calls Kubernaut Agent through its own client — it does not go through Gateway. See [DD-AF-004](decisions/DD-AF-004-investigation-tool-split.md).

### **Service Categories (12 Active Services)**

#### **🎯 Core Remediation Pipeline (5 services)**
- **Gateway** (`cmd/gateway`): Stateless HTTP, no `ctrl.Manager` — signal webhook ingestion, deduplication, creates `RemediationRequest` CRDs
- **Signal Processing** (`cmd/signalprocessing`): CRD controller — enrichment, environment/severity/priority classification, owner-chain traversal
- **AI Analysis** (`cmd/aianalysis`): CRD controller — root-cause analysis and workflow selection via **async** Kubernaut Agent submit/poll (`pkg/agentclient`); Rego policy gating (`pkg/aianalysis/rego/evaluator.go`)
- **Workflow Execution** (`cmd/workflowexecution`): CRD controller — dispatches to one of 3 execution engines by Strategy pattern (`pkg/workflowexecution/executor/`): Tekton `PipelineRun`, native `batchv1.Job`, or Ansible/AWX
- **Remediation Orchestrator** (`cmd/remediationorchestrator`): CRD controller, reconciliation hub — owns/watches 6 child CRD kinds: SignalProcessing, AIAnalysis, WorkflowExecution, RemediationApprovalRequest, NotificationRequest, EffectivenessAssessment

#### **🧠 AI Engine (1 service)**
- **Kubernaut Agent (KA)** (`cmd/kubernautagent`): Stateless HTTPS, native Go (**not** a Python/HolmesGPT SDK wrapper) — async session-based investigation API (submit, then poll); multi-provider LLM support; static built-in toolsets (Kubernetes, Prometheus, Alertmanager)

#### **🔧 Support Services (3 services)**
- **Data Storage** (`cmd/datastorage`): Stateless HTTP — persistence, vector search, and the **unified audit sink** for every other service ([ADR-034](decisions/ADR-034-unified-audit-table-design.md)); computes the final weighted effectiveness score on demand
- **Notification** (`cmd/notification`): CRD controller — multi-channel delivery driven by `NotificationRequest` CRDs (migrated from stateless HTTP in Oct 2025; the old stateless design is dead)
- **Effectiveness Monitor** (`cmd/effectivenessmonitor`): CRD controller, no business API port — Level 1 automated assessment via 4 deterministic scorers (health, alert, metrics, hash); zero AI/LLM dependency in V1.0

#### **🚪 External-Facing & Platform Services (3 services)**
- **apifrontend (AF)** (`cmd/apifrontend`): Stateless HTTP + an embedded mini CRD controller for its own `InvestigationSession` CRD — external-facing A2A/MCP natural-language gateway with its own triage LLM agent (Claude via Vertex AI); calls Kubernaut Agent independently for deep investigation
- **Auth Webhook** (`cmd/authwebhook`): K8s admission webhook + CRD controller — validates admission and reconciles the `RemediationWorkflow` catalog CRD
- **Fleet Metadata Cache (FMC)** (`cmd/fleetmetadatacache`): Stateless HTTP — multi-cluster "fleet" metadata caching for cross-cluster workflow targeting

### **Planned / Backlog (Not Currently Active)**

| Item | Status |
|------|--------|
| **Effectiveness Monitor Level 2** (AI-powered post-execution analysis) | ⏸️ V1.1-planned, **not implemented**. Requires 8+ weeks of Level 1 data ([DD-017 v2.6](decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)). If ever built, it would call Kubernaut Agent, not "HolmesGPT". |
| **Multi-Model Orchestration** | ⛔ Zero code ever written. V2.0 backlog concept only — never a peer of the active services above. |
| **Dynamic Toolset** (separate service discovery service) | ⛔ Code deleted. Historical record only ([DD-016](decisions/DD-016-dynamic-toolset-v2-deferral.md)). Kubernaut Agent's toolsets are static/built-in today. |
| **Intelligence / Security & Access Control / Enhanced Health Monitoring** | ⛔ Not started — V2.0 backlog concepts with no `cmd/` entrypoint |

**Confirmed dead / eliminated** (do not re-introduce as active): standalone **HolmesGPT API (HAPI)** Python service — fully replaced by Kubernaut Agent, a native Go rewrite ([DD-KA-019](decisions/DD-KA-019-go-rewrite-design/DD-KA-019-go-rewrite-design.md)); **Context API** — fully removed, Data Storage absorbed its role ([DD-CONTEXT-006](decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md)); **Kubernetes Executor** — designed, never implemented, replaced by WorkflowExecution's execution engines ([ADR-025](decisions/ADR-025-kubernetesexecutor-service-elimination.md)).

---

## 🔍 **Investigation vs Execution Separation**

### **Clear Responsibility Boundaries**

```mermaid
flowchart TB
    subgraph INVESTIGATION ["🔍 Investigation Zone (Safe)"]
        AI_ENGINE[AI Analysis<br/>• Root-cause analysis<br/>• Workflow selection<br/>• Rego approval gating]
        KA[Kubernaut Agent<br/>• Async investigation<br/>• Multi-provider LLM<br/>• NO execution]
    end

    subgraph EXECUTION ["⚡ Execution Zone (Controlled)"]
        WF_EXEC[Workflow Execution<br/>• Tekton \| Job \| Ansible-AWX<br/>• Strategy-pattern dispatch<br/>• One engine per spec.executionEngine]
    end

    subgraph COORDINATION ["🎯 Coordination Layer"]
        ORCH[Remediation Orchestrator<br/>• Owns 6 child CRD kinds<br/>• Approval gate coordination<br/>• End-to-end lifecycle]
    end

    INVESTIGATION --> COORDINATION
    COORDINATION --> EXECUTION
    EXECUTION -.->|Results, audit events| COORDINATION

    classDef investigation fill:#e3f2fd,stroke:#0d47a1,stroke-width:3px
    classDef execution fill:#e8f5e8,stroke:#2e7d32,stroke-width:3px
    classDef coordination fill:#fff3e0,stroke:#ef6c00,stroke-width:2px

    class AI_ENGINE,KA investigation
    class WF_EXEC execution
    class ORCH coordination
```

### **Safety Guarantees**
- ✅ **Investigation services (AI Analysis, Kubernaut Agent) cannot execute infrastructure changes**
- ✅ **Only WorkflowExecution's engines (Tekton/Job/Ansible-AWX) can modify Kubernetes resources**
- ✅ **Rego-gated approval required for confidence bands or policies that mandate it** (`RemediationApprovalRequest`)
- ✅ **Complete audit trail for compliance** — every service writes to the unified Data Storage audit sink (ADR-034)

---

## 📊 **Signal Tracking Flow**

### **End-to-End Traceability**
```mermaid
sequenceDiagram
    participant SRC as Signal Source<br/>(Prometheus, K8s Events)
    participant G as Gateway
    participant ORCH as Remediation<br/>Orchestrator
    participant SP as Signal Processing
    participant AI as AI Analysis
    participant KA as Kubernaut Agent
    participant WF as Workflow Execution
    participant ENG as Execution Engine<br/>(Tekton\|Job\|Ansible-AWX)
    participant S as Data Storage

    SRC->>G: Signal webhook
    G->>ORCH: Creates RemediationRequest CRD
    ORCH->>SP: Creates SignalProcessing CRD
    SP->>SP: Enrichment + environment classification
    SP->>S: Audit event
    SP-.->ORCH: Status update
    ORCH->>AI: Creates AIAnalysis CRD
    AI->>KA: Submit investigation session (async)
    KA->>KA: Root cause analysis + recommendations
    AI->>KA: Poll for result
    KA->>AI: Investigation result

    alt Approval required (confidence band or Rego policy)
        AI->>ORCH: phase=Approving
        ORCH->>ORCH: Creates RemediationApprovalRequest
        Note over ORCH: Watched via field index;<br/>Approved unblocks next phase
    end

    AI->>S: Audit event
    AI-.->ORCH: Status update
    ORCH->>WF: Creates WorkflowExecution CRD
    WF->>ENG: Dispatch by spec.executionEngine
    Note over ENG: Exactly one engine per workflow:<br/>Tekton PipelineRun, batchv1.Job, or Ansible/AWX
    ENG->>S: Execution results (audit event)
    WF-.->ORCH: Status update

    Note over G,S: Complete audit trail; Data Storage is the single<br/>unified audit sink for the entire lifecycle (ADR-034)
```

### **Tracking Benefits**
- **🔍 Complete Visibility**: Track a signal from reception to resolution via `RemediationRequest` and correlated audit events
- **📋 Audit Compliance**: Full reconstruction of the remediation lifecycle from Data Storage's unified audit table (SOC2 CC8.1)
- **⚡ Performance Monitoring**: Measure end-to-end processing times per phase
- **🎯 Business Intelligence**: Learn from signal patterns and remediation effectiveness (Effectiveness Monitor Level 1)

---

## 🚀 **Current System Status**

All 12 services listed above are **active in production**. There is no separate "V1 in progress" phase to describe — the system already implements the investigation/execution separation, the async Kubernaut Agent integration, and Rego-gated approvals described in this document.

### **What Changed Since the Original V1 Design**
- **AI investigation**: Originally scoped around a standalone Python "HolmesGPT-API" service calling external LLM providers directly. That service never reached GA. **Kubernaut Agent (KA)**, a native Go rewrite, replaced it entirely ([DD-KA-019](decisions/DD-KA-019-go-rewrite-design/DD-KA-019-go-rewrite-design.md)), and AI Analysis calls it **asynchronously** (submit, then poll) rather than synchronously.
- **Historical context**: The originally-planned "Context API" was never completed (semantic search was stub-only) and was deprecated in favor of Data Storage, which now serves as the single unified audit/history source ([DD-CONTEXT-006](decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md)).
- **Execution**: The originally-planned standalone "Action Executor" / "Kubernetes Executor" service was fully designed but never implemented. WorkflowExecution's in-process execution engines (Tekton, native Job, Ansible/AWX) replaced it entirely ([ADR-025](decisions/ADR-025-kubernetesexecutor-service-elimination.md)).
- **New services not in the original design**: Kubernaut Agent, apifrontend, Auth Webhook, and Fleet Metadata Cache did not exist in the original V1 plan and have since been built and shipped.

### **Planned Enhancement: Effectiveness Monitor Level 2**
Effectiveness Monitor's Level 1 (deterministic, non-AI) scoring is active in V1.0. A planned Level 2 (AI-powered post-execution pattern analysis, calling Kubernaut Agent) remains unimplemented pending 8+ weeks of Level 1 data ([DD-017 v2.6](decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)).

---

## 📋 **Key Performance Targets**

### **Response Time Targets**
| Component | Target | Business Impact |
|-----------|--------|-----------------|
| **Gateway** | <50ms forwarding | 99.9% availability |
| **Signal Processing** | <5s end-to-end | User experience |
| **AI Analysis** | <10s investigation (excludes Kubernaut Agent async wait) | Decision quality |
| **Workflow Execution** | <30s dispatch to engine | MTTR improvement |

### **Scalability Targets**
| Metric | Target | Justification |
|--------|--------|---------------|
| **Concurrent Signals** | 1,000/minute | Peak load handling |
| **System Availability** | 99.9% uptime | Business continuity |
| **Signal Tracking** | 100% coverage | Audit compliance |
| **Execution Success** | >95% rate | Operational reliability |

---

## 🔗 **Related Documentation**

### **Detailed Architecture**
- **[Approved Microservices Architecture](APPROVED_MICROSERVICES_ARCHITECTURE.md)** - Full service decomposition, ports, and current-architecture Mermaid diagrams
- **[Service Catalog](KUBERNAUT_SERVICE_CATALOG.md)** - Individual service specifications
- **[Microservices Communication Architecture](MICROSERVICES_COMMUNICATION_ARCHITECTURE.md)** - Inter-service communication patterns
- **[Stateless Services Port Standard](STATELESS_SERVICES_PORT_STANDARD.md)** - Per-service port assignments

### **Business Context**
- **[Business Requirements Overview](../requirements/00_REQUIREMENTS_OVERVIEW.md)** - Business requirements index across all modules
- **Kubernaut Agent (KA) Integration** - Investigation service details (`internal/kubernautagent/`, `cmd/kubernautagent/`); service docs at [`docs/services/kubernaut-agent/`](../services/kubernaut-agent/)

### **Implementation Guides**
- **[Development Methodology](../../AGENTS.md)** - Pre-Implementation Workflow and TDD
- **[Quick Reference Card](../development/getting-started/QUICK_REFERENCE_CARD.md)** - Developer guidelines
- **[Developer Guide](../DEVELOPER_GUIDE.md)** - Setup, build, test, and deployment instructions

---

## 🎯 **Success Metrics**

### **Business Value Indicators**
- **⚡ Faster MTTR** through intelligent, async AI investigation
- **🛡️ 99.9% system availability** with fault isolation between investigation and execution
- **📋 100% audit compliance** with end-to-end tracking via the unified audit sink (ADR-034)
- **💰 Operational cost reduction** through automation

### **Technical Excellence**
- **🔍 High-quality AI analysis** for decision quality (see [AI Analysis service docs](../services/crd-controllers/02-aianalysis/) for current measured figures)
- **⚡ High workflow execution success rate** for reliability (see [Workflow Execution service docs](../services/crd-controllers/03-workflowexecution/) for current measured figures)
- **📊 Fast signal processing time** for user experience
- **🔄 Low workflow failure rate** for operational stability

---

*This overview provides a human-readable introduction to Kubernaut's architecture. For detailed specifications, see the related documentation links above.*
