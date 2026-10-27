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
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("legacy deploy manifest ownership [BR-PLATFORM-014, BR-PLATFORM-005]", func() {
	It("UT-INFRA-DEPLOY-2551-001: keeps active fixtures under test ownership", func() {
		root := getProjectRoot()
		required := []string{
			"test/fixtures/kubernetes/apifrontend/base/02-rbac.yaml",
			"test/fixtures/kubernetes/apifrontend/base/05-prometheusrule.yaml",
			"test/fixtures/kubernetes/apifrontend/base/06-networkpolicy.yaml",
			"test/fixtures/kubernetes/apifrontend/base/08-servicemonitor.yaml",
			"test/fixtures/kubernetes/apifrontend/base/config.yaml",
			"test/fixtures/kubernetes/apifrontend/e2e/config.yaml",
			"test/fixtures/kubernetes/apifrontend/e2e/dex.yaml",
			"test/fixtures/kubernetes/apifrontend/e2e/e2e-user-rbac.yaml",
			"test/fixtures/kubernetes/apifrontend/e2e/generate-certs.sh",
			"test/fixtures/kubernetes/apifrontend/e2e/mock-llm.yaml",
			"test/fixtures/kubernetes/integration/postgresql/deployment.yaml",
			"test/fixtures/kubernetes/integration/redis/deployment.yaml",
			"test/fixtures/kubernetes/integration/monitoring/prometheus/deployment.yaml",
			"test/fixtures/kubernetes/integration/monitoring/alertmanager/deployment.yaml",
			"test/fixtures/kubernetes/datastorage/kustomization.yaml",
			"test/fixtures/kubernetes/datastorage/client-rbac-v2.yaml",
			"test/fixtures/kubernetes/datastorage/service-rbac.yaml",
			"test/fixtures/kubernetes/cert-manager/selfsigned-issuer.yaml",
			"test/fixtures/kubernetes/mock-llm/kustomization.yaml",
			"hack/setup-demo-infra/fixtures/mcp-gateway/kustomization.yaml",
			"config/monitoring/audit-prometheus-rules.yaml",
			"config/monitoring/gateway-alerts.yaml",
			"config/monitoring/gateway-dashboard.json",
		}
		for _, relativePath := range required {
			_, err := os.Stat(filepath.Join(root, relativePath))
			Expect(err).NotTo(HaveOccurred(), "required test fixture %s must exist", relativePath)
		}

		_, err := os.Stat(filepath.Join(root, "deploy"))
		Expect(err).To(HaveOccurred(), "application deploy root must not remain a supported surface")
	})

	It("UT-INFRA-DEPLOY-2551-002: active callers do not reference legacy deploy paths", func() {
		root := getProjectRoot()
		activeFiles := []string{
			"scripts/bootstrap-external-deps.sh",
			"test/infrastructure/apifrontend_dex_test.go",
			"test/infrastructure/apifrontend_e2e.go",
			"test/infrastructure/apifrontend_fleet_test.go",
			"test/infrastructure/apifrontend_prometheus_e2e.go",
			"test/infrastructure/rbac_parity_test.go",
			"test/infrastructure/datastorage.go",
			"test/services/mock-llm/apifrontend_structured_decision_selector_test.go",
		}
		for _, relativePath := range activeFiles {
			data, err := os.ReadFile(filepath.Join(root, relativePath)) //nolint:gosec // G304: fixed repository paths
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("deploy/"), "%s must use test-owned paths", relativePath)
		}
	})

	It("IT-INFRA-DEPLOY-2551-001: no standalone application deployment script remains", func() {
		root := getProjectRoot()
		for _, relativePath := range []string{
			"scripts/build-and-deploy.sh",
			"deploy/kustomization.yaml",
			"deploy/integration/kustomization.yaml",
			"deploy/integration/kubernaut/kustomization.yaml",
		} {
			_, err := os.Stat(filepath.Join(root, relativePath))
			Expect(err).To(HaveOccurred(), "%s is an unsupported standalone application path", relativePath)
		}
	})

	It("IT-INFRA-DEPLOY-2551-002: retained fixture content preserves security markers", func() {
		root := getProjectRoot()
		for _, relativePath := range []string{
			"test/fixtures/kubernetes/apifrontend/base/02-rbac.yaml",
			"test/fixtures/kubernetes/apifrontend/e2e/e2e-user-rbac.yaml",
			"test/fixtures/kubernetes/apifrontend/e2e/config.yaml",
		} {
			data, err := os.ReadFile(filepath.Join(root, relativePath)) //nolint:gosec // G304: fixed repository paths
			Expect(err).NotTo(HaveOccurred())
			text := string(data)
			Expect(strings.TrimSpace(text)).NotTo(BeEmpty(), "%s must remain non-empty", relativePath)
			switch relativePath {
			case "test/fixtures/kubernetes/apifrontend/base/02-rbac.yaml":
				Expect(text).To(ContainSubstring("subjectaccessreviews"), "AC-6 SAR authorization must remain present")
				Expect(text).To(ContainSubstring("tokenreviews"), "authentication token review permission must remain present")
			case "test/fixtures/kubernetes/apifrontend/e2e/e2e-user-rbac.yaml":
				Expect(text).To(ContainSubstring("kubernaut_"), "E2E persona tool permissions must remain present")
			case "test/fixtures/kubernetes/apifrontend/e2e/config.yaml":
				Expect(text).To(ContainSubstring("oidcCaFile"), "TLS trust configuration must remain present")
			}
		}
	})
})
