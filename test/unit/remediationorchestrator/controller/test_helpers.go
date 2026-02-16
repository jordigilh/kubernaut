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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
)

// setupScheme creates a scheme with all CRD types registered
func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = remediationv1.AddToScheme(scheme)
	_ = signalprocessingv1.AddToScheme(scheme)
	_ = aianalysisv1.AddToScheme(scheme)
	_ = workflowexecutionv1.AddToScheme(scheme)
	_ = notificationv1.AddToScheme(scheme)
	_ = eav1.AddToScheme(scheme)
	return scheme
}

// MockRoutingEngine is a mock implementation for unit tests
type MockRoutingEngine struct{}

func (m *MockRoutingEngine) CheckBlockingConditions(ctx context.Context, rr *remediationv1.RemediationRequest, workflowID string) (*routing.BlockingCondition, error) {
	return nil, nil // Always return not blocked for unit tests
}

func (m *MockRoutingEngine) CheckUnmanagedResource(ctx context.Context, rr *remediationv1.RemediationRequest) *routing.BlockingCondition {
	return nil // Always return managed for unit tests
}

func (m *MockRoutingEngine) Config() routing.Config {
	return routing.Config{
		ConsecutiveFailureThreshold: 3,
		ConsecutiveFailureCooldown:  3600,
		RecentlyRemediatedCooldown:  300,
	}
}

func (m *MockRoutingEngine) CalculateExponentialBackoff(consecutiveFailures int32) time.Duration {
	return time.Duration(consecutiveFailures) * time.Minute
}

// ptr is a helper to get pointer to bool
func ptr(b bool) *bool {
	return &b
}

// newWorkflowExecutionCompleted creates a completed WorkflowExecution CRD
func newWorkflowExecutionCompleted(name, namespace, rrName string) *workflowexecutionv1.WorkflowExecution {
	we := newWorkflowExecution(name, namespace, rrName, workflowexecutionv1.PhaseCompleted)
	now := metav1.Now()
	we.Status.CompletionTime = &now
	return we
}

// newRemediationRequest creates a basic RemediationRequest for testing
func newRemediationRequest(name, namespace string, phase remediationv1.RemediationPhase) *remediationv1.RemediationRequest {
	now := metav1.Now()
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: now,
			UID:               types.UID(name + "-uid"),
			Generation:        1, // K8s increments on create/update
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalName:        "test-signal",
			SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			Severity:          "critical",
			SignalType:        "prometheus",
			TargetType:        "kubernetes",
			FiringTime:        now,
			ReceivedTime:      now,
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: phase,
		},
	}
}

// newRemediationRequestWithChildRefs creates an RR with child CRD references
func newRemediationRequestWithChildRefs(name, namespace string, phase remediationv1.RemediationPhase, spName, aiName, weName string) *remediationv1.RemediationRequest {
	rr := newRemediationRequest(name, namespace, phase)
	rr.Status.StartTime = &metav1.Time{Time: time.Now()}

	if spName != "" {
		rr.Status.SignalProcessingRef = &corev1.ObjectReference{
			APIVersion: signalprocessingv1.GroupVersion.String(),
			Kind:       "SignalProcessing",
			Name:       spName,
			Namespace:  namespace,
		}
	}

	if aiName != "" {
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{
			APIVersion: aianalysisv1.GroupVersion.String(),
			Kind:       "AIAnalysis",
			Name:       aiName,
			Namespace:  namespace,
		}
	}

	if weName != "" {
		rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
			APIVersion: workflowexecutionv1.GroupVersion.String(),
			Kind:       "WorkflowExecution",
			Name:       weName,
			Namespace:  namespace,
		}
	}

	return rr
}

// newSignalProcessingCompleted creates a completed SignalProcessing CRD
func newSignalProcessingCompleted(name, namespace, rrName string) *signalprocessingv1.SignalProcessing {
	sp := newSignalProcessing(name, namespace, rrName, signalprocessingv1.PhaseCompleted)
	now := metav1.Now()
	sp.Status.CompletionTime = &now
	return sp
}

// newSignalProcessing creates a SignalProcessing CRD
func newSignalProcessing(name, namespace, rrName string, phase signalprocessingv1.SignalProcessingPhase) *signalprocessingv1.SignalProcessing {
	now := metav1.Now()
	return &signalprocessingv1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: now,
			UID:               types.UID(name + "-uid"),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					UID:        types.UID(rrName + "-uid"),
					Controller: ptr(true),
				},
			},
		},
		Spec: signalprocessingv1.SignalProcessingSpec{
			RemediationRequestRef: signalprocessingv1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rrName,
				Namespace:  namespace,
			},
		},
		Status: signalprocessingv1.SignalProcessingStatus{
			Phase: phase,
		},
	}
}

// newAIAnalysisCompleted creates a completed AIAnalysis CRD
func newAIAnalysisCompleted(name, namespace, rrName string, confidence float64, workflowID string) *aianalysisv1.AIAnalysis {
	ai := newAIAnalysis(name, namespace, rrName, aianalysisv1.PhaseCompleted)
	now := metav1.Now()
	ai.Status.CompletedAt = &now
	ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
		WorkflowID:     workflowID,
		Version:        "v1",
		ContainerImage: "test-image:latest",
		Confidence:     confidence,
	}

	// Set approval required based on confidence
	if confidence < 0.8 {
		ai.Status.ApprovalRequired = true
		ai.Status.ApprovalReason = "Low confidence score requires approval"
	}

	return ai
}

// newAIAnalysis creates an AIAnalysis CRD
func newAIAnalysis(name, namespace, rrName string, phase string) *aianalysisv1.AIAnalysis {
	now := metav1.Now()
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: now,
			UID:               types.UID(name + "-uid"),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					UID:        types.UID(rrName + "-uid"),
					Controller: ptr(true),
				},
			},
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rrName,
				Namespace:  namespace,
			},
		},
		Status: aianalysisv1.AIAnalysisStatus{
			Phase: phase,
		},
	}
}

// newWorkflowExecution creates a WorkflowExecution CRD
func newWorkflowExecution(name, namespace, rrName string, phase string) *workflowexecutionv1.WorkflowExecution {
	now := metav1.Now()
	return &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: now,
			UID:               types.UID(name + "-uid"),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					UID:        types.UID(rrName + "-uid"),
					Controller: ptr(true),
				},
			},
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rrName,
				Namespace:  namespace,
			},
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				WorkflowID: "test-workflow",
				Version:    "v1",
			},
			TargetResource: namespace + "/deployment/test-app",
		},
		Status: workflowexecutionv1.WorkflowExecutionStatus{
			Phase: phase,
		},
	}
}

// newWorkflowExecutionFailed creates a failed WorkflowExecution CRD
func newWorkflowExecutionFailed(name, namespace, rrName, message string) *workflowexecutionv1.WorkflowExecution {
	we := newWorkflowExecution(name, namespace, rrName, workflowexecutionv1.PhaseFailed)
	now := metav1.Now()
	we.Status.CompletionTime = &now
	we.Status.FailureReason = message
	return we
}

// newRemediationApprovalRequestApproved creates an approved RAR
func newRemediationApprovalRequestApproved(name, namespace, rrName, decidedBy string) *remediationv1.RemediationApprovalRequest {
	now := metav1.Now()
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					UID:        types.UID(rrName + "-uid"),
					Controller: ptr(true),
				},
			},
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rrName,
				Namespace:  namespace,
			},
			AIAnalysisRef: remediationv1.ObjectRef{
				Name: "ai-" + rrName,
			},
			Confidence:           0.4,
			ConfidenceLevel:      "low",
			Reason:               "Low confidence score requires approval",
			RecommendedWorkflow:  remediationv1.RecommendedWorkflowSummary{},
			InvestigationSummary: "Test investigation summary",
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			Decision:        remediationv1.ApprovalDecisionApproved,
			DecidedBy:       decidedBy,
			DecidedAt:       &now,
			DecisionMessage: "Approved for execution",
		},
	}
}

// newRemediationApprovalRequestRejected creates a rejected RAR
func newRemediationApprovalRequestRejected(name, namespace, rrName, decidedBy, reason string) *remediationv1.RemediationApprovalRequest {
	now := metav1.Now()
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					UID:        types.UID(rrName + "-uid"),
					Controller: ptr(true),
				},
			},
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rrName,
				Namespace:  namespace,
			},
			AIAnalysisRef: remediationv1.ObjectRef{
				Name: "ai-" + rrName,
			},
			Confidence:           0.4,
			ConfidenceLevel:      "low",
			Reason:               "Low confidence score requires approval",
			RecommendedWorkflow:  remediationv1.RecommendedWorkflowSummary{},
			InvestigationSummary: "Test investigation summary",
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			Decision:        remediationv1.ApprovalDecisionRejected,
			DecidedBy:       decidedBy,
			DecidedAt:       &now,
			DecisionMessage: reason,
		},
	}
}

// newRemediationRequestWithTimeout creates an RR with a specific start time (for global timeout testing)
func newRemediationRequestWithTimeout(name, namespace string, phase remediationv1.RemediationPhase, timeDelta time.Duration) *remediationv1.RemediationRequest {
	startTime := metav1.NewTime(time.Now().Add(timeDelta))
	creationTime := metav1.NewTime(time.Now().Add(timeDelta))
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: creationTime,
			UID:               types.UID(name + "-uid"),
			Generation:        1, // K8s increments on create/update
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalName:        "test-signal",
			SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			Severity:          "critical",
			SignalType:        "prometheus",
			TargetType:        "kubernetes",
			FiringTime:        creationTime,
			ReceivedTime:      creationTime,
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: phase,
			StartTime:    &startTime,
		},
	}
}

// newRemediationRequestWithGatewayMetadata creates an RR with Gateway metadata
// BR-ORCH-038: Gateway deduplication metadata must be preserved across phase transitions
func newRemediationRequestWithGatewayMetadata(name, namespace string) *remediationv1.RemediationRequest {
	rr := newRemediationRequest(name, namespace, remediationv1.PhasePending)
	now := metav1.Now()
	rr.Status.Deduplication = &remediationv1.DeduplicationStatus{
		OccurrenceCount: 1,
		FirstSeenAt:     &now,
		LastSeenAt:      &now,
	}
	// Gateway passes deduplication metadata via SignalLabels
	rr.Spec.SignalLabels = map[string]string{
		"dedup_group": "test-group",
		"process_id":  "test-process",
	}
	return rr
}

// newSignalProcessingFailed creates a failed SignalProcessing CRD
func newSignalProcessingFailed(name, namespace, rrName, message string) *signalprocessingv1.SignalProcessing {
	sp := newSignalProcessing(name, namespace, rrName, signalprocessingv1.PhaseFailed)
	now := metav1.Now()
	sp.Status.CompletionTime = &now
	sp.Status.Error = message
	return sp
}

// newAIAnalysisWorkflowNotNeeded creates an AIAnalysis that determined no workflow is needed
func newAIAnalysisWorkflowNotNeeded(name, namespace, rrName, reason string) *aianalysisv1.AIAnalysis {
	ai := newAIAnalysis(name, namespace, rrName, aianalysisv1.PhaseCompleted)
	now := metav1.Now()
	ai.Status.CompletedAt = &now
	ai.Status.Message = reason
	ai.Status.Reason = "WorkflowNotNeeded"
	return ai
}

// newAIAnalysisFailed creates a failed AIAnalysis CRD
func newAIAnalysisFailed(name, namespace, rrName, message string) *aianalysisv1.AIAnalysis {
	ai := newAIAnalysis(name, namespace, rrName, aianalysisv1.PhaseFailed)
	now := metav1.Now()
	ai.Status.CompletedAt = &now
	ai.Status.Message = message
	ai.Status.Reason = "AnalysisFailed"
	return ai
}

// newWorkflowExecutionSucceeded creates a succeeded WorkflowExecution CRD
func newWorkflowExecutionSucceeded(name, namespace, rrName string) *workflowexecutionv1.WorkflowExecution {
	return newWorkflowExecutionCompleted(name, namespace, rrName)
}

// newRemediationApprovalRequestExpired creates an expired RAR
func newRemediationApprovalRequestExpired(name, namespace, rrName string) *remediationv1.RemediationApprovalRequest {
	pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					UID:        types.UID(rrName + "-uid"),
					Controller: ptr(true),
				},
			},
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rrName,
				Namespace:  namespace,
			},
			AIAnalysisRef: remediationv1.ObjectRef{
				Name: "ai-" + rrName,
			},
			Confidence:           0.4,
			ConfidenceLevel:      "low",
			Reason:               "Low confidence score requires approval",
			RequiredBy:           pastTime,
			RecommendedWorkflow:  remediationv1.RecommendedWorkflowSummary{},
			InvestigationSummary: "Test investigation summary",
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			Decision: remediationv1.ApprovalDecisionExpired,
		},
	}
}

// newRemediationApprovalRequestPending creates a pending RAR
func newRemediationApprovalRequestPending(name, namespace, rrName string) *remediationv1.RemediationApprovalRequest {
	futureTime := metav1.NewTime(time.Now().Add(1 * time.Hour))
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					UID:        types.UID(rrName + "-uid"),
					Controller: ptr(true),
				},
			},
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rrName,
				Namespace:  namespace,
			},
			AIAnalysisRef: remediationv1.ObjectRef{
				Name: "ai-" + rrName,
			},
			Confidence:           0.4,
			ConfidenceLevel:      "low",
			Reason:               "Low confidence score requires approval",
			RequiredBy:           futureTime,
			RecommendedWorkflow:  remediationv1.RecommendedWorkflowSummary{},
			InvestigationSummary: "Test investigation summary",
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			Decision: remediationv1.ApprovalDecisionPending,
		},
	}
}

// newRemediationRequestWithPhaseTimeout creates an RR with a phase-specific timeout
func newRemediationRequestWithPhaseTimeout(name, namespace string, phase remediationv1.RemediationPhase, childRefName string, timeDelta time.Duration) *remediationv1.RemediationRequest {
	rr := newRemediationRequestWithTimeout(name, namespace, phase, 0)

	// Set phase-specific start time
	phaseStartTime := metav1.NewTime(time.Now().Add(timeDelta))
	switch phase {
	case remediationv1.PhaseProcessing:
		rr.Status.ProcessingStartTime = &phaseStartTime
		if childRefName != "" {
			rr.Status.SignalProcessingRef = &corev1.ObjectReference{
				APIVersion: signalprocessingv1.GroupVersion.String(),
				Kind:       "SignalProcessing",
				Name:       childRefName,
				Namespace:  namespace,
			}
		}
	case remediationv1.PhaseAnalyzing:
		rr.Status.AnalyzingStartTime = &phaseStartTime
		if childRefName != "" {
			rr.Status.AIAnalysisRef = &corev1.ObjectReference{
				APIVersion: aianalysisv1.GroupVersion.String(),
				Kind:       "AIAnalysis",
				Name:       childRefName,
				Namespace:  namespace,
			}
		}
	case remediationv1.PhaseExecuting:
		rr.Status.ExecutingStartTime = &phaseStartTime
		if childRefName != "" {
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				APIVersion: workflowexecutionv1.GroupVersion.String(),
				Kind:       "WorkflowExecution",
				Name:       childRefName,
				Namespace:  namespace,
			}
		}
	}

	return rr
}

// newRemediationRequestWithBothTimeouts creates an RR with both global and phase-specific timeouts
func newRemediationRequestWithBothTimeouts(name, namespace string, phase remediationv1.RemediationPhase, childRefName string, globalTimeDelta, phaseTimeDelta time.Duration) *remediationv1.RemediationRequest {
	rr := newRemediationRequestWithPhaseTimeout(name, namespace, phase, childRefName, phaseTimeDelta)

	// Set global start time
	globalStartTime := metav1.NewTime(time.Now().Add(globalTimeDelta))
	rr.Status.StartTime = &globalStartTime

	return rr
}

// verifyChildrenExistence checks if expected child CRDs exist or don't exist
func verifyChildrenExistence(ctx context.Context, c client.Client, rr *remediationv1.RemediationRequest, expectedChildren map[string]bool) {
	namespace := rr.Namespace
	rrName := rr.Name

	if expectSP, ok := expectedChildren["SP"]; ok {
		var sp signalprocessingv1.SignalProcessing
		spName := "sp-" + rrName
		err := c.Get(ctx, client.ObjectKey{Name: spName, Namespace: namespace}, &sp)
		if expectSP {
			Expect(err).ToNot(HaveOccurred(), "Expected SignalProcessing to exist")
		} else {
			Expect(err).To(HaveOccurred(), "Expected SignalProcessing to not exist")
		}
	}

	if expectAI, ok := expectedChildren["AI"]; ok {
		var ai aianalysisv1.AIAnalysis
		aiName := "ai-" + rrName
		err := c.Get(ctx, client.ObjectKey{Name: aiName, Namespace: namespace}, &ai)
		if expectAI {
			Expect(err).ToNot(HaveOccurred(), "Expected AIAnalysis to exist")
		} else {
			Expect(err).To(HaveOccurred(), "Expected AIAnalysis to not exist")
		}
	}

	if expectWE, ok := expectedChildren["WE"]; ok {
		var we workflowexecutionv1.WorkflowExecution
		weName := "we-" + rrName
		err := c.Get(ctx, client.ObjectKey{Name: weName, Namespace: namespace}, &we)
		if expectWE {
			Expect(err).ToNot(HaveOccurred(), "Expected WorkflowExecution to exist")
		} else {
			Expect(err).To(HaveOccurred(), "Expected WorkflowExecution to not exist")
		}
	}
}
