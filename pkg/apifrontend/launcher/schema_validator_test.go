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

var _ = Describe("Issue #1437: searched_target and signal_target schema validation", func() {

	It("UT-AF-1437-003: accepts valid structured searched_target and signal_target", func() {
		data := map[string]any{
			"session_id": "sess-1437-003",
			"summary":    "ConfigMap misconfiguration caused Deployment failure",
			"rca": map[string]any{
				"explanation": "ConfigMap worker-config has invalid key causing BackOff",
			},
			"searched_target": map[string]any{
				"api_version": "v1",
				"kind":        "ConfigMap",
				"name":        "worker-config",
				"namespace":   "demo-storefront",
			},
			"signal_target": map[string]any{
				"api_version": "apps/v1",
				"kind":        "Deployment",
				"name":        "worker",
				"namespace":   "demo-storefront",
			},
		}
		err := launcher.ValidatePayloadForTest("investigation_summary", data)
		Expect(err).NotTo(HaveOccurred(),
			"valid structured target objects must pass schema validation")
	})

	It("UT-AF-1437-003-neg: rejects searched_target when it is a plain string", func() {
		data := map[string]any{
			"session_id": "sess-1437-003-neg",
			"summary":    "ConfigMap misconfiguration",
			"rca": map[string]any{
				"explanation": "ConfigMap worker-config has invalid key",
			},
			"searched_target": "v1/ConfigMap/worker-config",
		}
		err := launcher.ValidatePayloadForTest("investigation_summary", data)
		Expect(err).To(HaveOccurred(),
			"searched_target must be a structured object, not a plain string")
	})

	It("UT-AF-1437-003-neg2: rejects signal_target when it is a plain string", func() {
		data := map[string]any{
			"session_id": "sess-1437-003-neg2",
			"summary":    "Deployment failure",
			"rca": map[string]any{
				"explanation": "Deployment worker is failing",
			},
			"signal_target": "apps/v1/Deployment/worker",
		}
		err := launcher.ValidatePayloadForTest("investigation_summary", data)
		Expect(err).To(HaveOccurred(),
			"signal_target must be a structured object, not a plain string")
	})
})
