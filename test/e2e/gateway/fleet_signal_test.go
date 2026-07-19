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

package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	trueFixture = "true"
)

// E2E-FLEET-004: GW cluster-aware alert ingestion E2E.
// Validates that posting a Prometheus alert with commonLabels.cluster produces
// a RemediationRequest with the correct spec.clusterID and a cluster-aware fingerprint.
//
// This test exercises the full HTTP path:
//
//	POST /api/v1/signals/prometheus → PrometheusAdapter → CRDCreator → K8s API
var _ = Describe("E2E-FLEET-004: GW Cluster-Aware Signal Ingestion", Label("fleet", "e2e"), func() {
	BeforeEach(func() {
		if os.Getenv("FLEET_E2E") != trueFixture {
			Skip("FLEET_E2E=true required for fleet E2E tests")
		}
	})

	It("should create RR with spec.clusterID when alert has cluster label", func() {

		payload := buildPrometheusAlertWithCluster("HighMemory", "default", "critical",
			"Deployment", "nginx", "prod-east")

		resp, err := http.Post(
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
		Expect(rrName).To(HavePrefix("rr-"),
			"RR name must follow naming convention")
	})

	It("should produce different fingerprints for same resource on different clusters", func() {

		payloadEast := buildPrometheusAlertWithCluster("HighCPU", "default", "warning",
			"Deployment", "nginx", "prod-east")
		payloadWest := buildPrometheusAlertWithCluster("HighCPU", "default", "warning",
			"Deployment", "nginx", "prod-west")

		respEast, err := http.Post(gatewayURL+"/api/v1/signals/prometheus",
			"application/json", strings.NewReader(string(payloadEast)))
		Expect(err).ToNot(HaveOccurred())
		defer respEast.Body.Close()
		Expect(respEast.StatusCode).To(Equal(http.StatusOK))

		respWest, err := http.Post(gatewayURL+"/api/v1/signals/prometheus",
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
			"Same resource on different clusters must create separate RRs")
	})
})

func buildPrometheusAlertWithCluster(alertName, namespace, severity, kind, name, clusterID string) []byte {
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
					"namespace":           namespace,
					"severity":            severity,
					strings.ToLower(kind): name,
				},
				"annotations": map[string]string{
					"description": fmt.Sprintf("Fleet E2E test alert for %s/%s on %s", namespace, name, clusterID),
				},
				"startsAt": time.Now().Format(time.RFC3339),
			},
		},
	}
	data, _ := json.Marshal(payload)
	return data
}
