# Day 1: Foundation Setup - AI Analysis Service

**Part of**: AI Analysis Implementation Plan V1.0
**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md)
**Duration**: 6-8 hours
**Target Confidence**: 60%

---

## 🎯 Day 1 Objectives

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Create package structure | P0 | BR-AI-001 |
| Implement basic reconciler | P0 | BR-AI-001 |
| Define phase handlers | P0 | BR-AI-002 |
| Setup Ginkgo test suite | P0 | BR-AI-001 |
| Write first failing tests | P0 | TDD-RED |

---

## 📁 Package Structure

### Create Directory Structure

```bash
# Create package directories
mkdir -p pkg/aianalysis/{handlers,metrics,rego,client}
mkdir -p test/unit/aianalysis
mkdir -p test/integration/aianalysis
mkdir -p test/e2e/aianalysis
```

### Expected File Structure

```
pkg/aianalysis/
├── reconciler.go           # Main reconciler
├── handler.go              # Handler interface
├── handlers/
│   ├── validating.go       # ValidatingHandler
│   ├── investigating.go    # InvestigatingHandler
│   ├── analyzing.go        # AnalyzingHandler
│   └── recommending.go     # RecommendingHandler
├── metrics/
│   └── metrics.go          # Prometheus metrics
├── rego/
│   └── evaluator.go        # Rego policy evaluator
└── client/
    └── holmesgpt.go        # HolmesGPT-API client wrapper

test/unit/aianalysis/
├── suite_test.go           # Ginkgo suite
├── reconciler_test.go      # Reconciler unit tests
└── validating_handler_test.go  # First handler tests
```

---

## 🔴 TDD RED Phase: Write Failing Tests First

### 1. Create Test Suite

```go
// test/unit/aianalysis/suite_test.go
package aianalysis

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAIAnalysis(t *testing.T) {
	RegisterFailHandler(Fail)
    RunSpecs(t, "AIAnalysis Unit Test Suite")
}
```

### 2. Reconciler Tests (Write First - Should Fail)

```go
// test/unit/aianalysis/reconciler_test.go
package aianalysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis"
)

var _ = Describe("AIAnalysisReconciler", func() {
	var (
        reconciler *aianalysis.Reconciler
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
        // Setup will be implemented in GREEN phase
    })

    // BR-AI-001: Basic reconciliation
    Describe("Reconcile", func() {
        Context("when AIAnalysis CRD exists", func() {
            It("should process through phase handlers", func() {
                // Create test AIAnalysis
                analysis := &aianalysisv1.AIAnalysis{
                    ObjectMeta: metav1.ObjectMeta{
                        Name:      "test-analysis",
                        Namespace: "default",
                    },
                    Spec: validAIAnalysisSpec(),
                }

                // This test should FAIL initially - reconciler doesn't exist yet
                result, err := reconciler.Reconcile(ctx, reconcile.Request{
                    NamespacedName: client.ObjectKeyFromObject(analysis),
                })

                Expect(err).NotTo(HaveOccurred())
                Expect(result).NotTo(BeNil())
            })
        })

        Context("when AIAnalysis CRD does not exist", func() {
            It("should return without error (Category A handling)", func() {
                // Category A: CRD deleted during reconciliation
                result, err := reconciler.Reconcile(ctx, reconcile.Request{
                    NamespacedName: types.NamespacedName{
                        Name:      "non-existent",
                        Namespace: "default",
                    },
                })

                Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
            })
        })
    })

    // BR-AI-002: Phase state machine
    Describe("Phase Transitions", func() {
        DescribeTable("transitions between phases",
            func(currentPhase, expectedNextPhase aianalysisv1.Phase) {
                analysis := &aianalysisv1.AIAnalysis{
                    Status: aianalysisv1.AIAnalysisStatus{
                        Phase: currentPhase,
                    },
                }

                // This should FAIL - handler router doesn't exist yet
                nextPhase := reconciler.GetNextPhase(analysis)
                Expect(nextPhase).To(Equal(expectedNextPhase))
            },
            Entry("Validating → Investigating", aianalysisv1.PhaseValidating, aianalysisv1.PhaseInvestigating),
            Entry("Investigating → Analyzing", aianalysisv1.PhaseInvestigating, aianalysisv1.PhaseAnalyzing),
            Entry("Analyzing → Recommending", aianalysisv1.PhaseAnalyzing, aianalysisv1.PhaseRecommending),
            Entry("Recommending → Completed", aianalysisv1.PhaseRecommending, aianalysisv1.PhaseCompleted),
        )
    })
})
```

### 3. ValidatingHandler Tests (Write First - Should Fail)

```go
// test/unit/aianalysis/validating_handler_test.go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

var _ = Describe("ValidatingHandler", func() {
    var handler *handlers.ValidatingHandler

    BeforeEach(func() {
        // This should FAIL - ValidatingHandler doesn't exist yet
        handler = handlers.NewValidatingHandler()
    })

    // BR-AI-001: Spec validation
    Describe("ValidateSpec", func() {
        Context("with valid spec", func() {
            It("should pass validation", func() {
                analysis := &aianalysisv1.AIAnalysis{
                    Spec: validAIAnalysisSpec(),
                }

                err := handler.ValidateSpec(ctx, analysis)
                Expect(err).NotTo(HaveOccurred())
            })
        })

        Context("with missing required fields", func() {
            It("should fail validation for missing environment", func() {
                analysis := &aianalysisv1.AIAnalysis{
                    Spec: specWithoutEnvironment(),
                }

                err := handler.ValidateSpec(ctx, analysis)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("environment"))
		})
	})
})

    // DD-WORKFLOW-001: FailedDetections validation
    Describe("ValidateFailedDetections", func() {
        DescribeTable("validates FailedDetections field names",
            func(fields []string, expectValid bool) {
                analysis := &aianalysisv1.AIAnalysis{
                    Spec: specWithFailedDetections(fields),
                }

                err := handler.ValidateFailedDetections(ctx, analysis)

                if expectValid {
                    Expect(err).NotTo(HaveOccurred())
                } else {
                    Expect(err).To(HaveOccurred())
                }
            },
            Entry("valid fields", []string{"gitOpsManaged", "pdbProtected"}, true),
            Entry("empty slice", []string{}, true),
            Entry("nil slice", nil, true),
            Entry("invalid field", []string{"unknownField"}, false),
            Entry("mixed valid/invalid", []string{"gitOpsManaged", "badField"}, false),
        )
    })
})
```

### 4. Run Tests - Verify They Fail

```bash
# Run tests - they should all FAIL
go test -v ./test/unit/aianalysis/...

# Expected output:
# --- FAIL: TestAIAnalysis (0.00s)
#     reconciler_test.go:XX: undefined: aianalysis.Reconciler
#     validating_handler_test.go:XX: undefined: handlers.ValidatingHandler
```

---

## 🟢 TDD GREEN Phase: Minimal Implementation

### 1. Handler Interface

```go
// pkg/aianalysis/handler.go
package aianalysis

import (
    "context"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    ctrl "sigs.k8s.io/controller-runtime"
)

// PhaseHandler processes an AIAnalysis in a specific phase
type PhaseHandler interface {
    // Handle processes the AIAnalysis and returns reconcile result
    Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error)

    // Phase returns the phase this handler processes
    Phase() aianalysisv1.Phase
}
```

### 2. Reconciler Skeleton

```go
// pkg/aianalysis/reconciler.go
package aianalysis

import (
    "context"

    "github.com/go-logr/logr"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler reconciles AIAnalysis CRDs
type Reconciler struct {
    client.Client
    Log      logr.Logger
    Handlers map[aianalysisv1.Phase]PhaseHandler
}

// NewReconciler creates a new AIAnalysis reconciler
func NewReconciler(client client.Client, log logr.Logger) *Reconciler {
    return &Reconciler{
        Client:   client,
        Log:      log,
        Handlers: make(map[aianalysisv1.Phase]PhaseHandler),
    }
}

// RegisterHandler registers a phase handler
func (r *Reconciler) RegisterHandler(handler PhaseHandler) {
    r.Handlers[handler.Phase()] = handler
}

// Reconcile implements reconcile.Reconciler
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Log.WithValues("aianalysis", req.NamespacedName)

    // Fetch AIAnalysis
    var analysis aianalysisv1.AIAnalysis
    if err := r.Get(ctx, req.NamespacedName, &analysis); err != nil {
        if apierrors.IsNotFound(err) {
            // Category A: CRD deleted
            log.Info("AIAnalysis not found, assuming deleted")
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // Get handler for current phase
    handler, exists := r.Handlers[analysis.Status.Phase]
    if !exists {
        log.Error(nil, "No handler for phase", "phase", analysis.Status.Phase)
        return ctrl.Result{}, nil
    }

    // Execute handler
    return handler.Handle(ctx, &analysis)
}

// GetNextPhase returns the next phase in the state machine
func (r *Reconciler) GetNextPhase(analysis *aianalysisv1.AIAnalysis) aianalysisv1.Phase {
    switch analysis.Status.Phase {
    case aianalysisv1.PhaseValidating:
        return aianalysisv1.PhaseInvestigating
    case aianalysisv1.PhaseInvestigating:
        return aianalysisv1.PhaseAnalyzing
    case aianalysisv1.PhaseAnalyzing:
        return aianalysisv1.PhaseRecommending
    case aianalysisv1.PhaseRecommending:
        return aianalysisv1.PhaseCompleted
    default:
        return aianalysisv1.PhaseCompleted
    }
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&aianalysisv1.AIAnalysis{}).
        Complete(r)
}
```

### 3. ValidatingHandler Skeleton

```go
// pkg/aianalysis/handlers/validating.go
package handlers

import (
    "context"
    "fmt"

    "github.com/go-logr/logr"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    ctrl "sigs.k8s.io/controller-runtime"
)

// ValidDetectedLabelFields defines valid FailedDetections values
var ValidDetectedLabelFields = []string{
    "gitOpsManaged",
    "pdbProtected",
    "hpaEnabled",
    "stateful",
    "helmManaged",
    "networkIsolated",
    "serviceMesh",
}

// ValidatingHandler handles the Validating phase
type ValidatingHandler struct {
    log logr.Logger
}

// NewValidatingHandler creates a new ValidatingHandler
func NewValidatingHandler(opts ...Option) *ValidatingHandler {
    h := &ValidatingHandler{}
    for _, opt := range opts {
        opt(h)
    }
    return h
}

// Phase returns the phase this handler processes
func (h *ValidatingHandler) Phase() aianalysisv1.Phase {
    return aianalysisv1.PhaseValidating
}

// Handle validates the AIAnalysis spec
func (h *ValidatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    // Validate spec
    if err := h.ValidateSpec(ctx, analysis); err != nil {
        return ctrl.Result{}, err
    }

    // Validate FailedDetections
    if err := h.ValidateFailedDetections(ctx, analysis); err != nil {
        return ctrl.Result{}, err
    }

    // Transition to next phase
    analysis.Status.Phase = aianalysisv1.PhaseInvestigating
    return ctrl.Result{Requeue: true}, nil
}

// ValidateSpec validates required spec fields
func (h *ValidatingHandler) ValidateSpec(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    spec := &analysis.Spec.SignalContext

    if spec.Environment == "" {
        return fmt.Errorf("environment is required")
    }
    if len(spec.Environment) > 63 {
        return fmt.Errorf("environment exceeds maximum length (63)")
    }
    if spec.TargetResource.Kind == "" {
        return fmt.Errorf("targetResource.kind is required")
    }

    return nil
}

// ValidateFailedDetections validates FailedDetections field names
func (h *ValidatingHandler) ValidateFailedDetections(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
    enrichment := analysis.Spec.SignalContext.EnrichmentResults
    if enrichment == nil || enrichment.DetectedLabels == nil {
        return nil
    }

    for _, field := range enrichment.DetectedLabels.FailedDetections {
        if !isValidField(field) {
            return fmt.Errorf("invalid FailedDetections field: %s", field)
        }
    }

    return nil
}

func isValidField(field string) bool {
    for _, valid := range ValidDetectedLabelFields {
        if field == valid {
            return true
        }
    }
    return false
}
```

### 4. Run Tests - Verify They Pass

```bash
# Run tests - they should all PASS now
go test -v ./test/unit/aianalysis/...

# Expected output:
# --- PASS: TestAIAnalysis (0.XX s)
#     [Reconcile] should process through phase handlers
#     [Phase Transitions] transitions between phases
#     [ValidateSpec] should pass validation with valid spec
```

---

## ✅ Day 1 Completion Checklist

### Code Deliverables

- [ ] `pkg/aianalysis/reconciler.go` - Basic reconciler
- [ ] `pkg/aianalysis/handler.go` - Handler interface
- [ ] `pkg/aianalysis/handlers/validating.go` - ValidatingHandler
- [ ] `test/unit/aianalysis/suite_test.go` - Test suite
- [ ] `test/unit/aianalysis/reconciler_test.go` - Reconciler tests
- [ ] `test/unit/aianalysis/validating_handler_test.go` - Handler tests

### Verification Commands

```bash
# Build verification
go build ./pkg/aianalysis/...

# Lint check
golangci-lint run ./pkg/aianalysis/...

# Test execution
go test -v ./test/unit/aianalysis/...

# Coverage check
go test -coverprofile=coverage.out ./pkg/aianalysis/...
go tool cover -func=coverage.out | grep total
```

### EOD Documentation

Create: `docs/services/crd-controllers/02-aianalysis/implementation/phase0/01-day1-complete.md`

```markdown
# Day 1 Complete - Foundation Setup

**Date**: YYYY-MM-DD
**Confidence**: 60%

## Completed
- ✅ Package structure created
- ✅ Reconciler skeleton implemented
- ✅ ValidatingHandler implemented
- ✅ Test suite created
- ✅ TDD RED→GREEN cycle completed

## Tests
- Unit tests: X passing
- Coverage: XX%

## Blockers
- None

## Tomorrow
- InvestigatingHandler (Day 2)
- HolmesGPT-API client integration
```

---

## 📚 Related Documents

- [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md) - Main plan
- [DAY_02_INVESTIGATING_HANDLER.md](./DAY_02_INVESTIGATING_HANDLER.md) - Next day
- [APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md](../appendices/APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md) - Error patterns
