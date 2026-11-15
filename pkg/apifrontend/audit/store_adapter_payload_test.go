package audit

import (
	"encoding/json"

	"github.com/go-faster/jx"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-AUDIT-001 / BR-AUDIT-005: complete denial payload (AU-3, SI-10)", func() {
	It("UT-AUD-2570-036: BR-AUDIT-005 preserves approval intent in the typed tool payload", func() {
		rationale := "reviewed"
		union := buildToolExecutedPayload(&Event{Type: EventToolExecuted, Approval: &ApprovalAudit{WorkflowID: "wf-reviewed", OriginalParameters: typedParameters(`{"P":2,"Q":"keep"}`), OverrideParameters: typedParameters(`{"P":1.0}`), EffectiveParameters: typedParameters(`{"P":1.0,"Q":"keep"}`), ParameterEvidence: `{"P":2,"Q":"keep"}`, OverrideRationale: &rationale}, Detail: map[string]string{
			"tool_name": "kubernaut_approve", "tool_outcome": "success", "workflow_id": "wf-reviewed", "approval_request": "rar-audit-2570", "approval_decision": "Approved",
			"original_parameters": `{"P":"2","Q":"keep"}`, "override_parameters": `{"P":"1.0"}`,
			"effective_parameters": `{"P":"1.0","Q":"keep"}`, "override_rationale": "reviewed",
		}})
		Expect(union.Validate()).To(Succeed())
		var encoder jx.Encoder
		union.Encode(&encoder)
		var fields map[string]json.RawMessage
		Expect(json.Unmarshal(encoder.Bytes(), &fields)).To(Succeed())
		Expect(string(fields["workflow_id"])).To(MatchJSON(`"wf-reviewed"`))
		Expect(string(fields["approval_request"])).To(MatchJSON(`"rar-audit-2570"`))
		Expect(string(fields["approval_decision"])).To(MatchJSON(`"Approved"`))
		Expect(string(fields["original_parameters"])).To(MatchJSON(`{"P":{"kind":"integer","integerValue":2},"Q":{"kind":"string","stringValue":"keep"}}`))
		Expect(string(fields["override_parameters"])).To(MatchJSON(`{"P":{"kind":"float","floatValue":1}}`))
		Expect(string(fields["effective_parameters"])).To(MatchJSON(`{"P":{"kind":"float","floatValue":1},"Q":{"kind":"string","stringValue":"keep"}}`))
		Expect(string(fields["parameter_evidence"])).To(MatchJSON(`"{\"P\":2,\"Q\":\"keep\"}"`))
		Expect(string(fields["override_rationale"])).To(MatchJSON(`"reviewed"`))
	})

	It("UT-AUD-2570-038: BR-AUDIT-005 distinguishes no override from an explicit empty override", func() {
		event := &Event{Type: EventToolExecuted, Detail: map[string]string{"tool_name": "kubernaut_approve", "tool_outcome": "success"}, Approval: &ApprovalAudit{OriginalParameters: typedParameters(`{}`), EffectiveParameters: typedParameters(`{}`)}}
		without := buildToolExecutedPayload(event)
		payload, ok := without.GetApifrontendToolExecutedPayload()
		Expect(ok).To(BeTrue())
		_, present := payload.OverrideParameters.Get()
		Expect(present).To(BeFalse())
		_, present = payload.OverrideRationale.Get()
		Expect(present).To(BeFalse())
		rationale := ""
		event.Approval.OverrideParameters = typedParameters(`{}`)
		event.Approval.OverrideRationale = &rationale
		with := buildToolExecutedPayload(event)
		payload, ok = with.GetApifrontendToolExecutedPayload()
		Expect(ok).To(BeTrue())
		values, present := payload.OverrideParameters.Get()
		Expect(present).To(BeTrue())
		Expect(values).To(BeEmpty())
		_, present = payload.OverrideRationale.Get()
		Expect(present).To(BeTrue())
	})
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

func typedParameters(raw string) sharedtypes.TypedParameterMap {
	values, _, err := sharedtypes.DecodeTypedParameterMap([]byte(raw))
	if err != nil {
		panic(err)
	}
	return values
}
