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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

// recordingSessionEndedMetrics captures RecordInteractiveSessionEnded calls
// for assertion, implementing mcpinternal.SessionEndedMetrics.
type recordingSessionEndedMetrics struct {
	endedCount int
}

func (m *recordingSessionEndedMetrics) RecordInteractiveSessionEnded() {
	m.endedCount++
}

// #2103: aiagent_mcp_interactive_sessions_active only ever decremented for
// two of the ways an interactive session can end (explicit complete/cancel)
// plus two callback paths owned directly by cmd/kubernautagent/main.go.
// Every other completion path (complete_no_action, workflow_selected,
// no_matching_workflows, and #2100's own new SessionJanitor backstop) called
// LeaseSessionManager.Release() without ever decrementing the gauge, because
// the decrement was the caller's responsibility and most callers never wired
// agentMetrics at all. These tests prove the fix: centralizing the decrement
// inside Release() itself, paired with the existing activeCount.Add(-1), so
// every current and future Release() caller keeps the gauge accurate without
// needing its own metrics field.
var _ = Describe("LeaseSessionManager.Release centralized metrics decrement — #2103", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		namespace string
		logger    logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "kubernaut-system"
		logger = logr.Discard()

		scheme := runtime.NewScheme()
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	Describe("UT-KA-2103-001: Release calls the wired metrics callback exactly once on success", func() {
		It("should decrement the metrics recorder when the session is found and released", func() {
			metrics := &recordingSessionEndedMetrics{}
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionEndedMetrics(metrics))

			user := mcpinternal.UserInfo{Username: "alice@example.com", Groups: []string{"sre"}}
			sess, err := mgr.Takeover(ctx, "rr-2103-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(metrics.endedCount).To(Equal(0), "must not decrement before Release is called")

			Expect(mgr.Release(sess.SessionID, "complete_no_action")).To(Succeed())
			Expect(metrics.endedCount).To(Equal(1),
				"#2103: Release must call the wired SessionEndedMetrics exactly once, "+
					"regardless of which caller/reason triggered it")
		})
	})

	Describe("UT-KA-2103-002 (regression guard): Release on an unknown session ID never decrements", func() {
		It("should return ErrSessionNotFound and leave the metrics recorder untouched", func() {
			metrics := &recordingSessionEndedMetrics{}
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionEndedMetrics(metrics))

			err := mgr.Release("nonexistent-session-id", "complete_no_action")
			Expect(err).To(MatchError(mcpinternal.ErrSessionNotFound))
			Expect(metrics.endedCount).To(Equal(0),
				"#2103: a session that was never tracked (or already released) must not phantom-decrement the gauge")
		})
	})

	Describe("UT-KA-2103-003 (regression guard): Release with no metrics wired does not panic", func() {
		It("should complete the release successfully with a nil SessionEndedMetrics (matches every pre-#2103 call site)", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger)

			user := mcpinternal.UserInfo{Username: "bob@example.com", Groups: []string{"sre"}}
			sess, err := mgr.Takeover(ctx, "rr-2103-003", user)
			Expect(err).NotTo(HaveOccurred())

			Expect(func() {
				Expect(mgr.Release(sess.SessionID, "complete_no_action")).To(Succeed())
			}).NotTo(Panic(), "#2103: WithSessionEndedMetrics is optional -- Release must stay nil-safe")
		})
	})

	Describe("UT-KA-2103-004 (regression guard): a double Release only decrements once", func() {
		It("should decrement exactly once even if Release is called twice for the same session ID", func() {
			metrics := &recordingSessionEndedMetrics{}
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionEndedMetrics(metrics))

			user := mcpinternal.UserInfo{Username: "carol@example.com", Groups: []string{"sre"}}
			sess, err := mgr.Takeover(ctx, "rr-2103-004", user)
			Expect(err).NotTo(HaveOccurred())

			Expect(mgr.Release(sess.SessionID, "complete")).To(Succeed())
			Expect(metrics.endedCount).To(Equal(1))

			secondErr := mgr.Release(sess.SessionID, "explicit")
			Expect(secondErr).To(MatchError(mcpinternal.ErrSessionNotFound))
			Expect(metrics.endedCount).To(Equal(1),
				"#2103: the second (redundant) Release call must not double-decrement")
		})
	})
})
