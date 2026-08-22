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

package kubernautagent

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
)

// E2E Snapshot Coverage — BR-SESSION-002 / BR-AUDIT-070
//
// The retired GET /api/v1/incident/session/{id}/snapshot endpoint is
// deleted outright (issue #2190, DD-AA-KA-001): forensic post-mortem data
// is now the AgentSession CRD's own Status.Result, always readable via a
// normal Get once RBAC-authorized — there is no separate "snapshot" RPC,
// no 409-while-running conflict status (Get is never exclusive), and no
// cancelled_phase field (cancellation is delete-based with no finalizer,
// so a cancelled investigation's terminal state is never observable via
// Get — confirmed in internal/kubernautagent/agentsession/dispatcher.go's
// cancelOnDelete). What survives here is the underlying business value:
// completed investigations expose a structured, non-empty RCA snapshot,
// and a nonexistent session surfaces a clear not-found signal.

var _ = Describe("E2E-KA-SNAP: AgentSession Forensic Snapshot", Label("e2e", "ka", "snapshot"), func() {

	Context("Completed session snapshot", func() {

		It("E2E-KA-SNAP-001: Completed AgentSession exposes a structured RCA snapshot", func() {
			By("Creating an AgentSession and waiting for completion")
			as := &agentsessionv1.AgentSession{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "as-test-snap-001",
					Namespace: sharedNamespace,
				},
				Spec: agentsessionv1.AgentSessionSpec{
					IncidentID:        "test-snap-001",
					RemediationID:     "test-rem-snap-001",
					SignalName:        "OOMKilled",
					Severity:          "high",
					SignalSource:      "kubernetes",
					ResourceNamespace: "production",
					ResourceKind:      "Pod",
					ResourceName:      "snap-pod-001",
					ErrorMessage:      "Container OOMKilled",
					Environment:       "production",
					Priority:          "P1",
					RiskTolerance:     "medium",
					BusinessCategory:  "standard",
					ClusterName:       "e2e-test",
				},
			}
			Expect(k8sClient.Create(ctx, as)).To(Succeed())

			Eventually(func() agentsessionv1.AgentSessionPhase {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(as), as); err != nil {
					return ""
				}
				return as.Status.Phase
			}, 60*time.Second, 1*time.Second).Should(Equal(agentsessionv1.AgentSessionPhaseCompleted),
				"investigation should complete")

			By("Validating the snapshot fields on Status")
			Expect(as.Status.SessionID).ToNot(BeEmpty(), "sessionID should be set")
			Expect(as.CreationTimestamp.IsZero()).To(BeFalse(), "creationTimestamp should be set by the API server")
			Expect(as.Status.Result).ToNot(BeNil(), "Status.Result must be populated on Completed")

			By("Validating RCA summary is populated (Mock LLM always produces one)")
			Expect(as.Status.Result.RootCauseAnalysis).ToNot(BeNil(),
				"rootCauseAnalysis should be set for completed investigation")
			Expect(as.Status.Result.RootCauseAnalysis.Raw).ToNot(BeEmpty(),
				"rootCauseAnalysis should not be empty")

			var rca map[string]json.RawMessage
			Expect(json.Unmarshal(as.Status.Result.RootCauseAnalysis.Raw, &rca)).To(Succeed(),
				"rootCauseAnalysis must be valid JSON")
			Expect(rca).ToNot(BeEmpty(), "rootCauseAnalysis should decode to a non-empty object")
		})
	})

	Context("Non-existent session snapshot", func() {

		It("E2E-KA-SNAP-003: Get on a non-existent AgentSession returns NotFound", func() {
			as := &agentsessionv1.AgentSession{}
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: sharedNamespace,
				Name:      fmt.Sprintf("as-nonexistent-%d", GinkgoRandomSeed()),
			}, as)

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"Get on a non-existent AgentSession should return a NotFound error")
		})
	})
})
