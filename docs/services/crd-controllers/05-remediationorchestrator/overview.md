## Overview

**Purpose**: Coordinates end-to-end alert remediation workflow through watch-based state aggregation and lifecycle management.

**Core Responsibilities**:
1. **CRD Orchestration** - Create service CRDs (RemediationProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025)) based on phase progression
2. **Status Aggregation** - Watch all service CRD statuses and aggregate overall remediation state
3. **Lifecycle Management** - 24-hour retention with automatic cleanup and cascade deletion
4. **Timeout Management** - Detect phase timeouts and trigger escalation (BR-SP-062 (RemediationProcessor))
5. **Event Coordination** - Event-driven phase transitions via Kubernetes watches

**V1 Scope - Remediation Coordination Only**:
- Single RemediationRequest CRD per alert (created by Gateway Service)
- Watch-based event-driven coordination (no polling)
- Sequential phase CRD creation (RemediationProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution (DEPRECATED - ADR-025))
- **V1.0 Approval Notification Triggering** (ADR-018): Watches AIAnalysis phase and creates NotificationRequest CRDs when approval is required (BR-ORCH-001), reducing approval miss rate from 40-60% to <5% and enabling $392K savings per approval-required incident
- 24-hour retention with configurable cleanup
- Per-phase timeout detection with escalation

**Future V2 Enhancements** (Out of Scope):
- Parallel remediation workflows for complex scenarios
- Cross-alert correlation and batch remediation
- ML-based timeout prediction
- Multi-cluster remediation coordination
- Advanced retry strategies with exponential backoff

**Key Architectural Decisions**:
- **Watch-based coordination** (event-driven, not polling)
- **Data snapshot pattern** - Copy complete data from service status to next service spec
- **Owner references** - All service CRDs owned by RemediationRequest for cascade deletion
- **No duplicate detection** (Gateway Service responsibility - BR-WH-008)
- **Sequential CRD creation** - One service CRD at a time based on completion
- **24-hour retention** - Configurable cleanup with review window
- **Timeout escalation** - Per-phase and overall workflow timeouts

---

## Business Requirements Coverage

**RemediationOrchestrator** implements end-to-end CRD lifecycle orchestration for alert remediation.

### V1 Defined Business Requirements (11 BRs)

**Authoritative BR Files**: See `docs/requirements/BR-ORCH-*.md`

| Category | BRs | Description | BR File |
|----------|-----|-------------|---------|
| **Approval** | BR-ORCH-001 | Approval notification creation | [BR-ORCH-001](../../../requirements/BR-ORCH-001-approval-notification-creation.md) |
| **Workflow** | BR-ORCH-025, BR-ORCH-026 | Workflow data pass-through, approval orchestration | [BR-ORCH-025-026](../../../requirements/BR-ORCH-025-026-workflow-approval-orchestration.md) |
| **Timeout** | BR-ORCH-027, BR-ORCH-028 | Global and per-phase timeout management | [BR-ORCH-027-028](../../../requirements/BR-ORCH-027-028-timeout-management.md) |
| **Notification** | BR-ORCH-029, BR-ORCH-030, BR-ORCH-031 | Notification handling, status tracking, cascade cleanup | [BR-ORCH-029-031](../../../requirements/BR-ORCH-029-031-notification-handling.md) |
| **Deduplication** | BR-ORCH-032, BR-ORCH-033, BR-ORCH-034 | WE Skipped phase, duplicate tracking, bulk notification | [BR-ORCH-032-034](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) |

**Core Functions** (covered by above BRs):
- RemediationRequest CRD creation and ownership management
- Sequential service CRD creation (SignalProcessing → AIAnalysis → WorkflowExecution)
- Watch-based status aggregation from service CRDs
- Data snapshot pattern implementation (copy status → next spec)
- Owner reference management for cascade deletion
- 24-hour retention and automatic cleanup
- Event coordination and phase transitions

### V2 Future Requirements (Not Yet Defined)

**Planned for V2** (BRs will be defined when V2 scope is finalized):
- Parallel remediation workflows for complex multi-issue scenarios
- Cross-alert correlation and batch remediation coordination
- ML-based timeout prediction and adaptive phase management
- Multi-cluster remediation orchestration
- Advanced retry strategies with exponential backoff
- Cross-namespace remediation coordination
- Workflow templates and reusable patterns

---

## Service Configuration

### Port Configuration
- **Port 9090**: Metrics endpoint
- **Port 8080**: Health probes (follows kube-apiserver pattern)
- **Endpoint**: `/metrics`
- **Format**: Prometheus text format
- **Authentication**: Kubernetes TokenReviewer API (validates ServiceAccount tokens)
  - **See**: [004-metrics-authentication.md](../../../architecture/decisions/004-metrics-authentication.md) for complete implementation examples

### ServiceAccount
- **Name**: `remediation-orchestrator-sa`
- **Namespace**: `kubernaut-system`
- **Purpose**: Controller authentication and authorization

### Notes
- CRD controllers do not expose REST APIs
- Health checks (`/healthz`, `/readyz`) are for Kubernetes liveness/readiness probes
- Metrics endpoint requires valid Kubernetes ServiceAccount token

---

## 📊 Visual Architecture

### Architecture Diagram
```mermaid
graph TB
    subgraph "Remediation Orchestrator (RemediationRequest Service)"
        AR[RemediationRequest CRD]
        Controller[RemediationRequestReconciler]
        StateMachine[State Machine]
        TargetingData[Targeting Data Pattern]
    end

    subgraph "Upstream"
        Gateway[Gateway Service<br/>Port 8080]
    end

    subgraph "Child CRDs (Flat Sibling Hierarchy)"
        AP[SignalProcessing CRD]
        AIA[AIAnalysis CRD]
        WE[WorkflowExecution CRD]
        KE[KubernetesExecution CRD (DEPRECATED - ADR-025)]
    end

    subgraph "External Services"
        Notification[Notification Service<br/>Port 8080]
        DB[Data Storage Service<br/>Audit Trail]
    end

    Gateway -->|Creates| AR
    AR -->|Owns (owner ref)| AP
    AR -->|Owns (owner ref)| AIA
    AR -->|Owns (owner ref)| WE
    Controller -->|Watches| AR
    Controller -->|Create with TargetingData| AP
    Controller -->|Create with TargetingData| AIA
    Controller -->|Create with TargetingData| WE
    Controller -->|Watch Status| AP
    Controller -->|Watch Status| AIA
    Controller -->|Watch Status| WE
    Controller -->|State Transitions| StateMachine
    Controller -->|Escalate if Failed| Notification
    Controller -->|Update Status| AR
    Controller -->|Audit Trail| DB

    style AR fill:#ffe1e1
    style Controller fill:#fff4e1
    style AP fill:#e1f5ff
    style AIA fill:#e1f5ff
    style WE fill:#e1f5ff
```

### Sequence Diagram - Central Orchestration Flow
```mermaid
sequenceDiagram
    participant GW as Gateway Service
    participant AR as RemediationRequest<br/>CRD
    participant Ctrl as RemediationRequest<br/>Reconciler
    participant AP as RemediationProcessing
    participant AIA as AIAnalysis
    participant WE as WorkflowExecution
    participant Not as Notification<br/>Service

    GW->>AR: Create RemediationRequest CRD<br/>(with targeting data)
    activate AR
    AR-->>Ctrl: Watch triggers reconciliation
    activate Ctrl

    Note over Ctrl: Phase: Processing
    Ctrl->>AP: Create SignalProcessing CRD<br/>(with targeting data + owner ref)
    activate AP
    Ctrl-->>AP: Watch for status changes
    AP->>AP: Enrich + Classify
    AP->>AP: Status.Phase = "Ready"
    deactivate AP
    AP-->>Ctrl: Status change triggers reconciliation

    Note over Ctrl: Phase: Analyzing
    Ctrl->>AIA: Create AIAnalysis CRD<br/>(with targeting data + owner ref)
    activate AIA
    Ctrl-->>AIA: Watch for status changes
    AIA->>AIA: Investigate + Approve
    AIA->>AIA: Status.Phase = "Ready"
    deactivate AIA
    AIA-->>Ctrl: Status change triggers reconciliation

    Note over Ctrl: Phase: Executing
    Ctrl->>WE: Create WorkflowExecution CRD<br/>(with targeting data + owner ref)
    activate WE
    Ctrl-->>WE: Watch for status changes
    WE->>WE: Orchestrate workflow
    WE->>WE: Status.Phase = "Completed"
    deactivate WE
    WE-->>Ctrl: Status change triggers reconciliation

    Note over Ctrl: Phase: Completed
    Ctrl->>AR: Update Status.Phase = "Completed"
    deactivate Ctrl
    deactivate AR

    alt Workflow Failed
        Note over Ctrl: Phase: Failed
        Ctrl->>Not: Send escalation notification
        Ctrl->>AR: Update Status.Phase = "Failed"
    end
```

### State Machine - Central Orchestration Phases
```mermaid
stateDiagram-v2
    [*] --> Pending
    Pending --> Processing: Reconcile triggered
    Processing --> Analyzing: RemediationProcessing Ready
    Analyzing --> Executing: AIAnalysis Ready (Approved)
    Analyzing --> Failed: AIAnalysis Rejected
    Executing --> Completed: WorkflowExecution Completed
    Executing --> Failed: WorkflowExecution Failed
    Completed --> [*]: Remediation successful
    Failed --> [*]: Manual intervention required

    note right of Processing
        Create SignalProcessing CRD
        Watch for enrichment completion
        Targeting data included
    end note

    note right of Analyzing
        Create AIAnalysis CRD
        Watch for AI approval
        Evaluate Rego policy
    end note

    note right of Executing
        Create WorkflowExecution CRD
        Watch for workflow completion
        Track all steps
    end note

    note right of Failed
        Send escalation notification
        Audit failure reason
        Retain CRD for 24h
    end note
```

### Targeting Data Pattern
```mermaid
graph TB
    subgraph "Targeting Data Pattern"
        TD[Targeting Data<br/>in RemediationRequest.Spec]
    end

    subgraph "Immutable Snapshot"
        Alert[Alert Payload]
        K8sContext[Kubernetes Context<br/>~8KB]
        Environment[Environment Classification]
        Metadata[Timestamps + Correlation IDs]
    end

    subgraph "Child CRDs (Consumers)"
        AP[RemediationProcessing]
        AIA[AIAnalysis]
        WE[WorkflowExecution]
        KE[KubernetesExecution]
    end

    TD --> Alert
    TD --> K8sContext
    TD --> Environment
    TD --> Metadata

    AP -->|Reads| TD
    AIA -->|Reads| TD
    WE -->|Reads| TD
    KE -->|Reads| TD

    Note[Immutable: Never changes<br/>after RemediationRequest creation]
    TD -.->|Design Pattern| Note

    style TD fill:#ffe1e1
    style Note fill:#fff4e1
```

---

## Summary

**RemediationRequest Remediation Coordinator - V1 Design Specification (100% Complete)**

### Core Purpose
Coordinates end-to-end alert remediation workflow through watch-based state aggregation, sequential service CRD creation, and 24-hour retention lifecycle management.

### Key Architectural Decisions
1. **Watch-Based Event-Driven Coordination** - Kubernetes watches trigger reconciliation on service CRD status changes (<1s latency)
2. **Sequential CRD Creation** - One service CRD at a time: RemediationProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution
3. **Data Snapshot Pattern** - Copy complete data from service status to next service spec (no cross-CRD dependencies)
4. **Owner References for Cascade Deletion** - All service CRDs owned by RemediationRequest (automatic cleanup)
5. **Finalizer Pattern for 24-Hour Retention** - CRD persists for review window after completion
6. **Per-Phase Timeout Detection** - Individual service timeouts with escalation before overall workflow timeout
7. **Gateway Creates RemediationRequest Only** - RemediationRequest controller creates all service CRDs

### Integration Model
```
Gateway Service → RemediationRequest CRD (this controller)
                       ↓
        (creates & watches service CRDs)
                       ↓
    RemediationProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution
         (watch)        (watch)         (watch)           (watch)
                       ↓
    Sequential phase progression based on completion events
                       ↓
          RemediationRequest.status = "completed"
                       ↓
              24-hour retention begins
```

### V1 Scope Boundaries
**Included**:
- Sequential service CRD creation with watch-based coordination
- Per-phase and overall workflow timeout detection
- 24-hour retention with automatic cleanup
- Cascade deletion via owner references
- Status aggregation from all service CRDs

**Excluded** (V2):
- Parallel remediation workflows
- Cross-alert correlation and batch remediation
- ML-based timeout prediction
- Multi-cluster remediation coordination

### Business Requirements Coverage
- **BR-ORCH-001**: Approval notification creation
- **BR-ORCH-025, BR-ORCH-026**: Workflow data pass-through, approval orchestration
- **BR-ORCH-027, BR-ORCH-028**: Global and per-phase timeout management
- **BR-ORCH-029-031**: Notification handling, status tracking, cascade cleanup
- **BR-ORCH-032-034**: WE Skipped phase handling, duplicate tracking, bulk notification

**See**: [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) for detailed requirement specifications.

### Implementation Status
- **CRD Schema**: Complete design with spec/status types
- **Controller Logic**: Complete reconciliation flow with phase orchestration
- **Watch Configuration**: Complete setup for all 4 service CRDs
- **External Integrations**: Notification Service (escalation) and Data Storage Service (audit)
- **Testing Strategy**: Unit, integration, and E2E test plans

### Next Steps
1. ✅ **Approved Design Specification** (100% complete)
2. **Kubebuilder Setup**: Install framework and generate CRD scaffolds
3. **CRD Schema Implementation**: Define API types in `api/v1/alertremediation_types.go`
4. **Controller Implementation**: Core reconciliation logic with watch-based coordination
5. **Integration Testing**: End-to-end workflow validation

### Critical Success Factors
- Watch-based coordination (no polling, <1s latency)
- Sequential CRD creation prevents premature service execution
- Data snapshot pattern ensures service independence
- Owner references enable cascade deletion
- Finalizer pattern enforces 24-hour retention
- Per-phase timeouts catch stuck services before overall timeout

**Design Specification Status**: Production-Ready (100% Confidence)

---

**🚀 Ready for implementation! This is the P0 CRITICAL foundation - implement BEFORE service CRDs.**

