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

// Package aggregator provides status aggregation logic for the Remediation Orchestrator.
//
// Business Requirements:
// - BR-ORCH-026: Status aggregation from child CRDs
// - BR-ORCH-010: State machine orchestration
// - BR-ORCH-033: Duplicate detection handling
package aggregator

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Verify imports are used
var (
	_ = aianalysisv1.GroupVersion
)

// AggregatedStatus contains combined status from all child CRDs.
// Reference: BR-ORCH-026
type AggregatedStatus struct {
	// SignalProcessing status
	SignalProcessingPhase string
	SignalProcessingReady bool
	EnrichmentResults     interface{} // From SignalProcessing.status.enrichmentResults

	// AIAnalysis status
	AIAnalysisPhase  string
	AIAnalysisReady  bool
	RequiresApproval bool
	Approved         bool
	SelectedWorkflow interface{} // From AIAnalysis.status.selectedWorkflow

	// WorkflowExecution status
	WorkflowExecutionPhase string
	WorkflowExecutionReady bool
	ExecutionSkipped       bool
	SkipReason             string
	DuplicateOf            string // For BR-ORCH-033

	// Overall status
	OverallReady bool
	Error        error
}

// StatusCondition represents a condition for a child CRD.
type StatusCondition struct {
	Type               string
	Status             string // "True", "False", "Unknown"
	Reason             string
	Message            string
	LastTransitionTime metav1.Time
}

// StatusAggregator aggregates status from child CRDs.
type StatusAggregator struct {
	client client.Client
}

// NewStatusAggregator creates a new StatusAggregator.
func NewStatusAggregator(c client.Client) *StatusAggregator {
	return &StatusAggregator{
		client: c,
	}
}

// Aggregate collects and combines status from all child CRDs.
// Reference: BR-ORCH-026
func (a *StatusAggregator) Aggregate(ctx context.Context, rr *remediationv1.RemediationRequest) (*AggregatedStatus, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
	)

	result := &AggregatedStatus{}

	// 1. Aggregate SignalProcessing status
	if rr.Status.RemediationProcessingRef != nil {
		sp, err := a.getSignalProcessing(ctx, rr.Namespace, rr.Status.RemediationProcessingRef.Name)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to get SignalProcessing")
				return nil, err
			}
			// Not found - leave empty
		} else {
			result.SignalProcessingPhase = string(sp.Status.Phase)
			result.SignalProcessingReady = sp.Status.Phase == signalprocessingv1.PhaseCompleted
			result.EnrichmentResults = sp.Status.EnrichmentResults
		}
	}

	// 2. Aggregate AIAnalysis status
	if rr.Status.AIAnalysisRef != nil {
		ai, err := a.getAIAnalysis(ctx, rr.Namespace, rr.Status.AIAnalysisRef.Name)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to get AIAnalysis")
				return nil, err
			}
		} else {
			result.AIAnalysisPhase = ai.Status.Phase
			result.RequiresApproval = ai.Status.ApprovalRequired
			// Approved is determined by: ApprovalRequired=true AND Phase=Completed
			// (If approval was required and phase is now Completed, approval was granted)
			result.Approved = ai.Status.ApprovalRequired && ai.Status.Phase == "Completed"
			result.AIAnalysisReady = ai.Status.Phase == "Completed"
			result.SelectedWorkflow = ai.Status.SelectedWorkflow
		}
	}

	// 3. Aggregate WorkflowExecution status
	if rr.Status.WorkflowExecutionRef != nil {
		we, err := a.getWorkflowExecution(ctx, rr.Namespace, rr.Status.WorkflowExecutionRef.Name)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to get WorkflowExecution")
				return nil, err
			}
		} else {
			result.WorkflowExecutionPhase = we.Status.Phase
			// WorkflowExecution uses "Completed" and "Skipped" as success phases
			result.WorkflowExecutionReady = we.Status.Phase == workflowexecutionv1.PhaseCompleted ||
				we.Status.Phase == workflowexecutionv1.PhaseSkipped
			result.ExecutionSkipped = we.Status.Phase == workflowexecutionv1.PhaseSkipped
			// Extract skip details (BR-ORCH-033)
			if we.Status.SkipDetails != nil {
				result.SkipReason = we.Status.SkipDetails.Reason
				if we.Status.SkipDetails.ConflictingWorkflow != nil {
					result.DuplicateOf = we.Status.SkipDetails.ConflictingWorkflow.Name
				}
			}
		}
	}

	// 4. Determine overall readiness
	result.OverallReady = result.SignalProcessingReady &&
		result.AIAnalysisReady &&
		result.WorkflowExecutionReady

	return result, nil
}

// CalculateProgress calculates the progress percentage based on completed child CRDs.
// Reference: BR-ORCH-026
func (a *StatusAggregator) CalculateProgress(status *AggregatedStatus) float64 {
	if status == nil {
		return 0.0
	}

	completed := 0
	total := 3 // SP, AI, WE

	if status.SignalProcessingReady {
		completed++
	}
	if status.AIAnalysisReady {
		completed++
	}
	if status.WorkflowExecutionReady {
		completed++
	}

	return float64(completed) / float64(total) * 100.0
}

// DetermineOverallPhase determines the overall phase based on child CRD statuses.
// Reference: BR-ORCH-010 (State machine orchestration)
func (a *StatusAggregator) DetermineOverallPhase(status *AggregatedStatus) string {
	if status == nil {
		return "Pending"
	}

	// Check for failures first
	// SignalProcessing uses lowercase phases ("failed"), others use capitalized ("Failed")
	if status.SignalProcessingPhase == string(signalprocessingv1.PhaseFailed) {
		return "Failed"
	}
	if status.AIAnalysisPhase == "Failed" {
		return "Failed"
	}
	if status.WorkflowExecutionPhase == workflowexecutionv1.PhaseFailed {
		return "Failed"
	}

	// Check for completion
	// WorkflowExecution uses "Completed" and "Skipped" as terminal success phases
	if status.WorkflowExecutionPhase == workflowexecutionv1.PhaseCompleted ||
		status.WorkflowExecutionPhase == workflowexecutionv1.PhaseSkipped {
		return "Completed"
	}

	// Check for executing phase
	if status.AIAnalysisReady && status.WorkflowExecutionPhase != "" {
		return "Executing"
	}

	// Check for awaiting approval
	if status.RequiresApproval && !status.Approved {
		return "AwaitingApproval"
	}

	// Check for analyzing phase
	if status.SignalProcessingReady && status.AIAnalysisPhase != "" {
		return "Analyzing"
	}

	// Check for processing phase
	if status.SignalProcessingPhase != "" {
		return "Processing"
	}

	// Default to Pending
	return "Pending"
}

// BuildStatusConditions builds conditions for each child CRD.
// Reference: BR-ORCH-026
func (a *StatusAggregator) BuildStatusConditions(status *AggregatedStatus) []StatusCondition {
	now := metav1.NewTime(time.Now())
	conditions := make([]StatusCondition, 0, 3)

	// SignalProcessing condition
	spStatus := "False"
	spReason := status.SignalProcessingPhase
	if status.SignalProcessingReady {
		spStatus = "True"
	}
	if spReason == "" {
		spReason = "NotStarted"
	}
	conditions = append(conditions, StatusCondition{
		Type:               "SignalProcessingReady",
		Status:             spStatus,
		Reason:             spReason,
		Message:            "SignalProcessing phase: " + spReason,
		LastTransitionTime: now,
	})

	// AIAnalysis condition
	aiStatus := "False"
	aiReason := status.AIAnalysisPhase
	if status.AIAnalysisReady {
		aiStatus = "True"
	}
	if aiReason == "" {
		aiReason = "NotStarted"
	}
	conditions = append(conditions, StatusCondition{
		Type:               "AIAnalysisReady",
		Status:             aiStatus,
		Reason:             aiReason,
		Message:            "AIAnalysis phase: " + aiReason,
		LastTransitionTime: now,
	})

	// WorkflowExecution condition
	weStatus := "False"
	weReason := status.WorkflowExecutionPhase
	if status.WorkflowExecutionReady {
		weStatus = "True"
	}
	if weReason == "" {
		weReason = "NotStarted"
	}
	conditions = append(conditions, StatusCondition{
		Type:               "WorkflowExecutionReady",
		Status:             weStatus,
		Reason:             weReason,
		Message:            "WorkflowExecution phase: " + weReason,
		LastTransitionTime: now,
	})

	return conditions
}

// Helper methods to fetch child CRDs

func (a *StatusAggregator) getSignalProcessing(ctx context.Context, namespace, name string) (*signalprocessingv1.SignalProcessing, error) {
	sp := &signalprocessingv1.SignalProcessing{}
	err := a.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, sp)
	return sp, err
}

func (a *StatusAggregator) getAIAnalysis(ctx context.Context, namespace, name string) (*aianalysisv1.AIAnalysis, error) {
	ai := &aianalysisv1.AIAnalysis{}
	err := a.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, ai)
	return ai, err
}

func (a *StatusAggregator) getWorkflowExecution(ctx context.Context, namespace, name string) (*workflowexecutionv1.WorkflowExecution, error) {
	we := &workflowexecutionv1.WorkflowExecution{}
	err := a.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, we)
	return we, err
}

