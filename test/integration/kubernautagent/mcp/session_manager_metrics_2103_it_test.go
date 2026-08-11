/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcp_test

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

// recordingSessionEndedMetricsIT is a concurrency-safe SessionEndedMetrics
// recorder for IT assertions -- the SessionJanitor's sweep (IT-KA-2103-002)
// fires the callback from its own background goroutine, so a plain int
// counter would race with the test's read.
type recordingSessionEndedMetricsIT struct {
	endedCount atomic.Int64
}

func (m *recordingSessionEndedMetricsIT) RecordInteractiveSessionEnded() {
	m.endedCount.Add(1)
}

// #2103: aiagent_mcp_interactive_sessions_active drifted upward over time
// because several production Release() callers -- CompleteNoActionTool,
// SelectWorkflowTool's async release, InvestigateTool's no_matching_workflows
// auto-close, and the SessionJanitor's onExpire backstop -- released the
// underlying Lease correctly but never called RecordInteractiveSessionEnded,
// so the gauge never came back down even though capacity was reclaimed.
//
// The fix centralizes the decrement inside LeaseSessionManager.Release()
// itself (WithSessionEndedMetrics), so every current and future caller gets
// it for free. These IT tests exercise that production wiring against a
// real envtest API server and real coordination.k8s.io/Lease objects -- the
// same wiring buildMCPHandler (cmd/kubernautagent/main.go) uses -- covering
// both call shapes that existed pre-fix:
//   - an explicit Release() call from a tool handler (stands in for
//     complete_no_action / workflow_selected / no_matching_workflows, which
//     all bottom out in the same mgr.Release(sessionID, reason) call with
//     only the reason string differing)
//   - the SessionJanitor's onExpire callback, which also routes through
//     Release() (janitor_expired)
var _ = Describe("LeaseSessionManager + SessionEndedMetrics wiring IT — #2103 SI-4 (metrics integrity)", Label("integration", "interactive"), func() {

	Describe("IT-KA-2103-001: an explicit Release call decrements the wired gauge exactly once (complete_no_action / workflow_selected / no_matching_workflows shape)", func() {
		It("should call RecordInteractiveSessionEnded exactly once per real Release, regardless of the reason string", func() {
			nsName := uniqueNamespace("metrics-2103-001")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			logger := logr.Discard()
			metrics := &recordingSessionEndedMetricsIT{}
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger,
				mcpinternal.WithSessionEndedMetrics(metrics))

			for i, reason := range []string{"complete_no_action", "workflow_selected", "no_matching_workflows"} {
				user := mcpinternal.UserInfo{Username: "sre-2103-001@example.com", Groups: []string{"sre"}}
				sess, err := mgr.Takeover(context.Background(), "rr-2103-001", user)
				Expect(err).NotTo(HaveOccurred())

				Expect(mgr.Release(sess.SessionID, reason)).To(Succeed())
				Expect(metrics.endedCount.Load()).To(Equal(int64(i+1)),
					"#2103: Release(reason=%q) must call the wired SessionEndedMetrics through the real "+
						"production chokepoint, same as every other Release() caller", reason)
			}
		})
	})

	Describe("IT-KA-2103-002: the SessionJanitor's onExpire sweep decrements the wired gauge too (janitor_expired shape)", func() {
		It("should have the janitor's backstop Release call the wired SessionEndedMetrics, closing the original SI-4 gap", func() {
			nsName := uniqueNamespace("metrics-2103-002")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			logger := logr.Discard()
			metrics := &recordingSessionEndedMetricsIT{}
			janitor := mcpinternal.NewSessionJanitor(100*time.Millisecond, logger)
			janitorCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go janitor.Run(janitorCtx)

			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger,
				mcpinternal.WithSessionJanitor(janitor),
				mcpinternal.WithSessionEndedMetrics(metrics))

			user := mcpinternal.UserInfo{Username: "sre-2103-002@example.com", Groups: []string{"sre"}}
			_, err := mgr.Takeover(context.Background(), "rr-2103-002", user)
			Expect(err).NotTo(HaveOccurred())

			Expect(metrics.endedCount.Load()).To(Equal(int64(0)), "must not decrement before the janitor sweeps")

			Eventually(func() *mcpinternal.InteractiveSession {
				driver, _ := mgr.GetDriver("rr-2103-002")
				return driver
			}, 5*time.Second, 50*time.Millisecond).Should(BeNil(),
				"#2100: the orphaned session must be reclaimed by the janitor's backstop sweep")

			Eventually(func() int64 {
				return metrics.endedCount.Load()
			}, 5*time.Second, 50*time.Millisecond).Should(Equal(int64(1)),
				"#2103: the janitor's onExpire callback routes through the same Release() chokepoint, "+
					"so it must decrement the gauge too -- pre-fix this path never did, driving the metric-only drift")
		})
	})
})
