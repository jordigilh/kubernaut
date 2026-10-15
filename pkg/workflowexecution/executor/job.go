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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/shared/sizeutil"
)

const (
	SecretMountBasePath    = "/run/kubernaut/secrets"
	ConfigMapMountBasePath = "/run/kubernaut/configmaps"

	// defaultJobActiveDeadline is the fallback ActiveDeadlineSeconds applied to
	// the Job (BR-WORKFLOW-008) when the WFE does not declare an explicit
	// Spec.TimesOutAt (DD-TIMEOUT-002). Matches the 30-minute default already
	// used for Tekton executions and RemediationOrchestrator's Executing-phase
	// safety net (BR-ORCH-028), so a Pod unable to mount a missing dependency
	// reaches a terminal JobFailed condition instead of hanging indefinitely.
	defaultJobActiveDeadline = 30 * time.Minute

	// minJobActiveDeadlineSeconds floors the computed ActiveDeadlineSeconds
	// (DD-TIMEOUT-002) so an already-past or near-past Spec.TimesOutAt still
	// produces a valid, strictly-positive K8s Job deadline instead of a
	// zero/negative value.
	minJobActiveDeadlineSeconds = 1

	// jobDeletionPollInterval and jobDeletionTimeout cover asynchronous remote
	// delete acknowledgements for the legacy cleanup path. New Job identities do
	// not require deletion before creation (DD-WE-010).
	jobDeletionPollInterval = 100 * time.Millisecond
	jobDeletionTimeout      = 30 * time.Second

	jobNamePrefix              = "wfe-"
	jobAttemptSuffixLength     = 4
	maxKubernetesJobNameLength = 63
)

const (
	jobWorkflowExecutionLabel     = "kubernaut.ai/workflow-execution"
	jobWorkflowExecutionUIDLabel  = "kubernaut.ai/workflow-execution-uid"
	jobRemediationRequestLabel    = "kubernaut.ai/remediation-request"
	jobRemediationRequestNSLabel  = "kubernaut.ai/remediation-request-namespace"
	jobRemediationRequestUIDLabel = "kubernaut.ai/remediation-request-uid"
	jobWorkflowIDLabel            = "kubernaut.ai/workflow-id"
	jobSourceNamespaceLabel       = "kubernaut.ai/source-namespace"
	jobTargetResourceLabel        = "kubernaut.ai/target-resource"
	jobExecutionEngineLabel       = "kubernaut.ai/execution-engine"
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

// JobExecutionResourceName returns the immutable Job identity for one
// WorkflowExecution attempt (DD-WE-010). A RemediationRequest UID separates
// sequential remediation cycles, while the WFE UID suffix separates attempts
// associated with one RR. The normal UUID shape is 45 characters long:
// wfe-<full RR UID>-<four-character WFE UID suffix>.
//
// Objects created before RR UID propagation may not have a reference UID. They
// retain the old target-derived name as a migration/test fallback; production
// WorkflowExecutions created by RemediationOrchestrator always carry the RR
// UID in spec.remediationRequestRef.
func JobExecutionResourceName(wfe *workflowexecutionv1alpha1.WorkflowExecution) string {
	if wfe == nil {
		return ""
	}

	rrUID := sanitizeJobNamePart(string(wfe.Spec.RemediationRequestRef.UID))
	if rrUID == "" {
		return ExecutionResourceName(wfe.Spec.TargetResource)
	}

	suffix := jobAttemptSuffix(wfe)
	maxRRUIDLength := maxKubernetesJobNameLength - len(jobNamePrefix) - 1 - jobAttemptSuffixLength
	if len(rrUID) > maxRRUIDLength {
		rrUID = strings.TrimRight(rrUID[:maxRRUIDLength], "-")
	}
	return fmt.Sprintf("%s%s-%s", jobNamePrefix, rrUID, suffix)
}

func sanitizeJobNamePart(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '-':
			builder.WriteRune(char)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func jobAttemptSuffix(wfe *workflowexecutionv1alpha1.WorkflowExecution) string {
	uid := sanitizeJobNamePart(string(wfe.UID))
	if len(uid) >= jobAttemptSuffixLength {
		return uid[len(uid)-jobAttemptSuffixLength:]
	}

	// A WFE UID is assigned by the API server before production reconciliation.
	// Hash the immutable namespace/name only for direct unit fixtures that have
	// not gone through the API server yet.
	hash := sha256.Sum256([]byte(wfe.Namespace + "/" + wfe.Name))
	return hex.EncodeToString(hash[:])[:jobAttemptSuffixLength]
}

// jobNameForWFE returns the Job name that a WFE is authorized to observe or
// clean up. New WFEs use JobExecutionResourceName. Existing WFEs may already
// have Status.ExecutionRef set to the pre-DD-WE-010 target-derived name; that
// recorded legacy reference is accepted during migration, but arbitrary names
// are rejected to prevent adoption of unrelated Jobs.
func jobNameForWFE(wfe *workflowexecutionv1alpha1.WorkflowExecution) (string, error) {
	expectedName := JobExecutionResourceName(wfe)
	if wfe.Status.ExecutionRef == nil || wfe.Status.ExecutionRef.Name == "" {
		if wfe.Spec.RemediationRequestRef.UID == "" {
			return "", fmt.Errorf("legacy Job access requires Status.ExecutionRef for WFE %s/%s", wfe.Namespace, wfe.Name)
		}
		return expectedName, nil
	}

	recordedName := wfe.Status.ExecutionRef.Name
	if recordedName == expectedName || recordedName == ExecutionResourceName(wfe.Spec.TargetResource) {
		return recordedName, nil
	}
	return "", fmt.Errorf("execution ref %q does not match expected Job %q for WFE %s/%s", recordedName, expectedName, wfe.Namespace, wfe.Name)
}

func jobProvenanceMetadata(wfe *workflowexecutionv1alpha1.WorkflowExecution) (map[string]string, map[string]string) {
	labels := map[string]string{
		jobWorkflowExecutionLabel: jobLabelValue(wfe.Name),
		jobWorkflowIDLabel:        jobLabelValue(wfe.Spec.WorkflowRef.WorkflowID),
		jobTargetResourceLabel:    sanitizeLabelValue(wfe.Spec.TargetResource),
		jobSourceNamespaceLabel:   jobLabelValue(wfe.Namespace),
		jobExecutionEngineLabel:   "job",
	}
	annotations := map[string]string{
		jobWorkflowExecutionLabel: wfe.Name,
		jobWorkflowIDLabel:        wfe.Spec.WorkflowRef.WorkflowID,
		jobTargetResourceLabel:    wfe.Spec.TargetResource,
		jobSourceNamespaceLabel:   wfe.Namespace,
	}

	if wfe.UID != "" {
		labels[jobWorkflowExecutionUIDLabel] = jobLabelValue(string(wfe.UID))
		annotations[jobWorkflowExecutionUIDLabel] = string(wfe.UID)
	}
	if ref := wfe.Spec.RemediationRequestRef; ref.Name != "" {
		labels[jobRemediationRequestLabel] = jobLabelValue(ref.Name)
		annotations[jobRemediationRequestLabel] = ref.Name
	}
	if ref := wfe.Spec.RemediationRequestRef; ref.Namespace != "" {
		labels[jobRemediationRequestNSLabel] = jobLabelValue(ref.Namespace)
		annotations[jobRemediationRequestNSLabel] = ref.Namespace
	}
	if ref := wfe.Spec.RemediationRequestRef; ref.UID != "" {
		labels[jobRemediationRequestUIDLabel] = jobLabelValue(string(ref.UID))
		annotations[jobRemediationRequestUIDLabel] = string(ref.UID)
	}
	return labels, annotations
}

func jobLabelValue(value string) string {
	return sanitizeLabelValue(value)
}

func cloneStringMap(values map[string]string) map[string]string {
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func jobIdentityMatches(existing, expected *batchv1.Job) bool {
	if !jobProvenanceAndImageMatches(existing, expected) {
		return false
	}
	if !jobExecutionTemplateMatches(existing, expected) {
		return false
	}
	return true
}

// jobProvenanceAndImageMatches is the status/cleanup-safe identity check. The
// controller cannot reconstruct CreateOptions after a restart, so status and
// cleanup validate the authoritative name, exact image, and corroborating
// provenance without comparing option-dependent volume/env projections. The
// create AlreadyExists path uses jobIdentityMatches for the stricter template
// comparison before adopting an existing same-name Job.
func jobProvenanceAndImageMatches(existing, expected *batchv1.Job) bool {
	if existing.Name != expected.Name || len(existing.Spec.Template.Spec.Containers) != 1 || len(expected.Spec.Template.Spec.Containers) != 1 {
		return false
	}
	if existing.Spec.Template.Spec.Containers[0].Image != expected.Spec.Template.Spec.Containers[0].Image {
		return false
	}

	identityLabels := []string{
		jobWorkflowExecutionLabel,
		jobWorkflowExecutionUIDLabel,
		jobRemediationRequestLabel,
		jobRemediationRequestNSLabel,
		jobRemediationRequestUIDLabel,
		jobWorkflowIDLabel,
		jobSourceNamespaceLabel,
		jobExecutionEngineLabel,
	}
	for _, key := range identityLabels {
		expectedValue, expectedSet := expected.Labels[key]
		if !expectedSet || expectedValue == "" {
			continue
		}
		if existing.Labels[key] != expectedValue {
			return false
		}
	}
	return true
}

// jobExecutionTemplateMatches compares the deterministic execution projection
// while excluding mutable/time-derived fields such as TTL and the relative
// active deadline. Environment, mounts, and volumes are normalized because
// parameter maps and dependency maps are not ordered.
func jobExecutionTemplateMatches(existing, expected *batchv1.Job) bool {
	existingPod := existing.Spec.Template.Spec
	expectedPod := expected.Spec.Template.Spec
	if existingPod.ServiceAccountName != expectedPod.ServiceAccountName ||
		existingPod.RestartPolicy != expectedPod.RestartPolicy ||
		!reflect.DeepEqual(existingPod.SecurityContext, expectedPod.SecurityContext) ||
		!reflect.DeepEqual(normalizedVolumes(existingPod.Volumes), normalizedVolumes(expectedPod.Volumes)) ||
		!reflect.DeepEqual(normalizedContainer(existingPod.Containers[0]), normalizedContainer(expectedPod.Containers[0])) {
		return false
	}
	return reflect.DeepEqual(existing.Spec.PodFailurePolicy, expected.Spec.PodFailurePolicy) &&
		reflect.DeepEqual(existing.Spec.PodReplacementPolicy, expected.Spec.PodReplacementPolicy) &&
		int32Value(existing.Spec.BackoffLimit) == int32Value(expected.Spec.BackoffLimit)
}

func normalizedContainer(container corev1.Container) corev1.Container {
	if container.ImagePullPolicy == "" {
		container.ImagePullPolicy = defaultImagePullPolicy(container.Image)
	}
	if container.TerminationMessagePath == "" {
		container.TerminationMessagePath = corev1.TerminationMessagePathDefault
	}
	if container.TerminationMessagePolicy == "" {
		container.TerminationMessagePolicy = corev1.TerminationMessageReadFile
	}
	container.Env = append([]corev1.EnvVar(nil), container.Env...)
	sort.Slice(container.Env, func(i, j int) bool { return container.Env[i].Name < container.Env[j].Name })
	container.EnvFrom = append([]corev1.EnvFromSource(nil), container.EnvFrom...)
	sort.Slice(container.EnvFrom, func(i, j int) bool {
		return envFromKey(container.EnvFrom[i]) < envFromKey(container.EnvFrom[j])
	})
	container.VolumeMounts = append([]corev1.VolumeMount(nil), container.VolumeMounts...)
	sort.Slice(container.VolumeMounts, func(i, j int) bool { return container.VolumeMounts[i].Name < container.VolumeMounts[j].Name })
	return container
}

func defaultImagePullPolicy(image string) corev1.PullPolicy {
	if strings.Contains(image, "@sha256:") {
		return corev1.PullIfNotPresent
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon <= lastSlash || strings.HasSuffix(image, ":latest") {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
}

func normalizedVolumes(volumes []corev1.Volume) []corev1.Volume {
	result := append([]corev1.Volume(nil), volumes...)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func envFromKey(source corev1.EnvFromSource) string {
	if source.ConfigMapRef != nil {
		return "configmap/" + source.ConfigMapRef.Name
	}
	if source.SecretRef != nil {
		return "secret/" + source.SecretRef.Name
	}
	return ""
}

func int32Value(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

// jobCanBeDeletedByWFE applies the strong identity checks available to a
// cleanup operation. Labels are only corroborating provenance hints: the
// expected name and Status.ExecutionRef are checked by the caller, and a
// production WFE also requires its immutable UID/RR UID hints and exact image
// before deletion. Legacy objects without either UID retain the historical
// label-only compatibility path.
func jobCanBeDeletedByWFE(existing, expected *batchv1.Job, wfe *workflowexecutionv1alpha1.WorkflowExecution) bool {
	if existing.Name != expected.Name || !jobOwnerMetadataMatches(existing, wfe.Name) {
		return false
	}
	strongIdentity := wfe.UID != "" || wfe.Spec.RemediationRequestRef.UID != ""
	if existing.Name == ExecutionResourceName(wfe.Spec.TargetResource) &&
		(existing.Name != JobExecutionResourceName(wfe) ||
			existing.Labels[jobWorkflowExecutionUIDLabel] == "" ||
			existing.Labels[jobRemediationRequestUIDLabel] == "") {
		if !strongIdentity {
			return true
		}
		return legacyJobIdentityMatches(existing, expected, wfe)
	}
	if strongIdentity {
		return jobProvenanceAndImageMatches(existing, expected)
	}
	return true
}

func jobOwnerMetadataMatches(existing *batchv1.Job, wfeName string) bool {
	if existing.Labels[jobWorkflowExecutionLabel] == wfeName {
		return true
	}
	return existing.Annotations[jobWorkflowExecutionLabel] == wfeName
}

// legacyJobIdentityMatches supports cleanup of Jobs created before DD-WE-010.
// The recorded legacy name and exact image are checked by the caller; legacy
// labels provide the remaining owner corroboration without becoming an
// authorization path for new Jobs.
func legacyJobIdentityMatches(existing, expected *batchv1.Job, wfe *workflowexecutionv1alpha1.WorkflowExecution) bool {
	if len(existing.Spec.Template.Spec.Containers) != 1 || len(expected.Spec.Template.Spec.Containers) != 1 {
		return false
	}
	if existing.Spec.Template.Spec.Containers[0].Image != expected.Spec.Template.Spec.Containers[0].Image {
		return false
	}
	return jobOwnerMetadataMatches(existing, wfe.Name) &&
		existing.Labels[jobSourceNamespaceLabel] == wfe.Namespace &&
		existing.Labels[jobWorkflowIDLabel] == expected.Labels[jobWorkflowIDLabel] &&
		existing.Labels[jobExecutionEngineLabel] == "job"
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
		if !apierrors.IsAlreadyExists(err) {
			return nil, err // Preserve original error for controller classification
		}

		// A deterministic name makes a lost status update safe to retry. Never
		// adopt or delete a resource that only happens to occupy the name: the
		// exact immutable bundle image and provenance identity must match.
		var existing batchv1.Job
		existing.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))
		if getErr := c.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: namespace}, &existing); getErr != nil {
			return nil, fmt.Errorf("verify existing Job %s/%s after create collision: %w", namespace, job.Name, getErr)
		}
		if !jobIdentityMatches(&existing, job) {
			collisionErr := fmt.Errorf("execution Job %s/%s already exists with mismatched identity or image", namespace, job.Name)
			log.FromContext(ctx).Error(collisionErr, "refusing to adopt existing execution Job")
			// Keep AlreadyExists classification for the controller's fail-closed
			// collision/audit path; no delete or replacement is attempted here.
			return nil, err
		}
		return &CreateResult{ResourceName: job.Name}, nil
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
	jobName, nameErr := jobNameForWFE(wfe)
	if nameErr != nil {
		return nil, nameErr
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
		Name:      jobName,
		Namespace: namespace,
	}, &job); err != nil {
		return nil, err
	}
	if wfe.UID != "" || wfe.Spec.RemediationRequestRef.UID != "" {
		expected := j.buildJob(ctx, wfe, namespace, CreateOptions{})
		expected.Name = jobName
		matches := jobProvenanceAndImageMatches(&job, expected)
		if jobName == ExecutionResourceName(wfe.Spec.TargetResource) && jobName != JobExecutionResourceName(wfe) {
			matches = legacyJobIdentityMatches(&job, expected, wfe)
		}
		if !matches {
			return nil, fmt.Errorf("execution Job %s/%s has mismatched identity or image", namespace, jobName)
		}
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
		default:
			// Suspended/FailureTarget/SuccessCriteriaMet are non-terminal
			// signals -- only Complete/Failed determine this loop's
			// outcome; anything else falls through to "still running".
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
// (BR-WORKFLOW-008) from the WFE's Spec.TimesOutAt (DD-TIMEOUT-002 / Issue
// #2176), an absolute deadline propagated by RO from
// RemediationRequest.Status.TimeoutConfig.Executing, converted to the
// K8s Job's relative-duration field via time.Until at Job-creation time.
// Falls back to defaultJobActiveDeadline when Spec.TimesOutAt is unset. This
// bounds how long a Pod can remain unable to start (e.g. stuck mounting a
// missing dependency) before the Job reaches a terminal JobFailed condition.
func activeDeadlineSecondsFor(wfe *workflowexecutionv1alpha1.WorkflowExecution) int64 {
	timeout := defaultJobActiveDeadline
	if remaining, ok := remainingUntilDeadline(wfe.Spec.TimesOutAt); ok {
		timeout = remaining
	}
	if seconds := int64(timeout.Seconds()); seconds >= minJobActiveDeadlineSeconds {
		return seconds
	}
	return minJobActiveDeadlineSeconds
}

// IsCompleted checks whether the legacy target-derived Job for the given target
// resource is in a terminal state. It is retained for compatibility with older
// callers and tests; current Job creation and collision handling use the
// per-WFE identity from JobExecutionResourceName instead (DD-WE-010).
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
// DD-WE-010: cleanup uses the WFE's recorded execution reference and expected
// per-WFE name. A mismatched identity or image is left untouched.
func (j *JobExecutor) Cleanup(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) error {
	c, err := j.factory.ClientFor(ctx, wfe.Spec.ClusterID)
	if err != nil {
		return fmt.Errorf("get client for cluster %q: %w", wfe.Spec.ClusterID, err)
	}

	jobName, nameErr := jobNameForWFE(wfe)
	if nameErr != nil {
		return nameErr
	}

	var existing batchv1.Job
	existing.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))
	if err := c.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, &existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil // Already gone
		}
		return fmt.Errorf("failed to get Job %s/%s for ownership check: %w", namespace, jobName, err)
	}

	expected := j.buildJob(ctx, wfe, namespace, CreateOptions{})
	expected.Name = jobName
	if !jobCanBeDeletedByWFE(&existing, expected, wfe) {
		log.FromContext(ctx).Info("refusing to delete execution Job with mismatched identity or image", "job", jobName, "wfe", wfe.Name)
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

	if err := wait.PollUntilContextTimeout(ctx, jobDeletionPollInterval, jobDeletionTimeout, true,
		func(ctx context.Context) (bool, error) {
			var deleted batchv1.Job
			deleted.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))
			err := c.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, &deleted)
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			if err != nil {
				return false, fmt.Errorf("check deletion of Job %s/%s: %w", namespace, jobName, err)
			}
			return false, nil
		}); err != nil {
		return fmt.Errorf("wait for deletion of Job %s/%s: %w", namespace, jobName, err)
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

	jobName := JobExecutionResourceName(wfe)
	labels, annotations := jobProvenanceMetadata(wfe)

	var backoffLimit int32 = 0
	var ttlSeconds int32 = 600
	activeDeadlineSeconds := activeDeadlineSecondsFor(wfe)
	var ttlSecondsAfterFinished *int32
	if !opts.RetainFailedExecutions {
		ttlSecondsAfterFinished = &ttlSeconds
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        jobName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: ttlSecondsAfterFinished,
			ActiveDeadlineSeconds:   &activeDeadlineSeconds,
			PodFailurePolicy:        jobPodFailurePolicy(),
			PodReplacementPolicy:    ptr.To(batchv1.Failed),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      cloneStringMap(labels),
					Annotations: cloneStringMap(annotations),
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: wfe.Spec.WorkflowRef.ServiceAccountName,
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
// (BR-WE-019 / DD-WE-008), read directly from the immutable
// wfe.Spec.WorkflowRef.Resources CRD-embedded snapshot (Issue #1661 Change
// 11f). Returns the zero value (no requests/limits, BestEffort QoS) when
// the snapshot declares none -- backward compatible with pre-DD-WE-008 Job
// specs.
func resourcesFor(wfe *workflowexecutionv1alpha1.WorkflowExecution) corev1.ResourceRequirements {
	if wfe.Spec.WorkflowRef.Resources == nil {
		return corev1.ResourceRequirements{}
	}
	return *wfe.Spec.WorkflowRef.Resources
}

// buildDependencyVolumes creates Volumes and VolumeMounts for schema-declared
// dependencies (DD-WE-006). Returns empty slices when deps is nil.
func buildDependencyVolumes(deps *models.WorkflowDependencies) ([]corev1.Volume, []corev1.VolumeMount) {
	if deps == nil {
		return nil, nil
	}

	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount
	defaultMode := int32(0644)

	for _, s := range deps.Secrets {
		volName := "secret-" + s.Name
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: s.Name, DefaultMode: &defaultMode},
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
					DefaultMode:          &defaultMode,
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
	// Issue #1684: sizeutil.SafeCap makes the overflow check on this capacity
	// hint explicit (see its doc comment for why CodeQL flags raw "a+b").
	envVars := make([]corev1.EnvVar, 0, sizeutil.SafeCap(1, len(params)))
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
