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

package infrastructure

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Monitoring TLS cache validation", func() {
	It("UT-INFRA-TLS-003 [BR-E2E-001]: rejects a host CA that differs from the Secret", func() {
		caPEM := []byte("-----BEGIN CERTIFICATE-----\ncurrent-ca\n-----END CERTIFICATE-----\n")
		encoded := base64.StdEncoding.EncodeToString(caPEM)

		Expect(monitoringCABytesMatch(caPEM, encoded)).To(BeTrue())
		Expect(monitoringCABytesMatch([]byte("stale-ca"), encoded)).To(BeFalse())
		Expect(monitoringCABytesMatch(caPEM, "not-base64")).To(BeFalse())
	})

	It("UT-INFRA-EM-TLS-001 [BR-EM-001]: deploys TLS monitoring endpoints for the HTTPS-aware harness", func() {
		source, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "effectivenessmonitor_e2e.go"))
		Expect(err).NotTo(HaveOccurred())
		text := string(source)
		Expect(text).To(ContainSubstring("EnsureMonitoringTLS(ctx, kubeconfigPath, namespace, namespace, writer)"))
		Expect(text).To(ContainSubstring("DeployPrometheusTLS(ctx, namespace, kubeconfigPath, writer)"))
		Expect(text).To(ContainSubstring("DeployAlertManagerTLS(ctx, namespace, namespace, kubeconfigPath, \"\", writer)"))
	})

	It("UT-INFRA-EM-TLS-002 [BR-EM-001, BR-PLATFORM-014]: gives EM HTTPS endpoints and the monitoring CA bundle", func() {
		source, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "effectivenessmonitor_e2e.go"))
		Expect(err).NotTo(HaveOccurred())
		text := string(source)
		Expect(text).To(ContainSubstring("prometheusUrl: https://prometheus-svc:9090"))
		Expect(text).To(ContainSubstring("alertManagerUrl: https://alertmanager-svc:9093"))
		Expect(text).To(ContainSubstring("tlsCaFile: /etc/monitoring-ca/ca.crt"))
		Expect(text).To(ContainSubstring("mountPath: /etc/monitoring-ca"))
		Expect(text).To(ContainSubstring("name: monitoring-ca"))
	})

	It("UT-INFRA-EM-TLS-003 [BR-EM-001, BR-PLATFORM-014]: renders the monitoring CA volume in the EM deployment", func() {
		manifest := captureKubectlManifest(func() error {
			return DeployEMController(context.Background(), "kubernaut-system", "test-kubeconfig", "localhost/effectivenessmonitor:test", io.Discard)
		})

		Expect(manifest).To(ContainSubstring("prometheusUrl: https://prometheus-svc:9090"))
		Expect(manifest).To(ContainSubstring("alertManagerUrl: https://alertmanager-svc:9093"))
		Expect(manifest).To(ContainSubstring("tlsCaFile: /etc/monitoring-ca/ca.crt"))
		Expect(manifest).To(ContainSubstring("mountPath: /etc/monitoring-ca"))
		Expect(manifest).To(ContainSubstring("name: monitoring-ca"))
	})
})
