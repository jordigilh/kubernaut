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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// UT-RO-2658-006 / UT-WE-2658-001 (#2658 E3b): the WFE validator must accept
// the group-aware identity the producers now write, validating only the
// identity part (namespace/kind/name) and treating the group/version path parts as a
// valid DNS subdomain.
var _ = Describe("Group-aware targetResource validation (#2658) [DD-WE-001]", func() {
	validWFE := func(targetResource string) *workflowexecutionv1alpha1.WorkflowExecution {
		return &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-1", Namespace: "kubernaut-system"},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1ObjectReference("rr-1"),
				TargetResource:        targetResource,
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					WorkflowID:      "wf-1",
					WorkflowName:    "wf-1",
					ExecutionBundle: "quay.io/kubernaut/test-workflow:1.0.0",
					ExecutionEngine: "job",
				}},
			},
		}
	}

	DescribeTable("accepts every identity form the producers emit",
		func(targetResource string) {
			r := &WorkflowExecutionReconciler{}
			Expect(r.ValidateSpec(validWFE(targetResource))).To(Succeed())
		},
		Entry("namespaced grouped", "prod/networking.k8s.io/v1/Ingress/web"),
		Entry("namespaced core", "prod/core/v1/Pod/api"),
		Entry("cluster-scoped grouped", "metrics.k8s.io/v1/Node/worker-1"),
		Entry("cluster-scoped core", "core/v1/Node/worker-1"),
		Entry("unknown apiVersion sentinel", "prod/unresolved/unresolved/Pod/api"),
		// R17-1: the CRD pattern's group slot must accept DNS1123 subdomains
		// with hyphens, matching ParseTargetIdentity/ParseAPIVersion.
		Entry("hyphenated group", "prod/cert-manager.io/v1/Certificate/le"),
	)

	DescribeTable("rejects malformed identities",
		func(targetResource string) {
			r := &WorkflowExecutionReconciler{}
			Expect(r.ValidateSpec(validWFE(targetResource))).NotTo(Succeed())
		},
		Entry("too many identity parts", "a/b/c/d/e/f"),
		Entry("empty group slot", "prod//v1/Ingress/web"),
		Entry("invalid namespace", "PROD/apps/v1/Ingress/web"),
		Entry("invalid name", "prod/apps/v1/Ingress/web_"),
		Entry("only a sentinel", "unresolved"),
	)
})

func corev1ObjectReference(name string) corev1.ObjectReference {
	return corev1.ObjectReference{Name: name, Namespace: "kubernaut-system", UID: "rr-uid"}
}
