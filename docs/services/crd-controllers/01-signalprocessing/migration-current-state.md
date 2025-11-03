## Current State & Migration Path

---

## ⚠️ NAMING DEPRECATION NOTICE

**ALERT PREFIX DEPRECATED**: This document contains type definitions using **"Alert" prefix** (e.g., `AlertService`, `AlertProcessorService`, `ProcessAlert()`), which is **DEPRECATED** and being migrated to **"Signal" prefix** to reflect multi-signal architecture.

**Why Deprecated**: Kubernaut processes multiple signal types (Prometheus alerts, Kubernetes events, AWS CloudWatch alarms), not just alerts. The "Alert" prefix creates semantic confusion.

**Migration Decision**: [ADR-015: Alert to Signal Naming Migration](../../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)

**Current Naming Standards**:
- `AlertService` → **`SignalProcessorService`**
- `ProcessAlert()` → **`ProcessSignal()`**
- `AlertContext` → **`SignalContext`**
- `AlertMetrics` → **`SignalProcessingMetrics`**

**⚠️ When implementing**: Use Signal-prefixed names. The Alert-prefixed types shown below are **for migration reference only** and will be replaced in Phase 1 of the migration (see ADR-015).

---

### Existing Business Logic (Verified)

**Current Location**: `pkg/alert/` (1,103 lines of reusable code)
**Target Location**: `pkg/remediationprocessing/` (after migration)

```
pkg/alert/ → pkg/remediationprocessing/
├── service.go (109 lines)          ✅ AlertService → AlertProcessorService interface
├── implementation.go (282 lines)   ✅ Complete 4-step processing pipeline
└── components.go (712 lines)       ✅ All business logic components
```

**Existing Tests** (Verified - to be migrated):
- `test/unit/alert/` → `test/unit/remediationprocessing/` - Unit tests with Ginkgo/Gomega
- `test/integration/alert_processing/` → `test/integration/remediationprocessing/` - Integration tests

### Business Logic Components (Highly Reusable)

**AlertService Interface** - `pkg/alert/service.go:13-38` (to be migrated to `pkg/remediationprocessing/service.go`)

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
// pkg/remediationprocessing/service.go
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

// Supporting structured types (create in pkg/remediationprocessing/types.go)
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
- **Package Rename**: `pkg/alert/` → `pkg/remediationprocessing/` (4 hours)
- **Interface Rename**: `AlertService` → `AlertProcessorService`
- **Type Safety Refactor**: `map[string]interface{}` → 7 structured types (violates coding standards)
- **Deduplication Removal**: `DeduplicationStats` removed - Gateway Service responsibility (BR-WH-008)
- **Strategy**: Parallel implementation to avoid breaking changes
- **Total Effort**: 1-2 days for complete migration

**4-Step Processing Pipeline** - `pkg/alert/implementation.go:75-118` (to be migrated to `pkg/remediationprocessing/`)
```go
func (s *ServiceImpl) ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error) {
    // Step 1: Validate alert
    validation := s.ValidateAlert(alert)

    // Note: Deduplication handled by Gateway Service (BR-WH-008)
    // Remediation Processor receives only non-duplicate alerts

    // Step 2: Enrich alert with context
    enrichment := s.EnrichAlert(ctx, alert)

    // Step 3: Route alert to appropriate handler
    routing := s.RouteAlert(ctx, alert)

    // Step 4: Persist alert to storage
    persistence := s.PersistAlert(ctx, alert)
}
```

**Business Components** - `pkg/alert/components.go` (to be migrated to `pkg/remediationprocessing/components.go`)
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
func (r *RemediationProcessingReconciler) Reconcile(ctx, req) (ctrl.Result, error) {
    // SignalProcessing CRD only created for non-duplicate alerts
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

**Removed from Remediation Processor**: `AlertDeduplicatorImpl` - Moved to Gateway Service (BR-WH-008)

**Interface Refactoring Required**:
- **Package Migration**: `pkg/alert/` → `pkg/remediationprocessing/`
- **Interface Rename**: `AlertService` → `AlertProcessorService`
- Replace all `map[string]interface{}` return types with 7 structured types
- Add proper error returns to all methods (except ProcessAlert which already has error)
- Create `pkg/remediationprocessing/types.go` for all result type definitions
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
- ❌ SignalProcessing CRD schema (need to create)
- ❌ RemediationProcessingReconciler controller (need to create)
- ❌ CRD lifecycle management (owner refs, finalizers)
- ❌ Watch-based status coordination
- ❌ Phase timeout detection
- ❌ Event emission for visibility

**Code Quality Issues to Address**:
- ⚠️ **Package Migration & Type Safety Refactor**: Current implementation needs modernization
  - **Package Rename**: `pkg/alert/` → `pkg/remediationprocessing/` (~4 hours)
  - **Interface Rename**: `AlertService` → `AlertProcessorService`
  - Violates coding standards (`.cursor/rules/00-project-guidelines.mdc`) - uses `map[string]interface{}` anti-pattern
  - Replace with 7 structured types (`*ValidationResult`, `*RoutingResult`, `*EnrichmentResult`, `*PersistenceResult`, `*AlertHistoryResult`, `*AlertMetrics`, `*HealthStatus`)
  - Remove `GetDeduplicationStats()` method (Gateway Service responsibility - BR-WH-008)
  - Add proper error handling to all methods
  - Create `pkg/remediationprocessing/types.go` for result type definitions
  - Estimated effort: 1-2 days (can be done in parallel with CRD work)

**Estimated Migration Effort**: 5-7 days
- Day 1: Type safety refactor (structured types + error handling)
- Day 2: CRD schema + controller skeleton
- Day 3-4: Business logic integration into reconciliation phases
- Day 5: Testing and refinement
- Day 6: Integration with type-safe interfaces
- Day 7: Documentation and deployment

