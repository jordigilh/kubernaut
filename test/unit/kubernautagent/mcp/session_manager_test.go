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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
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
		logger    *slog.Logger
		mgr       mcpinternal.SessionManager
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "kubernaut-system"
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

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
})
