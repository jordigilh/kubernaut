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

	weName, err := cbs.CreateWFE(ctx, rr, ai)
	if err != nil {
		logger.Error(err, "Failed to create WorkflowExecution CRD")
		return phase.Requeue(config.RequeueGenericError, "WFE creation failed"), nil
	}
	logger.Info("Created WorkflowExecution CRD", "weName", weName)

	cbs.EmitWorkflowCreatedAudit(ctx, rr, ai, preHash)

	m.ChildCRDCreationsTotal.WithLabelValues("WorkflowExecution", rr.Namespace).Inc()

	if err := persistWFERefAndDisplay(ctx, k8sClient, m, rr, ai, weName); err != nil {
		logger.Error(err, "Failed to set WorkflowExecutionRef in status")
		return phase.Requeue(config.RequeueGenericError, "WFE status update failed"), nil
	}
	logger.V(1).Info("Set WorkflowExecutionRef in status", "weName", weName)

	return phase.Advance(phase.Executing, "WFE created"), nil
}

// persistWFERefAndDisplay records the newly created WorkflowExecution CRD on
// rr.Status along with the selected-workflow and remediation-target display
// fields (BR-ORCH-025/031), and marks WorkflowExecutionReady=true. Extracted
// from CreateWFEAndTransition (Wave 6 6e-i GREEN: funlen remediation) — pure
// code motion, no behavior change.
func persistWFERefAndDisplay(
	ctx context.Context,
	k8sClient client.Client,
	m *metrics.Metrics,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	weName string,
) error {
	rca := ai.Status.GetRCAResult()
	sw := rca.SelectedWorkflow
	var workflowDisplayName, confidence string
	if sw != nil {
		// Issue #1677 Phase 1 (DD-WORKFLOW-018 v1.1): ActionType/WorkflowName
		// are catalog-authoritative, Required fields on SelectedWorkflow
		// (never LLM-suppliable) -- no live DataStorage lookup needed.
		workflowDisplayName = remediationrequest.FormatWorkflowDisplay(
			sw.ActionType, sw.WorkflowName)
		confidence = remediationrequest.FormatConfidence(sw.Confidence)
	}

	return helpers.UpdateRemediationRequestStatus(ctx, k8sClient, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.EnsurePhaseProgress().WorkflowExecutionRef = &corev1.ObjectReference{
			APIVersion: workflowexecutionv1.GroupVersion.String(),
			Kind:       "WorkflowExecution",
			Name:       weName,
			Namespace:  rr.Namespace,
		}
		if sw != nil {
			rr.Status.EnsureWorkflowSelection().SelectedWorkflowRef = &remediationv1.WorkflowReference{
				WorkflowID:            sw.WorkflowID,
				Version:               sw.Version,
				ExecutionBundle:       sw.ExecutionBundle,
				ExecutionBundleDigest: sw.ExecutionBundleDigest,
			}
			rr.Status.EnsureWorkflowSelection().WorkflowDisplayName = workflowDisplayName
			rr.Status.EnsureWorkflowSelection().Confidence = confidence
		}
		if rca.RootCauseAnalysis != nil && rca.RootCauseAnalysis.RemediationTarget != nil {
			ar := rca.RootCauseAnalysis.RemediationTarget
			rr.Status.EnsureWorkflowSelection().RemediationTarget = &remediationv1.ResourceIdentifier{
				Kind:       ar.Kind,
				Name:       ar.Name,
				Namespace:  ar.Namespace,
				APIVersion: ar.APIVersion, // #1040
			}
			rr.Status.EnsureWorkflowSelection().TargetDisplay = remediationrequest.FormatResourceDisplay(ar.Kind, ar.Name)
		}
		rr.Status.EnsureWorkflowSelection().SignalTargetDisplay = remediationrequest.FormatResourceDisplay(
			rr.Spec.TargetResource.Kind, rr.Spec.TargetResource.Name)
		remediationrequest.SetWorkflowExecutionReady(rr, true,
			fmt.Sprintf("WorkflowExecution CRD %s created successfully", weName), m)
		return nil
	})
}
