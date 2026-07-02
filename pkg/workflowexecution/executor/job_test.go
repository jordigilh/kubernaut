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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
