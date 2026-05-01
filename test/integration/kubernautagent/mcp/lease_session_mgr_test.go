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

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("LeaseSessionManager deep IT — BR-INTERACTIVE-005", Label("integration", "lease"), func() {

	var (
		logger logr.Logger
		nsName string
	)

	BeforeEach(func() {
		logger = logr.Discard()
		nsName = uniqueNamespace("lsm")
		createNamespace(context.Background(), sharedK8sClient, nsName)
	})

	Describe("IT-KA-LSM-001: empty username is rejected (SEC-01)", func() {
		It("should return ErrEmptyUsername when username is blank", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			_, err := mgr.Takeover(context.Background(), "rr-sec01", mcpinternal.UserInfo{Username: ""})
			Expect(err).To(MatchError(mcpinternal.ErrEmptyUsername))
		})
	})

	Describe("IT-KA-LSM-002: concurrent takeover on same rrID — only one succeeds", func() {
		It("should allow exactly one of N concurrent takeover attempts", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)

			const goroutines = 5
			var (
				wg      sync.WaitGroup
				mu      sync.Mutex
				winners []string
				losers  int
			)

			for i := 0; i < goroutines; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					user := mcpinternal.UserInfo{Username: "user@example.com"}
					sess, err := mgr.Takeover(context.Background(), "rr-contention", user)
					mu.Lock()
					defer mu.Unlock()
					if err != nil {
						losers++
					} else {
						winners = append(winners, sess.SessionID)
					}
				}(i)
			}
			wg.Wait()

			Expect(winners).To(HaveLen(1), "exactly one goroutine should win the lease")
			Expect(losers).To(Equal(goroutines - 1))
		})
	})

	Describe("IT-KA-LSM-003: max concurrent sessions enforced (SEC-03)", func() {
		It("should reject the N+1th session when maxSessions=N", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger,
				mcpinternal.WithMaxConcurrentSessions(2))

			user := mcpinternal.UserInfo{Username: "alice@example.com"}

			sess1, err := mgr.Takeover(context.Background(), "rr-max-a", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess1).NotTo(BeNil())

			sess2, err := mgr.Takeover(context.Background(), "rr-max-b", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess2).NotTo(BeNil())

			_, err = mgr.Takeover(context.Background(), "rr-max-c", user)
			Expect(err).To(MatchError(mcpinternal.ErrMaxSessionsReached))

			By("releasing one session frees capacity")
			Expect(mgr.Release(sess1.SessionID, "test")).To(Succeed())

			sess3, err := mgr.Takeover(context.Background(), "rr-max-c", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess3).NotTo(BeNil())
		})
	})

	Describe("IT-KA-LSM-004: release non-existent session returns ErrSessionNotFound", func() {
		It("should return ErrSessionNotFound for unknown session ID", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			err := mgr.Release("does-not-exist", "test")
			Expect(err).To(MatchError(mcpinternal.ErrSessionNotFound))
		})
	})

	Describe("IT-KA-LSM-005: GetDriver returns nil for unknown rrID", func() {
		It("should return nil session when no driver holds the rrID", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			sess, err := mgr.GetDriver("rr-nobody")
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).To(BeNil())
		})
	})

	Describe("IT-KA-LSM-006: TTL expiry detected on GetDriver", func() {
		It("should return ErrSessionExpired when session exceeds TTL", func() {
			// K8s Lease leaseDurationSeconds must be >=1, so minimum testable TTL is 1s.
			ttl := 1 * time.Second
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger,
				mcpinternal.WithSessionTTL(ttl))

			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			_, err := mgr.Takeover(context.Background(), "rr-ttl", user)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(ttl + 200*time.Millisecond)

			_, err = mgr.GetDriver("rr-ttl")
			Expect(err).To(MatchError(mcpinternal.ErrSessionExpired))
		})
	})

	Describe("IT-KA-LSM-007: TouchActivity resets the inactivity window", func() {
		It("should keep session alive when activity is reported", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger,
				mcpinternal.WithSessionTTL(30*time.Minute))

			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			_, err := mgr.Takeover(context.Background(), "rr-touch", user)
			Expect(err).NotTo(HaveOccurred())

			mgr.TouchActivity("rr-touch")
			sess, err := mgr.GetDriver("rr-touch")
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil())
			Expect(sess.ActingUser.Username).To(Equal("alice@example.com"))
		})
	})

	Describe("IT-KA-LSM-008: IsDriverActive reflects session state", func() {
		It("should return true when active, false after release", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			user := mcpinternal.UserInfo{Username: "alice@example.com"}

			Expect(mgr.IsDriverActive("rr-active")).To(BeFalse())

			sess, err := mgr.Takeover(context.Background(), "rr-active", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(mgr.IsDriverActive("rr-active")).To(BeTrue())

			Expect(mgr.Release(sess.SessionID, "done")).To(Succeed())
			Expect(mgr.IsDriverActive("rr-active")).To(BeFalse())
		})
	})

	Describe("IT-KA-LSM-009: signal metadata storage and retrieval", func() {
		It("should store and retrieve metadata for active sessions", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			user := mcpinternal.UserInfo{Username: "alice@example.com"}

			sess, err := mgr.Takeover(context.Background(), "rr-meta", user)
			Expect(err).NotTo(HaveOccurred())

			meta := map[string]string{"signal_id": "sig-001", "priority": "high"}
			mgr.StoreSignalMetadata(sess.SessionID, meta)

			retrieved := mgr.GetSignalMetadata(sess.SessionID)
			Expect(retrieved).To(Equal(meta))

			rrID, sMeta := mgr.GetSessionInfo(sess.SessionID)
			Expect(rrID).To(Equal("rr-meta"))
			Expect(sMeta).To(Equal(meta))
		})
	})

	Describe("IT-KA-LSM-010: re-takeover after release on same rrID", func() {
		It("should produce a different session ID on second takeover", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			user := mcpinternal.UserInfo{Username: "alice@example.com"}

			sess1, err := mgr.Takeover(context.Background(), "rr-retake", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(mgr.Release(sess1.SessionID, "done")).To(Succeed())

			sess2, err := mgr.Takeover(context.Background(), "rr-retake", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess2.SessionID).NotTo(Equal(sess1.SessionID))
		})
	})
})
