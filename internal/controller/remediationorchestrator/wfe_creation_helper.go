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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// WFECreationCallbacks provides the reconciler methods needed by the shared
// WFE creation flow. Used by both AnalyzingHandler and AwaitingApprovalHandler.
//
// Reference: Issue #666, TP-666-v1 §8.3
type WFECreationCallbacks struct {
	EmitWorkflowCreatedAudit func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis, preHash string)
	CreateWFE                func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, error)
	ResolveWorkflowDisplay   func(ctx context.Context, workflowID string) (string, string)
}

// CreateWFEAndTransition is the shared flow for creating a WorkflowExecution CRD,
// updating the RemediationRequest status with refs and display fields, and returning
// a TransitionIntent to advance to the Executing phase.
//
// This utility is used by both the Analyzing and AwaitingApproval handlers to avoid
// duplicating the ~80 lines of WFE creation, status update, and metric tracking.
//
// Reference: Issue #666, TP-666-v1 §8.3, BR-ORCH-025, BR-ORCH-031
func CreateWFEAndTransition(
	ctx context.Context,
	k8sClient client.Client,
	m *metrics.Metrics,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	preHash string,
	cbs WFECreationCallbacks,
) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	cbs.EmitWorkflowCreatedAudit(ctx, rr, ai, preHash)

	weName, err := cbs.CreateWFE(ctx, rr, ai)
	if err != nil {
		logger.Error(err, "Failed to create WorkflowExecution CRD")
		return phase.Requeue(config.RequeueGenericError, "WFE creation failed"), nil
	}
	logger.Info("Created WorkflowExecution CRD", "weName", weName)

	m.ChildCRDCreationsTotal.WithLabelValues("WorkflowExecution", rr.Namespace).Inc()

	var workflowDisplayName, confidence string
	if ai.Status.SelectedWorkflow != nil {
		actionType, workflowName := cbs.ResolveWorkflowDisplay(ctx, ai.Status.SelectedWorkflow.WorkflowID)
		workflowDisplayName = remediationrequest.FormatWorkflowDisplay(actionType, workflowName)
		confidence = remediationrequest.FormatConfidence(ai.Status.SelectedWorkflow.Confidence)
	}

	err = helpers.UpdateRemediationRequestStatus(ctx, k8sClient, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
			APIVersion: workflowexecutionv1.GroupVersion.String(),
			Kind:       "WorkflowExecution",
			Name:       weName,
			Namespace:  rr.Namespace,
		}
		if ai.Status.SelectedWorkflow != nil {
			rr.Status.SelectedWorkflowRef = &remediationv1.WorkflowReference{
				WorkflowID:           ai.Status.SelectedWorkflow.WorkflowID,
				Version:              ai.Status.SelectedWorkflow.Version,
				ExecutionBundle:      ai.Status.SelectedWorkflow.ExecutionBundle,
				ExecutionBundleDigest: ai.Status.SelectedWorkflow.ExecutionBundleDigest,
			}
			rr.Status.WorkflowDisplayName = workflowDisplayName
			rr.Status.Confidence = confidence
		}
		if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.RemediationTarget != nil {
			ar := ai.Status.RootCauseAnalysis.RemediationTarget
			rr.Status.RemediationTarget = &remediationv1.ResourceIdentifier{
				Kind:      ar.Kind,
				Name:      ar.Name,
				Namespace: ar.Namespace,
			}
			rr.Status.TargetDisplay = remediationrequest.FormatResourceDisplay(ar.Kind, ar.Name)
		}
		rr.Status.SignalTargetDisplay = remediationrequest.FormatResourceDisplay(
			rr.Spec.TargetResource.Kind, rr.Spec.TargetResource.Name)
		remediationrequest.SetWorkflowExecutionReady(rr, true,
			fmt.Sprintf("WorkflowExecution CRD %s created successfully", weName), m)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to set WorkflowExecutionRef in status")
		return phase.Requeue(config.RequeueGenericError, "WFE status update failed"), nil
	}
	logger.V(1).Info("Set WorkflowExecutionRef in status", "weName", weName)

	return phase.Advance(phase.Executing, "WFE created"), nil
}
