package launcher

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
)

var _ = Describe("A2A failure audit reconstruction (#2444)", func() {
	It("IT-AF-2444-005 correlates task start and failure without emitting completion", func() {
		spy := &auditSpy{}
		before := buildBeforeExecuteCallback(nil, spy)
		ctx, err := before(context.Background(), &a2asrv.RequestContext{
			TaskID:    a2a.TaskID("task-2444"),
			ContextID: "session-2444",
		})
		Expect(err).NotTo(HaveOccurred())

		started := spy.eventsByType(audit.EventA2ATaskStarted)
		Expect(started).To(HaveLen(1))
		Expect(started[0].CorrelationID).To(Equal("session-2444"))
		Expect(started[0].RequestID).To(Equal("task-2444"))
		Expect(started[0].Detail).To(HaveKeyWithValue("session_id", "session-2444"))

		after := buildAfterExecuteCallback(logr.Discard(), spy)
		err = after(&stubExecutorContext{Context: ctx}, &a2a.TaskStatusUpdateEvent{
			TaskID: a2a.TaskID("task-2444"),
		}, errors.New("openaicompat: stream ended before [DONE]"))
		Expect(err).NotTo(HaveOccurred())

		failed := spy.eventsByType(audit.EventA2ATaskFailed)
		Expect(failed).To(HaveLen(1))
		Expect(failed[0].CorrelationID).To(Equal("session-2444"))
		Expect(failed[0].RequestID).To(Equal("task-2444"))
		Expect(failed[0].Detail).To(HaveKeyWithValue("session_id", "session-2444"))
		Expect(failed[0].ErrorDetails).NotTo(BeNil())
		Expect(failed[0].ErrorDetails.Code).To(Equal("ERR_UPSTREAM_FAILURE"))
		Expect(failed[0].ErrorDetails.Component).To(Equal("apifrontend"))
		Expect(spy.eventsByType(audit.EventA2ATaskCompleted)).To(BeEmpty())
	})

	It("IT-AF-2444-005 correlates successful task completion", func() {
		spy := &auditSpy{}
		before := buildBeforeExecuteCallback(nil, spy)
		ctx, err := before(context.Background(), &a2asrv.RequestContext{
			TaskID:    a2a.TaskID("task-2444-ok"),
			ContextID: "session-2444-ok",
		})
		Expect(err).NotTo(HaveOccurred())

		after := buildAfterExecuteCallback(logr.Discard(), spy)
		err = after(&stubExecutorContext{Context: ctx}, &a2a.TaskStatusUpdateEvent{
			TaskID: a2a.TaskID("task-2444-ok"),
		}, nil)
		Expect(err).NotTo(HaveOccurred())

		completed := spy.eventsByType(audit.EventA2ATaskCompleted)
		Expect(completed).To(HaveLen(1))
		Expect(completed[0].CorrelationID).To(Equal("session-2444-ok"))
		Expect(completed[0].RequestID).To(Equal("task-2444-ok"))
		Expect(completed[0].Detail).To(HaveKeyWithValue("session_id", "session-2444-ok"))
	})
})
