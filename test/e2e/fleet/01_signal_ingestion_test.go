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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-001: Signal ingestion with cluster_id creates RR with spec.clusterID
// Authority: Issue #54, ADR-068
// FedRAMP: AC-4 (information flow enforcement -- cluster provenance)
var _ = Describe("E2E-FLEET-001 [AC-4]: Signal ingestion with cluster_id creates RR with spec.clusterID (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should create RR with spec.clusterID when alert has cluster label", func() {
		payload := buildPrometheusAlertWithCluster("FleetSignalIngestion", "default", "critical",
			"Deployment", "nginx-fleet-001", "prod-east")

		gatewayURL := "http://localhost:30080"
		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			"application/json",
			strings.NewReader(string(payload)))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"Gateway should accept alert with cluster label")

		body, _ := io.ReadAll(resp.Body)
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
		payloadEast := buildPrometheusAlertWithCluster("FleetDedup", "default", "warning",
			"Deployment", "nginx-fleet-002", "prod-east")
		payloadWest := buildPrometheusAlertWithCluster("FleetDedup", "default", "warning",
			"Deployment", "nginx-fleet-002", "prod-west")

		gatewayURL := "http://localhost:30080"
		respEast, err := postWithFleetAuth(gatewayURL+"/api/v1/signals/prometheus",
			"application/json", strings.NewReader(string(payloadEast)))
		Expect(err).ToNot(HaveOccurred())
		defer respEast.Body.Close()
		Expect(respEast.StatusCode).To(Equal(http.StatusOK))

		respWest, err := postWithFleetAuth(gatewayURL+"/api/v1/signals/prometheus",
			"application/json", strings.NewReader(string(payloadWest)))
		Expect(err).ToNot(HaveOccurred())
		defer respWest.Body.Close()
		Expect(respWest.StatusCode).To(Equal(http.StatusOK))

		var eastResp, westResp map[string]interface{}
		bodyEast, _ := io.ReadAll(respEast.Body)
		bodyWest, _ := io.ReadAll(respWest.Body)
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
