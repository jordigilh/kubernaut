package audit

import (
	"encoding/json"

	"github.com/go-faster/jx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-AUDIT-001 / BR-AUDIT-005: complete denial payload (AU-3, SI-10)", func() {
	DescribeTable("required array and protocol survive generated validation and serialization",
		func(endpoint string) {
			union := buildAuthAccessDeniedPayload(&Event{Type: EventAuthAccessDenied, Detail: map[string]string{
				"tool_name": "kubectl_exec", "user_role": "viewer", "endpoint": endpoint,
			}})
			Expect(union.Validate()).To(Succeed())
			payload, ok := union.GetApifrontendAuthAccessDeniedPayload()
			Expect(ok).To(BeTrue())
			Expect(string(payload.Endpoint)).To(Equal(endpoint))
			Expect(payload.RequiredRoles).ToNot(BeNil())
			Expect(payload.RequiredRoles).To(BeEmpty(), "unknown SAR policy roles must not be fabricated")
			Expect(payload.ToolName).To(Equal("kubectl_exec"))
			Expect(payload.UserRole).To(Equal("viewer"))
			var encoder jx.Encoder
			union.Encode(&encoder)
			var serialized struct {
				RequiredRoles json.RawMessage `json:"required_roles"`
				EventType     string          `json:"event_type"`
				ToolName      string          `json:"tool_name"`
				UserRole      string          `json:"user_role"`
				Endpoint      string          `json:"endpoint"`
			}
			Expect(json.Unmarshal(encoder.Bytes(), &serialized)).To(Succeed())
			Expect(string(serialized.RequiredRoles)).To(Equal("[]"))
			Expect(serialized.EventType).To(Equal("apifrontend.auth.access_denied"))
			Expect(serialized.ToolName).To(Equal("kubectl_exec"))
			Expect(serialized.UserRole).To(Equal("viewer"))
			Expect(serialized.Endpoint).To(Equal(endpoint))
		},
		Entry("UT-AF-2500-001/004: explicit mcp with truthful known tool and user role", "mcp"),
		Entry("UT-AF-2500-002/004: explicit a2a preserves protocol attribution", "a2a"),
	)
	DescribeTable("UT-AF-2500-003: empty/unknown protocols stay invalid, never defaulted",
		func(endpoint string) {
			union := buildAuthAccessDeniedPayload(&Event{Type: EventAuthAccessDenied, Detail: map[string]string{
				"tool_name": "kubectl_exec", "endpoint": endpoint,
			}})
			Expect(union.Validate()).ToNot(Succeed())
			payload, ok := union.GetApifrontendAuthAccessDeniedPayload()
			Expect(ok).To(BeTrue())
			Expect(string(payload.Endpoint)).To(Equal(endpoint))
		}, Entry("missing", ""), Entry("unsupported", "not-a-protocol"))
})
