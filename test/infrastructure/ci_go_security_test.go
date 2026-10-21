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
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/mod/semver"
)

var _ = Describe("CI security remediation [BR-SECURITY-2493]", func() {
	It("UT-SEC-2493-001: root and all active workflow selections require patched Go", func() {
		module := readSecurityFixtureSource("go.mod")
		match := regexp.MustCompile(`(?m)^go (\S+)$`).FindStringSubmatch(module)
		Expect(match).To(HaveLen(2))
		Expect(semver.Compare("v"+match[1], "v1.26.9")).To(BeNumerically(">=", 0))
		workflowCounts := map[string]int{
			"ci-pipeline.yml": 9, "release.yml": 4, "codeql.yml": 1,
			"chart-release.yml": 1, "helm-smoke-test.yml": 1,
		}
		for workflow, expectedCount := range workflowCounts {
			body := readSecurityFixtureSource(filepath.Join(".github", "workflows", workflow))
			versions := regexp.MustCompile(`(?m)^\s*go-version:\s*['"]?([0-9.]+)`).FindAllStringSubmatch(body, -1)
			Expect(versions).To(HaveLen(expectedCount), "workflow: %s", workflow)
			for _, version := range versions {
				Expect(semver.Compare("v"+version[1], "v1.26.9")).To(BeNumerically(">=", 0), "workflow: %s", workflow)
			}
		}
	})

	It("UT-SEC-2493-002: patches selected x/net and the independent compiler without changing certified FIPS", func() {
		module := readSecurityFixtureSource("go.mod")
		match := regexp.MustCompile(`(?m)^\s*golang.org/x/net (v\S+)`).FindStringSubmatch(module)
		Expect(match).To(HaveLen(2))
		Expect(semver.Compare(match[1], "v0.60.0")).To(BeNumerically(">=", 0))
		goose := readSecurityFixtureSource("docker/db-migrate.Dockerfile")
		Expect(goose).To(MatchRegexp(`(?m)^ENV GOTOOLCHAIN=go1\.26\.10\+auto$`))
		frontend := readSecurityFixtureSource("docker/apifrontend.Dockerfile")
		Expect(frontend).To(ContainSubstring("ENV GOFIPS140=certified"))
		Expect(frontend).NotTo(ContainSubstring("GOEXPERIMENT=boringcrypto"))
	})
})

func readSecurityFixtureSource(relativePath string) string {
	body, err := os.ReadFile(filepath.Join(getProjectRoot(), relativePath)) //nolint:gosec // G304: test-owned repository path
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return string(body)
}
