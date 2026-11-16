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

var _ = Describe("operator qualification workflow credentials", func() {
	DescribeTable("UT-INFRA-CI-2643-001 [BR-PLATFORM-008]: preserves the operator-selected toolset in the actual native build preparation step",
		func(builder string, succeeds bool) {
			dir := GinkgoT().TempDir()
			operatorDir := filepath.Join(dir, "kubernaut-operator")
			Expect(os.MkdirAll(operatorDir, 0755)).To(Succeed())
			original := "# operator fixture\nFROM " + builder + " AS builder\nARG TARGETARCH\nRUN go build ./cmd/main.go\nFROM scratch\n"
			source := filepath.Join(operatorDir, "Dockerfile")
			Expect(os.WriteFile(source, []byte(original), 0600)).To(Succeed())
			GinkgoT().Setenv("OPERATOR_FIXTURE_ROOT", dir)
			script := readSecurityRunBlock(".github/workflows/ci-pipeline.yml", "Prepare native cross-compile Dockerfile")
			output, err := runSecurityShell("cd \"$OPERATOR_FIXTURE_ROOT\"\n" + script)
			if !succeeds {
				Expect(err).To(HaveOccurred(), "unexpected builder must fail closed: %s", output)
				return
			}
			Expect(err).NotTo(HaveOccurred(), "step output: %s", output)
			prepared, err := os.ReadFile(filepath.Join(operatorDir, "Dockerfile.ci"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(prepared)).To(Equal(strings.Replace(original, "FROM "+builder+" AS builder", "FROM --platform=$BUILDPLATFORM "+builder+" AS builder", 1)))
			unchanged, err := os.ReadFile(source)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(unchanged)).To(Equal(original), "the upstream Dockerfile must not be modified")
		},
		Entry("previous 10.2 operator", "registry.access.redhat.com/ubi10/go-toolset:10.2", true),
		Entry("current 10.3 operator", "registry.access.redhat.com/ubi10/go-toolset:10.3", true),
		Entry("digest-pinned operator", "registry.access.redhat.com/ubi10/go-toolset:10.3@sha256:"+strings.Repeat("a", 64), true),
		Entry("unexpected builder image", "example.com/unexpected:10.3", false),
	)

	It("UT-INFRA-CI-2390-001 [BR-PLATFORM-008]: uses operator qualification without application-chart jobs", func() {
		workflowPath := filepath.Join(getProjectRoot(), ".github", "workflows", "ci-pipeline.yml")
		workflow, err := os.ReadFile(workflowPath) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())
		body := string(workflow)
		Expect(body).To(ContainSubstring("build-operator:"))
		Expect(body).To(ContainSubstring("KUBERNAUT_OPERATOR_IMAGE"))
		Expect(body).To(ContainSubstring("KUBERNAUT_OPERATOR_CHART_PATH"))
		Expect(body).To(ContainSubstring("KUBERNAUT_DEPENDENCY_CHART_PATH"))
		Expect(body).NotTo(ContainSubstring("deprecated-chart-validation:"))
		Expect(body).NotTo(ContainSubstring("helm-unittest"))
		Expect(body).NotTo(ContainSubstring("Create ArgoCD Application"))
		Expect(body).NotTo(ContainSubstring("--from-literal=OPENAI_API_KEY="))
	})
})
