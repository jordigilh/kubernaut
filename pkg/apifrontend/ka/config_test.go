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
