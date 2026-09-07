package tools_test

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// #2364: fleet-hinted turns die at ADK strict schema validation because the
// model propagates ambient fields (cluster_id from the console fleet hint,
// rr_id/session_id from prompt.txt's preserve-across-phases instruction) into
// tool calls whose schemas declare none of them. Every struct below must
// tolerate its ambient fields as ignored omitempty hints.
var _ = Describe("ambient fleet-field tolerance (#2364)", func() {
	It("UT-2364-001: SI-10 PresentDecisionArgs schema tolerates cluster_id/rr_id", func() {
		schema, err := jsonschema.For[tools.PresentDecisionArgs](nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(schema.Properties).To(HaveKey("cluster_id"),
			"fleet-hinted present_decision calls carry cluster_id -- strict schema must tolerate it (#2364)")
		Expect(schema.Properties).To(HaveKey("rr_id"),
			"prompt.txt instructs rr_id preservation -- present_decision must tolerate it (#2364)")
		Expect(schema.Required).NotTo(ContainElement("cluster_id"))
		Expect(schema.Required).NotTo(ContainElement("rr_id"))
		Expect(schema.Required).To(ContainElement("session_id"),
			"fields the LLM IS expected to supply must remain required")
		Expect(schema.Required).To(ContainElement("summary"))
		Expect(schema.Required).To(ContainElement("rca"))
	})

	It("UT-2364-002: SI-10 discover/select schemas tolerate cluster_id/session_id", func() {
		discSchema, err := jsonschema.For[tools.DiscoverWorkflowsArgs](nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(discSchema.Properties).To(HaveKey("cluster_id"))
		Expect(discSchema.Properties).To(HaveKey("session_id"))
		Expect(discSchema.Required).NotTo(ContainElement("cluster_id"))
		Expect(discSchema.Required).NotTo(ContainElement("session_id"))
		Expect(discSchema.Required).To(ContainElement("rr_id"))

		selSchema, err := jsonschema.For[tools.SelectWorkflowArgs](nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(selSchema.Properties).To(HaveKey("cluster_id"))
		Expect(selSchema.Properties).To(HaveKey("session_id"))
		Expect(selSchema.Required).NotTo(ContainElement("cluster_id"))
		Expect(selSchema.Required).NotTo(ContainElement("session_id"))
		Expect(selSchema.Required).To(ContainElement("rr_id"))
		Expect(selSchema.Required).To(ContainElement("workflow_id"))
	})

	It("UT-2364-003: SI-10 WatchArgs schema tolerates cluster_id/session_id", func() {
		schema, err := jsonschema.For[tools.WatchArgs](nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(schema.Properties).To(HaveKey("cluster_id"))
		Expect(schema.Properties).To(HaveKey("session_id"))
		Expect(schema.Required).NotTo(ContainElement("cluster_id"))
		Expect(schema.Required).NotTo(ContainElement("session_id"))
	})

	It("UT-2364-004: SI-10 Tier-2 schemas tolerate the ambient trio", func() {
		investSchema, err := jsonschema.For[tools.InvestigateMCPArgs](nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(investSchema.Properties).To(HaveKey("session_id"),
			"prompt.txt preservation contaminates the most-called tool (#2364 Tier 2)")

		for name, schema := range map[string]*jsonschema.Schema{
			"InteractiveActionArgs": mustSchema[tools.InteractiveActionArgs](),
			"CompleteNoActionArgs":  mustSchema[tools.CompleteNoActionArgs](),
			"GetRemediationArgs":    mustSchema[tools.GetRemediationArgs](),
			"CancelRemediationArgs": mustSchema[tools.CancelRemediationArgs](),
			"GetAuditTrailArgs":     mustSchema[tools.GetAuditTrailArgs](),
			"ListEventsArgs":        mustSchema[tools.ListEventsArgs](),
		} {
			Expect(schema.Properties).To(HaveKey("cluster_id"), "%s must tolerate fleet ambient fields", name)
			Expect(schema.Required).NotTo(ContainElement("cluster_id"), "%s must not require ambient fields", name)
		}
	})

	It("UT-2364-N01: SI-10 nested RCAData/TargetInfo tolerate cluster_id", func() {
		rcaSchema, err := jsonschema.For[tools.RCAData](nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(rcaSchema.Properties).To(HaveKey("cluster_id"),
			"jsonschema-go rejects unknown NESTED properties too (infer.go) -- spoke-scoped RCA must validate")
		Expect(rcaSchema.Required).NotTo(ContainElement("cluster_id"))
		Expect(rcaSchema.Required).To(ContainElement("severity"))

		targetSchema, err := jsonschema.For[tools.TargetInfo](nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(targetSchema.Properties).To(HaveKey("cluster_id"))
		Expect(targetSchema.Required).NotTo(ContainElement("cluster_id"))
		Expect(targetSchema.Required).To(ContainElement("kind"))
	})

	It("UT-2364-S01: AU-3 ambient fields omit when empty, round-trip when set", func() {
		args := tools.PresentDecisionArgs{SessionID: "sess-1", Summary: "s"}
		data, err := json.Marshal(args)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring("cluster_id"))
		Expect(string(data)).NotTo(ContainSubstring("rr_id"))

		withAmbient := tools.PresentDecisionArgs{
			SessionID: "sess-1", Summary: "s",
			ClusterID: "spoke", RRID: "ns/rr-1",
		}
		data2, err := json.Marshal(withAmbient)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data2)).To(ContainSubstring(`"cluster_id":"spoke"`))
		Expect(string(data2)).To(ContainSubstring(`"rr_id":"ns/rr-1"`))
		var decoded tools.PresentDecisionArgs
		Expect(json.Unmarshal(data2, &decoded)).To(Succeed())
		Expect(decoded.ClusterID).To(Equal("spoke"))
		Expect(decoded.RRID).To(Equal("ns/rr-1"))
	})

	It("UT-2364-H01: AC-6 HandlePresentDecision ignores ambient extras", func() {
		plain := tools.HandlePresentDecision(tools.PresentDecisionArgs{
			SessionID: "sess-1", Summary: "leak",
			Options: []tools.WorkflowOption{{WorkflowID: "wf-1", Name: "Restart"}},
		})
		hinted := tools.HandlePresentDecision(tools.PresentDecisionArgs{
			SessionID: "sess-1", Summary: "leak",
			Options:   []tools.WorkflowOption{{WorkflowID: "wf-1", Name: "Restart"}},
			ClusterID: "spoke", RRID: "ns/rr-1",
		})
		Expect(hinted.Message).To(Equal(plain.Message),
			"ambient hints must never alter presentation -- hints only, never routing (AC-6)")
	})

	It("UT-2364-H02: AC-6 HandleDiscoverWorkflows ignores ambient extras", func() {
		ctx := context.Background()
		newMock := func() *ka.MockMCPClient {
			return &ka.MockMCPClient{
				DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
					return &ka.DiscoverWorkflowsResult{Workflows: []ka.DiscoveredWorkflow{
						{WorkflowID: "wf-1", Name: "Restart"},
					}}, nil
				},
			}
		}
		plain, err := tools.HandleDiscoverWorkflows(ctx, newMock(), tools.DiscoverWorkflowsArgs{RRID: "ns/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		hinted, err := tools.HandleDiscoverWorkflows(ctx, newMock(), tools.DiscoverWorkflowsArgs{
			RRID: "ns/rr-1", ClusterID: "spoke", SessionID: "sess-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(hinted).To(Equal(plain))
	})

	It("UT-2364-H03: AC-6 HandleSelectWorkflow ignores ambient extras", func() {
		ctx := context.Background()
		newMock := func() *ka.MockMCPClient {
			return &ka.MockMCPClient{
				SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
					return &ka.SelectWorkflowResult{Status: "selected"}, nil
				},
			}
		}
		plain, err := tools.HandleSelectWorkflow(ctx, newMock(), tools.SelectWorkflowArgs{
			RRID: "ns/rr-1", WorkflowID: "wf-1",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		hinted, err := tools.HandleSelectWorkflow(ctx, newMock(), tools.SelectWorkflowArgs{
			RRID: "ns/rr-1", WorkflowID: "wf-1", ClusterID: "spoke", SessionID: "sess-1",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(hinted).To(Equal(plain))
	})
})

func mustSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	Expect(err).NotTo(HaveOccurred())
	return schema
}
