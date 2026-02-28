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

// Package validators provides CRD status validation helpers for E2E and unit tests.
// See docs/testing/test-plans/FULLPIPELINE_E2E_STATUS_VALIDATION_TEST_PLAN.md
package validators

import (
	"fmt"
	"reflect"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type validationConfig struct {
	approvalFlow bool
}

// ValidationOption configures validator behavior.
type ValidationOption func(*validationConfig)

// WithApprovalFlow enables approval-flow-specific validation (extra fields for AA and RR).
func WithApprovalFlow() ValidationOption {
	return func(c *validationConfig) {
		c.approvalFlow = true
	}
}

func applyOpts(opts []ValidationOption) validationConfig {
	var cfg validationConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// --- Common helpers ---

func checkNonNil(field, impact string, val interface{}) string {
	if val == nil {
		return fmt.Sprintf("%s not populated -- %s", field, impact)
	}
	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return fmt.Sprintf("%s not populated -- %s", field, impact)
	}
	return ""
}

func checkNonEmpty(field, impact string, val string) string {
	if val != "" {
		return ""
	}
	return fmt.Sprintf("%s not populated -- %s", field, impact)
}

func checkTimeSet(field, impact string, t *metav1.Time) string {
	if t != nil && !t.Time.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s not populated -- %s", field, impact)
}

func checkNonZeroTime(field, impact string, t metav1.Time) string {
	if !t.Time.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s not populated -- %s", field, impact)
}

func checkConditions(field, impact string, c []metav1.Condition) string {
	if len(c) > 0 {
		return ""
	}
	return fmt.Sprintf("%s not populated -- %s", field, impact)
}

func appendIfNonEmpty(failures []string, msg string) []string {
	if msg != "" {
		return append(failures, msg)
	}
	return failures
}

// ValidateSPStatus validates SignalProcessing status fields (23 checks).
func ValidateSPStatus(sp *signalprocessingv1.SignalProcessing) []string {
	s := &sp.Status
	var f []string

	// Phase
	if string(s.Phase) != "Completed" {
		f = append(f, fmt.Sprintf("SP: Phase is %q -- downstream controllers will not process this signal", s.Phase))
	}
	f = appendIfNonEmpty(f, checkTimeSet("SP: StartTime", "audit trail has no start timestamp for signal processing", s.StartTime))
	f = appendIfNonEmpty(f, checkTimeSet("SP: CompletionTime", "SLA monitoring cannot compute processing duration", s.CompletionTime))
	if s.ObservedGeneration <= 0 {
		f = append(f, "SP: ObservedGeneration not set -- controller may not have reconciled")
	}

	// KubernetesContext (Issue #113: now uses sharedtypes.KubernetesContext)
	if s.KubernetesContext == nil {
		f = append(f, "SP: KubernetesContext not populated -- enrichment data unavailable for AI analysis")
	} else {
		f = appendIfNonEmpty(f, checkNonNil("SP: KubernetesContext.Namespace", "namespace context unavailable for environment classification", s.KubernetesContext.Namespace))
		if s.KubernetesContext.Workload == nil {
			f = append(f, "SP: KubernetesContext.Workload not populated -- target workload context missing for classification")
		} else {
			f = appendIfNonEmpty(f, checkNonEmpty("SP: KubernetesContext.Workload.Kind", "workload type unknown, Rego policies cannot classify", s.KubernetesContext.Workload.Kind))
			f = appendIfNonEmpty(f, checkNonEmpty("SP: KubernetesContext.Workload.Name", "workload name unknown, historical remediation lookup impossible", s.KubernetesContext.Workload.Name))
		}
	}

	// EnvironmentClassification
	if s.EnvironmentClassification == nil {
		f = append(f, "SP: EnvironmentClassification not populated -- approval policy cannot evaluate environment, workflow matching may fail")
	} else {
		f = appendIfNonEmpty(f, checkNonEmpty("SP: EnvironmentClassification.Environment", "approval policy cannot determine if production", s.EnvironmentClassification.Environment))
		f = appendIfNonEmpty(f, checkNonEmpty("SP: EnvironmentClassification.Source", "audit trail missing classification source", s.EnvironmentClassification.Source))
		f = appendIfNonEmpty(f, checkNonZeroTime("SP: EnvironmentClassification.ClassifiedAt", "audit trail missing classification timestamp", s.EnvironmentClassification.ClassifiedAt))
	}

	// PriorityAssignment
	if s.PriorityAssignment == nil {
		f = append(f, "SP: PriorityAssignment not populated -- priority-based routing unavailable")
	} else {
		f = appendIfNonEmpty(f, checkNonEmpty("SP: PriorityAssignment.Priority", "downstream consumers cannot prioritize remediation", s.PriorityAssignment.Priority))
		f = appendIfNonEmpty(f, checkNonEmpty("SP: PriorityAssignment.Source", "audit trail missing priority assignment source", s.PriorityAssignment.Source))
		f = appendIfNonEmpty(f, checkNonZeroTime("SP: PriorityAssignment.AssignedAt", "audit trail missing priority assignment timestamp", s.PriorityAssignment.AssignedAt))
	}

	f = appendIfNonEmpty(f, checkNonNil("SP: BusinessClassification", "business context unavailable for SLA routing", s.BusinessClassification))
	f = appendIfNonEmpty(f, checkNonEmpty("SP: Severity", "severity-based routing and notification escalation unavailable", s.Severity))
	f = appendIfNonEmpty(f, checkNonEmpty("SP: SignalMode", "reactive vs proactive classification missing", s.SignalMode))
	f = appendIfNonEmpty(f, checkNonEmpty("SP: SignalType", "signal type unknown, workflow matching may fail", s.SignalName))
	f = appendIfNonEmpty(f, checkNonEmpty("SP: PolicyHash", "policy versioning broken, cannot detect policy drift", s.PolicyHash))
	f = appendIfNonEmpty(f, checkConditions("SP: Conditions", "controller status conditions missing", s.Conditions))

	return f
}

// ValidateAAStatus validates AIAnalysis status fields (23 base + 9 approval = 32 with approval).
func ValidateAAStatus(aa *aianalysisv1.AIAnalysis, opts ...ValidationOption) []string {
	cfg := applyOpts(opts)
	s := &aa.Status
	var f []string

	if s.Phase != aianalysisv1.PhaseCompleted {
		f = append(f, fmt.Sprintf("AA: Phase is %q -- workflow execution will not proceed", s.Phase))
	}
	f = appendIfNonEmpty(f, checkTimeSet("AA: StartedAt", "audit trail has no start timestamp for AI analysis", s.StartedAt))
	f = appendIfNonEmpty(f, checkTimeSet("AA: CompletedAt", "SLA monitoring cannot compute analysis duration", s.CompletedAt))
	if s.ObservedGeneration <= 0 {
		f = append(f, "AA: ObservedGeneration not set -- controller may not have reconciled")
	}

	// RootCauseAnalysis
	if s.RootCauseAnalysis == nil {
		f = append(f, "AA: RootCauseAnalysis not populated -- no root cause available for workflow selection")
	} else {
		f = appendIfNonEmpty(f, checkNonEmpty("AA: RootCauseAnalysis.Summary", "operator has no RCA summary for audit or review", s.RootCauseAnalysis.Summary))
		f = appendIfNonEmpty(f, checkNonEmpty("AA: RootCauseAnalysis.Severity", "severity-based workflow selection unavailable", s.RootCauseAnalysis.Severity))
		f = appendIfNonEmpty(f, checkNonEmpty("AA: RootCauseAnalysis.SignalType", "RO cannot determine analyzed signal type for routing", s.RootCauseAnalysis.SignalType))
	}

	// SelectedWorkflow
	if s.SelectedWorkflow == nil {
		f = append(f, "AA: SelectedWorkflow not populated -- no workflow available for execution")
	} else {
		f = appendIfNonEmpty(f, checkNonEmpty("AA: SelectedWorkflow.WorkflowID", "WE cannot determine which workflow to execute", s.SelectedWorkflow.WorkflowID))
		f = appendIfNonEmpty(f, checkNonEmpty("AA: SelectedWorkflow.ExecutionEngine", "WE cannot determine execution engine (job/tekton)", s.SelectedWorkflow.ExecutionEngine))
		if s.SelectedWorkflow.Confidence <= 0 {
			f = append(f, "AA: SelectedWorkflow.Confidence not set -- approval policy cannot evaluate confidence threshold")
		}
		f = appendIfNonEmpty(f, checkNonEmpty("AA: SelectedWorkflow.Version", "audit trail missing workflow version", s.SelectedWorkflow.Version))
		f = appendIfNonEmpty(f, checkNonEmpty("AA: SelectedWorkflow.ExecutionBundle", "WE cannot pull OCI bundle for execution", s.SelectedWorkflow.ExecutionBundle))
		f = appendIfNonEmpty(f, checkNonEmpty("AA: SelectedWorkflow.Rationale", "operator has no rationale for workflow choice", s.SelectedWorkflow.Rationale))
	}

	f = appendIfNonEmpty(f, checkNonEmpty("AA: InvestigationID", "audit trail cannot correlate investigation with analysis", s.InvestigationID))
	if s.TotalAnalysisTime <= 0 {
		f = append(f, "AA: TotalAnalysisTime not set -- SLA monitoring cannot track analysis duration")
	}

	// InvestigationSession
	if s.InvestigationSession == nil {
		f = append(f, "AA: InvestigationSession not populated -- investigation tracking unavailable")
	} else {
		f = appendIfNonEmpty(f, checkNonEmpty("AA: InvestigationSession.ID", "session tracking broken, cannot resume investigation", s.InvestigationSession.ID))
	}

	// PostRCAContext
	if s.PostRCAContext == nil {
		f = append(f, "AA: PostRCAContext not populated -- detected labels unavailable for workflow parameters")
	} else {
		f = appendIfNonEmpty(f, checkNonNil("AA: PostRCAContext.DetectedLabels", "label-based workflow parameters unavailable", s.PostRCAContext.DetectedLabels))
		f = appendIfNonEmpty(f, checkTimeSet("AA: PostRCAContext.SetAt", "audit trail missing PostRCAContext timestamp", s.PostRCAContext.SetAt))
	}

	f = appendIfNonEmpty(f, checkConditions("AA: Conditions", "controller status conditions missing", s.Conditions))

	// Approval-specific fields
	if cfg.approvalFlow {
		if !s.ApprovalRequired {
			f = append(f, "AA: ApprovalRequired is false -- expected true for production approval flow")
		}
		f = appendIfNonEmpty(f, checkNonEmpty("AA: ApprovalReason", "operator has no reason why approval was required", s.ApprovalReason))
		if s.ApprovalContext == nil {
			f = append(f, "AA: ApprovalContext not populated -- operator has no context for approval decision")
		} else {
			f = appendIfNonEmpty(f, checkNonEmpty("AA: ApprovalContext.Reason", "operator has no structured reason for approval", s.ApprovalContext.Reason))
			f = appendIfNonEmpty(f, checkNonEmpty("AA: ApprovalContext.WhyApprovalRequired", "operator cannot understand why approval was triggered", s.ApprovalContext.WhyApprovalRequired))
			if s.ApprovalContext.ConfidenceScore <= 0 {
				f = append(f, "AA: ApprovalContext.ConfidenceScore not set -- operator cannot assess confidence level")
			}
			f = appendIfNonEmpty(f, checkNonEmpty("AA: ApprovalContext.ConfidenceLevel", "operator has no categorical confidence (low/medium/high)", s.ApprovalContext.ConfidenceLevel))
			f = appendIfNonEmpty(f, checkNonEmpty("AA: ApprovalContext.InvestigationSummary", "operator has no investigation summary for decision", s.ApprovalContext.InvestigationSummary))
			if len(s.ApprovalContext.RecommendedActions) == 0 {
				f = append(f, "AA: ApprovalContext.RecommendedActions empty -- operator has no action guidance")
			}
		}
	}

	return f
}

// ValidateRRStatus validates RemediationRequest status fields (15 base + 2 approval = 17 with approval).
func ValidateRRStatus(rr *remediationv1.RemediationRequest, opts ...ValidationOption) []string {
	cfg := applyOpts(opts)
	s := &rr.Status
	var f []string

	if string(s.OverallPhase) != "Completed" {
		f = append(f, fmt.Sprintf("RR: OverallPhase is %q -- pipeline has not completed", s.OverallPhase))
	}
	f = appendIfNonEmpty(f, checkTimeSet("RR: StartTime", "audit trail has no start timestamp for remediation", s.StartTime))
	f = appendIfNonEmpty(f, checkTimeSet("RR: CompletedAt", "SLA monitoring cannot compute total remediation time", s.CompletedAt))
	if s.ObservedGeneration <= 0 {
		f = append(f, "RR: ObservedGeneration not set -- controller may not have reconciled")
	}

	f = appendIfNonEmpty(f, checkNonNil("RR: SignalProcessingRef", "audit trail cannot link to signal processing", s.SignalProcessingRef))
	f = appendIfNonEmpty(f, checkNonNil("RR: AIAnalysisRef", "audit trail cannot link to AI analysis", s.AIAnalysisRef))
	f = appendIfNonEmpty(f, checkNonNil("RR: WorkflowExecutionRef", "audit trail cannot link to workflow execution", s.WorkflowExecutionRef))

	if cfg.approvalFlow {
		if len(s.NotificationRequestRefs) < 2 {
			f = append(f, fmt.Sprintf("RR: NotificationRequestRefs has %d refs, expected >= 2 for approval flow (approval + completion)", len(s.NotificationRequestRefs)))
		}
	} else {
		if len(s.NotificationRequestRefs) < 1 {
			f = append(f, fmt.Sprintf("RR: NotificationRequestRefs has %d refs, expected >= 1 (completion notification)", len(s.NotificationRequestRefs)))
		}
	}

	f = appendIfNonEmpty(f, checkNonNil("RR: EffectivenessAssessmentRef", "audit trail cannot link to effectiveness assessment", s.EffectivenessAssessmentRef))
	f = appendIfNonEmpty(f, checkNonEmpty("RR: Outcome", "remediation outcome unknown, audit trail incomplete", s.Outcome))

	if s.SelectedWorkflowRef == nil {
		f = append(f, "RR: SelectedWorkflowRef not populated -- audit trail missing which workflow was selected")
	} else {
		f = appendIfNonEmpty(f, checkNonEmpty("RR: SelectedWorkflowRef.WorkflowID", "audit trail missing workflow ID", s.SelectedWorkflowRef.WorkflowID))
		f = appendIfNonEmpty(f, checkNonEmpty("RR: SelectedWorkflowRef.Version", "audit trail missing workflow version", s.SelectedWorkflowRef.Version))
		f = appendIfNonEmpty(f, checkNonEmpty("RR: SelectedWorkflowRef.ExecutionBundle", "audit trail missing execution bundle reference", s.SelectedWorkflowRef.ExecutionBundle))
	}

	f = appendIfNonEmpty(f, checkConditions("RR: Conditions", "controller status conditions missing", s.Conditions))

	if cfg.approvalFlow {
		if !s.ApprovalNotificationSent {
			f = append(f, "RR: ApprovalNotificationSent is false -- expected true for approval flow")
		}
	}

	return f
}

// ValidateWEStatus validates WorkflowExecution status fields (9 checks).
func ValidateWEStatus(we *workflowexecutionv1.WorkflowExecution) []string {
	s := &we.Status
	var f []string

	if s.Phase != workflowexecutionv1.PhaseCompleted {
		f = append(f, fmt.Sprintf("WE: Phase is %q -- execution has not completed", s.Phase))
	}
	f = appendIfNonEmpty(f, checkTimeSet("WE: StartTime", "audit trail has no start timestamp for workflow execution", s.StartTime))
	f = appendIfNonEmpty(f, checkTimeSet("WE: CompletionTime", "SLA monitoring cannot compute execution duration", s.CompletionTime))
	if s.ObservedGeneration <= 0 {
		f = append(f, "WE: ObservedGeneration not set -- controller may not have reconciled")
	}
	f = appendIfNonEmpty(f, checkNonEmpty("WE: Duration", "execution duration not recorded for SLA tracking", s.Duration))
	f = appendIfNonEmpty(f, checkNonNil("WE: ExecutionRef", "no reference to underlying execution resource (Job/PipelineRun)", s.ExecutionRef))
	if s.ExecutionStatus == nil {
		f = append(f, "WE: ExecutionStatus not populated -- execution result unknown")
	} else {
		f = appendIfNonEmpty(f, checkNonEmpty("WE: ExecutionStatus.Status", "execution result status missing", s.ExecutionStatus.Status))
	}
	f = appendIfNonEmpty(f, checkConditions("WE: Conditions", "controller status conditions missing", s.Conditions))

	return f
}

// ValidateNTStatus validates NotificationRequest status fields (8 checks).
func ValidateNTStatus(nr *notificationv1.NotificationRequest) []string {
	s := &nr.Status
	var f []string

	terminalPhases := map[notificationv1.NotificationPhase]bool{
		notificationv1.NotificationPhaseSent:          true,
		notificationv1.NotificationPhasePartiallySent: true,
		notificationv1.NotificationPhaseFailed:        true,
	}
	if !terminalPhases[s.Phase] {
		f = append(f, fmt.Sprintf("NT: Phase is %q -- not a terminal state (expected Sent, PartiallySent, or Failed)", s.Phase))
	}
	f = appendIfNonEmpty(f, checkTimeSet("NT: QueuedAt", "notification lifecycle tracking has no queue timestamp", s.QueuedAt))
	f = appendIfNonEmpty(f, checkTimeSet("NT: ProcessingStartedAt", "notification processing start time missing", s.ProcessingStartedAt))
	f = appendIfNonEmpty(f, checkTimeSet("NT: CompletionTime", "notification delivery tracking has no completion time", s.CompletionTime))
	if s.ObservedGeneration <= 0 {
		f = append(f, "NT: ObservedGeneration not set -- controller may not have reconciled")
	}
	if s.TotalAttempts < 1 {
		f = append(f, "NT: TotalAttempts < 1 -- no delivery attempts recorded")
	}
	if len(s.DeliveryAttempts) < 1 {
		f = append(f, "NT: DeliveryAttempts empty -- no delivery attempt details recorded")
	}
	f = appendIfNonEmpty(f, checkConditions("NT: Conditions", "controller status conditions missing", s.Conditions))

	return f
}

// ValidateEAStatus validates EffectivenessAssessment status fields (8 checks).
func ValidateEAStatus(ea *eav1.EffectivenessAssessment) []string {
	s := &ea.Status
	var f []string

	if s.Phase != eav1.PhaseCompleted && s.Phase != eav1.PhaseFailed {
		f = append(f, fmt.Sprintf("EA: Phase is %q -- not a terminal state (expected Completed or Failed)", s.Phase))
	}
	f = appendIfNonEmpty(f, checkTimeSet("EA: CompletedAt", "effectiveness assessment completion time missing", s.CompletedAt))

	c := &s.Components
	if !c.HealthAssessed {
		f = append(f, "EA: Components.HealthAssessed is false -- health assessment did not complete")
	}
	if !c.HashComputed {
		f = append(f, "EA: Components.HashComputed is false -- spec hash comparison not performed")
	}
	f = appendIfNonEmpty(f, checkNonEmpty("EA: Components.PostRemediationSpecHash", "cannot compare pre/post remediation state", c.PostRemediationSpecHash))
	f = appendIfNonEmpty(f, checkNonEmpty("EA: Components.CurrentSpecHash", "cannot verify current resource state matches remediation", c.CurrentSpecHash))
	if c.HealthScore == nil {
		f = append(f, "EA: Components.HealthScore not populated -- remediation effectiveness unknown")
	}
	f = appendIfNonEmpty(f, checkConditions("EA: Conditions", "controller status conditions missing", s.Conditions))

	return f
}

// ValidateRARStatus validates RemediationApprovalRequest status fields (6 checks).
func ValidateRARStatus(rar *remediationv1.RemediationApprovalRequest) []string {
	s := &rar.Status
	var f []string

	if s.Decision != remediationv1.ApprovalDecisionApproved {
		f = append(f, fmt.Sprintf("RAR: Decision is %q -- expected Approved", s.Decision))
	}
	f = appendIfNonEmpty(f, checkNonEmpty("RAR: DecidedBy", "audit trail missing who approved the remediation", s.DecidedBy))
	f = appendIfNonEmpty(f, checkTimeSet("RAR: DecidedAt", "audit trail missing approval timestamp", s.DecidedAt))
	f = appendIfNonEmpty(f, checkTimeSet("RAR: CreatedAt", "audit trail missing RAR creation timestamp", s.CreatedAt))
	if s.Expired {
		f = append(f, "RAR: Expired is true -- approval should not be expired after approval")
	}
	f = appendIfNonEmpty(f, checkConditions("RAR: Conditions", "controller status conditions missing", s.Conditions))

	return f
}

// ValidateRARSpec validates RemediationApprovalRequest spec fields (13 checks).
func ValidateRARSpec(rar *remediationv1.RemediationApprovalRequest) []string {
	sp := &rar.Spec
	var f []string

	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: RemediationRequestRef.Name", "operator cannot identify which remediation requires approval", sp.RemediationRequestRef.Name))
	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: AIAnalysisRef.Name", "operator cannot review the AI analysis that triggered approval", sp.AIAnalysisRef.Name))
	if sp.Confidence <= 0 {
		f = append(f, "RAR Spec: Confidence not set -- operator cannot assess AI confidence level")
	}
	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: ConfidenceLevel", "operator has no categorical confidence (low/medium/high)", sp.ConfidenceLevel))
	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: Reason", "operator has no reason for why this remediation was proposed", sp.Reason))

	wf := &sp.RecommendedWorkflow
	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: RecommendedWorkflow.WorkflowID", "operator cannot identify the proposed workflow", wf.WorkflowID))
	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: RecommendedWorkflow.Version", "operator cannot verify workflow version", wf.Version))
	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: RecommendedWorkflow.ExecutionBundle", "operator cannot verify what will execute", wf.ExecutionBundle))
	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: RecommendedWorkflow.Rationale", "operator has no rationale for workflow selection", wf.Rationale))

	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: InvestigationSummary", "operator has no investigation context for decision", sp.InvestigationSummary))
	if len(sp.RecommendedActions) == 0 {
		f = append(f, "RAR Spec: RecommendedActions empty -- operator has no action guidance")
	}
	f = appendIfNonEmpty(f, checkNonEmpty("RAR Spec: WhyApprovalRequired", "operator cannot understand why approval was triggered", sp.WhyApprovalRequired))
	f = appendIfNonEmpty(f, checkNonZeroTime("RAR Spec: RequiredBy", "operator has no deadline for approval decision", sp.RequiredBy))

	return f
}
