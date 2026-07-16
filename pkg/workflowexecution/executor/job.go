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
	"path/filepath"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

const (
	SecretMountBasePath    = "/run/kubernaut/secrets"
	ConfigMapMountBasePath = "/run/kubernaut/configmaps"

	// defaultJobActiveDeadline is the fallback ActiveDeadlineSeconds applied to
	// the Job (BR-WORKFLOW-008) when the WFE does not declare an explicit
	// ExecutionConfig.Timeout. Matches the 30-minute default already used for
	// Tekton executions and RemediationOrchestrator's Executing-phase safety
	// net (BR-ORCH-028), so a Pod unable to mount a missing dependency reaches
	// a terminal JobFailed condition instead of hanging indefinitely.
	defaultJobActiveDeadline = 30 * time.Minute
)

// podMountFailureReasons are the kubelet Event reasons that indicate a Pod
// could not start because a referenced Secret/ConfigMap dependency is missing
// or misconfigured (BR-WORKFLOW-008). Used by GetStatus to enrich the generic
// Job condition message with the specific missing resource.
var podMountFailureReasons = map[string]bool{
	"FailedMount":                true,
	"CreateContainerConfigError": true,
}

// imagePullFailureReasons are the kubelet Event reasons that MAY indicate a
// container image could not be pulled (Issue #1645, BR-WORKFLOW-008),
// mirroring the Tekton engine's existing ImagePullBackOff message
// classification (internal/controller/workflowexecution/failure_analysis.go).
//
// Unlike podMountFailureReasons, a reason match alone is not sufficient:
// kubelet reuses the generic "Failed" and "BackOff" reasons for many
// unrelated container lifecycle events (e.g. liveness probe failures,
// CrashLoopBackOff), so isImagePullFailureEvent also checks the message text.
var imagePullFailureReasons = map[string]bool{
	"Failed":  true,
	"BackOff": true,
}

// isImagePullFailureEvent reports whether evt is a kubelet Event describing
// a container image-pull failure, as opposed to some other cause of the
// generic "Failed"/"BackOff" reason. Real kubelet image-pull events always
// mention "pull image" in the detailed message (e.g. `Failed to pull image
// "x": rpc error: ...`) or, for the terser retry-summary variant, "Error:
// ErrImagePull" / "Error: ImagePullBackOff" (see
// k8s.io/kubernetes/pkg/kubelet/images).
func isImagePullFailureEvent(evt corev1.Event) bool {
	if !imagePullFailureReasons[evt.Reason] {
		return false
	}
	messageLower := strings.ToLower(evt.Message)
	return strings.Contains(messageLower, "pull image") ||
		strings.Contains(messageLower, "errimagepull") ||
		strings.Contains(messageLower, "imagepullbackoff")
}

// JobExecutor implements the Executor interface for Kubernetes Jobs.
// DD-WE-005 v2.0: SA is read from WFE spec at execution time, not from executor config.
//
// Authority: BR-WE-014 (Kubernetes Job Execution Backend)
// BR-FLEET-054: Uses ClientFactory for local/remote client routing.
type JobExecutor struct {
	factory ClientFactory
}

// NewJobExecutor creates a new JobExecutor using the given client for local
// (hub cluster) execution only. For fleet-enabled deployments that need remote
// execution, use NewJobExecutorWithFactory instead.
func NewJobExecutor(c client.Client) *JobExecutor {
	return &JobExecutor{factory: NewLocalClientFactory(c)}
}

// NewJobExecutorWithFactory creates a JobExecutor that routes K8s API calls
// through the given ClientFactory, enabling remote cluster execution via MCP.
func NewJobExecutorWithFactory(f ClientFactory) *JobExecutor {
	return &JobExecutor{factory: f}
}

// Engine returns "job".
func (j *JobExecutor) Engine() string {
	return "job"
}

// Create builds and creates a Kubernetes Job in the execution namespace.
// Returns the name of the created Job.
//
// The Job runs the container image from the workflow catalog with parameters
// injected as environment variables. DD-WE-006: opts.Dependencies are mounted
// as volumes at /run/kubernaut/secrets/<name> and /run/kubernaut/configmaps/<name>.
func (j *JobExecutor) Create(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string, opts CreateOptions) (*CreateResult, error) {
	c, err := j.factory.ClientFor(ctx, wfe.Spec.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %q: %w", wfe.Spec.ClusterID, err)
	}

	job := j.buildJob(ctx, wfe, namespace, opts)

	if err := c.Create(ctx, job); err != nil {
		return nil, err // Preserve original error for IsAlreadyExists checks
	}
	return &CreateResult{ResourceName: job.Name}, nil
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

	c, err := j.factory.ClientFor(ctx, wfe.Spec.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("get client for cluster %q: %w", wfe.Spec.ClusterID, err)
	}

	// BR-FLEET-054: the MCP remote client requires an explicit GVK on Get
	// (it has no scheme/RESTMapper to infer one from the Go type, unlike the
	// local controller-runtime client). Setting it is a no-op for the local
	// path.
	var job batchv1.Job
	job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))
	if err := c.Get(ctx, client.ObjectKey{
		Name:      wfe.Status.ExecutionRef.Name,
		Namespace: namespace,
	}, &job); err != nil {
		return nil, err
	}

	summary := j.buildStatusSummary(ctx, c, &job)

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
				message := condition.Message
				if enriched := j.enrichFailureMessage(ctx, c, &job); enriched != "" {
					message = enriched
				}
				// BR-WORKFLOW-008: MarkFailed persists FailureDetails.Message from
				// summary.Message (not this result's top-level Message), so the
				// enrichment must also be reflected there or the specific missing
				// dependency detail never reaches the WFE status / K8s Event.
				summary.Message = message
				return &ExecutionResult{
					Phase:   workflowexecutionv1alpha1.PhaseFailed,
					Reason:  condition.Reason,
					Message: message,
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

// jobPodNameSuffixLength is the length of the random suffix Kubernetes
// appends to a GenerateName-created Pod's name (k8s.io/apiserver's
// names.SimpleNameGenerator), e.g. "<job-name>-x7k2m". Used to attribute an
// Event to this Job's Pod(s) by name prefix without requiring the Pod object
// to still exist (see enrichFailureMessage).
const jobPodNameSuffixLength = 5

// enrichFailureMessage inspects Events involving the failed Job's Pod(s) for
// a FailedMount/CreateContainerConfigError reason (missing Secret/ConfigMap
// dependency, #1481) or an image-pull failure (#1645) and, if found, returns
// its message in place of the generic Job condition message
// (BR-WORKFLOW-008). Since dependency existence is no longer pre-flight
// validated (#1481) and, as of #1642, neither is execution.bundle image
// existence, this is the only place either specific failure detail surfaces.
//
// Deliberately does NOT list the Job's Pods first: Kubernetes' job-controller
// deletes a Job's active Pods as soon as ActiveDeadlineSeconds is exceeded —
// before the next reconcile typically observes the JobFailed condition — so
// by the time this runs the Pod is usually already gone and only its Events
// (independent TTL, ~1h default) remain. Instead, Events are attributed to
// this Job by matching the Pod-name-prefix convention ("<job-name>-<5-char
// suffix>") that both the real job-controller and buildJob's Pod template use.
//
// Returns "" (caller keeps the original message) if no matching Event or a
// list error is encountered — this is best-effort enrichment, never a hard
// failure of status reporting.
func (j *JobExecutor) enrichFailureMessage(ctx context.Context, c ExecutorClient, job *batchv1.Job) string {
	logger := log.FromContext(ctx).WithValues("job", job.Name, "namespace", job.Namespace)

	var events corev1.EventList
	if err := c.List(ctx, &events, client.InNamespace(job.Namespace)); err != nil {
		logger.V(1).Info("failed to list events for job failure enrichment", "error", err)
		return ""
	}

	podNamePrefix := job.Name + "-"
	for _, evt := range events.Items {
		if evt.InvolvedObject.Kind != "Pod" || !isJobPodName(evt.InvolvedObject.Name, podNamePrefix) {
			continue
		}
		if podMountFailureReasons[evt.Reason] || isImagePullFailureEvent(evt) {
			return evt.Message
		}
	}
	return ""
}

// isJobPodName reports whether podName was generated for a Pod owned by the
// Job whose name produces the given "<job-name>-" prefix.
func isJobPodName(podName, jobNamePrefix string) bool {
	return len(podName) == len(jobNamePrefix)+jobPodNameSuffixLength &&
		podName[:len(jobNamePrefix)] == jobNamePrefix
}

// successfulCreateReason is the Kubernetes job-controller's Event Reason
// emitted on the Job object each time it creates a Pod -- the initial
// attempt and every PodFailurePolicy-Ignore replacement alike. This is a
// Kubernetes-internal implementation detail, not a versioned API contract;
// see countPodCreationAttempts's doc comment for the defensive fallback if
// it is ever renamed upstream.
const successfulCreateReason = "SuccessfulCreate"

// countPodCreationAttempts returns the number of AC4-tolerated pod-failure
// attempts observed before the Job reached its current state (BR-WE-019
// AC10 / DD-WE-008 Wiring Point C), computed as (total Pods the job-
// controller has ever created for this Job) - 1.
//
// This is deliberately NOT job.Status.Failed: k8s.io/api batch/v1's
// PodFailurePolicyActionIgnore doc comment states the counter towards
// .backoffLimit -- job.Status.Failed itself -- "is not incremented" for
// Ignore-action failures, which a real-cluster spike (DD-WE-008 Section 8)
// confirmed empirically: job.Status.Failed stayed 0 across multiple
// verified exit-137 pod failures. There is also no reliable per-Pod signal
// to fall back on: like enrichFailureMessage above, this deliberately does
// NOT list Pods, since the job-controller's ActiveDeadlineSeconds handling
// (and ordinary replacement-pod churn) typically removes them before this
// runs. Events on the Job object outlive individual Pods (independent TTL,
// ~1h default, comfortably inside this Job's 30m ActiveDeadlineSeconds
// ceiling) and reliably reflect Ignore-tolerated creations. Confirmed
// empirically: a real-cluster spike found one "SuccessfulCreate" Event per
// Pod created (Count=1 each, well under the ~10-events/10min aggregation
// threshold), persisting past the Job's transition to a terminal condition.
//
// Best-effort, not a mathematical guarantee (same scope boundary as
// enrichFailureMessage/per-attempt root-cause attribution, SOC2 CC8.1/AU-3):
// depends on a cluster-operator-controlled kube-apiserver --event-ttl
// outliving this Job's ActiveDeadlineSeconds, and on "SuccessfulCreate"
// remaining stable (it is Kubernetes-internal, not a versioned API). Logs a
// warning -- rather than silently returning an understated count -- when a
// Job that clearly ran (Succeeded or Failed) has zero matching Events, so a
// future regression is observable.
func (j *JobExecutor) countPodCreationAttempts(ctx context.Context, c ExecutorClient, job *batchv1.Job) int32 {
	logger := log.FromContext(ctx).WithValues("job", job.Name, "namespace", job.Namespace)

	var events corev1.EventList
	if err := c.List(ctx, &events, client.InNamespace(job.Namespace)); err != nil {
		logger.V(1).Info("failed to list events for retry-count computation (BR-WE-019 AC10)", "error", err)
		return 0
	}

	var totalCreations int32
	for _, evt := range events.Items {
		if evt.InvolvedObject.Kind != "Job" || evt.InvolvedObject.Name != job.Name || evt.Reason != successfulCreateReason {
			continue
		}
		count := evt.Count
		if count == 0 {
			count = 1
		}
		totalCreations += count
	}

	if totalCreations == 0 {
		if job.Status.Succeeded > 0 || job.Status.Failed > 0 {
			logger.Info("no SuccessfulCreate events found for a Job that reached a terminal outcome; retry count may be understated (BR-WE-019 AC10 best-effort boundary)")
		}
		return 0
	}
	return totalCreations - 1
}

// activeDeadlineSecondsFor resolves the Job's ActiveDeadlineSeconds
// (BR-WORKFLOW-008) from the WFE's ExecutionConfig.Timeout, falling back to
// defaultJobActiveDeadline when unset. This bounds how long a Pod can remain
// unable to start (e.g. stuck mounting a missing dependency) before the Job
// reaches a terminal JobFailed condition.
func activeDeadlineSecondsFor(wfe *workflowexecutionv1alpha1.WorkflowExecution) int64 {
	timeout := defaultJobActiveDeadline
	if wfe.Spec.ExecutionConfig != nil && wfe.Spec.ExecutionConfig.Timeout != nil && wfe.Spec.ExecutionConfig.Timeout.Duration > 0 {
		timeout = wfe.Spec.ExecutionConfig.Timeout.Duration
	}
	return int64(timeout.Seconds())
}

// IsCompleted checks whether the existing Job for the given target resource is
// in a terminal state (Succeeded or Failed). Used by the controller to determine
// if a stale completed Job can be cleaned up before retrying creation (Issue #374).
//
// Returns (true, nil) if the Job has a terminal condition (JobComplete or JobFailed).
// Returns (false, nil) if the Job is still running (no terminal condition).
// Returns (false, err) if the Job cannot be fetched (e.g., NotFound race).
func (j *JobExecutor) IsCompleted(ctx context.Context, clusterID string, targetResource string, namespace string) (bool, error) {
	c, err := j.factory.ClientFor(ctx, clusterID)
	if err != nil {
		return false, fmt.Errorf("get client for cluster %q: %w", clusterID, err)
	}

	jobName := ExecutionResourceName(targetResource)
	var job batchv1.Job
	job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))
	if err := c.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, &job); err != nil {
		return false, err
	}

	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		if condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed {
			return true, nil
		}
	}
	return false, nil
}

// Cleanup deletes the Job in the execution namespace.
// Returns nil if the Job doesn't exist (idempotent).
//
// Issue #383: Before deleting, verify the Job's kubernaut.ai/workflow-execution
// label matches this WFE's name. Because the Job name is deterministic (derived
// from TargetResource), a newer WFE for the same target may have already
// replaced the Job. Deleting without this check would destroy the new WFE's Job.
func (j *JobExecutor) Cleanup(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) error {
	c, err := j.factory.ClientFor(ctx, wfe.Spec.ClusterID)
	if err != nil {
		return fmt.Errorf("get client for cluster %q: %w", wfe.Spec.ClusterID, err)
	}

	jobName := ExecutionResourceName(wfe.Spec.TargetResource)

	var existing batchv1.Job
	existing.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))
	if err := c.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, &existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil // Already gone
		}
		return fmt.Errorf("failed to get Job %s/%s for ownership check: %w", namespace, jobName, err)
	}

	if owner := existing.Labels["kubernaut.ai/workflow-execution"]; owner != wfe.Name {
		return nil // Job belongs to a different WFE; leave it alone
	}

	propagation := metav1.DeletePropagationBackground
	if err := c.Delete(ctx, &existing, &client.DeleteOptions{
		PropagationPolicy: &propagation,
	}); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		}
		return fmt.Errorf("failed to delete Job %s/%s: %w", namespace, jobName, err)
	}
	return nil
}

// buildJob creates a Kubernetes Job from the WFE spec.
// Parameters are injected as environment variables.
// DD-WE-006: deps are mounted as volumes when non-nil.
// #243: Parameters are filtered against DeclaredParameterNames before injection.
// BR-WE-018: pod and container are hardened to the restricted SecurityContext
// profile, with a /tmp scratch volume so tools needing writable space (e.g.
// kubectl's discovery cache) still function under readOnlyRootFilesystem.
func (j *JobExecutor) buildJob(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string, opts CreateOptions) *batchv1.Job {
	logger := log.FromContext(ctx).WithValues("wfe", wfe.Name, "workflowID", wfe.Spec.WorkflowRef.WorkflowID)
	params := FilterDeclaredParameters(wfe.Spec.Parameters, opts.DeclaredParameterNames, logger)
	envVars := buildEnvVars(wfe.Spec.TargetResource, params)
	envVars = append(envVars, scratchSpaceEnvVars()...)
	volumes, mounts := buildDependencyVolumes(opts.Dependencies)
	volumes = append(volumes, scratchSpaceVolume())
	mounts = append(mounts, scratchSpaceVolumeMount())

	jobName := ExecutionResourceName(wfe.Spec.TargetResource)

	var backoffLimit int32 = 0
	var ttlSeconds int32 = 600
	activeDeadlineSeconds := activeDeadlineSecondsFor(wfe)

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
			ActiveDeadlineSeconds:   &activeDeadlineSeconds,
			PodFailurePolicy:        jobPodFailurePolicy(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": wfe.Name,
						"kubernaut.ai/execution-engine":   "job",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: wfe.Status.ServiceAccountName,
					SecurityContext:    restrictedPodSecurityContext(),
					Volumes:            volumes,
					Containers: []corev1.Container{
						{
							Name:            "workflow",
							Image:           wfe.Spec.WorkflowRef.ExecutionBundle,
							Env:             envVars,
							VolumeMounts:    mounts,
							SecurityContext: restrictedContainerSecurityContext(),
							Resources:       resourcesFor(wfe),
						},
					},
				},
			},
		},
	}
}

// jobPodFailurePolicy returns the unconditional PodFailurePolicy (BR-WE-019 /
// DD-WE-008) that tolerates infra-caused pod failures -- OOM-kill (exit 137)
// and node-disruption (DisruptionTarget) -- by Ignoring them, while every
// other failure keeps today's Count (fail-fast) behavior against the
// unchanged backoffLimit: 0. This grants tolerance for specific,
// identifiable causes without weakening fail-fast for a genuinely-failing,
// potentially non-idempotent remediation script (DD-WE-008 Scenario 5).
func jobPodFailurePolicy() *batchv1.PodFailurePolicy {
	return &batchv1.PodFailurePolicy{
		Rules: []batchv1.PodFailurePolicyRule{
			{
				Action: batchv1.PodFailurePolicyActionIgnore,
				OnPodConditions: []batchv1.PodFailurePolicyOnPodConditionsPattern{
					{Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue},
				},
			},
			{
				Action: batchv1.PodFailurePolicyActionIgnore,
				OnExitCodes: &batchv1.PodFailurePolicyOnExitCodesRequirement{
					ContainerName: ptr.To("workflow"),
					Operator:      batchv1.PodFailurePolicyOnExitCodesOpIn,
					Values:        []int32{137},
				},
			},
		},
	}
}

// resourcesFor returns the "workflow" container's resource requirements
// (BR-WE-019 / DD-WE-008), resolved once during Pending into
// wfe.Status.Resources by resolveWorkflowCatalog. Returns the zero value
// (no requests/limits, BestEffort QoS) when the catalog entry declared none
// -- backward compatible with pre-DD-WE-008 Job specs.
func resourcesFor(wfe *workflowexecutionv1alpha1.WorkflowExecution) corev1.ResourceRequirements {
	if wfe.Status.Resources == nil {
		return corev1.ResourceRequirements{}
	}
	return *wfe.Status.Resources
}

// buildDependencyVolumes creates Volumes and VolumeMounts for schema-declared
// dependencies (DD-WE-006). Returns empty slices when deps is nil.
func buildDependencyVolumes(deps *models.WorkflowDependencies) ([]corev1.Volume, []corev1.VolumeMount) {
	if deps == nil {
		return nil, nil
	}

	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount

	for _, s := range deps.Secrets {
		volName := "secret-" + s.Name
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: s.Name},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: filepath.Join(SecretMountBasePath, s.Name),
			ReadOnly:  true,
		})
	}

	for _, cm := range deps.ConfigMaps {
		volName := "configmap-" + cm.Name
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: cm.Name},
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: filepath.Join(ConfigMapMountBasePath, cm.Name),
			ReadOnly:  true,
		})
	}

	return volumes, mounts
}

// buildEnvVars converts workflow parameters to container environment variables.
// Also adds TARGET_RESOURCE for consistency with Tekton pipelines.
// #243: Accepts pre-filtered params (filtering is done in buildJob).
func buildEnvVars(targetResource string, params map[string]string) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0, 1+len(params))
	envVars = append(envVars, corev1.EnvVar{
		Name:  "TARGET_RESOURCE",
		Value: targetResource,
	})

	for key, value := range params {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	return envVars
}

// buildStatusSummary creates a lightweight status summary from a Job.
func (j *JobExecutor) buildStatusSummary(ctx context.Context, c ExecutorClient, job *batchv1.Job) *workflowexecutionv1alpha1.ExecutionStatusSummary {
	summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
		Status:     corev1.ConditionUnknown,
		TotalTasks: 1,
		// BR-WE-019 AC10 / DD-WE-008 Wiring Point C: captured unconditionally
		// (including on the success branch below) so PodFailurePolicy-
		// tolerated pod failures still surface in the audit trail as a
		// retry count. See countPodCreationAttempts for why this is NOT
		// job.Status.Failed (confirmed, via a real-cluster spike, to never
		// increment for Ignore-action failures).
		RetryCount: j.countPodCreationAttempts(ctx, c, job),
	}

	switch {
	case job.Status.Succeeded > 0:
		summary.Status = corev1.ConditionTrue
		summary.Reason = "Succeeded"
		summary.CompletedTasks = 1
	case job.Status.Failed > 0:
		summary.Status = corev1.ConditionFalse
		summary.Reason = "Failed"
		summary.Message = fmt.Sprintf("%d pod(s) failed", job.Status.Failed)
	case job.Status.Active > 0:
		summary.Status = corev1.ConditionUnknown
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
