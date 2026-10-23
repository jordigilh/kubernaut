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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("operator qualification workflow credentials", func() {
	It("UT-INFRA-CI-2390-001 [BR-PLATFORM-008]: keeps legacy chart smoke offline and uses operator qualification", func() {
		workflowPath := filepath.Join(getProjectRoot(), ".github", "workflows", "ci-pipeline.yml")
		workflow, err := os.ReadFile(workflowPath) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())
		body := string(workflow)
		Expect(body).To(ContainSubstring("build-operator:"))
		Expect(body).To(ContainSubstring("KUBERNAUT_OPERATOR_IMAGE"))
		Expect(body).To(ContainSubstring("deprecated-chart-validation:"))
		Expect(body).NotTo(ContainSubstring("Create ArgoCD Application"))
		Expect(body).NotTo(ContainSubstring("--from-literal=OPENAI_API_KEY="))
	})
})
