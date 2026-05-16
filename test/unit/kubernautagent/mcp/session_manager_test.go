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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("LeaseSessionManager — #703 BR-INTERACTIVE-002", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		namespace string
		logger    logr.Logger
		mgr       mcpinternal.SessionManager
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

		mgr = mcpinternal.NewLeaseSessionManager(k8sClient, namespace, logger)
	})

	Describe("UT-KA-703-I01: Takeover creates Lease, returns InteractiveSession", func() {
		It("should create a K8s Lease and return a valid session", func() {
			user := mcpinternal.UserInfo{Username: "alice@example.com", Groups: []string{"sre"}}
			session, err := mgr.Takeover(ctx, "rr-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())
			Expect(session.SessionID).NotTo(BeEmpty())
			Expect(session.CorrelationID).To(Equal("rr-001"))
			Expect(session.ActingUser.Username).To(Equal("alice@example.com"))
			Expect(session.StartedAt).NotTo(BeZero())

			leaseList := &coordinationv1.LeaseList{}
			Expect(k8sClient.List(ctx, leaseList, client.InNamespace(namespace))).To(Succeed())
			Expect(leaseList.Items).To(HaveLen(1))
			Expect(*leaseList.Items[0].Spec.HolderIdentity).To(Equal(session.SessionID))
		})
	})

	Describe("UT-KA-703-I02: Takeover rejects when Lease held by another driver", func() {
		It("should return ErrLeaseHeld when another user holds the Lease", func() {
			user1 := mcpinternal.UserInfo{Username: "alice@example.com", Groups: []string{"sre"}}
			_, err := mgr.Takeover(ctx, "rr-002", user1)
			Expect(err).NotTo(HaveOccurred())

			user2 := mcpinternal.UserInfo{Username: "bob@example.com", Groups: []string{"sre"}}
			_, err = mgr.Takeover(ctx, "rr-002", user2)
			Expect(err).To(MatchError(mcpinternal.ErrLeaseHeld))
		})
	})

	Describe("UT-KA-703-I03: Release deletes Lease, marks session completed", func() {
		It("should remove the Lease and set CompletedAt", func() {
			user := mcpinternal.UserInfo{Username: "alice@example.com", Groups: []string{"sre"}}
			session, err := mgr.Takeover(ctx, "rr-003", user)
			Expect(err).NotTo(HaveOccurred())

			err = mgr.Release(session.SessionID, "explicit")
			Expect(err).NotTo(HaveOccurred())

			leaseList := &coordinationv1.LeaseList{}
			Expect(k8sClient.List(ctx, leaseList, client.InNamespace(namespace))).To(Succeed())
			Expect(leaseList.Items).To(BeEmpty())

			Expect(mgr.IsDriverActive("rr-003")).To(BeFalse())
		})
	})

	Describe("UT-KA-703-I04: Release with unknown session returns ErrSessionNotFound", func() {
		It("should return ErrSessionNotFound for a nonexistent session", func() {
			err := mgr.Release("nonexistent-session-id", "explicit")
			Expect(err).To(MatchError(mcpinternal.ErrSessionNotFound))
		})
	})

	Describe("UT-KA-703-I05: GetDriver returns nil when no active session", func() {
		It("should return nil session when no driver is active", func() {
			session, err := mgr.GetDriver("rr-005")
			Expect(err).NotTo(HaveOccurred())
			Expect(session).To(BeNil())
		})
	})

	Describe("UT-KA-703-I06: GetDriver returns session when driver active", func() {
		It("should return the active session for the given rrID", func() {
			user := mcpinternal.UserInfo{Username: "charlie@example.com", Groups: []string{"ops"}}
			created, err := mgr.Takeover(ctx, "rr-006", user)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := mgr.GetDriver("rr-006")
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved).NotTo(BeNil())
			Expect(retrieved.SessionID).To(Equal(created.SessionID))
			Expect(retrieved.ActingUser.Username).To(Equal("charlie@example.com"))
		})
	})

	Describe("UT-KA-703-I07: IsDriverActive returns correct boolean", func() {
		It("should return false before Takeover and true after", func() {
			Expect(mgr.IsDriverActive("rr-007")).To(BeFalse())

			user := mcpinternal.UserInfo{Username: "dave@example.com", Groups: []string{"sre"}}
			_, err := mgr.Takeover(ctx, "rr-007", user)
			Expect(err).NotTo(HaveOccurred())

			Expect(mgr.IsDriverActive("rr-007")).To(BeTrue())
		})
	})

	Describe("UT-KA-SEC01-001: Takeover rejects empty username", func() {
		It("should return ErrEmptyUsername for anonymous identity", func() {
			user := mcpinternal.UserInfo{Username: "", Groups: nil}
			_, err := mgr.Takeover(ctx, "rr-sec01", user)
			Expect(err).To(MatchError(mcpinternal.ErrEmptyUsername))
		})
	})

	Describe("UT-KA-SEC03-001: Takeover enforces maxConcurrentSessions", func() {
		It("should reject when max sessions reached", func() {
			mgrLimited := mcpinternal.NewLeaseSessionManager(k8sClient, namespace, logger,
				mcpinternal.WithMaxConcurrentSessions(2),
			)

			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			_, err := mgrLimited.Takeover(ctx, "rr-max-1", user)
			Expect(err).NotTo(HaveOccurred())

			_, err = mgrLimited.Takeover(ctx, "rr-max-2", user)
			Expect(err).NotTo(HaveOccurred())

			_, err = mgrLimited.Takeover(ctx, "rr-max-3", user)
			Expect(err).To(MatchError(mcpinternal.ErrMaxSessionsReached))
		})

		It("should allow new sessions after release", func() {
			mgrLimited := mcpinternal.NewLeaseSessionManager(k8sClient, namespace, logger,
				mcpinternal.WithMaxConcurrentSessions(1),
			)

			user := mcpinternal.UserInfo{Username: "bob@example.com"}
			sess, err := mgrLimited.Takeover(ctx, "rr-max-release-1", user)
			Expect(err).NotTo(HaveOccurred())

			Expect(mgrLimited.Release(sess.SessionID, "complete")).To(Succeed())

			_, err = mgrLimited.Takeover(ctx, "rr-max-release-2", user)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-SEC04-001: GetDriver auto-releases expired sessions (TTL)", func() {
		It("should return ErrSessionExpired when session TTL has elapsed", func() {
			mgrShortTTL := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionTTL(1*time.Millisecond),
			)

			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			_, err := mgrShortTTL.Takeover(ctx, "rr-ttl-001", user)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(5 * time.Millisecond)

			sess, err := mgrShortTTL.GetDriver("rr-ttl-001")
			Expect(err).To(MatchError(mcpinternal.ErrSessionExpired))
			Expect(sess).To(BeNil())

			Expect(mgrShortTTL.IsDriverActive("rr-ttl-001")).To(BeFalse())
		})
	})

	Describe("UT-KA-SEC04-002: GetDriver auto-releases inactive sessions", func() {
		It("should return ErrSessionExpired when inactivity timeout exceeded", func() {
			mgrShortInactivity := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionTTL(1*time.Hour),
				mcpinternal.WithInactivityTimeout(1*time.Millisecond),
			)

			user := mcpinternal.UserInfo{Username: "bob@example.com"}
			_, err := mgrShortInactivity.Takeover(ctx, "rr-inact-001", user)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(5 * time.Millisecond)

			sess, err := mgrShortInactivity.GetDriver("rr-inact-001")
			Expect(err).To(MatchError(mcpinternal.ErrSessionExpired))
			Expect(sess).To(BeNil())
		})

		It("should NOT expire when TouchActivity resets the timer", func() {
			mgrShortInactivity := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionTTL(1*time.Hour),
				mcpinternal.WithInactivityTimeout(50*time.Millisecond),
			)

			user := mcpinternal.UserInfo{Username: "charlie@example.com"}
			_, err := mgrShortInactivity.Takeover(ctx, "rr-inact-002", user)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(30 * time.Millisecond)
			mgrShortInactivity.TouchActivity("rr-inact-002")

			time.Sleep(30 * time.Millisecond)
			sess, err := mgrShortInactivity.GetDriver("rr-inact-002")
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil())
		})
	})

	// Regression test: M1 — WithSessionExpiredCallback must fire when GetDriver
	// auto-releases a session due to TTL or inactivity expiry.
	// Bug: TTL/inactivity paths never emitted interactive.completed audit because
	// LeaseSessionManager had no callback mechanism to notify the audit system.
	Describe("UT-KA-SEC04-M1: SessionExpiredCallback fires on TTL expiry (M1 regression)", func() {
		It("should invoke the callback with sessionID, rrID, and reason on TTL expiry", func() {
			callbackCh := make(chan [3]string, 1)
			mgrWithCallback := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionTTL(1*time.Millisecond),
				mcpinternal.WithSessionExpiredCallback(func(sessionID, rrID, reason string) {
					callbackCh <- [3]string{sessionID, rrID, reason}
				}),
			)

			user := mcpinternal.UserInfo{Username: "audit-m1@example.com"}
			_, err := mgrWithCallback.Takeover(ctx, "rr-m1-ttl-001", user)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(5 * time.Millisecond)

			sess, err := mgrWithCallback.GetDriver("rr-m1-ttl-001")
			Expect(err).To(MatchError(mcpinternal.ErrSessionExpired))
			Expect(sess).To(BeNil())

			Eventually(callbackCh).Should(Receive(SatisfyAll(
				WithTransform(func(a [3]string) string { return a[1] }, Equal("rr-m1-ttl-001")),
				WithTransform(func(a [3]string) string { return a[2] }, Equal("ttl_expired")),
			)), "M1 fix: callback must fire with reason=ttl_expired")
		})

		It("should invoke the callback with reason=inactivity_timeout on inactivity expiry", func() {
			callbackCh := make(chan [3]string, 1)
			mgrWithCallback := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionTTL(1*time.Hour),
				mcpinternal.WithInactivityTimeout(1*time.Millisecond),
				mcpinternal.WithSessionExpiredCallback(func(sessionID, rrID, reason string) {
					callbackCh <- [3]string{sessionID, rrID, reason}
				}),
			)

			user := mcpinternal.UserInfo{Username: "audit-m1@example.com"}
			_, err := mgrWithCallback.Takeover(ctx, "rr-m1-inact-001", user)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(5 * time.Millisecond)

			sess, err := mgrWithCallback.GetDriver("rr-m1-inact-001")
			Expect(err).To(MatchError(mcpinternal.ErrSessionExpired))
			Expect(sess).To(BeNil())

			Eventually(callbackCh).Should(Receive(SatisfyAll(
				WithTransform(func(a [3]string) string { return a[1] }, Equal("rr-m1-inact-001")),
				WithTransform(func(a [3]string) string { return a[2] }, Equal("inactivity_timeout")),
			)), "M1 fix: callback must fire with reason=inactivity_timeout")
		})
	})

	Describe("UT-KA-RECONNECT-001: Same-user Takeover returns existing session", func() {
		It("should return existing session with Reconnected=true for same user", func() {
			user := mcpinternal.UserInfo{Username: "alice@example.com", Groups: []string{"sre"}}
			sess1, err := mgr.Takeover(ctx, "rr-recon-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess1.Reconnected).To(BeFalse(), "first call is not a reconnect")

			sess2, err := mgr.Takeover(ctx, "rr-recon-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess2).NotTo(BeNil())
			Expect(sess2.SessionID).To(Equal(sess1.SessionID), "should return same session ID")
			Expect(sess2.Reconnected).To(BeTrue(), "second call from same user is a reconnect")
		})

		It("should still reject a different user while same user holds Lease", func() {
			alice := mcpinternal.UserInfo{Username: "alice@example.com", Groups: []string{"sre"}}
			_, err := mgr.Takeover(ctx, "rr-recon-002", alice)
			Expect(err).NotTo(HaveOccurred())

			bob := mcpinternal.UserInfo{Username: "bob@example.com", Groups: []string{"sre"}}
			_, err = mgr.Takeover(ctx, "rr-recon-002", bob)
			Expect(err).To(MatchError(mcpinternal.ErrLeaseHeld))
		})
	})

	Describe("UT-KA-ORPHAN-001: Takeover reclaims expired orphaned K8s Lease", func() {
		It("should reclaim an expired Lease and create a new session", func() {
			mgrConcrete := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionTTL(30*time.Minute),
			)

			By("Creating an orphaned Lease directly in K8s (simulating pod restart)")
			pastAcquire := metav1.NewMicroTime(time.Now().Add(-2 * time.Hour))
			leaseDuration := int32(1800) // 30 minutes — expired long ago
			orphanedLease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-interactive-rr-orphan-001",
					Namespace: namespace,
					Annotations: map[string]string{
						"kubernaut.io/session-ttl": "30m0s",
					},
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       strPtr("dead-session-id"),
					LeaseDurationSeconds: &leaseDuration,
					AcquireTime:          &pastAcquire,
				},
			}
			Expect(k8sClient.Create(ctx, orphanedLease)).To(Succeed())

			By("Attempting Takeover — should reclaim the expired Lease")
			user := mcpinternal.UserInfo{Username: "charlie@example.com"}
			sess, err := mgrConcrete.Takeover(ctx, "rr-orphan-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil())
			Expect(sess.SessionID).NotTo(Equal("dead-session-id"))
			Expect(sess.ActingUser.Username).To(Equal("charlie@example.com"))
		})

		It("should NOT reclaim a non-expired Lease from a live pod", func() {
			By("Creating a Lease that is still valid (acquired now, 30min TTL)")
			recentAcquire := metav1.NewMicroTime(time.Now())
			leaseDuration := int32(1800)
			liveLease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-interactive-rr-orphan-002",
					Namespace: namespace,
					Annotations: map[string]string{
						"kubernaut.io/session-ttl": "30m0s",
					},
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       strPtr("live-session-id"),
					LeaseDurationSeconds: &leaseDuration,
					AcquireTime:          &recentAcquire,
				},
			}
			Expect(k8sClient.Create(ctx, liveLease)).To(Succeed())

			mgrConcrete := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger,
				mcpinternal.WithSessionTTL(30*time.Minute),
			)

			By("Attempting Takeover — should be rejected (Lease not expired)")
			user := mcpinternal.UserInfo{Username: "dave@example.com"}
			_, err := mgrConcrete.Takeover(ctx, "rr-orphan-002", user)
			Expect(err).To(MatchError(mcpinternal.ErrLeaseHeld))
		})
	})
})
