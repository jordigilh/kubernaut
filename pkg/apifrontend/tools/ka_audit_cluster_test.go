package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("APIFrontend fleet audit attribution", func() {
	It("UT-AF-2429-003 [AU-3/CC7.2]: attributes workflow selection to the RR cluster", func() {
		ctx := launcher.WithEventBridge(context.Background(), nil, "task-2429-select", "context-2429-select", nil)
		launcher.SetRRContextSafe(ctx, &launcher.RRContext{RRID: "rr-select-2429", ClusterID: "remote-east"})
		auditor := &auditRecorderFor2429{}
		mcpClient := &ka.MockMCPClient{
			SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				return &ka.SelectWorkflowResult{Status: "selected"}, nil
			},
		}

		_, err := tools.HandleSelectWorkflow(ctx, mcpClient, tools.SelectWorkflowArgs{
			RRID: "rr-select-2429", WorkflowID: "workflow-1", ClusterID: "untrusted-ambient-hint",
		}, auditor)
		Expect(err).NotTo(HaveOccurred())
		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].ClusterID).To(Equal("remote-east"))
	})

	It("UT-AF-2429-004 [AU-3/CC7.2]: attributes operator dismissal to the RR cluster", func() {
		ctx := launcher.WithEventBridge(context.Background(), nil, "task-2429-result", "context-2429-result", nil)
		launcher.SetRRContextSafe(ctx, &launcher.RRContext{RRID: "rr-result-2429", ClusterID: "remote-west"})
		auditor := &auditRecorderFor2429{}
		mcpClient := &ka.MockMCPClient{
			CompleteNoActionFn: func(_ context.Context, _ ka.CompleteNoActionArgs) (*ka.CompleteNoActionResult, error) {
				return &ka.CompleteNoActionResult{Status: "completed"}, nil
			},
		}

		_, err := tools.HandleCompleteNoAction(ctx, mcpClient, tools.CompleteNoActionArgs{
			RRID: "rr-result-2429", Reason: "operator dismissed",
		}, auditor)
		Expect(err).NotTo(HaveOccurred())
		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].ClusterID).To(Equal("remote-west"))
	})
})

type auditRecorderFor2429 struct {
	events []*audit.Event
}

func (r *auditRecorderFor2429) Emit(_ context.Context, event *audit.Event) {
	r.events = append(r.events, event)
}
