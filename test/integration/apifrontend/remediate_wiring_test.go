package apifrontend_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_remediate wiring (#1332)", func() {
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
	isGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "investigationsessions"}

	It("IT-AF-1332-W01: HandleRemediate creates RR via envtest", func() {
		ctx := context.Background()
		ns := "default"

		result, err := tools.HandleRemediate(ctx, dynamicClient, ns, &tools.RemediateArgs{
			Namespace:   ns,
			Kind:        "Deployment",
			Name:        "web-1332-w01",
			Description: "kubernaut_remediate wiring IT",
		}, "it-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).To(HavePrefix("rr-"))
		Expect(result.AlreadyExists).To(BeFalse())

		created, getErr := dynamicClient.Resource(rrGVR).Namespace(ns).Get(ctx, result.RRID, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())
		Expect(created.GetNamespace()).To(Equal(ns))

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1332-W02: HandleRemediate does NOT create InvestigationSession", func() {
		ctx := context.Background()
		ns := "default"

		result, err := tools.HandleRemediate(ctx, dynamicClient, ns, &tools.RemediateArgs{
			Namespace:   ns,
			Kind:        "Deployment",
			Name:        "web-1332-w02",
			Description: "no IS expected",
		}, "it-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).To(HavePrefix("rr-"))

		isList, listErr := dynamicClient.Resource(isGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		Expect(listErr).NotTo(HaveOccurred())
		for _, is := range isList.Items {
			owners := is.GetOwnerReferences()
			for _, ref := range owners {
				Expect(ref.Name).NotTo(Equal(result.RRID),
					"autonomous HandleRemediate must NOT create an IS for RR %s", result.RRID)
			}
		}

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})

	It("IT-AF-1332-W03: HandleRemediate returns existing RR via RRID lookup", func() {
		ctx := context.Background()
		ns := "default"

		rr := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"metadata": map[string]interface{}{
					"name":      "rr-existing-1332-w03",
					"namespace": ns,
				},
				"spec": map[string]interface{}{
					"description": "pre-seeded RR",
					"targetResource": map[string]interface{}{
						"kind":      "Deployment",
						"name":      "web-existing",
						"namespace": ns,
					},
				},
				"status": map[string]interface{}{
					"phase": "InProgress",
				},
			},
		}
		_, err := dynamicClient.Resource(rrGVR).Namespace(ns).Create(ctx, rr, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, "rr-existing-1332-w03", metav1.DeleteOptions{})
		})

		result, err := tools.HandleRemediate(ctx, dynamicClient, ns, &tools.RemediateArgs{
			RRID: "rr-existing-1332-w03",
		}, "it-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeTrue())
		Expect(result.RRID).To(Equal("rr-existing-1332-w03"))
	})

	It("IT-AF-1332-W04: HandleRemediate with non-existent RRID returns graceful not-found", func() {
		ctx := context.Background()
		ns := "default"

		result, err := tools.HandleRemediate(ctx, dynamicClient, ns, &tools.RemediateArgs{
			RRID: "rr-nonexistent-1332",
		}, "it-user", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeFalse())
		Expect(result.Message).To(ContainSubstring("not found"))
	})

	It("IT-AF-1332-W05: HandleRemediate with nil client returns ErrK8sUnavailable", func() {
		ctx := context.Background()

		_, err := tools.HandleRemediate(ctx, nil, "default", &tools.RemediateArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "web-nil",
			Description: "nil client test",
		}, "it-user", nil, nil)
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("IT-AF-1332-W06: HandleRemediate emits audit event via envtest", func() {
		ctx := context.Background()
		ns := "default"
		auditRecorder.Reset()

		result, err := tools.HandleRemediate(ctx, dynamicClient, ns, &tools.RemediateArgs{
			Namespace:   ns,
			Kind:        "Deployment",
			Name:        "web-1332-w06",
			Description: "audit wiring IT",
		}, "audit-user-1332", nil, auditRecorder)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).NotTo(BeEmpty())

		events := auditRecorder.EventsOfType(audit.EventRRCreated)
		Expect(events).To(HaveLen(1))
		Expect(events[0].UserID).To(Equal("audit-user-1332"))

		DeferCleanup(func() {
			_ = dynamicClient.Resource(rrGVR).Namespace(ns).Delete(ctx, result.RRID, metav1.DeleteOptions{})
		})
	})
})
