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

package aianalysis

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// ========================================
// Deletion Handler
// Pattern: P2 - Controller Decomposition
// ========================================
//
// This handler implements AIAnalysis deletion logic.
//
// Reference: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md

// handleDeletion handles AIAnalysis deletion.
//
// Business Requirements:
//   - BR-AI-060: Graceful resource cleanup
//   - BR-AI-050: Audit deletion events
func (r *AIAnalysisReconciler) handleDeletion(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("name", analysis.Name)
	log.Info("Handling AIAnalysis deletion")

	// Cleanup logic: Record deletion audit event
	// Note: Audit writes blocked by Data Storage batch endpoint (NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md)
	// When unblocked, add: r.AuditClient.RecordDeletion(ctx, analysis)
	if r.AuditClient != nil {
		// Audit client available but batch endpoint not implemented by Data Storage
		// Events will be logged locally until Data Storage implements /api/v1/audit/events batch endpoint
		log.V(1).Info("Audit deletion event (batch endpoint pending)", "analysis", analysis.Name)
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(analysis, FinalizerName)
	if err := r.Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}


