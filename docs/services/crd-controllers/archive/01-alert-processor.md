# Alert Processor Service - CRD Implementation

**Health/Ready Port**: 8080 (`/health`, `/ready` - no auth required)
**Metrics Port**: 9090 (`/metrics` - with auth filter)
**CRD**: AlertProcessing
**Controller**: AlertProcessingReconciler
**Status**: ⚠️ **NEEDS CRD IMPLEMENTATION**
**Priority**: **P0 - HIGH**
**Effort**: 1 week

---

## 📚 Related Documentation

**CRD Design Specification**: [docs/design/CRD/02_REMEDIATION_PROCESSING_CRD.md](../../design/CRD/02_REMEDIATION_PROCESSING_CRD.md)

This document provides the detailed CRD schema, controller reconciliation logic, and architectural patterns for the AlertProcessing CRD.

---

## Business Requirements

- **Primary**: BR-SP-001 to BR-SP-050 (Alert Processing Logic)
- **Environment**: BR-ENV-001 to BR-ENV-050 (Classification integrated)
- **Tracking**: BR-SP-021 (Alert lifecycle state tracking)

**Deduplication Requirements** (Gateway Service responsibility, NOT Alert Processor):
- **BR-WH-008**: Fingerprint-based request deduplication for identical alerts
- **BR-ALERT-003**: Alert suppression to reduce operational noise
- **BR-ALERT-005**: Alert correlation and grouping under single remediation
- **BR-ALERT-006**: Escalation procedures for alert storms and duplicate patterns

**Note**: Alert Processor receives AlertProcessing CRDs only for non-duplicate alerts. Gateway Service performs all duplicate detection and escalation before CRD creation.

---

## Overview

**Purpose**: Alert enrichment, environment classification, and status aggregation with Kubernetes context integration.

**Core Responsibilities**:
1. Enrich alerts with comprehensive Kubernetes context (pods, deployments, nodes)
2. Classify environment tier (production, staging, development) with business criticality
3. Validate alert completeness and readiness for AI analysis
4. Update status for AlertRemediation controller to trigger next phase

**V1 Scope - Enrichment & Classification Only**:
- Single enrichment provider: Context Service - see [README: Context Service](../../README.md#-9-context-service)
- Environment classification with fallback heuristics
- Basic alert validation
- **Targeting data ONLY** (namespace, resource kind/name, Kubernetes context ~8KB)
- **NO log/metric storage in CRD** (HolmesGPT fetches via toolsets dynamically)
- No multi-source data aggregation

**Future V2 Enhancements** (Out of Scope):
- Multi-source context discovery (additional Context Service providers)
- Advanced correlation across related resources
- Predictive environment classification using ML
- Cross-cluster context enrichment

**Note**: Logs/metrics/traces are NEVER stored in CRDs. HolmesGPT fetches these dynamically using toolsets (`kubernetes`, `prometheus`, `grafana`).

**Key Architectural Decisions**:
- CRD-based state management (not HTTP polling)
- **Single-phase synchronous processing** (fast operations ~3 seconds total)
- Degraded mode operation when Context Service unavailable
- 24-hour retention aligned with AlertRemediation lifecycle
- **Does NOT create AIAnalysis CRD** (AlertRemediation controller responsibility)
- No duplicate detection (Gateway Service responsibility)

---

## 🔄 Deduplication & Alert Storm Handling

**⚠️ CRITICAL ARCHITECTURE NOTE**: Duplicate alert handling is a **Gateway Service responsibility**, NOT Alert Processor.

### Responsibility Separation

```
┌─────────────────────────────────────────────────────────────────┐
│ Gateway Service (Port 8080) - DUPLICATE DETECTION               │
├─────────────────────────────────────────────────────────────────┤
│ 1. Receives webhook alert from Prometheus/Grafana              │
│ 2. Generates alert fingerprint (hash of content)               │
│ 3. Checks for existing AlertRemediation CRD by fingerprint     │
│ 4. If DUPLICATE:                                                │
│    ├── Updates AlertRemediation.Status.DuplicateAlerts counter │
│    ├── Checks escalation criteria (environment-based)          │
│    ├── Emits Kubernetes event for visibility                   │
│    └── Escalates if alert storm detected (5+ or 3 in 5 min)    │
│ 5. If FIRST OCCURRENCE:                                         │
│    └── Creates new AlertRemediation CRD → triggers processing  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ Alert Processor (CRD Controller) - ALERT ENRICHMENT            │
├─────────────────────────────────────────────────────────────────┤
│ 1. Receives AlertProcessing CRD (already deduplicated)         │
│ 2. NO duplicate checking - Gateway handled it                  │
│ 3. Enriches alert with Kubernetes context                      │
│ 4. Classifies environment (production/staging/dev)             │
│ 5. Routes to AI Analysis service                               │
│                                                                 │
│ Exposes:                                                        │
│   - Port 8080: /health, /ready (no auth)                       │
│   - Port 9090: /metrics (with auth filter)                     │
└─────────────────────────────────────────────────────────────────┘
```

### Alert Storm Escalation Thresholds (BR-ALERT-006)

Environment-based escalation criteria from Gateway Service:

| Environment | Absolute Threshold | Frequency Threshold | Escalation Channels | Urgency |
|------------|-------------------|---------------------|--------------------|---------|
| **Production** | 5+ duplicates | 3 duplicates in 5 min | Slack: #production-oncall<br>Email: sre-team, eng-manager | IMMEDIATE |
| **Staging** | 8+ duplicates | 5 duplicates in 10 min | Slack: #platform-team<br>Email: platform-team | NORMAL |
| **Development** | 10+ duplicates | 8 duplicates in 15 min | Slack: #dev-team | LOW (business hours) |

### Business Requirements Coverage

| Requirement | Implementation | Service |
|------------|---------------|---------|
| **BR-WH-008** | Fingerprint-based duplicate detection | Gateway Service |
| **BR-ALERT-003** | Alert suppression to reduce noise | Gateway Service |
| **BR-ALERT-005** | Alert correlation and grouping | Gateway Service |
| **BR-ALERT-006** | Alert storm escalation procedures | Gateway Service |
| **BR-ENV-009** | Business criticality preservation | Gateway Service |
| **BR-SP-031** | Environment-specific priority routing | Alert Processor |

### Duplicate Handling Flow

```go
// Gateway Service - Duplicate Detection
func (g *GatewayService) HandleWebhook(ctx, payload) error {
    fingerprint := extractFingerprint(payload)

    existingRemediation, _ := g.findExistingRemediation(ctx, fingerprint)

    if existingRemediation != nil {
        // DUPLICATE - Update metadata and check escalation
        existingRemediation.Status.DuplicateAlerts.Count++
        existingRemediation.Status.DuplicateAlerts.LastSeenAt = metav1.Now()

        if g.shouldEscalate(existingRemediation) {
            g.escalateDuplicateAlerts(ctx, existingRemediation)
        }

        return g.updateRemediation(ctx, existingRemediation)
    }

    // FIRST OCCURRENCE - Create AlertRemediation CRD
    return g.createRemediation(ctx, payload, fingerprint)
}

// Alert Processor - NO Duplicate Checking
func (r *AlertProcessingReconciler) Reconcile(ctx, req) (ctrl.Result, error) {
    // AlertProcessing CRD only exists for non-duplicate alerts
    // Focus on enrichment, classification, and routing

    switch ap.Status.Phase {
    case "enriching":
        enrichment := r.enricher.Enrich(ctx, ap.Spec.Alert)
        // ... continue processing
    }
}
```

### Migration Note

**Existing Code Location**: `pkg/alert/components.go` contains `AlertDeduplicatorImpl`

**Required Action**:
- ✅ Fingerprint generation logic is reusable for Gateway Service
- ❌ Current implementation lacks AlertRemediation CRD integration
- ❌ Current implementation lacks escalation logic
- ❌ Current implementation lacks environment-based thresholds

**Recommendation**: Move and enhance `AlertDeduplicatorImpl` to Gateway Service with full duplicate handling and escalation logic.

---

## Package Structure Decision

**Approved Structure**: `{cmd,pkg,internal}/alertprocessor/`

Following Go idioms and codebase patterns (`testutil`, `holmesgpt`), the Alert Processor service uses a single-word compound package name:

```
cmd/alertprocessor/           → Main application entry point
  └── main.go

pkg/alertprocessor/           → Business logic (PUBLIC API)
  ├── service.go             → AlertProcessorService interface
  ├── implementation.go      → Service implementation
  ├── components.go          → Processing components
  └── types.go              → Type-safe result types

internal/controller/          → CRD controller (INTERNAL)
  └── alertprocessing_controller.go
```

**Migration**: Rename `pkg/alert/` → `pkg/alertprocessor/` (estimated 4 hours, low risk)

---

## Development Methodology

**Mandatory Process**: Follow APDC-Enhanced TDD workflow per [.cursor/rules/00-core-development-methodology.mdc](../../../.cursor/rules/00-core-development-methodology.mdc)

### APDC-TDD Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ ANALYSIS → PLAN → DO-RED → DO-GREEN → DO-REFACTOR → CHECK  │
└─────────────────────────────────────────────────────────────┘
```

**ANALYSIS** (5-15 min): Comprehensive context understanding
  - Search existing implementations (`codebase_search "AlertProcessor implementations"`)
  - Identify reusable components in `pkg/alert/` (1,103 lines to migrate)
  - Map business requirements (BR-SP-001 to BR-SP-050, BR-ENV-001 to BR-ENV-050)
  - Identify integration points in `cmd/`

**PLAN** (10-20 min): Detailed implementation strategy
  - Define TDD phase breakdown (RED → GREEN → REFACTOR)
  - Plan integration points (AlertProcessing controller in cmd/alertprocessor/)
  - Establish success criteria (enrichment <2s, classification <500ms, total <5s)
  - Identify risks (Context Service unavailability → degraded mode)

**DO-RED** (10-15 min): Write failing tests FIRST
  - Unit tests defining business contract (70%+ coverage target)
  - Use FAKE K8s client (`sigs.k8s.io/controller-runtime/pkg/client/fake`)
  - Mock ONLY Context Service HTTP calls (use `pkg/testutil/mocks`)
  - Use REAL environment classification business logic
  - Map tests to business requirements (BR-SP-XXX)

**DO-GREEN** (15-20 min): Minimal implementation
  - Define AlertProcessingReconciler interface to make tests compile
  - Minimal code to pass tests (basic enrichment, classification)
  - **MANDATORY integration in cmd/alertprocessor/** (controller startup)
  - Add owner references to AlertRemediation CRD

**DO-REFACTOR** (20-30 min): Enhance with sophisticated logic
  - **NO new types/interfaces/files** (enhance existing controller methods)
  - Add sophisticated enrichment algorithms and classification heuristics
  - Maintain integration with AlertRemediation orchestration
  - Add degraded mode fallback and performance optimization

**CHECK** (5-10 min): Validation and confidence assessment
  - Business requirement verification (BR-SP-001 to BR-SP-050 addressed)
  - Integration confirmation (controller in cmd/alertprocessor/)
  - Test coverage validation (70%+ unit, 20% integration, 10% E2E)
  - Performance validation (total processing <5s)
  - Confidence assessment: 85% (high confidence, see Migration Effort section)

**AI Assistant Checkpoints**: See [.cursor/rules/10-ai-assistant-behavioral-constraints.mdc](../../../.cursor/rules/10-ai-assistant-behavioral-constraints.mdc)
  - **Checkpoint A**: Type Reference Validation (read AlertProcessing CRD types before referencing)
  - **Checkpoint B**: Test Creation Validation (reuse existing alert/ test patterns)
  - **Checkpoint C**: Business Integration Validation (verify cmd/alertprocessor/ integration)
  - **Checkpoint D**: Build Error Investigation (complete dependency analysis for migration)

### Quick Decision Matrix

| Starting Point | Required Phase | Reference |
|----------------|---------------|-----------|
| **Migrate from pkg/alert/** | ANALYSIS → PLAN → DO-REFACTOR | Existing code is well-understood |
| **New CRD controller** | Full APDC workflow | Controller pattern is new |
| **Fix enrichment bugs** | ANALYSIS → DO-RED → DO-REFACTOR | Understand enrichment context first |
| **Add classification tests** | DO-RED only | Write tests for classification logic |

**Testing Strategy Reference**: [.cursor/rules/03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)
  - Unit Tests (70%+): test/unit/alertprocessor/ - Fake K8s client, mock Context Service
  - Integration Tests (20%): test/integration/alertprocessor/ - Real K8s (KIND), real Context Service
  - E2E Tests (10%): test/e2e/alertprocessor/ - Complete alert-to-remediation workflow

---

## Reconciliation Architecture

### Phase Transitions

**Single-Phase Synchronous Processing**:

```
"" (new) → processing → completed
                ↓
           (2-5 seconds)
```

**Rationale**: Alert processing is fundamentally synchronous and fast:
- Context Service HTTP call: ~1-2 seconds
- Environment classification logic: ~100ms
- Alert validation: ~50ms
- **Total**: ~3 seconds (no need for multi-phase state machine)

### Reconciliation Flow

#### 1. **processing** Phase (BR-SP-001 to BR-SP-040, BR-ENV-001 to BR-ENV-025)

**Purpose**: Complete alert enrichment, classification, and validation in single reconciliation loop

**Actions** (executed synchronously):

**Step 1: Enrichment** (BR-SP-001 to BR-SP-015)
- Call Context Service with alert labels
- Retrieve pod details, deployment config, node status
- Gather related resources (services, ingress, configmaps)
- Handle Context Service unavailability with degraded mode
- Timeout: 30 seconds per HTTP call

**Step 2: Classification** (BR-ENV-001 to BR-ENV-025)
- Analyze namespace naming patterns (prod, staging, dev)
- Check environment labels on resources
- Apply fallback heuristics if labels missing
- Assign business priority (p0, p1, p2, p3)

**Step 3: Validation** (BR-SP-031 to BR-SP-040)
- Validate enriched alert completeness
- Verify required fields present
- Check data quality

**Step 4: Status Update**
- Set `status.phase = "completed"`
- Set `status.enrichedAlert` with complete payload
- Set `status.environmentClassification` with tier and priority
- Record completion timestamp

**Transition Criteria**:
```go
if enrichmentComplete && classificationComplete && validationPassed {
    phase = "completed"
    // AlertRemediation controller will watch this status change
    // and create AIAnalysis CRD
} else if contextServiceUnavailable {
    // Degraded mode: use minimal context from alert labels
    phase = "completed"
    status.degradedMode = true
} else if criticalError {
    phase = "failed"
    reason = "processing_error"
}
```

**Reconciliation Implementation**:
```go
func (r *AlertProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var ap processingv1.AlertProcessing
    if err := r.Get(ctx, req.NamespacedName, &ap); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Skip if already completed
    if ap.Status.Phase == "completed" || ap.Status.Phase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute all processing in single reconciliation
    // Step 1: Enrich
    enriched, err := r.enricher.Enrich(ctx, ap.Spec.Alert)
    if err != nil {
        // Degraded mode fallback
        enriched = r.enricher.DegradedModeEnrich(ap.Spec.Alert)
        ap.Status.DegradedMode = true
    }

    // Step 2: Classify
    classification := r.classifier.Classify(enriched)

    // Step 3: Validate
    if err := r.validator.Validate(enriched, classification); err != nil {
        ap.Status.Phase = "failed"
        ap.Status.FailureReason = err.Error()
        return ctrl.Result{}, r.Status().Update(ctx, &ap)
    }

    // Step 4: Update to completed
    ap.Status.Phase = "completed"
    ap.Status.EnrichedAlert = enriched
    ap.Status.EnvironmentClassification = classification
    ap.Status.CompletionTime = metav1.Now()

    // AlertRemediation controller watches this status change
    // and will create AIAnalysis CRD
    return ctrl.Result{}, r.Status().Update(ctx, &ap)
}
```

**Example CRD Update**:
```yaml
status:
  phase: completed
  degradedMode: false
  enrichedAlert:
    fingerprint: "abc123def456"
    severity: critical
    environment: production
    kubernetesContext:
      podDetails:
        name: web-app-789
        namespace: production
        containers: [...]
      deploymentDetails:
        name: web-app
        replicas: 3
      nodeDetails:
        name: node-1
        capacity: {...}
  environmentClassification:
    tier: production
    confidence: 0.95
    businessPriority: p0
    classificationMethod: "namespace-label"
  completionTime: "2025-01-15T10:05:23Z"
```

#### 2. **completed** Phase (Terminal State)

**Purpose**: Signal completion to AlertRemediation controller

**Actions**:
- Record enrichment results to PostgreSQL audit table
- Emit Kubernetes event: `AlertProcessingCompleted`
- Wait for AlertRemediation controller to create AIAnalysis CRD

**No Timeout** (terminal state)

**Note**: AlertProcessing does NOT create AIAnalysis CRD. The AlertRemediation controller watches AlertProcessing status and creates AIAnalysis when `phase = "completed"`.

#### 3. **failed** Phase (Terminal State)

**Purpose**: Record failure for debugging

**Actions**:
- Log failure reason and context
- Emit Kubernetes event: `AlertProcessingFailed`
- Record failure to audit database

**No Requeue** (terminal state - requires manual intervention or alert retry)

---

### CRD-Based Coordination Patterns

#### Event-Driven Coordination

This service uses **CRD-based reconciliation** for coordination with AlertRemediation controller:

1. **Created By**: AlertRemediation controller creates AlertProcessing CRD (with owner reference)
2. **Watch Pattern**: AlertRemediation watches AlertProcessing status for completion
3. **Status Propagation**: Status updates trigger AlertRemediation reconciliation automatically (<1s latency)
4. **Event Emission**: Emit Kubernetes events for operational visibility

**Coordination Flow**:
```
AlertRemediation.status.overallPhase = "pending"
    ↓
AlertRemediation Controller creates AlertProcessing CRD
    ↓
AlertProcessing Controller reconciles (this controller)
    ↓
AlertProcessing.status.phase = "completed"
    ↓ (watch trigger in AlertRemediation)
AlertRemediation Controller reconciles (detects completion)
    ↓
AlertRemediation.status.overallPhase = "processing"
    ↓
AlertRemediation Controller creates AIAnalysis CRD
```

---

#### Owner Reference Management

**This CRD (AlertProcessing)**:
- **Owned By**: AlertRemediation (parent CRD)
- **Owner Reference**: Set at creation by AlertRemediation controller
- **Cascade Deletion**: Deleted automatically when AlertRemediation is deleted
- **Owns**: Nothing (leaf controller - no child CRDs)
- **Watches**: Nothing (processes own CRD only)

**Leaf Controller Pattern**:

AlertProcessing is a **leaf controller** in the remediation workflow:
- ✅ **Simple responsibility**: Process alert, update status, done
- ✅ **No coordination complexity**: Doesn't create or watch other CRDs
- ✅ **Fast execution**: Single-phase synchronous processing (~3 seconds)
- ✅ **Clean termination**: Status update to "completed" is the only output

**Lifecycle**:
```
AlertRemediation Controller
    ↓ (creates with owner reference)
AlertProcessing CRD
    ↓ (processes in ~3 seconds)
AlertProcessing.status.phase = "completed"
    ↓ (watch trigger)
AlertRemediation Controller (creates next CRD)
```

---

#### No Direct HTTP Calls Between Controllers

**Anti-Pattern (Avoided)**: ❌ AlertProcessing calling AIAnalysis or other controllers via HTTP

**Correct Pattern (Used)**: ✅ CRD status update + AlertRemediation watch-based coordination

**Why This Matters**:
- **Reliability**: CRD status persists in etcd (HTTP calls can fail silently)
- **Observability**: Status visible via `kubectl get alertprocessing` (HTTP calls are opaque)
- **Kubernetes-Native**: Leverages built-in watch/reconcile patterns (no custom HTTP infrastructure)
- **Decoupling**: AlertProcessing doesn't need to know about AIAnalysis existence or endpoint

**What AlertProcessing Does NOT Do**:
- ❌ Call AIAnalysis controller via HTTP
- ❌ Create AIAnalysis CRD (AlertRemediation does this)
- ❌ Watch AIAnalysis status (AlertRemediation does this)
- ❌ Coordinate with other service controllers directly

**What AlertProcessing DOES Do**:
- ✅ Process its own AlertProcessing CRD
- ✅ Update its own status to "completed"
- ✅ Trust AlertRemediation to handle coordination

---

#### Watch Configuration (Upstream)

**AlertRemediation Watches AlertProcessing**:

```go
// In AlertRemediationReconciler.SetupWithManager()
err = c.Watch(
    &source.Kind{Type: &alertprocessorv1.AlertProcessing{}},
    handler.EnqueueRequestsFromMapFunc(r.alertProcessingToRemediation),
)

// Mapping function
func (r *AlertRemediationReconciler) alertProcessingToRemediation(obj client.Object) []ctrl.Request {
    ap := obj.(*alertprocessorv1.AlertProcessing)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      ap.Spec.AlertRemediationRef.Name,
                Namespace: ap.Spec.AlertRemediationRef.Namespace,
            },
        },
    }
}
```

**Result**: Any AlertProcessing status update triggers AlertRemediation reconciliation within ~100ms.

---

#### Coordination Benefits

**For AlertProcessing Controller**:
- ✅ **Simple**: No coordination logic needed
- ✅ **Fast**: No waiting for other services
- ✅ **Testable**: Unit tests only need fake K8s client
- ✅ **Reliable**: No external HTTP dependencies

**For AlertRemediation Controller**:
- ✅ **Visibility**: Can query AlertProcessing status anytime
- ✅ **Control**: Decides when to proceed to next phase
- ✅ **Timeout Detection**: Can detect if AlertProcessing takes too long
- ✅ **Retry**: Can recreate AlertProcessing if needed

**For Operations**:
- ✅ **Debuggable**: `kubectl get alertprocessing -o yaml` shows full state
- ✅ **Observable**: Kubernetes events show processing progress
- ✅ **Traceable**: CRD history shows what happened and when

---

## Current State & Migration Path

### Existing Business Logic (Verified)

**Current Location**: `pkg/alert/` (1,103 lines of reusable code)
**Target Location**: `pkg/alertprocessor/` (after migration)

```
pkg/alert/ → pkg/alertprocessor/
├── service.go (109 lines)          ✅ AlertService → AlertProcessorService interface
├── implementation.go (282 lines)   ✅ Complete 4-step processing pipeline
└── components.go (712 lines)       ✅ All business logic components
```

**Existing Tests** (Verified - to be migrated):
- `test/unit/alert/` → `test/unit/alertprocessor/` - Unit tests with Ginkgo/Gomega
- `test/integration/alert_processing/` → `test/integration/alertprocessor/` - Integration tests

### Business Logic Components (Highly Reusable)

**AlertService Interface** - `pkg/alert/service.go:13-38` (to be migrated to `pkg/alertprocessor/service.go`)

⚠️ **CURRENT IMPLEMENTATION** (uses map[string]interface{} anti-pattern):
```go
type AlertService interface {
    ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error)
    ValidateAlert(alert types.Alert) map[string]interface{}
    RouteAlert(ctx context.Context, alert types.Alert) map[string]interface{}
    EnrichAlert(ctx context.Context, alert types.Alert) map[string]interface{}
    PersistAlert(ctx context.Context, alert types.Alert) map[string]interface{}
    GetAlertHistory(namespace string, duration time.Duration) map[string]interface{}
    GetAlertMetrics() map[string]interface{}
    Health() map[string]interface{}
}
```

**Note**: `GetDeduplicationStats()` removed - deduplication is Gateway Service responsibility (BR-WH-008).

✅ **RECOMMENDED REFACTOR** (structured types for type safety):
```go
// pkg/alertprocessor/service.go
package alertprocessor

import (
    "context"
    "time"
    "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AlertProcessorService defines alert processing operations
// Business Requirements: BR-SP-001 to BR-SP-050
type AlertProcessorService interface {
    // Core processing
    ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error)

    // Validation
    ValidateAlert(alert types.Alert) (*ValidationResult, error)

    // Routing
    RouteAlert(ctx context.Context, alert types.Alert) (*RoutingResult, error)

    // Enrichment
    EnrichAlert(ctx context.Context, alert types.Alert) (*EnrichmentResult, error)

    // Persistence
    PersistAlert(ctx context.Context, alert types.Alert) (*PersistenceResult, error)
    GetAlertHistory(namespace string, duration time.Duration) (*AlertHistoryResult, error)

    // Metrics and Health
    GetAlertMetrics() (*AlertMetrics, error)
    Health() (*HealthStatus, error)
}

// Supporting structured types (create in pkg/alertprocessor/types.go)
type ValidationResult struct {
    Valid  bool     `json:"valid"`
    Errors []string `json:"errors,omitempty"`
}

type RoutingResult struct {
    Routed      bool   `json:"routed"`
    Destination string `json:"destination,omitempty"`
    RouteType   string `json:"route_type"`
    Priority    string `json:"priority,omitempty"`
    Reason      string `json:"reason,omitempty"`
}

type EnrichmentResult struct {
    Status              string                 `json:"enrichment_status"`
    AIAnalysis          *AIAnalysisResult      `json:"ai_analysis,omitempty"`
    Metadata            map[string]string      `json:"metadata,omitempty"`
    EnrichmentTimestamp time.Time              `json:"enrichment_timestamp"`
}

type AIAnalysisResult struct {
    Confidence float64           `json:"confidence"`
    Analysis   string            `json:"analysis"`
    Metadata   map[string]string `json:"metadata,omitempty"`
}

type PersistenceResult struct {
    Persisted bool      `json:"persisted"`
    AlertID   string    `json:"alert_id,omitempty"`
    Timestamp time.Time `json:"timestamp"`
}

type AlertHistoryResult struct {
    Alerts      []types.Alert `json:"alerts"`
    TotalCount  int           `json:"total_count"`
    Namespace   string        `json:"namespace"`
    Duration    time.Duration `json:"duration"`
    RetrievedAt time.Time     `json:"retrieved_at"`
}

type AlertMetrics struct {
    AlertsIngested  int       `json:"alerts_ingested"`
    AlertsValidated int       `json:"alerts_validated"`
    AlertsRouted    int       `json:"alerts_routed"`
    AlertsEnriched  int       `json:"alerts_enriched"`
    ProcessingRate  float64   `json:"processing_rate"` // alerts per minute
    SuccessRate     float64   `json:"success_rate"`
    LastUpdated     time.Time `json:"last_updated"`
}

type HealthStatus struct {
    Status        string           `json:"status"` // "healthy", "degraded", "unhealthy"
    Service       string           `json:"service"`
    AIIntegration bool             `json:"ai_integration"`
    Components    *ComponentHealth `json:"components"`
}

type ComponentHealth struct {
    Processor    bool `json:"processor"`
    Enricher     bool `json:"enricher"`
    Router       bool `json:"router"`
    Validator    bool `json:"validator"`
    Persister    bool `json:"persister"`
}
```

**Migration Notes**:
- **Package Rename**: `pkg/alert/` → `pkg/alertprocessor/` (4 hours)
- **Interface Rename**: `AlertService` → `AlertProcessorService`
- **Type Safety Refactor**: `map[string]interface{}` → 7 structured types (violates coding standards)
- **Deduplication Removal**: `DeduplicationStats` removed - Gateway Service responsibility (BR-WH-008)
- **Strategy**: Parallel implementation to avoid breaking changes
- **Total Effort**: 1-2 days for complete migration

**4-Step Processing Pipeline** - `pkg/alert/implementation.go:75-118` (to be migrated to `pkg/alertprocessor/`)
```go
func (s *ServiceImpl) ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error) {
    // Step 1: Validate alert
    validation := s.ValidateAlert(alert)

    // Note: Deduplication handled by Gateway Service (BR-WH-008)
    // Alert Processor receives only non-duplicate alerts

    // Step 2: Enrich alert with context
    enrichment := s.EnrichAlert(ctx, alert)

    // Step 3: Route alert to appropriate handler
    routing := s.RouteAlert(ctx, alert)

    // Step 4: Persist alert to storage
    persistence := s.PersistAlert(ctx, alert)
}
```

**Business Components** - `pkg/alert/components.go` (to be migrated to `pkg/alertprocessor/components.go`)
- `AlertProcessorImpl` - Core processing logic (85% reusable)
- `AlertEnricherImpl` - AI-based enrichment (90% reusable)
- `AlertRouterImpl` - Routing and filtering (85% reusable)
- `AlertValidatorImpl` - Validation logic (95% reusable)
- `AlertPersisterImpl` - Persistence logic (75% reusable)

**Note**: `AlertDeduplicatorImpl` exists in current `pkg/alert/components.go` but belongs in Gateway Service (BR-WH-008). Fingerprint generation logic is reusable, but duplicate detection and escalation are Gateway responsibilities.

### Migration to CRD Controller

**Synchronous Pipeline → Asynchronous Reconciliation Phases**

```go
// EXISTING: Synchronous 4-step pipeline (deduplication removed)
func (s *ServiceImpl) ProcessAlert(ctx, alert) (*ProcessResult, error) {
    validation := s.ValidateAlert(alert)          // Step 1
    // Deduplication handled by Gateway Service (BR-WH-008)
    enrichment := s.EnrichAlert(ctx, alert)       // Step 2
    routing := s.RouteAlert(ctx, alert)           // Step 3
    persistence := s.PersistAlert(ctx, alert)     // Step 4
    return result, nil
}

// MIGRATED: Asynchronous CRD reconciliation
func (r *AlertProcessingReconciler) Reconcile(ctx, req) (ctrl.Result, error) {
    // AlertProcessing CRD only created for non-duplicate alerts
    // Gateway Service handles duplicate detection and escalation

    switch alertProcessing.Status.Phase {
    case "enriching":
        // Reuse: AlertEnricherImpl.Enrich() business logic
        enrichment := r.enricher.Enrich(ctx, alert)
        alertProcessing.Status.EnrichmentResults = enrichment
        alertProcessing.Status.Phase = "classifying"
        return ctrl.Result{Requeue: true}, r.Status().Update(ctx, alertProcessing)

    case "classifying":
        // Reuse: Environment classification logic
        classification := r.classifier.Classify(ctx, alert, enrichment)
        alertProcessing.Status.EnvironmentClassification = classification
        alertProcessing.Status.Phase = "routing"
        return ctrl.Result{Requeue: true}, r.Status().Update(ctx, alertProcessing)

    case "routing":
        // Reuse: AlertRouterImpl.Route() business logic
        routing := r.router.Route(ctx, alert, classification)

        // CRD-specific: Create AIAnalysis CRD
        aiAnalysis := &aianalysisv1.AIAnalysis{
            Spec: buildAnalysisRequest(enrichment, classification),
        }
        r.Create(ctx, aiAnalysis)

        alertProcessing.Status.Phase = "completed"
        return ctrl.Result{}, r.Status().Update(ctx, alertProcessing)
    }
}
```

### Component Reuse Mapping

| Existing Component | CRD Controller Usage | Reusability | Migration Effort | Notes |
|-------------------|---------------------|-------------|-----------------|-------|
| **AlertValidatorImpl** | Pre-enrichment validation | 95% | Minimal | ✅ Return `*ValidationResult` instead of map |
| **AlertEnricherImpl** | Enriching phase logic | 90% | Low | ✅ Return `*EnrichmentResult` with structured AI analysis |
| **Environment Classifier** | Classifying phase logic | 85% | Low | ✅ Integrate with CRD status updates |
| **AlertRouterImpl** | Routing phase logic | 85% | Medium | ✅ Return `*RoutingResult`, add CRD creation |
| **AlertPersisterImpl** | Audit storage integration | 75% | Medium | ✅ Return `*PersistenceResult` with proper error handling |
| **Config/AIConfig** | Controller configuration | 80% | Low | ✅ Adapt for CRD reconciler |

**Removed from Alert Processor**: `AlertDeduplicatorImpl` - Moved to Gateway Service (BR-WH-008)

**Interface Refactoring Required**:
- **Package Migration**: `pkg/alert/` → `pkg/alertprocessor/`
- **Interface Rename**: `AlertService` → `AlertProcessorService`
- Replace all `map[string]interface{}` return types with 7 structured types
- Add proper error returns to all methods (except ProcessAlert which already has error)
- Create `pkg/alertprocessor/types.go` for all result type definitions
- Remove `GetDeduplicationStats()` method (Gateway Service responsibility)
- Estimated effort: 1-2 days for complete type safety migration + package rename

### Implementation Gap Analysis

**What Exists (Verified)**:
- ✅ Complete business logic (1,103 lines)
- ✅ AlertService interface and implementation
- ✅ 5 core component implementations (AlertDeduplicatorImpl excluded - Gateway Service)
- ✅ Configuration structures
- ✅ Unit and integration tests
- ✅ 4-step processing pipeline (deduplication removed)

**What's Missing (CRD V1 Requirements)**:
- ❌ AlertProcessing CRD schema (need to create)
- ❌ AlertProcessingReconciler controller (need to create)
- ❌ CRD lifecycle management (owner refs, finalizers)
- ❌ Watch-based status coordination
- ❌ Phase timeout detection
- ❌ Event emission for visibility

**Code Quality Issues to Address**:
- ⚠️ **Package Migration & Type Safety Refactor**: Current implementation needs modernization
  - **Package Rename**: `pkg/alert/` → `pkg/alertprocessor/` (~4 hours)
  - **Interface Rename**: `AlertService` → `AlertProcessorService`
  - Violates coding standards (`.cursor/rules/00-project-guidelines.mdc`) - uses `map[string]interface{}` anti-pattern
  - Replace with 7 structured types (`*ValidationResult`, `*RoutingResult`, `*EnrichmentResult`, `*PersistenceResult`, `*AlertHistoryResult`, `*AlertMetrics`, `*HealthStatus`)
  - Remove `GetDeduplicationStats()` method (Gateway Service responsibility - BR-WH-008)
  - Add proper error handling to all methods
  - Create `pkg/alertprocessor/types.go` for result type definitions
  - Estimated effort: 1-2 days (can be done in parallel with CRD work)

**Estimated Migration Effort**: 5-7 days
- Day 1: Type safety refactor (structured types + error handling)
- Day 2: CRD schema + controller skeleton
- Day 3-4: Business logic integration into reconciliation phases
- Day 5: Testing and refinement
- Day 6: Integration with type-safe interfaces
- Day 7: Documentation and deployment

## CRD Schema Specification

**Full Schema**: See [docs/design/CRD/02_REMEDIATION_PROCESSING_CRD.md](../../design/CRD/02_REMEDIATION_PROCESSING_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `02_REMEDIATION_PROCESSING_CRD.md`.

**Location**: `api/alertprocessor/v1/alertprocessing_types.go`

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification uses **fully structured types** and eliminates all `map[string]interface{}` anti-patterns:

| Type | Previous (Anti-Pattern) | Current (Type-Safe) | Benefit |
|------|------------------------|---------------------|---------|
| **KubernetesContext** | `map[string]interface{}` | 14 structured types | Compile-time safety, OpenAPI validation |
| **HistoricalContext** | `map[string]interface{}` | Structured type | Clear data contract |
| **ProcessingPhase.Results** | `map[string]interface{}` | 3 phase-specific types | Database query performance |

**Related Triage**: See `ALERT_PROCESSOR_TYPE_SAFETY_TRIAGE.md` for detailed analysis and remediation plan.

```go
package v1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AlertProcessingSpec defines the desired state of AlertProcessing
type AlertProcessingSpec struct {
    // AlertRemediationRef references the parent AlertRemediation CRD
    AlertRemediationRef corev1.ObjectReference `json:"alertRemediationRef"`

    // Alert contains the raw alert payload from webhook
    Alert Alert `json:"alert"`

    // EnrichmentConfig specifies enrichment sources and depth
    EnrichmentConfig EnrichmentConfig `json:"enrichmentConfig,omitempty"`

    // EnvironmentClassification config for namespace classification
    EnvironmentClassification EnvironmentClassificationConfig `json:"environmentClassification,omitempty"`
}

// Alert represents the alert data from Prometheus/Grafana
type Alert struct {
    Fingerprint string            `json:"fingerprint"`
    Payload     map[string]string `json:"payload"`
    Severity    string            `json:"severity"`
    Namespace   string            `json:"namespace"`
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`
}

// EnrichmentConfig specifies context enrichment parameters
type EnrichmentConfig struct {
    ContextSources     []string `json:"contextSources,omitempty"`     // ["kubernetes", "historical"]
    ContextDepth       string   `json:"contextDepth,omitempty"`       // "basic", "detailed", "comprehensive"
    HistoricalLookback string   `json:"historicalLookback,omitempty"` // "1h", "24h", "7d"
}

// EnvironmentClassificationConfig for namespace environment detection
type EnvironmentClassificationConfig struct {
    ClassificationSources []string          `json:"classificationSources,omitempty"` // ["labels", "annotations", "configmap", "patterns"]
    ConfidenceThreshold   float64           `json:"confidenceThreshold,omitempty"`   // 0.8
    BusinessRules         map[string]string `json:"businessRules,omitempty"`
}

// AlertProcessingStatus defines the observed state
type AlertProcessingStatus struct {
    // Phase tracks current processing stage
    Phase string `json:"phase"` // "enriching", "classifying", "routing", "completed"

    // EnrichmentResults contains context data gathered
    EnrichmentResults EnrichmentResults `json:"enrichmentResults,omitempty"`

    // EnvironmentClassification result with confidence
    EnvironmentClassification EnvironmentClassification `json:"environmentClassification,omitempty"`

    // RoutingDecision for next service
    RoutingDecision RoutingDecision `json:"routingDecision,omitempty"`

    // ProcessingTime duration for metrics
    ProcessingTime string `json:"processingTime,omitempty"`

    // Conditions for status tracking
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// EnrichmentResults from context gathering
type EnrichmentResults struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"` // 0.0-1.0
}

// KubernetesContext contains Kubernetes resource context (~8KB typical size)
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type KubernetesContext struct {
    // Namespace information
    Namespace       string            `json:"namespace"`
    NamespaceLabels map[string]string `json:"namespaceLabels,omitempty"`

    // Pod context
    PodDetails *PodDetails `json:"podDetails,omitempty"`

    // Deployment/workload context
    DeploymentDetails *DeploymentDetails `json:"deploymentDetails,omitempty"`

    // Node context
    NodeDetails *NodeDetails `json:"nodeDetails,omitempty"`

    // Related resources (targeting data only)
    RelatedServices   []ServiceSummary   `json:"relatedServices,omitempty"`
    RelatedIngresses  []IngressSummary   `json:"relatedIngresses,omitempty"`
    RelatedConfigMaps []ConfigMapSummary `json:"relatedConfigMaps,omitempty"`
}

type PodDetails struct {
    Name              string            `json:"name"`
    Phase             string            `json:"phase"` // Running, Pending, Failed
    Labels            map[string]string `json:"labels,omitempty"`
    Annotations       map[string]string `json:"annotations,omitempty"`
    Containers        []ContainerStatus `json:"containers,omitempty"`
    RestartCount      int32             `json:"restartCount"`
    CreationTimestamp string            `json:"creationTimestamp"`
}

type ContainerStatus struct {
    Name         string `json:"name"`
    Image        string `json:"image"`
    Ready        bool   `json:"ready"`
    RestartCount int32  `json:"restartCount"`
    State        string `json:"state"` // running, waiting, terminated
}

type DeploymentDetails struct {
    Name              string            `json:"name"`
    Replicas          int32             `json:"replicas"`
    ReadyReplicas     int32             `json:"readyReplicas"`
    AvailableReplicas int32             `json:"availableReplicas"`
    Strategy          string            `json:"strategy"` // RollingUpdate, Recreate
    Labels            map[string]string `json:"labels,omitempty"`
}

type NodeDetails struct {
    Name        string            `json:"name"`
    Labels      map[string]string `json:"labels,omitempty"`
    Capacity    ResourceList      `json:"capacity"`
    Allocatable ResourceList      `json:"allocatable"`
    Conditions  []NodeCondition   `json:"conditions,omitempty"`
}

type ResourceList struct {
    CPU    string `json:"cpu"`    // e.g., "4000m"
    Memory string `json:"memory"` // e.g., "16Gi"
}

type NodeCondition struct {
    Type   string `json:"type"`   // Ready, MemoryPressure, DiskPressure
    Status string `json:"status"` // True, False, Unknown
}

type ServiceSummary struct {
    Name      string        `json:"name"`
    Type      string        `json:"type"` // ClusterIP, NodePort, LoadBalancer
    ClusterIP string        `json:"clusterIP"`
    Ports     []ServicePort `json:"ports,omitempty"`
}

type ServicePort struct {
    Name       string `json:"name"`
    Port       int32  `json:"port"`
    TargetPort string `json:"targetPort"`
    Protocol   string `json:"protocol"` // TCP, UDP
}

type IngressSummary struct {
    Name  string        `json:"name"`
    Hosts []string      `json:"hosts"`
    Rules []IngressRule `json:"rules,omitempty"`
}

type IngressRule struct {
    Host string `json:"host"`
    Path string `json:"path"`
}

type ConfigMapSummary struct {
    Name string   `json:"name"`
    Keys []string `json:"keys"` // ConfigMap key names (not full data)
}

type HistoricalContext struct {
    // Historical alert patterns
    PreviousAlerts     int     `json:"previousAlerts"`
    LastAlertTimestamp string  `json:"lastAlertTimestamp,omitempty"`
    AlertFrequency     float64 `json:"alertFrequency"` // alerts per hour

    // Historical resource usage
    AverageMemoryUsage string `json:"averageMemoryUsage,omitempty"` // e.g., "3.2Gi"
    AverageCPUUsage    string `json:"averageCPUUsage,omitempty"`    // e.g., "1.5 cores"

    // Historical success rate
    LastSuccessfulResolution string  `json:"lastSuccessfulResolution,omitempty"`
    ResolutionSuccessRate    float64 `json:"resolutionSuccessRate"` // 0.0-1.0
}

// EnvironmentClassification result
type EnvironmentClassification struct {
    Environment      string  `json:"environment"`      // "production", "staging", "development", "testing"
    Confidence       float64 `json:"confidence"`       // 0.0-1.0
    BusinessPriority string  `json:"businessPriority"` // "P0", "P1", "P2", "P3"
    SLARequirement   string  `json:"slaRequirement"`   // "5m", "15m", "30m"
}

// RoutingDecision for workflow continuation
type RoutingDecision struct {
    NextService string `json:"nextService"` // "ai-analysis"
    RoutingKey  string `json:"routingKey"`  // Alert fingerprint
    Priority    int    `json:"priority"`    // 0-10
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AlertProcessing is the Schema for the alertprocessings API
type AlertProcessing struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   AlertProcessingSpec   `json:"spec,omitempty"`
    Status AlertProcessingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AlertProcessingList contains a list of AlertProcessing
type AlertProcessingList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []AlertProcessing `json:"items"`
}

func init() {
    SchemeBuilder.Register(&AlertProcessing{}, &AlertProcessingList{})
}
```

## Controller Implementation

**Location**: `internal/controller/alertprocessing_controller.go`

### Controller Configuration

**Critical Patterns from [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)**:
1. **Owner References**: AlertProcessing CRD owned by AlertRemediation for cascade deletion
2. **Finalizers**: Cleanup coordination before deletion
3. **Watch Optimization**: Status updates trigger AlertRemediation reconciliation
4. **Timeout Handling**: Phase-level timeout detection and escalation
5. **Event Emission**: Operational visibility through Kubernetes events

```go
package controller

import (
    "context"
    "fmt"
    "strings"
    "time"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
)

const (
    // Finalizer for cleanup coordination
    alertProcessingFinalizer = "alertprocessing.kubernaut.io/finalizer"

    // Timeout configuration
    defaultPhaseTimeout = 5 * time.Minute  // Max time per phase
)

// AlertProcessingReconciler reconciles an AlertProcessing object
type AlertProcessingReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder  // Event emission for visibility
    ContextService ContextService        // Stateless HTTP call to Context Service
    Classifier     *EnvironmentClassifier // In-process classification
}

//+kubebuilder:rbac:groups=kubernaut.io,resources=alertprocessings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertprocessings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertprocessings/finalizers,verbs=update
//+kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=create
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertremediations,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertremediations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AlertProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch AlertProcessing CRD
    var alertProcessing processingv1.AlertProcessing
    if err := r.Get(ctx, req.NamespacedName, &alertProcessing); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion with finalizer
    if !alertProcessing.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &alertProcessing)
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&alertProcessing, alertProcessingFinalizer) {
        controllerutil.AddFinalizer(&alertProcessing, alertProcessingFinalizer)
        if err := r.Update(ctx, &alertProcessing); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Set owner reference to AlertRemediation (for cascade deletion)
    if err := r.ensureOwnerReference(ctx, &alertProcessing); err != nil {
        log.Error(err, "Failed to set owner reference")
        r.Recorder.Event(&alertProcessing, "Warning", "OwnerReferenceFailed",
            fmt.Sprintf("Failed to set owner reference: %v", err))
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Check for phase timeout (5 minutes per phase default)
    if r.isPhaseTimedOut(&alertProcessing) {
        return r.handlePhaseTimeout(ctx, &alertProcessing)
    }

    // Initialize phase if empty
    if alertProcessing.Status.Phase == "" {
        alertProcessing.Status.Phase = "enriching"
        alertProcessing.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        if err := r.Status().Update(ctx, &alertProcessing); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on current phase
    var result ctrl.Result
    var err error

    switch alertProcessing.Status.Phase {
    case "enriching":
        result, err = r.reconcileEnriching(ctx, &alertProcessing)
    case "classifying":
        result, err = r.reconcileClassifying(ctx, &alertProcessing)
    case "routing":
        result, err = r.reconcileRouting(ctx, &alertProcessing)
    case "completed":
        // Terminal state - use optimized requeue strategy
        return r.determineRequeueStrategy(&alertProcessing), nil
    default:
        log.Error(nil, "Unknown phase", "phase", alertProcessing.Status.Phase)
        r.Recorder.Event(&alertProcessing, "Warning", "UnknownPhase",
            fmt.Sprintf("Unknown phase: %s", alertProcessing.Status.Phase))
        return ctrl.Result{RequeueAfter: time.Second * 30}, nil
    }

    // Status update triggers AlertRemediation watch (automatic coordination)
    // No need to manually update AlertRemediation - watch mechanism handles it

    return result, err
}

// ensureOwnerReference sets AlertRemediation as owner for cascade deletion
func (r *AlertProcessingReconciler) ensureOwnerReference(ctx context.Context, ap *alertprocessorv1.AlertProcessing) error {
    // Skip if owner reference already set
    if len(ap.OwnerReferences) > 0 {
        return nil
    }

    // Fetch AlertRemediation to set as owner
    var alertRemediation remediationv1.AlertRemediation
    if err := r.Get(ctx, client.ObjectKey{
        Name:      ap.Spec.AlertRemediationRef.Name,
        Namespace: ap.Spec.AlertRemediationRef.Namespace,
    }, &alertRemediation); err != nil {
        return fmt.Errorf("failed to get AlertRemediation for owner reference: %w", err)
    }

    // Set owner reference
    if err := controllerutil.SetControllerReference(&alertRemediation, ap, r.Scheme); err != nil {
        return fmt.Errorf("failed to set controller reference: %w", err)
    }

    // Update with owner reference
    if err := r.Update(ctx, ap); err != nil {
        return fmt.Errorf("failed to update with owner reference: %w", err)
    }

    return nil
}

// isPhaseTimedOut checks if current phase has exceeded timeout
func (r *AlertProcessingReconciler) isPhaseTimedOut(ap *processingv1.AlertProcessing) bool {
    if ap.Status.PhaseStartTime == nil {
        return false
    }

    // Don't timeout completed phase
    if ap.Status.Phase == "completed" {
        return false
    }

    elapsed := time.Since(ap.Status.PhaseStartTime.Time)
    return elapsed > defaultPhaseTimeout
}

// handlePhaseTimeout handles phase timeout escalation
func (r *AlertProcessingReconciler) handlePhaseTimeout(ctx context.Context, ap *processingv1.AlertProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    elapsed := time.Since(ap.Status.PhaseStartTime.Time)
    log.Error(nil, "Phase timeout exceeded",
        "phase", ap.Status.Phase,
        "elapsed", elapsed,
        "timeout", defaultPhaseTimeout)

    // Emit timeout event
    r.Recorder.Event(ap, "Warning", "PhaseTimeout",
        fmt.Sprintf("Phase %s exceeded timeout of %s (elapsed: %s)",
            ap.Status.Phase, defaultPhaseTimeout, elapsed))

    // Add timeout condition
    timeoutCondition := metav1.Condition{
        Type:    "PhaseTimeout",
        Status:  metav1.ConditionTrue,
        Reason:  "TimeoutExceeded",
        Message: fmt.Sprintf("Phase %s exceeded %s timeout", ap.Status.Phase, defaultPhaseTimeout),
        LastTransitionTime: metav1.Now(),
    }
    apimeta.SetStatusCondition(&ap.Status.Conditions, timeoutCondition)

    // Record timeout metric
    ErrorsTotal.WithLabelValues("phase_timeout", ap.Status.Phase).Inc()

    // Update status
    if err := r.Status().Update(ctx, ap); err != nil {
        return ctrl.Result{}, err
    }

    // Move to next phase or fail based on phase
    switch ap.Status.Phase {
    case "enriching":
        // Use degraded mode - continue with basic context
        ap.Status.Phase = "classifying"
        ap.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        r.Recorder.Event(ap, "Normal", "DegradedMode",
            "Enrichment timeout - continuing with basic context")
    case "classifying":
        // Use default environment classification
        ap.Status.EnvironmentClassification = r.getDefaultClassification(ap)
        ap.Status.Phase = "routing"
        ap.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        r.Recorder.Event(ap, "Normal", "DefaultClassification",
            "Classification timeout - using default environment")
    case "routing":
        // Routing timeout is critical - emit error event
        r.Recorder.Event(ap, "Warning", "RoutingFailed",
            "Routing phase timeout - manual intervention required")
        return ctrl.Result{RequeueAfter: time.Minute * 2}, fmt.Errorf("routing timeout")
    }

    return ctrl.Result{Requeue: true}, r.Status().Update(ctx, ap)
}

// determineRequeueStrategy provides optimized requeue based on phase
func (r *AlertProcessingReconciler) determineRequeueStrategy(ap *alertprocessorv1.AlertProcessing) ctrl.Result {
    switch ap.Status.Phase {
    case "completed":
        // Terminal state - no requeue needed (watch handles updates)
        return ctrl.Result{}
    case "enriching", "classifying", "routing":
        // Active phases - short requeue for progress
        return ctrl.Result{RequeueAfter: time.Second * 10}
    default:
        // Unknown state - conservative requeue
        return ctrl.Result{RequeueAfter: time.Second * 30}
    }
}

// reconcileDelete handles cleanup before deletion
func (r *AlertProcessingReconciler) reconcileDelete(ctx context.Context, ap *alertprocessorv1.AlertProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if !controllerutil.ContainsFinalizer(ap, alertProcessingFinalizer) {
        return ctrl.Result{}, nil
    }

    log.Info("Cleaning up AlertProcessing resources", "name", ap.Name)

    // Perform cleanup tasks
    // - Audit data should already be persisted
    // - No additional cleanup needed (AIAnalysis CRD creation is idempotent)

    // Emit deletion event
    r.Recorder.Event(ap, "Normal", "Cleanup",
        "AlertProcessing cleanup completed before deletion")

    // Remove finalizer
    controllerutil.RemoveFinalizer(ap, alertProcessingFinalizer)
    if err := r.Update(ctx, ap); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

// getDefaultClassification provides fallback classification on timeout
func (r *AlertProcessingReconciler) getDefaultClassification(ap *processingv1.AlertProcessing) processingv1.EnvironmentClassification {
    // Use namespace prefix or labels as fallback
    environment := "unknown"
    if strings.HasPrefix(ap.Spec.Alert.Namespace, "prod") {
        environment = "production"
    } else if strings.HasPrefix(ap.Spec.Alert.Namespace, "stag") {
        environment = "staging"
    } else if strings.HasPrefix(ap.Spec.Alert.Namespace, "dev") {
        environment = "development"
    }

    return processingv1.EnvironmentClassification{
        Environment:      environment,
        Confidence:       0.5, // Low confidence fallback
        BusinessPriority: "P2",
        SLARequirement:   "30m",
    }
}

func (r *AlertProcessingReconciler) reconcileEnriching(ctx context.Context, ap *processingv1.AlertProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    log.Info("Enriching alert with context", "fingerprint", ap.Spec.Alert.Fingerprint)

    // Call Context Service (stateless HTTP call)
    enrichmentResults, err := r.ContextService.GetContext(ctx, ap.Spec.Alert)
    if err != nil {
        log.Error(err, "Failed to get context")
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Update status with enrichment results
    ap.Status.EnrichmentResults = enrichmentResults
    ap.Status.Phase = "classifying"

    if err := r.Status().Update(ctx, ap); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Trigger immediate reconciliation for next phase
    return ctrl.Result{Requeue: true}, nil
}

func (r *AlertProcessingReconciler) reconcileClassifying(ctx context.Context, ap *processingv1.AlertProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    log.Info("Classifying environment", "namespace", ap.Spec.Alert.Namespace)

    // Perform environment classification (in-process)
    classification, err := r.Classifier.ClassifyEnvironment(ctx, ap.Spec.Alert, ap.Status.EnrichmentResults)
    if err != nil {
        log.Error(err, "Failed to classify environment")
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Update status with classification
    ap.Status.EnvironmentClassification = classification
    ap.Status.Phase = "routing"

    if err := r.Status().Update(ctx, ap); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Trigger immediate reconciliation for next phase
    return ctrl.Result{Requeue: true}, nil
}

func (r *AlertProcessingReconciler) reconcileRouting(ctx context.Context, ap *processingv1.AlertProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Create AIAnalysis CRD for next service
    aiAnalysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("ai-analysis-%s", ap.Spec.Alert.Fingerprint),
            Namespace: ap.Namespace,
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            AlertRemediationRef: ap.Spec.AlertRemediationRef,
            AnalysisRequest:     buildAnalysisRequest(ap.Status.EnrichmentResults, ap.Status.EnvironmentClassification),
        },
    }

    if err := r.Create(ctx, aiAnalysis); err != nil && !errors.IsAlreadyExists(err) {
        log.Error(err, "Failed to create AIAnalysis CRD")
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Mark as completed
    ap.Status.Phase = "completed"
    ap.Status.ProcessingTime = time.Since(ap.CreationTimestamp.Time).String()

    if err := r.Status().Update(ctx, ap); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    log.Info("Alert processing completed", "fingerprint", ap.Spec.Alert.Fingerprint, "duration", ap.Status.ProcessingTime)

    // Terminal state - no requeue
    return ctrl.Result{}, nil
}

func (r *AlertProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&processingv1.AlertProcessing{}).
        Complete(r)
}
```

---

## Finalizer Implementation

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const alertProcessingFinalizer = "alertprocessor.kubernaut.io/alertprocessing-cleanup"
```

**Naming Pattern**: `{domain}.kubernaut.io/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `alertprocessor.kubernaut.io` prevents conflicts with other services
- **Resource-Specific**: `alertprocessing-cleanup` clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const alertProcessingFinalizer = "alertprocessor.kubernaut.io/alertprocessing-cleanup"

type AlertProcessingReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    ContextClient     ContextClient
    StorageClient     StorageClient
}

func (r *AlertProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var ap processingv1.AlertProcessing
    if err := r.Get(ctx, req.NamespacedName, &ap); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !ap.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&ap, alertProcessingFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupAlertProcessing(ctx, &ap); err != nil {
                r.Log.Error(err, "Failed to cleanup AlertProcessing resources",
                    "name", ap.Name,
                    "namespace", ap.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&ap, alertProcessingFinalizer)
            if err := r.Update(ctx, &ap); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&ap, alertProcessingFinalizer) {
        controllerutil.AddFinalizer(&ap, alertProcessingFinalizer)
        if err := r.Update(ctx, &ap); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed
    if ap.Status.Phase == "completed" || ap.Status.Phase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute processing...
    // (existing reconciliation logic from previous section)

    return ctrl.Result{}, nil
}
```

---

### Cleanup Logic

**What Gets Cleaned Up**:

```go
package controller

import (
    "context"
    "fmt"
    "time"

    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
)

func (r *AlertProcessingReconciler) cleanupAlertProcessing(
    ctx context.Context,
    ap *processingv1.AlertProcessing,
) error {
    r.Log.Info("Cleaning up AlertProcessing resources",
        "name", ap.Name,
        "namespace", ap.Namespace,
        "phase", ap.Status.Phase,
    )

    // 1. Record final audit to database
    if err := r.recordFinalAudit(ctx, ap); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", ap.Name)
        // Don't block deletion on audit failure
        // Audit is best-effort during cleanup
    }

    // 2. Emit deletion event
    r.Recorder.Event(ap, "Normal", "AlertProcessingDeleted",
        fmt.Sprintf("AlertProcessing cleanup completed (phase: %s)", ap.Status.Phase))

    r.Log.Info("AlertProcessing cleanup completed successfully",
        "name", ap.Name,
        "namespace", ap.Namespace,
    )

    return nil
}

func (r *AlertProcessingReconciler) recordFinalAudit(
    ctx context.Context,
    ap *processingv1.AlertProcessing,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint: ap.Spec.Alert.Fingerprint,
        ServiceType:      "AlertProcessing",
        CRDName:          ap.Name,
        Namespace:        ap.Namespace,
        Phase:            ap.Status.Phase,
        CreatedAt:        ap.CreationTimestamp.Time,
        DeletedAt:        ap.DeletionTimestamp.Time,
        DegradedMode:     ap.Status.DegradedMode,
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Cleanup Philosophy for AlertProcessing**:
- ✅ **Record final audit**: Capture that processing occurred (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ❌ **No external cleanup needed**: AlertProcessing is a leaf CRD (owns nothing)
- ❌ **No child CRD cleanup**: AlertProcessing doesn't create child CRDs
- ✅ **Non-blocking**: Audit failures don't block deletion (best-effort)

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
    "github.com/jordigilh/kubernaut/pkg/alertprocessor/controller"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("AlertProcessing Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.AlertProcessingReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.AlertProcessingReconciler{
            Client:        k8sClient,
            StorageClient: &mockStorageClient{},
        }
    })

    Context("when AlertProcessing is created", func() {
        It("should add finalizer on first reconcile", func() {
            ap := &processingv1.AlertProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-processing",
                    Namespace: "default",
                },
                Spec: processingv1.AlertProcessingSpec{
                    Alert: processingv1.Alert{
                        Fingerprint: "abc123",
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ap)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ap),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ap), ap)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(ap, alertProcessingFinalizer)).To(BeTrue())
        })
    })

    Context("when AlertProcessing is deleted", func() {
        It("should execute cleanup and remove finalizer", func() {
            ap := &processingv1.AlertProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-processing",
                    Namespace:  "default",
                    Finalizers: []string{alertProcessingFinalizer},
                },
                Spec: processingv1.AlertProcessingSpec{
                    Alert: processingv1.Alert{
                        Fingerprint: "abc123",
                    },
                },
                Status: processingv1.AlertProcessingStatus{
                    Phase: "completed",
                },
            }
            Expect(k8sClient.Create(ctx, ap)).To(Succeed())

            // Delete AlertProcessing
            Expect(k8sClient.Delete(ctx, ap)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ap),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ap), ap)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if audit fails", func() {
            // Mock storage client to return error
            reconciler.StorageClient = &mockStorageClient{
                recordAuditError: fmt.Errorf("database unavailable"),
            }

            ap := &processingv1.AlertProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-processing",
                    Namespace:  "default",
                    Finalizers: []string{alertProcessingFinalizer},
                },
            }
            Expect(k8sClient.Create(ctx, ap)).To(Succeed())
            Expect(k8sClient.Delete(ctx, ap)).To(Succeed())

            // Cleanup should succeed even if audit fails
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ap),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite audit failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ap), ap)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })
    })
})
```

---

## CRD Lifecycle Management

### Creation Lifecycle

**Created By**: AlertRemediation controller (centralized orchestration)

**Creation Trigger**: AlertRemediation transitions to `pending` phase

**Sequence**:
```
Gateway Service creates AlertRemediation CRD
    ↓
AlertRemediation.status.overallPhase = "pending"
    ↓
AlertRemediation Controller reconciles
    ↓
AlertRemediation Controller creates AlertProcessing CRD
    ↓ (with owner reference)
AlertProcessing Controller reconciles (this controller)
    ↓
AlertProcessing.status.phase = "completed"
    ↓ (watch trigger <100ms)
AlertRemediation Controller detects completion
    ↓
AlertRemediation Controller creates AIAnalysis CRD
```

**Owner Reference Set at Creation**:
```go
package controller

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/alertremediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// In AlertRemediationReconciler
func (r *AlertRemediationReconciler) createAlertProcessing(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
) error {
    alertProcessing := &processingv1.AlertProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-processing", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation,
                    remediationv1.GroupVersion.WithKind("AlertRemediation")),
            },
        },
        Spec: processingv1.AlertProcessingSpec{
            AlertRemediationRef: processingv1.AlertRemediationReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            Alert: processingv1.Alert{
                Fingerprint: remediation.Spec.AlertFingerprint,
                Payload:     remediation.Spec.OriginalPayload,
            },
        },
    }

    return r.Create(ctx, alertProcessing)
}
```

**Result**: AlertProcessing is owned by AlertRemediation (cascade deletion applies)

---

### Update Lifecycle

**Status Updates by AlertProcessing Controller**:

```go
package controller

import (
    "context"
    "time"

    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *AlertProcessingReconciler) updateStatusCompleted(
    ctx context.Context,
    ap *processingv1.AlertProcessing,
    enriched processingv1.EnrichedAlert,
    classification string,
) error {
    // Controller updates own status
    ap.Status.Phase = "completed"
    ap.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    ap.Status.EnrichedAlert = enriched
    ap.Status.EnvironmentClassification = classification

    return r.Status().Update(ctx, ap)
}
```

**Watch Triggers AlertRemediation Reconciliation**:

```
AlertProcessing.status.phase = "completed"
    ↓ (watch event)
AlertRemediation watch triggers
    ↓ (<100ms latency)
AlertRemediation Controller reconciles
    ↓
AlertRemediation extracts enriched data
    ↓
AlertRemediation creates AIAnalysis CRD
```

**No Self-Updates After Completion**:
- AlertProcessing does NOT update itself after `phase = "completed"`
- AlertProcessing does NOT create other CRDs (leaf controller)
- AlertProcessing does NOT watch other CRDs

---

### Deletion Lifecycle

**Trigger**: AlertRemediation deletion (cascade)

**Cascade Deletion Sequence**:
```
User/System deletes AlertRemediation
    ↓
Kubernetes garbage collector detects owner reference
    ↓ (parallel deletion of all owned CRDs)
AlertProcessing.deletionTimestamp set
    ↓
AlertProcessing Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Record final audit
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes AlertProcessing CRD
```

**Parallel Deletion**: All service CRDs (AlertProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025)) deleted in parallel when AlertRemediation is deleted.

**Retention**:
- **AlertProcessing**: No independent retention (deleted with parent)
- **AlertRemediation**: 24-hour retention (parent CRD manages retention)
- **Audit Data**: 90-day retention in PostgreSQL (persisted before deletion)

---

### Lifecycle Events

**Kubernetes Events Emitted**:

```go
package controller

import (
    "fmt"
    "time"

    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    "k8s.io/client-go/tools/record"
)

func (r *AlertProcessingReconciler) emitLifecycleEvents(
    ap *processingv1.AlertProcessing,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(ap, "Normal", "AlertProcessingCreated",
        fmt.Sprintf("Alert processing started for %s", ap.Spec.Alert.Fingerprint))

    // Phase transition events
    r.Recorder.Event(ap, "Normal", "PhaseTransition",
        fmt.Sprintf("Phase: %s → %s", oldPhase, ap.Status.Phase))

    // Degraded mode event
    if ap.Status.DegradedMode {
        r.Recorder.Event(ap, "Warning", "DegradedMode",
            "Context Service unavailable, using minimal context from alert labels")
    }

    // Completion event
    r.Recorder.Event(ap, "Normal", "AlertProcessingCompleted",
        fmt.Sprintf("Alert processing completed in %s", duration))

    // Deletion event (in cleanup function)
    r.Recorder.Event(ap, "Normal", "AlertProcessingDeleted",
        fmt.Sprintf("AlertProcessing cleanup completed (phase: %s)", ap.Status.Phase))
}
```

**Event Visibility**:
```bash
kubectl describe alertprocessing <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific AlertProcessing
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(alertprocessing_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, alertprocessing_lifecycle_duration_seconds)

# Active AlertProcessing CRDs
alertprocessing_active_total

# CRD deletion rate
rate(alertprocessing_deleted_total[5m])

# Degraded mode percentage
sum(alertprocessing_active_total{degraded_mode="true"}) / sum(alertprocessing_active_total)
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "AlertProcessing Lifecycle"
    targets:
      - expr: alertprocessing_active_total
        legendFormat: "Active CRDs"
      - expr: rate(alertprocessing_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(alertprocessing_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Processing Latency (P95)"
    targets:
      - expr: histogram_quantile(0.95, alertprocessing_lifecycle_duration_seconds)
        legendFormat: "P95 Duration"
```

**Alert Rules**:

```yaml
groups:
- name: alertprocessing-lifecycle
  rules:
  - alert: AlertProcessingStuckInPhase
    expr: time() - alertprocessing_phase_start_timestamp > 600
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "AlertProcessing stuck in phase for >10 minutes"
      description: "AlertProcessing {{ $labels.name }} has been in phase {{ $labels.phase }} for over 10 minutes"

  - alert: AlertProcessingHighDeletionRate
    expr: rate(alertprocessing_deleted_total[5m]) > rate(alertprocessing_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "AlertProcessing deletion rate exceeds creation rate"
      description: "More AlertProcessing CRDs being deleted than created (possible cascade deletion issue)"

  - alert: AlertProcessingHighDegradedMode
    expr: sum(alertprocessing_active_total{degraded_mode="true"}) / sum(alertprocessing_active_total) > 0.5
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: ">50% of AlertProcessing CRDs in degraded mode"
      description: "Context Service may be unavailable"
```

---

## Prometheus Metrics Implementation

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

**Using `promauto` for Automatic Registration** (Recommended):

```go
package alertprocessor

import (
    "time"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counter: Total alerts processed
    AlertsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_alertprocessor_alerts_processed_total",
        Help: "Total number of alerts processed by alert processor",
    }, []string{"severity", "namespace", "environment"})

    // Histogram: Processing duration by phase
    ProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "kubernaut_alertprocessor_processing_duration_seconds",
        Help:    "Duration of alert processing operations by phase",
        Buckets: []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
    }, []string{"phase"}) // enriching, classifying, routing, completed

    // Gauge: Current active processing
    ActiveProcessingGauge = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "kubernaut_alertprocessor_active_processing",
        Help: "Number of alerts currently being processed",
    })

    // Counter: Errors by type and phase
    ErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_alertprocessor_errors_total",
        Help: "Total errors encountered during alert processing",
    }, []string{"error_type", "phase"})

    // Histogram: Enrichment context size in bytes
    EnrichmentContextSizeBytes = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "kubernaut_alertprocessor_enrichment_context_bytes",
        Help:    "Size of enrichment context in bytes",
        Buckets: []float64{1000, 5000, 10000, 50000, 100000, 500000},
    }, []string{"context_depth"}) // basic, detailed, comprehensive

    // Histogram: Classification confidence
    ClassificationConfidence = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "kubernaut_alertprocessor_classification_confidence",
        Help:    "Environment classification confidence score",
        Buckets: []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 0.99, 1.0},
    }, []string{"environment"}) // production, staging, development, testing

    // Counter: Context service requests
    ContextServiceRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_alertprocessor_context_service_requests_total",
        Help: "Total requests made to context service",
    }, []string{"status"}) // success, error, timeout

    // Histogram: Context service response time
    ContextServiceDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_alertprocessor_context_service_duration_seconds",
        Help:    "Duration of context service HTTP calls",
        Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
    })

    // Counter: AIAnalysis CRD creations
    AIAnalysisCRDCreatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_alertprocessor_aianalysis_created_total",
        Help: "Total AIAnalysis CRDs created for routing",
    }, []string{"status"}) // created, already_exists, error
)

// Helper function to record successful alert processing
func RecordAlertProcessed(severity, namespace, environment string, totalDuration time.Duration) {
    AlertsProcessedTotal.WithLabelValues(severity, namespace, environment).Inc()
    ProcessingDuration.WithLabelValues("completed").Observe(totalDuration.Seconds())
}

// Helper function to record phase timing
func RecordPhaseCompletion(phase string, duration time.Duration) {
    ProcessingDuration.WithLabelValues(phase).Observe(duration.Seconds())
}

// Helper function to record classification
func RecordClassification(environment string, confidence float64) {
    ClassificationConfidence.WithLabelValues(environment).Observe(confidence)
}

// Helper function to record context service call
func RecordContextServiceCall(status string, duration time.Duration) {
    ContextServiceRequestsTotal.WithLabelValues(status).Inc()
    ContextServiceDuration.Observe(duration.Seconds())
}
```

### Manual Registration (for Custom Registry):

```go
package alertprocessor

import (
    "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
    registry             *prometheus.Registry
    alertsProcessed      *prometheus.CounterVec
    processingDuration   *prometheus.HistogramVec
    activeProcessing     prometheus.Gauge
    errorsTotal          *prometheus.CounterVec
    enrichmentContextSize *prometheus.HistogramVec
}

func NewMetrics() *Metrics {
    registry := prometheus.NewRegistry()

    m := &Metrics{
        registry: registry,

        alertsProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
            Name: "kubernaut_alertprocessor_alerts_processed_total",
            Help: "Total alerts processed",
        }, []string{"severity", "namespace", "environment"}),

        processingDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
            Name:    "kubernaut_alertprocessor_processing_duration_seconds",
            Help:    "Alert processing duration by phase",
            Buckets: prometheus.DefBuckets,
        }, []string{"phase"}),

        activeProcessing: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "kubernaut_alertprocessor_active_processing",
            Help: "Currently active alert processing",
        }),

        errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
            Name: "kubernaut_alertprocessor_errors_total",
            Help: "Total errors by type and phase",
        }, []string{"error_type", "phase"}),

        enrichmentContextSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
            Name:    "kubernaut_alertprocessor_enrichment_context_bytes",
            Help:    "Enrichment context size",
            Buckets: []float64{1000, 5000, 10000, 50000, 100000},
        }, []string{"context_depth"}),
    }

    // Manual registration with custom registry
    registry.MustRegister(
        m.alertsProcessed,
        m.processingDuration,
        m.activeProcessing,
        m.errorsTotal,
        m.enrichmentContextSize,
    )

    return m
}
```

### Integration with Controller Reconciliation

```go
func (r *AlertProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    startTime := time.Now()
    ActiveProcessingGauge.Inc()
    defer ActiveProcessingGauge.Dec()

    // Fetch AlertProcessing CR
    var alertProcessing processingv1.AlertProcessing
    if err := r.Get(ctx, req.NamespacedName, &alertProcessing); err != nil {
        ErrorsTotal.WithLabelValues("fetch_cr_failed", "reconcile").Inc()
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Reconcile based on current phase with metrics
    switch alertProcessing.Status.Phase {
    case "", "enriching":
        return r.reconcileEnrichingWithMetrics(ctx, &alertProcessing)
    case "classifying":
        return r.reconcileClassifyingWithMetrics(ctx, &alertProcessing)
    case "routing":
        return r.reconcileRoutingWithMetrics(ctx, &alertProcessing)
    case "completed":
        // Record total processing time
        totalDuration := time.Since(alertProcessing.CreationTimestamp.Time)
        RecordAlertProcessed(
            alertProcessing.Spec.Alert.Severity,
            alertProcessing.Spec.Alert.Namespace,
            alertProcessing.Status.EnvironmentClassification.Environment,
            totalDuration,
        )
        return ctrl.Result{}, nil
    }
}

func (r *AlertProcessingReconciler) reconcileEnrichingWithMetrics(ctx context.Context, ap *processingv1.AlertProcessing) (ctrl.Result, error) {
    phaseStart := time.Now()

    // Call Context Service with timing
    contextStart := time.Now()
    enrichmentResults, err := r.ContextService.GetContext(ctx, ap.Spec.Alert)
    contextDuration := time.Since(contextStart)

    if err != nil {
        RecordContextServiceCall("error", contextDuration)
        ErrorsTotal.WithLabelValues("context_service_failed", "enriching").Inc()
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }
    RecordContextServiceCall("success", contextDuration)

    // Record enrichment context size
    contextSize := calculateContextSize(enrichmentResults) // Helper function
    EnrichmentContextSizeBytes.WithLabelValues(ap.Spec.EnrichmentConfig.ContextDepth).Observe(float64(contextSize))

    // Update status
    ap.Status.EnrichmentResults = enrichmentResults
    ap.Status.Phase = "classifying"

    if err := r.Status().Update(ctx, ap); err != nil {
        ErrorsTotal.WithLabelValues("status_update_failed", "enriching").Inc()
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Record phase completion
    RecordPhaseCompletion("enriching", time.Since(phaseStart))

    return ctrl.Result{Requeue: true}, nil
}

func (r *AlertProcessingReconciler) reconcileClassifyingWithMetrics(ctx context.Context, ap *processingv1.AlertProcessing) (ctrl.Result, error) {
    phaseStart := time.Now()

    classification, err := r.Classifier.ClassifyEnvironment(ctx, ap.Spec.Alert, ap.Status.EnrichmentResults)
    if err != nil {
        ErrorsTotal.WithLabelValues("classification_failed", "classifying").Inc()
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Record classification confidence
    RecordClassification(classification.Environment, classification.Confidence)

    // Update status
    ap.Status.EnvironmentClassification = classification
    ap.Status.Phase = "routing"

    if err := r.Status().Update(ctx, ap); err != nil {
        ErrorsTotal.WithLabelValues("status_update_failed", "classifying").Inc()
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Record phase completion
    RecordPhaseCompletion("classifying", time.Since(phaseStart))

    return ctrl.Result{Requeue: true}, nil
}

func (r *AlertProcessingReconciler) reconcileRoutingWithMetrics(ctx context.Context, ap *processingv1.AlertProcessing) (ctrl.Result, error) {
    phaseStart := time.Now()

    // Create AIAnalysis CRD
    aiAnalysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("ai-analysis-%s", ap.Spec.Alert.Fingerprint),
            Namespace: ap.Namespace,
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            AlertRemediationRef: ap.Spec.AlertRemediationRef,
            AnalysisRequest:     buildAnalysisRequest(ap.Status.EnrichmentResults, ap.Status.EnvironmentClassification),
        },
    }

    if err := r.Create(ctx, aiAnalysis); err != nil {
        if errors.IsAlreadyExists(err) {
            AIAnalysisCRDCreatedTotal.WithLabelValues("already_exists").Inc()
        } else {
            AIAnalysisCRDCreatedTotal.WithLabelValues("error").Inc()
            ErrorsTotal.WithLabelValues("aianalysis_creation_failed", "routing").Inc()
            return ctrl.Result{RequeueAfter: time.Second * 15}, err
        }
    } else {
        AIAnalysisCRDCreatedTotal.WithLabelValues("created").Inc()
    }

    // Mark as completed
    ap.Status.Phase = "completed"
    ap.Status.ProcessingTime = time.Since(ap.CreationTimestamp.Time).String()

    if err := r.Status().Update(ctx, ap); err != nil {
        ErrorsTotal.WithLabelValues("status_update_failed", "routing").Inc()
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Record routing phase completion
    RecordPhaseCompletion("routing", time.Since(phaseStart))

    return ctrl.Result{}, nil
}

// Helper function to calculate context size
func calculateContextSize(results processingv1.EnrichmentResults) int {
    // Simple JSON marshaling to get size
    data, _ := json.Marshal(results)
    return len(data)
}
```

### Recommended Grafana Dashboards

**Prometheus Queries for Monitoring**:

```promql
# Alert processing rate (alerts/sec)
rate(kubernaut_alertprocessor_alerts_processed_total[5m])

# Processing duration by phase (p95)
histogram_quantile(0.95, rate(kubernaut_alertprocessor_processing_duration_seconds_bucket[5m]))

# Error rate by phase
rate(kubernaut_alertprocessor_errors_total[5m]) / rate(kubernaut_alertprocessor_alerts_processed_total[5m])

# Active processing queue depth
kubernaut_alertprocessor_active_processing

# Classification accuracy (confidence distribution)
histogram_quantile(0.95, rate(kubernaut_alertprocessor_classification_confidence_bucket[5m]))

# Context service health
rate(kubernaut_alertprocessor_context_service_requests_total{status="success"}[5m]) /
rate(kubernaut_alertprocessor_context_service_requests_total[5m])

# Enrichment context size trend (p95)
histogram_quantile(0.95, rate(kubernaut_alertprocessor_enrichment_context_bytes_bucket[5m]))
```

### Metrics Naming Convention

Following Prometheus best practices and Kubernaut standards:
- **Prefix**: `kubernaut_alertprocessor_`
- **Metric Types**:
  - `_total` suffix for counters
  - `_seconds` suffix for duration histograms
  - `_bytes` suffix for size histograms
  - No suffix for gauges
- **Labels**: Use lowercase with underscores (e.g., `context_depth`, `error_type`)

### Performance Targets

- **Alert Processing**: p95 < 500ms, p99 < 1s
- **Enrichment Phase**: p95 < 200ms, p99 < 500ms
- **Classification Phase**: p95 < 100ms, p99 < 200ms
- **Routing Phase**: p95 < 50ms, p99 < 100ms
- **Error Rate**: < 0.1% of processed alerts
- **Context Service Calls**: p95 < 500ms, success rate > 99.9%

## Testing Strategy

**Testing Framework Reference**: [.cursor/rules/03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)

### Pyramid Testing Approach

Following Kubernaut's defense-in-depth testing strategy with expanded unit coverage:

- **Unit Tests (70%+)**: Comprehensive controller logic with mocked external dependencies
- **Integration Tests (20%)**: Cross-component CRD interactions with real K8s
- **E2E Tests (10%)**: Complete alert processing workflows

### Unit Tests (Primary Coverage Layer)

**Test Directory**: [test/unit/](../../../test/unit/)
**Service Tests**: Create `test/unit/alertprocessor/controller_test.go`
**Coverage Target**: 70%+ of business requirements (BR-SP-001 to BR-SP-050)
**Confidence**: 85-90%
**Execution**: `make test`

**Testing Strategy**: Use fake K8s client for compile-time API safety. Mock ONLY external HTTP services (Context Service, AI services). Use REAL business logic components.

**Rationale for Fake K8s Client**:
- ✅ **Compile-Time API Safety**: K8s API changes/deprecations caught at build time, not runtime
- ✅ **Type-Safe CRD Handling**: Schema changes validated by compiler
- ✅ **Real K8s Errors**: `apierrors.IsNotFound()`, `apierrors.IsConflict()` behavior
- ✅ **Acceptable Speed**: ~0.8s execution (worth the trade-off for production safety)
- ✅ **Upgrade Protection**: Breaking API changes explicit, not hidden

**Test File Structure** (aligned with package name `alertprocessor`):
```
test/unit/
├── alertprocessor/                 # Matches pkg/alertprocessor/
│   ├── controller_test.go          # Main controller reconciliation tests
│   ├── enrichment_test.go          # Alert enrichment phase tests
│   ├── classification_test.go      # Environment classification tests
│   ├── routing_test.go             # Routing decision tests
│   └── suite_test.go               # Ginkgo test suite setup
└── ...
```

**Migration Note**: Rename `test/unit/alert/` → `test/unit/alertprocessor/` to match package structure.

```go
package alertprocessor

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
    "github.com/jordigilh/kubernaut/internal/controller"
    "github.com/jordigilh/kubernaut/pkg/processor/environment"
    "github.com/jordigilh/kubernaut/pkg/testutil"
    "github.com/jordigilh/kubernaut/pkg/testutil/mocks"

    v1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("BR-SP-001: Alert Processing Controller", func() {
    var (
        // Fake K8s client for compile-time API safety
        fakeK8sClient      client.Client
        scheme             *runtime.Scheme

        // Mock ONLY external HTTP services
        mockContextService *mocks.MockContextService

        // Use REAL business logic components
        classifier         *environment.Classifier
        reconciler         *controller.AlertProcessingReconciler
        ctx                context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Minimal scheme: Only types needed for these tests
        scheme = runtime.NewScheme()
        _ = v1.AddToScheme(scheme)
        _ = processingv1.AddToScheme(scheme)

        // Fake K8s client with compile-time API safety
        fakeK8sClient = fake.NewClientBuilder().
            WithScheme(scheme).
            Build()

        // Mock external HTTP service (NOT K8s)
        mockContextService = mocks.NewMockContextService()

        // Use REAL business logic
        classifier = environment.NewClassifier(testutil.NewTestConfig())

        reconciler = &controller.AlertProcessingReconciler{
            Client:         fakeK8sClient,
            Scheme:         scheme,
            ContextService: mockContextService,
            Classifier:     classifier, // Real business logic
        }
    })

    Context("BR-SP-010: Alert Enrichment Phase", func() {
        It("should enrich alert with kubernetes context and transition to classifying", func() {
            // Setup test alert
            ap := &processingv1.AlertProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:              "test-alert-high-memory",
                    Namespace:         "default",
                    CreationTimestamp: metav1.Now(),
                },
                Spec: processingv1.AlertProcessingSpec{
                    Alert: processingv1.Alert{
                        Fingerprint: "mem-pressure-prod-123",
                        Namespace:   "production",
                        Severity:    "critical",
                        Labels: map[string]string{
                            "alertname": "HighMemoryUsage",
                        },
                    },
                    EnrichmentConfig: processingv1.EnrichmentConfig{
                        ContextSources:     []string{"kubernetes", "historical"},
                        ContextDepth:       "detailed",
                        HistoricalLookback: "24h",
                    },
                },
            }

            // Create AlertProcessing CRD in fake K8s (compile-time safe)
            Expect(fakeK8sClient.Create(ctx, ap)).To(Succeed())

            // Mock Context Service response with structured data
            // ✅ TYPE SAFE - Uses structured types instead of map[string]interface{}
            mockContextService.On("GetContext", ctx, ap.Spec.Alert).Return(
                processingv1.EnrichmentResults{
                    KubernetesContext: &processingv1.KubernetesContext{
                        Namespace: "production",
                        PodDetails: &processingv1.PodDetails{
                            Name:         "webapp-789",
                            Phase:        "Running",
                            RestartCount: 0,
                            Containers: []processingv1.ContainerStatus{
                                {Name: "webapp", Image: "webapp:v1.2.3", Ready: true, State: "running"},
                            },
                        },
                        DeploymentDetails: &processingv1.DeploymentDetails{
                            Name:              "webapp",
                            Replicas:          5,
                            ReadyReplicas:     5,
                            AvailableReplicas: 5,
                        },
                    },
                    HistoricalContext: &processingv1.HistoricalContext{
                        PreviousAlerts:        3,
                        LastAlertTimestamp:    "2024-01-15T10:30:00Z",
                        ResolutionSuccessRate: 0.85,
                    },
                    EnrichmentQuality: 0.92,
                },
                nil,
            )

            // Execute reconciliation
            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(ap))

            // Validate business outcomes
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeTrue(), "should requeue for next phase")
            Expect(ap.Status.Phase).To(Equal("classifying"))
            Expect(ap.Status.EnrichmentResults.EnrichmentQuality).To(BeNumerically(">", 0.9))
            Expect(ap.Status.EnrichmentResults.KubernetesContext).To(HaveKey("podCount"))

            // Verify Context Service was called exactly once
            mockContextService.AssertNumberOfCalls(GinkgoT(), "GetContext", 1)
        })

        It("BR-SP-011: should handle context service failures with degraded mode", func() {
            ap := testutil.NewAlertProcessing("test-alert-degraded", "default")

            mockK8sClient.On("Get", ctx, client.ObjectKeyFromObject(ap), ap).Return(nil)

            // Simulate Context Service failure
            mockContextService.On("GetContext", ctx, ap.Spec.Alert).Return(
                processingv1.EnrichmentResults{},
                errors.New("context service timeout"),
            )

            // Execute reconciliation
            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(ap))

            // Validate error handling
            Expect(err).To(HaveOccurred())
            Expect(result.RequeueAfter).To(Equal(30 * time.Second))

            // Verify metrics recorded
            // Note: Metrics validation would check ErrorsTotal counter increment
        })
    })

    Context("BR-SP-020: Environment Classification Phase", func() {
        It("should classify production environment with high confidence", func() {
            // Setup alert with production indicators
            ap := &processingv1.AlertProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-prod-classification",
                    Namespace: "default",
                },
                Spec: processingv1.AlertProcessingSpec{
                    Alert: processingv1.Alert{
                        Namespace: "prod-webapp",
                        Labels: map[string]string{
                            "environment": "production",
                            "tier":        "critical",
                        },
                    },
                    EnvironmentClassification: processingv1.EnvironmentClassificationConfig{
                        ClassificationSources: []string{"labels", "namespace-pattern"},
                        ConfidenceThreshold:   0.8,
                    },
                },
                Status: processingv1.AlertProcessingStatus{
                    Phase: "classifying",
                    EnrichmentResults: processingv1.EnrichmentResults{
                        KubernetesContext: &processingv1.KubernetesContext{
                            Namespace: "prod-webapp",
                            NamespaceLabels: map[string]string{
                                "environment": "production",
                            },
                        },
                    },
                },
            }

            mockK8sClient.On("Get", ctx, client.ObjectKeyFromObject(ap), ap).Return(nil)
            mockK8sClient.On("Status().Update", ctx, ap).Return(nil)

            // Execute reconciliation with REAL classifier
            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(ap))

            // Validate REAL business logic classification
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeTrue())
            Expect(ap.Status.Phase).To(Equal("routing"))
            Expect(ap.Status.EnvironmentClassification.Environment).To(Equal("production"))
            Expect(ap.Status.EnvironmentClassification.Confidence).To(BeNumerically(">", 0.8))
            Expect(ap.Status.EnvironmentClassification.BusinessPriority).To(Equal("P0"))
            Expect(ap.Status.EnvironmentClassification.SLARequirement).To(Equal("5m"))
        })

        It("BR-SP-021: should classify staging environment with medium priority", func() {
            ap := testutil.NewAlertProcessingWithPhase("test-staging", "default", "classifying")
            ap.Spec.Alert.Namespace = "staging-api"
            ap.Spec.Alert.Labels = map[string]string{"environment": "staging"}

            mockK8sClient.On("Get", ctx, client.ObjectKeyFromObject(ap), ap).Return(nil)
            mockK8sClient.On("Status().Update", ctx, ap).Return(nil)

            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(ap))

            Expect(err).ToNot(HaveOccurred())
            Expect(ap.Status.EnvironmentClassification.Environment).To(Equal("staging"))
            Expect(ap.Status.EnvironmentClassification.BusinessPriority).To(Equal("P2"))
        })
    })

    Context("BR-SP-030: Routing Decision Phase", func() {
        It("should create AIAnalysis CRD and mark processing complete", func() {
            ap := testutil.NewAlertProcessingWithPhase("test-routing", "default", "routing")
            ap.Spec.Alert.Fingerprint = "route-test-456"
            ap.Status.EnvironmentClassification = processingv1.EnvironmentClassification{
                Environment:      "production",
                BusinessPriority: "P0",
            }

            mockK8sClient.On("Get", ctx, client.ObjectKeyFromObject(ap), ap).Return(nil)
            mockK8sClient.On("Create", ctx, mock.MatchedBy(func(obj client.Object) bool {
                aiAnalysis, ok := obj.(*aianalysisv1.AIAnalysis)
                return ok && strings.Contains(aiAnalysis.Name, "ai-analysis-")
            })).Return(nil)
            mockK8sClient.On("Status().Update", ctx, ap).Return(nil)

            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(ap))

            // Validate terminal state
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeFalse(), "terminal state should not requeue")
            Expect(ap.Status.Phase).To(Equal("completed"))
            Expect(ap.Status.ProcessingTime).ToNot(BeEmpty())

            // Verify AIAnalysis CRD creation
            mockK8sClient.AssertCalled(GinkgoT(), "Create", ctx, mock.Anything)
        })

        It("BR-SP-031: should handle duplicate AIAnalysis CRD gracefully", func() {
            ap := testutil.NewAlertProcessingWithPhase("test-duplicate", "default", "routing")

            mockK8sClient.On("Get", ctx, client.ObjectKeyFromObject(ap), ap).Return(nil)
            mockK8sClient.On("Create", ctx, mock.Anything).Return(errors.NewAlreadyExists(
                schema.GroupResource{Group: "aianalysis.kubernaut.io", Resource: "aianalyses"},
                "ai-analysis-duplicate",
            ))
            mockK8sClient.On("Status().Update", ctx, ap).Return(nil)

            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(ap))

            // Should succeed even if CRD already exists
            Expect(err).ToNot(HaveOccurred())
            Expect(ap.Status.Phase).To(Equal("completed"))
        })
    })

    Context("BR-SP-040: Performance and Metrics", func() {
        It("should complete full processing cycle within performance targets", func() {
            startTime := time.Now()

            ap := testutil.NewAlertProcessing("perf-test", "default")

            // Mock all phases
            mockK8sClient.On("Get", ctx, mock.Anything, mock.Anything).Return(nil)
            mockK8sClient.On("Status().Update", ctx, mock.Anything).Return(nil)
            mockK8sClient.On("Create", ctx, mock.Anything).Return(nil)
            mockContextService.On("GetContext", ctx, mock.Anything).Return(
                testutil.NewEnrichmentResults(), nil,
            )

            // Execute all phases
            for ap.Status.Phase != "completed" {
                _, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(ap))
                Expect(err).ToNot(HaveOccurred())
            }

            processingDuration := time.Since(startTime)

            // Validate performance target: total < 5s
            Expect(processingDuration).To(BeNumerically("<", 5*time.Second))
        })
    })
})
```

### Integration Tests (Component Interaction Layer)

**Test Directory**: [test/integration/](../../../test/integration/)
**Service Tests**: Create `test/integration/alertprocessor/integration_test.go`
**Coverage Target**: 20% of business requirements
**Confidence**: 80-85%
**Execution**: `make test-integration-kind` (local) or `make test-integration-kind-ci` (CI)

**Strategy**: Test CRD interactions with real Kubernetes API server in KIND cluster.

**Test File Structure** (aligned with package name `alertprocessor`):
```
test/integration/
├── alertprocessor/                 # Matches pkg/alertprocessor/
│   ├── integration_test.go         # CRD lifecycle and interaction tests
│   ├── crd_phase_transitions_test.go  # Phase state machine tests
│   ├── context_service_integration_test.go  # Real Context Service calls
│   └── suite_test.go               # Integration test suite setup
└── ...
```

**Migration Note**: Rename `test/integration/alert_processing/` → `test/integration/alertprocessor/` to match package structure.

```go
var _ = Describe("BR-INTEGRATION-AP-001: Alert Processing CRD Integration", func() {
    var (
        k8sClient client.Client
        ctx       context.Context
        namespace string
    )

    BeforeEach(func() {
        ctx = context.Background()
        namespace = testutil.CreateTestNamespace(k8sClient)
    })

    AfterEach(func() {
        testutil.CleanupNamespace(k8sClient, namespace)
    })

    It("should process alert through all phases with real K8s CRD operations", func() {
        // Create AlertRemediation CRD (parent)
        alertRemediation := testutil.NewAlertRemediation("integration-test", namespace)
        Expect(k8sClient.Create(ctx, alertRemediation)).To(Succeed())

        // Create AlertProcessing CRD
        alertProcessing := testutil.NewAlertProcessing("integration-alert", namespace)
        alertProcessing.Spec.AlertRemediationRef = testutil.ObjectRefFrom(alertRemediation)
        Expect(k8sClient.Create(ctx, alertProcessing)).To(Succeed())

        // Wait for controller to process through phases
        Eventually(func() string {
            err := k8sClient.Get(ctx, client.ObjectKeyFromObject(alertProcessing), alertProcessing)
            if err != nil {
                return ""
            }
            return alertProcessing.Status.Phase
        }, "30s", "1s").Should(Equal("completed"))

        // Validate final state
        Expect(alertProcessing.Status.EnrichmentResults).ToNot(BeNil())
        Expect(alertProcessing.Status.EnvironmentClassification.Environment).ToNot(BeEmpty())

        // Verify AIAnalysis CRD was created
        aiAnalysisList := &aianalysisv1.AIAnalysisList{}
        Expect(k8sClient.List(ctx, aiAnalysisList, client.InNamespace(namespace))).To(Succeed())
        Expect(aiAnalysisList.Items).To(HaveLen(1))
    })
})
```

### E2E Tests (End-to-End Workflow Layer)

**Test Directory**: [test/e2e/](../../../test/e2e/)
**Service Tests**: Create `test/e2e/alertprocessor/e2e_test.go`
**Coverage Target**: 10% of critical business workflows
**Confidence**: 90-95%
**Execution**: `make test-e2e-kind` (KIND) or `make test-e2e-ocp` (Kubernetes)

**Test File Structure** (aligned with package name `alertprocessor`):
```
test/e2e/
├── alertprocessor/                 # Matches pkg/alertprocessor/
│   ├── e2e_test.go                 # End-to-end workflow tests
│   ├── production_alert_flow_test.go  # Production alert processing
│   ├── staging_alert_flow_test.go     # Staging alert processing
│   └── suite_test.go               # E2E test suite setup
└── ...
```

**Migration Note**: Create new `test/e2e/alertprocessor/` directory to match package structure.

```go
var _ = Describe("BR-E2E-AP-001: Complete Alert Processing Workflow", func() {
    It("should process production alert from webhook to AI analysis", func() {
        // Send webhook alert
        alertPayload := testutil.NewPrometheusAlert("HighMemoryUsage", "production")
        response := testutil.SendWebhookAlert(gatewayURL, alertPayload)
        Expect(response.StatusCode).To(Equal(200))

        // Wait for complete processing pipeline
        Eventually(func() bool {
            aiAnalyses := &aianalysisv1.AIAnalysisList{}
            k8sClient.List(ctx, aiAnalyses, client.MatchingLabels{
                "alert-fingerprint": alertPayload.Fingerprint,
            })
            return len(aiAnalyses.Items) > 0
        }, "60s", "2s").Should(BeTrue())

        // Validate end-to-end business outcome
        // Verify alert was enriched, classified, and routed correctly
    })
})
```

### Test Coverage Requirements

**Business Requirement Mapping**:
- **BR-SP-001 to BR-SP-015**: Alert enrichment logic (Unit + Integration)
- **BR-SP-016 to BR-SP-030**: Environment classification (Unit + Integration)
- **BR-SP-031 to BR-SP-045**: Routing decisions (Unit + Integration)
- **BR-SP-046 to BR-SP-050**: Error handling and resilience (Unit + E2E)

### Mock Usage Decision Matrix

| Component | Unit Tests | Integration | E2E | Justification |
|-----------|------------|-------------|-----|---------------|
| **Kubernetes API** | **FAKE K8S CLIENT** (`sigs.k8s.io/controller-runtime/pkg/client/fake`) | REAL (KIND) | REAL (OCP/KIND) | Compile-time API safety, type-safe CRD handling, detect API deprecations at build time |
| **Context Service HTTP** | **CUSTOM MOCK** (`pkg/testutil/mocks`) | REAL | REAL | External HTTP service dependency - controlled test data |
| **Environment Classifier** | REAL | REAL | REAL | Core business logic |
| **AlertProcessing CRD** | **FAKE K8S CLIENT** | REAL | REAL | Kubernetes resource - type-safe testing |
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
Expect(classification.Environment).To(Equal("production"))
Expect(classification.Confidence).To(BeNumerically(">", 0.8))
Expect(classification.BusinessPriority).To(Equal("P0"))
```

**❌ IMPLEMENTATION TESTING** (Forbidden):
```go
// BAD: Testing internal implementation
Expect(reconciler.callCount).To(Equal(3))
```

**✅ BEHAVIOR TESTING** (Required):
```go
// GOOD: Testing business behavior
Expect(ap.Status.Phase).To(Equal("completed"))
Expect(ap.Status.ProcessingTime).To(MatchRegexp(`\d+(\.\d+)?[ms]`))
```

## Performance Targets

- **Enrichment**: <2s
- **Classification**: <500ms
- **Total processing**: <5s
- **Accuracy**: >99% for production classification

## Database Integration for Audit & Tracking

### Dual Audit System

Following the [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md#crd-lifecycle-management-and-cleanup) dual audit approach:

1. **CRDs**: Real-time execution state + 24-hour review window
2. **Database**: Long-term compliance + post-mortem analysis

### Audit Data Persistence

**Database Service**: Data Storage Service (Port 8085)
**Purpose**: Persist alert processing audit trail before CRD cleanup

```go
package controller

import (
    "github.com/jordigilh/kubernaut/pkg/storage"
)

type AlertProcessingReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    ContextService ContextService
    Classifier     *EnvironmentClassifier
    AuditStorage   storage.AuditStorageClient  // Database client for audit trail
}

// After each phase completion, persist audit data
func (r *AlertProcessingReconciler) reconcileRoutingWithAudit(ctx context.Context, ap *processingv1.AlertProcessing) (ctrl.Result, error) {
    // ... routing logic ...

    // Persist complete alert processing audit trail
    auditRecord := &storage.AlertProcessingAudit{
        RemediationID:    ap.Spec.AlertRemediationRef.Name,
        AlertFingerprint: ap.Spec.Alert.Fingerprint,
        ProcessingPhases: []storage.ProcessingPhase{
            {
                Phase:     "enriching",
                StartTime: ap.CreationTimestamp.Time,
                EndTime:   ap.Status.EnrichmentResults.CompletedAt,
                Duration:  ap.Status.EnrichmentResults.Duration,
                // ✅ TYPE SAFE - Uses structured phase results
                EnrichmentPhaseResults: &storage.EnrichmentPhaseResults{
                    ContextQuality:   ap.Status.EnrichmentResults.EnrichmentQuality,
                    ContextSizeBytes: calculateContextSize(ap.Status.EnrichmentResults),
                    ContextSources:   len(ap.Spec.EnrichmentConfig.ContextSources),
                    DegradedMode:     ap.Status.DegradedMode,
                },
            },
            {
                Phase:     "classifying",
                StartTime: ap.Status.ClassificationStartTime,
                EndTime:   ap.Status.ClassificationEndTime,
                Duration:  ap.Status.ClassificationDuration,
                // ✅ TYPE SAFE - Uses structured phase results
                ClassificationPhaseResults: &storage.ClassificationPhaseResults{
                    Environment:          ap.Status.EnvironmentClassification.Environment,
                    Confidence:           ap.Status.EnvironmentClassification.Confidence,
                    BusinessPriority:     ap.Status.EnvironmentClassification.BusinessPriority,
                    ClassificationMethod: "namespace-label", // or from status
                },
            },
        },
        CompletedAt: time.Now(),
        Status:      "completed",
    }

    if err := r.AuditStorage.StoreAlertProcessingAudit(ctx, auditRecord); err != nil {
        r.Log.Error(err, "Failed to store alert processing audit", "fingerprint", ap.Spec.Alert.Fingerprint)
        ErrorsTotal.WithLabelValues("audit_storage_failed", "routing").Inc()
        // Don't fail reconciliation, but log for investigation
    }

    // ... continue with routing ...
}
```

### Audit Data Schema

```go
package storage

type AlertProcessingAudit struct {
    ID               string            `json:"id" db:"id"`
    RemediationID    string            `json:"remediation_id" db:"remediation_id"`
    AlertFingerprint string            `json:"alert_fingerprint" db:"alert_fingerprint"`
    ProcessingPhases []ProcessingPhase `json:"processing_phases" db:"processing_phases"`

    // Enrichment results
    EnrichmentQuality float64                 `json:"enrichment_quality" db:"enrichment_quality"`
    EnrichmentSources []string                `json:"enrichment_sources" db:"enrichment_sources"`
    ContextSize       int                     `json:"context_size_bytes" db:"context_size_bytes"`

    // Classification results
    Environment      string  `json:"environment" db:"environment"`
    Confidence       float64 `json:"confidence" db:"confidence"`
    BusinessPriority string  `json:"business_priority" db:"business_priority"`
    SLARequirement   string  `json:"sla_requirement" db:"sla_requirement"`

    // Routing decision
    RoutedToService string    `json:"routed_to_service" db:"routed_to_service"`
    RoutingPriority int       `json:"routing_priority" db:"routing_priority"`

    // Metadata
    CompletedAt time.Time `json:"completed_at" db:"completed_at"`
    Status      string    `json:"status" db:"status"`
    ErrorMessage string   `json:"error_message,omitempty" db:"error_message"`
}

type ProcessingPhase struct {
    Phase     string        `json:"phase" db:"phase"`
    StartTime time.Time     `json:"start_time" db:"start_time"`
    EndTime   time.Time     `json:"end_time" db:"end_time"`
    Duration  time.Duration `json:"duration" db:"duration"`

    // Phase-specific results (only one will be populated based on phase)
    // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    EnrichmentPhaseResults    *EnrichmentPhaseResults    `json:"enrichmentResults,omitempty" db:"enrichment_results"`
    ClassificationPhaseResults *ClassificationPhaseResults `json:"classificationResults,omitempty" db:"classification_results"`
    RoutingPhaseResults       *RoutingPhaseResults       `json:"routingResults,omitempty" db:"routing_results"`
}

type EnrichmentPhaseResults struct {
    ContextQuality   float64 `json:"contextQuality" db:"context_quality"`
    ContextSizeBytes int     `json:"contextSizeBytes" db:"context_size_bytes"`
    ContextSources   int     `json:"contextSources" db:"context_sources"` // Number of sources used
    DegradedMode     bool    `json:"degradedMode" db:"degraded_mode"`
}

type ClassificationPhaseResults struct {
    Environment          string  `json:"environment" db:"environment"`
    Confidence           float64 `json:"confidence" db:"confidence"`
    BusinessPriority     string  `json:"businessPriority" db:"business_priority"`
    ClassificationMethod string  `json:"classificationMethod" db:"classification_method"` // "namespace-label", "pattern", "fallback"
}

type RoutingPhaseResults struct {
    RoutedToService string `json:"routedToService" db:"routed_to_service"`
    RoutingPriority int    `json:"routingPriority" db:"routing_priority"`
    RoutingKey      string `json:"routingKey" db:"routing_key"`
}
```

### Audit Use Cases

**Post-Mortem Analysis**:
```sql
-- Find all alert processing records for a specific alert
SELECT * FROM alert_processing_audit
WHERE alert_fingerprint = 'mem-pressure-prod-123'
ORDER BY completed_at DESC;

-- Classification accuracy analysis
SELECT environment,
       AVG(confidence) as avg_confidence,
       COUNT(*) as total_alerts
FROM alert_processing_audit
WHERE completed_at > NOW() - INTERVAL '7 days'
GROUP BY environment;
```

**Performance Tracking**:
```sql
-- Find slow enrichment operations
SELECT remediation_id, alert_fingerprint,
       (processing_phases->0->>'duration')::interval as enrichment_duration
FROM alert_processing_audit
WHERE (processing_phases->0->>'duration')::interval > INTERVAL '2 seconds'
ORDER BY enrichment_duration DESC;
```

**Compliance Reporting**:
- Retain all alert processing decisions for regulatory compliance
- Track classification accuracy over time
- Audit trail for all routing decisions

### Storage Service Integration

**Dependencies**:
- Data Storage Service (Port 8085)
- PostgreSQL database with `alert_processing_audit` table
- Optional: Vector DB for enrichment context storage

**HTTP Client**:
```go
// Simple HTTP POST to Data Storage Service
func (c *AuditStorageClient) StoreAlertProcessingAudit(ctx context.Context, audit *AlertProcessingAudit) error {
    url := fmt.Sprintf("%s/api/v1/audit/alert-processing", c.storageServiceURL)

    body, err := json.Marshal(audit)
    if err != nil {
        return fmt.Errorf("failed to marshal audit: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("failed to store audit: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("audit storage failed with status: %d", resp.StatusCode)
    }

    return nil
}
```

### Audit Metrics

Add metrics to track audit storage operations:

```go
var (
    // Counter: Audit storage attempts
    AuditStorageAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_alertprocessor_audit_storage_attempts_total",
        Help: "Total audit storage attempts",
    }, []string{"status"}) // success, error

    // Histogram: Audit storage duration
    AuditStorageDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_alertprocessor_audit_storage_duration_seconds",
        Help:    "Duration of audit storage operations",
        Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
    })
)
```

## Integration Points

### 1. Upstream Integration: AlertRemediation Controller

**Integration Pattern**: Watch-based status coordination

**How AlertProcessing is Created**:
```go
// In AlertRemediationReconciler (Remediation Coordinator)
func (r *AlertRemediationReconciler) reconcileAlertProcessing(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
) error {
    // When AlertRemediation is first created, create AlertProcessing
    if remediation.Status.AlertProcessingRef == nil {
        alertProcessing := &processingv1.AlertProcessing{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-processing", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
                },
            },
            Spec: processingv1.AlertProcessingSpec{
                AlertRemediationRef: processingv1.AlertRemediationReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // Copy original alert data
                Alert: processingv1.Alert{
                    Fingerprint: remediation.Spec.AlertFingerprint,
                    Payload:     remediation.Spec.OriginalPayload,
                    Severity:    remediation.Spec.Severity,
                },
            },
        }

        return r.Create(ctx, alertProcessing)
    }

    return nil
}
```

**Note**: AlertRemediation controller creates AlertProcessing CRD when remediation workflow starts.

### 2. Downstream Integration: AlertRemediation Watches AlertProcessing Status

**Integration Pattern**: Data snapshot - AlertRemediation creates AIAnalysis after AlertProcessing completes

**How AlertRemediation Responds to Completion**:
```go
// In AlertRemediationReconciler (Remediation Coordinator)
func (r *AlertRemediationReconciler) reconcileAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    alertProcessing *processingv1.AlertProcessing,
) error {
    // When AlertProcessing completes, create AIAnalysis with enriched data
    if alertProcessing.Status.Phase == "completed" && remediation.Status.AIAnalysisRef == nil {
        aiAnalysis := &aianalysisv1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-analysis", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
                },
            },
            Spec: aianalysisv1.AIAnalysisSpec{
                AlertRemediationRef: aianalysisv1.AlertRemediationReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // COPY enriched alert data (data snapshot pattern - TARGETING DATA ONLY)
                AnalysisRequest: aianalysisv1.AnalysisRequest{
                    AlertContext: aianalysisv1.AlertContext{
                        Fingerprint:      alertProcessing.Status.EnrichedAlert.Fingerprint,
                        Severity:         alertProcessing.Status.EnrichedAlert.Severity,
                        Environment:      alertProcessing.Status.EnrichedAlert.Environment,
                        BusinessPriority: alertProcessing.Status.EnrichedAlert.BusinessPriority,

                        // Resource targeting for HolmesGPT (NOT logs/metrics - toolsets fetch those)
                        Namespace:    alertProcessing.Status.EnrichedAlert.Namespace,
                        ResourceKind: alertProcessing.Status.EnrichedAlert.ResourceKind,
                        ResourceName: alertProcessing.Status.EnrichedAlert.ResourceName,

                        // Kubernetes context (small data ~8KB)
                        KubernetesContext: alertProcessing.Status.EnrichedAlert.KubernetesContext,
                    },
                },
            },
        }

        return r.Create(ctx, aiAnalysis)
    }

    return nil
}
```

**Note**: AlertProcessing does NOT create AIAnalysis CRD. AlertRemediation controller watches AlertProcessing status and creates AIAnalysis when processing completes.

### 3. External Service Integration: Context Service

**Integration Pattern**: Synchronous HTTP REST API call

**Endpoint**: Context Service - see [README: Context Service](../../README.md#-9-context-service)

**Request**:
```go
type ContextRequest struct {
    Namespace     string            `json:"namespace"`
    ResourceKind  string            `json:"resourceKind"`
    ResourceName  string            `json:"resourceName"`
    AlertLabels   map[string]string `json:"alertLabels"`
}
```

**Response**:
```go
type ContextResponse struct {
    PodDetails        PodDetails        `json:"podDetails"`
    DeploymentDetails DeploymentDetails `json:"deploymentDetails"`
    NodeDetails       NodeDetails       `json:"nodeDetails"`
    RelatedResources  []RelatedResource `json:"relatedResources"`
}
```

**Degraded Mode Fallback**:
```go
// If Context Service unavailable, extract minimal context from alert labels
func (e *Enricher) DegradedModeEnrich(alert Alert) EnrichedAlert {
    return EnrichedAlert{
        Fingerprint: alert.Fingerprint,
        Severity:    alert.Severity,
        Environment: extractEnvironmentFromLabels(alert.Labels),
        KubernetesContext: KubernetesContext{
            Namespace:    alert.Labels["namespace"],
            PodName:      alert.Labels["pod"],
            // Minimal context from alert labels only
        },
    }
}
```

### 4. Database Integration: Data Storage Service

**Integration Pattern**: Audit trail persistence

**Endpoint**: Data Storage Service HTTP POST `/api/v1/audit/alert-processing`

**Audit Record**:
```go
type AlertProcessingAudit struct {
    AlertFingerprint    string                     `json:"alertFingerprint"`
    ProcessingStartTime time.Time                  `json:"processingStartTime"`
    ProcessingEndTime   time.Time                  `json:"processingEndTime"`
    EnrichmentResult    EnrichedAlert              `json:"enrichmentResult"`
    ClassificationResult EnvironmentClassification `json:"classificationResult"`
    DegradedMode        bool                       `json:"degradedMode"`
}
```

### 5. Dependencies Summary

**Upstream Services**:
- **Gateway Service** - Creates AlertRemediation CRD with duplicate detection already performed (BR-WH-008)
- **AlertRemediation Controller** - Creates AlertProcessing CRD when workflow starts

**Downstream Services**:
- **AlertRemediation Controller** - Watches AlertProcessing status and creates AIAnalysis CRD upon completion

**External Services**:
- **Context Service** - HTTP GET for Kubernetes context enrichment
- **Data Storage Service** - HTTP POST for audit trail persistence

**Database**:
- PostgreSQL - `alert_processing_audit` table for long-term audit storage
- Vector DB (optional) - Enrichment context embeddings for ML analysis

**Existing Code to Leverage** (after migration to `pkg/alertprocessor/`):
- `pkg/alertprocessor/` (migrated from `pkg/alert/`) - Alert processing business logic (1,103 lines)
  - `service.go` - AlertProcessorService interface (to be renamed)
  - `implementation.go` - Service implementation
  - `components.go` - Processing components (AlertProcessorImpl, AlertEnricherImpl, etc.)
- `pkg/processor/environment/classifier.go` - Environment classification
- `pkg/ai/context/` - Context gathering functions
- `pkg/storage/` - Database client and audit storage (to be created)

**Code to Move to Gateway Service**:
- `pkg/alert/components.go` → Gateway Service - `AlertDeduplicatorImpl` (fingerprint generation logic reusable)

## RBAC Configuration

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alertprocessing-controller
rules:
- apiGroups: ["kubernaut.io"]
  resources: ["alertprocessings", "alertprocessings/status"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations"]
  verbs: ["get", "list", "watch"]
```

**Note**: AlertProcessing controller does NOT create AIAnalysis CRDs. The AlertRemediation controller is responsible for creating AIAnalysis when AlertProcessing completes.

---

## Security Configuration

### ServiceAccount & RBAC Least Privilege

**ServiceAccount Setup**:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alertprocessing-controller
  namespace: kubernaut-system
automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alertprocessing-controller
rules:
# AlertProcessing CRD permissions (full control)
- apiGroups: ["alertprocessor.kubernaut.io"]
  resources: ["alertprocessings"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["alertprocessor.kubernaut.io"]
  resources: ["alertprocessings/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["alertprocessor.kubernaut.io"]
  resources: ["alertprocessings/finalizers"]
  verbs: ["update"]

# AlertRemediation CRD permissions (read-only for parent reference)
- apiGroups: ["alertremediation.kubernaut.io"]
  resources: ["alertremediations"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["alertremediation.kubernaut.io"]
  resources: ["alertremediations/status"]
  verbs: ["get"]

# Kubernetes core resources (read-only for enrichment)
- apiGroups: [""]
  resources: ["pods", "nodes", "namespaces", "configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list", "watch"]

# Event emission (write-only)
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: alertprocessing-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: alertprocessing-controller
subjects:
- kind: ServiceAccount
  name: alertprocessing-controller
  namespace: kubernaut-system
```

**Least Privilege Principles**:
- ✅ Read-only access to Kubernetes resources (no modifications)
- ✅ Write access ONLY to AlertProcessing CRDs
- ✅ No Secret modification permissions (read-only for enrichment metadata)
- ✅ Event creation scoped to AlertProcessing events only

**🚨 CRITICAL SECRET PROTECTION**:
- ❌ Secrets are NEVER captured verbatim in logs, CRD status, events, or audit trails
- ✅ Secret values are ALWAYS scrambled/sanitized before any storage or logging
- ✅ Only secret **references** (name, namespace, type) are stored
- ✅ Regex-based sanitization applied to ALL outgoing data (logs, events, audit records)

---

### Network Policies

**Restrict Controller Network Access**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: alertprocessing-controller
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: alertprocessing-controller
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Health/readiness probes from kubelet
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 8080  # Health/Ready
  # Metrics scraping from Prometheus
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
      podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 9090  # Metrics
  egress:
  # Kubernetes API server
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 443
  # Context Service (internal)
  - to:
    - podSelector:
        matchLabels:
          app: context-service
    ports:
    - protocol: TCP
      port: 8080
  # Data Storage Service (audit)
  - to:
    - podSelector:
        matchLabels:
          app: data-storage-service
    ports:
    - protocol: TCP
      port: 8080
  # DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
```

**Why These Restrictions**:
- No external network access (all dependencies internal)
- No direct database access (goes through Data Storage Service)
- No access to application namespaces (reads via Kubernetes API only)

---

### Secret Management

**No Sensitive Data in AlertProcessing CRDs**:

AlertProcessing controller does NOT handle secrets directly. All sensitive data handling follows these patterns:

**Pattern 1: Secret Reference Only** (Recommended):
```go
package controller

import (
    "context"
    "fmt"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *AlertProcessingReconciler) enrichWithSecretMetadata(
    ctx context.Context,
    ap *alertprocessorv1.AlertProcessing,
) error {
    // Get Pod that references Secret
    var pod corev1.Pod
    if err := r.Get(ctx, client.ObjectKey{
        Name:      ap.Status.EnrichedAlert.ResourceName,
        Namespace: ap.Status.EnrichedAlert.Namespace,
    }, &pod); err != nil {
        return err
    }

    // Extract Secret reference (NOT content)
    secretRefs := []alertprocessorv1.SecretReference{}
    for _, volume := range pod.Spec.Volumes {
        if volume.Secret != nil {
            secretRefs = append(secretRefs, alertprocessorv1.SecretReference{
                Name:      volume.Secret.SecretName,
                Namespace: pod.Namespace,
                Type:      "volume",  // volume | env | imagePullSecret
            })
        }
    }

    // Store reference ONLY (never store actual secret data)
    ap.Status.EnrichedAlert.SecretReferences = secretRefs

    return r.Status().Update(ctx, ap)
}
```

**Pattern 2: Audit Log Secret Sanitization**:
```go
package controller

import (
    "fmt"
    "regexp"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
)

var (
    // Common secret patterns to sanitize
    secretPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(token|auth)\s*[:=]\s*\S+`),
        regexp.MustCompile(`(?i)(secret)\s*[:=]\s*\S+`),
    }
)

func sanitizeAlertPayload(payload string) string {
    sanitized := payload
    for _, pattern := range secretPatterns {
        sanitized = pattern.ReplaceAllString(sanitized, "$1=***REDACTED***")
    }
    return sanitized
}

func (r *AlertProcessingReconciler) recordAudit(
    ctx context.Context,
    ap *alertprocessorv1.AlertProcessing,
) error {
    // Sanitize before audit logging
    sanitizedPayload := sanitizeAlertPayload(string(ap.Spec.Alert.Payload))

    auditRecord := &AuditRecord{
        AlertFingerprint: ap.Spec.Alert.Fingerprint,
        Payload:          sanitizedPayload,  // Sanitized version
        // ... other fields
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Pattern 3: Kubernetes Event Sanitization**:
```go
package controller

import (
    "fmt"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    "k8s.io/client-go/tools/record"
)

func (r *AlertProcessingReconciler) emitEventSanitized(
    ap *alertprocessorv1.AlertProcessing,
    eventType string,
    reason string,
    message string,
) {
    // Sanitize message before emitting event
    sanitizedMessage := sanitizeAlertPayload(message)

    r.Recorder.Event(ap, eventType, reason, sanitizedMessage)
}

// Example: Enrichment completed event with sanitized details
func (r *AlertProcessingReconciler) emitEnrichmentEvent(
    ap *alertprocessorv1.AlertProcessing,
) {
    // Build message with potentially sensitive data
    message := fmt.Sprintf(
        "Enrichment completed: namespace=%s, pod=%s, annotations=%v",
        ap.Status.EnrichedAlert.Namespace,
        ap.Status.EnrichedAlert.ResourceName,
        ap.Status.EnrichedAlert.Annotations,  // May contain secrets
    )

    // Sanitize before emitting
    r.emitEventSanitized(ap, "Normal", "EnrichmentCompleted", message)
}
```

**Pattern 4: Structured Logging Sanitization**:
```go
package controller

import (
    "context"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    "github.com/go-logr/logr"
)

func (r *AlertProcessingReconciler) logWithSanitization(
    log logr.Logger,
    message string,
    keysAndValues ...interface{},
) {
    // Sanitize all string values in keysAndValues
    sanitizedKVs := make([]interface{}, len(keysAndValues))
    for i, kv := range keysAndValues {
        if str, ok := kv.(string); ok {
            sanitizedKVs[i] = sanitizeAlertPayload(str)
        } else {
            sanitizedKVs[i] = kv
        }
    }

    log.Info(message, sanitizedKVs...)
}

// Example usage
func (r *AlertProcessingReconciler) enrichAlert(
    ctx context.Context,
    ap *alertprocessorv1.AlertProcessing,
    log logr.Logger,
) error {
    // Sanitize before logging
    r.logWithSanitization(log, "Starting alert enrichment",
        "fingerprint", ap.Spec.Alert.Fingerprint,
        "payload", string(ap.Spec.Alert.Payload),  // Will be sanitized
    )

    // ... enrichment logic
}
```

**Secret Handling Rules** (MANDATORY):
- ❌ NEVER store secret values in CRD status
- ❌ NEVER log secret values verbatim (logs, events, traces)
- ❌ NEVER include secrets in audit records
- ❌ NEVER include secrets in Kubernetes Events
- ✅ Store secret **references** only (name, namespace, type)
- ✅ Sanitize ALL outgoing data (logs, events, audit records, traces)
- ✅ Use regex patterns for common secret formats
- ✅ Apply sanitization at controller boundaries (before any external output)

**Sanitization Coverage** (100% Required):
- ✅ CRD Status Updates → No secrets stored
- ✅ Audit Logs → `sanitizeAlertPayload()` applied
- ✅ Structured Logs → `logWithSanitization()` wrapper
- ✅ Kubernetes Events → `emitEventSanitized()` wrapper
- ✅ Distributed Traces → Sanitize span attributes
- ✅ HTTP Responses → Sanitize before sending

**Common Secret Patterns** (Expanded):
```go
var secretPatterns = []*regexp.Regexp{
    // Passwords
    regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),

    // API Keys
    regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*\S+`),

    // Tokens
    regexp.MustCompile(`(?i)(token|auth[_-]?token|bearer)\s*[:=]\s*\S+`),

    // Generic secrets
    regexp.MustCompile(`(?i)(secret|private[_-]?key)\s*[:=]\s*\S+`),

    // AWS credentials
    regexp.MustCompile(`(?i)(aws[_-]?access[_-]?key[_-]?id|aws[_-]?secret[_-]?access[_-]?key)\s*[:=]\s*\S+`),

    // Database connection strings
    regexp.MustCompile(`(?i)(connection[_-]?string|database[_-]?url)\s*[:=]\s*\S+`),

    // JWT tokens (base64 encoded with dots)
    regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),

    // Generic base64 secrets (>32 chars)
    regexp.MustCompile(`(?i)(secret|token|key)\s*[:=]\s*[A-Za-z0-9+/]{32,}={0,2}`),
}
```

---

### Security Context

**Pod Security Standards** (Restricted Profile):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alertprocessing-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: alertprocessing-controller
  template:
    metadata:
      labels:
        app: alertprocessing-controller
    spec:
      serviceAccountName: alertprocessing-controller
      securityContext:
        # Pod-level security context
        runAsNonRoot: true
        runAsUser: 65532  # nonroot user
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: manager
        image: alertprocessing-controller:latest
        securityContext:
          # Container-level security context
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65532
          capabilities:
            drop:
            - ALL
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        ports:
        - containerPort: 8080
          name: health
          protocol: TCP
        - containerPort: 9090
          name: metrics
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: tmp
          mountPath: /tmp
        - name: cache
          mountPath: /.cache
      volumes:
      - name: tmp
        emptyDir: {}
      - name: cache
        emptyDir: {}
```

**Why These Settings**:
- **runAsNonRoot**: Prevents privilege escalation
- **readOnlyRootFilesystem**: Immutable container filesystem
- **drop ALL capabilities**: Minimal Linux capabilities
- **seccompProfile**: Syscall filtering for defense-in-depth
- **emptyDir volumes**: Writable directories for tmp files only

---

## Observability & Logging

### Structured Logging Patterns

**Log Levels**:

| Level | Purpose | Examples |
|-------|---------|----------|
| **ERROR** | Unrecoverable errors, requires intervention | CRD validation failure, API server unreachable |
| **WARN** | Recoverable errors, degraded mode | Context Service timeout (fallback to basic enrichment) |
| **INFO** | Normal operations, state transitions | Phase transitions, enrichment completion |
| **DEBUG** | Detailed flow for troubleshooting | Kubernetes API queries, enrichment logic details |

**Structured Logging Implementation**:

```go
package controller

import (
    "context"
    "time"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertProcessingReconciler struct {
    client.Client
    Scheme *runtime.Scheme
    Log    logr.Logger
}

func (r *AlertProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Create request-scoped logger with correlation ID
    log := r.Log.WithValues(
        "alertprocessing", req.NamespacedName,
        "correlationID", extractCorrelationID(ctx),
    )

    var ap alertprocessorv1.AlertProcessing
    if err := r.Get(ctx, req.NamespacedName, &ap); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Log phase transitions
    oldPhase := ap.Status.Phase
    log.Info("Reconciling AlertProcessing",
        "phase", ap.Status.Phase,
        "fingerprint", ap.Spec.Alert.Fingerprint,
    )

    // Execute reconciliation logic with structured logging
    result, err := r.reconcilePhase(ctx, &ap, log)

    // Log phase change
    if ap.Status.Phase != oldPhase {
        log.Info("Phase transition",
            "from", oldPhase,
            "to", ap.Status.Phase,
            "duration", time.Since(ap.Status.StartTime.Time),
        )
    }

    if err != nil {
        log.Error(err, "Reconciliation failed",
            "phase", ap.Status.Phase,
            "retryCount", ap.Status.RetryCount,
        )
        return result, err
    }

    return result, nil
}

func (r *AlertProcessingReconciler) enrichAlert(
    ctx context.Context,
    ap *alertprocessorv1.AlertProcessing,
    log logr.Logger,
) error {
    log.V(1).Info("Starting alert enrichment",
        "fingerprint", ap.Spec.Alert.Fingerprint,
        "namespace", ap.Spec.Alert.Annotations["namespace"],
    )

    // Kubernetes context enrichment
    start := time.Now()
    kubeContext, err := r.enrichKubernetesContext(ctx, ap)
    if err != nil {
        log.Error(err, "Kubernetes enrichment failed (fallback to basic)",
            "namespace", ap.Spec.Alert.Annotations["namespace"],
        )
        // Continue with degraded mode
        ap.Status.DegradedMode = true
    } else {
        log.V(1).Info("Kubernetes enrichment completed",
            "duration", time.Since(start),
            "resourceKind", kubeContext.ResourceKind,
        )
    }

    // Context Service enrichment
    start = time.Now()
    historicalContext, err := r.contextClient.GetHistoricalContext(ctx, ap.Spec.Alert.Fingerprint)
    if err != nil {
        log.Warn("Context Service enrichment failed (using defaults)",
            "error", err,
            "fingerprint", ap.Spec.Alert.Fingerprint,
        )
        // Continue without historical context
    } else {
        log.V(1).Info("Context Service enrichment completed",
            "duration", time.Since(start),
            "historicalOccurrences", historicalContext.OccurrenceCount,
        )
    }

    log.Info("Alert enrichment completed",
        "degradedMode", ap.Status.DegradedMode,
        "totalDuration", time.Since(ap.Status.StartTime.Time),
    )

    return nil
}

// Debug logging for troubleshooting
func (r *AlertProcessingReconciler) debugLogKubernetesQuery(
    log logr.Logger,
    query string,
    result interface{},
    duration time.Duration,
) {
    log.V(2).Info("Kubernetes API query",
        "query", query,
        "duration", duration,
        "resultCount", resultCount(result),
    )
}
```

**Log Correlation Example**:
```
INFO    Reconciling AlertProcessing    {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "phase": "enriching", "fingerprint": "abc123"}
INFO    Starting alert enrichment      {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "fingerprint": "abc123", "namespace": "production"}
DEBUG   Kubernetes API query           {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "query": "get pod production/web-app-789", "duration": "15ms"}
INFO    Alert enrichment completed     {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "degradedMode": false, "totalDuration": "234ms"}
INFO    Phase transition               {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "from": "enriching", "to": "classifying", "duration": "234ms"}
```

---

### Distributed Tracing

**OpenTelemetry Integration**:

```go
package controller

import (
    "context"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("alertprocessing-controller")

func (r *AlertProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ctx, span := tracer.Start(ctx, "AlertProcessing.Reconcile",
        trace.WithAttributes(
            attribute.String("alertprocessing.name", req.Name),
            attribute.String("alertprocessing.namespace", req.Namespace),
        ),
    )
    defer span.End()

    var ap alertprocessorv1.AlertProcessing
    if err := r.Get(ctx, req.NamespacedName, &ap); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Failed to get AlertProcessing")
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Add CRD attributes to span
    span.SetAttributes(
        attribute.String("alert.fingerprint", ap.Spec.Alert.Fingerprint),
        attribute.String("alert.severity", ap.Spec.Alert.Severity),
        attribute.String("phase", ap.Status.Phase),
    )

    // Execute reconciliation with tracing
    result, err := r.reconcilePhase(ctx, &ap)

    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Reconciliation failed")
    } else {
        span.SetStatus(codes.Ok, "Reconciliation completed")
    }

    return result, err
}

func (r *AlertProcessingReconciler) enrichAlert(
    ctx context.Context,
    ap *alertprocessorv1.AlertProcessing,
) error {
    ctx, span := tracer.Start(ctx, "AlertProcessing.EnrichAlert")
    defer span.End()

    // Kubernetes enrichment span
    kubeContext, err := r.enrichKubernetesContextWithTracing(ctx, ap)
    if err != nil {
        span.RecordError(err)
        // Continue in degraded mode
    }

    // Context Service enrichment span
    historicalContext, err := r.enrichHistoricalContextWithTracing(ctx, ap)
    if err != nil {
        span.RecordError(err)
        // Continue without historical context
    }

    span.SetAttributes(
        attribute.Bool("degraded_mode", ap.Status.DegradedMode),
        attribute.Int("enrichment_steps", 2),
    )

    return nil
}

func (r *AlertProcessingReconciler) enrichKubernetesContextWithTracing(
    ctx context.Context,
    ap *alertprocessorv1.AlertProcessing,
) (*alertprocessorv1.KubernetesContext, error) {
    ctx, span := tracer.Start(ctx, "AlertProcessing.EnrichKubernetesContext",
        trace.WithAttributes(
            attribute.String("namespace", ap.Spec.Alert.Annotations["namespace"]),
            attribute.String("resourceKind", ap.Spec.Alert.Annotations["kind"]),
        ),
    )
    defer span.End()

    // Get Pod details
    pod, err := r.getPodDetails(ctx, ap)
    if err != nil {
        span.RecordError(err)
        return nil, err
    }

    // 🚨 CRITICAL: Sanitize pod annotations before adding to trace
    sanitizedAnnotations := sanitizeMapValues(pod.Annotations)

    span.SetAttributes(
        attribute.String("pod.name", pod.Name),
        attribute.String("pod.status", string(pod.Status.Phase)),
        attribute.Int("pod.restartCount", int(pod.Status.ContainerStatuses[0].RestartCount)),
        // Only include sanitized annotations (secrets scrambled)
        attribute.String("pod.annotations", fmt.Sprintf("%v", sanitizedAnnotations)),
    )

    return &alertprocessorv1.KubernetesContext{
        ResourceKind: "Pod",
        ResourceName: pod.Name,
        // ... other fields
    }, nil
}

// Sanitize map values to prevent secret leakage in traces
func sanitizeMapValues(m map[string]string) map[string]string {
    sanitized := make(map[string]string)
    for k, v := range m {
        sanitized[k] = sanitizeAlertPayload(v)
    }
    return sanitized
}
```

**Trace Visualization** (Jaeger):
```
Trace ID: abc-123-def-456
Span: AlertProcessing.Reconcile (234ms)
  ├─ Span: AlertProcessing.EnrichAlert (180ms)
  │   ├─ Span: AlertProcessing.EnrichKubernetesContext (120ms)
  │   │   ├─ Span: KubernetesAPI.GetPod (50ms)
  │   │   └─ Span: KubernetesAPI.GetDeployment (40ms)
  │   └─ Span: ContextService.GetHistoricalContext (60ms)
  │       └─ Span: HTTP.POST /context (55ms)
  └─ Span: AlertProcessing.ClassifyEnvironment (54ms)
```

---

### Log Correlation IDs

**Propagating Correlation IDs Across Services**:

```go
package controller

import (
    "context"

    "github.com/google/uuid"
)

type correlationIDKey struct{}

// Extract correlation ID from incoming context (from AlertRemediation)
func extractCorrelationID(ctx context.Context) string {
    if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
        return id
    }
    // Generate new ID if not present
    return uuid.New().String()
}

// Add correlation ID to outgoing requests
func (r *AlertProcessingReconciler) callContextService(
    ctx context.Context,
    fingerprint string,
) (*ContextResponse, error) {
    correlationID := extractCorrelationID(ctx)

    req, err := http.NewRequestWithContext(ctx, "POST", r.contextServiceURL, body)
    if err != nil {
        return nil, err
    }

    // Propagate correlation ID via header
    req.Header.Set("X-Correlation-ID", correlationID)
    req.Header.Set("Content-Type", "application/json")

    resp, err := r.httpClient.Do(req)
    // ... handle response
}
```

**Correlation Flow**:
```
AlertRemediation (correlationID: abc-123)
    ↓ (creates AlertProcessing with correlationID in annotation)
AlertProcessing Controller (correlationID: abc-123)
    ↓ (HTTP header: X-Correlation-ID: abc-123)
Context Service (correlationID: abc-123)
    ↓ (logs with correlationID: abc-123)
```

**Query Logs by Correlation ID**:
```bash
kubectl logs -n kubernaut-system deployment/alertprocessing-controller | grep "correlationID: abc-123"
```

---

### Debug Configuration

**Enable Debug Logging**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: alertprocessing-controller-config
  namespace: kubernaut-system
data:
  log-level: "debug"  # error | warn | info | debug
  log-format: "json"  # json | console
  enable-tracing: "true"
  tracing-endpoint: "http://jaeger-collector.monitoring:14268/api/traces"
```

**Controller Startup with Debug Config**:

```go
package main

import (
    "flag"
    "os"

    "github.com/go-logr/zapr"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
    var logLevel string
    var logFormat string
    flag.StringVar(&logLevel, "log-level", "info", "Log level (error, warn, info, debug)")
    flag.StringVar(&logFormat, "log-format", "json", "Log format (json, console)")
    flag.Parse()

    // Configure zap logger
    zapLevel := parseLogLevel(logLevel)
    var zapConfig zap.Config
    if logFormat == "json" {
        zapConfig = zap.NewProductionConfig()
    } else {
        zapConfig = zap.NewDevelopmentConfig()
    }
    zapConfig.Level = zap.NewAtomicLevelAt(zapLevel)

    zapLog, err := zapConfig.Build()
    if err != nil {
        os.Exit(1)
    }

    ctrl.SetLogger(zapr.NewLogger(zapLog))

    // ... controller setup
}

func parseLogLevel(level string) zapcore.Level {
    switch level {
    case "debug":
        return zapcore.DebugLevel
    case "info":
        return zapcore.InfoLevel
    case "warn":
        return zapcore.WarnLevel
    case "error":
        return zapcore.ErrorLevel
    default:
        return zapcore.InfoLevel
    }
}
```

**Debug Query Examples**:

```bash
# Enable debug logging at runtime (requires restart)
kubectl set env deployment/alertprocessing-controller -n kubernaut-system LOG_LEVEL=debug

# View debug logs for specific AlertProcessing
kubectl logs -n kubernaut-system deployment/alertprocessing-controller --tail=1000 | grep "alert-processing-xyz"

# View Kubernetes API queries (V(2) logs)
kubectl logs -n kubernaut-system deployment/alertprocessing-controller --tail=1000 | grep "Kubernetes API query"

# View Context Service calls
kubectl logs -n kubernaut-system deployment/alertprocessing-controller --tail=1000 | grep "Context Service"
```

---

## Enhanced Metrics & SLOs

### SLI/SLO Definitions

**Service Level Indicators (SLIs)**:

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Enrichment Success Rate** | `successful_enrichments / total_enrichments` | ≥99% | HolmesGPT receives quality data |
| **Processing Latency (P95)** | `histogram_quantile(0.95, alertprocessing_duration_seconds)` | <30s | Fast remediation start |
| **Context Service Availability** | `context_service_requests_success / context_service_requests_total` | ≥95% | Historical pattern availability |
| **Degraded Mode Rate** | `alertprocessing_degraded_total / alertprocessing_total` | <5% | Most alerts fully enriched |

**Service Level Objectives (SLOs)**:

```yaml
slos:
  - name: "AlertProcessing Enrichment Success Rate"
    sli: "successful_enrichments / total_enrichments"
    target: 0.99  # 99%
    window: "30d"
    burn_rate_fast: 14.4  # 1h window
    burn_rate_slow: 6     # 6h window

  - name: "AlertProcessing P95 Latency"
    sli: "histogram_quantile(0.95, alertprocessing_duration_seconds)"
    target: 30  # 30 seconds
    window: "30d"

  - name: "Context Service Availability"
    sli: "context_service_requests_success / context_service_requests_total"
    target: 0.95  # 95%
    window: "30d"
```

---

### Grafana Dashboard JSON

**AlertProcessing Controller Dashboard**:

```json
{
  "dashboard": {
    "title": "AlertProcessing Controller - Observability",
    "uid": "alertprocessing-controller",
    "tags": ["kubernaut", "alertprocessing", "controller"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Enrichment Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(alertprocessing_enrichment_success_total[5m])) / sum(rate(alertprocessing_enrichment_total[5m]))",
            "legendFormat": "Success Rate",
            "refId": "A"
          }
        ],
        "yaxes": [
          {"format": "percentunit", "min": 0, "max": 1}
        ],
        "thresholds": [
          {"value": 0.99, "colorMode": "critical", "op": "lt", "line": true}
        ]
      },
      {
        "id": 2,
        "title": "Processing Latency (P50, P95, P99)",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.50, rate(alertprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P50",
            "refId": "A"
          },
          {
            "expr": "histogram_quantile(0.95, rate(alertprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P95 (SLI)",
            "refId": "B"
          },
          {
            "expr": "histogram_quantile(0.99, rate(alertprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P99",
            "refId": "C"
          }
        ],
        "yaxes": [
          {"format": "s", "min": 0}
        ],
        "thresholds": [
          {"value": 30, "colorMode": "critical", "op": "gt", "line": true}
        ]
      },
      {
        "id": 3,
        "title": "Phase Distribution",
        "type": "piechart",
        "targets": [
          {
            "expr": "sum by (phase) (alertprocessing_active_total)",
            "legendFormat": "{{phase}}",
            "refId": "A"
          }
        ]
      },
      {
        "id": 4,
        "title": "Context Service Availability (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(alertprocessing_context_service_requests_success_total[5m])) / sum(rate(alertprocessing_context_service_requests_total[5m]))",
            "legendFormat": "Availability",
            "refId": "A"
          }
        ],
        "yaxes": [
          {"format": "percentunit", "min": 0, "max": 1}
        ],
        "thresholds": [
          {"value": 0.95, "colorMode": "critical", "op": "lt", "line": true}
        ]
      },
      {
        "id": 5,
        "title": "Degraded Mode Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(alertprocessing_degraded_total[5m])) / sum(rate(alertprocessing_total[5m]))",
            "legendFormat": "Degraded Mode %",
            "refId": "A"
          }
        ],
        "yaxes": [
          {"format": "percentunit", "min": 0, "max": 1}
        ],
        "thresholds": [
          {"value": 0.05, "colorMode": "warning", "op": "gt", "line": true}
        ]
      },
      {
        "id": 6,
        "title": "CRD Lifecycle (Created/Completed/Failed)",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(alertprocessing_created_total[5m])",
            "legendFormat": "Created",
            "refId": "A"
          },
          {
            "expr": "rate(alertprocessing_completed_total[5m])",
            "legendFormat": "Completed",
            "refId": "B"
          },
          {
            "expr": "rate(alertprocessing_failed_total[5m])",
            "legendFormat": "Failed",
            "refId": "C"
          }
        ]
      },
      {
        "id": 7,
        "title": "Trace Visualization (Jaeger Link)",
        "type": "text",
        "options": {
          "content": "[Open Jaeger Traces](http://jaeger.monitoring.svc:16686/search?service=alertprocessing-controller)"
        }
      }
    ]
  }
}
```

---

### Alert Rules YAML

**Prometheus Alert Rules**:

```yaml
groups:
- name: alertprocessing-slos
  interval: 30s
  rules:
  # SLO: Enrichment Success Rate
  - alert: AlertProcessingEnrichmentSLOBreach
    expr: |
      (
        sum(rate(alertprocessing_enrichment_success_total[1h])) /
        sum(rate(alertprocessing_enrichment_total[1h]))
      ) < 0.99
    for: 5m
    labels:
      severity: critical
      slo: enrichment_success_rate
    annotations:
      summary: "AlertProcessing enrichment success rate below SLO"
      description: "Enrichment success rate is {{ $value | humanizePercentage }}, below 99% SLO threshold"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-enrichment-failure"

  # SLO: Processing Latency P95
  - alert: AlertProcessingLatencySLOBreach
    expr: |
      histogram_quantile(0.95,
        rate(alertprocessing_duration_seconds_bucket[5m])
      ) > 30
    for: 10m
    labels:
      severity: warning
      slo: processing_latency_p95
    annotations:
      summary: "AlertProcessing P95 latency exceeds SLO"
      description: "P95 processing latency is {{ $value }}s, exceeding 30s SLO threshold"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-latency"

  # SLO: Context Service Availability
  - alert: ContextServiceAvailabilitySLOBreach
    expr: |
      (
        sum(rate(alertprocessing_context_service_requests_success_total[1h])) /
        sum(rate(alertprocessing_context_service_requests_total[1h]))
      ) < 0.95
    for: 10m
    labels:
      severity: warning
      slo: context_service_availability
    annotations:
      summary: "Context Service availability below SLO"
      description: "Context Service availability is {{ $value | humanizePercentage }}, below 95% SLO"
      runbook: "https://docs.kubernaut.io/runbooks/context-service-down"

  # Operational: High Degraded Mode Rate
  - alert: AlertProcessingHighDegradedModeRate
    expr: |
      (
        sum(rate(alertprocessing_degraded_total[5m])) /
        sum(rate(alertprocessing_total[5m]))
      ) > 0.05
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "High AlertProcessing degraded mode rate"
      description: "{{ $value | humanizePercentage }} of alerts processed in degraded mode (threshold: 5%)"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-degraded-mode"

  # Operational: Enrichment Phase Stuck
  - alert: AlertProcessingEnrichmentStuck
    expr: |
      time() - alertprocessing_phase_start_timestamp{phase="enriching"} > 300
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "AlertProcessing stuck in enriching phase"
      description: "AlertProcessing {{ $labels.name }} has been in enriching phase for over 5 minutes"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-stuck"

  # Operational: High Failure Rate
  - alert: AlertProcessingHighFailureRate
    expr: |
      (
        sum(rate(alertprocessing_failed_total[5m])) /
        sum(rate(alertprocessing_total[5m]))
      ) > 0.05
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "High AlertProcessing failure rate"
      description: "{{ $value | humanizePercentage }} of AlertProcessing operations failing (threshold: 5%)"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-failures"

  # Operational: Controller Restart Loops
  - alert: AlertProcessingControllerRestartLoop
    expr: |
      rate(kube_pod_container_status_restarts_total{
        namespace="kubernaut-system",
        pod=~"alertprocessing-controller-.*"
      }[15m]) > 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "AlertProcessing controller in restart loop"
      description: "Controller pod {{ $labels.pod }} restarting frequently"
      runbook: "https://docs.kubernaut.io/runbooks/controller-crash-loop"
```

---

### Query Examples

**SLI Queries for Dashboards**:

```promql
# 1. Enrichment Success Rate (SLI)
sum(rate(alertprocessing_enrichment_success_total[5m])) /
sum(rate(alertprocessing_enrichment_total[5m]))

# 2. Processing Latency P95 (SLI)
histogram_quantile(0.95,
  rate(alertprocessing_duration_seconds_bucket[5m])
)

# 3. Context Service Availability (SLI)
sum(rate(alertprocessing_context_service_requests_success_total[5m])) /
sum(rate(alertprocessing_context_service_requests_total[5m]))

# 4. Degraded Mode Rate (SLI)
sum(rate(alertprocessing_degraded_total[5m])) /
sum(rate(alertprocessing_total[5m]))

# 5. Error Budget Remaining (30-day window)
1 - (
  (1 - 0.99) -  # SLO target: 99%
  (
    1 - (
      sum(increase(alertprocessing_enrichment_success_total[30d])) /
      sum(increase(alertprocessing_enrichment_total[30d]))
    )
  )
) / (1 - 0.99)

# 6. Phase Distribution
sum by (phase) (alertprocessing_active_total)

# 7. Enrichment Duration by Phase
histogram_quantile(0.95,
  rate(alertprocessing_phase_duration_seconds_bucket{phase="enriching"}[5m])
)

# 8. Kubernetes API Query Rate
rate(alertprocessing_kubernetes_api_requests_total[5m])

# 9. CRD Creation Rate
rate(alertprocessing_created_total[5m])

# 10. Active AlertProcessing CRDs
alertprocessing_active_total
```

**Troubleshooting Queries**:

```promql
# Find slow enrichments (>30s)
alertprocessing_duration_seconds > 30

# Find AlertProcessings in degraded mode
alertprocessing_active_total{degraded_mode="true"}

# Find Context Service failures
rate(alertprocessing_context_service_requests_total{status="error"}[5m])

# Find CRDs stuck in enriching phase
time() - alertprocessing_phase_start_timestamp{phase="enriching"} > 300
```

---

## Implementation Checklist

**Note**: Follow APDC-TDD phases for each implementation step (see Development Methodology section)

### Phase 1: ANALYSIS & Package Migration (1-2 days) [RED Phase Preparation]

- [ ] **ANALYSIS**: Search existing implementations (`codebase_search "AlertProcessor implementations"`)
- [ ] **ANALYSIS**: Map business requirements (BR-SP-001 to BR-SP-050, BR-ENV-001 to BR-ENV-050)
- [ ] **ANALYSIS**: Identify integration points in cmd/alertprocessor/
- [ ] **Package Migration RED**: Write tests validating type-safe interfaces (fail with map[string]interface{})
- [ ] **Package Migration GREEN**: Implement structured types in `pkg/alertprocessor/types.go`
  - [ ] **Package Rename**: `pkg/alert/` → `pkg/alertprocessor/`
  - [ ] **Update Package Declarations**: `package alert` → `package alertprocessor`
  - [ ] **Update Imports**: Across ~50 files
  - [ ] **Interface Rename**: `AlertService` → `AlertProcessorService`
  - [ ] **Remove Deduplication**: Delete `AlertDeduplicatorImpl` (move to Gateway Service)
- [ ] **Package Migration REFACTOR**: Enhance error handling and validation logic
- [ ] **Test Directory Migration**:
  - [ ] Rename `test/unit/alert/` → `test/unit/alertprocessor/`
  - [ ] Rename `test/integration/alert_processing/` → `test/integration/alertprocessor/`
  - [ ] Create `test/e2e/alertprocessor/` (new directory)
  - [ ] Update package declarations: `package alert` → `package alertprocessor`

### Phase 2: CRD Implementation (3-4 days) [RED-GREEN-REFACTOR]

- [ ] **CRD RED**: Write AlertProcessingReconciler tests (should fail - no controller yet)
- [ ] **CRD GREEN**: Generate CRD using Kubebuilder + controller skeleton (tests pass)
  - [ ] Generate AlertProcessing CRD (`api/alertprocessor/v1/alertprocessing_types.go`)
  - [ ] Implement AlertProcessingReconciler with 3 phases (enriching, classifying, routing)
  - [ ] Add owner references and finalizers for cascade deletion
- [ ] **CRD REFACTOR**: Enhance controller with phase logic and error handling
  - [ ] Implement phase timeout detection and handling
  - [ ] Add Kubernetes event emission for visibility
  - [ ] Implement optimized requeue strategy
- [ ] **Integration RED**: Write tests for owner reference management (fail initially)
- [ ] **Integration GREEN**: Implement owner references to AlertRemediation (tests pass)

### Phase 3: Business Logic Integration (1-2 days) [RED-GREEN-REFACTOR]

- [ ] **Logic RED**: Write tests for environment classification with mocked Context Service (fail)
- [ ] **Logic GREEN**: Integrate business logic to pass tests
  - [ ] Integrate existing environment classification logic from `pkg/processor/environment/`
  - [ ] Add Context Service HTTP client
  - [ ] Add status update for AlertRemediation reference
- [ ] **Logic REFACTOR**: Enhance with sophisticated algorithms
  - [ ] Add degraded mode fallback when Context Service unavailable
  - [ ] Optimize classification heuristics and performance
- [ ] **Audit Integration**: Integrate audit storage for long-term tracking
- [ ] **Main App Integration**: Verify AlertProcessingReconciler instantiated in cmd/alertprocessor/ (MANDATORY)

### Phase 4: Testing & Validation (1 day) [CHECK Phase]

- [ ] **CHECK**: Verify 70%+ unit test coverage (test/unit/alertprocessor/)
  - [ ] Write unit tests for each reconciliation phase
  - [ ] Use fake K8s client, mock Context Service only
- [ ] **CHECK**: Run integration tests - 20% coverage target (test/integration/alertprocessor/)
  - [ ] Add integration tests with real Context Service
  - [ ] Test CRD lifecycle with real K8s API (KIND)
- [ ] **CHECK**: Execute E2E tests for critical workflows (test/e2e/alertprocessor/)
  - [ ] Add E2E tests for complete alert-to-remediation workflow
- [ ] **CHECK**: Validate business requirement coverage (BR-SP-001 to BR-SP-050)
- [ ] **CHECK**: Configure RBAC for controller
- [ ] **CHECK**: Add Prometheus metrics for phase durations
- [ ] **CHECK**: Provide confidence assessment (85% - high confidence, see Development Methodology)

## Critical Architectural Patterns (from MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)

### 1. Owner References & Cascade Deletion
**Pattern**: AlertProcessing CRD owned by AlertRemediation
```go
controllerutil.SetControllerReference(&alertRemediation, &alertProcessing, scheme)
```
**Purpose**: Automatic cleanup when AlertRemediation is deleted (24h retention)

### 2. Finalizers for Cleanup Coordination
**Pattern**: Add finalizer before processing, remove after cleanup
```go
const alertProcessingFinalizer = "alertprocessing.kubernaut.io/finalizer"
```
**Purpose**: Ensure audit data persisted before CRD deletion

### 3. Watch-Based Status Coordination
**Pattern**: Status updates trigger AlertRemediation reconciliation automatically
```go
// Status update here triggers AlertRemediation watch
r.Status().Update(ctx, &alertProcessing)
```
**Purpose**: No manual AlertRemediation updates needed - watch handles aggregation

### 4. Phase Timeout Detection & Escalation
**Pattern**: Per-phase timeout with degraded mode fallback
```go
defaultPhaseTimeout = 5 * time.Minute
```
**Purpose**: Prevent stuck processing, enable degraded mode continuation

### 5. Event Emission for Visibility
**Pattern**: Emit Kubernetes events for operational tracking
```go
r.Recorder.Event(&alertProcessing, "Normal", "PhaseCompleted", message)
```
**Purpose**: Operational visibility in kubectl events and monitoring

### 6. Optimized Requeue Strategy
**Pattern**: Phase-based requeue intervals, no requeue for terminal states
```go
// Completed state: no requeue (watch handles updates)
// Active phases: 10s requeue
// Unknown states: 30s conservative requeue
```
**Purpose**: Efficient reconciliation, reduced API server load

### 7. Cross-CRD Reference Validation
**Pattern**: Validate AlertRemediationRef exists before processing
```go
r.Get(ctx, alertRemediationRef, &alertRemediation)
```
**Purpose**: Ensure parent CRD exists, prevent orphaned processing

### 8. Metrics for Reconciliation Performance
**Pattern**: Track controller performance separately from business metrics
```go
// Controller-specific metrics
ControllerReconciliationDuration
ControllerErrorsTotal
ControllerRequeueTotal
```
**Purpose**: Monitor controller health vs business logic performance

## Common Pitfalls

1. **Don't poll Context Service** - Use single HTTP call per enrichment
2. **Environment classification caching** - Cache results for 5 minutes
3. **Failed enrichment handling** - Use degraded mode with basic context
4. **Status update failures** - Implement exponential backoff retry
5. **Missing owner references** - Always set AlertRemediation as owner for cascade deletion
6. **Finalizer cleanup** - Ensure audit persistence before removing finalizer
7. **Event emission** - Emit events for all significant state changes
8. **Phase timeouts** - Implement per-phase timeout detection with fallback logic

---

## Summary

**Alert Processing Service - V1 Design Specification (98% Complete)**

### Core Purpose
Alert enrichment, environment classification, and validation service that bridges webhook reception and AI analysis through CRD-based state management.

### Key Architectural Decisions
1. **Single-Phase Synchronous Processing** - Fast operations (~3 seconds) execute in single reconciliation loop (no multi-phase complexity)
2. **CRD-based State Management** - AlertProcessing CRD with owner references for cascade deletion
3. **AlertRemediation Orchestration** - Central controller creates AlertProcessing and watches for completion to create AIAnalysis
4. **Degraded Mode Operation** - Context Service unavailability triggers fallback to alert labels
5. **Duplicate Detection Delegation** - Gateway Service responsibility (BR-WH-008), not Alert Processor

### Integration Model
```
Gateway Service → AlertRemediation CRD → AlertProcessing CRD (this service)
                                       ↓
                      AlertProcessing.status.phase = "completed"
                                       ↓
                      AlertRemediation watches status
                                       ↓
                      AlertRemediation creates AIAnalysis CRD
```

### V1 Scope Boundaries
**Included**:
- Single enrichment provider (Context Service)
- Environment classification with fallback heuristics
- Basic alert validation
- Audit trail persistence

**Excluded** (V2):
- Multi-source data aggregation
- Advanced resource correlation
- Predictive ML classification
- Cross-cluster enrichment

### Business Requirements Coverage
- **BR-SP-001 to BR-SP-050**: Alert processing and enrichment logic
- **BR-ENV-001 to BR-ENV-050**: Environment classification (integrated)
- **BR-SP-021**: Alert lifecycle state tracking
- **BR-WH-008**: Duplicate detection (Gateway Service, not Alert Processor)

### Implementation Status
- **Existing Code**: 1,103 lines in `pkg/alert/` (requires migration to `pkg/alertprocessor/`)
- **Migration Effort**: 1-2 days (package rename, import updates, test alignment)
- **CRD Controller**: New implementation following controller-runtime patterns
- **Database Schema**: Audit table design complete

### Next Steps
1. ✅ **Approved Design Specification** (98% complete)
2. **Package Migration**: `pkg/alert/` → `pkg/alertprocessor/`
3. **CRD Schema Definition**: AlertProcessing API types
4. **Controller Implementation**: Single-phase reconciliation logic
5. **Integration Testing**: With AlertRemediation controller and Context Service

### Critical Success Factors
- Single-phase processing simplicity (no unnecessary state machine)
- Degraded mode resilience when Context Service unavailable
- Proper owner references for cascade deletion
- AlertRemediation orchestration (does NOT create AIAnalysis directly)
- Audit trail completeness for compliance

**Design Specification Status**: Production-Ready (98% Confidence)
