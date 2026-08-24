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
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	urlLocalhost30080 = "http://localhost:30080"
)

// E2E-FLEET-CC81-001: SOC2 CC8.1 Fleet Reconstruction Compliance
// Validates that ReconstructRemediationRequest returns cluster_id
// for fleet-scoped remediations, proving end-to-end cluster provenance
// through the audit pipeline.
//
// BR-AUDIT-005 v2.0, DD-AUDIT-003 v2.2, SOC2 CC8.1
var _ = Describe("E2E-FLEET-CC81-001: Fleet Reconstruction Compliance [CC8.1]", Ordered, func() {
	var (
		rrName        string
		correlationID string
	)

	It("should include cluster_id in reconstruction response for fleet RRs", func() {
		// Issue #2043: this suite has multiple RR-creation paths sharing the
		// same namespace -- most go through Gateway's normal webhook ingestion
		// (which emits the mandatory `gateway.signal.received` audit event
		// reconstruction requires), but E2E-FLEET-018 creates its RR directly
		// via APIFrontend's kubernaut_remediate A2A/MCP tool
		// (pkg/apifrontend/tools/af_create_rr.go), bypassing Gateway entirely
		// and emitting a different (equally valid, but reconstruction-mapper-
		// unsupported) `apifrontend.rr.created` event instead. Locking onto
		// the FIRST RR matching the spec-level condition below is a race
		// against Ginkgo's spec ordering: if that first match happens to be
		// the APIFrontend-created one, reconstruction is *permanently*
		// impossible for it (not a timing issue -- confirmed via must-gather
		// RCA in #2043), and the old single-candidate retry loop had no way
		// to recover.
		//
		// Fixed by trying EVERY currently-listed candidate RR on each poll,
		// not just the first: this test's actual business intent (BR-AUDIT-005
		// v2.0 CC8.1: cluster_id survives reconstruction for fleet RRs) only
		// needs ONE successfully-reconstructable fleet RR to prove the
		// contract, not a specific one -- so falling through to the next
		// candidate is correct, not a weaker assertion.
		//
		// Follow-up hardening: the candidate filter now requires
		// PhaseCompleted explicitly (previously `OverallPhase != ""`, which
		// matched all 10 non-terminal/failure phases too -- e.g. Pending,
		// Processing, Blocked -- and is exactly how the AF-created RR above,
		// permanently stuck in Blocked, was being swept into candidacy in
		// the first place). This keeps the multi-candidate loop as
		// defense-in-depth for legitimately concurrent completed fleet RRs,
		// without silently accepting non-terminal ones as proof of anything.
		// Per-candidate failures are now joined (not just the last one kept)
		// so a systemic regression across every completed candidate is loud
		// in the failure output, rather than reporting only the final
		// candidate's error. This intentionally does NOT special-case the
		// unrelated, already-tracked #1985 cold-start audit-write race --
		// that failure mode should keep surfacing on its own merits if it
		// recurs, not be silently absorbed here.
		//
		// Further hardening: "reconstructed" alone was too weak a success
		// criterion for picking a candidate to `return` on -- a candidate
		// could get a 200 ReconstructionResponse back with cluster_id still
		// unset/empty (e.g. a genuine cluster_id-mapping bug affecting only
		// some RRs), and the loop would still latch onto it, return, and let
		// the *outer*, non-retried assertions below fail immediately on the
		// real business check -- with no chance for the loop to move on to
		// another, possibly-conformant candidate. The cluster_id check is
		// now part of the loop's own success predicate, so only a candidate
		// that fully satisfies the business contract can end the retry.
		//
		// Review follow-up: the loop used to end with TWO sequential
		// g.Expect calls -- `candidateCount > 0` then `resp != nil` -- but
		// the only place `resp` is ever assigned is the `return` above, so
		// reaching the second check at all already means this tick's loop
		// ran to completion without returning, i.e. `resp` is unconditionally
		// nil there regardless of `candidateCount`. The first check couldn't
		// actually gate anything: whenever it passed (candidateCount > 0),
		// the second check was guaranteed to fail anyway, making it dead
		// code wearing a real-looking assertion. Collapsed into one
		// assertion that always carries both diagnostics (candidateCount and
		// errs) in its failure message, whether zero candidates existed yet
		// or some existed and all failed.
		// Timeout margin (CI RCA, PR #2286, 2026-08-24): a candidate fleet RR
		// reached EffectivenessAssessed only 12s after the prior 2-minute
		// window expired, once E2E-FLEET-019a/b (07_em_fleet_metrics_test.go)
		// started adding two more concurrent EA assessments to this suite's
		// shared EM reconciler queue. 3 minutes restores headroom without
		// masking a genuine regression (a real cluster_id-mapping bug would
		// still fail every candidate, not just run out of time).
		By("Finding a completed fleet RemediationRequest whose audit trail is actually reconstructable")
		var resp *ogenclient.ReconstructionResponse
		Eventually(func(g Gomega) {
			rrList := &remediationv1.RemediationRequestList{}
			g.Expect(k8sClient.List(ctx, rrList, client.InNamespace(namespace))).To(Succeed())

			var candidateCount int
			var errs []error
			for _, rr := range rrList.Items {
				if rr.Spec.ClusterID == "" || rr.Status.OverallPhase != remediationv1.PhaseCompleted {
					continue
				}
				candidateCount++

				result, err := dataStorageClient.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
					CorrelationID: rr.Name,
				})
				if err != nil {
					errs = append(errs, fmt.Errorf("RR %s: %w", rr.Name, err))
					continue
				}
				reconResp, ok := result.(*ogenclient.ReconstructionResponse)
				if !ok {
					errs = append(errs, fmt.Errorf("RR %s: expected ReconstructionResponse, got %T", rr.Name, result))
					continue
				}
				if !reconResp.ClusterID.Set || reconResp.ClusterID.Value == "" {
					errs = append(errs, fmt.Errorf("RR %s: reconstructed but cluster_id not set (CC8.1 violation for this candidate)", rr.Name))
					continue
				}

				rrName = rr.Name
				correlationID = rr.Name
				resp = reconResp
				return
			}

			g.Expect(resp).ToNot(BeNil(),
				"no completed fleet RemediationRequest reconstructed with cluster_id set yet (%d candidate(s) tried): %v",
				candidateCount, errors.Join(errs...))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("Verifying cluster_id is present in reconstruction response")
		Expect(resp.ClusterID.Set).To(BeTrue(),
			"CC8.1 violation: cluster_id missing from reconstruction response for fleet RR %s", rrName)
		Expect(resp.ClusterID.Value).ToNot(BeEmpty(),
			"CC8.1 violation: cluster_id is empty for fleet RR %s", rrName)

		GinkgoWriter.Printf("CC8.1 PASS: cluster_id=%q in reconstruction for %s\n",
			resp.ClusterID.Value, rrName)

		By("Verifying YAML contains clusterID in spec")
		Expect(resp.RemediationRequestYaml).To(ContainSubstring("clusterID:"),
			"CC8.1 violation: reconstructed YAML missing clusterID in spec")
	})

	It("should reconstruct with valid correlation_id", func() {
		// No Skip guard here: correlationID is only ever empty if the
		// preceding ordered spec's Eventually failed, and Ginkgo's Ordered
		// decorator on the enclosing Describe already skips subsequent specs
		// automatically when an earlier one in the same container fails.
		By("Verifying correlation_id in reconstruction response")
		result, err := dataStorageClient.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
			CorrelationID: correlationID,
		})
		Expect(err).ToNot(HaveOccurred())

		reconResp, ok := result.(*ogenclient.ReconstructionResponse)
		Expect(ok).To(BeTrue())
		Expect(reconResp.CorrelationID.Value).To(Equal(correlationID))
	})

	Context("reconstruction without fleet cluster context", func() {
		It("should return empty cluster_id for hub-only RRs", func() {
			// Deliberately create a hub-only RR rather than opportunistically
			// scanning existing RRs for one without spec.clusterID: every RR
			// in this suite is fleet-scoped (DD-TEST-014: fleet E2E targets
			// only the remote cluster for reconciliation), so scanning would
			// never find one and always Skip -- which the project forbids
			// (no Skip()/pending tests; see AGENTS.md TDD Anti-Patterns).
			// Submitting a signal with no "cluster" label reproduces the
			// genuine backward-compatibility case: prometheus_adapter.go
			// reads spec.clusterID from commonLabels["cluster"], which is
			// empty here, so resolverForCluster falls back to the local/hub
			// resolver and the resulting RR's spec.clusterID stays empty.
			By("Creating a hub-only (non-fleet) target resource on the local/hub cluster")
			const targetName = "hub-only-reconstruction-target"
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
			if createErr := k8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
				Expect(createErr).NotTo(HaveOccurred(), "Failed to create %s fixture", targetName)
			}
			DeferCleanup(func() { _ = k8sClient.Delete(context.Background(), dep) })

			By("Submitting a signal with no cluster label to create a hub-only RR")
			payload := buildPrometheusAlertWithCluster("HubOnlyReconstruction", "warning",
				targetName, "")
			gatewayURL := urlLocalhost30080
			body := postFleetAlertUntilAccepted(gatewayURL, payload)

			var response map[string]interface{}
			Expect(json.Unmarshal(body, &response)).To(Succeed())
			Expect(response["status"]).To(Equal("created"),
				"Alert should result in a new hub-only RemediationRequest")
			hubRRName, ok := response["remediationRequestName"].(string)
			Expect(ok).To(BeTrue(), "Response must contain remediationRequestName")

			By("Waiting for the hub-only RR to be picked up by reconciliation")
			Eventually(func(g Gomega) {
				var rr remediationv1.RemediationRequest
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: hubRRName, Namespace: namespace,
				}, &rr)).To(Succeed())
				g.Expect(rr.Spec.ClusterID).To(BeEmpty(), "hub-only RR must not have spec.clusterID set")
				g.Expect(rr.Status.OverallPhase).To(BeElementOf(
					remediationv1.PhasePending, remediationv1.PhaseProcessing, remediationv1.PhaseAnalyzing,
					remediationv1.PhaseAwaitingApproval, remediationv1.PhaseExecuting, remediationv1.PhaseVerifying,
					remediationv1.PhaseBlocked, remediationv1.PhaseCompleted, remediationv1.PhaseFailed,
					remediationv1.PhaseTimedOut, remediationv1.PhaseSkipped),
					"RR must have entered a known reconciliation phase, got %q", rr.Status.OverallPhase)
			}, timeout, interval).Should(Succeed())

			// AGENTS.md testing standard: no fixed time.Sleep() in tests --
			// poll for the actual condition instead. The async audit
			// pipeline's persistence delay is absorbed entirely by this
			// Eventually's own retry loop (it already treats a query error,
			// e.g. audit events not yet persisted, as a retryable failure),
			// so a separate blind sleep before it added nothing but wasted
			// wall-clock time on every run. Budget bumped from 30s to 40s to
			// preserve the same total wait headroom the sleep+30s poll gave.
			By(fmt.Sprintf("Reconstructing hub-only RR: %s (polling for async audit-event persistence)", hubRRName))
			var resp *ogenclient.ReconstructionResponse
			Eventually(func(g Gomega) {
				result, err := dataStorageClient.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
					CorrelationID: hubRRName,
				})
				g.Expect(err).ToNot(HaveOccurred())
				reconResp, ok := result.(*ogenclient.ReconstructionResponse)
				g.Expect(ok).To(BeTrue(), "expected ReconstructionResponse, got %T", result)
				resp = reconResp
			}, 40*time.Second, 2*time.Second).Should(Succeed())

			Expect(resp.ClusterID.Set).To(BeFalse(),
				"hub-only RR should not have cluster_id set")

			GinkgoWriter.Printf("Backward compat PASS: hub-only RR %s has no cluster_id\n", hubRRName)
		})
	})
})
