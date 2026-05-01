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
	"errors"
	"log/slog"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("CP-5 INT Degraded Scenarios (Unit Tests)", func() {

	// ---------------------------------------------------------------
	// UT-KA-INT002: DS degraded during takeover auto-inject
	// Reclassified from E2E INT-002 (BR-INTERACTIVE-008)
	// Validates: takeover succeeds even when DS is unreachable for context reconstruction
	// ---------------------------------------------------------------
	Describe("UT-KA-INT002: Takeover succeeds when DS unavailable for auto-inject", func() {
		It("should complete takeover with empty context when DS returns error", func() {
			ctx := context.Background()
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

			By("Setting up DS reconstructor that simulates DS unavailability")
			dsDown := &mockAuditQuerier{
				err: errors.New("connection refused: datastorage service unavailable"),
			}
			recon := mcpinternal.NewDSContextReconstructor(dsDown, 2*time.Second, logger)

			By("Verifying reconstructor returns empty (not error) on DS failure")
			turns, err := recon.Reconstruct(ctx, "rr-int002-degraded", "")
			Expect(err).NotTo(HaveOccurred(), "reconstructor must be best-effort: no error on DS failure")
			Expect(turns).To(BeEmpty(), "no turns when DS is down")

			By("Verifying session can still be created (Lease-based, independent of DS)")
			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			mgr := mcpinternal.NewLeaseSessionManager(fakeClient, "test-ns", logger,
				mcpinternal.WithMaxConcurrentSessions(10),
			)

			user := mcpinternal.UserInfo{Username: "user-int002@example.com", Groups: []string{"sre"}}
			sess, err := mgr.Takeover(ctx, "rr-int002-degraded", user)
			Expect(err).NotTo(HaveOccurred(), "takeover must succeed even when DS is down")
			Expect(sess).NotTo(BeNil())
			Expect(sess.SessionID).NotTo(BeEmpty())
			Expect(sess.ActingUser.Username).To(Equal("user-int002@example.com"))

			GinkgoWriter.Println("✅ UT-KA-INT002: Takeover succeeded with DS unavailable")
		})
	})

	// ---------------------------------------------------------------
	// UT-KA-INT006: Lease loss after pod restart — re-takeover succeeds
	// Reclassified from E2E INT-006 (BR-INTERACTIVE-005, BR-INTERACTIVE-008)
	// Validates: after process restart (empty session map) and Lease expiry,
	// a fresh takeover succeeds and creates a new session.
	// ---------------------------------------------------------------
	Describe("UT-KA-INT006: Re-takeover succeeds after Lease expiry (pod restart)", func() {
		It("should allow fresh takeover when no Lease exists (simulating expiry after restart)", func() {
			ctx := context.Background()
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())

			By("Starting with empty cluster state (Lease expired/GC'd after pod restart)")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			mgr := mcpinternal.NewLeaseSessionManager(fakeClient, "test-ns", logger,
				mcpinternal.WithMaxConcurrentSessions(10),
			)

			rrID := "rr-int006-restart"
			user := mcpinternal.UserInfo{Username: "recon-user@example.com", Groups: []string{"sre"}}

			By("Fresh takeover should succeed (no stale Lease blocking)")
			sess, err := mgr.Takeover(ctx, rrID, user)
			Expect(err).NotTo(HaveOccurred(), "takeover on empty state should succeed")
			Expect(sess).NotTo(BeNil())
			Expect(sess.SessionID).NotTo(BeEmpty())

			By("Verifying Lease was created")
			leaseList := &coordinationv1.LeaseList{}
			Expect(fakeClient.List(ctx, leaseList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(leaseList.Items).To(HaveLen(1))
			Expect(leaseList.Items[0].Name).To(Equal("kubernaut-interactive-" + rrID))

			GinkgoWriter.Println("✅ UT-KA-INT006: Re-takeover succeeded after simulated Lease expiry")
		})

		It("should block takeover when stale Lease still exists (not yet expired)", func() {
			ctx := context.Background()
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())

			rrID := "rr-int006-stale"
			leaseName := "kubernaut-interactive-" + rrID
			staleLease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      leaseName,
					Namespace: "test-ns",
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       strPtr("old-session-from-crashed-pod"),
					LeaseDurationSeconds: int32Ptr(1800),
				},
			}

			By("Pre-creating stale Lease (simulating Lease not yet expired)")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(staleLease).Build()
			mgr := mcpinternal.NewLeaseSessionManager(fakeClient, "test-ns", logger,
				mcpinternal.WithMaxConcurrentSessions(10),
			)

			user := mcpinternal.UserInfo{Username: "recon-user@example.com", Groups: []string{"sre"}}

			By("Takeover should fail with ErrLeaseHeld (stale Lease blocks)")
			_, err := mgr.Takeover(ctx, rrID, user)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, mcpinternal.ErrLeaseHeld)).To(BeTrue(),
				"stale Lease should block new takeover until K8s GC expires it")

			GinkgoWriter.Println("✅ UT-KA-INT006: Stale Lease correctly blocks premature re-takeover")
		})
	})

	// ---------------------------------------------------------------
	// UT-KA-INT002-B: DS recovery — context available after DS returns
	// Validates: once DS is back, reconstruction returns prior turns
	// ---------------------------------------------------------------
	Describe("UT-KA-INT002-B: DS recovery restores context reconstruction", func() {
		It("should return turns once DS becomes available again", func() {
			ctx := context.Background()
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

			t1 := time.Now().Add(-2 * time.Minute)
			dsRecovered := &mockAuditQuerier{
				response: &ogenclient.AuditEventsQueryResponse{
					Data: []ogenclient.AuditEvent{
						makeLLMRequestEvent("rr-recovery", "what happened?", t1),
						makeLLMResponseEvent("rr-recovery", "OOM detected", t1.Add(time.Second)),
					},
				},
			}

			recon := mcpinternal.NewDSContextReconstructor(dsRecovered, 5*time.Second, logger)
			turns, err := recon.Reconstruct(ctx, "rr-recovery", "sess-recovery")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(HaveLen(2), "should reconstruct 2 prior turns after DS recovery")
			Expect(turns[0].Role).To(Equal("user"))
			Expect(turns[1].Role).To(Equal("assistant"))

			GinkgoWriter.Println("✅ UT-KA-INT002-B: DS recovery restores context reconstruction")
		})
	})
})

func strPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32 { return &i }
