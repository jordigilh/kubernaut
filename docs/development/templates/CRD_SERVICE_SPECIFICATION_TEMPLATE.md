# [Service Name] Service - CRD Implementation Template

**Version**: 1.1
**Last Updated**: 2025-11-30
**Based On**: Remediation Processor, AI Analysis, and RemediationRequest specifications

---

## 📋 Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| 1.1 | 2025-11-30 | **Added**: Document classification (COMMON PATTERN vs SERVICE-SPECIFIC); Added `BR_MAPPING.md` to standard structure; Added service-specific file guidelines and examples | [DD-006](../../architecture/decisions/DD-006-controller-scaffolding-strategy.md) |
| 1.0 | 2025-10-15 | Initial template structure | - |

---

## 📋 TEMPLATE USAGE INSTRUCTIONS

**Purpose**: This template provides the standard structure for all CRD-based service specifications in Kubernaut V1.

### **⚠️ IMPORTANT: New Directory Structure (v1.0+)**

**As of 2025-11-30**, all service specifications use a **directory-per-service** structure instead of monolithic documents.

**Modern Approach** (Recommended ✅):
```
docs/services/crd-controllers/
└── 06-newservice/              (Create this directory)
    ├── README.md               (Service navigation hub)
    ├── overview.md             (Architecture & decisions)
    ├── crd-schema.md           (CRD type definitions)
    ├── controller-implementation.md
    ├── reconciliation-phases.md
    ├── finalizers-lifecycle.md
    ├── testing-strategy.md     (COMMON PATTERN)
    ├── security-configuration.md (COMMON PATTERN)
    ├── observability-logging.md (COMMON PATTERN)
    ├── metrics-slos.md         (COMMON PATTERN)
    ├── database-integration.md
    ├── integration-points.md
    ├── migration-current-state.md
    ├── implementation-checklist.md
    ├── BR_MAPPING.md           (COMMON PATTERN)
    └── [domain-specific].md    (SERVICE-SPECIFIC - see below)
```

### **Document Classification** (per DD-006)

| Classification | Description | When to Use |
|----------------|-------------|-------------|
| **COMMON PATTERN** | Standard files present in ALL CRD services. Structure is templated; content is service-specific. | Files listed above without SERVICE-SPECIFIC tag |
| **SERVICE-SPECIFIC** | Domain-specific documents unique to this service. Not all services will have these. | When service has unique domain requirements not covered by common patterns |

**SERVICE-SPECIFIC Examples**:
- **AIAnalysis**: `REGO_POLICY_EXAMPLES.md` (approval policies), `ai-holmesgpt-approval.md`
- **WorkflowExecution**: `tekton-pipeline-spec.md`, `workflow-parameters.md`
- **Notification**: `notification-channels.md`, `template-engine.md`
- **SignalProcessing**: `label-extraction.md`, `enrichment-pipeline.md`

**Guidelines for SERVICE-SPECIFIC files**:
1. Name files descriptively to indicate domain
2. Mark with `(SERVICE-SPECIFIC)` in README.md file organization section
3. Reference from appropriate common pattern files (e.g., link from `overview.md`)
4. Document in DD if pattern may apply to future services

**Legacy Approach** (Archived 📦):
```
docs/services/crd-controllers/
└── 06-new-service.md          (Single monolithic file - DO NOT USE)
```

### **How to Use This Template**

**Option A: Create Directory Structure** (Recommended ✅):
1. See [MAINTENANCE_GUIDE.md](./MAINTENANCE_GUIDE.md) → "Adding New Service" section
2. Copy structure from `01-signalprocessing/` as a reference
3. Use this template as content guidance for each document
4. Follow the 14-file structure for consistency

**Option B: Single Document** (Legacy, not recommended):
1. Copy this template to create a new service specification (e.g., `06-new-service.md`)
2. Replace all `[PLACEHOLDER]` text with service-specific information
3. Remove sections marked `[OPTIONAL]` if not applicable
4. **Note**: Consider migrating to directory structure after completion

**Examples to Follow**:
- **Directory Structure**: `01-signalprocessing/`, `02-aianalysis/`, `05-remediationorchestrator/`
- **Legacy Single File**: `archive/01-alert-processor.md` (archived)

**Section Guidelines**:
- **REQUIRED**: Must be completed for all services
- **CONDITIONAL**: Required only if service has this capability
- **OPTIONAL**: Include if it adds value to the specification

**See Also**:
- [RESTRUCTURE_COMPLETE.md](./RESTRUCTURE_COMPLETE.md) - Directory structure documentation
- [MAINTENANCE_GUIDE.md](./MAINTENANCE_GUIDE.md) - Maintenance procedures

---

## 🛠️ **CRD Controller Templates**

> **📋 Design Decision: DD-006**
>
> **Controller Scaffolding Strategy**: Custom Production Templates (Approved)
> **See**: [DD-006-controller-scaffolding-strategy.md](../../architecture/decisions/DD-006-controller-scaffolding-strategy.md)
>
> These templates are the approved scaffolding approach, chosen over Kubebuilder, Operator SDK, and manual creation.

**Rapid Development**: Use gap remediation templates to save 40-60% implementation time.

**Template Library**: [docs/templates/crd-controller-gap-remediation/](../../templates/crd-controller-gap-remediation/)

**Key Templates**:
1. **`cmd-main-template.go.template`**: Main entry point with configuration, health checks, and controller manager setup
2. **`config-template.go.template`**: Configuration package with YAML + environment variable overrides
3. **`config-test-template.go.template`**: Configuration unit tests
4. **`metrics-template.go.template`**: Prometheus metrics following DD-005 standards
5. **`dockerfile-template`**: Red Hat UBI9 multi-arch Dockerfile
6. **`makefile-targets-template`**: Standard Makefile targets for building and deployment
7. **`configmap-template.yaml`**: Kubernetes ConfigMap for controller configuration

**Usage Guide**: See [GAP_REMEDIATION_GUIDE.md](../../templates/crd-controller-gap-remediation/GAP_REMEDIATION_GUIDE.md) for step-by-step instructions.

**Standards Compliance**: All templates follow:
- [DD-005 Observability Standards](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) - Metrics and logging
- [LOGGING_STANDARD.md](../../architecture/LOGGING_STANDARD.md) - Zap structured logging
- [Controller-Runtime Best Practices](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md) - Architecture patterns

**Time Savings**: ~4-6 hours per controller implementation

---

## HEADER SECTION [REQUIRED]

**Service Type**: [CRD Controller / Central Coordinator]
**Health/Ready Port**: 8080 (`/health`, `/ready` - no auth required)
**Metrics Port**: 9090 (`/metrics` - with auth filter)
**CRD**: [CRD Name] (e.g., WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025))
**Controller**: [Controller Name]Reconciler
**Status**: [⚠️ NEEDS IMPLEMENTATION / ✅ IMPLEMENTED / 🚧 IN PROGRESS]
**Priority**: [P0 - CRITICAL / P1 - HIGH / P2 - MEDIUM]
**Effort**: [X weeks/days]
**Confidence**: [60-100]%

---

## 📚 Related Documentation [REQUIRED]

**CRD Design Specification**: [docs/design/CRD/XX_[CRD_NAME]_CRD.md](../../design/CRD/XX_[CRD_NAME]_CRD.md)

**Related Services**:
- **Creates & Watches**: [List of CRDs this service creates/watches]
- **Integrates With**: [List of services this service calls]

**Architecture References**:
- [Multi-CRD Reconciliation Architecture](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- [Service Connectivity Specification](../../architecture/SERVICE_CONNECTIVITY_SPECIFICATION.md)

---

## Business Requirements [REQUIRED]

**Primary Business Requirements**:
- **BR-[CATEGORY]-XXX**: [Brief description]
- **BR-[CATEGORY]-XXX**: [Brief description]
- **BR-[CATEGORY]-XXX**: [Brief description]

**Secondary Business Requirements**:
- **BR-[CATEGORY]-XXX**: [Brief description]
- **BR-[CATEGORY]-XXX**: [Brief description]

**Excluded Requirements** (Delegated to Other Services):
- **BR-[CATEGORY]-XXX**: [Requirement name] - [Owning Service] responsibility
- **BR-[CATEGORY]-XXX**: [Requirement name] - [Owning Service] responsibility

**Note**: [Service Name] receives [CRD Name] CRDs from [Upstream Service]. [Explain delegation pattern if applicable].

---

## Overview [REQUIRED]

**Purpose**: [1-2 sentence description of what this service does]

**Core Responsibilities**:
1. [Primary responsibility with brief description]
2. [Secondary responsibility with brief description]
3. [Tertiary responsibility with brief description]
4. [Additional responsibilities as needed]

**V1 Scope - [Service-Specific Focus]**:
- [Key V1 capability 1]
- [Key V1 capability 2]
- [Key V1 capability 3]
- [Size/performance constraints]
- [Data handling pattern]
- [Critical exclusions]

**Future V2 Enhancements** (Out of Scope):
- [Future enhancement 1]
- [Future enhancement 2]
- [Future enhancement 3]
- [Future enhancement 4]

**Key Architectural Decisions**:
- **[Decision Area 1]**: [Decision and rationale]
- **[Decision Area 2]**: [Decision and rationale]
- **[Decision Area 3]**: [Decision and rationale]
- **[Decision Area 4]**: [Decision and rationale]
- **[Decision Area 5]**: [Decision and rationale]

---

## Package Structure [REQUIRED]

**Approved Structure**: `cmd/{service_name}/` where `{service_name}` is a **single-word compound name**

**Naming Convention** (MANDATORY):
- Use single-word compound names (lowercase, no underscores, no dashes)
- Examples: `alertprocessor`, `workflowexecution`, `aianalysis`
- ❌ **NOT**: `alert-processor`, `alert_processor`, `workflow/execution`, `ai/analysis`
- ✅ **CORRECT**: `alertprocessor`, `workflowexecution`, `aianalysis`

Following Go idioms and codebase patterns:

```
cmd/{service_name}/             → Main application entry point (MANDATORY: single-word compound)
  └── main.go

pkg/[package_path]/             → Business logic (PUBLIC API, can be nested if needed)
  ├── service.go               → [Service]Service interface
  ├── implementation.go        → Service implementation
  ├── components.go            → Processing components
  └── types.go                 → Type-safe result types

internal/controller/            → CRD controller (INTERNAL)
  └── [crdname]_controller.go
```

**Examples**:
- Remediation Processor: `cmd/alertprocessor/` (NOT `cmd/alert/processor/`)
- Workflow Execution: `cmd/workflowexecution/` (NOT `cmd/workflow/execution/`)
- AI Analysis: `cmd/aianalysis/` (NOT `cmd/ai/analysis/`)

**Note**: Business logic packages (`pkg/`) MAY use nested paths for organization (e.g., `pkg/workflow/execution/`), but `cmd/` MUST use single-word compound names.

**Migration** [CONDITIONAL]: [If migrating from existing code, describe migration path]

---

## CRD API Package Naming Convention [CRITICAL]

**Golden Rule**: "`kubernaut` ONLY appears in Kubernetes API Group strings (e.g., `workflowexecution.kubernaut.io`), NEVER in Go package import paths."

### **Each CRD Gets Its Own Dedicated API Package**

| CRD | K8s API Group | Go Package Import | Used By |
|-----|---------------|-------------------|---------|
| **RemediationRequest** | `kubernaut.io` | `remediationv1 "...api/remediation/v1"` | Remediation Orchestrator (owns), All services (parent ref) |
| **RemediationProcessing** | `signalprocessing.kubernaut.io` | `processingv1 "...api/alertprocessor/v1"` | Remediation Processor (owns), AI Analysis (reads) |
| **AIAnalysis** | `aianalysis.kubernaut.io` | `aianalysisv1 "...api/aianalysis/v1"` | AI Analysis (owns), Workflow Execution (reads) |
| **WorkflowExecution** | `workflowexecution.kubernaut.io` | `workflowexecutionv1 "...api/workflowexecution/v1"` | Workflow Execution (owns) |
| **KubernetesExecution** (DEPRECATED - ADR-025) | `kubernetesexecution.kubernaut.io` | `kubernetesexecutionv1 "...api/kubernetesexecution/v1"` | Kubernetes Executor (owns), Workflow watches |

### **Import Pattern Per Service**

**Each controller only imports CRDs it directly manages or interacts with:**

```go
// Remediation Processor Controller
import (
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"      // Parent (owner reference)
    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"    // Own CRD
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"                         // Creates downstream
)

// AI Analysis Controller
import (
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"      // Parent
    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"    // Reads from
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"                         // Own CRD
    workflowv1 "github.com/jordigilh/kubernaut/api/workflow/v1"             // Creates downstream
)

// Workflow Execution Controller
import (
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"      // Parent
    workflowv1 "github.com/jordigilh/kubernaut/api/workflow/v1"             // Own CRD
    executorv1 "github.com/jordigilh/kubernaut/api/executor/v1"             // Creates downstream
)
```

### **Benefits of Dedicated Packages**

1. ✅ **Single Responsibility**: Each CRD package is self-contained
2. ✅ **Clean Dependencies**: Controllers only import what they manage
3. ✅ **Independent Versioning**: CRDs evolve independently
4. ✅ **Smaller Binaries**: No unused CRD types compiled in
5. ✅ **API Stability**: Changes isolated to owning package

### **Anti-Pattern to AVOID**

❌ **DO NOT** create a catch-all `api/v1` package with all CRDs:
```go
// WRONG - Catch-all package anti-pattern
kubernautv1 "github.com/jordigilh/kubernaut/api/v1"  // ❌ Contains all CRDs
```

✅ **DO** use dedicated packages per CRD:
```go
// CORRECT - Dedicated package per CRD
remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
```

---

## Development Methodology [REQUIRED]

**Mandatory Process**: Follow APDC-Enhanced TDD workflow per [.cursor/rules/00-core-development-methodology.mdc](../../../../.cursor/rules/00-core-development-methodology.mdc)

### APDC-TDD Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ ANALYSIS → PLAN → DO-RED → DO-GREEN → DO-REFACTOR → CHECK  │
└─────────────────────────────────────────────────────────────┘
```

**ANALYSIS** (5-15 min): Comprehensive context understanding
  - Search existing implementations (`codebase_search "[ComponentType] implementations"`)
  - Identify reusable components in `pkg/` and `internal/`
  - Map business requirements (BR-[CATEGORY]-XXX)
  - Identify integration points in `cmd/`

**PLAN** (10-20 min): Detailed implementation strategy
  - Define TDD phase breakdown (RED → GREEN → REFACTOR)
  - Plan integration points (controller in `cmd/{service_name}/`)
  - Establish success criteria (measurable business outcomes)
  - Identify risks and mitigation strategies

**DO-RED** (10-15 min): Write failing tests FIRST
  - Unit tests defining business contract (70%+ coverage target)
  - Use FAKE K8s client (`sigs.k8s.io/controller-runtime/pkg/client/fake`)
  - Mock ONLY external HTTP services (use `pkg/testutil/mocks`)
  - Use REAL business logic components
  - Map tests to business requirements (BR-[CATEGORY]-XXX)

**DO-GREEN** (15-20 min): Minimal implementation
  - Define interfaces to make tests compile
  - Minimal code to pass tests
  - **MANDATORY integration in cmd/{service_name}/** (controller startup)
  - Add owner references and finalizers

**DO-REFACTOR** (20-30 min): Enhance with sophisticated logic
  - **NO new types/interfaces/files** (enhance existing only)
  - Add sophisticated algorithms and business logic
  - Maintain integration throughout
  - Improve performance and error handling

**CHECK** (5-10 min): Validation and confidence assessment
  - Business requirement verification (all BR-[CATEGORY]-XXX addressed)
  - Integration confirmation (controller in `cmd/{service_name}/`)
  - Test coverage validation (70%+ unit, 20% integration, 10% E2E)
  - Confidence assessment (60-100%)

**AI Assistant Checkpoints**: See [.cursor/rules/10-ai-assistant-behavioral-constraints.mdc](../../../../.cursor/rules/10-ai-assistant-behavioral-constraints.mdc)
  - **Checkpoint A**: Type Reference Validation (read type definitions before referencing fields)
  - **Checkpoint B**: Test Creation Validation (search existing patterns before creating new)
  - **Checkpoint C**: Business Integration Validation (verify cmd/{service_name}/ integration)
  - **Checkpoint D**: Build Error Investigation (complete dependency analysis before fixing)

### Quick Decision Matrix

| Starting Point | Required Phase | Reference |
|----------------|---------------|-----------|
| **New feature from scratch** | Full APDC workflow | 00-core-development-methodology.mdc |
| **Enhancing existing code** | DO-RED → DO-REFACTOR | Skip ANALYSIS if code is well-understood |
| **Fixing bugs** | ANALYSIS → DO-RED → DO-REFACTOR | Understand root cause first |
| **Adding tests to existing code** | DO-RED only | Write tests for uncovered behavior |

**Testing Strategy Reference**: [.cursor/rules/03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)
  - Unit Tests (70%+): Use fake K8s client, mock external HTTP services only
  - Integration Tests (20%): Real K8s API (KIND cluster), cross-component validation
  - E2E Tests (10%): Complete workflow scenarios with live services

---

## Reconciliation Architecture [REQUIRED]

### Phase Transitions

**[If Single-Phase]**:
```
"" (new) → processing → completed
              ↓
         ([X seconds])
```

**[If Multi-Phase]**:
```
"" (new) → phase1 → phase2 → phase3 → completed
            ↓        ↓        ↓
         ([Xs])   ([Xs])   ([Xs])
```

**Rationale**: [Explain why single or multi-phase and timing considerations]

### Reconciliation Flow

#### 1. **[Phase 1 Name]** Phase (BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX)

**Purpose**: [What this phase accomplishes]

**Trigger**: [What initiates this phase]

**Actions** (executed [synchronously/asynchronously]):

**Step 1: [Action Name]** (BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX)
- [Action detail 1]
- [Action detail 2]
- [Action detail 3]
- [Timeout/error handling]

**Step 2: [Action Name]** (BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX)
- [Action detail 1]
- [Action detail 2]
- [Action detail 3]

**Step 3: [Action Name]** (BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX)
- [Action detail 1]
- [Action detail 2]

**Step 4: Status Update**
- Set `status.phase = "[next_phase]"`
- Set `status.[relevant_field]` with [data description]
- Record [timestamp/metrics]

**Transition Criteria**:
```go
if [condition1] && [condition2] {
    phase = "[next_phase]"
    // [Next action description]
} else if [degraded_condition] {
    // Degraded mode handling
    phase = "[next_phase]"
    status.degradedMode = true
} else if [critical_error] {
    phase = "failed"
    reason = "[error_type]"
}
```

**Timeout**: [X minutes/seconds] ([Rationale for timeout duration])

**Reconciliation Implementation**:
```go
func (r *[CRD]Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var [crdvar] [package]v1.[CRD]
    if err := r.Get(ctx, req.NamespacedName, &[crdvar]); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Skip if already in terminal state
    if [crdvar].Status.Phase == "completed" || [crdvar].Status.Phase == "failed" {
        return ctrl.Result{}, nil
    }

    // [Phase-specific reconciliation logic]
    // ... implementation details ...

    return ctrl.Result{}, nil
}
```

**Example CRD Update**:
```yaml
status:
  phase: [next_phase]
  [relevant_field]:
    [key]: [value]
    [key]: [value]
```

#### 2. **[Phase 2 Name]** Phase [CONDITIONAL - If Multi-Phase]

[Repeat Phase 1 structure for each additional phase]

#### N. **completed** Phase (Terminal State)

**Purpose**: [Final state responsibilities]

**Actions**:
- Record [completion data]
- Emit Kubernetes event: `[EventName]`
- [Database audit recording - if applicable]
- [Wait for next action]

**No Timeout** (terminal state)

**Note**: [Service Name] [DOES / DOES NOT] create [NextCRD] CRD. [Explain orchestration pattern].

#### N+1. **failed** Phase (Terminal State)

**Purpose**: Record failure for debugging

**Actions**:
- Log failure reason and context
- Emit Kubernetes event: `[ServiceName]Failed`
- Record failure to audit database
- [Escalation if applicable]

**No Requeue** (terminal state - requires manual intervention or alert retry)

---

## Current State & Migration Path [CONDITIONAL]

### Existing Business Logic (Verified)

**Current Location**: `pkg/[oldpackage]/` ([X lines of reusable code])
**Target Location**: `pkg/[newpackage]/` (after migration)

```
pkg/[oldpackage]/ → pkg/[newpackage]/
├── service.go ([X lines])          ✅ [Service] → [NewService] interface
├── implementation.go ([X lines])   ✅ [Description of reusability]
└── components.go ([X lines])       ✅ [Description of business logic]
```

**Existing Tests** (Verified - to be migrated):
- `test/unit/[oldpackage]/` → `test/unit/[newpackage]/` - Unit tests with Ginkgo/Gomega
- `test/integration/[oldpackage]/` → `test/integration/[newpackage]/` - Integration tests

### Business Logic Components (Highly Reusable)

[List and describe reusable components with percentages]

### Migration to CRD Controller

**[Synchronous/Asynchronous] Pipeline → [Synchronous/Asynchronous] Reconciliation**

[Code examples showing before/after migration]

### Component Reuse Mapping

| Existing Component | CRD Controller Usage | Reusability | Migration Effort | Notes |
|--------------------|---------------------|-------------|-----------------|-------|
| **[Component1]** | [Usage description] | [X%] | [Effort level] | [Notes] |
| **[Component2]** | [Usage description] | [X%] | [Effort level] | [Notes] |

### Implementation Gap Analysis

**What Exists (Verified)**:
- ✅ [Existing capability 1]
- ✅ [Existing capability 2]
- ✅ [Existing capability 3]

**What's Missing (CRD V1 Requirements)**:
- ❌ [Missing capability 1]
- ❌ [Missing capability 2]
- ❌ [Missing capability 3]

**Code Quality Issues to Address**:
- ⚠️ [Issue 1 with solution]
- ⚠️ [Issue 2 with solution]

**Estimated Migration Effort**: [X days/weeks]
- Day 1-2: [Phase 1]
- Day 3-4: [Phase 2]
- Day 5-6: [Phase 3]

---

## CRD Schema Specification [REQUIRED]

**Full Schema**: See [docs/design/CRD/XX_[CRD_NAME]_CRD.md](../../design/CRD/XX_[CRD_NAME]_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `XX_[CRD_NAME]_CRD.md`.

**Location**: `api/v1/[crdname]_types.go`

```go
package v1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// [CRD]Spec defines the desired state of [CRD]
type [CRD]Spec struct {
    // RemediationRequestRef references the parent RemediationRequest CRD
    RemediationRequestRef corev1.ObjectReference `json:"alertRemediationRef"`

    // [Spec fields based on service requirements]
}

// [CRD]Status defines the observed state
type [CRD]Status struct {
    // Phase tracks current processing stage
    Phase string `json:"phase"` // "[phase1]", "[phase2]", "completed", "failed"

    // [Status fields based on service outputs]
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// [CRD] is the Schema for the [crds] API
type [CRD] struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   [CRD]Spec   `json:"spec,omitempty"`
    Status [CRD]Status `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// [CRD]List contains a list of [CRD]
type [CRD]List struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           [][CRD] `json:"items"`
}

func init() {
    SchemeBuilder.Register(&[CRD]{}, &[CRD]List{})
}
```

---

## Controller Implementation [REQUIRED]

**Location**: `internal/controller/[crdname]_controller.go`

### Controller Configuration

**Critical Patterns from MULTI_CRD_RECONCILIATION_ARCHITECTURE.md**:
1. **Owner References**: [CRD] owned by RemediationRequest for cascade deletion
2. **Finalizers**: Cleanup coordination before deletion [CONDITIONAL]
3. **Watch Optimization**: Status updates trigger RemediationRequest reconciliation [CONDITIONAL]
4. **Timeout Handling**: Phase-level timeout detection and escalation [CONDITIONAL]
5. **Event Emission**: Operational visibility through Kubernetes events
6. **Optimized Requeue Strategy**: Phase-based requeue intervals

```go
package controller

import (
    "context"
    "fmt"
    "time"

    // CRD API Imports - Each CRD in its own dedicated package
    // NOTE: "kubernaut" ONLY appears in K8s API Group strings, NOT in Go import paths
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"         // RemediationRequest (parent orchestrator)
    [servicename]v1 "github.com/jordigilh/kubernaut/api/[servicename]/v1"     // This service's CRD (own types)
    [downstream]v1 "github.com/jordigilh/kubernaut/api/[downstream]/v1"       // Downstream CRDs (creates)

    // Example for Remediation Processor:
    // remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    // processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
    // aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
    // Finalizer for cleanup coordination [CONDITIONAL]
    [crdname]Finalizer = "[crdname].kubernaut.io/finalizer"

    // Timeout configuration [CONDITIONAL]
    defaultPhaseTimeout = [X] * time.Minute
)

// [CRD]Reconciler reconciles a [CRD] object
type [CRD]Reconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder

    // Service dependencies
    [Service1] [Service1Type]
    [Service2] [Service2Type]
}

//+kubebuilder:rbac:groups=kubernaut.io,resources=[crds],verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.io,resources=[crds]/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.io,resources=[crds]/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertremediations,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertremediations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *[CRD]Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch [CRD] CRD
    var [crdvar] [packageimport].[CRD]
    if err := r.Get(ctx, req.NamespacedName, &[crdvar]); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion with finalizer [CONDITIONAL]
    if ![crdvar].DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &[crdvar])
    }

    // Add finalizer if not present [CONDITIONAL]
    if !controllerutil.ContainsFinalizer(&[crdvar], [crdname]Finalizer) {
        controllerutil.AddFinalizer(&[crdvar], [crdname]Finalizer)
        if err := r.Update(ctx, &[crdvar]); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Set owner reference to RemediationRequest (for cascade deletion)
    if err := r.ensureOwnerReference(ctx, &[crdvar]); err != nil {
        log.Error(err, "Failed to set owner reference")
        r.Recorder.Event(&[crdvar], "Warning", "OwnerReferenceFailed",
            fmt.Sprintf("Failed to set owner reference: %v", err))
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Check for phase timeout [CONDITIONAL]
    if r.isPhaseTimedOut(&[crdvar]) {
        return r.handlePhaseTimeout(ctx, &[crdvar])
    }

    // Initialize phase if empty
    if [crdvar].Status.Phase == "" {
        [crdvar].Status.Phase = "[initial_phase]"
        [crdvar].Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        if err := r.Status().Update(ctx, &[crdvar]); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on current phase
    var result ctrl.Result
    var err error

    switch [crdvar].Status.Phase {
    case "[phase1]":
        result, err = r.reconcile[Phase1](ctx, &[crdvar])
    case "[phase2]":
        result, err = r.reconcile[Phase2](ctx, &[crdvar])
    case "[phase3]":
        result, err = r.reconcile[Phase3](ctx, &[crdvar])
    case "completed":
        return r.determineRequeueStrategy(&[crdvar]), nil
    default:
        log.Error(nil, "Unknown phase", "phase", [crdvar].Status.Phase)
        r.Recorder.Event(&[crdvar], "Warning", "UnknownPhase",
            fmt.Sprintf("Unknown phase: %s", [crdvar].Status.Phase))
        return ctrl.Result{RequeueAfter: time.Second * 30}, nil
    }

    return result, err
}

func (r *[CRD]Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&[packageimport].[CRD]{}).
        Complete(r)
}
```

---

## Prometheus Metrics Implementation [REQUIRED]

### Metrics Server Setup

**Two-Port Architecture** for security separation:

```go
package main

import (
    "github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/sirupsen/logrus"
    "net/http"
)

func main() {
    log := logrus.New()

    // Port 8080: Health and Readiness (NO AUTH - for Kubernetes probes)
    healthMux := http.NewServeMux()
    healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    })
    healthMux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("READY"))
    })

    go func() {
        log.Info("Starting health/ready server on :8080")
        if err := http.ListenAndServe(":8080", healthMux); err != nil {
            log.Fatal("Health server failed: ", err)
        }
    }()

    // Port 9090: Prometheus Metrics (WITH AUTH FILTER)
    metricsMux := http.NewServeMux()
    metricsMux.Handle("/metrics", promhttp.Handler())

    go func() {
        log.Info("Starting metrics server on :9090")
        if err := http.ListenAndServe(":9090", metricsMux); err != nil {
            log.Fatal("Metrics server failed: ", err)
        }
    }()

    // Endpoints exposed:
    // - http://localhost:8080/health  (no auth - Kubernetes liveness probe)
    // - http://localhost:8080/ready   (no auth - Kubernetes readiness probe)
    // - http://localhost:9090/metrics (with auth - Prometheus scraping)

    // ... rest of controller initialization
}
```

**Security Rationale**:
- **Port 8080 (Health/Ready)**: No authentication - Kubernetes needs unauthenticated access for liveness/readiness probes
- **Port 9090 (Metrics)**: With auth filter - Prometheus metrics can contain sensitive operational data

### Service-Specific Metrics Registration

**Using `promauto` for Automatic Registration**:

```go
package [packagename]

import (
    "time"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counter: [Metric description]
    [MetricName]Total = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_[servicename]_[metric]_total",
        Help: "[Help text describing what this measures]",
    }, []string{"[label1]", "[label2]", "[label3]"})

    // Histogram: [Metric description]
    [MetricName]Duration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "kubernaut_[servicename]_[metric]_duration_seconds",
        Help:    "[Help text describing duration measurement]",
        Buckets: []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
    }, []string{"[label1]", "[label2]"})

    // Gauge: [Metric description]
    [MetricName]Gauge = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "kubernaut_[servicename]_[metric]",
        Help: "[Help text describing current value]",
    })

    // Counter: [Error tracking]
    ErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_[servicename]_errors_total",
        Help: "[Help text for error tracking]",
    }, []string{"error_type", "phase"})
)

// Helper function to record [metric description]
func Record[MetricName]([params]) {
    [MetricName]Total.WithLabelValues([label_values]).Inc()
    [MetricName]Duration.WithLabelValues([label_values]).Observe([duration].Seconds())
}
```

### Recommended Grafana Dashboards

**Prometheus Queries for Monitoring**:

```promql
# [Service] processing rate ([metric]/sec)
rate(kubernaut_[servicename]_[metric]_total[5m])

# Processing duration by phase (p95)
histogram_quantile(0.95, rate(kubernaut_[servicename]_processing_duration_seconds_bucket[5m]))

# Error rate by phase
rate(kubernaut_[servicename]_errors_total[5m]) / rate(kubernaut_[servicename]_[metric]_total[5m])

# Active processing queue depth
kubernaut_[servicename]_active_[metric]
```

### Metrics Naming Convention

Following Prometheus best practices and Kubernaut standards:
- **Prefix**: `kubernaut_[servicename]_`
- **Metric Types**:
  - `_total` suffix for counters
  - `_seconds` suffix for duration histograms
  - `_bytes` suffix for size histograms
  - No suffix for gauges
- **Labels**: Use lowercase with underscores (e.g., `phase`, `error_type`)

### Performance Targets

- **[Service] Processing**: p95 < [Xms], p99 < [Xms]
- **[Phase 1] Phase**: p95 < [Xms], p99 < [Xms]
- **[Phase 2] Phase**: p95 < [Xms], p99 < [Xms]
- **[Phase N] Phase**: p95 < [Xms], p99 < [Xms]
- **Error Rate**: < 0.1% of processed [items]
- **[External Service] Calls**: p95 < [Xms], success rate > 99.9%

---

## Testing Strategy [REQUIRED]

**Testing Framework Reference**: [.cursor/rules/03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)

### Pyramid Testing Approach

Following Kubernaut's defense-in-depth testing strategy:

- **Unit Tests (70%+)**: Comprehensive controller logic with mocked external dependencies
- **Integration Tests (20%)**: Cross-component CRD interactions with real K8s
- **E2E Tests (10%)**: Complete [workflow description] workflows

---

## 🎯 Test Level Selection: Maintainability First

**Principle**: Prioritize maintainability and simplicity when choosing between unit, integration, and e2e tests.

### Decision Framework

```mermaid
flowchart TD
    Start[New [Service] Test] --> Question1{Can test with<br/>Fake K8s client?<br/>&lt;[N] lines setup}

    Question1 -->|Yes| Question2{Testing [logic type]<br/>or [infrastructure type]?}
    Question1 -->|No| Integration[Integration Test]

    Question2 -->|[Logic Type]| Question3{Test readable<br/>and maintainable?}
    Question2 -->|[Infrastructure]| Integration

    Question3 -->|Yes| Unit[Unit Test]
    Question3 -->|No| Integration

    Unit --> Validate1[✅ Use Unit Test]
    Integration --> Validate2{Complete [workflow]<br/>pipeline with real<br/>[external services]?}

    Validate2 -->|Yes| E2E[E2E Test]
    Validate2 -->|No| Validate3[✅ Use Integration Test]

    E2E --> Validate4[✅ Use E2E Test]

    style Unit fill:#90EE90
    style Integration fill:#87CEEB
    style E2E fill:#FFB6C1
```

**Instructions for Service Customization**:
- Replace `[Service]` with service name (e.g., "RemediationProcessor", "AIAnalysis")
- Replace `[N]` with appropriate line threshold (typically 20-40 lines)
- Replace `[logic type]` with service-specific logic (e.g., "classification logic", "workflow logic")
- Replace `[infrastructure type]` with service-specific infrastructure (e.g., "K8s reconciliation", "CRD lifecycle")
- Replace `[Logic Type]` with capitalized version
- Replace `[Infrastructure]` with capitalized version
- Replace `[workflow]` with service-specific workflow (e.g., "remediation", "analysis")
- Replace `[external services]` with service-specific externals (e.g., "LLM + K8s", "SMTP + Slack")

### Test at Unit Level WHEN

- ✅ Scenario can be tested with **Fake K8s client** (sigs.k8s.io/controller-runtime/pkg/client/fake)
- ✅ Focus is on **[service-specific logic]** ([examples of logic types])
- ✅ Setup is **straightforward** (< [N] lines of Fake K8s client configuration)
- ✅ Test remains **readable and maintainable** with mocking

**[Service Name] Unit Test Examples**:
- [Logic type 1] ([specific example])
- [Logic type 2] ([specific example])
- [Logic type 3] ([specific example])
- [Logic type 4] ([specific example])

---

### Move to Integration Level WHEN

- ✅ Scenario requires **real K8s API Server** (envtest with controller-runtime)
- ✅ Validating **[service-specific integration]** ([examples of integration patterns])
- ✅ Unit test would require **excessive Fake K8s mocking** (>[N] lines of fake client setup)
- ✅ Integration test is **simpler to understand** and maintain
- ✅ Testing **cross-component [interaction type]** ([specific examples])

**[Service Name] Integration Test Examples**:
- [Integration scenario 1]
- [Integration scenario 2]
- [Integration scenario 3]
- [Integration scenario 4]

---

### Move to E2E Level WHEN

- ✅ Testing **complete [service workflow]** ([end-to-end flow description])
- ✅ Validating **end-to-end [service pipeline]** (all services + real infrastructure)
- ✅ Lower-level tests **cannot reproduce full [workflow]** ([specific integration needs])

**[Service Name] E2E Test Examples**:
- [E2E scenario 1]
- [E2E scenario 2]
- [E2E scenario 3]

---

## 🧭 Maintainability Decision Criteria

**Ask these 5 questions before implementing a unit test:**

### 1. Mock Complexity
**Question**: Will [external dependency] mocking be >[N] lines?
- ✅ **YES** → Consider integration test with [real infrastructure type]
- ❌ **NO** → Unit test acceptable

**[Service Name] Example**:
```go
// ❌ COMPLEX: [N]+ lines of [dependency] mock setup
mock[Dependency] := &Mock[Dependency]{
    // [N]+ lines of mock configuration
}
// BETTER: Integration test with [real infrastructure]

// ✅ SIMPLE: [Logic] with Fake K8s client
fakeClient := fake.NewClientBuilder().WithObjects([objects]...).Build()
result := [component].[Method](ctx, fakeClient, [params])
Expect(result).To([matcher])
```

---

### 2. Readability
**Question**: Would a new developer understand this test in 2 minutes?
- ✅ **YES** → Unit test is good
- ❌ **NO** → Consider higher test level

**[Service Name] Example**:
```go
// ✅ READABLE: Clear [logic] test
It("should [behavior description]", func() {
    [setup] := testutil.New[Type]([params])

    result, err := [component].[Method](ctx, [setup])

    Expect(err).ToNot(HaveOccurred())
    Expect(result.[Field]).To(Equal("[expected]"))
    Expect(result.[Field2]).To(ContainSubstring("[value]"))
})
```

---

### 3. Fragility
**Question**: Does test break when [internal implementation] changes?
- ✅ **YES** → Move to integration test (testing implementation, not behavior)
- ❌ **NO** → Unit test is appropriate

**[Service Name] Example**:
```go
// ❌ FRAGILE: Breaks if we change [internal detail]
Expect([internal].Field).To(Equal("[specific_value]"))

// ✅ STABLE: Tests [service] outcome, not [internal detail]
result, err := [component].[Method](ctx, [input])
Expect(err).ToNot(HaveOccurred())
Expect(result.[PublicField]).To(Equal("[expected]"))
Expect(result.[Metric]).To(BeNumerically(">", [threshold]))
```

---

### 4. Real Value
**Question**: Is this testing [logic type] or [infrastructure type]?
- **[Logic Type]** → Unit test with Fake K8s client
- **[Infrastructure Type]** → Integration test with real K8s/infrastructure

**[Service Name] Decision**:
- **Unit**: [Logic examples] (pure logic)
- **Integration**: [Infrastructure examples] (infrastructure)

---

### 5. Maintenance Cost
**Question**: How much effort to maintain this vs integration test?
- **Lower cost** → Choose that option

**[Service Name] Example**:
- **Unit test with [N]-line [dependency] mock**: HIGH maintenance (breaks on [dependency] changes)
- **Integration test with [real infrastructure]**: LOW maintenance (automatically adapts to evolution)

---

## 🎯 Realistic vs. Exhaustive Testing

**Principle**: Test realistic [service scenarios] necessary to validate business requirements - not more, not less.

### [Service Name]: Requirement-Driven Coverage

**Business Requirement Analysis** (BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX):

| [Service] Dimension | Realistic Values | Test Strategy |
|---|---|---|
| **[Dimension 1]** | [value1], [value2], [value3] ([N] values) | Test [strategy1] |
| **[Dimension 2]** | [value1], [value2], [value3] ([N] values) | Test [strategy2] |
| **[Dimension 3]** | [value1], [value2], [value3] ([N] values) | Test [strategy3] |
| **[Dimension 4]** | [value1], [value2], [value3] ([N] values) | Test [strategy4] |

**Total Possible Combinations**: [N1] × [N2] × [N3] × [N4] = [Total] combinations
**Distinct Business Behaviors**: [N] behaviors (per BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX)
**Tests Needed**: ~[N] tests (covering [N] distinct behaviors with edge cases)

---

### ✅ DO: Test Distinct [Service] Behaviors Using DescribeTable

**BEST PRACTICE**: Use Ginkgo DescribeTable for [service-specific pattern] testing.

```go
// ✅ GOOD: Tests distinct [behavior] using DescribeTable
var _ = Describe("BR-[CATEGORY]-XXX: [Behavior Name]", func() {
    DescribeTable("[Behavior description]",
        func([params]) {
            [setup] := testutil.New[Type]([param1], [param2])

            result := [component].[Method](ctx, [setup])

            Expect(result.[Field]).To(Equal([expectedValue]))
        },
        // BR-[CATEGORY]-XXX.1: [Scenario 1 description]
        Entry("[scenario 1 name]",
            [param1_value], [param2_value], [expected1]),

        // BR-[CATEGORY]-XXX.2: [Scenario 2 description]
        Entry("[scenario 2 name]",
            [param1_value], [param2_value], [expected2]),

        // BR-[CATEGORY]-XXX.3: [Scenario 3 description]
        Entry("[scenario 3 name]",
            [param1_value], [param2_value], [expected3]),

        // BR-[CATEGORY]-XXX.4: [Edge case]
        Entry("[edge case name]",
            [param1_value], [param2_value], [expected4]),
    )
})
```

**Why DescribeTable is Better for [Service] Testing**:
- ✅ [N] scenarios in single test function (vs. [N] separate test functions)
- ✅ Change [logic] once, all scenarios tested
- ✅ Clear [decision] rules visible
- ✅ Easy to add new [scenarios]
- ✅ Perfect for testing [pattern] matrices

---

### ❌ DON'T: Test Redundant [Service] Variations

```go
// ❌ BAD: Redundant tests that validate SAME [logic]
func Test[Scenario]1() {
    // Tests [logic] for [input1]
}

func Test[Scenario]2() {
    // Tests [logic] for [input2]
}

func Test[Scenario]3() {
    // Tests [logic] for [input3]
}
// All 3 tests validate SAME [logic] with different [inputs]
// BETTER: One test for [logic], one for edge case

// ❌ BAD: Exhaustive [parameter] permutations
func Test[Param]1() {}
func Test[Param]2() {}
// ... [N] more combinations
// These don't test DISTINCT [behavior]
```

---

### Decision Criteria: Is This [Service] Test Necessary?

Ask these 4 questions:

1. **Does this test validate a distinct [service behavior] or [rule]?**
   - ✅ YES: [Example of distinct behavior] (BR-[CATEGORY]-XXX.X)
   - ❌ NO: Testing different [inputs] with same [logic]

2. **Does this [scenario] actually occur in production?**
   - ✅ YES: [Realistic production scenario]
   - ❌ NO: [Unrealistic scenario]

3. **Would this test catch a [service] bug the other tests wouldn't?**
   - ✅ YES: [Edge case or boundary condition]
   - ❌ NO: Testing [N] different [inputs] with same [logic]

4. **Is this testing [behavior] or implementation variation?**
   - ✅ [Behavior]: [Business logic example]
   - ❌ Implementation: [Internal detail example]

**If answer is "NO" to all 4 questions** → Skip the test, it adds maintenance cost without [service] value

---

### [Service Name] Test Coverage Example with DescribeTable

**BR-[CATEGORY]-XXX: [Behavior Name] ([N] distinct scenarios)**

```go
var _ = Describe("BR-[CATEGORY]-XXX: [Behavior]", func() {
    // ANALYSIS: [N1] × [N2] × [N3] = [Total] combinations
    // REQUIREMENT ANALYSIS: Only [N] distinct behaviors per BR-[CATEGORY]-XXX
    // TEST STRATEGY: Use DescribeTable for [N] scenarios + [M] edge cases

    DescribeTable("[Test description]",
        func([params], [expected]) {
            [setup] := testutil.New[Type]([param1], [param2])

            result, err := [component].[Method](ctx, [setup])

            if [expectError] {
                Expect(err).To(HaveOccurred())
            } else {
                Expect(err).ToNot(HaveOccurred())
            }
            Expect(result).To(Equal([expected]))
        },
        // Scenario 1: [Description]
        Entry("[scenario 1]",
            [param1], [param2], [expected1], false),

        // Scenario 2: [Description]
        Entry("[scenario 2]",
            [param1], [param2], [expected2], false),

        // Edge case 1: [Description]
        Entry("[edge case]",
            [param1], [param2], [expected3], true),
    )
})
```

**Benefits for [Service] Testing**:
- ✅ **[N] scenarios tested in ~[M] lines** (vs. ~[K] lines with separate functions)
- ✅ **Single [component]** - changes apply to all scenarios
- ✅ **Clear [rules]** - [logic] immediately visible
- ✅ **Easy to add [scenarios]** - new parameter for new [types]
- ✅ **[X]% less maintenance** for complex [testing type]

---

## ⚠️ Anti-Patterns to AVOID

### ❌ OVER-EXTENDED UNIT TESTS (Forbidden)

**Problem**: Excessive [dependency] mocking (>[N] lines) makes [service] tests unmaintainable

```go
// ❌ BAD: [N]+ lines of [dependency] mock setup
mock[Dependency] := &Mock[Dependency]{
    // [N]+ lines of mock configuration
}
// THIS SHOULD BE AN INTEGRATION TEST

// BETTER: Integration test with [real infrastructure]
```

**Solution**: Move to integration test with [infrastructure type]

```go
// ✅ GOOD: Integration test with [real infrastructure] (envtest)
var _ = Describe("Integration: [Scenario]", func() {
    It("should [behavior] with real [infrastructure]", func() {
        // Test with real [infrastructure] - validates actual behavior
    })
})
```

---

### ❌ WRONG TEST LEVEL (Forbidden)

**Problem**: Testing [infrastructure] in unit tests

```go
// ❌ BAD: Testing actual [infrastructure] in unit test
func Test[Infrastructure]() {
    // Complex Fake K8s client setup
    // Real [infrastructure] behavior - belongs in integration test
}
```

**Solution**: Use integration test for [infrastructure]

```go
// ✅ GOOD: Integration test for [infrastructure]
Describe("Integration: [Infrastructure]", func() {
    It("should [behavior]", func() {
        // Test with real [infrastructure]
    })
})
```

---

### ❌ REDUNDANT COVERAGE (Forbidden)

**Problem**: Testing same [logic] at multiple levels

```go
// ❌ BAD: Testing exact same [logic] at all 3 levels
// Unit test: [Logic]
// Integration test: [Logic] (duplicate)
// E2E test: [Logic] (duplicate)
// NO additional value
```

**Solution**: Test [logic] in unit tests, test INTEGRATION in higher levels

```go
// ✅ GOOD: Each level tests distinct aspect
// Unit test: [Logic] correctness
// Integration test: [Logic] + [infrastructure integration]
// E2E test: [Logic] + [infrastructure] + complete [workflow] pipeline
// Each level adds unique [service] value
```

---

### Unit Tests (Primary Coverage Layer)

**Test Directory**: [test/unit/](../../../test/unit/)
**Service Tests**: Create `test/unit/[packagename]/controller_test.go`
**Coverage Target**: 70%+ of business requirements (BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX)
**Confidence**: 85-90%
**Execution**: `make test`

**Testing Strategy**: Use fake K8s client for compile-time API safety. Mock ONLY external HTTP services ([External Service Names]). Use REAL business logic components.

**Rationale for Fake K8s Client**:
- ✅ **Compile-Time API Safety**: K8s API changes/deprecations caught at build time
- ✅ **Type-Safe CRD Handling**: Schema changes validated by compiler
- ✅ **Real K8s Errors**: `apierrors.IsNotFound()`, `apierrors.IsConflict()` behavior
- ✅ **Acceptable Speed**: ~0.8s execution (worth the trade-off for production safety)
- ✅ **Upgrade Protection**: Breaking API changes explicit, not hidden

**Test File Structure** (aligned with package name `[packagename]`):
```
test/unit/
├── [packagename]/              # Matches pkg/[packagename]/
│   ├── controller_test.go      # Main controller reconciliation tests
│   ├── [phase1]_test.go        # [Phase 1] phase tests
│   ├── [phase2]_test.go        # [Phase 2] phase tests
│   └── suite_test.go           # Ginkgo test suite setup
└── ...
```

**Example Unit Test**:
```go
package [packagename]

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "time"

    // CRD API Imports - Each CRD in its own dedicated package
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    [servicename]v1 "github.com/jordigilh/kubernaut/api/[servicename]/v1"
    [downstream]v1 "github.com/jordigilh/kubernaut/api/[downstream]/v1"
    "github.com/jordigilh/kubernaut/internal/controller"
    "github.com/jordigilh/kubernaut/pkg/testutil"
    "github.com/jordigilh/kubernaut/pkg/testutil/mocks"

    v1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("BR-[CATEGORY]-XXX: [Service] Controller", func() {
    var (
        // Fake K8s client for compile-time API safety
        fakeK8sClient client.Client
        scheme        *runtime.Scheme

        // Mock ONLY external HTTP services
        mock[Service] *mocks.Mock[Service]

        // Use REAL business logic components
        [component] *[componentType]
        reconciler  *controller.[CRD]Reconciler
        ctx         context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Minimal scheme: Only types needed for these tests
        scheme = runtime.NewScheme()
        _ = v1.AddToScheme(scheme)
        _ = remediationv1.AddToScheme(scheme)
        _ = [servicename]v1.AddToScheme(scheme)
        _ = [downstream]v1.AddToScheme(scheme)

        // Fake K8s client with compile-time API safety
        fakeK8sClient = fake.NewClientBuilder().
            WithScheme(scheme).
            Build()

        // Mock external HTTP service (NOT K8s)
        mock[Service] = mocks.NewMock[Service]()

        // Use REAL business logic
        [component] = [componentType].New(testutil.NewTestConfig())

        reconciler = &controller.[CRD]Reconciler{
            Client:     fakeK8sClient,
            Scheme:     scheme,
            [Service]: mock[Service],
            [Component]: [component], // Real business logic
        }
    })

    Context("BR-[CATEGORY]-XXX: [Test Context Description]", func() {
        It("should [behavior description]", func() {
            // Setup test [CRD]
            [crdvar] := &[packageimport].[CRD]{
                ObjectMeta: metav1.ObjectMeta{
                    Name:              "test-[name]",
                    Namespace:         "default",
                    CreationTimestamp: metav1.Now(),
                },
                Spec: [packageimport].[CRD]Spec{
                    // [Spec fields]
                },
            }

            // Create [CRD] CRD in fake K8s (compile-time safe)
            Expect(fakeK8sClient.Create(ctx, [crdvar])).To(Succeed())

            // Mock [External Service] response
            mock[Service].On("[Method]", ctx, [params]).Return(
                [returnType]{
                    // [Return data]
                },
                nil,
            )

            // Execute reconciliation
            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest([crdvar]))

            // Validate business outcomes
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeTrue(), "[requeue reason]")
            Expect([crdvar].Status.Phase).To(Equal("[expected_phase]"))
            Expect([crdvar].Status.[Field]).To([matcher])

            // Verify [External Service] was called
            mock[Service].AssertNumberOfCalls(GinkgoT(), "[Method]", 1)
        })

        // Example: Table-Driven Tests for Multiple Scenarios
        DescribeTable("BR-[CATEGORY]-XXX: [Behavior] across different scenarios",
            func(
                testName string,
                inputPhase string,
                inputData map[string]interface{},
                expectedPhase string,
                expectedField string,
                shouldSucceed bool,
            ) {
                // Setup test [CRD] with parameterized input
                [crdvar] := &[packageimport].[CRD]{
                    ObjectMeta: metav1.ObjectMeta{
                        Name:      fmt.Sprintf("test-%s", testName),
                        Namespace: "default",
                    },
                    Spec: [packageimport].[CRD]Spec{
                        // Use inputData to populate spec
                    },
                    Status: [packageimport].[CRD]Status{
                        Phase: inputPhase,
                    },
                }

                Expect(fakeK8sClient.Create(ctx, [crdvar])).To(Succeed())

                // Mock external service with parameterized response
                if shouldSucceed {
                    mock[Service].On("[Method]", ctx, mock.Anything).Return(
                        [returnType]{/* success response */},
                        nil,
                    )
                } else {
                    mock[Service].On("[Method]", ctx, mock.Anything).Return(
                        [returnType]{},
                        fmt.Errorf("simulated error for %s", testName),
                    )
                }

                // Execute reconciliation
                result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest([crdvar]))

                // Validate based on expected outcome
                if shouldSucceed {
                    Expect(err).ToNot(HaveOccurred())
                    Expect([crdvar].Status.Phase).To(Equal(expectedPhase))
                    Expect([crdvar].Status.[Field]).To(ContainSubstring(expectedField))
                } else {
                    Expect(err).To(HaveOccurred())
                    Expect(result.RequeueAfter).To(BeNumerically(">", 0))
                }
            },
            // Table entries: each Entry represents a test case
            Entry("critical priority with complete data",
                "critical-complete",           // testName
                "processing",                  // inputPhase
                map[string]interface{}{        // inputData
                    "priority": "critical",
                    "complete": true,
                },
                "completed",                   // expectedPhase
                "success",                     // expectedField
                true,                          // shouldSucceed
            ),
            Entry("warning priority with partial data",
                "warning-partial",
                "processing",
                map[string]interface{}{
                    "priority": "warning",
                    "complete": false,
                },
                "processing",
                "pending",
                true,
            ),
            Entry("degraded mode with missing service",
                "degraded-missing",
                "processing",
                map[string]interface{}{
                    "priority": "high",
                    "complete": true,
                },
                "failed",
                "error",
                false,
            ),
            Entry("production environment with high confidence",
                "prod-high-confidence",
                "classifying",
                map[string]interface{}{
                    "environment": "production",
                    "confidence":  0.95,
                },
                "completed",
                "production",
                true,
            ),
            Entry("development environment with low confidence",
                "dev-low-confidence",
                "classifying",
                map[string]interface{}{
                    "environment": "development",
                    "confidence":  0.6,
                },
                "completed",
                "development",
                true,
            ),
        )
    })
})
```

**Table-Driven Test Benefits**:
- ✅ **Reduced Duplication**: Single test logic for multiple scenarios
- ✅ **Easy to Extend**: Add new test cases by adding `Entry` items
- ✅ **Clear Test Matrix**: Each `Entry` clearly shows input/output relationships
- ✅ **Better Coverage**: Encourages testing edge cases and variations
- ✅ **Readable Reports**: Ginkgo reports show each table entry as a separate test

### Integration Tests (Component Interaction Layer)

**Test Directory**: [test/integration/](../../../test/integration/)
**Service Tests**: Create `test/integration/[packagename]/integration_test.go`
**Coverage Target**: 20% of business requirements
**Confidence**: 80-85%
**Execution**: `make test-integration-kind` (local) or `make test-integration-kind-ci` (CI)

**Strategy**: Test CRD interactions with real Kubernetes API server in KIND cluster.

**Test File Structure**:
```
test/integration/
├── [packagename]/              # Matches pkg/[packagename]/
│   ├── integration_test.go     # CRD lifecycle and interaction tests
│   ├── crd_phase_transitions_test.go
│   └── suite_test.go           # Integration test suite setup
└── ...
```

### E2E Tests (End-to-End Workflow Layer)

**Test Directory**: [test/e2e/](../../../test/e2e/)
**Service Tests**: Create `test/e2e/[packagename]/e2e_test.go`
**Coverage Target**: 10% of critical business workflows
**Confidence**: 90-95%
**Execution**: `make test-e2e-kind` (KIND) or `make test-e2e-ocp` (Kubernetes)

**Test File Structure**:
```
test/e2e/
├── [packagename]/              # Matches pkg/[packagename]/
│   ├── e2e_test.go             # End-to-end workflow tests
│   ├── [scenario1]_flow_test.go
│   └── suite_test.go           # E2E test suite setup
└── ...
```

### Test Coverage Requirements

**Business Requirement Mapping**:
- **BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX**: [Capability description] (Unit + Integration)
- **BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX**: [Capability description] (Unit + Integration)
- **BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX**: [Capability description] (Unit + E2E)

### Mock Usage Decision Matrix

| Component | Unit Tests | Integration | E2E | Justification |
|-----------|------------|-------------|-----|---------------|
| **Kubernetes API** | **FAKE K8S CLIENT** (`sigs.k8s.io/controller-runtime/pkg/client/fake`) | REAL (KIND) | REAL (OCP/KIND) | Compile-time API safety, type-safe CRD handling |
| **[External Service]** | **CUSTOM MOCK** (`pkg/testutil/mocks`) | REAL | REAL | External HTTP service - controlled test data |
| **[Business Component]** | REAL | REAL | REAL | Core business logic |
| **[CRD] CRD** | **FAKE K8S CLIENT** | REAL | REAL | Kubernetes resource - type-safe testing |
| **Metrics Recording** | REAL | REAL | REAL | Business observability |

**Terminology**:
- **FAKE K8S CLIENT**: In-memory K8s API server (`fake.NewClientBuilder()`) - provides compile-time type safety
- **CUSTOM MOCK**: Test doubles from `pkg/testutil/mocks` for external HTTP services
- **REAL**: Actual implementation (business logic or live external service)

### Anti-Patterns to AVOID

**❌ NULL-TESTING** (Forbidden):
```go
// BAD: Weak assertions
Expect(result).ToNot(BeNil())
Expect(count).To(BeNumerically(">", 0))
```

**✅ BUSINESS OUTCOME TESTING** (Required):
```go
// GOOD: Business-meaningful validations
Expect([field].Value).To(Equal("[expected_value]"))
Expect([metric]).To(BeNumerically(">", [threshold]))
Expect([status].Phase).To(Equal("[expected_phase]"))
```

**❌ IMPLEMENTATION TESTING** (Forbidden):
```go
// BAD: Testing internal implementation
Expect(reconciler.callCount).To(Equal(3))
```

**✅ BEHAVIOR TESTING** (Required):
```go
// GOOD: Testing business behavior
Expect([crdvar].Status.Phase).To(Equal("completed"))
Expect([crdvar].Status.[Field]).To(MatchRegexp(`[pattern]`))
```

---

## Performance Targets [REQUIRED]

- **[Phase 1]**: <[X]s
- **[Phase 2]**: <[X]ms
- **[Phase N]**: <[X]ms
- **Total processing**: <[X]s
- **Accuracy**: >[X]% for [metric description]
- **[External Service] Response Time**: p95 < [X]ms, p99 < [X]ms

---

## Database Integration for Audit & Tracking [CONDITIONAL]

### Dual Audit System

Following the [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md#crd-lifecycle-management-and-cleanup) dual audit approach:

1. **CRDs**: Real-time execution state + 24-hour review window
2. **Database**: Long-term compliance + post-mortem analysis

### Audit Data Persistence

**Database Service**: Data Storage Service (Port 8085)
**Purpose**: Persist [service name] audit trail before CRD cleanup

```go
package controller

import (
    "github.com/jordigilh/kubernaut/pkg/storage"
)

type [CRD]Reconciler struct {
    client.Client
    Scheme       *runtime.Scheme
    AuditStorage storage.AuditStorageClient  // Database client for audit trail
}

// After each phase completion, persist audit data
func (r *[CRD]Reconciler) reconcile[Phase]WithAudit(ctx context.Context, [crdvar] *[packageimport].[CRD]) (ctrl.Result, error) {
    // ... phase logic ...

    // Persist audit trail
    auditRecord := &storage.[Service]Audit{
        // [Audit record fields]
    }

    if err := r.AuditStorage.Store[Service]Audit(ctx, auditRecord); err != nil {
        r.Log.Error(err, "Failed to store audit", "fingerprint", [crdvar].Spec.[Field])
        ErrorsTotal.WithLabelValues("audit_storage_failed", "[phase]").Inc()
        // Don't fail reconciliation, but log for investigation
    }

    // ... continue with phase ...
}
```

### Audit Data Schema

```go
package storage

type [Service]Audit struct {
    ID            string    `json:"id" db:"id"`
    RemediationID string    `json:"remediation_id" db:"remediation_id"`
    // [Service-specific audit fields]
    CompletedAt   time.Time `json:"completed_at" db:"completed_at"`
    Status        string    `json:"status" db:"status"`
    ErrorMessage  string    `json:"error_message,omitempty" db:"error_message"`
}
```

### Audit Use Cases

**Post-Mortem Analysis**:
```sql
-- [Service-specific queries]
```

**Performance Tracking**:
```sql
-- [Service-specific queries]
```

**Compliance Reporting**:
- [Compliance use case 1]
- [Compliance use case 2]

### Storage Service Integration

**Dependencies**:
- Data Storage Service (Port 8085)
- PostgreSQL database with `[service]_audit` table
- Optional: Vector DB for [data type] storage

### Audit Metrics

```go
var (
    // Counter: Audit storage attempts
    AuditStorageAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_[servicename]_audit_storage_attempts_total",
        Help: "Total audit storage attempts",
    }, []string{"status"}) // success, error

    // Histogram: Audit storage duration
    AuditStorageDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_[servicename]_audit_storage_duration_seconds",
        Help:    "Duration of audit storage operations",
        Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
    })
)
```

---

## Integration Points [REQUIRED]

### 1. Upstream Integration: [Upstream Service/CRD Name]

**Integration Pattern**: [Watch-based / HTTP / CRD creation / etc.]

**How [CRD] is Created**:
```go
// [Code example showing how this CRD is created]
```

**Note**: [Important notes about creation pattern]

### 2. Downstream Integration: [Downstream Service/CRD Name]

**Integration Pattern**: [Integration pattern description]

**How [Next Action] Happens**:
```go
// [Code example showing downstream integration]
```

**Note**: [Important notes about downstream integration]

### 3. External Service Integration: [External Service Name]

**Integration Pattern**: [Synchronous HTTP / Async / etc.]

**Endpoint**: [External Service] - see [README: Service Description](../../README.md#section)

**Request/Response**:
```go
type [Request] struct {
    // [Request fields]
}

type [Response] struct {
    // [Response fields]
}
```

**Degraded Mode Fallback** [CONDITIONAL]:
```go
// [Code showing fallback behavior]
```

### 4. Database Integration: Data Storage Service [CONDITIONAL]

**Integration Pattern**: Audit trail persistence

**Endpoint**: Data Storage Service HTTP POST `/api/v1/audit/[service]`

**Audit Record**:
```go
type [Service]Audit struct {
    // [Audit record structure]
}
```

### 5. Dependencies Summary

**Upstream Services**:
- **[Service Name]** - [Description of interaction]

**Downstream Services**:
- **[Service Name]** - [Description of interaction]

**External Services**:
- **[Service Name]** - HTTP [GET/POST] for [purpose]
- **Data Storage Service** - HTTP POST for audit trail persistence

**Database**:
- PostgreSQL - `[service]_audit` table for long-term audit storage
- Vector DB (optional) - [Description if applicable]

**Existing Code to Leverage** [CONDITIONAL]:
- [List of existing code and reusability]

---

## RBAC Configuration [REQUIRED]

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: [crdname]-controller
rules:
- apiGroups: ["kubernaut.io"]
  resources: ["[crds]", "[crds]/status"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["kubernaut.io"]
  resources: ["[crds]/finalizers"]
  verbs: ["update"]
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
# [Additional RBAC rules as needed]
```

**Note**: [Important notes about RBAC permissions]

---

## Implementation Checklist [REQUIRED]

**Note**: Follow APDC-TDD phases for each implementation step (see Development Methodology section)

### Phase 1: ANALYSIS & [Phase Name] ([X days/weeks]) [RED Phase Preparation]

- [ ] **ANALYSIS**: Search existing implementations (`codebase_search "[ComponentType]"`)
- [ ] **ANALYSIS**: Map business requirements (BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX)
- [ ] **ANALYSIS**: Identify integration points in cmd/{service_name}/
- [ ] **[Task-Specific] RED**: Write tests for [capability] (should fail initially)
- [ ] **[Task-Specific] GREEN**: Implement [capability] to pass tests
- [ ] **[Task-Specific] REFACTOR**: Enhance [capability] with sophisticated logic

### Phase 2: [Phase Name] ([X days/weeks]) [RED-GREEN-REFACTOR]

- [ ] **[Component] RED**: Write controller reconciliation tests (should fail - no controller yet)
- [ ] **[Component] GREEN**: Generate CRD schema + controller skeleton (tests pass)
- [ ] **[Component] REFACTOR**: Enhance controller with phase logic and error handling
- [ ] **Integration RED**: Write tests for [integration pattern] (fail initially)
- [ ] **Integration GREEN**: Implement [integration pattern] (tests pass)
- [ ] **Main App Integration**: Verify component instantiated in cmd/{service_name}/ (MANDATORY)

### Phase 3: [Phase Name] ([X days/weeks]) [RED-GREEN-REFACTOR]

- [ ] **Logic RED**: Write tests for [business logic] with mocked externals (fail)
- [ ] **Logic GREEN**: Integrate business logic to pass tests
- [ ] **Logic REFACTOR**: Enhance with sophisticated algorithms
- [ ] **[Additional task]**: [Description]
- [ ] **Main App Integration**: Confirm component usage in cmd/{service_name}/ (MANDATORY)

### Phase 4: Testing & Validation ([X days/weeks]) [CHECK Phase]

- [ ] **CHECK**: Verify 70%+ unit test coverage (test/unit/{service_name}/)
- [ ] **CHECK**: Run integration tests - 20% coverage target (test/integration/{service_name}/)
- [ ] **CHECK**: Execute E2E tests for critical workflows (test/e2e/{service_name}/)
- [ ] **CHECK**: Validate business requirement coverage (all BR-[CATEGORY]-XXX)
- [ ] **CHECK**: Provide confidence assessment (60-100% with justification)

---

## Critical Architectural Patterns [REQUIRED]

### 1. [Pattern Name]
**Pattern**: [Pattern description]

```go
// [Code example]
```

**Purpose**: [Why this pattern is important]

### 2. [Pattern Name]
**Pattern**: [Pattern description]

```go
// [Code example]
```

**Purpose**: [Why this pattern is important]

[Repeat for all critical patterns from MULTI_CRD_RECONCILIATION_ARCHITECTURE.md]

---

## Common Pitfalls [REQUIRED]

1. **[Pitfall 1]** - [Solution/mitigation]
2. **[Pitfall 2]** - [Solution/mitigation]
3. **[Pitfall 3]** - [Solution/mitigation]
4. **[Pitfall 4]** - [Solution/mitigation]
5. **[Pitfall 5]** - [Solution/mitigation]
6. **[Pitfall 6]** - [Solution/mitigation]
7. **[Pitfall 7]** - [Solution/mitigation]
8. **[Pitfall 8]** - [Solution/mitigation]

---

## Summary [REQUIRED]

**[Service Name] - V1 Design Specification ([X]% Complete)**

### Core Purpose
[1-2 sentence summary of service purpose]

### Key Architectural Decisions
1. **[Decision 1]** - [Brief description and rationale]
2. **[Decision 2]** - [Brief description and rationale]
3. **[Decision 3]** - [Brief description and rationale]
4. **[Decision 4]** - [Brief description and rationale]
5. **[Decision 5]** - [Brief description and rationale]

### Integration Model
```
[Upstream Service] → [CRD] (this service)
                      ↓
        [CRD].status.phase = "completed"
                      ↓
    [Downstream integration pattern]
```

### V1 Scope Boundaries
**Included**:
- [Scope item 1]
- [Scope item 2]
- [Scope item 3]

**Excluded** (V2):
- [Future item 1]
- [Future item 2]
- [Future item 3]

### Business Requirements Coverage
- **BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX**: [Coverage description]
- **BR-[CATEGORY]-XXX to BR-[CATEGORY]-XXX**: [Coverage description]
- **BR-[CATEGORY]-XXX**: [Coverage description]

### Implementation Status
- **Existing Code**: [Description of reusable code]
- **Migration Effort**: [X days/weeks]
- **CRD Controller**: [New/Migrated] implementation
- **Database Schema**: [Status]

### Next Steps
1. ✅ **[Step 1]** ([status]%)
2. **[Step 2]**: [Description]
3. **[Step 3]**: [Description]
4. **[Step 4]**: [Description]
5. **[Step 5]**: [Description]

### Critical Success Factors
- [Success factor 1]
- [Success factor 2]
- [Success factor 3]
- [Success factor 4]
- [Success factor 5]

**Design Specification Status**: [Status] ([X]% Confidence)

---

## TEMPLATE COMPLETION CHECKLIST

Before finalizing the service specification, verify:

- [ ] All `[PLACEHOLDER]` text replaced with service-specific information
- [ ] All Business Requirements mapped and validated
- [ ] CRD schema matches design document in `docs/design/CRD/`
- [ ] Controller implementation follows MULTI_CRD_RECONCILIATION_ARCHITECTURE patterns
- [ ] Prometheus metrics follow naming conventions
- [ ] Testing strategy includes unit (70%+), integration (20%), and E2E (10%) tests
- [ ] RBAC permissions documented and validated
- [ ] Integration points with upstream/downstream services documented
- [ ] Common pitfalls from similar services documented
- [ ] Implementation checklist is realistic and time-bound
- [ ] Summary provides clear overview of service purpose and scope
- [ ] Optional sections removed if not applicable
- [ ] All code examples compile and follow Go best practices
- [ ] All references to other documents are valid links

---

**Template Version**: 1.1
**Last Updated**: November 2025
**Maintained By**: Kubernaut Architecture Team

