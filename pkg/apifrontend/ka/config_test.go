package ka_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("ParseDiscoverWorkflowsResponse — name mapping", func() {
	Describe("UT-AF-1408-030: mapKAWorkflow uses catalog-provided Name, not WorkflowID", func() {
		It("should map Name from KA response when provided", func() {
			raw := json.RawMessage(`{
				"session_id": "sess-1",
				"status": "workflows_discovered",
				"response": "{\"recommended\":{\"workflow_id\":\"abc-123-uuid\",\"name\":\"Increase Memory Limit\",\"confidence\":0.95,\"rationale\":\"Container OOMKilled\"},\"alternatives\":[]}"
			}`)
			result, err := ka.ParseDiscoverWorkflowsResponse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Workflows).To(HaveLen(1))
			Expect(result.Workflows[0].Name).To(Equal("Increase Memory Limit"))
			Expect(result.Workflows[0].WorkflowID).To(Equal("abc-123-uuid"))
		})
	})

	Describe("UT-AF-1408-031: mapKAWorkflow does NOT fall back to WorkflowID when Name is empty", func() {
		It("should leave Name empty when KA omits it", func() {
			raw := json.RawMessage(`{
				"session_id": "sess-2",
				"status": "workflows_discovered",
				"response": "{\"recommended\":{\"workflow_id\":\"abc-123-uuid\",\"confidence\":0.85,\"rationale\":\"Pod restart needed\"},\"alternatives\":[]}"
			}`)
			result, err := ka.ParseDiscoverWorkflowsResponse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Workflows).To(HaveLen(1))
			Expect(result.Workflows[0].Name).To(BeEmpty(), "Name must NOT fall back to WorkflowID")
			Expect(result.Workflows[0].WorkflowID).To(Equal("abc-123-uuid"))
		})
	})
})

var _ = Describe("Issue #1437: ParseDiscoverWorkflowsResponse — target propagation", func() {

	Describe("UT-AF-1437-001: KA envelope with divergent targets", func() {
		It("should parse searched_target and signal_target from KA response", func() {
			raw := json.RawMessage(`{
				"session_id": "sess-1437-001",
				"status": "workflows_discovered",
				"response": "{\"recommended\":{\"workflow_id\":\"fix-config\",\"name\":\"Fix Config\",\"confidence\":0.85,\"rationale\":\"ConfigMap misconfigured\"},\"searched_target\":{\"api_version\":\"v1\",\"kind\":\"ConfigMap\",\"name\":\"worker-config\",\"namespace\":\"demo-storefront\"},\"signal_target\":{\"api_version\":\"apps/v1\",\"kind\":\"Deployment\",\"name\":\"worker\",\"namespace\":\"demo-storefront\"}}"
			}`)
			result, err := ka.ParseDiscoverWorkflowsResponse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.SearchedTarget).NotTo(BeNil(), "SearchedTarget must be propagated")
			Expect(result.SearchedTarget.Kind).To(Equal("ConfigMap"))
			Expect(result.SearchedTarget.Name).To(Equal("worker-config"))
			Expect(result.SearchedTarget.Namespace).To(Equal("demo-storefront"))
			Expect(result.SearchedTarget.APIVersion).To(Equal("v1"))

			Expect(result.SignalTarget).NotTo(BeNil(), "SignalTarget must be propagated")
			Expect(result.SignalTarget.Kind).To(Equal("Deployment"))
			Expect(result.SignalTarget.Name).To(Equal("worker"))
			Expect(result.SignalTarget.Namespace).To(Equal("demo-storefront"))
			Expect(result.SignalTarget.APIVersion).To(Equal("apps/v1"))
		})
	})

	Describe("UT-AF-1437-002: KA envelope with identical targets", func() {
		It("should parse both targets when they are the same resource", func() {
			raw := json.RawMessage(`{
				"session_id": "sess-1437-002",
				"status": "workflows_discovered",
				"response": "{\"recommended\":{\"workflow_id\":\"restart-deploy\",\"name\":\"Restart Deployment\",\"confidence\":0.9,\"rationale\":\"OOMKilled\"},\"searched_target\":{\"api_version\":\"apps/v1\",\"kind\":\"Deployment\",\"name\":\"worker\",\"namespace\":\"demo-storefront\"},\"signal_target\":{\"api_version\":\"apps/v1\",\"kind\":\"Deployment\",\"name\":\"worker\",\"namespace\":\"demo-storefront\"}}"
			}`)
			result, err := ka.ParseDiscoverWorkflowsResponse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.SearchedTarget).NotTo(BeNil())
			Expect(result.SignalTarget).NotTo(BeNil())
			Expect(result.SearchedTarget.Kind).To(Equal("Deployment"))
			Expect(result.SignalTarget.Kind).To(Equal("Deployment"))
			Expect(result.SearchedTarget.APIVersion).To(Equal(result.SignalTarget.APIVersion))
			Expect(result.SearchedTarget.Name).To(Equal(result.SignalTarget.Name))
		})
	})
})
