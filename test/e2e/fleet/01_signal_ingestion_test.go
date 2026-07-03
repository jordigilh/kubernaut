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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-001: Signal ingestion with cluster_id creates RR with spec.clusterID
// Authority: Issue #54, ADR-068
// FedRAMP: AC-4 (information flow enforcement -- cluster provenance)
var _ = Describe("E2E-FLEET-001 [AC-4]: Signal ingestion with cluster_id creates RR with spec.clusterID (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should create RR with spec.clusterID when alert has cluster label", func() {
		// Issue #54 flakiness fix: distinct resource name from E2E-FLEET-002's east
		// payload below. Both used "memory-eater" + "prod-east", producing an identical
		// SHA256(clusterID:namespace:kind:name) fingerprint (see
		// pkg/gateway/types/fingerprint.go CalculateClusterAwareFingerprint). Ginkgo
		// runs these independent Describe blocks across parallel processes with no
		// ordering guarantee, so the two specs raced for the same dedup slot and
		// non-deterministically returned 500/200 instead of 201.
		//
		// The renamed target must exist as a real K8s object: Gateway's owner
		// resolution (pkg/gateway/types/fingerprint.go ResolveFingerprintWithCluster,
		// invoked via prometheus_adapter.go resolverForCluster) does a live lookup of
		// the target resource -- falling back to the local K8s API when the cluster's
		// MCP tools aren't registered (as is the case for the synthetic "prod-east"
		// cluster ID) -- and drops the signal with a 400/500 when the resource is not
		// found. It does NOT fall through gracefully like the separate scope/managed
		// check (pkg/shared/scope/manager.go) does.
		const targetName = "memory-eater-signalingest"
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: namespace,
				// BR-SCOPE-001/ADR-053: the resource-level label is required. The
				// namespace is also labeled kubernaut.ai/managed=true (SynchronizedBeforeSuite),
				// but relying on that fallback alone was observed to still return
				// "resource not managed by Kubernaut" for unlabeled fixtures, so label
				// the resource directly like the shared memory-eater fixture does
				// (test/infrastructure/fullpipeline_e2e.go DeployMemoryEaterWithLimits).
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
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
		// Created on the REMOTE cluster (DD-TEST-013): AllRegistrationsRemote
		// backs prod-east/prod-west with the same remote bridge kube-mcp-server
		// reads from, so the target Gateway's owner resolution looks up must
		// live there, not on the primary cluster.
		if createErr := remoteK8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			Expect(createErr).NotTo(HaveOccurred(), "Failed to create %s fixture", targetName)
		}
		DeferCleanup(func() { _ = remoteK8sClient.Delete(context.Background(), dep) })

		payload := buildPrometheusAlertWithCluster("FleetSignalIngestion", namespace, "critical",
			"Deployment", targetName, "prod-east")

		gatewayURL := "http://localhost:30080"
		_, body := postFleetAlertUntilAccepted(gatewayURL, payload)

		var response map[string]interface{}
		Expect(json.Unmarshal(body, &response)).To(Succeed())

		Expect(response["status"]).To(Equal("created"),
			"Alert should result in new RemediationRequest")

		rrName, ok := response["remediationRequestName"].(string)
		Expect(ok).To(BeTrue(), "Response must contain remediationRequestName")

		By("Verifying RR has spec.clusterID set to prod-east (AC-4: cluster provenance)")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: namespace,
			}, &rr)).To(Succeed())
			g.Expect(rr.Spec.ClusterID).To(Equal("prod-east"),
				"RR spec.clusterID must match alert cluster label")
		}, timeout, interval).Should(Succeed())
	})
})

// E2E-FLEET-002: Cluster-scoped dedup -- same resource on different clusters
// Authority: Issue #54, ADR-068
// FedRAMP: AC-3 (access enforcement -- distinct cluster identities)
var _ = Describe("E2E-FLEET-002 [AC-3]: Cluster-scoped dedup produces distinct fingerprints (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should produce different RRs for same resource on different clusters", func() {
		payloadEast := buildPrometheusAlertWithCluster("FleetDedup", namespace, "warning",
			"Deployment", "memory-eater", "prod-east")
		payloadWest := buildPrometheusAlertWithCluster("FleetDedup", namespace, "warning",
			"Deployment", "memory-eater", "prod-west")

		gatewayURL := "http://localhost:30080"
		_, bodyEast := postFleetAlertUntilAccepted(gatewayURL, payloadEast)
		_, bodyWest := postFleetAlertUntilAccepted(gatewayURL, payloadWest)

		var eastResp, westResp map[string]interface{}
		Expect(json.Unmarshal(bodyEast, &eastResp)).To(Succeed())
		Expect(json.Unmarshal(bodyWest, &westResp)).To(Succeed())

		Expect(eastResp["status"]).To(Equal("created"))
		Expect(westResp["status"]).To(Equal("created"))

		Expect(eastResp["remediationRequestName"]).ToNot(Equal(westResp["remediationRequestName"]),
			"AC-3: same resource on different clusters must create separate RRs with distinct fingerprints")
	})
})

func buildPrometheusAlertWithCluster(alertName, ns, severity, kind, name, clusterID string) []byte {
	payload := map[string]interface{}{
		"version":  "4",
		"groupKey": fmt.Sprintf("{}:{alertname=\"%s\"}", alertName),
		"status":   "firing",
		"commonLabels": map[string]string{
			"cluster": clusterID,
		},
		"alerts": []map[string]interface{}{
			{
				"status": "firing",
				"labels": map[string]string{
					"alertname":           alertName,
					"namespace":           ns,
					"severity":            severity,
					strings.ToLower(kind): name,
				},
				"annotations": map[string]string{
					"description": fmt.Sprintf("Fleet E2E test: %s/%s on %s", ns, name, clusterID),
				},
				"startsAt": time.Now().Format(time.RFC3339),
			},
		},
	}
	data, _ := json.Marshal(payload)
	return data
}
