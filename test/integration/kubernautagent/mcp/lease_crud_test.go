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

var _ = Describe("IT-KA-MCP-007: Lease CRUD lifecycle — PR4 BR-INTERACTIVE-005", func() {

	It("should create, verify, and delete Lease objects via controller-runtime client", func() {
		scheme := runtime.NewScheme()
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := slog.Default()

		sessionTTL := 20 * time.Minute
		mgr := mcpinternal.NewLeaseSessionManager(fakeClient, "kube-system", logger,
			mcpinternal.WithSessionTTL(sessionTTL))

		user := mcpinternal.UserInfo{Username: "sre@example.com"}

		// CREATE: Takeover creates a Lease
		sess, err := mgr.Takeover(context.Background(), "rr-crud-001", user)
		Expect(err).NotTo(HaveOccurred())
		Expect(sess).NotTo(BeNil())

		// READ: Verify Lease exists with correct fields
		leaseList := &coordinationv1.LeaseList{}
		Expect(fakeClient.List(context.Background(), leaseList, client.InNamespace("kube-system"))).To(Succeed())
		Expect(leaseList.Items).To(HaveLen(1))

		lease := leaseList.Items[0]
		Expect(lease.Name).To(HavePrefix("kubernaut-interactive-"))
		Expect(lease.Spec.HolderIdentity).NotTo(BeNil())
		Expect(*lease.Spec.HolderIdentity).To(Equal(sess.SessionID))
		Expect(lease.Spec.LeaseDurationSeconds).NotTo(BeNil())
		Expect(*lease.Spec.LeaseDurationSeconds).To(Equal(int32(sessionTTL.Seconds())))
		Expect(lease.Spec.AcquireTime).NotTo(BeNil())

		// DELETE: Release removes the Lease
		err = mgr.Release(sess.SessionID, "complete")
		Expect(err).NotTo(HaveOccurred())

		leaseList = &coordinationv1.LeaseList{}
		Expect(fakeClient.List(context.Background(), leaseList, client.InNamespace("kube-system"))).To(Succeed())
		Expect(leaseList.Items).To(HaveLen(0))

		// Verify second takeover on same rrID works after release
		sess2, err := mgr.Takeover(context.Background(), "rr-crud-001", user)
		Expect(err).NotTo(HaveOccurred())
		Expect(sess2.SessionID).NotTo(Equal(sess.SessionID))
	})
})
