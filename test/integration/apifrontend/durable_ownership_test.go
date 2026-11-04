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

package apifrontend_test

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adksession "google.golang.org/adk/v2/session"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// BR-INTERACTIVE-004 and BR-INTERACTIVE-010: a durable InvestigationSession
// remains the authoritative single owner across ephemeral Lease expiry and KA
// restart/reacquisition.
var _ = Describe("Durable interactive ownership (#168)", Label("integration", "ownership", "session"), func() {
	It("IT-AF-KA-168-001: retains User A ownership after expired Lease and fresh KA epoch", func() {
		ctx := context.Background()
		const (
			userA = "sre-alice@example.com"
			userB = "sre-bob@example.com"
		)
		rrName := fmt.Sprintf("rr-durable-owner-%d", time.Now().UnixNano())
		leaseNamespace := defaultFixture

		leaseScheme := runtime.NewScheme()
		Expect(coordinationv1.AddToScheme(leaseScheme)).To(Succeed())
		leaseClient, err := client.New(restCfg, client.Options{Scheme: leaseScheme})
		Expect(err).NotTo(HaveOccurred(), "Lease client should use the integration API server")

		service := session.NewCRDSessionService(
			adksession.InMemoryService(),
			k8sClient,
			scheme,
			leaseNamespace,
		)
		leaseTTL := time.Second
		managerA := mcpinternal.NewLeaseSessionManagerConcrete(
			leaseClient,
			leaseNamespace,
			logr.Discard(),
			mcpinternal.WithSessionTTL(leaseTTL),
		)

		By("having User A acquire the ephemeral KA Lease and durable AF session")
		sessionA, err := managerA.Takeover(ctx, rrName, mcpinternal.UserInfo{Username: userA})
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(leaseClient.Delete(ctx, &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{Namespace: leaseNamespace, Name: "kubernaut-interactive-" + rrName},
			}))).To(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &isv1alpha1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{Namespace: leaseNamespace, Name: sessionA.SessionID},
			}))).To(Succeed())
		})
		Expect(service.InitializeSessionByRR(ctx, leaseNamespace, rrName, sessionA.SessionID, userA, []string{"sre"})).To(Succeed())

		var initialIS isv1alpha1.InvestigationSession
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: leaseNamespace, Name: sessionA.SessionID}, &initialIS)).To(Succeed())
		Expect(initialIS.Spec.UserIdentity.Username).To(Equal(userA))
		Expect(initialIS.Status.Phase).To(Equal(isv1alpha1.SessionPhaseActive))

		By("waiting for the old Lease to expire, leaving the non-terminal AF session intact")
		time.Sleep(leaseTTL + 300*time.Millisecond)

		By("starting a fresh KA epoch and proving User B cannot claim the RR")
		managerB := mcpinternal.NewLeaseSessionManagerConcrete(
			leaseClient,
			leaseNamespace,
			logr.Discard(),
			mcpinternal.WithSessionTTL(leaseTTL),
		)
		sessionB, err := managerB.Takeover(ctx, rrName, mcpinternal.UserInfo{Username: userB})
		Expect(err).NotTo(HaveOccurred(), "a fresh KA epoch should reclaim the expired Lease")
		Expect(service.InitializeSessionByRR(ctx, leaseNamespace, rrName, sessionB.SessionID, userB, []string{"sre"})).To(MatchError(ContainSubstring("session_active")))
		Expect(managerB.Release(sessionB.SessionID, "durable_owner_denied")).To(Succeed())

		By("letting User A reacquire a fresh Lease without changing durable ownership")
		managerAAfterRestart := mcpinternal.NewLeaseSessionManagerConcrete(
			leaseClient,
			leaseNamespace,
			logr.Discard(),
			mcpinternal.WithSessionTTL(leaseTTL),
		)
		sessionAAfterRestart, err := managerAAfterRestart.Takeover(ctx, rrName, mcpinternal.UserInfo{Username: userA})
		Expect(err).NotTo(HaveOccurred())
		Expect(sessionAAfterRestart.SessionID).NotTo(Equal(sessionA.SessionID))
		Expect(sessionAAfterRestart.SessionID).NotTo(Equal(sessionB.SessionID))
		Expect(service.InitializeSessionByRR(ctx, leaseNamespace, rrName, sessionAAfterRestart.SessionID, userA, []string{"sre"})).To(Succeed())

		var finalSessions isv1alpha1.InvestigationSessionList
		Expect(k8sClient.List(ctx, &finalSessions, client.InNamespace(leaseNamespace))).To(Succeed())
		var ownedByA int
		for i := range finalSessions.Items {
			investigationSession := &finalSessions.Items[i]
			if investigationSession.Spec.RemediationRequestRef.Name == rrName {
				ownedByA++
				Expect(investigationSession.Spec.UserIdentity.Username).To(Equal(userA))
				Expect(investigationSession.Status.Phase).NotTo(BeElementOf(
					isv1alpha1.SessionPhaseCompleted,
					isv1alpha1.SessionPhaseCancelled,
					isv1alpha1.SessionPhaseFailed,
				))
			}
		}
		Expect(ownedByA).To(Equal(1), "Lease expiry and restart must not create a second durable owner")
	})
})
