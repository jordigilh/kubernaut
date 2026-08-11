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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

// #2100: SessionJanitor was fully implemented and independently
// unit/integration-tested (IT-KA-DES-005/006) but had zero production
// callers -- confirmed via grep -rn NewSessionJanitor returning only its
// own definition and test files. LeaseSessionManager.Takeover/Release never
// called Track/Untrack, so the janitor's backstop sweep could never fire
// for a real interactive session, no matter how long it leaked. These
// tests prove the chokepoint wiring inside session_manager.go itself (not
// just the janitor's own pre-existing sweep logic, which IT-KA-DES-005/006
// already cover).
var _ = Describe("LeaseSessionManager + SessionJanitor wiring — #2100", func() {
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

	Describe("UT-KA-2100-001: Takeover success Tracks the session; Release Untracks it", func() {
		It("should register the session with the janitor on Takeover and deregister it on Release", func() {
			janitor := mcpinternal.NewSessionJanitor(1*time.Hour, logger) // long interval: sweep must never fire in this test
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionJanitor(janitor))

			user := mcpinternal.UserInfo{Username: "alice@example.com", Groups: []string{"sre"}}
			sess, err := mgr.Takeover(ctx, "rr-2100-001", user)
			Expect(err).NotTo(HaveOccurred())

			Expect(janitor.TrackedForTest(sess.SessionID)).To(BeTrue(),
				"#2100: Takeover success must Track the session with the janitor (chokepoint wiring)")

			Expect(mgr.Release(sess.SessionID, "test_done")).To(Succeed())
			Expect(janitor.TrackedForTest(sess.SessionID)).To(BeFalse(),
				"#2100: Release must Untrack the session so the janitor's backstop sweep never fires for a cleanly-released session")
		})
	})

	Describe("UT-KA-2100-002: a session tracked via Takeover but never Released is reclaimed by the janitor's sweep", func() {
		It("should have the janitor's onExpire callback call Release, deleting the Lease and clearing the driver", func() {
			janitor := mcpinternal.NewSessionJanitor(50*time.Millisecond, logger)
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionJanitor(janitor))

			janitorCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go janitor.Run(janitorCtx)

			user := mcpinternal.UserInfo{Username: "bob@example.com", Groups: []string{"sre"}}
			_, err := mgr.Takeover(ctx, "rr-2100-002", user)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() *mcpinternal.InteractiveSession {
				driver, _ := mgr.GetDriver("rr-2100-002")
				return driver
			}, 2*time.Second, 20*time.Millisecond).Should(BeNil(),
				"#2100: the orphaned session must be reclaimed by the janitor's backstop sweep -- GetDriver must no longer find a driver")

			leaseList := &coordinationv1.LeaseList{}
			Expect(k8sClient.List(ctx, leaseList, client.InNamespace(namespace))).To(Succeed())
			Expect(leaseList.Items).To(BeEmpty(),
				"#2100: the janitor's onExpire callback must have deleted the orphaned session's Lease via Release, not just logged")
		})
	})

	Describe("UT-KA-2100-005 (regression guard): a same-user reconnect never touches the janitor", func() {
		It("should not panic or double-track when Takeover's reconnect branch returns early", func() {
			janitor := mcpinternal.NewSessionJanitor(1*time.Hour, logger)
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionJanitor(janitor))

			user := mcpinternal.UserInfo{Username: "carol@example.com", Groups: []string{"sre"}}
			first, err := mgr.Takeover(ctx, "rr-2100-002b", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(janitor.TrackedForTest(first.SessionID)).To(BeTrue())

			second, err := mgr.Takeover(ctx, "rr-2100-002b", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(second.SessionID).To(Equal(first.SessionID), "same-user Takeover must reconnect, not create a new session")
			Expect(janitor.TrackedForTest(first.SessionID)).To(BeTrue(),
				"the reconnect branch must not accidentally Untrack the still-active session")
		})
	})
})
