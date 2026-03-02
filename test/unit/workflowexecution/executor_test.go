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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
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
						ExecutionBundle: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
					},
					Parameters: map[string]string{
						"NAMESPACE": "default",
					},
				},
			}

			name, err := jobExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
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
						ExecutionBundle: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
					},
				},
			}

			name, err := jobExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
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

// ========================================
// BR-WE-014: TektonExecutor Unit Tests
// Coverage target: TektonExecutor.Create, GetStatus, Cleanup,
//                  buildPipelineRun, buildStatusSummary, convertParameters
// Test Plan: docs/testing/BR-WE-014/UNIT_TEST_PLAN_TEKTON_EXECUTOR.md
// ========================================

var _ = Describe("TektonExecutor (BR-WE-014)", func() {
	var (
		tektonExec *executor.TektonExecutor
		k8sClient  client.Client
		scheme     *runtime.Scheme
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================
	// Create tests (UT-WE-014-060 to 065)
	// ========================================

	Context("Create", func() {
		It("should create a PipelineRun with correct spec (UT-WE-014-060)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := newTestWFE("test-wfe-tekton", "default", "default/deployment/my-app",
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0",
				map[string]string{"NAMESPACE": "default"})

			name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(HavePrefix("wfe-"))

			// Verify the PipelineRun was created
			var pr tektonv1.PipelineRun
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())

			// Verify bundle resolver
			Expect(pr.Spec.PipelineRef).To(And(Not(BeNil()), HaveField("Resolver", Equal(tektonv1.ResolverName("bundles")))))
			bundleParams := pr.Spec.PipelineRef.Params
			Expect(bundleParams).To(HaveLen(3))

			// Verify bundle param "bundle" = container image
			var foundBundle bool
			for _, p := range bundleParams {
				if p.Name == "bundle" {
					foundBundle = true
					Expect(p.Value.StringVal).To(Equal("ghcr.io/kubernaut/workflows/restart:v1.0.0"))
				}
			}
			Expect(foundBundle).To(BeTrue(), "bundle param should be present in resolver")

			// Verify service account
			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("test-sa"))

			// Verify deterministic name matches ExecutionResourceName
			expectedName := executor.ExecutionResourceName("default/deployment/my-app")
			Expect(name).To(Equal(expectedName))
		})

		It("should use default service account when not specified (UT-WE-014-061)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "")

			wfe := newTestWFE("test-wfe-default-sa", "default", "default/deployment/another-app",
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0", nil)

			name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())
			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal(executor.DefaultServiceAccountName))
		})

		It("should convert parameters to Tekton params and inject TARGET_RESOURCE (UT-WE-014-062)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := newTestWFE("test-wfe-params", "default", "default/deployment/param-app",
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0",
				map[string]string{"NAMESPACE": "default", "REPLICAS": "3"})

			name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())

			// Should have NAMESPACE, REPLICAS, and TARGET_RESOURCE (3 total)
			params := pr.Spec.Params
			Expect(len(params)).To(BeNumerically(">=", 3))

			paramMap := make(map[string]string)
			for _, p := range params {
				paramMap[p.Name] = p.Value.StringVal
			}

			Expect(paramMap).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(paramMap).To(HaveKeyWithValue("REPLICAS", "3"))
			Expect(paramMap).To(HaveKeyWithValue("TARGET_RESOURCE", "default/deployment/param-app"))
		})

		It("should produce only TARGET_RESOURCE when parameters are empty (UT-WE-014-063)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := newTestWFE("test-wfe-no-params", "default", "default/deployment/no-param-app",
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0", nil)

			name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())

			// Only TARGET_RESOURCE should be present
			Expect(pr.Spec.Params).To(HaveLen(1))
			Expect(pr.Spec.Params[0].Name).To(Equal("TARGET_RESOURCE"))
			Expect(pr.Spec.Params[0].Value.StringVal).To(Equal("default/deployment/no-param-app"))
		})

		It("should set labels carrying WFE metadata for observability (UT-WE-014-064)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := newTestWFE("test-wfe-labels", "prod-ns", "prod-ns/deployment/api-server",
				"scale-up", "ghcr.io/kubernaut/workflows/scale:v2.0.0", nil)

			name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())

			// Verify all required labels
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution", "test-wfe-labels"))
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-id", "scale-up"))
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/source-namespace", "prod-ns"))
			Expect(pr.Labels).To(HaveKey("kubernaut.ai/target-resource"))

			// Verify annotation carries full (unsanitized) target resource
			Expect(pr.Annotations).To(HaveKeyWithValue("kubernaut.ai/target-resource", "prod-ns/deployment/api-server"))
		})

		It("should preserve AlreadyExists error for controller retry logic (UT-WE-014-065)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := newTestWFE("test-wfe-dup", "default", "default/deployment/dup-app",
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0", nil)

			// First create should succeed
			_, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Second create with same WFE (same target resource → same name) should fail
			_, err = tektonExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).To(HaveOccurred())
			// The error should be usable with apierrors.IsAlreadyExists
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})
	})

	// ========================================
	// GetStatus tests (UT-WE-014-070 to 075)
	// ========================================

	Context("GetStatus", func() {
		It("should return PhaseCompleted for succeeded PipelineRun (UT-WE-014-070)", func() {
			prName := "test-pr-completed"
			pr := newTestPipelineRun(prName, "kubernaut-workflows",
				corev1.ConditionTrue, "Succeeded", "Pipeline completed successfully", 2)

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: prName},
				},
			}

			result, err := tektonExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))
			Expect(result.Reason).To(Equal("Succeeded"))
			Expect(result.Message).To(Equal("Pipeline completed successfully"))
			Expect(result.Summary).To(Not(BeNil()), "GetStatus must populate Summary for completed PipelineRun")
		})

		It("should return PhaseFailed for failed PipelineRun (UT-WE-014-071)", func() {
			prName := "test-pr-failed"
			pr := newTestPipelineRun(prName, "kubernaut-workflows",
				corev1.ConditionFalse, "PipelineRunFailed", "Task 'validate' failed", 3)

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: prName},
				},
			}

			result, err := tektonExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Reason).To(Equal("PipelineRunFailed"))
			Expect(result.Message).To(Equal("Task 'validate' failed"))
		})

		It("should return PhaseRunning for running PipelineRun with Unknown condition (UT-WE-014-072)", func() {
			prName := "test-pr-running"
			pr := newTestPipelineRun(prName, "kubernaut-workflows",
				corev1.ConditionUnknown, "Running", "Tasks Completed: 1 (Incomplete)", 2)

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: prName},
				},
			}

			result, err := tektonExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning))
			Expect(result.Reason).To(Equal("Running"))
			Expect(result.Message).To(ContainSubstring("Pipeline executing"))
		})

		It("should return PhaseRunning when no condition exists yet (UT-WE-014-073)", func() {
			// PipelineRun just created, no conditions yet
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr-no-condition",
					Namespace: "kubernaut-workflows",
				},
				// Status with no conditions
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: "test-pr-no-condition"},
				},
			}

			result, err := tektonExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning))
			Expect(result.Reason).To(Equal("Pending"))
			Expect(result.Message).To(ContainSubstring("waiting for Tekton"))
		})

		It("should return error when ExecutionRef is nil (UT-WE-014-074)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-no-ref",
					Namespace: "default",
				},
			}

			_, err := tektonExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no execution ref set"))
		})

		It("should populate TotalTasks from ChildReferences count (UT-WE-014-075)", func() {
			prName := "test-pr-with-children"
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
				},
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							{
								Type:    apis.ConditionSucceeded,
								Status:  corev1.ConditionTrue,
								Reason:  "Succeeded",
								Message: "All tasks completed",
							},
						},
					},
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						ChildReferences: []tektonv1.ChildStatusReference{
							{Name: "task-1"},
							{Name: "task-2"},
							{Name: "task-3"},
						},
					},
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: prName},
				},
			}

			result, err := tektonExec.GetStatus(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Summary).To(And(Not(BeNil()), HaveField("TotalTasks", Equal(3))))
			Expect(result.Summary.Status).To(Equal(string(corev1.ConditionTrue)))
			Expect(result.Summary.Reason).To(Equal("Succeeded"))
		})
	})

	// ========================================
	// Cleanup tests (UT-WE-014-080 to 082)
	// ========================================

	Context("Cleanup", func() {
		It("should delete existing PipelineRun (UT-WE-014-080)", func() {
			targetResource := "default/deployment/cleanup-app"
			prName := executor.ExecutionResourceName(targetResource)

			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "tekton",
					TargetResource:  targetResource,
				},
			}

			err := tektonExec.Cleanup(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())

			// Verify PipelineRun was deleted
			var deletedPR tektonv1.PipelineRun
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      prName,
				Namespace: "kubernaut-workflows",
			}, &deletedPR)
			Expect(err).To(HaveOccurred()) // Should be NotFound
		})

		It("should be idempotent when PipelineRun doesn't exist (UT-WE-014-081)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "tekton",
					TargetResource:  "default/deployment/nonexistent",
				},
			}

			err := tektonExec.Cleanup(ctx, wfe, "kubernaut-workflows")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Engine identification", func() {
		It("should return 'tekton' as engine (UT-WE-014-082)", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec = executor.NewTektonExecutor(k8sClient, "")
			Expect(tektonExec.Engine()).To(Equal("tekton"))
		})
	})
})

// ========================================
// BR-WE-014: Utility Function Unit Tests
// convertParameters, sanitizeLabelValue
// ========================================

var _ = Describe("Utility Functions (BR-WE-014)", func() {
	Context("convertParameters", func() {
		It("should convert all key-value pairs to Tekton Params (UT-WE-014-085)", func() {
			params := map[string]string{
				"NAMESPACE": "default",
				"REPLICAS":  "3",
				"IMAGE":     "nginx:latest",
			}

			// Exercise convertParameters indirectly via Create (it's unexported)
			// We verify the params appear on the created PipelineRun
			scheme := runtime.NewScheme()
			Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec := executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := newTestWFE("test-wfe-convert", "default", "default/deployment/convert-app",
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0", params)

			name, err := tektonExec.Create(context.Background(), wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = k8sClient.Get(context.Background(), client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())

			// Should have 3 user params + 1 TARGET_RESOURCE = 4
			Expect(len(pr.Spec.Params)).To(Equal(4))

			paramMap := make(map[string]string)
			for _, p := range pr.Spec.Params {
				paramMap[p.Name] = p.Value.StringVal
			}
			Expect(paramMap).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(paramMap).To(HaveKeyWithValue("REPLICAS", "3"))
			Expect(paramMap).To(HaveKeyWithValue("IMAGE", "nginx:latest"))
			Expect(paramMap).To(HaveKey("TARGET_RESOURCE"))
		})

		It("should return empty slice for empty map (UT-WE-014-086)", func() {
			// Empty params should produce only TARGET_RESOURCE
			scheme := runtime.NewScheme()
			Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec := executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := newTestWFE("test-wfe-empty-params", "default", "default/deployment/empty-app",
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0",
				map[string]string{})

			name, err := tektonExec.Create(context.Background(), wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = k8sClient.Get(context.Background(), client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())

			// Only TARGET_RESOURCE should be present
			Expect(pr.Spec.Params).To(HaveLen(1))
			Expect(pr.Spec.Params[0].Name).To(Equal("TARGET_RESOURCE"))
		})
	})

	Context("sanitizeLabelValue", func() {
		It("should replace slashes with double-underscores (UT-WE-014-087)", func() {
			// Exercise sanitizeLabelValue indirectly via Create label output
			scheme := runtime.NewScheme()
			Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec := executor.NewTektonExecutor(k8sClient, "test-sa")

			wfe := newTestWFE("test-wfe-label-sanitize", "default", "prod-ns/deployment/api-server",
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0", nil)

			name, err := tektonExec.Create(context.Background(), wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = k8sClient.Get(context.Background(), client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())

			// Target resource "prod-ns/deployment/api-server" should become "prod-ns__deployment__api-server"
			Expect(pr.Labels["kubernaut.ai/target-resource"]).To(Equal("prod-ns__deployment__api-server"))
		})

		It("should truncate label values to 63 characters (UT-WE-014-088)", func() {
			// Exercise sanitizeLabelValue truncation via Create
			scheme := runtime.NewScheme()
			Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			tektonExec := executor.NewTektonExecutor(k8sClient, "test-sa")

			// Create a target resource that, when sanitized, exceeds 63 chars
			longTarget := "very-long-namespace/deployment/very-long-deployment-name-that-exceeds-sixty-three-chars"
			wfe := newTestWFE("test-wfe-label-truncate", "default", longTarget,
				"restart-deployment", "ghcr.io/kubernaut/workflows/restart:v1.0.0", nil)

			name, err := tektonExec.Create(context.Background(), wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = k8sClient.Get(context.Background(), client.ObjectKey{
				Name:      name,
				Namespace: "kubernaut-workflows",
			}, &pr)
			Expect(err).ToNot(HaveOccurred())

			// Label value should be truncated to <= 63 characters
			Expect(len(pr.Labels["kubernaut.ai/target-resource"])).To(BeNumerically("<=", 63))
		})
	})
})

// ========================================
// DD-WE-006: Schema-Declared Dependencies Tests
// Job executor: volume mounts
// Tekton executor: workspace bindings
// ========================================

var _ = Describe("Job Executor Dependencies (DD-WE-006)", func() {
	var (
		jobExec   *executor.JobExecutor
		k8sClient client.Client
		scheme    *runtime.Scheme
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		jobExec = executor.NewJobExecutor(k8sClient, "test-sa")
	})

	It("UT-WE-006-020: should mount secrets as volumes on the Job", func() {
		wfe := newTestWFE("test-wfe-deps", "default", "default/deployment/deps-app",
			"fix-cert", "ghcr.io/kubernaut/workflows/fix-cert:v1.0.0", nil)

		opts := executor.CreateOptions{
			Dependencies: &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{
					{Name: "gitea-repo-creds"},
				},
			},
		}

		name, err := jobExec.Create(ctx, wfe, "kubernaut-workflows", opts)
		Expect(err).ToNot(HaveOccurred())

		var job batchv1.Job
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "kubernaut-workflows"}, &job)).To(Succeed())

		Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
			HaveField("Name", "secret-gitea-repo-creds"),
		))

		container := job.Spec.Template.Spec.Containers[0]
		Expect(container.VolumeMounts).To(ContainElement(And(
			HaveField("Name", "secret-gitea-repo-creds"),
			HaveField("MountPath", "/run/kubernaut/secrets/gitea-repo-creds"),
			HaveField("ReadOnly", true),
		)))
	})

	It("UT-WE-006-021: should mount configMaps as volumes on the Job", func() {
		wfe := newTestWFE("test-wfe-cm", "default", "default/deployment/cm-app",
			"fix-cert", "ghcr.io/kubernaut/workflows/fix-cert:v1.0.0", nil)

		opts := executor.CreateOptions{
			Dependencies: &models.WorkflowDependencies{
				ConfigMaps: []models.ResourceDependency{
					{Name: "remediation-config"},
				},
			},
		}

		name, err := jobExec.Create(ctx, wfe, "kubernaut-workflows", opts)
		Expect(err).ToNot(HaveOccurred())

		var job batchv1.Job
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "kubernaut-workflows"}, &job)).To(Succeed())

		Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
			HaveField("Name", "configmap-remediation-config"),
		))

		container := job.Spec.Template.Spec.Containers[0]
		Expect(container.VolumeMounts).To(ContainElement(And(
			HaveField("Name", "configmap-remediation-config"),
			HaveField("MountPath", "/run/kubernaut/configmaps/remediation-config"),
			HaveField("ReadOnly", true),
		)))
	})

	It("UT-WE-006-022: should mount both secrets and configMaps at correct paths", func() {
		wfe := newTestWFE("test-wfe-both", "default", "default/deployment/both-app",
			"fix-cert", "ghcr.io/kubernaut/workflows/fix-cert:v1.0.0", nil)

		opts := executor.CreateOptions{
			Dependencies: &models.WorkflowDependencies{
				Secrets:    []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				ConfigMaps: []models.ResourceDependency{{Name: "remediation-config"}},
			},
		}

		name, err := jobExec.Create(ctx, wfe, "kubernaut-workflows", opts)
		Expect(err).ToNot(HaveOccurred())

		var job batchv1.Job
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "kubernaut-workflows"}, &job)).To(Succeed())

		Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(2))
		Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
			HaveField("Name", "secret-gitea-repo-creds"),
		))
		Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
			HaveField("Name", "configmap-remediation-config"),
		))

		container := job.Spec.Template.Spec.Containers[0]
		Expect(container.VolumeMounts).To(HaveLen(2))
		Expect(container.VolumeMounts).To(ContainElement(And(
			HaveField("Name", "secret-gitea-repo-creds"),
			HaveField("MountPath", "/run/kubernaut/secrets/gitea-repo-creds"),
			HaveField("ReadOnly", true),
		)))
		Expect(container.VolumeMounts).To(ContainElement(And(
			HaveField("Name", "configmap-remediation-config"),
			HaveField("MountPath", "/run/kubernaut/configmaps/remediation-config"),
			HaveField("ReadOnly", true),
		)))
	})

	It("UT-WE-006-023: should create Job without volumes when no dependencies", func() {
		wfe := newTestWFE("test-wfe-nodeps", "default", "default/deployment/nodeps-app",
			"restart", "ghcr.io/kubernaut/workflows/restart:v1.0.0", nil)

		name, err := jobExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		var job batchv1.Job
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "kubernaut-workflows"}, &job)).To(Succeed())

		Expect(job.Spec.Template.Spec.Volumes).To(BeEmpty())
		container := job.Spec.Template.Spec.Containers[0]
		Expect(container.VolumeMounts).To(BeEmpty())
	})
})

var _ = Describe("Tekton Executor Dependencies (DD-WE-006)", func() {
	var (
		tektonExec *executor.TektonExecutor
		k8sClient  client.Client
		scheme     *runtime.Scheme
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		tektonExec = executor.NewTektonExecutor(k8sClient, "test-sa")
	})

	It("UT-WE-006-024: should add secret workspace binding to PipelineRun", func() {
		wfe := newTestWFE("test-wfe-tekton-secret", "default", "default/deployment/secret-app",
			"fix-cert", "ghcr.io/kubernaut/workflows/fix-cert:v1.0.0", nil)

		opts := executor.CreateOptions{
			Dependencies: &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{
					{Name: "gitea-repo-creds"},
				},
			},
		}

		name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", opts)
		Expect(err).ToNot(HaveOccurred())

		var pr tektonv1.PipelineRun
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "kubernaut-workflows"}, &pr)).To(Succeed())

		Expect(pr.Spec.Workspaces).To(HaveLen(1))
		Expect(pr.Spec.Workspaces[0].Name).To(Equal("secret-gitea-repo-creds"))
		Expect(pr.Spec.Workspaces[0].Secret.SecretName).To(Equal("gitea-repo-creds"))
	})

	It("UT-WE-006-025: should add configMap workspace binding to PipelineRun", func() {
		wfe := newTestWFE("test-wfe-tekton-cm", "default", "default/deployment/cm-app-tekton",
			"fix-cert", "ghcr.io/kubernaut/workflows/fix-cert:v1.0.0", nil)

		opts := executor.CreateOptions{
			Dependencies: &models.WorkflowDependencies{
				ConfigMaps: []models.ResourceDependency{
					{Name: "remediation-config"},
				},
			},
		}

		name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", opts)
		Expect(err).ToNot(HaveOccurred())

		var pr tektonv1.PipelineRun
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "kubernaut-workflows"}, &pr)).To(Succeed())

		Expect(pr.Spec.Workspaces).To(HaveLen(1))
		Expect(pr.Spec.Workspaces[0].Name).To(Equal("configmap-remediation-config"))
		Expect(pr.Spec.Workspaces[0].ConfigMap.Name).To(Equal("remediation-config"))
	})

	It("UT-WE-006-026: should create PipelineRun without workspaces when no dependencies", func() {
		wfe := newTestWFE("test-wfe-tekton-nodeps", "default", "default/deployment/nodeps-tekton",
			"restart", "ghcr.io/kubernaut/workflows/restart:v1.0.0", nil)

		name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		var pr tektonv1.PipelineRun
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "kubernaut-workflows"}, &pr)).To(Succeed())

		Expect(pr.Spec.Workspaces).To(BeEmpty())
	})

	It("UT-WE-006-027: should add both secret and configMap workspace bindings", func() {
		wfe := newTestWFE("test-wfe-tekton-both", "default", "default/deployment/both-tekton",
			"fix-cert", "ghcr.io/kubernaut/workflows/fix-cert:v1.0.0", nil)

		opts := executor.CreateOptions{
			Dependencies: &models.WorkflowDependencies{
				Secrets:    []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				ConfigMaps: []models.ResourceDependency{{Name: "remediation-config"}},
			},
		}

		name, err := tektonExec.Create(ctx, wfe, "kubernaut-workflows", opts)
		Expect(err).ToNot(HaveOccurred())

		var pr tektonv1.PipelineRun
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "kubernaut-workflows"}, &pr)).To(Succeed())

		Expect(pr.Spec.Workspaces).To(HaveLen(2))
		Expect(pr.Spec.Workspaces).To(ContainElement(And(
			HaveField("Name", "secret-gitea-repo-creds"),
			HaveField("Secret.SecretName", "gitea-repo-creds"),
		)))
		Expect(pr.Spec.Workspaces).To(ContainElement(And(
			HaveField("Name", "configmap-remediation-config"),
			HaveField("ConfigMap.Name", "remediation-config"),
		)))
	})
})

// ========================================
// Test Helpers
// ========================================

// newTestWFE creates a WorkflowExecution object for testing.
func newTestWFE(name, namespace, targetResource, workflowID, containerImage string, params map[string]string) *workflowexecutionv1alpha1.WorkflowExecution {
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			ExecutionEngine: "tekton",
			TargetResource:  targetResource,
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:     workflowID,
				ExecutionBundle: containerImage,
			},
			Parameters: params,
		},
	}
}

// newTestPipelineRun creates a PipelineRun with a Succeeded condition for testing.
func newTestPipelineRun(name, namespace string, condStatus corev1.ConditionStatus, reason, message string, childRefs int) *tektonv1.PipelineRun {
	children := make([]tektonv1.ChildStatusReference, childRefs)
	for i := range children {
		children[i] = tektonv1.ChildStatusReference{
			Name: fmt.Sprintf("task-%d", i+1),
		}
	}

	return &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: tektonv1.PipelineRunStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:    apis.ConditionSucceeded,
						Status:  condStatus,
						Reason:  reason,
						Message: message,
					},
				},
			},
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: children,
			},
		},
	}
}
