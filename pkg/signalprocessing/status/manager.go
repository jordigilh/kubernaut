package status

import (
	"context"
	"fmt"

	k8sretry "k8s.io/client-go/util/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Manager handles SignalProcessing status updates with atomic operations
// Implements DD-PERF-001: Atomic Status Updates mandate
//
// This manager reduces K8s API calls by consolidating multiple status field updates
// into a single atomic operation, improving performance and reducing race conditions.
//
// SP-CACHE-001 Fix: Uses APIReader for cache-bypassed refetch to prevent stale reads
type Manager struct {
	client    client.Client
	apiReader client.Reader // Direct API server access (no cache)
}

// NewManager creates a new status manager
// apiReader should be mgr.GetAPIReader() to bypass cache for fresh refetches
func NewManager(client client.Client, apiReader client.Reader) *Manager {
	return &Manager{
		client:    client,
		apiReader: apiReader,
	}
}

// FreshGet fetches an object directly from the API server, bypassing the informer cache.
// Use when reading cross-resource status that may have been recently updated by another actor.
// SP-CACHE-001: Prevents stale reads from informer cache lag.
func (m *Manager) FreshGet(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return m.apiReader.Get(ctx, key, obj)
}

// GetCurrentPhase fetches the current phase using the non-cached APIReader
// This is used for idempotency checks to prevent duplicate phase processing
// SP-BUG-PHASE-TRANSITION-001: Bypass cache to get FRESH phase data
func (m *Manager) GetCurrentPhase(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) (signalprocessingv1alpha1.SignalProcessingPhase, error) {
	fresh := &signalprocessingv1alpha1.SignalProcessing{}
	if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(sp), fresh); err != nil {
		return "", fmt.Errorf("failed to fetch current phase: %w", err)
	}
	return fresh.Status.Phase, nil
}

// AtomicStatusUpdate atomically updates phase and status fields in a single API call
// This prevents race conditions and reduces API server load (1 write instead of N writes)
//
// This method consolidates:
// - Phase transition
// - Multiple status field updates (KubernetesContext, RecoveryContext, Classifications, etc.)
// - Condition updates
// - Consecutive failure tracking
// - Error state management
//
// Satisfies BR-SP-XXX and improves performance by 66-75% (DD-PERF-001)
//
// Example Usage:
//
//	err := m.AtomicStatusUpdate(ctx, sp, func() error {
//	    // Update phase
//	    sp.Status.Phase = signalprocessingv1alpha1.PhaseClassifying
//	    sp.Status.Message = "Classification in progress"
//
//	    // Update contexts
//	    sp.Status.KubernetesContext = k8sCtx
//	    sp.Status.RecoveryContext = recoveryCtx
//
//	    return nil
//	})
func (m *Manager) AtomicStatusUpdate(
	ctx context.Context,
	sp *signalprocessingv1alpha1.SignalProcessing,
	updateFunc func() error,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion (optimistic locking)
		// SP-CACHE-001: Use APIReader to bypass cache and get FRESH data
		// This prevents stale reads that could break idempotency checks
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(sp), sp); err != nil {
			return fmt.Errorf("failed to refetch SignalProcessing: %w", err)
		}

		// 2. Apply all status field changes in memory
		if err := updateFunc(); err != nil {
			return fmt.Errorf("failed to apply status updates: %w", err)
		}

		// 3. SINGLE ATOMIC UPDATE: Commit all changes together
		if err := m.client.Status().Update(ctx, sp); err != nil {
			return fmt.Errorf("failed to atomically update status: %w", err)
		}

		return nil
	})
}

// UpdatePhase updates the SignalProcessing phase with validation
// Satisfies BR-SP-XXX: CRD Lifecycle Management
//
// NOTE: For phase transitions that include multiple field updates, use AtomicStatusUpdate instead
// to reduce API calls and eliminate race conditions.
//
// This method uses retry logic to handle optimistic locking conflicts.
func (m *Manager) UpdatePhase(
	ctx context.Context,
	sp *signalprocessingv1alpha1.SignalProcessing,
	newPhase signalprocessingv1alpha1.SignalProcessingPhase,
	message string,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion
		// SP-CACHE-001: Use APIReader to bypass cache
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(sp), sp); err != nil {
			return fmt.Errorf("failed to refetch SignalProcessing: %w", err)
		}

		// 2. Update phase field
		sp.Status.Phase = newPhase
	sp.Status.ObservedGeneration = sp.Generation // DD-CONTROLLER-001
		// Note: SignalProcessing doesn't have a Message field, status details tracked in Conditions
		_ = message // Suppress unused parameter warning

		// 3. Set completion timestamp for terminal phases
		if isTerminalPhase(newPhase) {
			now := metav1.Now()
			sp.Status.CompletionTime = &now
		}

		// 4. Update status using status subresource
		if err := m.client.Status().Update(ctx, sp); err != nil {
			return fmt.Errorf("failed to update phase: %w", err)
		}

		return nil
	})
}

// isTerminalPhase checks if a phase is terminal (no further transitions allowed)
// Terminal phases: Completed (success) and Failed (permanent failure)
func isTerminalPhase(phase signalprocessingv1alpha1.SignalProcessingPhase) bool {
	return phase == signalprocessingv1alpha1.PhaseCompleted ||
		phase == signalprocessingv1alpha1.PhaseFailed
}

