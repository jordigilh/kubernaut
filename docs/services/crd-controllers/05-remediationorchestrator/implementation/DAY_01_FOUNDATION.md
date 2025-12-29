# Day 1: Foundation + CRD Controller Setup (8h)

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](./IMPLEMENTATION_PLAN_V1.1.md)
**Date**: Day 1 of 14-16
**Focus**: Controller skeleton, package structure, CRD integration
**Deliverable**: `01-day1-complete.md`

---

## 📑 Table of Contents

| Section | Purpose |
|---------|---------|
| [Morning: Package Structure](#morning-package-structure-4h) | Create directory layout and core types |
| [Afternoon: Controller Skeleton](#afternoon-controller-skeleton-4h) | Implement reconciler foundation |
| [Validation Checklist](#validation-checklist) | Day 1 completion criteria |
| [EOD Documentation](#eod-documentation) | Progress tracking |

---

## Morning: Package Structure (4h)

### 1. Create Directory Layout (30 min)

```bash
# Create package structure (flat pattern matching other services)
mkdir -p pkg/remediationorchestrator/{controller,phase,creator,aggregator,timeout,escalation}
mkdir -p pkg/remediationorchestrator/internal/{metrics,events}
mkdir -p cmd/remediationorchestrator
mkdir -p test/unit/remediationorchestrator
mkdir -p test/integration/remediationorchestrator

# Verify structure
tree pkg/remediationorchestrator/
```

**Expected Structure**:
```
pkg/remediationorchestrator/
├── controller/
│   ├── reconciler.go           # Main reconciliation loop
│   ├── reconciler_test.go      # Unit tests
│   └── setup.go                # SetupWithManager
├── phase/
│   ├── manager.go              # Phase state machine
│   ├── manager_test.go         # Phase transition tests
│   └── types.go                # Phase constants
├── creator/
│   ├── signalprocessing.go     # SignalProcessing CRD creator
│   ├── aianalysis.go           # AIAnalysis CRD creator
│   ├── workflowexecution.go    # WorkflowExecution CRD creator
│   └── notification.go         # NotificationRequest CRD creator
├── aggregator/
│   ├── status.go               # Status aggregation from children
│   └── status_test.go          # Aggregation tests
├── timeout/
│   ├── detector.go             # Phase timeout detection
│   └── detector_test.go        # Timeout tests
├── escalation/
│   ├── manager.go              # Escalation workflow
│   └── manager_test.go         # Escalation tests
└── internal/
    ├── metrics/
    │   └── prometheus.go       # Prometheus metrics
    └── events/
        └── recorder.go         # Kubernetes event recorder

cmd/remediationorchestrator/
└── main.go                     # Entry point
```

### 2. Define Core Types (1h)

**File**: `pkg/remediationorchestrator/types.go`

```go
package remediationorchestrator

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Phase represents the orchestration phase of a RemediationRequest
type Phase string

const (
	// PhasePending - Initial state, waiting to start
	PhasePending Phase = "Pending"
	// PhaseProcessing - SignalProcessing CRD created, awaiting completion
	PhaseProcessing Phase = "Processing"
	// PhaseAnalyzing - AIAnalysis CRD created, awaiting completion
	PhaseAnalyzing Phase = "Analyzing"
	// PhaseAwaitingApproval - Waiting for human approval (BR-ORCH-001)
	PhaseAwaitingApproval Phase = "AwaitingApproval"
	// PhaseExecuting - WorkflowExecution CRD created, awaiting completion
	PhaseExecuting Phase = "Executing"
	// PhaseCompleted - All phases completed successfully
	PhaseCompleted Phase = "Completed"
	// PhaseFailed - A phase failed
	PhaseFailed Phase = "Failed"
	// PhaseTimedOut - A phase exceeded timeout (BR-ORCH-027, BR-ORCH-028)
	PhaseTimedOut Phase = "TimedOut"
	// PhaseSkipped - WorkflowExecution was skipped due to resource lock (BR-ORCH-032)
	PhaseSkipped Phase = "Skipped"
)

// PhaseTimeouts defines configurable timeouts per phase (BR-ORCH-028)
type PhaseTimeouts struct {
	Processing time.Duration // Default: 5 minutes
	Analyzing  time.Duration // Default: 10 minutes
	Executing  time.Duration // Default: 30 minutes
	Global     time.Duration // Default: 60 minutes (BR-ORCH-027)
}

// DefaultPhaseTimeouts returns sensible defaults
func DefaultPhaseTimeouts() PhaseTimeouts {
	return PhaseTimeouts{
		Processing: 5 * time.Minute,
		Analyzing:  10 * time.Minute,
		Executing:  30 * time.Minute,
		Global:     60 * time.Minute,
	}
}

// ChildCRDRefs holds references to created child CRDs
type ChildCRDRefs struct {
	SignalProcessing  string
	AIAnalysis        string
	WorkflowExecution string
	NotificationRequest string // For approval notifications (BR-ORCH-001)
}

// ReconcileResult contains the result of a reconciliation cycle
type ReconcileResult struct {
	Requeue      bool
	RequeueAfter time.Duration
	Phase        Phase
	Error        error
	ChildCreated string // Name of child CRD created (if any)
}

// OrchestratorConfig holds controller configuration
type OrchestratorConfig struct {
	// Timeouts for each phase
	Timeouts PhaseTimeouts

	// RetentionPeriod after completion (default: 24h)
	RetentionPeriod time.Duration

	// MaxConcurrentReconciles limits parallel reconciliations
	MaxConcurrentReconciles int

	// EnableMetrics enables Prometheus metrics
	EnableMetrics bool
}

// DefaultConfig returns sensible defaults
func DefaultConfig() OrchestratorConfig {
	return OrchestratorConfig{
		Timeouts:                DefaultPhaseTimeouts(),
		RetentionPeriod:         24 * time.Hour,
		MaxConcurrentReconciles: 10,
		EnableMetrics:           true,
	}
}
```

### 3. Define Interfaces (1h)

**File**: `pkg/remediationorchestrator/interfaces.go`

```go
package remediationorchestrator

import (
	"context"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// PhaseManager manages phase transitions and validation
type PhaseManager interface {
	// CurrentPhase returns the current phase of the remediation
	CurrentPhase(rr *remediationv1.RemediationRequest) Phase

	// CanTransitionTo checks if transition from current to target phase is valid
	CanTransitionTo(current, target Phase) bool

	// TransitionTo transitions to the target phase with validation
	TransitionTo(ctx context.Context, rr *remediationv1.RemediationRequest, target Phase) error

	// NextPhase returns the next phase based on current state
	NextPhase(rr *remediationv1.RemediationRequest) (Phase, error)
}

// ChildCRDCreator creates and manages child CRDs
type ChildCRDCreator interface {
	// CreateSignalProcessing creates a SignalProcessing CRD for the given RemediationRequest
	CreateSignalProcessing(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error)

	// CreateAIAnalysis creates an AIAnalysis CRD for the given RemediationRequest
	CreateAIAnalysis(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error)

	// CreateWorkflowExecution creates a WorkflowExecution CRD for the given RemediationRequest
	CreateWorkflowExecution(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error)

	// CreateApprovalNotification creates a NotificationRequest CRD for approval (BR-ORCH-001)
	CreateApprovalNotification(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error)

	// CreateBulkDuplicateNotification creates a NotificationRequest for bulk duplicate notification (BR-ORCH-034)
	CreateBulkDuplicateNotification(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error)
}

// StatusAggregator aggregates status from child CRDs
type StatusAggregator interface {
	// AggregateStatus collects status from all child CRDs and updates RemediationRequest
	AggregateStatus(ctx context.Context, rr *remediationv1.RemediationRequest) (*AggregatedStatus, error)
}

// AggregatedStatus contains combined status from all child CRDs
type AggregatedStatus struct {
	// SignalProcessing status
	SignalProcessingPhase  string
	SignalProcessingReady  bool
	EnrichmentResults      interface{} // From SignalProcessing.status.enrichmentResults

	// AIAnalysis status
	AIAnalysisPhase        string
	AIAnalysisReady        bool
	RequiresApproval       bool
	SelectedWorkflow       interface{} // From AIAnalysis.status.selectedWorkflow

	// WorkflowExecution status
	WorkflowExecutionPhase string
	WorkflowExecutionReady bool
	ExecutionSkipped       bool
	SkipReason             string
	DuplicateOf            string // For BR-ORCH-033

	// Overall
	OverallReady           bool
	Error                  error
}

// TimeoutDetector detects phase timeouts
type TimeoutDetector interface {
	// CheckTimeout checks if the current phase has timed out
	CheckTimeout(rr *remediationv1.RemediationRequest) (timedOut bool, phase Phase, duration time.Duration)

	// CheckGlobalTimeout checks if the global timeout has been exceeded (BR-ORCH-027)
	CheckGlobalTimeout(rr *remediationv1.RemediationRequest) (timedOut bool, duration time.Duration)
}

// EscalationManager handles escalation workflows
type EscalationManager interface {
	// Escalate creates escalation notification for failed/timed out remediations
	Escalate(ctx context.Context, rr *remediationv1.RemediationRequest, reason string) error

	// TrackDuplicate records a duplicate remediation on the parent (BR-ORCH-033)
	TrackDuplicate(ctx context.Context, rr *remediationv1.RemediationRequest, duplicateOf string) error
}
```

### 4. Implement Phase Constants and Transitions (1h)

**File**: `pkg/remediationorchestrator/phase/types.go`

```go
package phase

import (
	"fmt"
)

// Phase represents the orchestration phase
type Phase string

const (
	Pending          Phase = "Pending"
	Processing       Phase = "Processing"
	Analyzing        Phase = "Analyzing"
	AwaitingApproval Phase = "AwaitingApproval"
	Executing        Phase = "Executing"
	Completed        Phase = "Completed"
	Failed           Phase = "Failed"
	TimedOut         Phase = "TimedOut"
	Skipped          Phase = "Skipped"
)

// ValidTransitions defines the state machine
var ValidTransitions = map[Phase][]Phase{
	Pending:          {Processing},
	Processing:       {Analyzing, Failed, TimedOut},
	Analyzing:        {AwaitingApproval, Executing, Failed, TimedOut},
	AwaitingApproval: {Executing, Failed, TimedOut},
	Executing:        {Completed, Failed, TimedOut, Skipped},
	// Terminal states - no transitions allowed
	Completed: {},
	Failed:    {},
	TimedOut:  {},
	Skipped:   {},
}

// IsTerminal returns true if the phase is a terminal state
func IsTerminal(p Phase) bool {
	switch p {
	case Completed, Failed, TimedOut, Skipped:
		return true
	default:
		return false
	}
}

// CanTransition checks if transition from current to target is valid
func CanTransition(current, target Phase) bool {
	validTargets, ok := ValidTransitions[current]
	if !ok {
		return false
	}
	for _, v := range validTargets {
		if v == target {
			return true
		}
	}
	return false
}

// Validate checks if a phase value is valid
func Validate(p Phase) error {
	switch p {
	case Pending, Processing, Analyzing, AwaitingApproval, Executing, Completed, Failed, TimedOut, Skipped:
		return nil
	default:
		return fmt.Errorf("invalid phase: %s", p)
	}
}
```

**File**: `pkg/remediationorchestrator/phase/manager.go`

```go
package phase

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Manager implements phase state machine logic
type Manager struct {
	// Add dependencies as needed
}

// NewManager creates a new phase manager
func NewManager() *Manager {
	return &Manager{}
}

// CurrentPhase returns the current phase of the remediation
func (m *Manager) CurrentPhase(rr *remediationv1.RemediationRequest) Phase {
	if rr.Status.OverallPhase == "" {
		return Pending
	}
	return Phase(rr.Status.OverallPhase)
}

// CanTransitionTo checks if transition from current to target phase is valid
func (m *Manager) CanTransitionTo(current, target Phase) bool {
	return CanTransition(current, target)
}

// TransitionTo transitions to the target phase with validation
func (m *Manager) TransitionTo(ctx context.Context, rr *remediationv1.RemediationRequest, target Phase) error {
	log := log.FromContext(ctx)
	current := m.CurrentPhase(rr)

	// Check if transition is valid
	if !m.CanTransitionTo(current, target) {
		return fmt.Errorf("invalid phase transition from %s to %s", current, target)
	}

	// Record transition
	now := metav1.Now()
	previousPhase := rr.Status.OverallPhase

	rr.Status.OverallPhase = string(target)
	rr.Status.LastTransitionTime = &now

	// Update phase-specific timestamps
	switch target {
	case Processing:
		rr.Status.ProcessingStartTime = &now
	case Analyzing:
		rr.Status.AnalyzingStartTime = &now
	case Executing:
		rr.Status.ExecutingStartTime = &now
	case Completed:
		rr.Status.CompletionTime = &now
	case Failed, TimedOut:
		rr.Status.FailureTime = &now
	}

	log.Info("Phase transition",
		"remediation", rr.Name,
		"from", previousPhase,
		"to", target,
	)

	return nil
}

// NextPhase returns the next phase based on current state and child CRD statuses
func (m *Manager) NextPhase(rr *remediationv1.RemediationRequest) (Phase, error) {
	current := m.CurrentPhase(rr)

	switch current {
	case Pending:
		return Processing, nil
	case Processing:
		// Check SignalProcessing status
		if rr.Status.SignalProcessingRef != "" {
			// If SignalProcessing completed, move to Analyzing
			return Analyzing, nil
		}
		return Processing, nil // Stay in Processing
	case Analyzing:
		// Check AIAnalysis status
		if rr.Status.RequiresApproval {
			return AwaitingApproval, nil
		}
		if rr.Status.AIAnalysisRef != "" {
			return Executing, nil
		}
		return Analyzing, nil
	case AwaitingApproval:
		if rr.Status.ApprovalDecision == "approved" {
			return Executing, nil
		}
		return AwaitingApproval, nil
	case Executing:
		// Check WorkflowExecution status
		if rr.Status.WorkflowExecutionRef != "" {
			return Completed, nil
		}
		return Executing, nil
	default:
		return current, nil // Terminal states stay as-is
	}
}
```

### 5. Create Test Skeleton (30 min)

**File**: `pkg/remediation/orchestrator/phase/manager_test.go`

```go
package phase_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediation/orchestrator/phase"
)

func TestPhase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Phase Manager Suite")
}

var _ = Describe("Phase Manager", func() {
	var (
		manager *phase.Manager
		ctx     context.Context
	)

	BeforeEach(func() {
		manager = phase.NewManager()
		ctx = context.Background()
	})

	Describe("Phase Transitions (BR-ORCH-025, BR-ORCH-026)", func() {
		DescribeTable("valid transitions",
			func(from, to phase.Phase, expectValid bool) {
				result := manager.CanTransitionTo(from, to)
				Expect(result).To(Equal(expectValid))
			},
			// Valid transitions
			Entry("Pending → Processing", phase.Pending, phase.Processing, true),
			Entry("Processing → Analyzing", phase.Processing, phase.Analyzing, true),
			Entry("Processing → Failed", phase.Processing, phase.Failed, true),
			Entry("Analyzing → Executing", phase.Analyzing, phase.Executing, true),
			Entry("Analyzing → AwaitingApproval", phase.Analyzing, phase.AwaitingApproval, true),
			Entry("AwaitingApproval → Executing", phase.AwaitingApproval, phase.Executing, true),
			Entry("Executing → Completed", phase.Executing, phase.Completed, true),
			Entry("Executing → Skipped (BR-ORCH-032)", phase.Executing, phase.Skipped, true),

			// Invalid transitions
			Entry("Pending → Completed (skip)", phase.Pending, phase.Completed, false),
			Entry("Completed → Pending (reverse)", phase.Completed, phase.Pending, false),
			Entry("Failed → Processing (recover)", phase.Failed, phase.Processing, false),
		)
	})

	Describe("Terminal States", func() {
		DescribeTable("terminal state detection",
			func(p phase.Phase, expectTerminal bool) {
				result := phase.IsTerminal(p)
				Expect(result).To(Equal(expectTerminal))
			},
			Entry("Completed is terminal", phase.Completed, true),
			Entry("Failed is terminal", phase.Failed, true),
			Entry("TimedOut is terminal", phase.TimedOut, true),
			Entry("Skipped is terminal", phase.Skipped, true),
			Entry("Pending is NOT terminal", phase.Pending, false),
			Entry("Processing is NOT terminal", phase.Processing, false),
			Entry("Executing is NOT terminal", phase.Executing, false),
		)
	})

	Describe("TransitionTo", func() {
		It("should update phase and timestamp on valid transition", func() {
			rr := &remediationv1.RemediationRequest{}
			rr.Status.OverallPhase = string(phase.Pending)

			err := manager.TransitionTo(ctx, rr, phase.Processing)

			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Status.OverallPhase).To(Equal(string(phase.Processing)))
			Expect(rr.Status.LastTransitionTime).ToNot(BeNil())
			Expect(rr.Status.ProcessingStartTime).ToNot(BeNil())
		})

		It("should reject invalid transition", func() {
			rr := &remediationv1.RemediationRequest{}
			rr.Status.OverallPhase = string(phase.Pending)

			err := manager.TransitionTo(ctx, rr, phase.Completed)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid phase transition"))
		})
	})
})
```

---

## Afternoon: Controller Skeleton (4h)

### 6. Implement Reconciler Foundation (2h)

**File**: `pkg/remediation/orchestrator/controller/reconciler.go`

```go
package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediation/orchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediation/orchestrator/internal/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediation/orchestrator/phase"
)

const (
	finalizerName = "remediation.kubernaut.ai/orchestrator-cleanup"
)

// Reconciler reconciles RemediationRequest objects
type Reconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	EventRecorder   record.EventRecorder
	Config          orchestrator.OrchestratorConfig

	// Components (injected)
	PhaseManager    orchestrator.PhaseManager
	ChildCreator    orchestrator.ChildCRDCreator
	StatusAggregator orchestrator.StatusAggregator
	TimeoutDetector orchestrator.TimeoutDetector
	EscalationMgr   orchestrator.EscalationManager
	Metrics         *metrics.Collector
}

//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests/finalizers,verbs=update
//+kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()

	// Metrics: track reconciliation
	defer func() {
		r.Metrics.ReconciliationDuration.WithLabelValues(req.Namespace).Observe(time.Since(startTime).Seconds())
	}()

	// 1. FETCH RESOURCE
	rr := &remediationv1.RemediationRequest{}
	if err := r.Get(ctx, req.NamespacedName, rr); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("RemediationRequest not found, likely deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get RemediationRequest")
		r.Metrics.ReconciliationErrors.WithLabelValues(req.Namespace, "fetch_error").Inc()
		return ctrl.Result{}, err
	}

	// 2. HANDLE DELETION (Finalizer pattern)
	if !rr.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, rr)
	}

	// 3. ADD FINALIZER if not present
	if !controllerutil.ContainsFinalizer(rr, finalizerName) {
		controllerutil.AddFinalizer(rr, finalizerName)
		if err := r.Update(ctx, rr); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 4. CHECK TERMINAL STATES
	currentPhase := r.PhaseManager.CurrentPhase(rr)
	if phase.IsTerminal(currentPhase) {
		log.Info("RemediationRequest in terminal state", "phase", currentPhase)
		return r.handleTerminalState(ctx, rr)
	}

	// 5. CHECK TIMEOUTS (BR-ORCH-027, BR-ORCH-028)
	if timedOut, timedOutPhase, duration := r.TimeoutDetector.CheckTimeout(rr); timedOut {
		log.Info("Phase timeout detected", "phase", timedOutPhase, "duration", duration)
		return r.handleTimeout(ctx, rr, timedOutPhase, duration)
	}

	// 6. AGGREGATE STATUS from child CRDs
	aggregatedStatus, err := r.StatusAggregator.AggregateStatus(ctx, rr)
	if err != nil {
		log.Error(err, "Failed to aggregate status")
		r.Metrics.ReconciliationErrors.WithLabelValues(req.Namespace, "status_aggregation_error").Inc()
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// 7. HANDLE CURRENT PHASE
	switch currentPhase {
	case phase.Pending:
		return r.handlePending(ctx, rr)
	case phase.Processing:
		return r.handleProcessing(ctx, rr, aggregatedStatus)
	case phase.Analyzing:
		return r.handleAnalyzing(ctx, rr, aggregatedStatus)
	case phase.AwaitingApproval:
		return r.handleAwaitingApproval(ctx, rr, aggregatedStatus)
	case phase.Executing:
		return r.handleExecuting(ctx, rr, aggregatedStatus)
	default:
		log.Info("Unknown phase", "phase", currentPhase)
		return ctrl.Result{}, nil
	}
}

// handlePending handles the Pending phase - creates SignalProcessing CRD
func (r *Reconciler) handlePending(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Create SignalProcessing CRD
	spName, err := r.ChildCreator.CreateSignalProcessing(ctx, rr)
	if err != nil {
		log.Error(err, "Failed to create SignalProcessing CRD")
		r.EventRecorder.Event(rr, "Warning", "CreateFailed", fmt.Sprintf("Failed to create SignalProcessing: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Update status with child reference
	rr.Status.SignalProcessingRef = spName

	// Transition to Processing phase
	if err := r.PhaseManager.TransitionTo(ctx, rr, phase.Processing); err != nil {
		log.Error(err, "Failed to transition to Processing phase")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.Status().Update(ctx, rr); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(rr, "Normal", "PhaseTransition", "Transitioned to Processing phase")
	r.Metrics.PhaseTransitions.WithLabelValues(rr.Namespace, string(phase.Processing)).Inc()

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleProcessing handles the Processing phase - waits for SignalProcessing completion
func (r *Reconciler) handleProcessing(ctx context.Context, rr *remediationv1.RemediationRequest, status *orchestrator.AggregatedStatus) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if SignalProcessing is ready
	if !status.SignalProcessingReady {
		log.Info("Waiting for SignalProcessing to complete")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// SignalProcessing completed - create AIAnalysis CRD
	aiName, err := r.ChildCreator.CreateAIAnalysis(ctx, rr)
	if err != nil {
		log.Error(err, "Failed to create AIAnalysis CRD")
		r.EventRecorder.Event(rr, "Warning", "CreateFailed", fmt.Sprintf("Failed to create AIAnalysis: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Update status with child reference
	rr.Status.AIAnalysisRef = aiName

	// Transition to Analyzing phase
	if err := r.PhaseManager.TransitionTo(ctx, rr, phase.Analyzing); err != nil {
		log.Error(err, "Failed to transition to Analyzing phase")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.Status().Update(ctx, rr); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(rr, "Normal", "PhaseTransition", "Transitioned to Analyzing phase")
	r.Metrics.PhaseTransitions.WithLabelValues(rr.Namespace, string(phase.Analyzing)).Inc()

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleAnalyzing handles the Analyzing phase - waits for AIAnalysis completion
func (r *Reconciler) handleAnalyzing(ctx context.Context, rr *remediationv1.RemediationRequest, status *orchestrator.AggregatedStatus) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if AIAnalysis is ready
	if !status.AIAnalysisReady {
		log.Info("Waiting for AIAnalysis to complete")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Check if approval is required (BR-ORCH-001)
	if status.RequiresApproval {
		log.Info("AIAnalysis requires approval")
		return r.handleApprovalRequired(ctx, rr, status)
	}

	// AIAnalysis completed with auto-approval - create WorkflowExecution CRD
	return r.createWorkflowExecution(ctx, rr, status)
}

// handleApprovalRequired handles the approval notification creation (BR-ORCH-001)
func (r *Reconciler) handleApprovalRequired(ctx context.Context, rr *remediationv1.RemediationRequest, status *orchestrator.AggregatedStatus) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Create approval notification if not already sent
	if rr.Status.ApprovalNotificationRef == "" {
		nrName, err := r.ChildCreator.CreateApprovalNotification(ctx, rr)
		if err != nil {
			log.Error(err, "Failed to create approval notification")
			r.EventRecorder.Event(rr, "Warning", "NotificationFailed", fmt.Sprintf("Failed to create approval notification: %v", err))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		rr.Status.ApprovalNotificationRef = nrName
	}

	// Transition to AwaitingApproval phase
	if err := r.PhaseManager.TransitionTo(ctx, rr, phase.AwaitingApproval); err != nil {
		log.Error(err, "Failed to transition to AwaitingApproval phase")
		return ctrl.Result{}, err
	}

	// Update status
	rr.Status.RequiresApproval = true
	if err := r.Status().Update(ctx, rr); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(rr, "Normal", "ApprovalRequired", "Remediation requires human approval")
	r.Metrics.PhaseTransitions.WithLabelValues(rr.Namespace, string(phase.AwaitingApproval)).Inc()

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleAwaitingApproval handles waiting for approval
func (r *Reconciler) handleAwaitingApproval(ctx context.Context, rr *remediationv1.RemediationRequest, status *orchestrator.AggregatedStatus) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if approval decision has been made
	if rr.Status.ApprovalDecision == "" {
		log.Info("Waiting for approval decision")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if rr.Status.ApprovalDecision == "approved" {
		log.Info("Approval granted, proceeding to execution")
		return r.createWorkflowExecution(ctx, rr, status)
	}

	// Approval rejected
	log.Info("Approval rejected", "reason", rr.Status.ApprovalRejectionReason)
	if err := r.PhaseManager.TransitionTo(ctx, rr, phase.Failed); err != nil {
		return ctrl.Result{}, err
	}
	rr.Status.FailureReason = fmt.Sprintf("Approval rejected: %s", rr.Status.ApprovalRejectionReason)
	if err := r.Status().Update(ctx, rr); err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(rr, "Warning", "ApprovalRejected", rr.Status.FailureReason)
	return ctrl.Result{}, nil
}

// createWorkflowExecution creates the WorkflowExecution CRD (BR-ORCH-025)
func (r *Reconciler) createWorkflowExecution(ctx context.Context, rr *remediationv1.RemediationRequest, status *orchestrator.AggregatedStatus) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	weName, err := r.ChildCreator.CreateWorkflowExecution(ctx, rr)
	if err != nil {
		log.Error(err, "Failed to create WorkflowExecution CRD")
		r.EventRecorder.Event(rr, "Warning", "CreateFailed", fmt.Sprintf("Failed to create WorkflowExecution: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Update status with child reference
	rr.Status.WorkflowExecutionRef = weName

	// Transition to Executing phase
	if err := r.PhaseManager.TransitionTo(ctx, rr, phase.Executing); err != nil {
		log.Error(err, "Failed to transition to Executing phase")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.Status().Update(ctx, rr); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(rr, "Normal", "PhaseTransition", "Transitioned to Executing phase")
	r.Metrics.PhaseTransitions.WithLabelValues(rr.Namespace, string(phase.Executing)).Inc()

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleExecuting handles the Executing phase - waits for WorkflowExecution completion
func (r *Reconciler) handleExecuting(ctx context.Context, rr *remediationv1.RemediationRequest, status *orchestrator.AggregatedStatus) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if WorkflowExecution is ready
	if !status.WorkflowExecutionReady {
		log.Info("Waiting for WorkflowExecution to complete")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Check if execution was skipped due to resource lock (BR-ORCH-032)
	if status.ExecutionSkipped {
		return r.handleWorkflowExecutionSkipped(ctx, rr, status)
	}

	// WorkflowExecution completed successfully
	if err := r.PhaseManager.TransitionTo(ctx, rr, phase.Completed); err != nil {
		log.Error(err, "Failed to transition to Completed phase")
		return ctrl.Result{}, err
	}

	// Update status
	now := metav1.Now()
	rr.Status.CompletionTime = &now
	if err := r.Status().Update(ctx, rr); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(rr, "Normal", "RemediationCompleted", "Remediation completed successfully")
	r.Metrics.PhaseTransitions.WithLabelValues(rr.Namespace, string(phase.Completed)).Inc()
	r.Metrics.RemediationsCompleted.WithLabelValues(rr.Namespace).Inc()

	return ctrl.Result{}, nil
}

// handleWorkflowExecutionSkipped handles the Skipped WorkflowExecution phase (BR-ORCH-032, BR-ORCH-033)
func (r *Reconciler) handleWorkflowExecutionSkipped(ctx context.Context, rr *remediationv1.RemediationRequest, status *orchestrator.AggregatedStatus) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("WorkflowExecution was skipped", "reason", status.SkipReason, "duplicateOf", status.DuplicateOf)

	// Transition to Skipped phase
	if err := r.PhaseManager.TransitionTo(ctx, rr, phase.Skipped); err != nil {
		log.Error(err, "Failed to transition to Skipped phase")
		return ctrl.Result{}, err
	}

	// Update status with skip details
	rr.Status.SkipReason = status.SkipReason
	rr.Status.DuplicateOf = status.DuplicateOf

	// Track duplicate on parent (BR-ORCH-033)
	if status.DuplicateOf != "" {
		if err := r.EscalationMgr.TrackDuplicate(ctx, rr, status.DuplicateOf); err != nil {
			log.Error(err, "Failed to track duplicate")
			// Non-fatal, continue
		}
	}

	if err := r.Status().Update(ctx, rr); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(rr, "Warning", "ExecutionSkipped", fmt.Sprintf("Execution skipped: %s", status.SkipReason))
	r.Metrics.PhaseTransitions.WithLabelValues(rr.Namespace, string(phase.Skipped)).Inc()

	return ctrl.Result{}, nil
}

// handleTimeout handles phase timeout (BR-ORCH-027, BR-ORCH-028)
func (r *Reconciler) handleTimeout(ctx context.Context, rr *remediationv1.RemediationRequest, timedOutPhase phase.Phase, duration time.Duration) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Transition to TimedOut phase
	if err := r.PhaseManager.TransitionTo(ctx, rr, phase.TimedOut); err != nil {
		log.Error(err, "Failed to transition to TimedOut phase")
		return ctrl.Result{}, err
	}

	// Update status
	rr.Status.TimeoutPhase = string(timedOutPhase)
	rr.Status.FailureReason = fmt.Sprintf("Phase %s timed out after %v", timedOutPhase, duration)

	if err := r.Status().Update(ctx, rr); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Escalate (create notification)
	if err := r.EscalationMgr.Escalate(ctx, rr, rr.Status.FailureReason); err != nil {
		log.Error(err, "Failed to escalate timeout")
		// Non-fatal, continue
	}

	r.EventRecorder.Event(rr, "Warning", "PhaseTimeout", rr.Status.FailureReason)
	r.Metrics.PhaseTransitions.WithLabelValues(rr.Namespace, string(phase.TimedOut)).Inc()
	r.Metrics.PhaseTimeouts.WithLabelValues(rr.Namespace, string(timedOutPhase)).Inc()

	return ctrl.Result{}, nil
}

// handleTerminalState handles terminal states (cleanup, retention)
func (r *Reconciler) handleTerminalState(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	currentPhase := r.PhaseManager.CurrentPhase(rr)

	// Check if retention period has elapsed
	if rr.Status.CompletionTime != nil || rr.Status.FailureTime != nil {
		var completionTime metav1.Time
		if rr.Status.CompletionTime != nil {
			completionTime = *rr.Status.CompletionTime
		} else {
			completionTime = *rr.Status.FailureTime
		}

		if time.Since(completionTime.Time) > r.Config.RetentionPeriod {
			log.Info("Retention period elapsed, allowing deletion")
			// Remove finalizer to allow garbage collection
			controllerutil.RemoveFinalizer(rr, finalizerName)
			if err := r.Update(ctx, rr); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Check for bulk duplicate notification (BR-ORCH-034)
	if currentPhase == phase.Completed && rr.Status.DuplicateCount > 0 && !rr.Status.BulkNotificationSent {
		if err := r.sendBulkDuplicateNotification(ctx, rr); err != nil {
			log.Error(err, "Failed to send bulk duplicate notification")
			// Non-fatal, continue
		}
	}

	// Requeue to check retention again
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

// sendBulkDuplicateNotification sends bulk notification for duplicates (BR-ORCH-034)
func (r *Reconciler) sendBulkDuplicateNotification(ctx context.Context, rr *remediationv1.RemediationRequest) error {
	log := log.FromContext(ctx)

	nrName, err := r.ChildCreator.CreateBulkDuplicateNotification(ctx, rr)
	if err != nil {
		return err
	}

	rr.Status.BulkNotificationRef = nrName
	rr.Status.BulkNotificationSent = true

	if err := r.Status().Update(ctx, rr); err != nil {
		return err
	}

	log.Info("Bulk duplicate notification sent", "duplicateCount", rr.Status.DuplicateCount)
	r.EventRecorder.Event(rr, "Normal", "BulkNotification", fmt.Sprintf("Sent bulk notification for %d duplicates", rr.Status.DuplicateCount))

	return nil
}

// handleDeletion handles the deletion of a RemediationRequest
func (r *Reconciler) handleDeletion(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(rr, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Cleanup: child CRDs are deleted via owner references (cascade deletion)
	log.Info("Performing cleanup before deletion")

	// Remove finalizer
	controllerutil.RemoveFinalizer(rr, finalizerName)
	if err := r.Update(ctx, rr); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Finalizer removed, allowing deletion")
	return ctrl.Result{}, nil
}
```

### 7. Implement SetupWithManager (1h)

**File**: `pkg/remediation/orchestrator/controller/setup.go`

```go
package controller

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Validate dependencies
	if r.PhaseManager == nil {
		return fmt.Errorf("PhaseManager is required")
	}
	if r.ChildCreator == nil {
		return fmt.Errorf("ChildCreator is required")
	}
	if r.StatusAggregator == nil {
		return fmt.Errorf("StatusAggregator is required")
	}
	if r.TimeoutDetector == nil {
		return fmt.Errorf("TimeoutDetector is required")
	}
	if r.EscalationMgr == nil {
		return fmt.Errorf("EscalationManager is required")
	}

	// Build controller with watches
	return ctrl.NewControllerManagedBy(mgr).
		// Primary resource
		For(&remediationv1.RemediationRequest{}).
		// Watch child CRDs owned by RemediationRequest
		Owns(&signalprocessingv1.SignalProcessing{}).
		Owns(&aianalysisv1.AIAnalysis{}).
		Owns(&workflowexecutionv1.WorkflowExecution{}).
		// Watch with status-only predicate to reduce reconciliation frequency
		WithEventFilter(predicate.Or(
			predicate.GenerationChangedPredicate{},
			predicate.AnnotationChangedPredicate{},
		)).
		// Configure concurrency
		WithOptions(ctrl.Options{
			MaxConcurrentReconciles: r.Config.MaxConcurrentReconciles,
		}).
		Complete(r)
}
```

### 8. Implement Metrics Collector (30 min)

**File**: `pkg/remediation/orchestrator/internal/metrics/prometheus.go`

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Collector holds all Prometheus metrics for the orchestrator
type Collector struct {
	ReconciliationDuration prometheus.HistogramVec
	ReconciliationErrors   prometheus.CounterVec
	PhaseTransitions       prometheus.CounterVec
	PhaseTimeouts          prometheus.CounterVec
	RemediationsCompleted  prometheus.CounterVec
	RemediationsFailed     prometheus.CounterVec
	ChildCRDCreations      prometheus.CounterVec
	ActiveRemediations     prometheus.GaugeVec
}

// NewCollector creates and registers all metrics
func NewCollector() *Collector {
	c := &Collector{
		ReconciliationDuration: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "kubernaut_orchestrator_reconciliation_duration_seconds",
				Help:    "Duration of reconciliation loops",
				Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
			},
			[]string{"namespace"},
		),
		ReconciliationErrors: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "kubernaut_orchestrator_reconciliation_errors_total",
				Help: "Total number of reconciliation errors",
			},
			[]string{"namespace", "error_type"},
		),
		PhaseTransitions: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "kubernaut_orchestrator_phase_transitions_total",
				Help: "Total number of phase transitions",
			},
			[]string{"namespace", "phase"},
		),
		PhaseTimeouts: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "kubernaut_orchestrator_phase_timeouts_total",
				Help: "Total number of phase timeouts",
			},
			[]string{"namespace", "phase"},
		),
		RemediationsCompleted: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "kubernaut_orchestrator_remediations_completed_total",
				Help: "Total number of completed remediations",
			},
			[]string{"namespace"},
		),
		RemediationsFailed: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "kubernaut_orchestrator_remediations_failed_total",
				Help: "Total number of failed remediations",
			},
			[]string{"namespace"},
		),
		ChildCRDCreations: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "kubernaut_orchestrator_child_crd_creations_total",
				Help: "Total number of child CRD creations",
			},
			[]string{"namespace", "crd_type"},
		),
		ActiveRemediations: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "kubernaut_orchestrator_active_remediations",
				Help: "Current number of active remediations",
			},
			[]string{"namespace", "phase"},
		),
	}

	// Register with controller-runtime metrics registry
	metrics.Registry.MustRegister(
		c.ReconciliationDuration,
		c.ReconciliationErrors,
		c.PhaseTransitions,
		c.PhaseTimeouts,
		c.RemediationsCompleted,
		c.RemediationsFailed,
		c.ChildCRDCreations,
		c.ActiveRemediations,
	)

	return c
}
```

---

## Validation Checklist

### Day 1 Completion Criteria

- [ ] Package structure created
- [ ] Core types defined (`types.go`, `interfaces.go`)
- [ ] Phase constants and transitions implemented
- [ ] Phase manager with unit tests
- [ ] Controller reconciler skeleton (compiles, no runtime errors)
- [ ] SetupWithManager with dependency validation
- [ ] Metrics collector registered
- [ ] All tests passing: `go test ./pkg/remediation/orchestrator/...`
- [ ] Code compiles: `go build ./...`
- [ ] Linting passes: `golangci-lint run`

### Commands to Validate

```bash
# Compile check
go build ./pkg/remediation/orchestrator/...

# Run tests
go test -v ./pkg/remediation/orchestrator/... -count=1

# Lint check
golangci-lint run ./pkg/remediation/orchestrator/...

# Verify package structure
tree pkg/remediation/orchestrator/
```

---

## EOD Documentation

**File**: `implementation/01-day1-complete.md`

```markdown
# Day 1 Complete: Foundation + CRD Controller Setup

**Date**: [DATE]
**Status**: ✅ Complete | ⚠️ Partial | ❌ Blocked
**Confidence**: XX%

## Summary

[2-3 sentences about what was accomplished]

## Deliverables

| Item | Status | Notes |
|------|--------|-------|
| Package structure | ✅/❌ | |
| Core types | ✅/❌ | |
| Phase manager | ✅/❌ | |
| Controller skeleton | ✅/❌ | |
| Metrics | ✅/❌ | |
| Unit tests | ✅/❌ | X/Y passing |

## Blockers

[List any blockers encountered]

## Tomorrow's Focus

- Day 2: Phase handlers (handlePending, handleProcessing)
- Implement SignalProcessing CRD creator
- Begin status aggregation

## Files Changed

- `pkg/remediation/orchestrator/types.go` - NEW
- `pkg/remediation/orchestrator/interfaces.go` - NEW
- `pkg/remediation/orchestrator/phase/manager.go` - NEW
- `pkg/remediation/orchestrator/controller/reconciler.go` - NEW
- `pkg/remediation/orchestrator/controller/setup.go` - NEW
- `pkg/remediation/orchestrator/internal/metrics/prometheus.go` - NEW
```

---

## Next Steps

**Day 2**: [DAYS_02_07_PHASE_HANDLERS.md](./DAYS_02_07_PHASE_HANDLERS.md)
- Implement child CRD creators
- Complete phase handlers
- Add error handling patterns

