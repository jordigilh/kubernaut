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

package executor

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// JobExecutor implements the Executor interface for Kubernetes Jobs.
// Used for single-step remediations that don't require Tekton pipeline machinery.
//
// Authority: BR-WE-014 (Kubernetes Job Execution Backend)
type JobExecutor struct {
	Client             client.Client
	ServiceAccountName string
}

// NewJobExecutor creates a new JobExecutor.
func NewJobExecutor(c client.Client, serviceAccountName string) *JobExecutor {
	if serviceAccountName == "" {
		serviceAccountName = DefaultServiceAccountName
	}
	return &JobExecutor{
		Client:             c,
		ServiceAccountName: serviceAccountName,
	}
}

// Engine returns "job".
func (j *JobExecutor) Engine() string {
	return "job"
}

// Create builds and creates a Kubernetes Job in the execution namespace.
// Returns the name of the created Job.
//
// The Job runs the container image from the workflow catalog with parameters
// injected as environment variables.
func (j *JobExecutor) Create(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) (string, error) {
	job := j.buildJob(wfe, namespace)

	if err := j.Client.Create(ctx, job); err != nil {
		return "", err // Preserve original error for IsAlreadyExists checks
	}
	return job.Name, nil
}

// GetStatus retrieves the current status of the Job and maps it to ExecutionResult.
// Returns nil result with nil error if the Job is not found.
//
// Job condition mapping:
//   - conditions[Complete]=True  → PhaseCompleted
//   - conditions[Failed]=True    → PhaseFailed
//   - no terminal condition      → PhaseRunning
func (j *JobExecutor) GetStatus(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) (*ExecutionResult, error) {
	if wfe.Status.ExecutionRef == nil {
		return nil, fmt.Errorf("no execution ref set on WFE %s/%s", wfe.Namespace, wfe.Name)
	}

	var job batchv1.Job
	if err := j.Client.Get(ctx, client.ObjectKey{
		Name:      wfe.Status.ExecutionRef.Name,
		Namespace: namespace,
	}, &job); err != nil {
		return nil, err
	}

	summary := j.buildStatusSummary(&job)

	// Check Job conditions for terminal states
	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == corev1.ConditionTrue {
				return &ExecutionResult{
					Phase:   workflowexecutionv1alpha1.PhaseCompleted,
					Reason:  string(condition.Type),
					Message: condition.Message,
					Summary: summary,
				}, nil
			}
		case batchv1.JobFailed:
			if condition.Status == corev1.ConditionTrue {
				return &ExecutionResult{
					Phase:   workflowexecutionv1alpha1.PhaseFailed,
					Reason:  condition.Reason,
					Message: condition.Message,
					Summary: summary,
				}, nil
			}
		}
	}

	// No terminal condition yet - Job is still running
	return &ExecutionResult{
		Phase:   workflowexecutionv1alpha1.PhaseRunning,
		Reason:  "Running",
		Message: fmt.Sprintf("Job running (%d/%d pods active)", job.Status.Active, pointerInt32(job.Spec.Completions, 1)),
		Summary: summary,
	}, nil
}

// Cleanup deletes the Job in the execution namespace.
// Returns nil if the Job doesn't exist (idempotent).
func (j *JobExecutor) Cleanup(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) error {
	jobName := ExecutionResourceName(wfe.Spec.TargetResource)

	// Use propagation policy to also delete pods
	propagation := metav1.DeletePropagationBackground
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
		},
	}

	if err := j.Client.Delete(ctx, job, &client.DeleteOptions{
		PropagationPolicy: &propagation,
	}); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil // Already gone
		}
		return fmt.Errorf("failed to delete Job %s/%s: %w", namespace, jobName, err)
	}
	return nil
}

// buildJob creates a Kubernetes Job from the WFE spec.
// Parameters are injected as environment variables.
func (j *JobExecutor) buildJob(wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) *batchv1.Job {
	// Convert workflow parameters to env vars
	envVars := j.buildEnvVars(wfe)

	// Use the same deterministic naming as TektonExecutor for resource locking
	jobName := ExecutionResourceName(wfe.Spec.TargetResource)

	// Job configuration
	var backoffLimit int32 = 0 // No retries - WFE handles retry logic via cooldown/backoff
	var ttlSeconds int32 = 600 // 10 minutes TTL after completion for debugging

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"kubernaut.ai/workflow-execution": wfe.Name,
				"kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
				"kubernaut.ai/target-resource":    sanitizeLabelValue(wfe.Spec.TargetResource),
				"kubernaut.ai/source-namespace":   wfe.Namespace,
				"kubernaut.ai/execution-engine":   "job",
			},
			Annotations: map[string]string{
				"kubernaut.ai/target-resource": wfe.Spec.TargetResource,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": wfe.Name,
						"kubernaut.ai/execution-engine":   "job",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: j.ServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:  "workflow",
							Image: wfe.Spec.WorkflowRef.ExecutionBundle,
							Env:   envVars,
						},
					},
				},
			},
		},
	}
}

// buildEnvVars converts workflow parameters to container environment variables.
// Also adds TARGET_RESOURCE for consistency with Tekton pipelines.
func (j *JobExecutor) buildEnvVars(wfe *workflowexecutionv1alpha1.WorkflowExecution) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "TARGET_RESOURCE",
			Value: wfe.Spec.TargetResource,
		},
	}

	for key, value := range wfe.Spec.Parameters {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	return envVars
}

// buildStatusSummary creates a lightweight status summary from a Job.
func (j *JobExecutor) buildStatusSummary(job *batchv1.Job) *workflowexecutionv1alpha1.ExecutionStatusSummary {
	summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
		Status:     "Unknown",
		TotalTasks: 1, // Jobs are always single-step
	}

	if job.Status.Succeeded > 0 {
		summary.Status = "True"
		summary.Reason = "Succeeded"
		summary.CompletedTasks = 1
	} else if job.Status.Failed > 0 {
		summary.Status = "False"
		summary.Reason = "Failed"
		summary.Message = fmt.Sprintf("%d pod(s) failed", job.Status.Failed)
	} else if job.Status.Active > 0 {
		summary.Status = "Unknown"
		summary.Reason = "Running"
		summary.Message = fmt.Sprintf("%d pod(s) active", job.Status.Active)
	}

	return summary
}

// pointerInt32 dereferences a *int32, returning defaultVal if nil.
func pointerInt32(p *int32, defaultVal int32) int32 {
	if p != nil {
		return *p
	}
	return defaultVal
}
