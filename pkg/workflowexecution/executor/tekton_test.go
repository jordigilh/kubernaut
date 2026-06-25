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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
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

func newTektonScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
	return scheme
}

// UT-WE-054-TEK: TektonExecutor unit tests
// Authority: BR-WE-014 (Workflow Execution Backend), BR-FLEET-054
// FedRAMP: AC-6 (Least Privilege) -- SA injection, AU-3 (Audit Content) -- labels
var _ = Describe("UT-WE-054-TEK: TektonExecutor", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = newTektonScheme()
		namespace = "kubernaut-workflows"
	})

	Describe("Engine", func() {
		It("UT-WE-054-TEK-001: should return 'tekton' as engine identifier", func() {
			te := executor.NewTektonExecutor(fake.NewClientBuilder().WithScheme(scheme).Build())
			Expect(te.Engine()).To(Equal("tekton"))
		})
	})

	Describe("Create", func() {
		It("UT-WE-054-TEK-002: should create PipelineRun with bundle resolver and correct labels", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			te := executor.NewTektonExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-tekton-001", "default/deployment/frontend", "")

			result, err := te.Create(ctx, wfe, namespace, executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.ResourceName).To(HavePrefix("wfe-"))

			var pr tektonv1.PipelineRun
			err = fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &pr)
			Expect(err).ToNot(HaveOccurred())
			Expect(pr.Labels["kubernaut.ai/workflow-execution"]).To(Equal("wfe-tekton-001"))
			Expect(pr.Spec.PipelineRef).ToNot(BeNil())
			Expect(pr.Spec.PipelineRef.ResolverRef.Resolver).To(Equal(tektonv1.ResolverName("bundles")))
			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("kubernaut-runner"))
		})

		It("UT-WE-054-TEK-003: should add workspace bindings for dependencies", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := &mockClientFactory{client: fakeClient}
			te := executor.NewTektonExecutorWithFactory(factory)
			wfe := newTestWFE("wfe-tekton-deps", "default/deployment/backend", "")

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets:    []models.ResourceDependency{{Name: "tls-cert"}},
					ConfigMaps: []models.ResourceDependency{{Name: "pipeline-config"}},
				},
			}

			result, err := te.Create(ctx, wfe, namespace, opts)
			Expect(err).ToNot(HaveOccurred())

			var pr tektonv1.PipelineRun
			err = fakeClient.Get(ctx, client.ObjectKey{Name: result.ResourceName, Namespace: namespace}, &pr)
			Expect(err).ToNot(HaveOccurred())

			wsNames := make([]string, 0, len(pr.Spec.Workspaces))
			for _, ws := range pr.Spec.Workspaces {
				wsNames = append(wsNames, ws.Name)
			}
			Expect(wsNames).To(ContainElement("secret-tls-cert"))
			Expect(wsNames).To(ContainElement("configmap-pipeline-config"))
		})
	})

	Describe("GetStatus", func() {
		It("UT-WE-054-TEK-004: should return Completed when PipelineRun succeeds", func() {
			prName := executor.ExecutionResourceName("default/deployment/complete")
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: prName, Namespace: namespace},
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
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			factory := &mockClientFactory{client: fakeClient}
			te := executor.NewTektonExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-tek-status", "default/deployment/complete", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: prName}

			result, err := te.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))
		})

		It("UT-WE-054-TEK-005: should return Failed when PipelineRun fails", func() {
			prName := executor.ExecutionResourceName("default/deployment/fail")
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: prName, Namespace: namespace},
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							{
								Type:    apis.ConditionSucceeded,
								Status:  corev1.ConditionFalse,
								Reason:  "TaskRunFailed",
								Message: "Task 'validate' failed",
							},
						},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			factory := &mockClientFactory{client: fakeClient}
			te := executor.NewTektonExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-tek-fail", "default/deployment/fail", "")
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{Name: prName}

			result, err := te.GetStatus(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(result.Reason).To(Equal("TaskRunFailed"))
		})
	})

	Describe("Cleanup", func() {
		It("UT-WE-054-TEK-006: should delete PipelineRun owned by this WFE", func() {
			prName := executor.ExecutionResourceName("default/deployment/tekcleanup")
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: namespace,
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "wfe-tek-cleanup",
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr).Build()
			factory := &mockClientFactory{client: fakeClient}
			te := executor.NewTektonExecutorWithFactory(factory)

			wfe := newTestWFE("wfe-tek-cleanup", "default/deployment/tekcleanup", "")

			err := te.Cleanup(ctx, wfe, namespace)
			Expect(err).ToNot(HaveOccurred())

			var deleted tektonv1.PipelineRun
			err = fakeClient.Get(ctx, client.ObjectKey{Name: prName, Namespace: namespace}, &deleted)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Shared utilities", func() {
		It("UT-WE-054-UTIL-001: ExecutionResourceName should be deterministic and prefixed", func() {
			name1 := executor.ExecutionResourceName("default/deployment/nginx")
			name2 := executor.ExecutionResourceName("default/deployment/nginx")
			name3 := executor.ExecutionResourceName("prod/deployment/nginx")

			Expect(name1).To(Equal(name2), "Same input must produce same name")
			Expect(name1).ToNot(Equal(name3), "Different input must produce different name")
			Expect(name1).To(HavePrefix("wfe-"))
		})

		It("UT-WE-054-UTIL-002: ConvertParameters should convert map to Tekton params", func() {
			params := executor.ConvertParameters(map[string]string{
				"TIMEOUT": "30s",
				"MODE":    "aggressive",
			})
			Expect(params).To(HaveLen(2))

			names := make([]string, 0, len(params))
			for _, p := range params {
				names = append(names, p.Name)
			}
			Expect(names).To(ContainElement("TIMEOUT"))
			Expect(names).To(ContainElement("MODE"))
		})

		It("UT-WE-054-UTIL-003: ConvertParameters should return empty slice for nil map", func() {
			params := executor.ConvertParameters(nil)
			Expect(params).To(BeEmpty())
		})
	})
})
