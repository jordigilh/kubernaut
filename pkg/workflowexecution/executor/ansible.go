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

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// AWXClient defines the interface for AWX/AAP REST API operations.
// Mocked in unit tests; real implementation provided by AWXHTTPClient.
type AWXClient interface {
	LaunchJobTemplate(ctx context.Context, templateID int, extraVars map[string]interface{}) (int, error)
	GetJobStatus(ctx context.Context, jobID int) (*AWXJobStatus, error)
	CancelJob(ctx context.Context, jobID int) error
	FindJobTemplateByName(ctx context.Context, name string) (int, error)
}

// AWXJobStatus represents the status response from AWX GET /api/v2/jobs/{id}/
type AWXJobStatus struct {
	ID           int    `json:"id"`
	Status       string `json:"status"`
	Failed       bool   `json:"failed"`
	ResultStdout string `json:"result_stdout,omitempty"`
}

// AnsibleExecutor implements the Executor interface for AWX/AAP workflow execution.
// BR-WE-015: Launches AWX Job Templates and tracks execution status via the AWX REST API.
type AnsibleExecutor struct {
	AWXClient AWXClient
	Logger    logr.Logger
}

// NewAnsibleExecutor creates a new AnsibleExecutor with the given AWX client.
func NewAnsibleExecutor(awxClient AWXClient, logger logr.Logger) *AnsibleExecutor {
	return &AnsibleExecutor{
		AWXClient: awxClient,
		Logger:    logger.WithName("ansible-executor"),
	}
}

// Engine returns "ansible".
func (a *AnsibleExecutor) Engine() string {
	return "ansible"
}

// Create launches an AWX Job Template from the WFE spec.
func (a *AnsibleExecutor) Create(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
	opts CreateOptions,
) (string, error) {
	cfg, err := a.parseEngineConfig(wfe)
	if err != nil {
		return "", fmt.Errorf("parse ansible engineConfig: %w", err)
	}

	templateID, err := a.resolveJobTemplate(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("resolve AWX job template: %w", err)
	}

	extraVars := BuildExtraVars(wfe.Spec.Parameters)

	a.Logger.Info("Launching AWX job",
		"templateID", templateID,
		"playbookPath", cfg.PlaybookPath,
		"wfe", wfe.Name,
	)

	jobID, err := a.AWXClient.LaunchJobTemplate(ctx, templateID, extraVars)
	if err != nil {
		return "", fmt.Errorf("launch AWX job template %d: %w", templateID, err)
	}

	return fmt.Sprintf("awx-job-%d", jobID), nil
}

// GetStatus polls AWX for the job status and maps it to an ExecutionResult.
func (a *AnsibleExecutor) GetStatus(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
) (*ExecutionResult, error) {
	if wfe.Status.ExecutionRef == nil {
		return nil, fmt.Errorf("no execution ref set on WFE %s/%s", wfe.Namespace, wfe.Name)
	}

	jobID, err := parseAWXJobID(wfe.Status.ExecutionRef.Name)
	if err != nil {
		return nil, fmt.Errorf("parse AWX job ID from executionRef %q: %w", wfe.Status.ExecutionRef.Name, err)
	}

	status, err := a.AWXClient.GetJobStatus(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get AWX job %d status: %w", jobID, err)
	}

	return MapAWXStatusToResult(status), nil
}

// Cleanup cancels the AWX job if it's still running.
func (a *AnsibleExecutor) Cleanup(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
) error {
	if wfe.Status.ExecutionRef == nil {
		return nil
	}

	jobID, err := parseAWXJobID(wfe.Status.ExecutionRef.Name)
	if err != nil {
		a.Logger.Info("Cannot parse AWX job ID for cleanup, skipping", "executionRef", wfe.Status.ExecutionRef.Name)
		return nil
	}

	if err := a.AWXClient.CancelJob(ctx, jobID); err != nil {
		if strings.Contains(err.Error(), "405") {
			a.Logger.Info("AWX job already completed, cancel not needed", "jobID", jobID)
			return nil
		}
		a.Logger.Error(err, "Failed to cancel AWX job during cleanup", "jobID", jobID)
		return fmt.Errorf("cancel AWX job %d: %w", jobID, err)
	}

	return nil
}

func (a *AnsibleExecutor) parseEngineConfig(wfe *workflowexecutionv1alpha1.WorkflowExecution) (*models.AnsibleEngineConfig, error) {
	if wfe.Spec.WorkflowRef.EngineConfig == nil {
		return nil, fmt.Errorf("engineConfig is required for ansible engine")
	}

	parsed, err := models.ParseEngineConfig("ansible", wfe.Spec.WorkflowRef.EngineConfig.Raw)
	if err != nil {
		return nil, err
	}

	cfg, ok := parsed.(*models.AnsibleEngineConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected engineConfig type: %T", parsed)
	}

	return cfg, nil
}

func (a *AnsibleExecutor) resolveJobTemplate(ctx context.Context, cfg *models.AnsibleEngineConfig) (int, error) {
	if cfg.JobTemplateName == "" {
		return 0, fmt.Errorf("jobTemplateName is required in ansible engineConfig for v1.0")
	}
	return a.AWXClient.FindJobTemplateByName(ctx, cfg.JobTemplateName)
}

// BuildExtraVars converts workflow parameters (map[string]string) to typed JSON values
// for AWX extra_vars. Attempts type coercion: integers, booleans, floats, JSON arrays/objects
// are converted to their native types. Plain strings remain strings.
func BuildExtraVars(params map[string]string) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}

	extraVars := make(map[string]interface{}, len(params))
	for k, v := range params {
		extraVars[k] = coerceValue(v)
	}
	return extraVars
}

func coerceValue(s string) interface{} {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	if (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) ||
		(strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) {
		var parsed interface{}
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			return parsed
		}
	}
	return s
}

// MapAWXStatusToResult maps an AWX job status to an ExecutionResult.
// AWX states: pending, waiting, running, successful, failed, error, canceled
func MapAWXStatusToResult(status *AWXJobStatus) *ExecutionResult {
	phase, reason, message := mapAWXStatusToPhase(status.Status)

	if status.ResultStdout != "" && phase == workflowexecutionv1alpha1.PhaseFailed {
		message = status.ResultStdout
	}

	summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
		Status:  phase,
		Reason:  reason,
		Message: message,
	}

	return &ExecutionResult{
		Phase:   phase,
		Reason:  reason,
		Message: message,
		Summary: summary,
	}
}

func mapAWXStatusToPhase(awxStatus string) (phase, reason, message string) {
	switch awxStatus {
	case "pending", "waiting":
		return workflowexecutionv1alpha1.PhasePending, "AWXJobPending", "AWX job is queued"
	case "running":
		return workflowexecutionv1alpha1.PhaseRunning, "AWXJobRunning", "AWX job is executing"
	case "successful":
		return workflowexecutionv1alpha1.PhaseCompleted, "AWXJobSuccessful", "AWX job completed successfully"
	case "failed":
		return workflowexecutionv1alpha1.PhaseFailed, "AWXJobFailed", "AWX job execution failed"
	case "error":
		return workflowexecutionv1alpha1.PhaseFailed, "AWXJobError", "AWX job encountered an internal error"
	case "canceled":
		return workflowexecutionv1alpha1.PhaseFailed, "AWXJobCanceled", "AWX job was canceled"
	default:
		return workflowexecutionv1alpha1.PhasePending, "AWXJobUnknown", fmt.Sprintf("Unknown AWX job status: %s", awxStatus)
	}
}

func parseAWXJobID(executionRefName string) (int, error) {
	const prefix = "awx-job-"
	if !strings.HasPrefix(executionRefName, prefix) {
		return 0, fmt.Errorf("execution ref %q does not have awx-job- prefix", executionRefName)
	}
	return strconv.Atoi(executionRefName[len(prefix):])
}
