package tools

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
)

var _ = Describe("APIFrontend RR audit cluster authority", func() {
	It("UT-AF-2429-005 [AU-3/CC7.2]: uses the returned RR cluster on create", func() {
		recorder := &rrAuditRecorder2429{}
		deps := &ToolDeps{Auditor: recorder, ControllerNS: "kubernaut-system"}
		args := &CreateRRArgs{Namespace: "prod", Kind: "Deployment", Name: "api", ClusterID: "ambient-hint"}
		result := &CreateRRResult{RRID: "rr-created-2429", ClusterID: "remote-east"}

		emitCreateRRAudit(context.Background(), deps, args, "alice", result, "critical")

		Expect(recorder.events).To(HaveLen(1))
		Expect(recorder.events[0].ClusterID).To(Equal("remote-east"))
	})

	It("UT-AF-2429-006 [AU-3/CC7.2]: uses the existing RR cluster on deduplication", func() {
		recorder := &rrAuditRecorder2429{}
		deps := &ToolDeps{Auditor: recorder, ControllerNS: "kubernaut-system"}
		args := &CreateRRArgs{Namespace: "prod", Kind: "Deployment", Name: "api", ClusterID: "stale-hint"}
		result := &CreateRRResult{RRID: "rr-existing-2429", AlreadyExists: true, ClusterID: "remote-west"}

		emitCreateRRAudit(context.Background(), deps, args, "alice", result, "critical")

		Expect(recorder.events).To(HaveLen(1))
		Expect(recorder.events[0].ClusterID).To(Equal("remote-west"))
	})
})

type rrAuditRecorder2429 struct {
	events []*audit.Event
}

func (r *rrAuditRecorder2429) Emit(_ context.Context, event *audit.Event) {
	r.events = append(r.events, event)
}
