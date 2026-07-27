/*
Copyright 2026 Jordi Gil.

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

package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// cascadeTerminalToChildren patches non-terminal child CRDs to a terminal
// state when the parent RR enters a terminal phase. Follows Kubernetes-native
// parent-manages-children convention (Issue #1421).
//
// Children are accessed via status refs on the RR (O(1) per child, no List).
// Already-terminal children are skipped (idempotent).
// Errors are logged internally but never returned to the caller -- a
// child-cascade failure must not fail (or be treated as failing) the
// parent's terminal transition.
func (r *Reconciler) cascadeTerminalToChildren(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)
	parentPhase := string(rr.Status.OverallPhase)
	message := fmt.Sprintf("Parent RR entered terminal phase: %s", parentPhase)

	if rr.Status.AIAnalysisRef != nil {
		if err := r.cascadeToAIAnalysis(ctx, rr.Status.AIAnalysisRef.Name, rr.Status.AIAnalysisRef.Namespace, message); err != nil {
			logger.Error(err, "Failed to cascade terminal to AIAnalysis",
				"aianalysis", rr.Status.AIAnalysisRef.Name)
		}
	}

	if rr.Status.SignalProcessingRef != nil {
		if err := r.cascadeToSignalProcessing(ctx, rr.Status.SignalProcessingRef.Name, rr.Status.SignalProcessingRef.Namespace, message); err != nil {
			logger.Error(err, "Failed to cascade terminal to SignalProcessing",
				"signalprocessing", rr.Status.SignalProcessingRef.Name)
		}
	}

	if rr.Status.WorkflowExecutionRef != nil {
		if err := r.cascadeToWorkflowExecution(ctx, rr.Status.WorkflowExecutionRef.Name, rr.Status.WorkflowExecutionRef.Namespace, message); err != nil {
			logger.Error(err, "Failed to cascade terminal to WorkflowExecution",
				"workflowexecution", rr.Status.WorkflowExecutionRef.Name)
		}
	}
}

func (r *Reconciler) cascadeToAIAnalysis(ctx context.Context, name, namespace, message string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		ai := &aianalysisv1.AIAnalysis{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ai); err != nil {
			return client.IgnoreNotFound(err)
		}

		if isAIAnalysisTerminal(ai.Status.Phase) {
			return nil
		}

		now := metav1.Now()
		ai.Status.Phase = aianalysisv1.PhaseFailed
		ai.Status.Reason = aianalysisv1.ReasonParentCancelled
		ai.Status.Message = message
		ai.Status.CompletedAt = &now
		ai.Status.ObservedGeneration = ai.Generation

		return r.client.Status().Update(ctx, ai)
	})
}

func (r *Reconciler) cascadeToSignalProcessing(ctx context.Context, name, namespace, message string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		sp := &signalprocessingv1.SignalProcessing{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, sp); err != nil {
			return client.IgnoreNotFound(err)
		}

		if isSPTerminal(sp.Status.Phase) {
			return nil
		}

		sp.Status.Phase = signalprocessingv1.PhaseFailed
		sp.Status.Error = message

		return r.client.Status().Update(ctx, sp)
	})
}

func (r *Reconciler) cascadeToWorkflowExecution(ctx context.Context, name, namespace, message string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		we := &workflowexecutionv1.WorkflowExecution{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, we); err != nil {
			return client.IgnoreNotFound(err)
		}

		if isWETerminal(we.Status.Phase) {
			return nil
		}

		we.Status.Phase = workflowexecutionv1.PhaseFailed
		we.Status.FailureReason = message

		return r.client.Status().Update(ctx, we)
	})
}

func isAIAnalysisTerminal(phase string) bool {
	return phase == aianalysisv1.PhaseCompleted || phase == aianalysisv1.PhaseFailed
}

func isSPTerminal(phase signalprocessingv1.SignalProcessingPhase) bool {
	return phase == signalprocessingv1.PhaseCompleted || phase == signalprocessingv1.PhaseFailed
}

func isWETerminal(phase string) bool {
	return phase == workflowexecutionv1.PhaseCompleted || phase == workflowexecutionv1.PhaseFailed
}
