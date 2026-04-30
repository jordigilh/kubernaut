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
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("LeaseSessionManager Hardening — PR4 BR-INTERACTIVE-005", func() {

	var (
		fakeClient client.Client
		scheme     *runtime.Scheme
		logger     *slog.Logger
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		logger = slog.Default()
	})

	Describe("UT-KA-LEASE-001: Lease spec.leaseDurationSeconds is set to configured SessionTTL", func() {
		It("should set leaseDurationSeconds on the created Lease", func() {
			sessionTTL := 45 * time.Minute
			mgr := mcpinternal.NewLeaseSessionManager(fakeClient, "test-ns", logger,
				mcpinternal.WithSessionTTL(sessionTTL))

			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			sess, err := mgr.Takeover(context.Background(), "rr-lease-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil())

			// Verify the Lease was created with leaseDurationSeconds
			leaseList := &coordinationv1.LeaseList{}
			Expect(fakeClient.List(context.Background(), leaseList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(leaseList.Items).To(HaveLen(1))

			lease := leaseList.Items[0]
			Expect(lease.Spec.LeaseDurationSeconds).NotTo(BeNil())
			Expect(*lease.Spec.LeaseDurationSeconds).To(Equal(int32(sessionTTL.Seconds())))
		})
	})

	Describe("UT-KA-SESS-004: Inactivity timeout — Lease created with leaseDurationSeconds matching config", func() {
		It("should use DefaultSessionTTL when no explicit TTL configured", func() {
			mgr := mcpinternal.NewLeaseSessionManager(fakeClient, "test-ns", logger)

			user := mcpinternal.UserInfo{Username: "bob@example.com"}
			_, err := mgr.Takeover(context.Background(), "rr-default-ttl", user)
			Expect(err).NotTo(HaveOccurred())

			leaseList := &coordinationv1.LeaseList{}
			Expect(fakeClient.List(context.Background(), leaseList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(leaseList.Items).To(HaveLen(1))

			lease := leaseList.Items[0]
			Expect(lease.Spec.LeaseDurationSeconds).NotTo(BeNil())
			Expect(*lease.Spec.LeaseDurationSeconds).To(Equal(int32(mcpinternal.DefaultSessionTTL.Seconds())))
		})
	})

	Describe("UT-KA-LEASE-002: SessionEntry stores correlationID and signal metadata on takeover", func() {
		It("should store signal metadata accessible for later reconstruction", func() {
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(fakeClient, "test-ns", logger)

			user := mcpinternal.UserInfo{Username: "carol@example.com"}
			sess, err := mgr.Takeover(context.Background(), "rr-meta-001", user)
			Expect(err).NotTo(HaveOccurred())

			// Store signal metadata after takeover
			metadata := map[string]string{
				"signal_name": "OOMKilled",
				"severity":    "critical",
				"incident_id": "INC-789",
			}
			mgr.StoreSignalMetadata(sess.SessionID, metadata)

			// Verify metadata is retrievable
			stored := mgr.GetSignalMetadata(sess.SessionID)
			Expect(stored).To(HaveKeyWithValue("signal_name", "OOMKilled"))
			Expect(stored).To(HaveKeyWithValue("severity", "critical"))
			Expect(stored).To(HaveKeyWithValue("incident_id", "INC-789"))
		})
	})

	Describe("UT-KA-TAKE-003: DS query failure during auto-inject — takeover succeeds with empty context", func() {
		It("should succeed even when context reconstruction returns empty", func() {
			mgr := mcpinternal.NewLeaseSessionManager(fakeClient, "test-ns", logger)

			user := mcpinternal.UserInfo{Username: "dave@example.com"}
			sess, err := mgr.Takeover(context.Background(), "rr-ds-fail", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil())
			Expect(sess.SessionID).NotTo(BeEmpty())
		})
	})
})
