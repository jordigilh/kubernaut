/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package processing

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
)

// CRDUpdater handles updates to RemediationRequest CRDs
//
// DD-GATEWAY-009: State-Based Deduplication
// This component provides a focused interface for updating CRD fields
// during duplicate alert processing, specifically:
// - Incrementing occurrence count
// - Updating lastSeen timestamp
//
// Design:
// - Retry logic with exponential backoff for K8s API failures
// - Optimistic concurrency control (handle resourceVersion conflicts)
// - Structured logging for troubleshooting
type CRDUpdater struct {
	k8sClient  *k8s.Client
	logger     *zap.Logger
	maxRetries int           // Maximum retry attempts for K8s API calls
	retryDelay time.Duration // Initial retry delay (exponential backoff)
}

// NewCRDUpdater creates a new CRD updater
//
// Parameters:
// - k8sClient: Kubernetes client for CRD operations
// - logger: Structured logger
//
// Returns:
// - *CRDUpdater: Configured CRD updater with default retry policy (3 retries, 100ms initial delay)
func NewCRDUpdater(k8sClient *k8s.Client, logger *zap.Logger) *CRDUpdater {
	return &CRDUpdater{
		k8sClient:  k8sClient,
		logger:     logger,
		maxRetries: 10, // Increased from 3 to handle high concurrency (20+ concurrent requests)
		retryDelay: 50 * time.Millisecond, // Reduced from 100ms for faster retries
	}
}

// IncrementOccurrenceCount increments the occurrence count for a duplicate alert
//
// DD-GATEWAY-009: CRD Update Logic
// This method:
// 1. Fetches the current CRD from Kubernetes (to get resourceVersion)
// 2. Increments Spec.Deduplication.OccurrenceCount
// 3. Updates Spec.Deduplication.LastSeen timestamp
// 4. Calls K8s API to update the CRD
// 5. Retries on conflict (optimistic concurrency control)
//
// Retry Strategy:
// - K8s API conflicts → Retry with exponential backoff (max 3 attempts)
// - K8s API unavailable → Return error (no retry)
// - CRD not found → Return error (no retry, caller should create CRD)
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - namespace: Namespace of the RemediationRequest CRD
// - name: Name of the RemediationRequest CRD
//
// Returns:
// - error: K8s API errors (not found, timeout, etc.)
func (u *CRDUpdater) IncrementOccurrenceCount(ctx context.Context, namespace, name string) error {
	var lastErr error

	for attempt := 0; attempt < u.maxRetries; attempt++ {
		// Step 1: Fetch current CRD (to get latest resourceVersion)
		crd, err := u.k8sClient.GetRemediationRequest(ctx, namespace, name)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// CRD doesn't exist - this is a critical error
				// Caller expected CRD to exist (deduplication check passed)
				return fmt.Errorf("CRD not found (expected to exist): %s/%s: %w", namespace, name, err)
			}

			// K8s API unavailable - don't retry
			return fmt.Errorf("failed to fetch CRD for update: %s/%s: %w", namespace, name, err)
		}

		// Step 2: Update deduplication fields
		crd.Spec.Deduplication.OccurrenceCount++
		crd.Spec.Deduplication.LastSeen = metav1.Now()

		u.logger.Debug("Updating CRD occurrence count",
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Int("occurrence_count", crd.Spec.Deduplication.OccurrenceCount),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", u.maxRetries))

		// Step 3: Update CRD in Kubernetes
		err = u.k8sClient.UpdateRemediationRequest(ctx, crd)
		if err == nil {
			// Success!
			u.logger.Info("Successfully updated CRD occurrence count",
				zap.String("namespace", namespace),
				zap.String("name", name),
				zap.Int("occurrence_count", crd.Spec.Deduplication.OccurrenceCount))
			return nil
		}

		// Handle update errors
		if k8serrors.IsConflict(err) {
			// Optimistic concurrency conflict - resourceVersion mismatch
			// This happens when another process updated the CRD between fetch and update
			// Retry with exponential backoff
			lastErr = err
			backoff := u.retryDelay * time.Duration(1<<uint(attempt)) // Exponential backoff

			u.logger.Debug("CRD update conflict, retrying",
				zap.String("namespace", namespace),
				zap.String("name", name),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff),
				zap.Error(err))

			time.Sleep(backoff)
			continue
		}

		// Other K8s API error (timeout, not found after fetch, etc.) - don't retry
		return fmt.Errorf("failed to update CRD: %s/%s: %w", namespace, name, err)
	}

	// Max retries exhausted
	return fmt.Errorf("max retries exhausted updating CRD: %s/%s (last error: %w)", namespace, name, lastErr)
}

// UpdateWithRetry provides a generic retry wrapper for CRD updates
//
// This method can be used for future update operations beyond occurrence count
// (e.g., updating priority, adding metadata, etc.)
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - namespace: Namespace of the CRD
// - name: Name of the CRD
// - updateFunc: Function that modifies the CRD (called inside retry loop)
//
// Returns:
// - error: K8s API errors or max retries exhausted
func (u *CRDUpdater) UpdateWithRetry(
	ctx context.Context,
	namespace, name string,
	updateFunc func(*remediationv1alpha1.RemediationRequest) error,
) error {
	var lastErr error

	for attempt := 0; attempt < u.maxRetries; attempt++ {
		// Fetch current CRD
		crd, err := u.k8sClient.GetRemediationRequest(ctx, namespace, name)
		if err != nil {
			return fmt.Errorf("failed to fetch CRD for update: %s/%s: %w", namespace, name, err)
		}

		// Apply user-defined update
		if err := updateFunc(crd); err != nil {
			return fmt.Errorf("update function failed: %w", err)
		}

		// Update CRD in Kubernetes
		err = u.k8sClient.UpdateRemediationRequest(ctx, crd)
		if err == nil {
			return nil // Success
		}

		// Handle conflicts with retry
		if k8serrors.IsConflict(err) {
			lastErr = err
			backoff := u.retryDelay * time.Duration(1<<uint(attempt))
			time.Sleep(backoff)
			continue
		}

		// Other errors - don't retry
		return fmt.Errorf("failed to update CRD: %s/%s: %w", namespace, name, err)
	}

	return fmt.Errorf("max retries exhausted: %s/%s (last error: %w)", namespace, name, lastErr)
}
