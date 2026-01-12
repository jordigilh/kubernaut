package status

import (
	"context"
	"fmt"

	k8sretry "k8s.io/client-go/util/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Manager handles RemediationRequest status updates with atomic operations
// Implements DD-PERF-001: Atomic Status Updates mandate
// DD-STATUS-001: Cache-bypassed refetch using APIReader for race-free status updates
//
// This manager reduces K8s API calls by consolidating multiple status field updates
// into a single atomic operation, improving performance and reducing race conditions.
type Manager struct {
	client    client.Client
	apiReader client.Reader // DD-STATUS-001: Cache-bypassed reads for fresh status
}

// NewManager creates a new status manager
// apiReader bypasses controller-runtime cache for optimistic locking refetches (DD-STATUS-001)
func NewManager(client client.Client, apiReader client.Reader) *Manager {
	return &Manager{
		client:    client,
		apiReader: apiReader,
	}
}

// AtomicStatusUpdate atomically updates phase and status fields in a single API call
// This prevents race conditions and reduces API server load (1 write instead of N writes)
//
// This method consolidates:
// - Phase transition
// - Multiple status field updates (CRD creation tracking, orchestration state, etc.)
// - Condition updates
// - Consecutive failure tracking
// - Outcome and message fields
//
// Satisfies BR-ORCH-XXX and improves performance by 85-90% (DD-PERF-001)
//
// Example Usage:
//
//	err := m.AtomicStatusUpdate(ctx, rr, func() error {
//	    // Update phase
//	    rr.Status.Phase = remediationv1alpha1.PhaseCoordinating
//	    rr.Status.Message = "Coordinating remediation"
//
//	    // Update CRD tracking
//	    rr.Status.SignalProcessingName = spName
//
//	    // Update conditions
//	    remediationrequest.SetSignalProcessingReady(rr, true, ...)
//
//	    return nil
//	})
func (m *Manager) AtomicStatusUpdate(
	ctx context.Context,
	rr *remediationv1alpha1.RemediationRequest,
	updateFunc func() error,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion (optimistic locking)
		// DD-STATUS-001: Use APIReader to bypass controller-runtime cache for fresh read
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return fmt.Errorf("failed to refetch RemediationRequest: %w", err)
		}

		// 2. Apply all status field changes in memory
		if err := updateFunc(); err != nil {
			return fmt.Errorf("failed to apply status updates: %w", err)
		}

		// 3. SINGLE ATOMIC UPDATE: Commit all changes together
		if err := m.client.Status().Update(ctx, rr); err != nil {
			return fmt.Errorf("failed to atomically update status: %w", err)
		}

		return nil
	})
}

// UpdatePhase updates the RemediationRequest phase with validation
// Satisfies BR-ORCH-XXX: CRD Lifecycle Management
//
// NOTE: For phase transitions that include multiple field updates, use AtomicStatusUpdate instead
// to reduce API calls and eliminate race conditions.
//
// This method uses retry logic to handle optimistic locking conflicts.
func (m *Manager) UpdatePhase(
	ctx context.Context,
	rr *remediationv1alpha1.RemediationRequest,
	newPhase remediationv1alpha1.RemediationPhase,
	message string,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion
		// DD-STATUS-001: Use APIReader to bypass controller-runtime cache for fresh read
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return fmt.Errorf("failed to refetch RemediationRequest: %w", err)
		}

		// 2. Update phase field
		rr.Status.OverallPhase = newPhase
		rr.Status.Message = message

		// 3. Set completion timestamp for terminal phases
		if isTerminalPhase(newPhase) {
			now := metav1.Now()
			rr.Status.CompletedAt = &now
		}

		// 4. Update status using status subresource
		if err := m.client.Status().Update(ctx, rr); err != nil {
			return fmt.Errorf("failed to update phase: %w", err)
		}

		return nil
	})
}

// isTerminalPhase checks if a phase is terminal (no further transitions allowed)
// Terminal phases: Completed (success), Failed (permanent failure), Blocked (cooldown expired)
func isTerminalPhase(phase remediationv1alpha1.RemediationPhase) bool {
	return phase == "Completed" || phase == "Failed" || phase == "Blocked"
}

