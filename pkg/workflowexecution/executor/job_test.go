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
	"strings"
	"time"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// delayedDeleteClient models remote deletion: Delete acknowledges before the
// resource disappears, so the first subsequent Get still observes the object.
type delayedDeleteClient struct {
	executor.ExecutorClient
	deletePending int
	deletedObject client.Object
}

func (c *delayedDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.deletePending = 1
	c.deletedObject = obj
	return nil
}

func (c *delayedDeleteClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.deletePending > 0 {
		c.deletePending--
		return c.ExecutorClient.Get(ctx, key, obj, opts...)
	}
	if c.deletedObject != nil {
		if err := c.ExecutorClient.Delete(ctx, c.deletedObject); err != nil {
			return err
		}
		c.deletedObject = nil
	}
	return c.ExecutorClient.Get(ctx, key, obj, opts...)
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
				WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					WorkflowID:         "wf-001",
					Version:            "v1.0.0",
					ExecutionBundle:    "registry.example.com/workflow:v1",
					ServiceAccountName: "kubernaut-runner",
				},
			},
			TargetResource: targetResource,
			ClusterID:      clusterID,
			Parameters:     map[string]string{"TIMEOUT": "30s"},
		},
	}
}

// newSuccessfulCreateEvent builds a "SuccessfulCreate" Event on a Job object,
// matching what the real job-controller emits each time it creates a Pod
// (initial attempt or PodFailurePolicy-Ignore replacement). Used to simulate
// BR-WE-019 AC10's RetryCount computation without job.Status.Failed, which a
// real-cluster spike (DD-WE-008 Section 8) confirmed is never incremented
// for Ignore-action failures.
func newSuccessfulCreateEvent(name, namespace, jobName, podName string) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Job",
			Name:      jobName,
			Namespace: namespace,
		},
		Reason:  "SuccessfulCreate",
		Message: fmt.Sprintf("Created pod: %s", podName),
		Count:   1,
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

		It("UT-WE-2392-JOB-001 [BR-WE-014, AU-11]: omits Kubernetes TTL when failed execution retention is enabled", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-retention-enabled", "default/deployment/api", "")

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{
				RetainFailedExecutions: true,
			})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())
			Expect(job.Spec.TTLSecondsAfterFinished).To(BeNil())
		})

		It("UT-WE-2392-JOB-002 [BR-WE-014]: preserves the existing TTL when retention is disabled", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-retention-disabled", "default/deployment/api", "")

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())
			Expect(job.Spec.TTLSecondsAfterFinished).ToNot(BeNil())
			Expect(*job.Spec.TTLSecondsAfterFinished).To(Equal(int32(600)))
		})

		It("UT-WE-2390-001 [DD-WE-006, AC-6]: should mount secret and configmap dependencies read-only", func() {
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
			Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(And(
				HaveField("Name", "configmap-app-config"),
				HaveField("VolumeSource.ConfigMap.Name", "app-config"),
			)), "ConfigMap volume must be backed by the declared resource")

			mountPaths := make([]string, 0, len(job.Spec.Template.Spec.Containers[0].VolumeMounts))
			for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
				mountPaths = append(mountPaths, m.MountPath)
			}
			Expect(mountPaths).To(ContainElement(ContainSubstring("secrets/db-creds")))
			Expect(mountPaths).To(ContainElement(ContainSubstring("configmaps/app-config")))
			Expect(job.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(And(
				HaveField("Name", "secret-db-creds"),
				HaveField("ReadOnly", true),
			)), "Secret dependency must be mounted read-only")
			Expect(job.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(And(
				HaveField("Name", "configmap-app-config"),
				HaveField("ReadOnly", true),
			)), "ConfigMap dependency must be mounted read-only")
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
		// and limits come from WFE.Spec.WorkflowRef.Resources, the immutable
		// CRD-embedded snapshot (Issue #1661 Change 11f).
		It("UT-WE-2390-001 [BR-WE-019]: should apply WFE.Spec.WorkflowRef.Resources to the workflow container", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-resources", "default/deployment/nginx", "")
			wfe.Spec.WorkflowRef.Resources = &corev1.ResourceRequirements{
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

		It("UT-WE-054-JOB-024 [BR-WE-019]: should leave the workflow container BestEffort when WFE.Spec.WorkflowRef.Resources is nil (backward compat)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-no-resources", "default/deployment/nginx", "")
			Expect(wfe.Spec.WorkflowRef.Resources).To(BeNil())

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
			Expect(job.Spec.PodReplacementPolicy).ToNot(BeNil())
			Expect(*job.Spec.PodReplacementPolicy).To(Equal(batchv1.Failed),
				"BR-WE-019: PodFailurePolicy requires the Failed pod replacement policy")
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
		It("UT-WE-054-JOB-016 [BR-WORKFLOW-008]: should default ActiveDeadlineSeconds to 30m when Spec.TimesOutAt is unset", func() {
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

		It("UT-WE-054-JOB-017 [BR-WORKFLOW-008]: should source ActiveDeadlineSeconds from Spec.TimesOutAt when set (DD-TIMEOUT-002)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-deadline-custom", "default/deployment/nginx", "")
			deadline := metav1.NewTime(time.Now().Add(5 * time.Minute))
			wfe.Spec.TimesOutAt = &deadline

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			Expect(job.Spec.ActiveDeadlineSeconds).ToNot(BeNil())
			// DD-TIMEOUT-002: Spec.TimesOutAt is an absolute deadline, converted
			// to a relative ActiveDeadlineSeconds via time.Until at Job-creation
			// time, so allow a small tolerance for test execution time instead
			// of asserting an exact value.
			Expect(*job.Spec.ActiveDeadlineSeconds).To(BeNumerically("~", 5*60, 2),
				"deadline should be sourced from WFE Spec.TimesOutAt when declared")
		})

		It("UT-WE-2176-001 [DD-TIMEOUT-002]: should floor ActiveDeadlineSeconds to 1 when Spec.TimesOutAt has already passed", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			je := executor.NewJobExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-deadline-past", "default/deployment/nginx", "")
			pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			wfe.Spec.TimesOutAt = &pastDeadline

			result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &job)).To(Succeed())

			Expect(job.Spec.ActiveDeadlineSeconds).ToNot(BeNil())
			Expect(*job.Spec.ActiveDeadlineSeconds).To(Equal(int64(1)),
				"an already-past Spec.TimesOutAt must still produce a strictly-positive K8s Job deadline, not zero/negative")
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
		It("UT-WE-2390-002 [BR-WORKFLOW-008]: should enrich Failed message with FailedMount Pod event detail", func() {
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
				newSuccessfulCreateEvent("retried-create-1", namespace, jobName, "pod-a"),
				newSuccessfulCreateEvent("retried-create-2", namespace, jobName, "pod-b"),
				newSuccessfulCreateEvent("retried-create-3", namespace, jobName, "pod-c"),
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
				newSuccessfulCreateEvent("clean-success-create-1", namespace, jobName, "pod-a"),
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
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

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
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}

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
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: executor.ExecutionResourceName(wfe.Spec.TargetResource)}

			err := je.Cleanup(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-WE-054-JOB-014 [BR-FLEET-054]: should wait for remote Job deletion to complete", func() {
			jobName := executor.ExecutionResourceName("default/deployment/remote-cleanup")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: namespace,
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "wfe-remote-cleanup",
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			remoteClient := &delayedDeleteClient{ExecutorClient: fakeClient}
			factory := &mockClientFactory{client: remoteClient}
			je := executor.NewJobExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-remote-cleanup", "default/deployment/remote-cleanup", "remote-cluster")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: jobName}
			Expect(je.Cleanup(ctx, wfe, namespace)).To(Succeed())

			var deleted batchv1.Job
			err := fakeClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, &deleted)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
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

// UT-WE-2512: Job identity must follow the immutable RR/WFE execution identity,
// not the target resource. BR-WE-014/011, FedRAMP AU-3/AU-9, SOC2 CC7.2,
// OWASP ASVS V4.1.3/V5.1.x.
var _ = Describe("UT-WE-2512: per-RR Job identity", func() {
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

	It("UT-WE-2512-001: includes the RR name and bounded WFE suffix in a valid Job name", func() {
		wfe := newTestWFE("wfe-rr-2512-a", "default/deployment/same-target", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174000")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-5da327bdcae1-702732e1",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440000"),
		}

		name := executor.JobExecutionResourceName(wfe)

		Expect(name).To(HavePrefix("wfe-" + wfe.Spec.RemediationRequestRef.Name + "-"))
		Expect(name).To(Equal("wfe-rr-5da327bdcae1-702732e1-4000"))

		otherRR := wfe.DeepCopy()
		otherRR.Spec.RemediationRequestRef.Name = "rr-5da327bdcae1-a7319216"
		Expect(executor.JobExecutionResourceName(otherRR)).NotTo(Equal(name),
			"different RR names must never reuse a Job identity for the same target")
	})

	It("UT-WE-2512-006: bounds long metadata labels and keeps full provenance in annotations", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})
		longWFEName := "wfe-" + strings.Repeat("long-name-", 20)
		longTarget := "default/deployment/" + strings.Repeat("target-", 20)
		longRRName := "rr-" + strings.Repeat("remediation-", 20)
		secretOutput := "secret-workflow-output-must-not-be-a-label"
		wfe := newTestWFE(longWFEName, longTarget, "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174006")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      longRRName,
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440006"),
		}
		wfe.Spec.Parameters = map[string]string{"workflow_output": secretOutput}

		created, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		job := &batchv1.Job{}
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: created.ResourceName, Namespace: namespace}, job)).To(Succeed())

		Expect(len(job.Name)).To(BeNumerically("<=", 63))
		Expect(job.Name).To(Equal(executor.JobExecutionResourceName(wfe)))
		Expect(job.Labels["kubernaut.ai/workflow-execution"]).To(HaveLen(63))
		Expect(job.Labels["kubernaut.ai/workflow-execution"]).NotTo(ContainSubstring(secretOutput))
		Expect(job.Labels["kubernaut.ai/target-resource"]).To(HaveLen(63))
		Expect(job.Annotations["kubernaut.ai/workflow-execution"]).To(Equal(longWFEName))
		Expect(job.Annotations["kubernaut.ai/target-resource"]).To(Equal(longTarget))
		Expect(job.Annotations["kubernaut.ai/remediation-request"]).To(Equal(longRRName))
		Expect(je.Cleanup(ctx, wfe, namespace)).To(Succeed(),
			"full WFE ownership annotation must authorize cleanup when the label is truncated")
	})

	It("UT-WE-2512-002: re-creating the same WFE is idempotent and does not create a second Job", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})
		wfe := newTestWFE("wfe-rr-2512-idempotent", "default/deployment/idempotent", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174001")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-2512-idempotent",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440002"),
		}

		first, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		second, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.ResourceName).To(Equal(first.ResourceName))

		jobs := &batchv1.JobList{}
		Expect(fakeClient.List(ctx, jobs, client.InNamespace(namespace))).To(Succeed())
		Expect(jobs.Items).To(HaveLen(1), "a same-WFE retry must not create a second Job")
	})

	It("UT-WE-2512-003: projects the exact bundle and RR/WFE provenance metadata", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})
		wfe := newTestWFE("wfe-rr-2512-metadata", "default/deployment/metadata", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174003")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-2512-metadata",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440003"),
		}
		wfe.Spec.WorkflowRef.ExecutionBundle = "registry.example.com/workflows/standard:v2@sha256:bundle-b"

		result, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		job := &batchv1.Job{}
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, job)).To(Succeed())
		Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
		Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal(wfe.Spec.WorkflowRef.ExecutionBundle))
		Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution", wfe.Name))
		Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution-uid", string(wfe.UID)))
		Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", wfe.Spec.RemediationRequestRef.Name))
		Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request-namespace", wfe.Spec.RemediationRequestRef.Namespace))
		Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request-uid", string(wfe.Spec.RemediationRequestRef.UID)))
		Expect(job.Spec.Template.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution-uid", string(wfe.UID)))
		Expect(job.Spec.Template.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request-uid", string(wfe.Spec.RemediationRequestRef.UID)))
		Expect(job.Spec.Template.Annotations).To(HaveKeyWithValue("kubernaut.ai/remediation-request-uid", string(wfe.Spec.RemediationRequestRef.UID)))
	})

	It("UT-WE-2512-005: retention policy changes do not change Job identity or image", func() {
		wfe := newTestWFE("wfe-rr-2512-retention", "default/deployment/retention", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174005")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-2512-retention",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440005"),
		}
		wfe.Spec.WorkflowRef.ExecutionBundle = "registry.example.com/workflows/retained@sha256:bundle"
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})

		retained, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{RetainFailedExecutions: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(retained.ResourceName).To(Equal(executor.JobExecutionResourceName(wfe)))
		Expect(je.Cleanup(ctx, wfe, namespace)).To(Succeed())

		// Use a fresh client because the first assertion intentionally exercises
		// cleanup; the identity helper must remain independent of TTL policy.
		freshClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		je = executor.NewJobExecutorWithFactory(&mockClientFactory{client: freshClient})
		standard, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(standard.ResourceName).To(Equal(retained.ResourceName))
		var job batchv1.Job
		Expect(freshClient.Get(ctx, client.ObjectKey{Name: standard.ResourceName, Namespace: namespace}, &job)).To(Succeed())
		Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal(wfe.Spec.WorkflowRef.ExecutionBundle))
	})

	It("UT-WE-2512-004: cleanup follows Status.ExecutionRef instead of target-derived identity", func() {
		wfe := newTestWFE("wfe-rr-2512-cleanup", "default/deployment/cleanup", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174004")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-2512-cleanup",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440004"),
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})
		created, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: created.ResourceName}

		Expect(je.Cleanup(ctx, wfe, namespace)).To(Succeed())
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: created.ResourceName, Namespace: namespace}, &batchv1.Job{})).To(HaveOccurred())
	})

	It("UT-WE-2512-007: fails closed on a foreign Job occupying the expected identity", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})
		wfe := newTestWFE("wfe-rr-2512-foreign", "default/deployment/foreign", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174007")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-2512-foreign",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440007"),
		}
		jobName := executor.JobExecutionResourceName(wfe)
		foreign := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"kubernaut.ai/workflow-execution":     "wfe-foreign-owner",
				"kubernaut.ai/workflow-execution-uid": "123e4567-e89b-12d3-a456-426614174099",
			},
		}, Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "workflow", Image: "registry.example.com/workflows/foreign:v9"}},
		}}}}
		Expect(fakeClient.Create(ctx, foreign)).To(Succeed())
		Expect(je.IsExpectedJob(ctx, wfe, foreign, namespace)).To(BeFalse(),
			"foreign same-name Jobs must not suppress workflow-selection audit idempotency")

		_, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).To(HaveOccurred(), "a foreign same-name Job must not be adopted")

		var remaining batchv1.Job
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, &remaining)).To(Succeed())
		Expect(remaining.Spec.Template.Spec.Containers[0].Image).To(Equal("registry.example.com/workflows/foreign:v9"),
			"the foreign Job must not be deleted or replaced")
	})

	It("UT-WE-2512-010: fails closed when a same-image Job has a different execution template", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})
		wfe := newTestWFE("wfe-rr-2512-template", "default/deployment/template", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174010")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-2512-template",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440010"),
		}
		created, err := je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		foreign := &batchv1.Job{}
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: created.ResourceName, Namespace: namespace}, foreign)).To(Succeed())
		Expect(fakeClient.Delete(ctx, foreign)).To(Succeed())
		foreign.ResourceVersion = ""
		foreign.UID = ""
		foreign.CreationTimestamp = metav1.Time{}
		foreign.Spec.Template.Spec.Containers[0].Env = append(foreign.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{Name: "TAMPERED_TEMPLATE", Value: "true"})
		Expect(fakeClient.Create(ctx, foreign)).To(Succeed())

		_, err = je.Create(ctx, wfe, namespace, executor.CreateOptions{})
		Expect(err).To(HaveOccurred(), "same-name same-image template mutation must fail closed")
		remaining := &batchv1.Job{}
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: created.ResourceName, Namespace: namespace}, remaining)).To(Succeed())
		Expect(remaining.Spec.Template.Spec.Containers[0].Env).To(ContainElement(
			HaveField("Name", "TAMPERED_TEMPLATE"),
		))
	})

	It("UT-WE-2512-011: reads a legacy target-named Job through an existing WFE execution reference", func() {
		wfe := newTestWFE("wfe-rr-2512-legacy-status", "default/deployment/legacy-status", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174011")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-2512-legacy-status",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440011"),
		}
		legacyName := executor.ExecutionResourceName(wfe.Spec.TargetResource)
		wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: legacyName}
		legacyJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      legacyName,
				Namespace: namespace,
				Labels: map[string]string{
					"kubernaut.ai/workflow-execution": wfe.Name,
					"kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
					"kubernaut.ai/source-namespace":   wfe.Namespace,
					"kubernaut.ai/execution-engine":   "job",
				},
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "workflow", Image: wfe.Spec.WorkflowRef.ExecutionBundle}},
			}}},
			Status: batchv1.JobStatus{
				Succeeded:  1,
				Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacyJob).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})

		result, err := je.GetStatus(ctx, wfe, namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))
	})

	It("UT-WE-2512-019: reads an existing RR-UID-named Job through its recorded reference", func() {
		wfe := newTestWFE("wfe-rr-2512-legacy-uid", "default/deployment/legacy-uid", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174019")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-5da327bdcae1-legacyuid",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440019"),
		}
		legacyName := "wfe-550e8400-e29b-41d4-a716-446655440019-4019"
		wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: legacyName}
		legacyJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      legacyName,
				Namespace: namespace,
				Labels: map[string]string{
					"kubernaut.ai/workflow-execution":            wfe.Name,
					"kubernaut.ai/workflow-execution-uid":        string(wfe.UID),
					"kubernaut.ai/remediation-request":           wfe.Spec.RemediationRequestRef.Name,
					"kubernaut.ai/remediation-request-namespace": wfe.Spec.RemediationRequestRef.Namespace,
					"kubernaut.ai/remediation-request-uid":       string(wfe.Spec.RemediationRequestRef.UID),
					"kubernaut.ai/workflow-id":                   wfe.Spec.WorkflowRef.WorkflowID,
					"kubernaut.ai/source-namespace":              wfe.Namespace,
					"kubernaut.ai/execution-engine":              "job",
				},
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "workflow", Image: wfe.Spec.WorkflowRef.ExecutionBundle}},
			}}},
			Status: batchv1.JobStatus{
				Succeeded:  1,
				Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacyJob).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})

		result, err := je.GetStatus(ctx, wfe, namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))
	})

	It("UT-WE-2512-012: cleans up a legacy target-named Job through its recorded reference", func() {
		wfe := newTestWFE("wfe-rr-2512-legacy-cleanup", "default/deployment/legacy-cleanup", "")
		wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174012")
		wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
			Name:      "rr-2512-legacy-cleanup",
			Namespace: "kubernaut-system",
			UID:       types.UID("550e8400-e29b-41d4-a716-446655440012"),
		}
		legacyName := executor.ExecutionResourceName(wfe.Spec.TargetResource)
		wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: legacyName}
		legacyJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      legacyName,
				Namespace: namespace,
				Labels: map[string]string{
					"kubernaut.ai/workflow-execution": wfe.Name,
					"kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
					"kubernaut.ai/source-namespace":   wfe.Namespace,
					"kubernaut.ai/execution-engine":   "job",
				},
			},
			Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "workflow", Image: wfe.Spec.WorkflowRef.ExecutionBundle}},
			}}},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacyJob).Build()
		je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})

		Expect(je.Cleanup(ctx, wfe, namespace)).To(Succeed())
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: legacyName, Namespace: namespace}, &batchv1.Job{})).To(HaveOccurred())
	})

	It("UT-WE-2512-015: rejects legacy status from a wrong owner or image", func() {
		cases := []struct {
			name          string
			owner         string
			image         string
			expectedImage string
		}{
			{name: "wrong-owner", owner: "other-wfe", image: "registry.example.com/workflow:v1", expectedImage: "registry.example.com/workflow:v1"},
			{name: "wrong-image", owner: "wfe-rr-2512-legacy-negative", image: "registry.example.com/foreign:v9", expectedImage: "registry.example.com/workflow:v1"},
		}
		for _, tc := range cases {
			wfe := newTestWFE("wfe-rr-2512-legacy-negative", "default/deployment/legacy-negative-"+tc.name, "")
			wfe.UID = types.UID("123e4567-e89b-12d3-a456-426614174015")
			wfe.Spec.RemediationRequestRef = corev1.ObjectReference{
				Name: "rr-2512-legacy-negative", Namespace: "kubernaut-system",
				UID: types.UID("550e8400-e29b-41d4-a716-446655440015"),
			}
			wfe.Spec.WorkflowRef.ExecutionBundle = tc.expectedImage
			legacyName := executor.ExecutionResourceName(wfe.Spec.TargetResource)
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: legacyName}
			legacyJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: legacyName, Namespace: namespace,
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": tc.owner,
						"kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
						"kubernaut.ai/source-namespace":   wfe.Namespace,
						"kubernaut.ai/execution-engine":   "job",
					},
				},
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "workflow", Image: tc.image}},
				}}},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacyJob).Build()
			je := executor.NewJobExecutorWithFactory(&mockClientFactory{client: fakeClient})

			_, err := je.GetStatus(ctx, wfe, namespace)
			Expect(err).To(HaveOccurred(), tc.name+": legacy mismatch must fail closed")
		}
	})
})
