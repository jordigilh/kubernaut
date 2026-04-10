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

package locking

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("CheckResourceBusy Owner-Ref Self-Detection (BR-ORCH-050)", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		engine     *routing.RoutingEngine
		scheme     *runtime.Scheme
		namespace  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "kubernaut-system"

		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&workflowexecutionv1.WorkflowExecution{}, "spec.targetResource", func(obj client.Object) []string {
				wfe := obj.(*workflowexecutionv1.WorkflowExecution)
				if wfe.Spec.TargetResource == "" {
					return nil
				}
				return []string{wfe.Spec.TargetResource}
			}).
			Build()

		config := routing.Config{
			ConsecutiveFailureThreshold: 3,
			ConsecutiveFailureCooldown:  3600,
			RecentlyRemediatedCooldown:  300,
		}
		engine = routing.NewRoutingEngine(fakeClient, fakeClient, namespace, config, &mocks.AlwaysManagedScopeChecker{})
	})

	Describe("UT-RO-189-008: CheckResourceBusy skips WFE owned by current RR", func() {
		It("should return nil (not blocked) when the active WFE is owned by the requesting RR", func() {
			targetResource := "Deployment/test-app"
			rrUID := types.UID("rr-uid-001")

			// Create the RR
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-001",
					Namespace: namespace,
					UID:       rrUID,
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Create a WFE owned by this RR (via controller owner reference)
			trueVal := true
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "we-rr-001",
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         remediationv1.GroupVersion.String(),
							Kind:               "RemediationRequest",
							Name:               "rr-001",
							UID:                rrUID,
							Controller:         &trueVal,
							BlockOwnerDeletion: &trueVal,
						},
					},
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: "Running", // Active, not terminal
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// CheckResourceBusy should skip because the WFE belongs to the same RR
			blocked, err := engine.CheckResourceBusy(ctx, rr, targetResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(blocked).To(BeNil(), "should not be blocked by own WFE (owner UID match)")
		})
	})

	Describe("UT-RO-189-009: CheckResourceBusy blocks WFE owned by different RR", func() {
		It("should return a BlockingCondition when the active WFE is owned by a different RR", func() {
			targetResource := "Deployment/test-app"

			// Create the requesting RR (different UID from the WFE owner)
			requestingRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-002",
					Namespace: namespace,
					UID:       types.UID("rr-uid-002"),
				},
			}
			Expect(fakeClient.Create(ctx, requestingRR)).To(Succeed())

			// Create a WFE owned by a DIFFERENT RR
			trueVal := true
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "we-rr-001",
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         remediationv1.GroupVersion.String(),
							Kind:               "RemediationRequest",
							Name:               "rr-001",
							UID:                types.UID("rr-uid-001"), // Different from requestingRR
							Controller:         &trueVal,
							BlockOwnerDeletion: &trueVal,
						},
					},
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: "Running",
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// CheckResourceBusy should block because the WFE belongs to a different RR
			blocked, err := engine.CheckResourceBusy(ctx, requestingRR, targetResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(blocked).NotTo(BeNil(), "should be blocked by WFE owned by a different RR")
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonResourceBusy)))
			Expect(blocked.BlockingWorkflowExecution).To(Equal("we-rr-001"))
		})

		It("should block when WFE has no owner references (orphaned)", func() {
			targetResource := "Deployment/test-app"

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-003",
					Namespace: namespace,
					UID:       types.UID("rr-uid-003"),
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Create WFE with no owner references (orphaned)
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "we-orphaned",
					Namespace: namespace,
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: "Running",
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// Orphaned WFE should block (can't prove ownership)
			blocked, err := engine.CheckResourceBusy(ctx, rr, targetResource)
			Expect(err).NotTo(HaveOccurred())
			Expect(blocked).NotTo(BeNil(), "orphaned WFE should block (no ownership proof)")
		})
	})
})
