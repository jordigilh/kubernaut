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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

// #2100: IT-KA-DES-005/006 (disconnect_handler_test.go) already prove
// SessionJanitor's own Track/sweep/Untrack logic in isolation — but
// NewSessionJanitor had zero production callers (confirmed via
// grep -rn NewSessionJanitor --include=*.go .), so that coverage never
// proved the janitor could ever fire for a real interactive session.
// These tests exercise the actual chokepoint wiring added to
// LeaseSessionManager.Takeover/Release (session_manager.go's
// WithSessionJanitor option) against a real envtest API server and real
// coordination.k8s.io/Lease objects — the same wiring buildMCPHandler
// (cmd/kubernautagent/main.go) uses in production.
var _ = Describe("LeaseSessionManager + SessionJanitor wiring IT — #2100 SC-5/AU-12", Label("integration", "interactive"), func() {

	Describe("IT-KA-2100-001: Takeover Tracks the session with the janitor; Release Untracks it (real Lease)", func() {
		It("should leave no Lease behind after a clean Release, and never let the janitor fire for it", func() {
			nsName := uniqueNamespace("janitor-2100-001")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			logger := logr.Discard()
			// Long interval: the janitor must never sweep during this test —
			// proves Release's Untrack call, not the sweep's own expiry logic
			// (already covered by IT-KA-DES-005/006).
			janitor := mcpinternal.NewSessionJanitor(1*time.Hour, logger)
			janitorCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go janitor.Run(janitorCtx)

			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger,
				mcpinternal.WithSessionJanitor(janitor))

			user := mcpinternal.UserInfo{Username: "sre-2100-001@example.com", Groups: []string{"sre"}}
			sess, err := mgr.Takeover(context.Background(), "rr-2100-001", user)
			Expect(err).NotTo(HaveOccurred())

			Expect(janitor.TrackedForTest(sess.SessionID)).To(BeTrue(),
				"#2100: Takeover success must Track the session with the janitor via the real production wiring path")

			leaseList := &coordinationv1.LeaseList{}
			Expect(sharedK8sClient.List(context.Background(), leaseList, client.InNamespace(nsName))).To(Succeed())
			Expect(leaseList.Items).To(HaveLen(1), "Takeover must still create the real Lease as before")

			Expect(mgr.Release(sess.SessionID, "test_done")).To(Succeed())
			Expect(janitor.TrackedForTest(sess.SessionID)).To(BeFalse(),
				"#2100: Release must Untrack the session so the janitor never fires for a cleanly-released session")

			leaseList = &coordinationv1.LeaseList{}
			Expect(sharedK8sClient.List(context.Background(), leaseList, client.InNamespace(nsName))).To(Succeed())
			Expect(leaseList.Items).To(BeEmpty(), "Release must still delete the real Lease as before")
		})
	})

	Describe("IT-KA-2100-002: a session Tracked via Takeover but never Released is reclaimed by the janitor sweep (real Lease deleted)", func() {
		It("should have the janitor's onExpire route through Release, deleting the real Lease object", func() {
			nsName := uniqueNamespace("janitor-2100-002")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			logger := logr.Discard()
			janitor := mcpinternal.NewSessionJanitor(100*time.Millisecond, logger)
			janitorCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go janitor.Run(janitorCtx)

			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger,
				mcpinternal.WithSessionJanitor(janitor))

			user := mcpinternal.UserInfo{Username: "sre-2100-002@example.com", Groups: []string{"sre"}}
			_, err := mgr.Takeover(context.Background(), "rr-2100-002", user)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() *mcpinternal.InteractiveSession {
				driver, _ := mgr.GetDriver("rr-2100-002")
				return driver
			}, 5*time.Second, 50*time.Millisecond).Should(BeNil(),
				"#2100: the orphaned session must be reclaimed by the janitor's backstop sweep")

			leaseList := &coordinationv1.LeaseList{}
			Expect(sharedK8sClient.List(context.Background(), leaseList, client.InNamespace(nsName))).To(Succeed())
			Expect(leaseList.Items).To(BeEmpty(),
				"#2100: the janitor's onExpire callback must have deleted the real Lease object via Release, closing the SC-5 capacity-exhaustion gap")
		})
	})
})
