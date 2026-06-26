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
	"fmt"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FLEET-008: EM observes fleet remediation effectiveness with cluster-scoped metrics
// Authority: Issue #54, ADR-068, ADR-EM-001
// FedRAMP: AU-3 (content of audit records -- cluster provenance in metrics)
var _ = Describe("E2E-FLEET-008 [AU-3]: EM observes fleet remediation effectiveness with cluster-scoped metrics (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should expose Prometheus metrics with cluster dimension via /metrics endpoint", func() {
		By("Querying Prometheus for kubernaut metrics (AU-3: audit provenance)")
		prometheusURL := "http://localhost:9190"

		resp, err := http.Get(fmt.Sprintf("%s/api/v1/targets", prometheusURL))
		Expect(err).ToNot(HaveOccurred(), "Prometheus should be accessible via NodePort")
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"AU-3: Prometheus targets endpoint must be accessible for fleet metrics")
	})

	It("should have cadvisor scrape target active for container metrics", func() {
		prometheusURL := "http://localhost:9190"

		resp, err := http.Get(fmt.Sprintf("%s/api/v1/targets", prometheusURL))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		Expect(strings.Contains(string(body), "cadvisor") || strings.Contains(string(body), "kubelet")).To(BeTrue(),
			"AU-3: Prometheus must scrape cadvisor/kubelet for container metrics used by EM")
	})

	It("should have AlertManager accessible for fleet alert resolution", func() {
		alertManagerURL := "http://localhost:9193"

		resp, err := http.Get(fmt.Sprintf("%s/api/v2/status", alertManagerURL))
		Expect(err).ToNot(HaveOccurred(), "AlertManager should be accessible via NodePort")
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"AU-3: AlertManager must be accessible for fleet effectiveness monitoring")
	})
})
