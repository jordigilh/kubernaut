package launcher

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
)

// Issue #2716: production code carries no persona logic. The triage.started
// audit event must record actor attribution (user_id, session_id, task_id)
// without deriving a persona from JWT group membership.
var _ = Describe("Triage-started audit persona removal (#2716, BR-AUDIT-005 / AU-3)", func() {
	It("UT-AF-2716-002: buildBeforeExecuteCallback emits triage.started without a persona detail", func() {
		spy := &auditSpy{}
		before := buildBeforeExecuteCallback(nil, spy)
		_, err := before(context.Background(), &a2asrv.RequestContext{
			TaskID:    a2a.TaskID("task-2716"),
			ContextID: "session-2716",
		})
		Expect(err).NotTo(HaveOccurred())

		started := spy.eventsByType(audit.EventTriageStarted)
		Expect(started).To(HaveLen(1))
		Expect(started[0].Detail).To(HaveKeyWithValue("session_id", "session-2716"))
		Expect(started[0].Detail).To(HaveKeyWithValue("task_id", "task-2716"))
		Expect(started[0].Detail).NotTo(HaveKey("persona"),
			"Issue #2716: triage.started must not derive or carry persona framing; actor attribution lives in user_id")
	})
})
