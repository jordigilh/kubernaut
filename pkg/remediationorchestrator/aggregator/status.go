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

package aggregator

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// StatusAggregator aggregates status from child CRDs into RemediationRequest.Status.
// Reference: BR-ORCH-029 (status aggregation), BR-ORCH-030 (phase tracking)
type StatusAggregator struct {
	client client.Client
}

// NewStatusAggregator creates a new StatusAggregator.
func NewStatusAggregator(c client.Client) *StatusAggregator {
	return &StatusAggregator{
		client: c,
	}
}

// AggregatedStatus holds the aggregated status from child CRDs.
// Used for phase determination and status reporting.
type AggregatedStatus struct {
	SignalProcessingPhase   string
	AIAnalysisPhase         string
	WorkflowExecutionPhase  string
	AllChildrenHealthy      bool
}

// AggregateStatus fetches child CRD statuses and returns aggregated status.
// Reference: BR-ORCH-029
func (a *StatusAggregator) AggregateStatus(ctx context.Context, rr *remediationv1.RemediationRequest) (*AggregatedStatus, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	result := &AggregatedStatus{AllChildrenHealthy: true}

	// Aggregate SignalProcessing status
	if rr.Status.SignalProcessingRef != nil {
		phase, err := a.getSignalProcessingPhase(ctx, rr.Status.SignalProcessingRef.Name, rr.Status.SignalProcessingRef.Namespace)
		if err != nil {
			logger.Error(err, "Failed to get SignalProcessing status")
			return nil, fmt.Errorf("failed to get SignalProcessing status: %w", err)
		}
		result.SignalProcessingPhase = phase
	}

	// Aggregate AIAnalysis status
	if rr.Status.AIAnalysisRef != nil {
		phase, err := a.getAIAnalysisPhase(ctx, rr.Status.AIAnalysisRef.Name, rr.Status.AIAnalysisRef.Namespace)
		if err != nil {
			logger.Error(err, "Failed to get AIAnalysis status")
			return nil, fmt.Errorf("failed to get AIAnalysis status: %w", err)
		}
		result.AIAnalysisPhase = phase
	}

	// Aggregate WorkflowExecution status
	if rr.Status.WorkflowExecutionRef != nil {
		phase, err := a.getWorkflowExecutionPhase(ctx, rr.Status.WorkflowExecutionRef.Name, rr.Status.WorkflowExecutionRef.Namespace)
		if err != nil {
			logger.Error(err, "Failed to get WorkflowExecution status")
			return nil, fmt.Errorf("failed to get WorkflowExecution status: %w", err)
		}
		result.WorkflowExecutionPhase = phase
	}

	logger.V(1).Info("Status aggregated successfully",
		"spPhase", result.SignalProcessingPhase,
		"aiPhase", result.AIAnalysisPhase,
		"wePhase", result.WorkflowExecutionPhase,
	)
	return result, nil
}

// getSignalProcessingPhase fetches the phase from a SignalProcessing CRD.
func (a *StatusAggregator) getSignalProcessingPhase(ctx context.Context, name, namespace string) (string, error) {
	sp := &signalprocessingv1.SignalProcessing{}
	if err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, sp); err != nil {
		return "", err
	}
	return string(sp.Status.Phase), nil
}

// getAIAnalysisPhase fetches the phase from an AIAnalysis CRD.
func (a *StatusAggregator) getAIAnalysisPhase(ctx context.Context, name, namespace string) (string, error) {
	ai := &aianalysisv1.AIAnalysis{}
	if err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ai); err != nil {
		return "", err
	}
	return string(ai.Status.Phase), nil
}

// getWorkflowExecutionPhase fetches the phase from a WorkflowExecution CRD.
func (a *StatusAggregator) getWorkflowExecutionPhase(ctx context.Context, name, namespace string) (string, error) {
	we := &workflowexecutionv1.WorkflowExecution{}
	if err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, we); err != nil {
		return "", err
	}
	return we.Status.Phase, nil
}

