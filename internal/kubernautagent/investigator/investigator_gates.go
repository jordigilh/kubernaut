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

package investigator

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func (inv *Investigator) sameKindValidationGate(
	ctx context.Context,
	result *katypes.InvestigationResult,
	signal katypes.SignalContext,
	history []llm.Message,
	tokens *TokenAccumulator,
	correlationID string,
	client llm.Client,
	modelName string,
	runtimeParams llm.RuntimeParams,
) *katypes.InvestigationResult {
	if signal.ResourceKind == "" || result.RemediationTarget.Kind == "" {
		return result
	}
	if !strings.EqualFold(result.RemediationTarget.Kind, signal.ResourceKind) {
		return result
	}

	inv.logger.Info("same-kind validation gate triggered: remediation_target.kind matches signal resource_kind",
		"target_kind", result.RemediationTarget.Kind,
		"signal_resource_kind", signal.ResourceKind,
		"correlation_id", correlationID)

	gateEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	gateEvent.EventAction = audit.ActionSameKindGate
	gateEvent.EventOutcome = audit.OutcomeSuccess
	gateEvent.Data["model"] = modelName
	gateEvent.Data["prompt_length"] = 0
	gateEvent.Data["prompt_preview"] = ""
	gateEvent.Data["signal_resource_kind"] = signal.ResourceKind
	gateEvent.Data["target_kind"] = result.RemediationTarget.Kind
	gateEvent.Data["target_name"] = result.RemediationTarget.Name
	audit.StoreBestEffort(ctx, inv.auditStore, gateEvent, inv.auditLog())

	correctionMsg := fmt.Sprintf(
		`Your remediation_target.kind is "%s", which is the same resource kind as the input signal. `+
			`Signals often propagate upward: workload-level issues manifest as conditions on parent resources `+
			`(e.g., pod memory leaks cause node DiskPressure, deployment misconfigurations appear as node conditions). `+
			`Please re-evaluate: is a child resource (Deployment, StatefulSet, DaemonSet, Pod) the actual root cause `+
			`whose configuration should be modified? If after re-evaluation you are confident the %s itself is the `+
			`correct remediation target, confirm by resubmitting with the same target and explain why in your `+
			`due_diligence.target_accuracy field.`,
		result.RemediationTarget.Kind,
		result.RemediationTarget.Kind,
	)

	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "user", Content: correctionMsg},
	)

	resp, err := llm.ChatWithParams(ctx, client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    submitOnlyTools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, runtimeParams)
	if err != nil {
		inv.logger.Error(err, "same-kind validation gate retry failed, keeping original result",
			"correlation_id", correlationID)
		return result
	}
	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	var retryContent string
	for _, tc := range resp.ToolCalls {
		if tc.Name == SubmitResultToolName {
			retryContent = tc.Arguments
			break
		}
	}
	if retryContent == "" && resp.Message.Content != "" {
		retryContent = resp.Message.Content
	}
	if retryContent == "" {
		inv.logger.Info("same-kind validation gate: no content in retry response, keeping original",
			"correlation_id", correlationID)
		return result
	}

	retryResult, parseErr := inv.resultParser.Parse(retryContent)
	if parseErr != nil {
		inv.logger.Error(parseErr, "same-kind validation gate: retry parse failed, keeping original",
			"correlation_id", correlationID)
		return result
	}

	if retryResult.RemediationTarget.Kind == "" && result.RemediationTarget.Kind != "" {
		inv.logger.Info("same-kind validation gate: retry lost remediation_target, keeping original",
			"original_target", result.RemediationTarget.Kind+"/"+result.RemediationTarget.Name,
			"correlation_id", correlationID)
		return result
	}

	inv.logger.Info("same-kind validation gate: accepted retry result",
		"original_target", result.RemediationTarget.Kind+"/"+result.RemediationTarget.Name,
		"retry_target", retryResult.RemediationTarget.Kind+"/"+retryResult.RemediationTarget.Name,
		"correlation_id", correlationID)
	return retryResult
}

func enrichFromCatalog(result *katypes.InvestigationResult, v *parser.Validator) {
	if result == nil || v == nil || result.WorkflowID == "" {
		return
	}
	meta, ok := v.GetWorkflowMeta(result.WorkflowID)
	if !ok {
		return
	}
	if result.ExecutionEngine == "" {
		result.ExecutionEngine = meta.ExecutionEngine
	}
	if result.ExecutionBundle == "" {
		result.ExecutionBundle = meta.ExecutionBundle
	}
	if result.ExecutionBundleDigest == "" {
		result.ExecutionBundleDigest = meta.ExecutionBundleDigest
	}
	if result.ServiceAccountName == "" {
		result.ServiceAccountName = meta.ServiceAccountName
	}
	result.WorkflowVersion = meta.Version
}

// CheckWorkflowTargetAlignment verifies that the selected workflow's component
// scope includes the RCA remediation target kind. Emits an audit event and
// appends a warning on mismatch. This is a WARNING-level gate: misalignment
// does not force human review because the LLM may have legitimate cross-type
// reasoning (Issue #934).
func CheckWorkflowTargetAlignment(ctx context.Context, result *katypes.InvestigationResult, v *parser.Validator, correlationID string, auditStore audit.AuditStore, logger logr.Logger) {
	if result == nil || v == nil || result.WorkflowID == "" {
		return
	}
	meta, ok := v.GetWorkflowMeta(result.WorkflowID)
	if !ok {
		return
	}

	aligned := meta.MatchesTargetKind(result.RemediationTarget.Kind)

	ev := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	ev.EventAction = audit.ActionWorkflowAlignmentGate
	if aligned {
		ev.EventOutcome = audit.OutcomeSuccess
	} else {
		ev.EventOutcome = audit.OutcomeFailure
	}
	ev.Data["model"] = ""
	ev.Data["prompt_length"] = 0
	ev.Data["prompt_preview"] = ""
	ev.Data["workflow_id"] = result.WorkflowID
	ev.Data["target_kind"] = result.RemediationTarget.Kind
	ev.Data["workflow_components"] = meta.Component
	ev.Data["aligned"] = aligned
	audit.StoreBestEffort(ctx, auditStore, ev, logger)

	if !aligned {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Workflow %q target kind %q is not in the workflow's component scope %v",
				result.WorkflowID, result.RemediationTarget.Kind, meta.Component))
	}
}
