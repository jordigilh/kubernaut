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

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	SignalProcessingPhase  string
	AIAnalysisPhase        string
	WorkflowExecutionPhase string
	AllChildrenHealthy     bool
}

// aggregateChildPhase resolves a single child CRD's phase via getPhase and applies it to
// *dest, falling back to graceful degradation (AllChildrenHealthy=false, empty phase) when
// the child is missing or unreachable. Shared by AggregateStatus for each of the three
// child CRD kinds to avoid duplicated nested error-handling per kind.
func (a *StatusAggregator) aggregateChildPhase(
	logger logr.Logger,
	result *AggregatedStatus,
	dest *string,
	refKind, refName string,
	getPhase func() (string, error),
) {
	phase, err := getPhase()
	if err == nil {
		*dest = phase
		return
	}
	if apierrors.IsNotFound(err) {
		logger.Info(refKind+" CRD not found, continuing gracefully", refKind+"Ref", refName)
	} else {
		logger.Error(err, "Failed to get "+refKind+" status")
	}
	result.AllChildrenHealthy = false
}

// AggregateStatus fetches child CRD statuses and returns aggregated status.
// Handles missing child CRDs gracefully - logs warning and continues with empty phase.
// Reference: BR-ORCH-029
func (a *StatusAggregator) AggregateStatus(ctx context.Context, rr *remediationv1.RemediationRequest) (*AggregatedStatus, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	result := &AggregatedStatus{AllChildrenHealthy: true}

	if ref := rr.Status.SignalProcessingRef; ref != nil {
		a.aggregateChildPhase(logger, result, &result.SignalProcessingPhase, "signalProcessing", ref.Name,
			func() (string, error) { return a.getSignalProcessingPhase(ctx, ref.Name, ref.Namespace) })
	}

	if ref := rr.Status.AIAnalysisRef; ref != nil {
		a.aggregateChildPhase(logger, result, &result.AIAnalysisPhase, "aiAnalysis", ref.Name,
			func() (string, error) { return a.getAIAnalysisPhase(ctx, ref.Name, ref.Namespace) })
	}

	if ref := rr.Status.WorkflowExecutionRef; ref != nil {
		a.aggregateChildPhase(logger, result, &result.WorkflowExecutionPhase, "workflowExecution", ref.Name,
			func() (string, error) { return a.getWorkflowExecutionPhase(ctx, ref.Name, ref.Namespace) })
	}

	logger.V(1).Info("Status aggregated successfully",
		"spPhase", result.SignalProcessingPhase,
		"aiPhase", result.AIAnalysisPhase,
		"wePhase", result.WorkflowExecutionPhase,
		"allChildrenHealthy", result.AllChildrenHealthy,
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
	return ai.Status.Phase, nil
}

// getWorkflowExecutionPhase fetches the phase from a WorkflowExecution CRD.
func (a *StatusAggregator) getWorkflowExecutionPhase(ctx context.Context, name, namespace string) (string, error) {
	we := &workflowexecutionv1.WorkflowExecution{}
	if err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, we); err != nil {
		return "", err
	}
	return we.Status.Phase, nil
}
