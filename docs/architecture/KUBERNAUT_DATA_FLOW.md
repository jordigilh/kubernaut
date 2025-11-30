# Kubernaut Architecture: Complete Data Flow

**Version**: 1.0
**Date**: 2025-11-28
**Status**: ✅ Authoritative

---

## Overview

This document defines the **authoritative data flow** for the Kubernaut incident response platform. The Remediation Orchestrator (RO) is the central coordinator that creates all child CRDs and watches their status.

---

## Complete Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                            KUBERNAUT COMPLETE DATA FLOW                                      │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                             │
│  ┌──────────────┐                                                                           │
│  │   SIGNAL     │ Prometheus Alert, K8s Event, CloudWatch, etc.                             │
│  └──────┬───────┘                                                                           │
│         │                                                                                   │
│         ▼                                                                                   │
│  ┌──────────────────────────────────────────────────────────────────────────────────────┐   │
│  │  1. GATEWAY SERVICE (Stateless)                                                      │   │
│  │     • Receives webhooks from signal sources                                          │   │
│  │     • Normalizes payload to unified schema                                           │   │
│  │     • Performs deduplication, storm detection                                        │   │
│  │     • Creates RemediationRequest CRD                                                 │   │
│  └────────────────────────────────────┬─────────────────────────────────────────────────┘   │
│                                       │ Creates RemediationRequest                          │
│                                       ▼                                                     │
│  ╔══════════════════════════════════════════════════════════════════════════════════════╗   │
│  ║  2. REMEDIATION ORCHESTRATOR (CRD Controller) - CENTRAL COORDINATOR                  ║   │
│  ║     • Watches RemediationRequest CRD                                                 ║   │
│  ║     • Creates child CRDs in sequence (SignalProcessing → AIAnalysis → etc.)          ║   │
│  ║     • Tracks global timeout (DD-TIMEOUT-001)                                         ║   │
│  ║     • Manages approval workflow (ADR-040)                                            ║   │
│  ║     • Handles recovery on failure                                                    ║   │
│  ╚════════════════════════════════════╤═════════════════════════════════════════════════╝   │
│                                       │                                                     │
│                                       │ Creates SignalProcessing CRD                        │
│                                       ▼                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────────┐   │
│  │  3. SIGNAL PROCESSING (CRD Controller)                                               │   │
│  │     • K8s Enricher: Fetches related cluster objects                                  │   │
│  │     • Environment Classifier: Rego policy evaluation                                 │   │
│  │     • Priority Engine: Rego policy evaluation                                        │   │
│  │     • Business Classifier: Rego policy evaluation                                    │   │
│  │     • Updates SignalProcessing.status with enriched context                          │   │
│  │     • Writes audit trace to Data Storage                                             │   │
│  └────────────────────────────────────┬─────────────────────────────────────────────────┘   │
│                                       │ Status: Completed                                   │
│                                       │ RO watches, reads enriched context                  │
│                                       ▼                                                     │
│  ╔══════════════════════════════════════════════════════════════════════════════════════╗   │
│  ║  REMEDIATION ORCHESTRATOR                                                            ║   │
│  ║     • Watches SignalProcessing.status → Completed                                    ║   │
│  ║     • Creates AIAnalysis CRD with enriched context                                   ║   │
│  ╚════════════════════════════════════╤═════════════════════════════════════════════════╝   │
│                                       │ Creates AIAnalysis CRD                              │
│                                       ▼                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────────┐   │
│  │  4. AI ANALYSIS (CRD Controller)                                                     │   │
│  │     • Calls HolmesGPT-API with enriched signal context                               │   │
│  │     ┌─────────────────────────────────────────────────────────────────────────────┐  │   │
│  │     │  4a. HOLMESGPT-API (Stateless Service)                                      │  │   │
│  │     │      • Queries workflow catalog via Data Storage API (MCP search)           │  │   │
│  │     │      • Constructs LLM prompt with available workflows                       │  │   │
│  │     │      • Calls LLM provider (OpenAI, Anthropic, etc.)                         │  │   │
│  │     │      • Parses response per ADR-041 contract                                 │  │   │
│  │     │      • **RESOLVES workflow_id → containerImage** (from catalog)             │  │   │
│  │     └─────────────────────────────────────────────────────────────────────────────┘  │   │
│  │     • Receives SelectedWorkflow from HolmesGPT-API (incl. containerImage)            │   │
│  │     • Updates AIAnalysis.status.selectedWorkflow (containerImage included)           │   │
│  │     • Sets approvalRequired: true if confidence < 80%                                │   │
│  │     • Phase: Completed (NOT "Approving")                                             │   │
│  │     • Writes audit trace to Data Storage                                             │   │
│  └────────────────────────────────────┬─────────────────────────────────────────────────┘   │
│                                       │ Status: Completed, approvalRequired: true/false     │
│                                       ▼                                                     │
│  ╔══════════════════════════════════════════════════════════════════════════════════════╗   │
│  ║  REMEDIATION ORCHESTRATOR                                                            ║   │
│  ║     • Watches AIAnalysis.status → Completed                                          ║   │
│  ║     • IF approvalRequired == true:                                                   ║   │
│  ║       → Creates NotificationRequest CRD (approval notification)                      ║   │
│  ║       → Creates RemediationApprovalRequest CRD (ADR-040)                             ║   │
│  ║       → Watches RemediationApprovalRequest.status.decision                           ║   │
│  ║     • IF approvalRequired == false OR decision == "Approved":                        ║   │
│  ║       → **PASSES THROUGH** containerImage from AIAnalysis (no catalog lookup)        ║   │
│  ║       → Creates WorkflowExecution CRD with containerImage from AIAnalysis            ║   │
│  ╚════════════════════════════════════╤═════════════════════════════════════════════════╝   │
│                                       │                                                     │
│  ┌────────────────────────────────────┼─────────────────────────────────────────────────┐   │
│  │  APPROVAL FLOW (If Required)       │                                                 │   │
│  │                                    ▼                                                 │   │
│  │  ┌──────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  5a. NOTIFICATION CONTROLLER (CRD Controller)                                │   │   │
│  │  │      • Watches NotificationRequest CRD                                       │   │   │
│  │  │      • Routes via Alertmanager routing rules (DD-NOTIFICATION-001)           │   │   │
│  │  │      • Delivers to Slack, PagerDuty, Email, etc.                             │   │   │
│  │  │      • Operators receive approval request notification                       │   │   │
│  │  └──────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                     │   │
│  │  ┌──────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │  5b. REMEDIATION APPROVAL REQUEST CONTROLLER (CRD Controller)                │   │   │
│  │  │      • Watches RemediationApprovalRequest CRD                                │   │   │
│  │  │      • Manages approval timeout (default: 15m per DD-TIMEOUT-001)            │   │   │
│  │  │      • Updates status.decision on operator action                            │   │   │
│  │  │      • Decision: Approved / Rejected / Expired                               │   │   │
│  │  └──────────────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                                     │   │
│  │  Operator approves via Slack/Console/API → status.decision = "Approved"            │   │
│  └─────────────────────────────────────────────────────────────────────────────────────┘   │
│                                       │ Decision: Approved                                  │
│                                       ▼                                                     │
│  ╔══════════════════════════════════════════════════════════════════════════════════════╗   │
│  ║  REMEDIATION ORCHESTRATOR                                                            ║   │
│  ║     • Catalog lookup: GET /api/v1/workflows/{workflow_id}                            ║   │
│  ║     • Resolves workflow_id → container_image (OCI bundle)                            ║   │
│  ║     • Creates WorkflowExecution CRD with WorkflowRef + Parameters                    ║   │
│  ╚════════════════════════════════════╤═════════════════════════════════════════════════╝   │
│                                       │ Creates WorkflowExecution CRD                       │
│                                       ▼                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────────┐   │
│  │  6. WORKFLOW EXECUTION (CRD Controller)                                              │   │
│  │     • Receives WorkflowExecution with WorkflowRef (containerImage) + Parameters      │   │
│  │     • Creates Tekton PipelineRun from OCI bundle (ADR-044)                           │   │
│  │     ┌─────────────────────────────────────────────────────────────────────────────┐  │   │
│  │     │  6a. TEKTON (Runtime)                                                       │  │   │
│  │     │      • Pulls OCI bundle with Pipeline definition                            │  │   │
│  │     │      • Executes workflow steps as Tekton Tasks                              │  │   │
│  │     │      • Handles step orchestration, retries, timeouts                        │  │   │
│  │     │      • Reports PipelineRun status                                           │  │   │
│  │     └─────────────────────────────────────────────────────────────────────────────┘  │   │
│  │     • Watches PipelineRun status                                                     │   │
│  │     • Updates WorkflowExecution.status (Running, Completed, Failed)                  │   │
│  │     • Writes audit trace to Data Storage                                             │   │
│  └────────────────────────────────────┬─────────────────────────────────────────────────┘   │
│                                       │ Status: Completed / Failed                          │
│                                       ▼                                                     │
│  ╔══════════════════════════════════════════════════════════════════════════════════════╗   │
│  ║  REMEDIATION ORCHESTRATOR                                                            ║   │
│  ║     • Watches WorkflowExecution.status → Completed / Failed                          ║   │
│  ║     • IF Completed:                                                                  ║   │
│  ║       → Creates NotificationRequest CRD (success notification)                       ║   │
│  ║       → Updates RemediationRequest.status.overallPhase = "completed"                 ║   │
│  ║     • IF Failed:                                                                     ║   │
│  ║       → Evaluates recovery viability                                                 ║   │
│  ║       → Creates recovery AIAnalysis OR escalates to manual review                    ║   │
│  ╚════════════════════════════════════╤═════════════════════════════════════════════════╝   │
│                                       │ Creates NotificationRequest CRD                     │
│                                       ▼                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────────────┐   │
│  │  7. NOTIFICATION CONTROLLER (CRD Controller)                                         │   │
│  │     • Watches NotificationRequest CRD                                                │   │
│  │     • Routes via Alertmanager routing rules                                          │   │
│  │     • Delivers remediation result notification                                       │   │
│  │     • Updates NotificationRequest.status = Delivered                                 │   │
│  └────────────────────────────────────┬─────────────────────────────────────────────────┘   │
│                                       │                                                     │
│                                       ▼                                                     │
│                                    ┌──────┐                                                 │
│                                    │ END  │                                                 │
│                                    └──────┘                                                 │
│                                                                                             │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Sequence Summary

| Step | Service | Action | Creates CRD |
|------|---------|--------|-------------|
| 1 | Gateway | Receives signal, normalizes | RemediationRequest |
| 2 | RO | Watches RemediationRequest | SignalProcessing |
| 3 | Signal Processing | Enriches signal, classifies | - |
| 4 | RO | Watches SignalProcessing | AIAnalysis |
| 5 | AI Analysis | Calls HolmesGPT-API | - |
| 6 | RO | Watches AIAnalysis (if approval needed) | NotificationRequest + RemediationApprovalRequest |
| 7 | Notification | Delivers approval request | - |
| 8 | Approval Request | Manages approval lifecycle | - |
| 9 | RO | Watches approval decision, looks up catalog | WorkflowExecution |
| 10 | Workflow Execution | Creates Tekton PipelineRun | - |
| 11 | RO | Watches WorkflowExecution | NotificationRequest (result) |
| 12 | Notification | Delivers result notification | - |

---

## Service Responsibilities

| Service | Role | Key Interactions |
|---------|------|------------------|
| **Gateway** | Entry point, normalization | Creates RemediationRequest |
| **Remediation Orchestrator** | Central coordinator | Creates ALL child CRDs, watches status, orchestrates flow |
| **Signal Processing** | Signal enrichment, classification | Updates status with enriched context |
| **AI Analysis** | AI investigation, workflow selection | Calls HolmesGPT-API, populates selectedWorkflow |
| **HolmesGPT-API** | LLM integration | Queries catalog, calls LLM, parses response |
| **Notification** | Alert delivery | Routes via Alertmanager, delivers to channels |
| **Approval Request** | Approval lifecycle | Manages timeout, records decision |
| **Workflow Execution** | Workflow orchestration | Creates PipelineRun, watches status |
| **Data Storage** | Persistence | Workflow catalog, audit traces, semantic search |

---

## Key Design Decisions

| Decision | Document | Impact |
|----------|----------|--------|
| RO is central coordinator | This document | RO creates/watches ALL child CRDs |
| AIAnalysis completes, RO orchestrates approval | DD-CONTRACT-001 | Clean separation of concerns |
| Tekton handles step orchestration | ADR-044 | Simplified WorkflowExecution controller |
| Alertmanager routing for notifications | DD-NOTIFICATION-001 | Industry-standard routing semantics |
| Global + per-phase timeouts | DD-TIMEOUT-001 | Predictable timeout behavior |
| Workflow catalog in Data Storage | DD-WORKFLOW-009 | Centralized workflow management |
| Workflow schema in OCI bundles | ADR-043 | Single source of truth for workflow definitions |

---

## Related Documents

- **ADR-040**: RemediationApprovalRequest Architecture
- **ADR-041**: LLM Prompt and Response Contract
- **ADR-043**: Workflow Schema Definition Standard
- **ADR-044**: Workflow Execution Engine Delegation
- **DD-CONTRACT-001**: AIAnalysis ↔ WorkflowExecution Contract Alignment
- **DD-NOTIFICATION-001**: Alertmanager Routing Reuse
- **DD-TIMEOUT-001**: Global Remediation Timeout

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-28 | Initial authoritative data flow document |


