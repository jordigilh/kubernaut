package handler

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("MCP bridge fleet audit attribution", func() {
	It("UT-AF-2429-007 [AU-3/CC7.2]: enriches bridge audit events from RR context", func() {
		ctx := launcher.WithEventBridge(context.Background(), nil, "task-2429-bridge", "context-2429-bridge", nil)
		launcher.SetRRContextSafe(ctx, &launcher.RRContext{RRID: "rr-bridge-2429", ClusterID: "remote-east"})
		recorder := &bridgeAuditRecorder2429{}

		emitAudit(ctx, &MCPBridgeConfig{Auditor: recorder}, "kubernaut_select_workflow", audit.EventToolExecuted, map[string]string{
			"rr_id": "rr-bridge-2429",
		})

		Expect(recorder.events).To(HaveLen(1))
		Expect(recorder.events[0].ClusterID).To(Equal("remote-east"))
	})
})

type bridgeAuditRecorder2429 struct {
	events []*audit.Event
}

func (r *bridgeAuditRecorder2429) Emit(_ context.Context, event *audit.Event) {
	r.events = append(r.events, event)
}
