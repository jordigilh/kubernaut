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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

func tektonWFE(targetResource string) *workflowexecutionv1alpha1.WorkflowExecution {
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "wfe-2658", Namespace: "kubernaut-system"},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			TargetResource: targetResource,
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
				WorkflowID:      "wf-1",
				ExecutionBundle: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
				ExecutionEngine: "tekton",
			}},
		},
	}
}

func targetResourceParam(wfe *workflowexecutionv1alpha1.WorkflowExecution) string {
	pr := (&weexecutor.TektonExecutor{}).BuildPipelineRun(context.Background(), wfe, "kubernaut-workflows", weexecutor.CreateOptions{})
	for _, param := range pr.Spec.Params {
		if param.Name == "TARGET_RESOURCE" {
			return param.Value.StringVal
		}
	}
	return ""
}

// UT-RO-2658-004 / UT-WE-2658-003 (#2658 E10): the group-aware identity must
// survive the Tekton path -- labels are legal and group-distinct, the
// PipelineRun name diverges per group, and TARGET_RESOURCE carries the exact
// identity (bundle-facing contract break, accepted under no-backcompat).
var _ = Describe("Tekton group-aware target identity (#2658) [DD-WE-001, DD-WE-003]", func() {
	DescribeTable("label values stay legal and distinguishable",
		func(targetResource string) {
			svc := &weexecutor.TektonExecutor{}
			pr := svc.BuildPipelineRun(context.Background(), tektonWFE(targetResource), "kubernaut-workflows", weexecutor.CreateOptions{})
			label, ok := pr.Labels["kubernaut.ai/target-resource"]
			Expect(ok).To(BeTrue(), "the target-resource provenance label must be present")
			Expect(validation.IsValidLabelValue(label)).To(BeEmpty(), "label %q must be a legal label value", label)
			Expect(label).NotTo(ContainSubstring("@"), "'@' is not a legal label character")
		},
		Entry("namespaced with a group", "prod/networking.k8s.io/v1/Ingress/web"),
		Entry("core group", "prod/core/v1/Pod/api"),
		Entry("unknown group", "prod/unresolved/unresolved/core/v1/Pod/api"),
		Entry("long identity with a group", "a-very-long-namespace-name/Ingress/a-very-long-resource-name-that-overflows@networking.k8s.io"),
	)

	It("keeps two >63-char targets distinguishable after truncation (hash suffix)", func() {
		svc := &weexecutor.TektonExecutor{}
		long := "a-very-long-namespace-name/Ingress/a-very-long-resource-name-that-overflows"
		first := svc.BuildPipelineRun(context.Background(), tektonWFE(long+"@networking.k8s.io"), "kubernaut-workflows", weexecutor.CreateOptions{}).Labels["kubernaut.ai/target-resource"]
		second := svc.BuildPipelineRun(context.Background(), tektonWFE(long+"@extensions"), "kubernaut-workflows", weexecutor.CreateOptions{}).Labels["kubernaut.ai/target-resource"]

		Expect(len(first)).To(BeNumerically("<=", 63))
		Expect(len(second)).To(BeNumerically("<=", 63))
		Expect(first).NotTo(Equal(second),
			"truncation must not collapse two cross-group targets into one label value")
		Expect(validation.IsValidLabelValue(first)).To(BeEmpty())
		Expect(validation.IsValidLabelValue(second)).To(BeEmpty())
	})

	It("generates a distinct PipelineRun name per API group", func() {
		svc := &weexecutor.TektonExecutor{}
		first := svc.BuildPipelineRun(context.Background(), tektonWFE("prod/networking.k8s.io/v1/Ingress/web"), "kubernaut-workflows", weexecutor.CreateOptions{}).Name
		second := svc.BuildPipelineRun(context.Background(), tektonWFE("prod/extensions/v1beta1/Ingress/web"), "kubernaut-workflows", weexecutor.CreateOptions{}).Name
		Expect(first).NotTo(Equal(second))
		Expect(strings.HasPrefix(first, "wfe-")).To(BeTrue())
	})

	It("passes TARGET_RESOURCE through byte-for-byte (bundle contract)", func() {
		target := "prod/networking.k8s.io/v1/Ingress/web"
		Expect(targetResourceParam(tektonWFE(target))).To(Equal(target),
			"bundles receive the exact group-aware identity, suffix included")
	})
})
