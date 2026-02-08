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

package workflowexecution

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// ========================================
// BR-WE-014: Executor Unit Tests
// Tests: Registry, JobExecutor, TektonExecutor, ExecutionResourceName
// ========================================

var _ = Describe("Executor Registry (BR-WE-014)", func() {
	var registry *executor.Registry

	BeforeEach(func() {
		registry = executor.NewRegistry()
	})

	Context("Registry operations", func() {
		It("should return error for unregistered engine (UT-WE-014-001)", func() {
			_, err := registry.Get("unknown")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported execution engine"))
		})

		It("should register and retrieve executors (UT-WE-014-002)", func() {
			scheme := runtime.NewScheme()
			_ = batchv1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			registry.Register("tekton", executor.NewTektonExecutor(fakeClient, ""))
			registry.Register("job", executor.NewJobExecutor(fakeClient, ""))

			tektonExec, err := registry.Get("tekton")
			Expect(err).ToNot(HaveOccurred())
			Expect(tektonExec.Engine()).To(Equal("tekton"))

			jobExec, err := registry.Get("job")
			Expect(err).ToNot(HaveOccurred())
			Expect(jobExec.Engine()).To(Equal("job"))
		})

		It("should list registered engines (UT-WE-014-003)", func() {
			scheme := runtime.NewScheme()
			_ = batchv1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			registry.Register("tekton", executor.NewTektonExecutor(fakeClient, ""))
			registry.Register("job", executor.NewJobExecutor(fakeClient, ""))

			engines := registry.Engines()
			Expect(engines).To(HaveLen(2))
			Expect(engines).To(ContainElements("tekton", "job"))
		})
	})
})

var _ = Describe("ExecutionResourceName (BR-WE-014)", func() {
	It("should generate deterministic names (UT-WE-014-010)", func() {
		name1 := executor.ExecutionResourceName("default/deployment/my-app")
		name2 := executor.ExecutionResourceName("default/deployment/my-app")
		Expect(name1).To(Equal(name2))
	})

	It("should have wfe- prefix (UT-WE-014-011)", func() {
		name := executor.ExecutionResourceName("default/deployment/my-app")
		Expect(name).To(HavePrefix("wfe-"))
	})

	It("should not exceed 63 characters (UT-WE-014-012)", func() {
		name := executor.ExecutionResourceName("very-long-namespace/deployment/very-long-deployment-name-exceeds")
		Expect(len(name)).To(BeNumerically("<=", 63))
	})

	It("should generate different names for different targets (UT-WE-014-013)", func() {
		name1 := executor.ExecutionResourceName("ns1/deployment/app1")
		name2 := executor.ExecutionResourceName("ns2/deployment/app2")
		Expect(name1).ToNot(Equal(name2))
	})
})

var _ = Describe("JobExecutor (BR-WE-014)", func() {
	var (
		jobExec   *executor.JobExecutor
		k8sClient client.Client
		scheme    *runtime.Scheme
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = batchv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = workflowexecutionv1alpha1.AddToScheme(scheme)
	})

	Context("Create", func() {
		It("should create a Job with correct spec (UT-WE-014-020)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "job",
					TargetResource:  "default/deployment/my-app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "restart-deployment",
						ContainerImage: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
					},
					Parameters: map[string]string{
						"NAMESPACE": "default",
					},
				},
			}

			name, err := jobExec.Create(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(HavePrefix("wfe-"))

			// Verify the Job was created
			var job batchv1.Job
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &job)
			Expect(err).ToNot(HaveOccurred())

			// Verify Job spec
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal("ghcr.io/kubernaut/workflows/restart:v1.0.0"))
			Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("test-sa"))
			Expect(job.Spec.Template.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))

			// Verify labels
			Expect(job.Labels["kubernaut.ai/workflow-execution"]).To(Equal("test-wfe"))
			Expect(job.Labels["kubernaut.ai/execution-engine"]).To(Equal("job"))

			// Verify env vars include TARGET_RESOURCE and parameters
			envVars := job.Spec.Template.Spec.Containers[0].Env
			var foundTarget, foundNamespace bool
			for _, env := range envVars {
				if env.Name == "TARGET_RESOURCE" {
					foundTarget = true
					Expect(env.Value).To(Equal("default/deployment/my-app"))
				}
				if env.Name == "NAMESPACE" {
					foundNamespace = true
					Expect(env.Value).To(Equal("default"))
				}
			}
			Expect(foundTarget).To(BeTrue(), "TARGET_RESOURCE env var should be present")
			Expect(foundNamespace).To(BeTrue(), "NAMESPACE env var should be present")
		})

		It("should use default service account when not specified (UT-WE-014-021)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-default-sa",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "job",
					TargetResource:  "default/deployment/another-app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "restart-deployment",
						ContainerImage: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
					},
				},
			}

			name, err := jobExec.Create(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())

			var job batchv1.Job
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &job)
			Expect(err).ToNot(HaveOccurred())
			Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal(executor.DefaultServiceAccountName))
		})
	})

	Context("GetStatus", func() {
		It("should return Running for active Job (UT-WE-014-030)", func() {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "kubernaut-workflows",
				},
				Status: batchv1.JobStatus{
					Active: 1,
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: "test-job"},
				},
			}

			result, err := jobExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning))
		})

		It("should return Completed for succeeded Job (UT-WE-014-031)", func() {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job-complete",
					Namespace: "kubernaut-workflows",
				},
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
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: "test-job-complete"},
				},
			}

			result, err := jobExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))
		})

		It("should return Failed for failed Job (UT-WE-014-032)", func() {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job-failed",
					Namespace: "kubernaut-workflows",
				},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job has reached backoff limit",
						},
					},
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: "test-job-failed"},
				},
			}

			result, err := jobExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Reason).To(Equal("BackoffLimitExceeded"))
		})

		It("should return error when ExecutionRef is nil (UT-WE-014-033)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-no-ref",
					Namespace: "default",
				},
			}

			_, err := jobExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no execution ref set"))
		})
	})

	Context("Cleanup", func() {
		It("should delete existing Job (UT-WE-014-040)", func() {
			targetResource := "default/deployment/cleanup-app"
			jobName := executor.ExecutionResourceName(targetResource)

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: "kubernaut-workflows",
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "job",
					TargetResource:  targetResource,
				},
			}

			err := jobExec.Cleanup(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())

			// Verify Job was deleted
			var deletedJob batchv1.Job
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      jobName,
				Namespace: "kubernaut-workflows",
			}, &deletedJob)
			Expect(err).To(HaveOccurred()) // Should be NotFound
		})

		It("should be idempotent when Job doesn't exist (UT-WE-014-041)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "job",
					TargetResource:  "default/deployment/nonexistent",
				},
			}

			err := jobExec.Cleanup(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Engine identification", func() {
		It("should return 'job' as engine (UT-WE-014-050)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			jobExec = executor.NewJobExecutor(k8sClient, "")
			Expect(jobExec.Engine()).To(Equal("job"))
		})
	})
})
