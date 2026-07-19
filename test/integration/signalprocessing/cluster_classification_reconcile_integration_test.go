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

package signalprocessing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet/fleettest"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// IT-SP-1511-002: Rego `cluster` classification through the real
// SignalProcessing reconcile loop (BR-FLEET-003, #1511).
//
// Authority: docs/tests/1511/TEST_PLAN.md,
// docs/requirements/BR-FLEET-003-cluster-scoped-workflow-targeting.md
//
// Scope: unlike IT-SP-1511-001 (which proves ClusterRegistry -> K8sEnricher
// wiring against the enricher directly), this test exercises the FULL
// reconcile loop -- SP CR creation, Enriching phase (K8sEnricher.Enrich
// populates KubernetesContext.Cluster), Classifying phase
// (PolicyEvaluator.EvaluateCluster via evaluateClusterOrSkip), and status
// persistence (Status.ClusterClassification via finalizeClassification) --
// against the exact same envtest reconciler used by every other test in this
// suite.
//
// Serial: mutates the shared hot-reload policy file (labelsPolicyFilePath)
// and the shared K8sEnricher's ClusterRegistry (sharedK8sEnricher), the same
// legitimate shared-resource constraint documented on the "SignalProcessing
// Hot-Reload Integration" Describe block (DD-TEST-010).
var _ = Describe("IT-SP-1511-002: cluster classification through the real reconcile loop (BR-FLEET-003)", Serial, Label("integration", "signalprocessing", "fleet"), func() {
	// clusterLabelPolicy mirrors the example shipped in
	// charts/kubernaut/examples/signalprocessing-policy.rego: no default rule,
	// so an unmatched/empty label is a valid "no classification" outcome
	// rather than an error (BR-FLEET-003 R2).
	const clusterLabelPolicy = `default labels := {}

# BR-FLEET-003 (#1511): classify the fleet cluster from its onboarding label.
cluster := input.cluster.labels.environment if {
    input.cluster.labels.environment != ""
}
`
	const noClusterRulePolicy = `default labels := {}
`

	AfterEach(func() {
		By("Restoring the original Rego policy and clearing the shared ClusterRegistry")
		updateLabelsPolicyFile(noClusterRulePolicy)
		sharedK8sEnricher.SetClusterRegistry(nil)
	})

	It("IT-SP-1511-002a: populates Status.ClusterClassification from a registered cluster's onboarding label", func() {
		ns := createTestNamespace(ctx, "fleet-1511-002a")
		defer deleteTestNamespace(ns)

		By("Loading a Rego policy with a cluster classification rule")
		updateLabelsPolicyFile(clusterLabelPolicy)

		By("Wiring a ClusterRegistry with a registered fleet cluster")
		sharedK8sEnricher.SetClusterRegistry(&fleettest.StubClusterQuerier{
			Clusters: map[string]registry.ClusterInfo{
				"prod-east-1": {
					ID:     "prod-east-1",
					Labels: map[string]string{"environment": "production", "tier": "gold"},
				},
			},
		})

		By("Creating a SignalProcessing CR whose signal originates from the registered cluster")
		targetResource := signalprocessingv1alpha1.ResourceIdentifier{Kind: "Node", Name: "node-fleet-1511-002a"}
		fingerprint := GenerateTestFingerprint("it-sp-1511-002a")
		rr := CreateTestRemediationRequest("it-sp-1511-002a-rr", ns, fingerprint, "critical", targetResource)
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		sp := CreateTestSignalProcessingWithParent("it-sp-1511-002a", ns, rr, fingerprint, targetResource)
		sp.Spec.Signal.ClusterID = "prod-east-1"
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())
		defer func() { _ = deleteAndWait(sp) }()

		By("Waiting for completion")
		Expect(waitForCompletion(sp.Name, sp.Namespace)).To(Succeed())

		By("Verifying Status.ClusterClassification was persisted via the real reconcile loop")
		var final signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, &final)).To(Succeed())
		Expect(final.Status.ClusterClassification).To(Equal("production"))
	})

	It("IT-SP-1511-002b: leaves Status.ClusterClassification empty for an unregistered cluster (graceful degradation, SI-10)", func() {
		ns := createTestNamespace(ctx, "fleet-1511-002b")
		defer deleteTestNamespace(ns)

		By("Loading a Rego policy with a cluster classification rule")
		updateLabelsPolicyFile(clusterLabelPolicy)

		By("Wiring a ClusterRegistry with no matching registration")
		sharedK8sEnricher.SetClusterRegistry(&fleettest.StubClusterQuerier{Clusters: map[string]registry.ClusterInfo{}})

		By("Creating a SignalProcessing CR whose signal originates from an unregistered cluster")
		targetResource := signalprocessingv1alpha1.ResourceIdentifier{Kind: "Node", Name: "node-fleet-1511-002b"}
		fingerprint := GenerateTestFingerprint("it-sp-1511-002b")
		rr := CreateTestRemediationRequest("it-sp-1511-002b-rr", ns, fingerprint, "critical", targetResource)
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		sp := CreateTestSignalProcessingWithParent("it-sp-1511-002b", ns, rr, fingerprint, targetResource)
		sp.Spec.Signal.ClusterID = "unregistered-cluster"
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())
		defer func() { _ = deleteAndWait(sp) }()

		By("Waiting for completion")
		Expect(waitForCompletion(sp.Name, sp.Namespace)).To(Succeed())

		By("Verifying Status.ClusterClassification stays empty and completion is not blocked")
		var final signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, &final)).To(Succeed())
		Expect(final.Status.ClusterClassification).To(BeEmpty())
	})

	It("IT-SP-1511-002c: leaves Status.ClusterClassification empty when fleet mode is disabled (no ClusterRegistry configured)", func() {
		ns := createTestNamespace(ctx, "fleet-1511-002c")
		defer deleteTestNamespace(ns)

		By("Loading a Rego policy with a cluster classification rule, but wiring no ClusterRegistry")
		updateLabelsPolicyFile(clusterLabelPolicy)

		By("Creating a SignalProcessing CR with no ClusterID (non-fleet signal)")
		targetResource := signalprocessingv1alpha1.ResourceIdentifier{Kind: "Node", Name: "node-fleet-1511-002c"}
		fingerprint := GenerateTestFingerprint("it-sp-1511-002c")
		rr := CreateTestRemediationRequest("it-sp-1511-002c-rr", ns, fingerprint, "critical", targetResource)
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		sp := CreateTestSignalProcessingWithParent("it-sp-1511-002c", ns, rr, fingerprint, targetResource)
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())
		defer func() { _ = deleteAndWait(sp) }()

		By("Waiting for completion")
		Expect(waitForCompletion(sp.Name, sp.Namespace)).To(Succeed())

		By("Verifying Status.ClusterClassification stays empty")
		var final signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, &final)).To(Succeed())
		Expect(final.Status.ClusterClassification).To(BeEmpty())
	})
})
