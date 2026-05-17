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
	"sync"
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

var _ = Describe("SessionDrainer — BR-OPS-013 graceful shutdown", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		namespace string
		logger    logr.Logger
		mgr       *mcpinternal.LeaseSessionManager
		notifier  *mcpinternal.SessionNotifier
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "kubernaut-system"
		logger = logr.Discard()

		scheme = runtime.NewScheme()
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		mgr = mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger)
		notifier = mcpinternal.NewSessionNotifier()
	})

	Describe("UT-KA-DRAIN-001: DrainSessions releases all active sessions on shutdown", func() {
		It("should release both sessions and delete Leases", func() {
			_, err := mgr.Takeover(ctx, "rr-drain-1", mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			_, err = mgr.Takeover(ctx, "rr-drain-2", mcpinternal.UserInfo{Username: "bob"})
			Expect(err).NotTo(HaveOccurred())

			ids := mgr.ActiveSessionIDs()
			Expect(ids).To(HaveLen(2))

			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			drainer := mcpinternal.NewSessionDrainer(mgr, notifier, logger)
			drainer.DrainSessions(drainCtx)

			Expect(mgr.ActiveSessionIDs()).To(BeEmpty(),
				"all sessions must be released after drain")

			Expect(mgr.IsDriverActive("rr-drain-1")).To(BeFalse())
			Expect(mgr.IsDriverActive("rr-drain-2")).To(BeFalse())

			var leases coordinationv1.LeaseList
			Expect(k8sClient.List(ctx, &leases, client.InNamespace(namespace))).To(Succeed())
			Expect(leases.Items).To(BeEmpty(),
				"all Leases must be deleted after drain")
		})
	})

	Describe("UT-KA-DRAIN-002: DrainSessions respects context timeout", func() {
		It("should return at timeout even if release is slow", func() {
			_, err := mgr.Takeover(ctx, "rr-timeout-1", mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())

			drainCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()

			drainer := mcpinternal.NewSessionDrainer(mgr, notifier, logger)

			start := time.Now()
			drainer.DrainSessions(drainCtx)
			elapsed := time.Since(start)

			Expect(elapsed).To(BeNumerically("<", 2*time.Second),
				"drain must complete within a reasonable time bound")
		})
	})

	Describe("UT-KA-DRAIN-003: DrainSessions sends notification to connected clients", func() {
		It("should notify each session before releasing", func() {
			sess1, err := mgr.Takeover(ctx, "rr-notify-1", mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			sess2, err := mgr.Takeover(ctx, "rr-notify-2", mcpinternal.UserInfo{Username: "bob"})
			Expect(err).NotTo(HaveOccurred())

			var mu sync.Mutex
			notifications := make(map[string]string)
			notifier.Register(sess1.SessionID, func(msg string) {
				mu.Lock()
				defer mu.Unlock()
				notifications[sess1.SessionID] = msg
			})
			notifier.Register(sess2.SessionID, func(msg string) {
				mu.Lock()
				defer mu.Unlock()
				notifications[sess2.SessionID] = msg
			})

			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			drainer := mcpinternal.NewSessionDrainer(mgr, notifier, logger)
			drainer.DrainSessions(drainCtx)

			mu.Lock()
			defer mu.Unlock()
			Expect(notifications).To(HaveLen(2),
				"both sessions must receive shutdown notification")
			Expect(notifications[sess1.SessionID]).To(ContainSubstring("shutting down"),
				"notification must indicate shutdown")
			Expect(notifications[sess2.SessionID]).To(ContainSubstring("shutting down"),
				"notification must indicate shutdown")
		})
	})

	Describe("DrainSessions with no active sessions returns immediately", func() {
		It("should be a no-op when no sessions exist", func() {
			drainer := mcpinternal.NewSessionDrainer(mgr, notifier, logger)
			start := time.Now()
			drainer.DrainSessions(ctx)
			Expect(time.Since(start)).To(BeNumerically("<", 100*time.Millisecond))
		})
	})

	Describe("DrainSessions with nil session manager is safe", func() {
		It("should not panic when session manager is nil", func() {
			drainer := mcpinternal.NewSessionDrainer(nil, notifier, logger)
			Expect(func() {
				drainer.DrainSessions(ctx)
			}).NotTo(Panic())
		})
	})
})
