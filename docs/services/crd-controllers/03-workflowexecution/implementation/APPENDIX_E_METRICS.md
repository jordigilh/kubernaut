# WorkflowExecution - Metrics Implementation

**Parent Document**: [IMPLEMENTATION_PLAN_V3.0.md](./IMPLEMENTATION_PLAN_V3.0.md)
**Version**: v1.0
**Last Updated**: 2025-12-03
**Status**: ✅ Ready for Implementation

---

## Document Purpose

This appendix provides complete Prometheus metrics implementation patterns for the WorkflowExecution Controller, aligned with Day 7 of [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md).

---

## 📊 Complete Metrics Definition

**File**: `pkg/workflowexecution/metrics/metrics.go`

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "kubernaut"
	subsystem = "workflowexecution"
)

// Metrics contains all Prometheus metrics for WorkflowExecution controller
type Metrics struct {
	// ==========================================
	// Phase Transition Metrics (Business-Critical)
	// ==========================================

	// PhaseTransitionsTotal tracks WFE phase changes
	// Labels: phase (Pending, Running, Completed, Failed, Skipped), from_phase
	PhaseTransitionsTotal *prometheus.CounterVec

	// PhaseDuration tracks time spent in each phase
	// Labels: phase
	PhaseDuration *prometheus.HistogramVec

	// ==========================================
	// PipelineRun Metrics (Core Functionality)
	// ==========================================

	// PipelineRunCreationDuration tracks PR creation latency
	// Labels: (none - single operation type)
	PipelineRunCreationDuration prometheus.Histogram

	// PipelineRunCreationTotal tracks PR creation attempts
	// Labels: result (success, failure, skipped)
	PipelineRunCreationTotal *prometheus.CounterVec

	// ==========================================
	// Resource Locking Metrics (DD-WE-001)
	// ==========================================

	// LockCheckTotal tracks lock validation attempts
	// Labels: result (allowed, blocked)
	LockCheckTotal *prometheus.CounterVec

	// SkipTotal tracks skipped WFEs
	// Labels: reason (ResourceBusy, RecentlyRemediated, AlreadyExists)
	SkipTotal *prometheus.CounterVec

	// LockDuration tracks time from lock acquisition to release
	// Labels: (none)
	LockDuration prometheus.Histogram

	// ActiveLocks tracks currently held locks
	// Labels: (none - gauge)
	ActiveLocks prometheus.Gauge

	// ==========================================
	// Reconciliation Metrics (Controller Health)
	// ==========================================

	// ReconciliationDuration tracks reconcile loop latency
	// Labels: phase
	ReconciliationDuration *prometheus.HistogramVec

	// ReconciliationTotal tracks reconcile attempts
	// Labels: phase, result (success, error, requeue)
	ReconciliationTotal *prometheus.CounterVec

	// ReconciliationErrors tracks errors by category
	// Labels: category (validation, external, permission, execution, system), reason
	ReconciliationErrors *prometheus.CounterVec

	// ==========================================
	// Queue Metrics (Workqueue Health)
	// ==========================================

	// QueueDepth tracks pending reconciles
	// Labels: (none - gauge)
	QueueDepth prometheus.Gauge

	// QueueLatency tracks time items spend in queue
	// Labels: (none)
	QueueLatency prometheus.Histogram

	// ==========================================
	// Business Outcome Metrics (BR-WE-*)
	// ==========================================

	// WorkflowDuration tracks total workflow execution time (E2E)
	// From WFE creation to completion/failure
	// Labels: outcome (completed, failed)
	WorkflowDuration *prometheus.HistogramVec

	// CostSavingsEstimate tracks remediation cost savings
	// (skipped * estimated_manual_cost)
	// Labels: skip_reason
	CostSavingsEstimate *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		// ==========================================
		// Phase Transition Metrics
		// ==========================================

		PhaseTransitionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "phase_transitions_total",
				Help:      "Total WorkflowExecution phase transitions",
			},
			[]string{"phase", "from_phase"},
		),

		PhaseDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "phase_duration_seconds",
				Help:      "Duration spent in each phase",
				Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"phase"},
		),

		// ==========================================
		// PipelineRun Metrics
		// ==========================================

		PipelineRunCreationDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "pipelinerun_creation_duration_seconds",
				Help:      "Duration to create PipelineRun in Kubernetes",
				Buckets:   prometheus.DefBuckets, // .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
			},
		),

		PipelineRunCreationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "pipelinerun_creation_total",
				Help:      "Total PipelineRun creation attempts",
			},
			[]string{"result"},
		),

		// ==========================================
		// Resource Locking Metrics
		// ==========================================

		LockCheckTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "lock_check_total",
				Help:      "Total resource lock check attempts",
			},
			[]string{"result"},
		),

		SkipTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "skip_total",
				Help:      "Total skipped WorkflowExecutions by reason",
			},
			[]string{"reason"},
		),

		LockDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "lock_duration_seconds",
				Help:      "Duration locks are held (from creation to release)",
				Buckets:   []float64{60, 120, 300, 600, 1200, 1800, 3600},
			},
		),

		ActiveLocks: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "active_locks",
				Help:      "Number of currently held resource locks",
			},
		),

		// ==========================================
		// Reconciliation Metrics
		// ==========================================

		ReconciliationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_duration_seconds",
				Help:      "Duration of reconciliation loops",
				Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"phase"},
		),

		ReconciliationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_total",
				Help:      "Total reconciliation attempts",
			},
			[]string{"phase", "result"},
		),

		ReconciliationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_errors_total",
				Help:      "Total reconciliation errors by category",
			},
			[]string{"category", "reason"},
		),

		// ==========================================
		// Queue Metrics
		// ==========================================

		QueueDepth: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "queue_depth",
				Help:      "Current number of items in reconcile queue",
			},
		),

		QueueLatency: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "queue_latency_seconds",
				Help:      "Time items spend in queue before processing",
				Buckets:   []float64{.1, .5, 1, 5, 10, 30, 60},
			},
		),

		// ==========================================
		// Business Outcome Metrics
		// ==========================================

		WorkflowDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "workflow_duration_seconds",
				Help:      "Total workflow execution duration (E2E)",
				Buckets:   []float64{10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"outcome"},
		),

		CostSavingsEstimate: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "cost_savings_estimate_dollars",
				Help:      "Estimated cost savings from skipped executions",
			},
			[]string{"skip_reason"},
		),
	}
}
```

---

## 🔄 Metric Recording Patterns

### Pattern 1: Phase Transition Recording

```go
// internal/controller/workflowexecution_controller.go

func (r *WorkflowExecutionReconciler) recordPhaseTransition(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	fromPhase, toPhase workflowexecutionv1.Phase,
) {
	// Record transition
	r.Metrics.PhaseTransitionsTotal.WithLabelValues(
		string(toPhase),
		string(fromPhase),
	).Inc()

	// Record duration in previous phase
	if wfe.Status.PhaseStartTime != nil {
		duration := time.Since(wfe.Status.PhaseStartTime.Time).Seconds()
		r.Metrics.PhaseDuration.WithLabelValues(string(fromPhase)).Observe(duration)
	}

	log.FromContext(ctx).Info("Phase transition recorded",
		"from", fromPhase,
		"to", toPhase,
		"name", wfe.Name)
}

// Usage in reconcile functions:
func (r *WorkflowExecutionReconciler) markRunning(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
	pipelineRunName string,
) (ctrl.Result, error) {
	fromPhase := wfe.Status.Phase
	wfe.Status.Phase = workflowexecutionv1.PhaseRunning
	wfe.Status.PipelineRunName = pipelineRunName
	wfe.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}

	if err := r.Status().Update(ctx, wfe); err != nil {
		return ctrl.Result{}, err
	}

	r.recordPhaseTransition(ctx, wfe, fromPhase, workflowexecutionv1.PhaseRunning)
	r.Metrics.ActiveLocks.Inc()

	return ctrl.Result{RequeueAfter: r.StatusCheckInterval}, nil
}
```

### Pattern 2: PipelineRun Creation with Timer

```go
func (r *WorkflowExecutionReconciler) createPipelineRun(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
	// Start timer for PR creation
	timer := prometheus.NewTimer(r.Metrics.PipelineRunCreationDuration)
	defer timer.ObserveDuration()

	pr := r.buildPipelineRun(wfe)

	if err := r.Create(ctx, pr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race condition - another controller instance created PR
			r.Metrics.PipelineRunCreationTotal.WithLabelValues("skipped").Inc()
			r.Metrics.SkipTotal.WithLabelValues("AlreadyExists").Inc()
			return r.markSkipped(ctx, wfe, "AlreadyExists", "PipelineRun already exists for target")
		}

		// Actual creation failure
		r.Metrics.PipelineRunCreationTotal.WithLabelValues("failure").Inc()
		r.Metrics.ReconciliationErrors.WithLabelValues("external", "PipelineRunCreateFailed").Inc()

		log.FromContext(ctx).Error(err, "Failed to create PipelineRun",
			"pipelinerun", pr.Name,
			"namespace", r.ExecutionNamespace)

		return ctrl.Result{RequeueAfter: r.calculateBackoff(wfe)}, nil
	}

	// Success
	r.Metrics.PipelineRunCreationTotal.WithLabelValues("success").Inc()
	return r.markRunning(ctx, wfe, pr.Name)
}
```

### Pattern 3: Lock Check Recording

```go
func (r *WorkflowExecutionReconciler) checkResourceLock(
	ctx context.Context,
	targetResource string,
) (blocked bool, reason string) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		// Optional: record lock check duration if needed
	}))
	defer timer.ObserveDuration()

	// Fast path: Check if Running WFE exists for this target (indexed query)
	wfeList := &workflowexecutionv1.WorkflowExecutionList{}
	if err := r.List(ctx, wfeList,
		client.MatchingFields{"spec.targetResource": targetResource},
	); err != nil {
		log.FromContext(ctx).Error(err, "Failed to query WFE index")
		r.Metrics.ReconciliationErrors.WithLabelValues("external", "IndexQueryFailed").Inc()
		return false, "" // Fail open - allow execution
	}

	for _, existing := range wfeList.Items {
		// Check for running execution (parallel block)
		if existing.Status.Phase == workflowexecutionv1.PhaseRunning {
			r.Metrics.LockCheckTotal.WithLabelValues("blocked").Inc()
			return true, "ResourceBusy"
		}

		// Check for recent completion (cooldown block)
		if (existing.Status.Phase == workflowexecutionv1.PhaseCompleted ||
			existing.Status.Phase == workflowexecutionv1.PhaseFailed) &&
			existing.Status.CompletionTime != nil {
			if time.Since(existing.Status.CompletionTime.Time) < r.CooldownPeriod {
				r.Metrics.LockCheckTotal.WithLabelValues("blocked").Inc()
				return true, "RecentlyRemediated"
			}
		}
	}

	r.Metrics.LockCheckTotal.WithLabelValues("allowed").Inc()
	return false, ""
}
```

### Pattern 4: Reconciliation Loop with Metrics

```go
func (r *WorkflowExecutionReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	// Start reconciliation timer
	start := time.Now()
	var phase workflowexecutionv1.Phase

	defer func() {
		duration := time.Since(start).Seconds()
		r.Metrics.ReconciliationDuration.WithLabelValues(string(phase)).Observe(duration)
	}()

	log := log.FromContext(ctx)

	// Fetch WFE
	wfe := &workflowexecutionv1.WorkflowExecution{}
	if err := r.Get(ctx, req.NamespacedName, wfe); err != nil {
		if apierrors.IsNotFound(err) {
			// WFE deleted - normal case
			r.Metrics.ReconciliationTotal.WithLabelValues("deleted", "success").Inc()
			return ctrl.Result{}, nil
		}
		r.Metrics.ReconciliationErrors.WithLabelValues("external", "GetFailed").Inc()
		return ctrl.Result{}, err
	}

	phase = wfe.Status.Phase

	// Reconcile by phase
	var result ctrl.Result
	var err error

	switch phase {
	case "", workflowexecutionv1.PhasePending:
		result, err = r.reconcilePending(ctx, wfe)
	case workflowexecutionv1.PhaseRunning:
		result, err = r.reconcileRunning(ctx, wfe)
	case workflowexecutionv1.PhaseCompleted, workflowexecutionv1.PhaseFailed:
		result, err = r.reconcileTerminal(ctx, wfe)
	case workflowexecutionv1.PhaseSkipped:
		// Terminal - no action needed
		r.Metrics.ReconciliationTotal.WithLabelValues(string(phase), "success").Inc()
		return ctrl.Result{}, nil
	}

	// Record result
	if err != nil {
		r.Metrics.ReconciliationTotal.WithLabelValues(string(phase), "error").Inc()
	} else if result.Requeue || result.RequeueAfter > 0 {
		r.Metrics.ReconciliationTotal.WithLabelValues(string(phase), "requeue").Inc()
	} else {
		r.Metrics.ReconciliationTotal.WithLabelValues(string(phase), "success").Inc()
	}

	return result, err
}
```

### Pattern 5: Business Outcome Recording

```go
func (r *WorkflowExecutionReconciler) recordWorkflowCompletion(
	ctx context.Context,
	wfe *workflowexecutionv1.WorkflowExecution,
) {
	// Record total workflow duration
	if wfe.CreationTimestamp.Time.IsZero() {
		return
	}

	duration := time.Since(wfe.CreationTimestamp.Time).Seconds()
	outcome := "completed"
	if wfe.Status.Phase == workflowexecutionv1.PhaseFailed {
		outcome = "failed"
	}

	r.Metrics.WorkflowDuration.WithLabelValues(outcome).Observe(duration)

	log.FromContext(ctx).Info("Workflow completed",
		"name", wfe.Name,
		"duration_seconds", duration,
		"outcome", outcome)
}

func (r *WorkflowExecutionReconciler) recordSkipCostSavings(
	ctx context.Context,
	reason string,
) {
	// Estimated cost per manual remediation (configurable)
	const estimatedManualCostDollars = 50.0 // DevOps time

	r.Metrics.CostSavingsEstimate.WithLabelValues(reason).Add(estimatedManualCostDollars)

	log.FromContext(ctx).Info("Cost savings recorded",
		"reason", reason,
		"estimated_savings_dollars", estimatedManualCostDollars)
}
```

---

## 📊 Metrics Cardinality Audit

**Status**: ✅ SAFE - Total < 1,000 unique combinations

### Metrics Inventory

| Metric | Labels | Cardinality | Status |
|--------|--------|-------------|--------|
| `phase_transitions_total` | phase(5), from_phase(5) | 25 | ✅ |
| `phase_duration_seconds` | phase(5) | 5 | ✅ |
| `pipelinerun_creation_duration_seconds` | (none) | 1 | ✅ |
| `pipelinerun_creation_total` | result(3) | 3 | ✅ |
| `lock_check_total` | result(2) | 2 | ✅ |
| `skip_total` | reason(3) | 3 | ✅ |
| `lock_duration_seconds` | (none) | 1 | ✅ |
| `active_locks` | (none) | 1 | ✅ |
| `reconciliation_duration_seconds` | phase(5) | 5 | ✅ |
| `reconciliation_total` | phase(5), result(3) | 15 | ✅ |
| `reconciliation_errors_total` | category(5), reason(~10) | 50 | ✅ |
| `queue_depth` | (none) | 1 | ✅ |
| `queue_latency_seconds` | (none) | 1 | ✅ |
| `workflow_duration_seconds` | outcome(2) | 2 | ✅ |
| `cost_savings_estimate_dollars` | skip_reason(3) | 3 | ✅ |
| **TOTAL** | | **~118** | ✅ |

**Cardinality Assessment**: ✅ Excellent (< 1,000)

---

## 🧪 Metrics Testing

**File**: `test/unit/workflowexecution/metrics_test.go`

```go
package workflowexecution

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
)

var _ = Describe("WorkflowExecution Metrics", func() {
	var (
		ctx context.Context
		m   *metrics.Metrics
		reg *prometheus.Registry
	)

	BeforeEach(func() {
		ctx = context.Background()
		reg = prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(reg)
	})

	Describe("Phase Transition Metrics", func() {
		It("should record phase transitions", func() {
			m.PhaseTransitionsTotal.WithLabelValues("Running", "Pending").Inc()

			count := testutil.ToFloat64(
				m.PhaseTransitionsTotal.WithLabelValues("Running", "Pending"),
			)
			Expect(count).To(Equal(1.0))
		})

		It("should record phase duration", func() {
			m.PhaseDuration.WithLabelValues("Pending").Observe(5.0)

			count := testutil.ToFloat64(
				m.PhaseDuration.WithLabelValues("Pending"),
			)
			Expect(count).To(BeNumerically(">", 0))
		})
	})

	Describe("PipelineRun Metrics", func() {
		It("should record successful creation", func() {
			m.PipelineRunCreationTotal.WithLabelValues("success").Inc()

			count := testutil.ToFloat64(
				m.PipelineRunCreationTotal.WithLabelValues("success"),
			)
			Expect(count).To(Equal(1.0))
		})

		It("should record creation duration", func() {
			timer := prometheus.NewTimer(m.PipelineRunCreationDuration)
			timer.ObserveDuration()

			// Verify histogram has observations
			count := testutil.ToFloat64(m.PipelineRunCreationDuration)
			Expect(count).To(BeNumerically(">", 0))
		})
	})

	Describe("Resource Locking Metrics", func() {
		It("should record lock checks", func() {
			m.LockCheckTotal.WithLabelValues("allowed").Inc()
			m.LockCheckTotal.WithLabelValues("blocked").Inc()

			allowed := testutil.ToFloat64(m.LockCheckTotal.WithLabelValues("allowed"))
			blocked := testutil.ToFloat64(m.LockCheckTotal.WithLabelValues("blocked"))

			Expect(allowed).To(Equal(1.0))
			Expect(blocked).To(Equal(1.0))
		})

		It("should record skip reasons", func() {
			m.SkipTotal.WithLabelValues("ResourceBusy").Inc()
			m.SkipTotal.WithLabelValues("RecentlyRemediated").Inc()

			busy := testutil.ToFloat64(m.SkipTotal.WithLabelValues("ResourceBusy"))
			recent := testutil.ToFloat64(m.SkipTotal.WithLabelValues("RecentlyRemediated"))

			Expect(busy).To(Equal(1.0))
			Expect(recent).To(Equal(1.0))
		})

		It("should track active locks", func() {
			m.ActiveLocks.Inc()
			Expect(testutil.ToFloat64(m.ActiveLocks)).To(Equal(1.0))

			m.ActiveLocks.Dec()
			Expect(testutil.ToFloat64(m.ActiveLocks)).To(Equal(0.0))
		})
	})

	Describe("Business Outcome Metrics", func() {
		It("should record workflow duration", func() {
			m.WorkflowDuration.WithLabelValues("completed").Observe(120.0)

			count := testutil.ToFloat64(
				m.WorkflowDuration.WithLabelValues("completed"),
			)
			Expect(count).To(BeNumerically(">", 0))
		})

		It("should record cost savings", func() {
			m.CostSavingsEstimate.WithLabelValues("ResourceBusy").Add(50.0)

			savings := testutil.ToFloat64(
				m.CostSavingsEstimate.WithLabelValues("ResourceBusy"),
			)
			Expect(savings).To(Equal(50.0))
		})
	})
})
```

---

## 📈 Metrics Endpoint Validation

```bash
# Verify metrics endpoint after Day 7
curl -s localhost:9090/metrics | grep workflowexecution | head -30

# Expected output includes:
# kubernaut_workflowexecution_phase_transitions_total{from_phase="Pending",phase="Running"} 42
# kubernaut_workflowexecution_phase_duration_seconds_bucket{phase="Running",le="60"} 35
# kubernaut_workflowexecution_pipelinerun_creation_duration_seconds_bucket{le="1"} 40
# kubernaut_workflowexecution_pipelinerun_creation_total{result="success"} 42
# kubernaut_workflowexecution_skip_total{reason="ResourceBusy"} 15
# kubernaut_workflowexecution_active_locks 3
# kubernaut_workflowexecution_reconciliation_duration_seconds_bucket{phase="Pending",le="0.1"} 100
# kubernaut_workflowexecution_workflow_duration_seconds_bucket{outcome="completed",le="120"} 38
# kubernaut_workflowexecution_cost_savings_estimate_dollars{skip_reason="ResourceBusy"} 750
```

---

## References

- [Metrics Implementation Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#metrics-implementation-2h--v20-enhanced)
- [Metrics Cardinality Audit Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#-metrics-cardinality-audit--v28-new---scope-common-all-services)
- [DD-005 Metrics Standards](../../../../architecture/decisions/DD-005-metrics-cardinality-management.md)

