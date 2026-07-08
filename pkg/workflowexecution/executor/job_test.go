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

package executor_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// mockClientFactory implements ClientFactory for testing.
type mockClientFactory struct {
	client    executor.ExecutorClient
	returnErr error
}

func (m *mockClientFactory) ClientFor(_ context.Context, _ string) (executor.ExecutorClient, error) {
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return m.client, nil
}

// recordingClientFactory records the clusterID passed to ClientFor for assertion.
type recordingClientFactory struct {
	client        executor.ExecutorClient
	lastClusterID string
}

func (r *recordingClientFactory) ClientFor(_ context.Context, clusterID string) (executor.ExecutorClient, error) {
	r.lastClusterID = clusterID
	return r.client, nil
}

// gvkCapturingClient wraps a delegate ExecutorClient and records the
// GroupVersionKind of the object passed to Create, so tests can assert the
// executor set TypeMeta before handing the object to the client (BR-FLEET-054:
// the MCP remote writer needs an explicit "kind" to decode the manifest,
// unlike the local controller-runtime client which infers it from the
// scheme).
type gvkCapturingClient struct {
	executor.ExecutorClient
	createdGVK schema.GroupVersionKind
}

func (g *gvkCapturingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	g.createdGVK = obj.GetObjectKind().GroupVersionKind()
	return g.ExecutorClient.Create(ctx, obj, opts...)
}

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(batchv1.AddToScheme(scheme)).To(Succeed())
	return scheme
}

func newTestWFE(name, targetResource, clusterID string) *workflowexecutionv1alpha1.WorkflowExecution {
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "kubernaut-system",
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:      "wf-001",
				Version:         "v1.0.0",
				ExecutionBundle: "registry.example.com/workflow:v1",
			},
			TargetResource: targetResource,
			ClusterID:      clusterID,
			Parameters:     map[string]string{"TIMEOUT": "30s"},
		},
		Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
			ServiceAccountName: "kubernaut-runner",
		},
	}
}

// newSuccessfulCreateEvent builds a "SuccessfulCreate" Event on a Job object,
// matching what the real job-controller emits each time it creates a Pod
// (initial attempt or PodFailurePolicy-Ignore replacement). Used to simulate
// BR-WE-019 AC10's RetryCount computation without job.Status.Failed, which a
// real-cluster spike (DD-WE-008 Section 8) confirmed is never incremented
// for Ignore-action failures.
func newSuccessfulCreateEvent(name, namespace, jobName, podName string, count int32) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Job",
			Name:      jobName,
			Namespace: namespace,
		},
		Reason:  "SuccessfulCreate",
		Message: fmt.Sprintf("Created pod: %s", podName),
		Count:   count,
		Type:    corev1.EventTypeNormal,
	}
}

// UT-WE-054-JOB: JobExecutor unit tests
// Authority: BR-WE-014 (Kubernetes Job Execution Backend), BR-FLEET-054
// FedRAMP: AC-6 (Least Privilege) -- SA injection, AU-3 (Audit Content) -- labels
var _ = Describe("UT-WE-054-JOB: JobExecutor", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = newTestScheme()
		namespace = "kubernaut-workflows"
	})

	Describe("Engine", func() {
		It("UT-WE-054-JOB-001: should return 'job' as engine identifier", func() {
			je := executor.NewJobExecutor(fake.NewClientBuilder().WithScheme(scheme).Build())
			Expect(je.Engine()).To(Equal("job"))
		})
	})

	Describe("Create", func() {
		It("UT-WE-054-JOB-002: should create a Job with correct labels and env vars", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-test-001", "default/deployment/nginx", "")

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.ResourceName).To(HavePrefix("wfe-"))

			var job batchv1.Job
			err = fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)
			Expect(err).ToNot(HaveOccurred())
			Expect(job.Labels["kubernaut.ai/workflow-execution"]).To(Equal("wfe-test-001"))
			Expect(job.Labels["kubernaut.ai/execution-engine"]).To(Equal("job"))
			Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("kubernaut-runner"))

			envNames := make([]string, 0, len(job.Spec.Template.Spec.Containers[0].Env))
			for _, e := range job.Spec.Template.Spec.Containers[0].Env {
				envNames = append(envNames, e.Name)
			}
			Expect(envNames).To(ContainElement("TARGET_RESOURCE"))
			Expect(envNames).To(ContainElement("TIMEOUT"))
		})

		It("UT-WE-054-JOB-003: should mount secret and configmap dependencies", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-dep-test", "default/deployment/api", "")

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets:    []models.ResourceDependency{{Name: "db-creds"}},
					ConfigMaps: []models.ResourceDependency{{Name: "app-config"}},
				},
			}

			result, err := je.Create(ctx, wfe, namespace, opts)
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			err = fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)
			Expect(err).ToNot(HaveOccurred())

			volNames := make([]string, 0, len(job.Spec.Template.Spec.Volumes))
			for _, v := range job.Spec.Template.Spec.Volumes {
				volNames = append(volNames, v.Name)
			}
			Expect(volNames).To(ContainElement("secret-db-creds"))
			Expect(volNames).To(ContainElement("configmap-app-config"))

			mountPaths := make([]string, 0, len(job.Spec.Template.Spec.Containers[0].VolumeMounts))
			for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
				mountPaths = append(mountPaths, m.MountPath)
			}
			Expect(mountPaths).To(ContainElement(ContainSubstring("secrets/db-creds")))
			Expect(mountPaths).To(ContainElement(ContainSubstring("configmaps/app-config")))
		})

		It("UT-WE-054-JOB-004: should propagate ClientFactory error", func() {
			factory := &mockClientFactory{returnErr: fmt.Errorf("connection refused")}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-err", "default/deployment/nginx", "unreachable")

			_, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get client for cluster"))
		})

		// BR-WE-018: spawned Job pods must be hardened to the restricted SecurityContext
		// profile (FedRAMP AC-6/CM-7), matching the WE controller's own pod hardening.
		It("UT-WE-054-JOB-014: should apply restricted pod and container SecurityContext", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-secctx", "default/deployment/nginx", "")

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			podSC := job.Spec.Template.Spec.SecurityContext
			Expect(podSC).ToNot(BeNil())
			Expect(*podSC.RunAsNonRoot).To(BeTrue())
			Expect(podSC.SeccompProfile).ToNot(BeNil())
			Expect(podSC.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))

			containerSC := job.Spec.Template.Spec.Containers[0].SecurityContext
			Expect(containerSC).ToNot(BeNil())
			Expect(*containerSC.AllowPrivilegeEscalation).To(BeFalse())
			Expect(*containerSC.ReadOnlyRootFilesystem).To(BeTrue())
			Expect(*containerSC.RunAsNonRoot).To(BeTrue())
			Expect(containerSC.Capabilities).ToNot(BeNil())
			Expect(containerSC.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
		})

		// BR-WE-019 / DD-WE-008: the "workflow" container's resource requests
		// and limits come from WFE.Status.Resources, resolved once during
		// Pending from the DS catalog's execution.resources section.
		It("UT-WE-054-JOB-021 [BR-WE-019]: should apply WFE.Status.Resources to the workflow container", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-resources", "default/deployment/nginx", "")
			wfe.Status.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			}

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			resources := job.Spec.Template.Spec.Containers[0].Resources
			Expect(resources.Requests.Cpu().String()).To(Equal("100m"))
			Expect(resources.Requests.Memory().String()).To(Equal("128Mi"))
			Expect(resources.Limits.Cpu().String()).To(Equal("500m"))
			Expect(resources.Limits.Memory().String()).To(Equal("256Mi"))
		})

		It("UT-WE-054-JOB-024 [BR-WE-019]: should leave the workflow container BestEffort when WFE.Status.Resources is nil (backward compat)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-no-resources", "default/deployment/nginx", "")
			Expect(wfe.Status.Resources).To(BeNil())

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			resources := job.Spec.Template.Spec.Containers[0].Resources
			Expect(resources.Requests).To(BeEmpty(), "no requests when catalog declares no resources")
			Expect(resources.Limits).To(BeEmpty(), "no limits when catalog declares no resources")
		})

		// BR-WE-019 / DD-WE-008 Wiring Point B: PodFailurePolicy tolerates
		// OOM-kill (exit 137) and node-disruption pod failures without
		// weakening fail-fast (BackoffLimit: 0) for any other failure cause.
		It("UT-WE-054-JOB-022 [BR-WE-019]: should set PodFailurePolicy with Ignore rules for exit-137 and DisruptionTarget", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-podfailurepolicy", "default/deployment/nginx", "")

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			Expect(job.Spec.PodFailurePolicy).ToNot(BeNil())
			Expect(job.Spec.PodFailurePolicy.Rules).To(ContainElement(
				And(
					HaveField("Action", batchv1.PodFailurePolicyActionIgnore),
					HaveField("OnPodConditions", ContainElement(
						HaveField("Type", corev1.DisruptionTarget),
					)),
				),
			))
			Expect(job.Spec.PodFailurePolicy.Rules).To(ContainElement(
				And(
					HaveField("Action", batchv1.PodFailurePolicyActionIgnore),
					HaveField("OnExitCodes", And(
						HaveField("ContainerName", HaveValue(Equal("workflow"))),
						HaveField("Operator", batchv1.PodFailurePolicyOnExitCodesOpIn),
						HaveField("Values", ConsistOf(int32(137))),
					)),
				),
			))
		})

		It("UT-WE-054-JOB-023 [BR-WE-019]: should keep BackoffLimit at 0 (fail-fast unchanged for non-tolerated failures)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-backofflimit", "default/deployment/nginx", "")

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			Expect(job.Spec.BackoffLimit).ToNot(BeNil())
			Expect(*job.Spec.BackoffLimit).To(Equal(int32(0)),
				"BackoffLimit must remain 0 -- PodFailurePolicy grants tolerance for specific causes, not a general retry budget")
		})

		// BR-WORKFLOW-008: without pre-flight dependency validation (#1481), a Job
		// referencing a missing Secret/ConfigMap must still reach a terminal state
		// instead of hanging in Pending forever.
		It("UT-WE-054-JOB-016 [BR-WORKFLOW-008]: should default ActiveDeadlineSeconds to 30m when ExecutionConfig.Timeout is unset", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-deadline-default", "default/deployment/nginx", "")

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			Expect(job.Spec.ActiveDeadlineSeconds).ToNot(BeNil(),
				"BR-WORKFLOW-008: Job must have a deadline so it reaches JobFailed instead of hanging on a missing dependency")
			Expect(*job.Spec.ActiveDeadlineSeconds).To(Equal(int64(30*60)),
				"default deadline should be 30 minutes when WFE does not declare an explicit timeout")
		})

		It("UT-WE-054-JOB-017 [BR-WORKFLOW-008]: should source ActiveDeadlineSeconds from ExecutionConfig.Timeout when set", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-deadline-custom", "default/deployment/nginx", "")
			wfe.Spec.ExecutionConfig = &workflowexecutionv1alpha1.ExecutionConfig{
				Timeout: &metav1.Duration{Duration: 5 * time.Minute},
			}

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			Expect(job.Spec.ActiveDeadlineSeconds).ToNot(BeNil())
			Expect(*job.Spec.ActiveDeadlineSeconds).To(Equal(int64(5*60)),
				"deadline should be sourced from WFE ExecutionConfig.Timeout when declared")
		})

		It("UT-WE-054-JOB-015: should mount a writable /tmp scratch volume for readOnlyRootFilesystem compatibility", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-scratch", "default/deployment/nginx", "")

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(And(
				HaveField("Name", "tmp"),
				HaveField("VolumeSource.EmptyDir", Not(BeNil())),
			)))

			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(ContainElement(And(
				HaveField("Name", "tmp"),
				HaveField("MountPath", "/tmp"),
			)))

			envByName := map[string]string{}
			for _, e := range container.Env {
				envByName[e.Name] = e.Value
			}
			Expect(envByName).To(HaveKeyWithValue("HOME", "/tmp"))
			Expect(envByName).To(HaveKeyWithValue("TMPDIR", "/tmp"))
		})

		// BR-FLEET-054 history: this previously asserted that buildJob() set
		// TypeMeta explicitly, working around pkg/fleet/mcpclient's remote
		// writer needing apiVersion/kind on the wire (Issue #1542
		// E2E-FLEET-014 CI failure: "Object 'Kind' is missing"). That
		// workaround was removed once mcpclient itself was hardened to infer
		// GVK for built-in types like batchv1.Job from a runtime.Scheme (see
		// pkg/fleet/mcpclient/gvk.go's ensureGVK) -- buildJob() no longer
		// needs to set it, and the "Create succeeds + wire manifest carries
		// kind=Job" contract is now proven at the mcpclient layer itself
		// (UT-FLEET-GVK-005), which is a stronger proof since it asserts the
		// actual serialized wire format rather than a pre-Create in-memory
		// GVK.
	})

	Describe("GetStatus", func() {
		It("UT-WE-054-JOB-005: should return Completed when Job has Complete condition", func() {
			jobName := executor.ExecutionResourceName("default/deployment/nginx")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Succeeded: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobComplete,
							Status:  corev1.ConditionTrue,
							Message: "Job completed successfully",
						},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-status", "default/deployment/nginx", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))
			Expect(result.Summary).ToNot(BeNil())
			Expect(result.Summary.Status).To(Equal(corev1.ConditionTrue))
		})

		It("UT-WE-054-JOB-006: should return Failed when Job has Failed condition", func() {
			jobName := executor.ExecutionResourceName("default/deployment/broken")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job has reached the specified backoff limit",
						},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-failed", "default/deployment/broken", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Reason).To(Equal("BackoffLimitExceeded"))
		})

		// BR-WORKFLOW-008: since dependency existence is no longer pre-flight
		// validated (#1481), the failure reason for a missing Secret/ConfigMap now
		// surfaces only via the Job's Pod events (kubelet-emitted FailedMount /
		// CreateContainerConfigError). GetStatus must inspect these and enrich
		// the generic Job condition message with the specific missing resource.
		It("UT-WE-054-JOB-018 [BR-WORKFLOW-008]: should enrich Failed message with FailedMount Pod event detail", func() {
			jobName := executor.ExecutionResourceName("default/deployment/dep-missing")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "DeadlineExceeded",
							Message: "Job was active longer than specified deadline",
						},
					},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName + "-abcde",
					Namespace: namespace,
					Labels:    map[string]string{"job-name": jobName},
				},
			}
			evt := &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "evt-failedmount", Namespace: namespace},
				InvolvedObject: corev1.ObjectReference{
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: namespace,
				},
				Reason:  "FailedMount",
				Message: `MountVolume.SetUp failed for volume "secret-my-creds" : secret "my-creds" not found`,
				Type:    corev1.EventTypeWarning,
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, pod, evt).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-dep-missing", "default/deployment/dep-missing", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Message).To(ContainSubstring(`secret "my-creds" not found`),
				"BR-WORKFLOW-008: message should be enriched with the specific missing dependency from the Pod event")
		})

		// UT-WE-054-JOB-020 reproduces a real-cluster gap found via E2E-WE-006-002
		// (issue #1481 PR CI): Kubernetes' job-controller deletes a Job's active
		// Pods as soon as ActiveDeadlineSeconds is exceeded, which normally
		// happens before the next reconcile calls GetStatus. UT-WE-054-JOB-018
		// only exercised the case where the Pod object is still present, which
		// masked this gap. Only the Event (with its own TTL, independent of the
		// Pod's lifetime) reliably survives long enough to be observed.
		It("UT-WE-054-JOB-020 [BR-WORKFLOW-008]: should enrich Failed message from a Pod event even after the Pod itself has been deleted", func() {
			jobName := executor.ExecutionResourceName("default/deployment/dep-missing-pod-gone")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "DeadlineExceeded",
							Message: "Job was active longer than specified deadline",
						},
					},
				},
			}
			// No Pod object created here: the job-controller has already
			// deleted it by the time GetStatus observes the Failed condition.
			evt := &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "evt-failedmount-pod-gone", Namespace: namespace},
				InvolvedObject: corev1.ObjectReference{
					Kind:      "Pod",
					Name:      jobName + "-x7k2m",
					Namespace: namespace,
				},
				Reason:  "FailedMount",
				Message: `MountVolume.SetUp failed for volume "secret-e2e-dep-secret" : secret "e2e-dep-secret" not found`,
				Type:    corev1.EventTypeWarning,
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, evt).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-dep-missing-pod-gone", "default/deployment/dep-missing-pod-gone", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Message).To(ContainSubstring(`secret "e2e-dep-secret" not found`),
				"BR-WORKFLOW-008: enrichment must not depend on the Pod object still existing")
		})

		It("UT-WE-054-JOB-019 [BR-WORKFLOW-008]: should fall back to the generic Job condition message when no matching Pod event exists", func() {
			jobName := executor.ExecutionResourceName("default/deployment/no-events")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job has reached the specified backoff limit",
						},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-no-events", "default/deployment/no-events", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Message).To(Equal("Job has reached the specified backoff limit"),
				"should preserve the generic Job condition message when no enriching Pod event is found")
		})

		// Issue #1645 / BR-WORKFLOW-008: extends the Job engine's runtime
		// observability (already present for dependency-mount failures, see
		// UT-WE-054-JOB-018 above) to container image-pull failures, mirroring
		// the Tekton engine's existing ImagePullBackOff message classification
		// (internal/controller/workflowexecution/failure_analysis.go). This
		// gap is more relevant since Issue #1642 removed the DataStorage
		// pre-flight execution.bundle registry existence check -- bad/
		// unreachable bundle references now always surface here at runtime.
		It("UT-WE-054-JOB-028 [BR-WORKFLOW-008]: should enrich Failed message with the kubelet's detailed image-pull-failure event", func() {
			jobName := executor.ExecutionResourceName("default/deployment/bad-image")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "DeadlineExceeded",
							Message: "Job was active longer than specified deadline",
						},
					},
				},
			}
			evt := &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "evt-errimagepull", Namespace: namespace},
				InvolvedObject: corev1.ObjectReference{
					Kind:      "Pod",
					Name:      jobName + "-x7k2m",
					Namespace: namespace,
				},
				// Real kubelet event: Reason is the generic "Failed" -- the
				// specific detail lives in the message (see
				// pkg/kubelet/images/image_manager.go upstream).
				Reason:  "Failed",
				Message: `Failed to pull image "quay.io/kubernaut/bad-bundle:v1": rpc error: code = NotFound desc = manifest unknown`,
				Type:    corev1.EventTypeWarning,
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, evt).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-bad-image", "default/deployment/bad-image", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Message).To(ContainSubstring("manifest unknown"),
				"BR-WORKFLOW-008: message should be enriched with the specific image-pull failure detail from the Pod event")
		})

		It("UT-WE-054-JOB-029 [BR-WORKFLOW-008]: should enrich Failed message with a terse 'Error: ImagePullBackOff' event when no detailed event exists", func() {
			jobName := executor.ExecutionResourceName("default/deployment/backoff-image")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "DeadlineExceeded",
							Message: "Job was active longer than specified deadline",
						},
					},
				},
			}
			evt := &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "evt-imagepullbackoff", Namespace: namespace},
				InvolvedObject: corev1.ObjectReference{
					Kind:      "Pod",
					Name:      jobName + "-x7k2m",
					Namespace: namespace,
				},
				Reason:  "Failed",
				Message: "Error: ImagePullBackOff",
				Type:    corev1.EventTypeWarning,
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, evt).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-backoff-image", "default/deployment/backoff-image", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Message).To(ContainSubstring("ImagePullBackOff"),
				"BR-WORKFLOW-008: message should be enriched even from the terse image-pull-backoff event variant")
		})

		It("UT-WE-054-JOB-030 [BR-WORKFLOW-008]: should NOT misclassify an unrelated 'Failed' Pod event as an image-pull failure", func() {
			jobName := executor.ExecutionResourceName("default/deployment/probe-failure")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job has reached the specified backoff limit",
						},
					},
				},
			}
			// kubelet reuses the generic "Failed" reason for many unrelated
			// container lifecycle events (e.g. liveness probe failures) --
			// this must NOT be misread as an image-pull failure.
			evt := &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "evt-probe-failed", Namespace: namespace},
				InvolvedObject: corev1.ObjectReference{
					Kind:      "Pod",
					Name:      jobName + "-x7k2m",
					Namespace: namespace,
				},
				Reason:  "Failed",
				Message: "Liveness probe failed: HTTP probe failed with statuscode: 500",
				Type:    corev1.EventTypeWarning,
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, evt).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-probe-failure", "default/deployment/probe-failure", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Message).To(Equal("Job has reached the specified backoff limit"),
				"an unrelated 'Failed' event (e.g. probe failure) must not be misclassified as an image-pull failure")
		})

		It("UT-WE-054-JOB-007: should return Running when Job has no terminal condition", func() {
			jobName := executor.ExecutionResourceName("default/deployment/running")
			completions := int32(1)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Spec:       batchv1.JobSpec{Completions: &completions},
				Status:     batchv1.JobStatus{Active: 1},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-running", "default/deployment/running", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning))
			Expect(result.Message).To(ContainSubstring("active"))
		})

		It("UT-WE-054-JOB-008: should error when ExecutionRef is nil", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-noref", "default/deployment/nginx", "")

			_, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no execution ref"))
		})

		// BR-WE-019 AC10 / DD-WE-008 Wiring Point C: PodFailurePolicy-tolerated
		// pod failures observed on job.Status.Failed before the Job eventually
		// succeeds must still surface in the audit trail as a retry count --
		// buildStatusSummary must read Failed unconditionally, not only on the
		// Failed branch.
		It("UT-WE-054-JOB-025 [BR-WE-019 AC10]: should set RetryCount from SuccessfulCreate event count when the Job ultimately succeeded after tolerated failures", func() {
			// job.Status.Failed is deliberately 0 here: k8s.io/api batch/v1's
			// PodFailurePolicyActionIgnore never increments it for Ignore-action
			// failures (confirmed via a real-cluster spike, DD-WE-008 Section 8).
			// This is also a regression guard -- if the implementation reverted
			// to reading job.Status.Failed, this test would observe RetryCount=0
			// instead of the expected 2.
			jobName := executor.ExecutionResourceName("default/deployment/retried")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Succeeded: 1,
					Failed:    0,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobComplete,
							Status:  corev1.ConditionTrue,
							Message: "Job completed successfully",
						},
					},
				},
			}
			// 3 "SuccessfulCreate" events on the Job (1 initial + 2
			// Ignore-tolerated replacements) => RetryCount = 3 - 1 = 2.
			events := []client.Object{
				newSuccessfulCreateEvent("retried-create-1", namespace, jobName, "pod-a", 1),
				newSuccessfulCreateEvent("retried-create-2", namespace, jobName, "pod-b", 1),
				newSuccessfulCreateEvent("retried-create-3", namespace, jobName, "pod-c", 1),
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).WithObjects(events...).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-retried", "default/deployment/retried", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))
			Expect(result.Summary).ToNot(BeNil())
			Expect(result.Summary.RetryCount).To(Equal(int32(2)),
				"BR-WE-019 AC10: tolerated pod failures before eventual success must be captured as RetryCount, computed from SuccessfulCreate events (not job.Status.Failed)")
		})

		It("UT-WE-054-JOB-026 [BR-WE-019 AC10]: should leave RetryCount at 0 when the Job succeeded with zero prior failures", func() {
			jobName := executor.ExecutionResourceName("default/deployment/clean-success")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Succeeded: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobComplete,
							Status:  corev1.ConditionTrue,
							Message: "Job completed successfully",
						},
					},
				},
			}
			// Exactly 1 "SuccessfulCreate" event (the initial, only attempt) =>
			// RetryCount = 1 - 1 = 0.
			events := []client.Object{
				newSuccessfulCreateEvent("clean-success-create-1", namespace, jobName, "pod-a", 1),
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).WithObjects(events...).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-clean-success", "default/deployment/clean-success", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Summary).ToNot(BeNil())
			Expect(result.Summary.RetryCount).To(Equal(int32(0)),
				"no spurious retryCount for the common zero-failure success path")
		})

		It("UT-WE-054-JOB-027 [BR-WE-019 AC10]: should leave RetryCount at 0 when no SuccessfulCreate events exist at all (defensive fallback)", func() {
			jobName := executor.ExecutionResourceName("default/deployment/no-events")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Succeeded: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobComplete,
							Status:  corev1.ConditionTrue,
							Message: "Job completed successfully",
						},
					},
				},
			}
			// No Events registered at all (e.g. expired past --event-ttl) --
			// must fail safe to 0, not error or panic.
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-no-events", "default/deployment/no-events", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

			result, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Summary).ToNot(BeNil())
			Expect(result.Summary.RetryCount).To(Equal(int32(0)),
				"missing/expired Events must fail safe to 0, not error (best-effort boundary)")
		})
	})

	Describe("Cleanup", func() {
		It("UT-WE-054-JOB-009: should delete Job owned by this WFE", func() {
			jobName := executor.ExecutionResourceName("default/deployment/cleanup")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: namespace,
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "wfe-cleanup",
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-cleanup", "default/deployment/cleanup", "")

			err := je.Cleanup(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())

			var deleted batchv1.Job
			err = fakeClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, &deleted)
			Expect(err).To(HaveOccurred())
		})

		It("UT-WE-054-JOB-010: should skip deletion when Job belongs to different WFE", func() {
			jobName := executor.ExecutionResourceName("default/deployment/shared")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: namespace,
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "wfe-other",
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-old", "default/deployment/shared", "")

			err := je.Cleanup(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())

			var existing batchv1.Job
			err = fakeClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, &existing)
			Expect(err).ToNot(HaveOccurred(), "Job belonging to different WFE should not be deleted")
		})

		It("UT-WE-054-JOB-011: should succeed when Job already deleted", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-gone", "default/deployment/gone", "")

			err := je.Cleanup(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("IsCompleted", func() {
		It("UT-WE-054-JOB-012: should return true for completed Job", func() {
			targetResource := "default/deployment/done"
			jobName := executor.ExecutionResourceName(targetResource)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			completed, err := je.IsCompleted(ctx, "prod-east", targetResource, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completed).To(BeTrue())
		})

		It("UT-WE-054-JOB-013: should return false for running Job", func() {
			targetResource := "default/deployment/active"
			jobName := executor.ExecutionResourceName(targetResource)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status:     batchv1.JobStatus{Active: 1},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			completed, err := je.IsCompleted(ctx, "", targetResource, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completed).To(BeFalse())
		})

		It("UT-WE-054-IC-001 [AC-3]: should route remote clusterID to ClientFactory", func() {
			targetResource := "default/deployment/remote-app"
			jobName := executor.ExecutionResourceName(targetResource)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &recordingClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			completed, err := je.IsCompleted(ctx, "prod-east", targetResource, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completed).To(BeTrue())
			Expect(factory.lastClusterID).To(Equal("prod-east"),
				"AC-3: IsCompleted must route to remote cluster, not hardcode empty clusterID")
		})

		It("UT-WE-054-IC-002 [AC-3]: should route empty clusterID to local client", func() {
			targetResource := "default/deployment/local-app"
			jobName := executor.ExecutionResourceName(targetResource)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			factory := &recordingClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)

			completed, err := je.IsCompleted(ctx, "", targetResource, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completed).To(BeTrue())
			Expect(factory.lastClusterID).To(Equal(""),
				"AC-3: IsCompleted with empty clusterID should use local client")
		})
	})
})
