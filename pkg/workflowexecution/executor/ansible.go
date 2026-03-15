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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

	// Credential lifecycle for dependencies.secrets injection (BR-WE-015).
	// The executor dynamically creates AWX credential types per unique K8s Secret name,
	// then creates ephemeral credentials per WFE execution and cleans them up after completion.
	CreateCredentialType(ctx context.Context, name string, inputs, injectors map[string]interface{}) (int, error)
	FindCredentialTypeByName(ctx context.Context, name string) (int, error)
	CreateCredential(ctx context.Context, name string, credTypeID, orgID int, inputs map[string]string) (int, error)
	DeleteCredential(ctx context.Context, credentialID int) error
	LaunchJobTemplateWithCreds(ctx context.Context, templateID int, extraVars map[string]interface{}, credentialIDs []int) (int, error)
	GetJobTemplateCredentials(ctx context.Context, templateID int) ([]int, error)
}

// AWXJobStatus represents the status response from AWX GET /api/v2/jobs/{id}/
type AWXJobStatus struct {
	ID           int    `json:"id"`
	Status       string `json:"status"`
	Failed       bool   `json:"failed"`
	ResultStdout string `json:"result_stdout,omitempty"`
}

const (
	// AnnotationEphemeralCredentials stores comma-separated AWX credential IDs
	// created by injectDependencySecrets for cleanup after execution.
	AnnotationEphemeralCredentials = "kubernaut.ai/awx-ephemeral-credentials"

	credentialTypePrefix = "kubernaut-secret-"
	credentialPrefix     = "kubernaut-ephemeral-"
)

// AnsibleExecutor implements the Executor interface for AWX/AAP workflow execution.
// BR-WE-015: Launches AWX Job Templates and tracks execution status via the AWX REST API.
type AnsibleExecutor struct {
	AWXClient      AWXClient
	K8sClient      client.Client
	OrganizationID int
	Logger         logr.Logger
}

// NewAnsibleExecutor creates a new AnsibleExecutor with the given AWX client.
// k8sClient is used to read K8s Secrets (dependencies.secrets) and write WFE annotations.
// orgID is the AWX organization ID for ephemeral credential creation.
func NewAnsibleExecutor(awxClient AWXClient, k8sClient client.Client, orgID int, logger logr.Logger) *AnsibleExecutor {
	if orgID <= 0 {
		orgID = 1
	}
	return &AnsibleExecutor{
		AWXClient:      awxClient,
		K8sClient:      k8sClient,
		OrganizationID: orgID,
		Logger:         logger.WithName("ansible-executor"),
	}
}

// Engine returns "ansible".
func (a *AnsibleExecutor) Engine() string {
	return "ansible"
}

// Create launches an AWX Job Template from the WFE spec.
// DD-WE-006: When opts.Dependencies contains secrets, the executor reads them from
// Kubernetes, creates ephemeral AWX credentials, and attaches them to the job launch.
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
	if extraVars == nil {
		extraVars = make(map[string]interface{})
	}
	extraVars["WFE_NAME"] = wfe.Name
	extraVars["WFE_NAMESPACE"] = wfe.Namespace
	extraVars["RR_NAME"] = wfe.Spec.RemediationRequestRef.Name
	extraVars["RR_NAMESPACE"] = wfe.Spec.RemediationRequestRef.Namespace

	if err := a.injectDependencyConfigMaps(ctx, opts.Dependencies, namespace, extraVars); err != nil {
		return "", fmt.Errorf("inject dependency configmaps: %w", err)
	}

	credentialIDs, err := a.injectDependencySecrets(ctx, opts.Dependencies, namespace, wfe.Name)
	if err != nil {
		return "", fmt.Errorf("inject dependency secrets: %w", err)
	}

	if len(credentialIDs) > 0 {
		templateCreds, tcErr := a.AWXClient.GetJobTemplateCredentials(ctx, templateID)
		if tcErr != nil {
			a.Logger.Error(tcErr, "Failed to fetch template credentials, launching with ephemeral only",
				"templateID", templateID)
		} else {
			credentialIDs = mergeCredentialIDs(templateCreds, credentialIDs)
		}
	}

	a.Logger.Info("Launching AWX job",
		"templateID", templateID,
		"playbookPath", cfg.PlaybookPath,
		"wfe", wfe.Name,
		"totalCredentials", len(credentialIDs),
	)

	var jobID int
	if len(credentialIDs) > 0 {
		jobID, err = a.AWXClient.LaunchJobTemplateWithCreds(ctx, templateID, extraVars, credentialIDs)
	} else {
		jobID, err = a.AWXClient.LaunchJobTemplate(ctx, templateID, extraVars)
	}
	if err != nil {
		return "", fmt.Errorf("launch AWX job template %d: %w", templateID, err)
	}

	if len(credentialIDs) > 0 {
		if annotErr := a.storeCredentialAnnotation(ctx, wfe, credentialIDs); annotErr != nil {
			a.Logger.Error(annotErr, "Failed to store ephemeral credential IDs in WFE annotation",
				"wfe", wfe.Name, "credentialIDs", credentialIDs)
		}
	}

	return fmt.Sprintf("awx-job-%d", jobID), nil
}

// injectDependencySecrets reads K8s Secrets declared in dependencies, creates
// dynamic AWX credential types and ephemeral credentials, and returns the
// credential IDs to attach to the AWX job launch.
func (a *AnsibleExecutor) injectDependencySecrets(
	ctx context.Context,
	deps *models.WorkflowDependencies,
	namespace string,
	wfeName string,
) ([]int, error) {
	if deps == nil || len(deps.Secrets) == 0 {
		return nil, nil
	}

	var credentialIDs []int

	for _, dep := range deps.Secrets {
		var secret corev1.Secret
		if err := a.K8sClient.Get(ctx, client.ObjectKey{
			Name:      dep.Name,
			Namespace: namespace,
		}, &secret); err != nil {
			return nil, fmt.Errorf("read dependency secret %q in %q: %w", dep.Name, namespace, err)
		}

		credTypeID, err := a.ensureCredentialType(ctx, dep.Name, secret.Data)
		if err != nil {
			return nil, fmt.Errorf("ensure credential type for secret %q: %w", dep.Name, err)
		}

		inputs := make(map[string]string, len(secret.Data))
		for k, v := range secret.Data {
			inputs[k] = string(v)
		}

		credName := fmt.Sprintf("%s%s-%s", credentialPrefix, dep.Name, wfeName)
		credID, err := a.AWXClient.CreateCredential(ctx, credName, credTypeID, a.OrganizationID, inputs)
		if err != nil {
			return nil, fmt.Errorf("create ephemeral credential for secret %q: %w", dep.Name, err)
		}

		a.Logger.Info("Created ephemeral AWX credential",
			"secret", dep.Name, "credentialID", credID, "credentialType", credTypeID)
		credentialIDs = append(credentialIDs, credID)
	}

	return credentialIDs, nil
}

// injectDependencyConfigMaps reads K8s ConfigMaps declared in dependencies and
// merges their data into extra_vars with a KUBERNAUT_CONFIGMAP_{NAME}_{KEY} prefix.
// ConfigMaps are non-sensitive, so they use AWX extra_vars (not credentials).
func (a *AnsibleExecutor) injectDependencyConfigMaps(
	ctx context.Context,
	deps *models.WorkflowDependencies,
	namespace string,
	extraVars map[string]interface{},
) error {
	if deps == nil || len(deps.ConfigMaps) == 0 {
		return nil
	}

	for _, dep := range deps.ConfigMaps {
		var cm corev1.ConfigMap
		if err := a.K8sClient.Get(ctx, client.ObjectKey{
			Name:      dep.Name,
			Namespace: namespace,
		}, &cm); err != nil {
			return fmt.Errorf("read dependency configmap %q in %q: %w", dep.Name, namespace, err)
		}

		prefix := "KUBERNAUT_CONFIGMAP_" + sanitizeEnvSegment(dep.Name) + "_"
		for k, v := range cm.Data {
			extraVars[prefix+sanitizeEnvSegment(k)] = v
		}

		a.Logger.Info("Injected ConfigMap data into extra_vars",
			"configMap", dep.Name, "keyCount", len(cm.Data))
	}

	return nil
}

// ensureCredentialType finds or creates an AWX credential type for the given
// K8s Secret name. The credential type's env injector maps each secret key to
// KUBERNAUT_SECRET_{SECRET_NAME}_{KEY}.
func (a *AnsibleExecutor) ensureCredentialType(
	ctx context.Context,
	secretName string,
	secretData map[string][]byte,
) (int, error) {
	typeName := credentialTypePrefix + secretName

	id, err := a.AWXClient.FindCredentialTypeByName(ctx, typeName)
	if err == nil {
		return id, nil
	}

	envPrefix := "KUBERNAUT_SECRET_" + sanitizeEnvSegment(secretName) + "_"

	fields := make([]map[string]interface{}, 0, len(secretData))
	envMap := make(map[string]string, len(secretData))

	for key := range secretData {
		fields = append(fields, map[string]interface{}{
			"id":     key,
			"label":  key,
			"type":   "string",
			"secret": true,
		})
		envMap[envPrefix+sanitizeEnvSegment(key)] = "{{" + key + "}}"
	}

	inputs := map[string]interface{}{"fields": fields}
	injectors := map[string]interface{}{"env": envMap}

	id, err = a.AWXClient.CreateCredentialType(ctx, typeName, inputs, injectors)
	if err != nil {
		return 0, fmt.Errorf("create AWX credential type %q: %w", typeName, err)
	}

	a.Logger.Info("Created AWX credential type", "name", typeName, "id", id)
	return id, nil
}

// storeCredentialAnnotation writes the ephemeral credential IDs to the WFE's
// annotations so Cleanup() can delete them later.
func (a *AnsibleExecutor) storeCredentialAnnotation(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	credentialIDs []int,
) error {
	parts := make([]string, len(credentialIDs))
	for i, id := range credentialIDs {
		parts[i] = strconv.Itoa(id)
	}

	if wfe.Annotations == nil {
		wfe.Annotations = make(map[string]string)
	}
	wfe.Annotations[AnnotationEphemeralCredentials] = strings.Join(parts, ",")

	return a.K8sClient.Update(ctx, wfe)
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

// Cleanup deletes ephemeral AWX credentials (if any) and cancels the AWX job.
func (a *AnsibleExecutor) Cleanup(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
) error {
	a.cleanupEphemeralCredentials(ctx, wfe)

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

// cleanupEphemeralCredentials reads the annotation storing ephemeral AWX
// credential IDs and deletes each one. Errors are logged but do not fail
// the cleanup -- the AWX job cancellation must still proceed.
func (a *AnsibleExecutor) cleanupEphemeralCredentials(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
) {
	if wfe.Annotations == nil {
		return
	}

	annotation, ok := wfe.Annotations[AnnotationEphemeralCredentials]
	if !ok || annotation == "" {
		return
	}

	parts := strings.Split(annotation, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		credID, err := strconv.Atoi(part)
		if err != nil {
			a.Logger.Error(err, "Invalid credential ID in annotation", "value", part)
			continue
		}

		if err := a.AWXClient.DeleteCredential(ctx, credID); err != nil {
			a.Logger.Error(err, "Failed to delete ephemeral AWX credential", "credentialID", credID)
			continue
		}

		a.Logger.Info("Deleted ephemeral AWX credential", "credentialID", credID)
	}
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

// sanitizeEnvSegment converts a Kubernetes resource name segment into an
// environment-variable-safe uppercase format: hyphens become underscores.
func sanitizeEnvSegment(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, "-", "_"))
}

func parseAWXJobID(executionRefName string) (int, error) {
	const prefix = "awx-job-"
	if !strings.HasPrefix(executionRefName, prefix) {
		return 0, fmt.Errorf("execution ref %q does not have awx-job- prefix", executionRefName)
	}
	return strconv.Atoi(executionRefName[len(prefix):])
}

func mergeCredentialIDs(templateIDs, ephemeralIDs []int) []int {
	seen := make(map[int]struct{}, len(templateIDs)+len(ephemeralIDs))
	merged := make([]int, 0, len(templateIDs)+len(ephemeralIDs))
	for _, id := range templateIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			merged = append(merged, id)
		}
	}
	for _, id := range ephemeralIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			merged = append(merged, id)
		}
	}
	return merged
}
