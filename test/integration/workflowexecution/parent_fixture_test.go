package workflowexecution

import (
	"fmt"
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Normal-execution parent fixtures (BR-ORCH-029, BR-WE-014)", func() {
	DescribeTable("IT-WE-2588-024: binds the WFE to an active persisted parent before dispatch",
		func(engine string) {
			id := fmt.Sprintf("parent-%s-%d", engine, time.Now().UnixNano())
			var wfe *workflowexecutionv1.WorkflowExecution
			switch engine {
			case "job":
				wfe = createUniqueJobWFE(id, "default/apps/v1/Deployment/parent-fixture")
			case "tekton":
				wfe = createUniqueWFE(id, "default/apps/v1/Deployment/parent-fixture")
			default:
				wfe = createMinimalWorkflowExecution(id, DefaultNamespace)
			}
			wfe.Spec.WorkflowRef.WorkflowID = trustedWorkflowFixture("Active parent dispatch fixture", wfe.Spec.WorkflowRef.ExecutionEngine)
			Expect(createWorkflowExecutionWithActiveParent(ctx, wfe)).To(Succeed())
			DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, wfe))).To(Succeed()) })

			parent := &remediationv1.RemediationRequest{}
			ref := wfe.Spec.RemediationRequestRef
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, parent)).To(Succeed())
			Expect(parent.Status.OverallPhase).To(Equal(remediationv1.PhaseExecuting))
			Expect(parent.UID).NotTo(BeEmpty())
			Expect(ref.UID).To(Equal(parent.UID))
			Expect(ref.APIVersion).To(Equal(remediationv1.GroupVersion.String()))
			Expect(ref.Kind).To(Equal("RemediationRequest"))
			Expect(wfe.OwnerReferences).To(ContainElement(SatisfyAll(
				HaveField("UID", parent.UID), HaveField("Name", parent.Name), HaveField("Controller", HaveValue(BeTrue())),
			)))

			Eventually(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1.PhaseRunning))
				g.Expect(updated.Status.ExecutionRef).NotTo(BeNil())
			}, 15*time.Second, 100*time.Millisecond).Should(Succeed())
		},
		Entry("Job builder", "job"),
		Entry("Tekton builder", "tekton"),
		Entry("failure-classification builder", "minimal"),
	)

	DescribeTable("IT-WE-2588-027: reuses an active parent without rewriting its identity (BR-ORCH-029)",
		func(crossNamespace bool) {
			id := fmt.Sprintf("reuse-%d", time.Now().UnixNano())
			var parentUID types.UID
			var parentVersion string
			for i := range 2 {
				wfe := createUniqueJobWFE(fmt.Sprintf("%s-%d", id, i), "default/apps/v1/Deployment/reuse-fixture")
				wfe.Spec.RemediationRequestRef.Name = "rr-" + id
				wfe.Spec.RemediationRequestRef.Namespace = "" // same-namespace default must become explicit
				if crossNamespace {
					wfe.Spec.RemediationRequestRef.Namespace = WorkflowExecutionNS
				}
				wfe.Spec.WorkflowRef.WorkflowID = trustedWorkflowFixture("Reused active parent fixture", "job")
				Expect(createWorkflowExecutionWithActiveParent(ctx, wfe)).To(Succeed())
				DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, wfe))).To(Succeed()) })
				ref := wfe.Spec.RemediationRequestRef
				parent := &remediationv1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, parent)).To(Succeed())
				Expect(parent.Status.OverallPhase).To(Equal(remediationv1.PhaseExecuting))
				Expect(ref.UID).NotTo(BeEmpty())
				Expect(ref.UID).To(Equal(parent.UID))
				if i == 0 {
					parentUID = parent.UID
					parentVersion = parent.ResourceVersion
				}
				Expect(parent.UID).To(Equal(parentUID))
				Expect(parent.ResourceVersion).To(Equal(parentVersion), "reuse must not recreate or rewrite the existing parent")
				persisted, err := getWFE(wfe.Name, wfe.Namespace)
				Expect(err).NotTo(HaveOccurred())
				Expect(persisted.Spec.RemediationRequestRef).To(Equal(ref))
				if crossNamespace {
					Expect(ref.Namespace).To(Equal(WorkflowExecutionNS))
					Expect(persisted.OwnerReferences).To(BeEmpty(), "cross-namespace controller ownership is invalid")
				} else {
					Expect(ref.Namespace).To(Equal(wfe.Namespace))
					Expect(persisted.OwnerReferences).To(ContainElement(HaveField("UID", parentUID)))
				}
			}
		},
		Entry("same namespace and sequential parent reuse", false),
		Entry("cross namespace and sequential parent reuse", true),
	)

	It("IT-WE-2588-025: keeps an owned missing-parent fixture on the cancellation path (BR-ORCH-029)", func() {
		id := fmt.Sprintf("orphan-%d", time.Now().UnixNano())
		wfe := createUniqueJobWFE(id, "default/apps/v1/Deployment/orphan-fixture")
		ref := &wfe.Spec.RemediationRequestRef
		ref.APIVersion = remediationv1.GroupVersion.String()
		ref.UID = types.UID(id)
		controller := true
		wfe.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: ref.APIVersion, Kind: ref.Kind, Name: ref.Name, UID: ref.UID, Controller: &controller,
		}}
		// Deliberately bypass the active-parent helper: this is an orphan, not
		// a normal-execution fixture. No parent may be synthesized for this test.
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, wfe))).To(Succeed()) })
		Eventually(func(g Gomega) {
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1.PhaseCancelled))
			g.Expect(updated.Status.ExecutionRef).To(BeNil())
		}, 15*time.Second, 100*time.Millisecond).Should(Succeed())
		parent := &remediationv1.RemediationRequest{}
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, parent))).To(BeTrue())
	})
})
