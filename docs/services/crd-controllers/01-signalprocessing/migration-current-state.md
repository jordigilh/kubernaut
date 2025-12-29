## Current State & Migration Path

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.2 | 2025-11-28 | Gateway migration scope added, CRD location fixed (kubernaut.io/v1alpha1), implementation effort updated | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
> | v1.1 | 2025-11-27 | Service rename: RemediationProcessing → SignalProcessing | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.1 | 2025-11-27 | Terminology: Alert → Signal | [ADR-015](../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md) |
> | v1.1 | 2025-11-27 | Package path: pkg/signalprocessing/ | - |
> | v1.0 | 2025-01-15 | Initial migration analysis | - |

---

## ⚠️ NAMING MIGRATION COMPLETE

**"Alert" prefix MIGRATED to "Signal"** per [ADR-015: Alert to Signal Naming Migration](../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md).

**Why Migrated**: Kubernaut processes multiple signal types (Prometheus alerts, Kubernetes events, AWS CloudWatch alarms), not just alerts. The "Signal" prefix reflects multi-signal architecture.

**Service Renamed**: `RemediationProcessing` → `SignalProcessing` per [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md).

**Current Naming Standards**:
- `AlertService` → **`SignalProcessingService`**
- `ProcessAlert()` → **`ProcessSignal()`**
- `AlertContext` → **`SignalContext`**
- `AlertMetrics` → **`SignalProcessingMetrics`**
- `RemediationProcessing` → **`SignalProcessing`**
- `RemediationProcessingReconciler` → **`SignalProcessingReconciler`**

---

### Existing Business Logic (To Be Created)

**Target Location**: `pkg/signalprocessing/`

```
pkg/signalprocessing/
├── service.go               ✅ SignalProcessingService interface
├── implementation.go        ✅ Complete 4-step processing pipeline
├── components.go            ✅ All business logic components
└── types.go                 ✅ Type-safe result types
```

**Target Tests**:
- `test/unit/signalprocessing/` - Unit tests with Ginkgo/Gomega
- `test/integration/signalprocessing/` - Integration tests
- `test/e2e/signalprocessing/` - E2E tests

### Business Logic Components

**SignalProcessingService Interface** - `pkg/signalprocessing/service.go`

```go
// pkg/signalprocessing/service.go
package signalprocessing

import (
    "context"
    "time"
    "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// SignalProcessingService defines signal processing operations
// Business Requirements: BR-SP-001 to BR-SP-075
type SignalProcessingService interface {
    // Core processing
    ProcessSignal(ctx context.Context, signal types.Signal) (*ProcessResult, error)

    // Validation
    ValidateSignal(signal types.Signal) (*ValidationResult, error)

    // Enrichment
    EnrichSignal(ctx context.Context, signal types.Signal) (*EnrichmentResult, error)

    // Classification
    ClassifyEnvironment(ctx context.Context, signal types.Signal, enrichment *EnrichmentResult) (*ClassificationResult, error)

    // Categorization (DD-CATEGORIZATION-001)
    CategorizeSignal(ctx context.Context, signal types.Signal, enrichment *EnrichmentResult, classification *ClassificationResult) (*CategorizationResult, error)

    // Persistence (via Data Storage Service - ADR-032)
    PersistAudit(ctx context.Context, signal types.Signal, result *ProcessResult) (*PersistenceResult, error)

    // Metrics and Health
    GetMetrics() (*SignalProcessingMetrics, error)
    Health() (*HealthStatus, error)
}

// Supporting structured types (in pkg/signalprocessing/types.go)
type ValidationResult struct {
    Valid  bool     `json:"valid"`
    Errors []string `json:"errors,omitempty"`
}

type EnrichmentResult struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    RecoveryContext   *RecoveryContext   `json:"recoveryContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality"`
    EnrichedAt        time.Time          `json:"enrichedAt"`
}

type ClassificationResult struct {
    Environment         string  `json:"environment"`
    Confidence          float64 `json:"confidence"`
    BusinessCriticality string  `json:"businessCriticality"`
    SLARequirement      string  `json:"slaRequirement"`
    ClassifiedAt        time.Time `json:"classifiedAt"`
}

type CategorizationResult struct {
    Priority             string              `json:"priority"`
    PriorityScore        int                 `json:"priorityScore"`
    CategorizationSource string              `json:"categorizationSource"`
    Factors              []CategorizationFactor `json:"factors,omitempty"`
    CategorizedAt        time.Time           `json:"categorizedAt"`
}

type CategorizationFactor struct {
    Factor       string  `json:"factor"`
    Value        string  `json:"value"`
    Weight       float64 `json:"weight"`
    Contribution int     `json:"contribution"`
}

type PersistenceResult struct {
    Persisted bool      `json:"persisted"`
    AuditID   string    `json:"auditId,omitempty"`
    Timestamp time.Time `json:"timestamp"`
}

type SignalProcessingMetrics struct {
    SignalsIngested   int       `json:"signalsIngested"`
    SignalsValidated  int       `json:"signalsValidated"`
    SignalsEnriched   int       `json:"signalsEnriched"`
    SignalsClassified int       `json:"signalsClassified"`
    SignalsCategorized int      `json:"signalsCategorized"`
    ProcessingRate    float64   `json:"processingRate"` // signals per minute
    SuccessRate       float64   `json:"successRate"`
    LastUpdated       time.Time `json:"lastUpdated"`
}

type HealthStatus struct {
    Status     string           `json:"status"` // "healthy", "degraded", "unhealthy"
    Service    string           `json:"service"`
    Components *ComponentHealth `json:"components"`
}

type ComponentHealth struct {
    Enricher    bool `json:"enricher"`
    Classifier  bool `json:"classifier"`
    Categorizer bool `json:"categorizer"`
    Validator   bool `json:"validator"`
    DataStorage bool `json:"dataStorage"`
}
```

**Notes**:
- **Deduplication removed**: Handled by Gateway Service (BR-WH-008)
- **Context API removed**: Deprecated per DD-CONTEXT-006
- **Categorization added**: Consolidated from Gateway per DD-CATEGORIZATION-001
- **Data access**: Via Data Storage Service REST API per ADR-032

### Migration to CRD Controller

**Pipeline → Asynchronous Reconciliation Phases**

```go
// MIGRATED: Asynchronous CRD reconciliation
func (r *SignalProcessingReconciler) Reconcile(ctx, req) (ctrl.Result, error) {
    // SignalProcessing CRD only created for non-duplicate signals
    // Gateway Service handles duplicate detection and escalation

    switch signalProcessing.Status.Phase {
    case "enriching":
        // Enrich with K8s context
        enrichment := r.enricher.Enrich(ctx, signal)
        signalProcessing.Status.EnrichmentResults = enrichment

        // If recovery attempt, read embedded failure data (DD-CONTEXT-006)
        if signalProcessing.Spec.IsRecoveryAttempt && signalProcessing.Spec.FailureData != nil {
            recoveryCtx := r.buildRecoveryContextFromFailureData(signalProcessing)
            signalProcessing.Status.EnrichmentResults.RecoveryContext = recoveryCtx
        }

        signalProcessing.Status.Phase = "classifying"
        return ctrl.Result{Requeue: true}, r.Status().Update(ctx, signalProcessing)

    case "classifying":
        // Classify environment
        classification := r.classifier.Classify(ctx, signal, enrichment)
        signalProcessing.Status.EnvironmentClassification = classification
        signalProcessing.Status.Phase = "categorizing"
        return ctrl.Result{Requeue: true}, r.Status().Update(ctx, signalProcessing)

    case "categorizing":
        // Assign priority (DD-CATEGORIZATION-001)
        categorization := r.categorizer.AssignPriority(ctx, signal, enrichment, classification)
        signalProcessing.Status.Categorization = categorization

        signalProcessing.Status.Phase = "completed"
        return ctrl.Result{}, r.Status().Update(ctx, signalProcessing)
    }
}
```

### Component Mapping

| Component | CRD Controller Usage | Responsibility |
|-----------|---------------------|----------------|
| **SignalEnricher** | Enriching phase logic | K8s context gathering |
| **EnvironmentClassifier** | Classifying phase logic | Environment tier classification |
| **PriorityCategorizer** | Categorizing phase logic | Priority assignment (DD-CATEGORIZATION-001) |
| **RecoveryContextBuilder** | Enriching phase (recovery) | Build context from spec.failureData |
| **DataStorageClient** | Audit persistence | REST API calls to Data Storage Service (ADR-032) |

**Removed from Signal Processing**:
- `AlertDeduplicatorImpl` - Moved to Gateway Service (BR-WH-008)
- `ContextAPIClient` - Removed, Context API deprecated (DD-CONTEXT-006)

### Implementation Gap Analysis

**What Needs to be Created**:
- ✅ SignalProcessing CRD schema (`api/kubernaut.io/v1alpha1/signalprocessing_types.go`)
- ✅ SignalProcessingReconciler controller (`internal/controller/signalprocessing/`)
- ✅ Business logic package (`pkg/signalprocessing/`)
- ✅ CRD lifecycle management (owner refs, finalizers)
- ✅ Watch-based status coordination
- ✅ Phase timeout detection
- ✅ Event emission for visibility
- ✅ Data Storage Service client for audit (ADR-032)

### Gateway Code Migration (DD-CATEGORIZATION-001)

**What Needs to be Migrated from Gateway**:

| Source (Gateway) | Target (Signal Processing) | Description |
|------------------|---------------------------|-------------|
| `pkg/gateway/processing/classification.go` | `pkg/signalprocessing/classification.go` | Environment classification logic |
| `pkg/gateway/processing/priority.go` | `pkg/signalprocessing/priority.go` | Priority engine logic |
| `config.app/gateway/policies/priority.rego` | `config.app/signalprocessing/policies/priority.rego` | Rego policy rules |
| `test/unit/gateway/processing/environment_classification_test.go` | `test/unit/signalprocessing/classifier_test.go` | Unit tests |
| `test/unit/gateway/priority_classification_test.go` | `test/unit/signalprocessing/categorizer_test.go` | Unit tests |

**Gateway Files to Update** (remove classification):
- `pkg/gateway/server.go` - Remove classifier/categorizer instantiation
- `pkg/gateway/processing/crd_creator.go` - Pass through raw values
- `pkg/gateway/config/config.go` - Remove classification config

**See**: [IMPLEMENTATION_PLAN_V1.11.md - Gateway Migration Section](./IMPLEMENTATION_PLAN_V1.11.md)

**Estimated Implementation Effort**: 11-12 days
- Days 1-2: CRD schema + controller skeleton
- Days 3-4: Gateway code migration
- Days 5-6: Core business logic implementation
- Days 7-8: Categorization phase and integration
- Days 9-10: Unit and integration testing (parallel execution)
- Days 11-12: E2E testing and documentation
