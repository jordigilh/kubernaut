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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// registerFleetClusterWorkflow creates a RemediationWorkflow catalog entry with
// wildcard severity/environment/component/priority labels (so those four
// mandatory dimensions never gate the result) and the given cluster
// classification label(s), isolating `cluster` as the only discriminating
// filter dimension for E2E-FLEET-1511-001 below.
//
// #1661 Phase 55b: registers via direct CRD creation and waits for the real
// AuthWebhook (deployed in this suite via SetupFullPipelineInfrastructure) to
// stamp .status.workflowId, replacing the retired createWorkflow REST call
// (DD-WORKFLOW-018 -- AuthWebhook is the sole write path).
func registerFleetClusterWorkflow(name string, cluster []string) {
	crd := testutil.NewTestWorkflowCRD(name, "ScaleReplicas", "tekton")
	crd.Spec.Labels.Severity = []string{"*"}
	crd.Spec.Labels.Environment = []string{"*"}
	crd.Spec.Labels.Component = []string{"*"}
	crd.Spec.Labels.Priority = "*"
	crd.Spec.Labels.Cluster = cluster
	crd.Spec.Description.What = fmt.Sprintf("BR-FLEET-003 (#1511) E2E fixture: %s", name)
	crd.Spec.Description.WhenToUse = "E2E-FLEET-1511-001 cluster-scoped workflow targeting"

	content := testutil.MarshalWorkflowCRD(crd)

	rw := &rwv1alpha1.RemediationWorkflow{}
	Expect(yaml.Unmarshal([]byte(content), rw)).To(Succeed(), "unmarshal workflow CRD fixture for %s", name)
	rw.Namespace = namespace

	if createErr := k8sClient.Create(ctx, rw); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
		Expect(createErr).ToNot(HaveOccurred(), "create RemediationWorkflow %s", name)
	}

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, rw)).To(Succeed())
		g.Expect(rw.Status.WorkflowID).ToNot(BeEmpty(),
			"AuthWebhook should stamp .status.workflowId for %s", name)
	}, timeout, interval).Should(Succeed())
}

// E2E-FLEET-1511-001: Cluster-scoped workflow targeting full chain.
// Authority: BR-FLEET-003, DD-FLEET-002, Issue #1511, docs/tests/1511/TEST_PLAN.md
// FedRAMP: AC-4 (Information Flow Enforcement), SC-7 (Boundary Protection)
//
// Validates the complete Wiring Manifest chain end-to-end:
//
//	SP Rego (input.cluster.labels.environment) -> Status.ClusterClassification
//	  -> RO buildSignalContext() -> AIAnalysis.Spec.AnalysisRequest.SignalContext.Cluster
//	  -> DataStorage discovery filter (labels->'cluster' JSONB match)
//
// Three scenarios per the plan's Phase 6 requirement:
//  1. Excluded on mismatch: a workflow classified for a different cluster is
//     never returned once a concrete cluster filter is active.
//  2. Included on match: a workflow classified for the signal's actual
//     cluster classification is returned.
//  3. Included when fleet disabled: omitting the cluster filter entirely
//     (simulating a non-fleet caller) returns both workflows -- backward
//     compatible, no regression for existing non-fleet deployments.
var _ = Describe("E2E-FLEET-1511-001 [AC-4, SC-7]: Cluster-scoped workflow targeting via SP Rego classification (BR-FLEET-003, #1511)", Label("fleet"), func() {
	It("should classify the fleet cluster via Rego, propagate it through RO/AIAnalysis, and scope DataStorage discovery accordingly", func() {
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		matchWorkflowName := "fleet-cluster-match-" + suffix
		mismatchWorkflowName := "fleet-cluster-mismatch-" + suffix

		By("Step 1: Registering a workflow classified for the remote-cluster's fleet classification (production)")
		registerFleetClusterWorkflow(matchWorkflowName, []string{"production"})

		By("Step 1b: Registering a workflow classified for a DIFFERENT cluster (staging-eu) -- must be excluded on mismatch")
		registerFleetClusterWorkflow(mismatchWorkflowName, []string{"staging-eu"})

		By("Step 2: Sending a fleet alert (cluster_id=remote-cluster) to Gateway (AC-4)")
		// Distinct target name from other fleet E2E specs to avoid a duplicate
		// dedup fingerprint race (see 08_full_fleet_journey_test.go's note on
		// the identical issue for "memory-eater").
		const targetName = "memory-eater-cluster-scoped"
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: namespace,
				Labels:    map[string]string{"kubernaut.ai/managed": "true"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": targetName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": targetName}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36"}},
					},
				},
			},
		}
		// Created on the REMOTE cluster (DD-TEST-013): see the equivalent note
		// in 01_signal_ingestion_test.go.
		if createErr := remoteK8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			Expect(createErr).NotTo(HaveOccurred(), "Failed to create %s fixture", targetName)
		}
		DeferCleanup(func() { _ = remoteK8sClient.Delete(context.Background(), dep) })

		payload := buildPrometheusAlertWithCluster("FleetClusterScoped", namespace, "critical",
			"Deployment", targetName, "remote-cluster")

		gatewayURL := "http://localhost:30080"
		_, body := postFleetAlertUntilAccepted(gatewayURL, payload)

		var response map[string]interface{}
		Expect(json.Unmarshal(body, &response)).To(Succeed())
		Expect(response["status"]).To(Equal("created"))
		rrName := response["remediationRequestName"].(string)

		By("Step 3: Verifying SP classifies the cluster as 'production' via input.cluster.labels.environment (BR-FLEET-003 R1)")
		var sp *signalprocessingv1.SignalProcessing
		Eventually(func(g Gomega) {
			spList := &signalprocessingv1.SignalProcessingList{}
			g.Expect(k8sClient.List(ctx, spList, client.InNamespace(namespace))).To(Succeed())

			for i := range spList.Items {
				candidate := &spList.Items[i]
				if candidate.Spec.Signal.ClusterID == "remote-cluster" &&
					candidate.Spec.RemediationRequestRef.Name == rrName {
					sp = candidate
					break
				}
			}
			g.Expect(sp).ToNot(BeNil(), "SP should be created for the fleet signal")
			g.Expect(sp.Status.ClusterClassification).To(Equal("production"),
				"BR-FLEET-003: SP Rego must classify remote-cluster as 'production' "+
					"from the MCPServerRegistration's environment=production onboarding label")
		}, timeout, interval).Should(Succeed())

		By("Step 4: Verifying AIAnalysis carries the propagated cluster classification (RO buildSignalContext(), IT-RO-1511-001 wiring)")
		var ai *aianalysisv1.AIAnalysis
		Eventually(func(g Gomega) {
			aiList := &aianalysisv1.AIAnalysisList{}
			g.Expect(k8sClient.List(ctx, aiList, client.InNamespace(namespace))).To(Succeed())

			for i := range aiList.Items {
				for _, ref := range aiList.Items[i].OwnerReferences {
					if ref.Kind == "RemediationRequest" && ref.Name == rrName {
						ai = &aiList.Items[i]
						break
					}
				}
			}
			g.Expect(ai).ToNot(BeNil(), "RO should have created an AIAnalysis owned by this RR")
			g.Expect(ai.Spec.AnalysisRequest.SignalContext.Cluster).To(Equal("production"),
				"AC-4: AIAnalysis.Spec.AnalysisRequest.SignalContext.Cluster must carry "+
					"SP's ClusterClassification end-to-end")
		}, timeout, interval).Should(Succeed())

		clusterClassification := ai.Spec.AnalysisRequest.SignalContext.Cluster

		By("Step 5: Verifying DataStorage discovery excludes the mismatched workflow and includes the matching one when cluster filter is active (AC-4, SC-7)")
		Eventually(func(g Gomega) {
			resp, err := dataStorageClient.ListWorkflowsByActionType(ctx, ogenclient.ListWorkflowsByActionTypeParams{
				ActionType:  "ScaleReplicas",
				Severity:    ogenclient.ListWorkflowsByActionTypeSeverityCritical,
				Component:   "apps/v1/Deployment",
				Environment: "production",
				Priority:    ogenclient.ListWorkflowsByActionTypePriorityP0,
				Cluster:     ogenclient.NewOptString(clusterClassification),
				Limit:       ogenclient.NewOptInt(100),
			})
			g.Expect(err).ToNot(HaveOccurred())

			discovery, ok := resp.(*ogenclient.WorkflowDiscoveryResponse)
			g.Expect(ok).To(BeTrue(), "Expected *WorkflowDiscoveryResponse, got %T", resp)

			names := make([]string, 0, len(discovery.Workflows))
			for _, wf := range discovery.Workflows {
				names = append(names, wf.WorkflowName)
			}
			g.Expect(names).To(ContainElement(matchWorkflowName),
				"SC-7: workflow classified for 'production' must be discoverable when cluster=production is active")
			g.Expect(names).NotTo(ContainElement(mismatchWorkflowName),
				"AC-4: workflow classified for a different cluster ('staging-eu') must be excluded on mismatch")
		}, timeout, interval).Should(Succeed())

		By("Step 6: Verifying DataStorage discovery includes BOTH workflows when no cluster filter is supplied (fleet-disabled backward compatibility, R6.1)")
		Eventually(func(g Gomega) {
			resp, err := dataStorageClient.ListWorkflowsByActionType(ctx, ogenclient.ListWorkflowsByActionTypeParams{
				ActionType:  "ScaleReplicas",
				Severity:    ogenclient.ListWorkflowsByActionTypeSeverityCritical,
				Component:   "apps/v1/Deployment",
				Environment: "production",
				Priority:    ogenclient.ListWorkflowsByActionTypePriorityP0,
				// Cluster deliberately omitted: simulates a non-fleet caller (KA
				// never sends the param when SP produced no classification).
				Limit: ogenclient.NewOptInt(100),
			})
			g.Expect(err).ToNot(HaveOccurred())

			discovery, ok := resp.(*ogenclient.WorkflowDiscoveryResponse)
			g.Expect(ok).To(BeTrue(), "Expected *WorkflowDiscoveryResponse, got %T", resp)

			names := make([]string, 0, len(discovery.Workflows))
			for _, wf := range discovery.Workflows {
				names = append(names, wf.WorkflowName)
			}
			g.Expect(names).To(ContainElement(matchWorkflowName),
				"R6.1: without a cluster filter, the 'production'-classified workflow remains discoverable")
			g.Expect(names).To(ContainElement(mismatchWorkflowName),
				"R6.1: without a cluster filter, the 'staging-eu'-classified workflow is NOT excluded "+
					"(no cluster condition is added to the SQL at all -- backward compatible)")
		}, timeout, interval).Should(Succeed())

		By("Step 7: Verifying RR progresses past signal processing (sanity check on the overall pipeline)")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: namespace,
			}, &rr)).To(Succeed())
			g.Expect(rr.Status.OverallPhase).ToNot(BeEmpty(),
				"RR should progress through the pipeline")
		}, timeout, interval).Should(Succeed())
	})
})
