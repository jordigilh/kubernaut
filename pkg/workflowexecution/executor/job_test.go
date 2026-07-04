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

		// BR-FLEET-054: regression for a cross-cluster Job dispatch failure
		// where the built Job's TypeMeta (apiVersion/kind) was left unset.
		// The local controller-runtime fake/real client tolerates this (it
		// infers the GVK from the scheme), but the MCP remote writer
		// serializes the object with runtime.DefaultUnstructuredConverter,
		// which drops empty apiVersion/kind (omitempty), causing the real
		// K8s MCP Server to reject the manifest with "Object 'Kind' is
		// missing". See Issue #1542 E2E-FLEET-014 CI failure.
		It("UT-WE-054-JOB-018 [BR-FLEET-054]: should set TypeMeta (Kind/APIVersion) on the Job before Create", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			spy := &gvkCapturingClient{ExecutorClient: fakeClient}
			factory := &mockClientFactory{client: spy}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-gvk-test", "default/deployment/nginx", "")

			_, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Expect(spy.createdGVK.Kind).To(Equal("Job"),
				"Job must carry an explicit Kind so remote MCP clients can decode the manifest")
			Expect(spy.createdGVK.GroupVersion().String()).To(Equal(batchv1.SchemeGroupVersion.String()))
		})
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
