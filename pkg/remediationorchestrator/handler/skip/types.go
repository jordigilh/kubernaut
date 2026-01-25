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

// Package skip provides handlers for WorkflowExecution skip reasons.
// Reference: REFACTOR-RO-002, BR-ORCH-032, BR-ORCH-033
package skip

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ========================================
// SKIP HANDLER INTERFACE (REFACTOR-RO-002)
// ========================================
//
// Handler defines the interface for handling WorkflowExecution skip reasons.
// Each skip reason has a dedicated handler that encapsulates its specific logic.
//
// WHY Extract Skip Handlers?
// - ✅ Single Responsibility: Each handler focuses on one skip reason
// - ✅ Testability: Handlers can be unit tested in isolation
// - ✅ Maintainability: Changes to one skip reason don't affect others
// - ✅ Extensibility: New skip reasons can be added without modifying existing code
//
// Reference: REFACTOR-RO-002 (skip handler extraction)
type Handler interface {
	// Handle processes a specific skip reason and returns the reconciliation result.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - rr: RemediationRequest being processed
	// - we: WorkflowExecution that was skipped
	// - sp: SignalProcessing CRD (for context)
	//
	// Returns:
	// - ctrl.Result: Reconciliation result (requeue, delay, etc.)
	// - error: Error if handling failed
	Handle(
		ctx context.Context,
		rr *remediationv1.RemediationRequest,
		we *workflowexecutionv1.WorkflowExecution,
		sp *signalprocessingv1.SignalProcessing,
	) (ctrl.Result, error)
}

// ========================================
// HANDLER CONTEXT (REFACTOR-RO-002)
// ========================================
//
// Context provides shared dependencies for all skip handlers.
// This avoids passing multiple parameters to each handler constructor.
//
// Reference: REFACTOR-RO-002 (skip handler extraction)
type Context struct {
	// Client is the Kubernetes client for API operations
	Client client.Client

	// Metrics for observability (DD-METRICS-001)
	Metrics *metrics.Metrics

	// NotificationCreator creates NotificationRequest CRDs
	// Used by manual review handlers (ExhaustedRetries, PreviousExecutionFailed)
	NotificationCreator interface {
		CreateManualReviewNotification(
			ctx context.Context,
			rr *remediationv1.RemediationRequest,
			we *workflowexecutionv1.WorkflowExecution,
			sp *signalprocessingv1.SignalProcessing,
		) (string, error)
	}

	// TransitionToFailedFunc delegates to reconciler's audit-emitting failure transition
	// Used by skip handlers to emit lifecycle.failed audit events
	TransitionToFailedFunc func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error)
}
