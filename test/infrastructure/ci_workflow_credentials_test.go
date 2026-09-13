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

var _ = Describe("GitOps smoke LLM credentials", func() {
	It("UT-INFRA-CI-2390-001 [BR-PLATFORM-008]: provisions the chart's mounted credential key", func() {
		workflowPath := filepath.Join(getProjectRoot(), ".github", "workflows", "ci-pipeline.yml")
		workflow, err := os.ReadFile(workflowPath) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())

		const (
			startMarker = "Provision GitOps prerequisite namespace and secrets"
			endMarker   = "Create ArgoCD Application"
		)
		start := strings.Index(string(workflow), startMarker)
		Expect(start).To(BeNumerically(">=", 0))
		if start < 0 {
			return
		}
		endOffset := strings.Index(string(workflow[start:]), endMarker)
		Expect(endOffset).To(BeNumerically(">=", 0))
		if endOffset < 0 {
			return
		}

		provisioningStep := string(workflow[start : start+endOffset])
		Expect(provisioningStep).To(ContainSubstring("--from-literal=api_key=sk-gitops-smoke-placeholder"))
		Expect(provisioningStep).NotTo(ContainSubstring("--from-literal=OPENAI_API_KEY="))
	})
})
