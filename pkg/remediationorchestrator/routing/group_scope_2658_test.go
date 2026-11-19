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

package routing_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// groupScopeEngine builds a RoutingEngine with the field indexes the busy and
// cooldown gates query (#2658), plus the fake client those tests seed.
func groupScopeEngine() (client.Client, *routing.RoutingEngine) {
	scheme := runtime.NewScheme()
	Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
	Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&workflowexecutionv1.WorkflowExecution{}, "spec.targetResource", func(obj client.Object) []string {
			wfe := obj.(*workflowexecutionv1.WorkflowExecution)
			if wfe.Spec.TargetResource == "" {
				return nil
			}
			return []string{wfe.Spec.TargetResource}
		}).
		Build()
	engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", routing.Config{
		ConsecutiveFailureThreshold:   3,
		ConsecutiveFailureCooldown:    3600,
		RecentlyRemediatedCooldown:    300,
		ExponentialBackoffBase:        60,
		ExponentialBackoffMax:         600,
		ExponentialBackoffMaxExponent: 4,
	}, &mocks.AlwaysManagedScopeChecker{})
	return fakeClient, engine
}

func groupScopeRR(namespace, kind, name, apiVersion string) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "rr-2658", Namespace: "default", UID: types.UID("rr-2658-uid")},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: "fp-2658",
			SignalName:        "sig",
			Severity:          "warning",
			SignalType:        "alert",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:       kind,
				Name:       name,
				Namespace:  namespace,
				APIVersion: apiVersion,
			},
		},
	}
}

func runningWFE(name, targetResource string) *workflowexecutionv1.WorkflowExecution {
	return &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name + "-uid")},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-2658", Namespace: "default"},
			TargetResource:        targetResource,
			WorkflowRef:           workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: "wf-1", WorkflowName: "wf-1"}},
		},
		Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseRunning},
	}
}

func completedWFE(name, targetResource, workflowID string) *workflowexecutionv1.WorkflowExecution {
	completion := metav1.NewTime(metav1.Now().Add(-30 * 1e9)) // 30s ago
	return &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name + "-uid")},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-2658", Namespace: "default"},
			TargetResource:        targetResource,
			WorkflowRef:           workflowexecutionv1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: workflowID, WorkflowName: workflowID}},
		},
		Status: workflowexecutionv1.WorkflowExecutionStatus{
			Phase:          workflowexecutionv1.PhaseCompleted,
			CompletionTime: &completion,
		},
	}
}

var _ = Describe("Group-aware busy/cooldown gates (#2658) [BR-ORCH-042, DD-RO-002, AC-4]", func() {
	var (
		ctx        context.Context
		kubeClient client.Client
		engine     *routing.RoutingEngine
		rr         *remediationv1.RemediationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		kubeClient, engine = groupScopeEngine()
		rr = groupScopeRR("prod", "Ingress", "web", "networking.k8s.io/v1")
	})

	Describe("UT-RO-2658-001: the busy gate distinguishes API groups", func() {
		It("does not block when the only running WFE targets the same name in another group", func() {
			Expect(kubeClient.Create(ctx, runningWFE("wfe-extensions", "prod/extensions/v1beta1/Ingress/web"))).To(Succeed())

			blocked, err := engine.CheckResourceBusy(ctx, rr, "prod/networking.k8s.io/v1/Ingress/web")
			Expect(errors.Is(err, routing.ErrNotBlocked)).To(BeTrue(), "a cross-group sibling must not make this target busy")
			Expect(blocked).To(BeNil())
		})

		It("still blocks when the running WFE targets the same group", func() {
			Expect(kubeClient.Create(ctx, runningWFE("wfe-same-group", "prod/networking.k8s.io/v1/Ingress/web"))).To(Succeed())

			blocked, err := engine.CheckResourceBusy(ctx, rr, "prod/networking.k8s.io/v1/Ingress/web")
			Expect(err).NotTo(HaveOccurred())
			Expect(blocked).NotTo(BeNil())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonResourceBusy)))
			// #2658: the condition carries the identity the gate evaluated (the
			// RCA remediation target), not the RR's signal target, so the audit
			// trail cannot name a resource that never collided.
			Expect(blocked.TargetResource).To(Equal("prod/networking.k8s.io/v1/Ingress/web"))
		})
	})

	Describe("UT-RO-2658-002: the cooldown gate distinguishes API groups", func() {
		It("does not block for a recently completed sibling in another group", func() {
			Expect(kubeClient.Create(ctx, completedWFE("wfe-cooldown-ext", "prod/extensions/v1beta1/Ingress/web", "wf-1"))).To(Succeed())

			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "wf-1", "prod/networking.k8s.io/v1/Ingress/web")
			Expect(errors.Is(err, routing.ErrNotBlocked)).To(BeTrue())
			Expect(blocked).To(BeNil())
		})

		It("blocks for a recently completed same-group execution", func() {
			Expect(kubeClient.Create(ctx, completedWFE("wfe-cooldown-same", "prod/networking.k8s.io/v1/Ingress/web", "wf-1"))).To(Succeed())

			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "wf-1", "prod/networking.k8s.io/v1/Ingress/web")
			Expect(err).NotTo(HaveOccurred())
			Expect(blocked).NotTo(BeNil())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonRecentlyRemediated)))
			Expect(blocked.TargetResource).To(Equal("prod/networking.k8s.io/v1/Ingress/web"))
		})
	})

	Describe("UT-RO-2658-007: parseTargetResource reads the canonical coordinates", func() {
		DescribeTable("recovers namespace/kind/name/apiVersion from the identity",
			func(input, namespace, kind, name, apiVersion string) {
				parsed := routing.ParseTargetResourceForTest(input)
				Expect(parsed.Namespace).To(Equal(namespace))
				Expect(parsed.Kind).To(Equal(kind))
				Expect(parsed.Name).To(Equal(name))
				Expect(parsed.APIVersion).To(Equal(apiVersion))
			},
			Entry("namespaced grouped", "prod/networking.k8s.io/v1/Ingress/web", "prod", "Ingress", "web", "networking.k8s.io/v1"),
			Entry("namespaced core", "prod/core/v1/Pod/api", "prod", "Pod", "api", "v1"),
			Entry("cluster-scoped grouped", "metrics.k8s.io/v1/Node/worker-1", "", "Node", "worker-1", "metrics.k8s.io/v1"),
			Entry("cluster-scoped core", "core/v1/Node/worker-1", "", "Node", "worker-1", "v1"),
			Entry("unknown renders the sentinel in both slots", "prod/unresolved/unresolved/Pod/api", "prod", "Pod", "api", ""),
		)
	})

	Describe("UT-RO-2658-004 (routing half): the group-aware string round-trips through the parser", func() {
		It("parses back every form the producers emit", func() {
			for _, target := range []string{
				"prod/networking.k8s.io/v1/Ingress/web",
				"prod/core/v1/Pod/api",
				"metrics.k8s.io/v1/Node/worker-1",
				"core/v1/Node/worker-1",
			} {
				parsed := routing.ParseTargetResourceForTest(target)
				Expect(parsed.String()).To(Equal(target),
					"parse+render must be stable so the gates and the DS query agree")
			}
		})
	})

	Describe("E4: TargetResource.String renders the canonical identity", func() {
		It("renders group and version as path parts", func() {
			Expect(routing.TargetResource{Namespace: "prod", Kind: "Pod", Name: "api", APIVersion: "v1"}.String()).To(Equal("prod/core/v1/Pod/api"))
			Expect(routing.TargetResource{Namespace: "prod", Kind: "Ingress", Name: "web", APIVersion: "extensions/v1beta1"}.String()).
				To(Equal("prod/extensions/v1beta1/Ingress/web"))
		})

		It("renders the cluster-scoped form without a leading empty segment", func() {
			Expect(routing.TargetResource{Kind: "Node", Name: "worker-1", APIVersion: "metrics.k8s.io/v1"}.String()).
				To(Equal("metrics.k8s.io/v1/Node/worker-1"))
		})
	})
})
