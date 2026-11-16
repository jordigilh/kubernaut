package main

import (
	"testing"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkflowExecutionScheme(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution production scheme wiring")
}

var _ = Describe("Cancellation parent watch startup (BR-ORCH-029)", func() {
	It("IT-WE-2588-026: registers parent RR objects and lists in the production scheme", func() {
		for _, kind := range []string{"RemediationRequest", "RemediationRequestList"} {
			gvk := remediationv1.GroupVersion.WithKind(kind)
			obj, err := scheme.New(gvk)
			Expect(err).NotTo(HaveOccurred(), "the parent watch must resolve %s before manager startup", kind)
			gvks, _, err := scheme.ObjectKinds(obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(gvks).To(ContainElement(gvk))
		}
	})
})
