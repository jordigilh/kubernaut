package launcher_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("Schema Validation (#1399)", func() {
	It("UT-AF-1399-012: ValidatePayload returns nil for valid investigation_summary", func() {
		data := map[string]any{
			"session_id": "sess-001",
			"summary":    "OOMKill detected in production pod",
			"rca": map[string]any{
				"explanation": "Memory leak in data-processor worker goroutine caused OOM",
			},
		}
		err := launcher.ValidatePayloadForTest("investigation_summary", data)
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-1399-013: ValidatePayload returns error for missing required field (rca)", func() {
		data := map[string]any{
			"session_id": "sess-001",
			"summary":    "OOMKill detected in production pod",
		}
		err := launcher.ValidatePayloadForTest("investigation_summary", data)
		Expect(err).To(HaveOccurred(), "SI-10: missing required field 'rca' must be caught")
		Expect(err.Error()).To(ContainSubstring("rca"))
	})

	It("UT-AF-1399-014: Schema validation failure triggers graceful degradation", func() {
		data := map[string]any{
			"summary": "Missing required fields",
		}
		err := launcher.ValidatePayloadForTest("investigation_summary", data)
		Expect(err).To(HaveOccurred(),
			"SI-17: schema failure must produce an error for graceful degradation logic")
	})
})
