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
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FLEET-SI10-001: Fleet input validation (malformed payloads)
// Authority: BR-GATEWAY-003, BR-GATEWAY-043, Issue #54
// FedRAMP: SI-10 (Information input validation)
// SOC2: CC7.2 (Internal controls -- input validation)
//
// Validates that the Gateway rejects malformed payloads in a fleet
// context, returning appropriate HTTP error codes without creating
// RemediationRequests from invalid data.
var _ = Describe("E2E-FLEET-SI10-001 [SI-10]: Fleet input validation rejects malformed payloads (BR-GATEWAY-003)", Label("fleet"), func() {

	gatewayURL := "http://localhost:30080"

	It("should return 400 for syntactically invalid JSON [SI-10]", func() {
		malformed := `{"alerts": [{"status": "firing", INVALID}]}`

		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			"application/json",
			strings.NewReader(malformed))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"SI-10: syntactically invalid JSON must be rejected with 400")
	})

	It("should return 400 for empty alerts array [SI-10]", func() {
		emptyAlerts := `{"version": "4", "status": "firing", "alerts": []}`

		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			"application/json",
			strings.NewReader(emptyAlerts))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"SI-10: empty alerts array must be rejected -- no signals to process")
	})

	It("should return 400 for alert missing required labels [SI-10]", func() {
		missingLabels := `{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"status": "firing",
				"labels": {},
				"annotations": {},
				"startsAt": "2026-06-29T12:00:00Z"
			}]
		}`

		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			"application/json",
			strings.NewReader(missingLabels))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"SI-10: alert without required labels (alertname) must be rejected")
	})

	It("should return 400 for empty request body [SI-10]", func() {
		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			"application/json",
			strings.NewReader(""))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"SI-10: empty body must be rejected")
	})

	It("should accept well-formed fleet alert with cluster_id and return success [SI-10, AC-4]", func() {
		payload := buildPrometheusAlertWithCluster("FleetSI10Valid", namespace, "warning",
			"Deployment", "validation-test-app", "prod-west")

		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			"application/json",
			strings.NewReader(string(payload)))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		Expect(resp.StatusCode).To(SatisfyAny(
			Equal(http.StatusCreated),
			Equal(http.StatusAccepted),
		), "SI-10: well-formed fleet alert must be accepted (got body: %s)", string(body))
	})
})
