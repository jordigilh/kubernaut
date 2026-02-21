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

package validators

import (
	"time"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	crdvalidators "github.com/jordigilh/kubernaut/test/shared/validators"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ============================================================================
// Helper functions: fully-populated CRD object builders
// ============================================================================

func completeSP() *signalprocessingv1.SignalProcessing {
	now := metav1.NewTime(time.Now())
	return &signalprocessingv1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{Name: "sp-test", Namespace: "default", Generation: 1},
		Status: signalprocessingv1.SignalProcessingStatus{
			Phase:             signalprocessingv1.PhaseCompleted,
			ObservedGeneration: 1,
			StartTime:         &now,
			CompletionTime:    &now,
			KubernetesContext: &sharedtypes.KubernetesContext{
				Namespace: &sharedtypes.NamespaceContext{Name: "default"},
				Workload:  &sharedtypes.WorkloadDetails{Kind: "Deployment", Name: "my-app"},
			},
			EnvironmentClassification: &signalprocessingv1.EnvironmentClassification{
				Environment:  "production",
				Source:       "namespace-labels",
				ClassifiedAt: now,
			},
			PriorityAssignment: &signalprocessingv1.PriorityAssignment{
				Priority:   "P0",
				Source:     "rego-policy",
				AssignedAt: now,
			},
			BusinessClassification: &sharedtypes.BusinessClassification{
				BusinessUnit:    "payments",
				ServiceOwner:    "platform-team",
				Criticality:     "critical",
				SLARequirement:  "platinum",
			},
			Severity:   "critical",
			SignalMode: "reactive",
			SignalType: "OOMKilled",
			PolicyHash: "a1b2c3d4e5f6",
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Completed"},
			},
		},
	}
}

func completeAA() *aianalysisv1.AIAnalysis {
	now := metav1.NewTime(time.Now())
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{Name: "aa-test", Namespace: "default", Generation: 1},
		Status: aianalysisv1.AIAnalysisStatus{
			Phase:             aianalysisv1.PhaseCompleted,
			ObservedGeneration: 1,
			StartedAt:         &now,
			CompletedAt:       &now,
			RootCauseAnalysis: &aianalysisv1.RootCauseAnalysis{
				Summary:            "Memory limit exceeded",
				Severity:           "critical",
				SignalType:         "OOMKilled",
				ContributingFactors: []string{"high memory usage"},
				AffectedResource:   &aianalysisv1.AffectedResource{Kind: "Deployment", Name: "my-app", Namespace: "default"},
			},
			SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
				WorkflowID:           "restart-pod",
				ActionType:          "RestartPod",
				Version:             "v1",
				ExecutionBundle:     "oci://registry/restart-pod:v1",
				ExecutionBundleDigest: "sha256:abc123",
				Confidence:          0.95,
				Parameters:          map[string]string{"NAMESPACE": "default"},
				Rationale:           "Restart will clear memory",
				ExecutionEngine:     "tekton",
			},
			InvestigationID:   "inv-abc-123",
			TotalAnalysisTime: 5000,
			InvestigationSession: &aianalysisv1.InvestigationSession{
				ID:        "sess-123",
				Generation: 0,
				LastPolled: &now,
				CreatedAt:  &now,
				PollCount:  1,
			},
			PostRCAContext: &aianalysisv1.PostRCAContext{
				DetectedLabels: &sharedtypes.DetectedLabels{},
				SetAt:          &now,
			},
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Completed"},
			},
		},
	}
}

func completeAAWithApproval() *aianalysisv1.AIAnalysis {
	aa := completeAA()
	aa.Status.ApprovalRequired = true
	aa.Status.ApprovalReason = "low confidence"
	aa.Status.ApprovalContext = &aianalysisv1.ApprovalContext{
		Reason:                "Confidence below threshold",
		ConfidenceScore:       0.65,
		ConfidenceLevel:       "medium",
		InvestigationSummary:  "Root cause identified",
		EvidenceCollected:     []string{"Memory metrics"},
		RecommendedActions:    []aianalysisv1.RecommendedAction{{Action: "Restart pod", Rationale: "Clear memory"}},
		AlternativesConsidered: []aianalysisv1.AlternativeApproach{{Approach: "Scale up", ProsCons: "Slower"}},
		WhyApprovalRequired:   "Policy requires manual review",
		PolicyEvaluation:     &aianalysisv1.PolicyEvaluation{PolicyName: "approval-policy", MatchedRules: []string{"low-confidence"}, Decision: "manual_review_required"},
	}
	return aa
}

func completeRR() *remediationv1.RemediationRequest {
	now := metav1.NewTime(time.Now())
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "rr-test", Namespace: "default", Generation: 1},
		Status: remediationv1.RemediationRequestStatus{
			ObservedGeneration:      1,
			OverallPhase:            remediationv1.PhaseCompleted,
			StartTime:               &now,
			CompletedAt:             &now,
			SignalProcessingRef:     &corev1.ObjectReference{Name: "sp-1", Kind: "SignalProcessing"},
			AIAnalysisRef:           &corev1.ObjectReference{Name: "aa-1", Kind: "AIAnalysis"},
			WorkflowExecutionRef:    &corev1.ObjectReference{Name: "we-1", Kind: "WorkflowExecution"},
			EffectivenessAssessmentRef: &corev1.ObjectReference{Name: "ea-1", Kind: "EffectivenessAssessment"},
			Outcome:                 "success",
			NotificationRequestRefs: []corev1.ObjectReference{
				{Name: "nr-1", Kind: "NotificationRequest"},
			},
			SelectedWorkflowRef: &remediationv1.WorkflowReference{
				WorkflowID:           "restart-pod",
				Version:              "v1",
				ExecutionBundle:      "oci://registry/restart-pod:v1",
				ExecutionBundleDigest: "sha256:abc123",
			},
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Completed"},
			},
		},
	}
}

func completeRRWithApproval() *remediationv1.RemediationRequest {
	rr := completeRR()
	rr.Status.NotificationRequestRefs = []corev1.ObjectReference{
		{Name: "nr-1", Kind: "NotificationRequest"},
		{Name: "nr-2", Kind: "NotificationRequest"},
	}
	rr.Status.ApprovalNotificationSent = true
	return rr
}

func completeWE() *workflowexecutionv1.WorkflowExecution {
	now := metav1.NewTime(time.Now())
	return &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "we-test", Namespace: "default", Generation: 1},
		Status: workflowexecutionv1.WorkflowExecutionStatus{
			Phase:             workflowexecutionv1.PhaseCompleted,
			ObservedGeneration: 1,
			StartTime:         &now,
			CompletionTime:    &now,
			Duration:          "2m30s",
			ExecutionRef:      &corev1.LocalObjectReference{Name: "pipelinerun-123"},
			ExecutionStatus: &workflowexecutionv1.ExecutionStatusSummary{
				Status:         "True",
				Reason:         "Succeeded",
				Message:        "Completed",
				CompletedTasks: 3,
				TotalTasks:     3,
			},
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Completed"},
			},
		},
	}
}

func completeNT() *notificationv1.NotificationRequest {
	now := metav1.NewTime(time.Now())
	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "nr-test", Namespace: "default", Generation: 1},
		Status: notificationv1.NotificationRequestStatus{
			Phase:              notificationv1.NotificationPhaseSent,
			ObservedGeneration: 1,
			QueuedAt:           &now,
			ProcessingStartedAt: &now,
			CompletionTime:     &now,
			DeliveryAttempts: []notificationv1.DeliveryAttempt{
				{Channel: "slack", Attempt: 1, Timestamp: now, Status: "success", DurationSeconds: 0.5},
			},
			TotalAttempts: 1,
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Sent"},
			},
		},
	}
}

func completeEA() *eav1.EffectivenessAssessment {
	now := metav1.NewTime(time.Now())
	healthScore := 0.95
	alertScore := 1.0
	metricsScore := 0.9
	return &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{Name: "ea-test", Namespace: "default", Generation: 1},
		Status: eav1.EffectivenessAssessmentStatus{
			Phase:       eav1.PhaseCompleted,
			CompletedAt: &now,
			Components: eav1.EAComponents{
				HealthAssessed:         true,
				HashComputed:           true,
				AlertAssessed:          true,
				MetricsAssessed:        true,
				HealthScore:            &healthScore,
				AlertScore:             &alertScore,
				MetricsScore:           &metricsScore,
				PostRemediationSpecHash: "hash123",
				CurrentSpecHash:         "hash123",
			},
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Completed"},
			},
		},
	}
}

func completeRAR() *remediationv1.RemediationApprovalRequest {
	now := metav1.NewTime(time.Now())
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "rar-test", Namespace: "default", Generation: 1},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-1", Kind: "RemediationRequest", Namespace: "default"},
			AIAnalysisRef:        remediationv1.ObjectRef{Name: "aa-1"},
			Confidence:           0.65,
			ConfidenceLevel:      "medium",
			Reason:               "Low confidence",
			RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
				WorkflowID:       "restart-pod",
				Version:         "v1",
				ExecutionBundle: "oci://registry/restart-pod:v1",
				Rationale:       "Restart recommended",
			},
			InvestigationSummary: "Root cause identified",
			RecommendedActions: []remediationv1.ApprovalRecommendedAction{
				{Action: "Approve", Rationale: "Safe to proceed"},
			},
			WhyApprovalRequired: "Policy requires review",
			RequiredBy:          now,
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			Decision:   remediationv1.ApprovalDecisionApproved,
			DecidedBy:  "operator@example.com",
			DecidedAt:  &now,
			CreatedAt:  &now,
			Expired:    false,
			Conditions: []metav1.Condition{
				{Type: "Approved", Status: metav1.ConditionTrue, Reason: "Approved"},
			},
		},
	}
}

// ============================================================================
// ValidateSPStatus tests
// ============================================================================

var _ = Describe("ValidateSPStatus", func() {
	It("returns no failures for a fully populated SignalProcessing", func() {
		sp := completeSP()
		failures := crdvalidators.ValidateSPStatus(sp)
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for an empty SignalProcessing", func() {
		sp := &signalprocessingv1.SignalProcessing{}
		failures := crdvalidators.ValidateSPStatus(sp)
		// 13 collapsed: nil parent structs (KubernetesContext, EnvironmentClassification, PriorityAssignment)
		// each produce 1 failure instead of individual sub-field failures
		Expect(failures).To(HaveLen(13))
	})
})

// ============================================================================
// ValidateAAStatus tests
// ============================================================================

var _ = Describe("ValidateAAStatus", func() {
	It("returns no failures for a fully populated AIAnalysis", func() {
		aa := completeAA()
		failures := crdvalidators.ValidateAAStatus(aa)
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for an empty AIAnalysis", func() {
		aa := &aianalysisv1.AIAnalysis{}
		failures := crdvalidators.ValidateAAStatus(aa)
		// 11 collapsed: nil parent structs collapse sub-field checks
		Expect(failures).To(HaveLen(11))
	})

	It("returns no failures for a fully populated AIAnalysis with WithApprovalFlow", func() {
		aa := completeAAWithApproval()
		failures := crdvalidators.ValidateAAStatus(aa, crdvalidators.WithApprovalFlow())
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for empty AIAnalysis with WithApprovalFlow", func() {
		aa := &aianalysisv1.AIAnalysis{}
		failures := crdvalidators.ValidateAAStatus(aa, crdvalidators.WithApprovalFlow())
		// 14 collapsed: 11 base + 3 approval (ApprovalRequired, ApprovalReason, ApprovalContext nil)
		Expect(failures).To(HaveLen(14))
	})
})

// ============================================================================
// ValidateRRStatus tests
// ============================================================================

var _ = Describe("ValidateRRStatus", func() {
	It("returns no failures for a fully populated RemediationRequest", func() {
		rr := completeRR()
		failures := crdvalidators.ValidateRRStatus(rr)
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for an empty RemediationRequest", func() {
		rr := &remediationv1.RemediationRequest{}
		failures := crdvalidators.ValidateRRStatus(rr)
		// 12 collapsed: SelectedWorkflowRef nil produces 1 instead of 4
		Expect(failures).To(HaveLen(12))
	})

	It("returns no failures for a fully populated RemediationRequest with WithApprovalFlow", func() {
		rr := completeRRWithApproval()
		failures := crdvalidators.ValidateRRStatus(rr, crdvalidators.WithApprovalFlow())
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for empty RemediationRequest with WithApprovalFlow", func() {
		rr := &remediationv1.RemediationRequest{}
		failures := crdvalidators.ValidateRRStatus(rr, crdvalidators.WithApprovalFlow())
		// 13 collapsed: 12 base (with NR refs < 2 instead of < 1) + ApprovalNotificationSent false
		Expect(failures).To(HaveLen(13))
	})
})

// ============================================================================
// ValidateWEStatus tests
// ============================================================================

var _ = Describe("ValidateWEStatus", func() {
	It("returns no failures for a fully populated WorkflowExecution", func() {
		we := completeWE()
		failures := crdvalidators.ValidateWEStatus(we)
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for an empty WorkflowExecution", func() {
		we := &workflowexecutionv1.WorkflowExecution{}
		failures := crdvalidators.ValidateWEStatus(we)
		// 8 collapsed: ExecutionStatus nil produces 1 instead of 2
		Expect(failures).To(HaveLen(8))
	})
})

// ============================================================================
// ValidateNTStatus tests
// ============================================================================

var _ = Describe("ValidateNTStatus", func() {
	It("returns no failures for a fully populated NotificationRequest", func() {
		nr := completeNT()
		failures := crdvalidators.ValidateNTStatus(nr)
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for an empty NotificationRequest", func() {
		nr := &notificationv1.NotificationRequest{}
		failures := crdvalidators.ValidateNTStatus(nr)
		Expect(failures).To(HaveLen(8))
	})
})

// ============================================================================
// ValidateEAStatus tests
// ============================================================================

var _ = Describe("ValidateEAStatus", func() {
	It("returns no failures for a fully populated EffectivenessAssessment", func() {
		ea := completeEA()
		failures := crdvalidators.ValidateEAStatus(ea)
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for an empty EffectivenessAssessment", func() {
		ea := &eav1.EffectivenessAssessment{}
		failures := crdvalidators.ValidateEAStatus(ea)
		Expect(failures).To(HaveLen(8))
	})
})

// ============================================================================
// ValidateRARStatus tests
// ============================================================================

var _ = Describe("ValidateRARStatus", func() {
	It("returns no failures for a fully populated RemediationApprovalRequest status", func() {
		rar := completeRAR()
		failures := crdvalidators.ValidateRARStatus(rar)
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for an empty RemediationApprovalRequest status", func() {
		rar := &remediationv1.RemediationApprovalRequest{}
		failures := crdvalidators.ValidateRARStatus(rar)
		// 5: Expired=false is the zero value and passes the check
		Expect(failures).To(HaveLen(5))
	})
})

// ============================================================================
// ValidateRARSpec tests
// ============================================================================

var _ = Describe("ValidateRARSpec", func() {
	It("returns no failures for a fully populated RemediationApprovalRequest spec", func() {
		rar := completeRAR()
		failures := crdvalidators.ValidateRARSpec(rar)
		Expect(failures).To(BeEmpty())
	})

	It("returns expected failure count for an empty RemediationApprovalRequest spec", func() {
		rar := &remediationv1.RemediationApprovalRequest{}
		failures := crdvalidators.ValidateRARSpec(rar)
		Expect(failures).To(HaveLen(13))
	})
})
