package status

import (
	"context"
	"fmt"

	k8sretry "k8s.io/client-go/util/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Manager handles WorkflowExecution status updates with atomic operations
// Implements DD-PERF-001: Atomic Status Updates mandate
//
// This manager reduces K8s API calls by consolidating multiple status field updates
// into a single atomic operation, improving performance and reducing race conditions.
type Manager struct {
	client client.Client
}

// NewManager creates a new status manager
func NewManager(client client.Client) *Manager {
	return &Manager{
		client: client,
	}
}

// AtomicStatusUpdate atomically updates phase and status fields in a single API call
// This prevents race conditions and reduces API server load (1 write instead of N writes)
//
// This method consolidates:
// - Phase transition
// - Multiple status field updates (Duration, CompletionTime, FailureDetails, etc.)
// - Condition updates (TektonPipelineComplete, audit conditions, etc.)
//
// Satisfies BR-WE-003, BR-WE-006, and improves performance by 50%+ (DD-PERF-001)
//
// Example Usage:
//
//	err := m.AtomicStatusUpdate(ctx, wfe, func() error {
//	    // Update phase
//	    wfe.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
//	    wfe.Status.CompletionTime = &now
//	    wfe.Status.Duration = duration.String()
//
//	    // Update conditions
//	    weconditions.SetTektonPipelineComplete(wfe, true, ...)
//	    weconditions.SetAuditRecorded(wfe, true, ...)
//
//	    return nil
//	})
func (m *Manager) AtomicStatusUpdate(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	updateFunc func() error,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion (optimistic locking)
		if err := m.client.Get(ctx, client.ObjectKeyFromObject(wfe), wfe); err != nil {
			return fmt.Errorf("failed to refetch WorkflowExecution: %w", err)
		}

		// 2. Apply all status field changes in memory
		if err := updateFunc(); err != nil {
			return fmt.Errorf("failed to apply status updates: %w", err)
		}

		// 3. SINGLE ATOMIC UPDATE: Commit all changes together
		if err := m.client.Status().Update(ctx, wfe); err != nil {
			return fmt.Errorf("failed to atomically update status: %w", err)
		}

		return nil
	})
}

// UpdatePhase updates the WorkflowExecution phase with validation
// Satisfies BR-WE-003: CRD Lifecycle Management
//
// NOTE: For phase transitions that include multiple field updates, use AtomicStatusUpdate instead
// to reduce API calls and eliminate race conditions.
//
// This method uses retry logic to handle optimistic locking conflicts.
func (m *Manager) UpdatePhase(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	newPhase string,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion
		if err := m.client.Get(ctx, client.ObjectKeyFromObject(wfe), wfe); err != nil {
			return fmt.Errorf("failed to refetch WorkflowExecution: %w", err)
		}

		// 2. Update phase field
		wfe.Status.Phase = newPhase

		// 3. Set completion time for terminal phases
		if isTerminalPhase(newPhase) {
			now := metav1.Now()
			wfe.Status.CompletionTime = &now
		}

		// 4. Update status using status subresource
		if err := m.client.Status().Update(ctx, wfe); err != nil {
			return fmt.Errorf("failed to update phase: %w", err)
		}

		return nil
	})
}

// isTerminalPhase checks if a phase is terminal (no further transitions allowed)
func isTerminalPhase(phase string) bool {
	return phase == workflowexecutionv1alpha1.PhaseCompleted ||
		phase == workflowexecutionv1alpha1.PhaseFailed
}

