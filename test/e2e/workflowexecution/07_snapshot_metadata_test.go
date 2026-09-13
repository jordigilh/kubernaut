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

package workflowexecution

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-WE-2390-002..004 validate the common WorkflowRef snapshot contract and
// the engine-specific dispatch fields on a real WorkflowExecution controller.
// The FullPipeline harness owns the interactive Job journey; this suite owns
// live Job, Tekton, and Ansible execution resources.
var _ = Describe("E2E-WE-2390: cross-engine workflow snapshot metadata", Label("e2e", "workflowexecution", "workflowref-snapshot"), func() {
	It("E2E-WE-2390-002: preserves the common snapshot and Job-specific fields", func() {
		resources := &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m")},
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Mi")},
		}
		wfe := newSnapshotWFE(snapshotWFEOptions{
			name:         "e2e-2390-job",
			workflowID:   infrastructure.RegisteredWorkflowUUIDs["test-job-hello-world"],
			workflowName: "test-job-hello-world",
			engine:       "job",
			bundle:       fmt.Sprintf("%s/job-hello-world:%s", infrastructure.TestWorkflowBundleRegistry, infrastructure.TestWorkflowBundleVersion),
			dependencies: &sharedtypes.WorkflowDependencies{Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "e2e-dep-secret"}}},
			resources:    resources,
		})
		wfe.Spec.WorkflowRef.ServiceAccountName = "workflow-job-executor"
		defer func() { _ = deleteWFE(wfe) }()

		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		assertSnapshotRoundTrip(wfe, resources, nil)

		Eventually(phaseOrFailFast(wfe.Name, wfe.Namespace), 60*time.Second, 2*time.Second).
			Should(Equal(workflowexecutionv1alpha1.PhaseRunning))
		var jobs batchv1.JobList
		Eventually(func() int {
			if err := apiReader.List(ctx, &jobs, client.InNamespace(infrastructure.ExecutionNamespace),
				client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name}); err != nil {
				return 0
			}
			return len(jobs.Items)
		}, 30*time.Second, 2*time.Second).Should(Equal(1))

		job := jobs.Items[0]
		Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("workflow-job-executor"))
		Expect(job.Spec.Template.Spec.Containers[0].Resources).To(Equal(*resources))
	})

	It("E2E-WE-2390-003: preserves the common snapshot and Tekton dispatch fields", func() {
		wfe := newSnapshotWFE(snapshotWFEOptions{
			name:         "e2e-2390-tekton",
			workflowID:   infrastructure.RegisteredWorkflowUUIDs["test-hello-world"],
			workflowName: "test-hello-world",
			engine:       "tekton",
			bundle:       "quay.io/kubernaut-cicd/tekton-bundles/hello-world:v1.0.0",
			dependencies: &sharedtypes.WorkflowDependencies{Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "e2e-dep-secret-tekton"}}},
		})
		defer func() { _ = deleteWFE(wfe) }()

		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		assertSnapshotRoundTrip(wfe, nil, nil)
		Eventually(phaseOrFailFast(wfe.Name, wfe.Namespace), 60*time.Second, 2*time.Second).
			Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

		var runs tektonv1.PipelineRunList
		Eventually(func() int {
			if err := apiReader.List(ctx, &runs, client.InNamespace(infrastructure.ExecutionNamespace),
				client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name}); err != nil {
				return 0
			}
			return len(runs.Items)
		}, 30*time.Second, 2*time.Second).Should(Equal(1))

		pipelineRun := runs.Items[0]
		By("E2E-WE-2390-003: verifying the PipelineRun uses the Tekton bundle resolver")
		Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil(), "PipelineRun should reference the resolved Pipeline")
		Expect(pipelineRun.Spec.PipelineRef.Resolver).To(BeEquivalentTo("bundles"))
		resolverParams := make(map[string]string, len(pipelineRun.Spec.PipelineRef.Params))
		for _, param := range pipelineRun.Spec.PipelineRef.Params {
			Expect(param.Value.Type).To(Equal(tektonv1.ParamTypeString),
				"Tekton resolver parameter %q should be a string", param.Name)
			resolverParams[param.Name] = param.Value.StringVal
		}
		Expect(resolverParams).To(Equal(map[string]string{
			"bundle": wfe.Spec.WorkflowRef.ExecutionBundle,
			"name":   "workflow",
			"kind":   "pipeline",
		}), "PipelineRun should resolve the Pipeline from the WFE execution bundle")

		By("Verifying the WFE references the observed PipelineRun")
		Eventually(func() string {
			updated, err := getWFEDirect(wfe.Name, wfe.Namespace)
			if err != nil || updated == nil || updated.Status.ExecutionRef == nil {
				return ""
			}
			return updated.Status.ExecutionRef.Name
		}, 30*time.Second, 2*time.Second).Should(Equal(pipelineRun.Name))

		Expect(pipelineRun.Spec.Workspaces).To(ContainElement(
			HaveField("Name", "secret-e2e-dep-secret-tekton")))

		By("E2E-WE-2390-003: waiting for Tekton PipelineRun success")
		Eventually(func() bool {
			var current tektonv1.PipelineRun
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(&pipelineRun), &current); err != nil {
				return false
			}
			condition := current.Status.GetCondition(apis.ConditionSucceeded)
			if condition == nil {
				return false
			}
			GinkgoWriter.Printf("Tekton PipelineRun %s condition: %s (%s)\n",
				current.Name, condition.Status, condition.Reason)
			return condition.IsTrue()
		}, 120*time.Second, 2*time.Second).Should(BeTrue(),
			"Tekton PipelineRun should reach Succeeded=True")

		By("E2E-WE-2390-003: verifying the controller maps Tekton success to WFE completion")
		Eventually(phaseOrFailFast(wfe.Name, wfe.Namespace), 30*time.Second, 2*time.Second).
			Should(Equal(workflowexecutionv1alpha1.PhaseCompleted))
	})

	It("E2E-WE-2390-004: preserves the common snapshot and Ansible engine configuration", func() {
		engineConfig := &apiextensionsv1.JSON{Raw: []byte(`{"playbookPath":"playbooks/test-success.yml","jobTemplateName":"kubernaut-test-success","inventoryName":"kubernaut-test-inventory"}`)}
		wfe := newSnapshotWFE(snapshotWFEOptions{
			name:         "e2e-2390-ansible",
			workflowID:   infrastructure.RegisteredWorkflowUUIDs["test-ansible-success"],
			workflowName: "test-ansible-success",
			engine:       "ansible",
			bundle:       "https://github.com/kubernaut/test-playbooks.git",
			dependencies: &sharedtypes.WorkflowDependencies{Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "e2e-dep-secret-ansible"}}},
			engineConfig: engineConfig,
		})
		defer func() { _ = deleteWFE(wfe) }()

		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		assertSnapshotRoundTrip(wfe, nil, engineConfig)
		Eventually(phaseOrFailFast(wfe.Name, wfe.Namespace), 60*time.Second, 2*time.Second).
			Should(Equal(workflowexecutionv1alpha1.PhaseRunning))
		updated, err := getWFEDirect(wfe.Name, wfe.Namespace)
		Expect(err).ToNot(HaveOccurred())
		Expect(updated.Status.ExecutionRef).ToNot(BeNil())
		Expect(updated.Status.ExecutionRef.Name).To(HavePrefix("awx-job-"))
	})
})

type snapshotWFEOptions struct {
	name         string
	workflowID   string
	workflowName string
	engine       string
	bundle       string
	dependencies *sharedtypes.WorkflowDependencies
	resources    *corev1.ResourceRequirements
	engineConfig *apiextensionsv1.JSON
}

func newSnapshotWFE(options snapshotWFEOptions) *workflowexecutionv1alpha1.WorkflowExecution {
	Expect(options.workflowID).ToNot(BeEmpty(), "registered workflow UUID must be available")
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{Name: options.name, Namespace: controllerNamespace},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
				Kind:       "RemediationRequest",
				Name:       "test-rr-" + options.name,
				Namespace:  controllerNamespace,
			},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
				WorkflowID: options.workflowID, WorkflowName: options.workflowName, ActionType: "RestartPod", Version: "1.0.0",
				ExecutionBundle: options.bundle, ExecutionEngine: options.engine, Dependencies: options.dependencies, Resources: options.resources, EngineConfig: options.engineConfig,
			}},
			TargetResource: "default/deployment/e2e-2390-" + options.name,
			Parameters:     map[string]string{"MESSAGE": "E2E-FP-2390 snapshot metadata"},
		},
	}
}

func assertSnapshotRoundTrip(wfe *workflowexecutionv1alpha1.WorkflowExecution, resources *corev1.ResourceRequirements, engineConfig *apiextensionsv1.JSON) {
	var persisted workflowexecutionv1alpha1.WorkflowExecution
	Eventually(func() error {
		return apiReader.Get(ctx, client.ObjectKeyFromObject(wfe), &persisted)
	}, 30*time.Second, time.Second).Should(Succeed())

	snapshot := persisted.Spec.WorkflowRef.WorkflowSnapshot
	Expect(snapshot.WorkflowID).To(Equal(wfe.Spec.WorkflowRef.WorkflowID))
	Expect(snapshot.WorkflowName).To(Equal(wfe.Spec.WorkflowRef.WorkflowName))
	Expect(snapshot.ActionType).To(Equal("RestartPod"))
	Expect(snapshot.Version).To(Equal("1.0.0"))
	Expect(snapshot.ExecutionBundle).To(Equal(wfe.Spec.WorkflowRef.ExecutionBundle))
	Expect(snapshot.ExecutionEngine).To(Equal(wfe.Spec.WorkflowRef.ExecutionEngine))
	Expect(snapshot.Dependencies).To(Equal(wfe.Spec.WorkflowRef.Dependencies))
	Expect(snapshot.Resources).To(Equal(resources))
	if engineConfig != nil {
		var expected, actual map[string]string
		Expect(json.Unmarshal(engineConfig.Raw, &expected)).To(Succeed())
		Expect(json.Unmarshal(snapshot.EngineConfig.Raw, &actual)).To(Succeed())
		Expect(actual).To(Equal(expected))
	} else {
		Expect(snapshot.EngineConfig).To(BeNil())
	}
}
