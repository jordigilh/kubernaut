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

package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// kaToolE2ETargetName mirrors the mock-llm scenario's constant of the same
// name (test/services/mock-llm/scenarios/scenario_ka_fleet_investigation.go)
// -- duplicated as a literal since E2E tests don't import mock-llm packages
// (same convention as afFleetKubectlE2EKeyword/coredns in 16's scenario).
const kaToolE2ETargetName = "ka-tool-e2e-target"

// kaToolE2EKeyword must match the mock-llm scenario's Match keyword exactly.
const kaToolE2EKeyword = "ka-tool-e2e-test"

// kaToolE2ELocalEvidence/RemoteEvidence mirror the mock-llm scenario's
// memory-limit evidence constants. Each It() deploys ka-tool-e2e-target with
// this exact memory limit on the one cluster it targets, then asserts the
// same value surfaces in the final RCA -- proving KA's real investigation
// loop called the correct tool (kubectl_get_by_name locally, resources_get
// via the real MCP Gateway for fleet) and reached the correct cluster, not
// just some cluster with a same-named resource on it.
const (
	kaToolE2ELocalEvidence  = "111Mi"
	kaToolE2ERemoteEvidence = "222Mi"
)

// runKAToolCallE2ECase drives the shared E2E-FLEET-017 flow end to end:
// deploy the dedicated ka-tool-e2e-target marker on the target cluster with
// evidence as its memory limit, post the alert with (fleet) or without
// (hub-local) clusterID -- the ONLY thing that varies between the two
// It()s below -- wait for AIAnalysis to complete, and assert the evidence
// value appears in the RCA. Neither this helper nor the mock-llm scenario
// it drives is ever told which environment it's running against; KA's own
// tool-schema advertisement (toolDefinitionsForPhase) and overlay routing
// (executeResolved) determine that, opaquely, from clusterID alone.
func runKAToolCallE2ECase(targetKubeconfig string, targetClient client.Client, clusterID, evidence string) {
	runKAToolCallE2ECaseWithAlert(targetKubeconfig, targetClient, clusterID, evidence, kaToolE2EKeyword)
}

func runKAToolCallE2ECaseWithAlert(targetKubeconfig string, targetClient client.Client, clusterID, evidence, alertName string) *aianalysisv1.AIAnalysis {
	By(fmt.Sprintf("Deploying dedicated %s marker (memLimit=%s) on the target cluster", kaToolE2ETargetName, evidence))
	Expect(infrastructure.DeployMemoryEaterNamed(ctx, kaToolE2ETargetName, namespace,
		targetKubeconfig, evidence, "20Mi", GinkgoWriter)).To(Succeed())
	DeferCleanup(func() {
		dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: kaToolE2ETargetName, Namespace: namespace}}
		_ = targetClient.Delete(context.Background(), dep)
	})

	By("Waiting for the marker Deployment to become Available")
	Eventually(func(g Gomega) {
		dep := &appsv1.Deployment{}
		g.Expect(targetClient.Get(ctx, client.ObjectKey{Name: kaToolE2ETargetName, Namespace: namespace}, dep)).To(Succeed())
		g.Expect(dep.Status.AvailableReplicas).To(BeNumerically(">=", 1))
	}, 2*time.Minute, 2*time.Second).Should(Succeed())

	if clusterID != "" {
		By("Waiting for the marker Deployment to be readable through the real MCP Gateway")
		mcpClient, err := newFleetMCPClient(ctx)
		Expect(err).ToNot(HaveOccurred(), "MCP Gateway must be ready before posting the fleet alert")
		DeferCleanup(func() { _ = mcpClient.Close() })

		Eventually(func(g Gomega) {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			g.Expect(mcpClient.Get(ctx, client.ObjectKey{Name: kaToolE2ETargetName, Namespace: namespace}, obj)).To(Succeed())
			g.Expect(obj.GetName()).To(Equal(kaToolE2ETargetName))
		}, 90*time.Second, 2*time.Second).Should(Succeed(),
			"the remote marker must be visible to resources_get before Gateway owner resolution")
	}

	By(fmt.Sprintf("Sending the alert (cluster_id=%q)", clusterID))
	payload := buildPrometheusAlertWithCluster(alertName, "high", kaToolE2ETargetName, clusterID)
	body := postFleetAlertUntilAccepted(urlLocalhost30080, payload)

	var response map[string]interface{}
	Expect(json.Unmarshal(body, &response)).To(Succeed())
	Expect(response["status"]).To(Equal("created"))
	rrName, ok := response["remediationRequestName"].(string)
	Expect(ok).To(BeTrue(), "Response must contain remediationRequestName")

	By("Waiting for KA's investigation to complete (AIAnalysis -> Completed)")
	var ai aianalysisv1.AIAnalysis
	Eventually(func(g Gomega) {
		aiList := &aianalysisv1.AIAnalysisList{}
		g.Expect(k8sClient.List(ctx, aiList, client.InNamespace(namespace))).To(Succeed())

		var found *aianalysisv1.AIAnalysis
		for i := range aiList.Items {
			for _, ref := range aiList.Items[i].OwnerReferences {
				if ref.Kind == remediationRequestKind && ref.Name == rrName {
					found = &aiList.Items[i]
					break
				}
			}
		}
		g.Expect(found).ToNot(BeNil(), "RO should have created an AIAnalysis owned by RR %s", rrName)
		g.Expect(found.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted),
			"AIAnalysis %s should reach Completed (phase=%s message=%q)",
			found.Name, found.Status.Phase, found.Status.Message)
		ai = *found
	}, timeout, interval).Should(Succeed())

	By("Verifying the RCA reflects a genuine, correctly-targeted tool call")
	Expect(ai.Status.GetRCAResult().RootCauseAnalysis).ToNot(BeNil(), "Completed AIAnalysis must carry rootCauseAnalysis")
	Expect(ai.Status.RCAResult.RootCauseAnalysis.Summary).To(ContainSubstring(evidence),
		"AC-4/AC-6, SI-4: the RCA summary must contain the environment-specific evidence %q, proving KA's "+
			"real investigation loop called the correct tool for this environment and reached the correct "+
			"cluster's live object -- not a canned response and not a wrong-cluster false positive", evidence)
	GinkgoWriter.Printf("  E2E-FLEET-017: confirmed genuine, correctly-targeted round trip (evidence=%s)\n", evidence)
	return &ai
}

// E2E-FLEET-017 [AC-4, AC-6, SI-4]: real KA investigation calls the correct
// tool for hub-local vs. fleet targets, closing issue #1729.
//
// Authority: issue #1729, DD-FLEET-005, ADR-068.
//
// Why this test exists (what it closes that no other test does): before
// #1729, KubernautAgent had no Helm-exposed `fleet` configuration block at
// all (unlike apifrontend/effectivenessmonitor/remediationorchestrator), so
// a Helm-deployed KA in a fleet-enabled cluster silently ran with
// fleetOverlayResolver == nil for every investigation, regardless of
// clusterID -- a fleet-tagged signal would be "investigated" entirely
// against KA's own hub cluster, producing a plausible-looking but wrong RCA
// with no error surfaced anywhere. No existing test caught this: unit and
// integration tests construct the Investigator directly with a real or fake
// resolver, bypassing Helm entirely; 06_aa_fleet_investigation_test.go
// drives newFleetMCPClient directly against the MCP gateway, never a real
// KA binary. This test closes that gap by driving a real, Helm-deployed KA
// through its full real investigation loop -- signal -> RemediationRequest
// -> SignalProcessing -> AIAnalysis -- against BOTH a hub-local and a fleet
// target, in the same run, with the test itself never told which internal
// tool KA should call for either case (see runKAToolCallE2ECase and
// scenario_ka_fleet_investigation.go's kaToolCallForAvailability).
var _ = Describe("E2E-FLEET-017 [AC-4, AC-6, SI-4]: KA real investigation calls the correct tool for hub-local vs. fleet targets (issue #1729)", Label("fleet", "ka"), func() {
	It("hub-local: should call kubectl_get_by_name directly and return genuine hub-cluster data", func() {
		runKAToolCallE2ECase(kubeconfigPath, k8sClient, "", kaToolE2ELocalEvidence)
	})

	It("fleet: should call resources_get via the real MCP Gateway and return genuine remote-cluster data", func() {
		runKAToolCallE2ECase(remoteKubeconfigPath, remoteK8sClient, remoteCluster, kaToolE2ERemoteEvidence)
	})

	It("fleet: should detect remote HPA and PDB labels through the real MCP Gateway", func() {
		By("Creating remote infrastructure whose labels must be detected by KA")
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: kaToolE2ETargetName + "-hpa", Namespace: namespace},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: kaToolE2ETargetName},
				MinReplicas:    ptr.To[int32](1), MaxReplicas: 3,
			},
		}
		Expect(remoteK8sClient.Create(ctx, hpa)).To(Succeed())
		DeferCleanup(func() { _ = remoteK8sClient.Delete(context.Background(), hpa) })

		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: kaToolE2ETargetName + "-pdb", Namespace: namespace},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: ptr.To(intstr.FromInt(1)),
				Selector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": kaToolE2ETargetName}},
			},
		}
		Expect(remoteK8sClient.Create(ctx, pdb)).To(Succeed())
		DeferCleanup(func() { _ = remoteK8sClient.Delete(context.Background(), pdb) })

		// Keep this signal distinct from E2E-FLEET-017's preceding fleet case;
		// otherwise Gateway deduplication rejects it before KA can inspect labels.
		ai := runKAToolCallE2ECaseWithAlert(remoteKubeconfigPath, remoteK8sClient, remoteCluster, kaToolE2ERemoteEvidence, kaToolE2EKeyword+"-labels")
		Expect(ai.Status.PostRCAContext).NotTo(BeNil(), "ADR-056: remote investigation must persist post-RCA context")
		Expect(ai.Status.PostRCAContext.DetectedLabels).NotTo(BeNil(), "BR-INTEGRATION-1489: remote detected labels must be persisted")
		Expect(ai.Status.PostRCAContext.DetectedLabels.HPAEnabled).To(BeTrue(), "AC-4/AC-6: HPA label must come from the remote cluster")
		Expect(ai.Status.PostRCAContext.DetectedLabels.PDBProtected).To(BeTrue(), "AC-4/AC-6: PDB label must come from the remote cluster")
	})
})
