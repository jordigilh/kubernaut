package status

import (
	"context"
	"fmt"

	k8sretry "k8s.io/client-go/util/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// Manager handles AIAnalysis status updates with atomic operations
// Implements DD-PERF-001: Atomic Status Updates mandate
//
// This manager reduces K8s API calls by consolidating multiple status field updates
// into a single atomic operation, improving performance and reducing race conditions.
//
// AA-HAPI-001 Fix: Uses APIReader for cache-bypassed refetch to prevent stale reads
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

// AtomicStatusUpdate atomically updates phase and status fields in a single API call
// This prevents race conditions and reduces API server load (1 write instead of N writes)
//
// This method consolidates:
// - Phase transition
// - Multiple status field updates (Message, AnalysisSteps, ErrorDetails, etc.)
// - Condition updates
//
// Satisfies BR-AI-XXX and improves performance by 50-75% (DD-PERF-001)
//
// Example Usage:
//
//	err := m.AtomicStatusUpdate(ctx, analysis, func() error {
//	    // Update phase
//	    analysis.Status.Phase = "Analyzed"
//	    analysis.Status.Message = "Analysis complete"
//
//	    // Update analysis steps
//	    analysis.Status.AnalysisSteps = append(analysis.Status.AnalysisSteps, steps...)
//
//	    return nil
//	})
func (m *Manager) AtomicStatusUpdate(
	ctx context.Context,
	analysis *aianalysisv1alpha1.AIAnalysis,
	updateFunc func() error,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion (optimistic locking)
		// AA-HAPI-001: Use APIReader to bypass cache and get FRESH data
		// This prevents duplicate HAPI calls when cache is stale after status write
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
		return fmt.Errorf("failed to refetch AIAnalysis: %w", err)
	}

	// 2. Apply all status field changes in memory
	if err := updateFunc(); err != nil {
		return fmt.Errorf("failed to apply status updates: %w", err)
	}

	// 3. SINGLE ATOMIC UPDATE: Commit all changes together
	if err := m.client.Status().Update(ctx, analysis); err != nil {
		return fmt.Errorf("failed to atomically update status: %w", err)
	}

	return nil
})
}

// UpdatePhase updates the AIAnalysis phase with validation
// Satisfies BR-AI-XXX: CRD Lifecycle Management
//
// NOTE: For phase transitions that include multiple field updates, use AtomicStatusUpdate instead
// to reduce API calls and eliminate race conditions.
//
// This method uses retry logic to handle optimistic locking conflicts.
func (m *Manager) UpdatePhase(
	ctx context.Context,
	analysis *aianalysisv1alpha1.AIAnalysis,
	newPhase string,
	message string,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// 1. Refetch to get latest resourceVersion
		// AA-HAPI-001: Use APIReader to bypass cache
		if err := m.apiReader.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
			return fmt.Errorf("failed to refetch AIAnalysis: %w", err)
		}

		// 2. Update phase field
		analysis.Status.Phase = newPhase
		analysis.Status.Message = message

		// 3. Set completion timestamp for terminal phases
		if isTerminalPhase(newPhase) {
			now := metav1.Now()
			analysis.Status.CompletedAt = &now
		}

		// 4. Update status using status subresource
		if err := m.client.Status().Update(ctx, analysis); err != nil {
			return fmt.Errorf("failed to update phase: %w", err)
		}

		return nil
	})
}

// isTerminalPhase checks if a phase is terminal (no further transitions allowed)
// Terminal phases: Completed (success) and Failed (permanent failure)
func isTerminalPhase(phase string) bool {
	return phase == "Completed" || phase == "Failed"
}

