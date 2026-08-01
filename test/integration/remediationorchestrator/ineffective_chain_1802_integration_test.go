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

// Integration tests for Issue #1802: target-resource-scoped ineffective
// remediation chain detection.
//
// BR-ORCH-042.5: Ineffective Remediation Chain Detection MUST be scoped by
// target resource (namespace/kind/name), not spec hash alone.
//
// Pattern: Real DataStorage + real PostgreSQL (dsClients.OpenAPIClient,
// wired the same way as suite_test.go's production DSHistoryAdapter), real
// RoutingEngine.CheckIneffectiveRemediationChain call (direct business logic
// call, no net/http wrapping of RO's own surface — TESTING_GUIDELINES.md
// "HTTP Testing in Integration Tests" anti-pattern). Audit events are seeded
// via the same audit manager + AuditStore.StoreAudit/Flush path RO's own
// controller uses in production (pkg/remediationorchestrator/audit), fed to
// DataStorage's real ingestion API — this is "given" test data for the
// ROUTING ENGINE's query-scoping logic under test, not a substitute for
// testing RO controller reconciliation (the anti-pattern documented in the
// now-deleted audit_integration_test.go concerned seeding events to test the
// audit *client library* itself, a different concern).
package remediationorchestrator

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("CheckIneffectiveRemediationChain target-resource scoping (Issue #1802)", Label("integration", "issue-1802"), func() {
	var (
		engine       *routing.RoutingEngine
		auditManager *roaudit.Manager
		testID       string
	)

	BeforeEach(func() {
		testID = uuid.New().String()[:8]
		auditManager = roaudit.NewManager(roaudit.ServiceName)

		dsHistoryAdapter := routing.NewDSHistoryAdapter(dsClients.OpenAPIClient)
		engine = routing.NewRoutingEngine(
			k8sClient, k8sClient, "",
			routing.Config{
				IneffectiveChainThreshold: 3,
				RecurrenceCountThreshold:  5,
				IneffectiveTimeWindow:     4 * time.Hour,
			},
			&mocks.AlwaysManagedScopeChecker{},
			dsHistoryAdapter,
		)
	})

	// seedROEvent stores a real remediation.workflow_created audit event in
	// DataStorage (via the same auditManager + AuditStore path RO's own
	// controller uses — pkg/remediationorchestrator/audit), then flushes to
	// guarantee it is queryable before the test proceeds.
	seedROEvent := func(correlationID, namespace, targetResource, preHash string) {
		GinkgoHelper()
		event, err := auditManager.BuildRemediationWorkflowCreatedEvent(
			correlationID, namespace, correlationID, "", // clusterID="" -- unscoped (Issue #1802 target-resource-only repro)
			roaudit.RemediationWorkflowCreatedData{
				PreRemediationSpecHash: preHash,
				TargetResource:         targetResource,
				WorkflowID:             "wf-" + correlationID,
				ActionType:             "ScaleUp",
				SignalType:             "HighCPULoad",
				SignalFingerprint:      "fp-" + correlationID,
			},
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to build RO workflow_created audit event")
		Expect(auditStore.StoreAudit(ctx, event)).To(Succeed(), "Failed to store RO audit event")
	}

	It("IT-RO-1802-001: two Deployments with identical spec hash in different namespaces do not cross-block (exact #1802 repro)", func() {
		sharedHash := "sha256:1802-shared-" + testID

		nsA := "prod-1802a-" + testID
		nsB := "prod-1802b-" + testID
		targetA := routing.TargetResource{Namespace: nsA, Kind: "Deployment", Name: "app"}
		targetB := routing.TargetResource{Namespace: nsB, Kind: "Deployment", Name: "app"}

		// Given: target A has 3 consecutive remediations sharing sharedHash as
		// their preRemediationSpecHash — exactly enough to trip
		// IneffectiveChainThreshold (Layer 1+2: HashMatch == preRemediation,
		// since preHash == the incoming query's currentSpecHash).
		for i := 0; i < 3; i++ {
			seedROEvent(
				fmt.Sprintf("corr-1802-a-%s-%d", testID, i),
				nsA, targetA.String(), sharedHash,
			)
		}
		Expect(auditStore.Flush(ctx)).To(Succeed(), "Failed to flush seeded RO audit events to DataStorage")

		rrA := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-1802-a-" + testID, Namespace: nsA, UID: types.UID("uid-1802-a-" + testID)},
		}

		// Sanity check: target A's OWN chain is real and must block — this
		// proves the seeded data + query wiring actually engage the
		// ineffective-chain detector (a false-negative here would mask a
		// false-negative below).
		blockedA := engine.CheckIneffectiveRemediationChain(ctx, rrA, targetA, sharedHash, "ScaleUp")
		Expect(blockedA).ToNot(BeNil(), "target A's own 3-entry chain must trigger the ineffective-chain block")
		Expect(blockedA.Reason).To(Equal(string(remediationv1.BlockReasonIneffectiveChain)))

		// When: target B (different namespace, same Kind/Name, NO remediation
		// history of its own) is checked with the SAME spec hash.
		rrB := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-1802-b-" + testID, Namespace: nsB, UID: types.UID("uid-1802-b-" + testID)},
		}
		blockedB := engine.CheckIneffectiveRemediationChain(ctx, rrB, targetB, sharedHash, "ScaleUp")

		// Then: target B must NOT be blocked. Before the #1802 fix,
		// QueryROEventsBySpecHash matched purely on spec hash, so target A's
		// 3-entry chain would have leaked into target B's query result and
		// incorrectly blocked a target that has never been remediated.
		Expect(blockedB).To(BeNil(),
			"Issue #1802 regression: target B must not be blocked by target A's cross-namespace "+
				"same-hash remediation chain — target-resource scoping must isolate the two targets")
	})

	It("IT-RO-1802-002: RemediationRequest.Spec.ClusterID reaches the DS query and isolates fleet clusters (main only)", func() {
		sharedHash := "sha256:1802-fleet-" + testID
		ns := "prod-1802fleet-" + testID
		// Same target (namespace/kind/name) on two different clusters -- a
		// realistic fleet scenario (GitOps-templated manifests applied
		// identically across clusters).
		target := routing.TargetResource{Namespace: ns, Kind: "Deployment", Name: "app"}

		for i := 0; i < 3; i++ {
			event, err := auditManager.BuildRemediationWorkflowCreatedEvent(
				fmt.Sprintf("corr-1802-fleet-clusterA-%s-%d", testID, i),
				ns, fmt.Sprintf("corr-1802-fleet-clusterA-%s-%d", testID, i), "cluster-a-"+testID,
				roaudit.RemediationWorkflowCreatedData{
					PreRemediationSpecHash: sharedHash,
					TargetResource:         target.String(),
					WorkflowID:             "wf-fleet-a",
					ActionType:             "ScaleUp",
					SignalType:             "HighCPULoad",
					SignalFingerprint:      "fp-fleet-a-" + testID,
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(auditStore.StoreAudit(ctx, event)).To(Succeed())
		}
		Expect(auditStore.Flush(ctx)).To(Succeed())

		// RR arriving on cluster-b, same target, same spec hash, but ZERO
		// history of its own on cluster-b.
		rrClusterB := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-1802-fleetb-" + testID, Namespace: ns, UID: types.UID("uid-1802-fleetb-" + testID)},
			Spec:       remediationv1.RemediationRequestSpec{ClusterID: "cluster-b-" + testID},
		}
		targetClusterB := target
		targetClusterB.ClusterID = rrClusterB.Spec.ClusterID

		blockedClusterB := engine.CheckIneffectiveRemediationChain(ctx, rrClusterB, targetClusterB, sharedHash, "ScaleUp")
		Expect(blockedClusterB).To(BeNil(),
			"cluster-b must not be blocked by cluster-a's identically-named-resource chain -- "+
				"RemediationRequest.Spec.ClusterID must reach the DS query and isolate fleet clusters")

		// Sanity: cluster-a's own chain (matching ClusterID) still blocks.
		rrClusterA := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-1802-fleeta-" + testID, Namespace: ns, UID: types.UID("uid-1802-fleeta-" + testID)},
			Spec:       remediationv1.RemediationRequestSpec{ClusterID: "cluster-a-" + testID},
		}
		targetClusterA := target
		targetClusterA.ClusterID = rrClusterA.Spec.ClusterID
		blockedClusterA := engine.CheckIneffectiveRemediationChain(ctx, rrClusterA, targetClusterA, sharedHash, "ScaleUp")
		Expect(blockedClusterA).ToNot(BeNil(), "cluster-a's own 3-entry chain must still trigger the block")
	})
})
