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

package mcp_test

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Helm RBAC — PR4 H1 BR-INTERACTIVE-001", func() {

	var helmTemplate string

	BeforeEach(func() {
		data, err := os.ReadFile("../../../../charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml")
		Expect(err).NotTo(HaveOccurred())
		helmTemplate = string(data)
	})

	Describe("UT-KA-HELM-001: coordination.k8s.io/leases RBAC present in ClusterRole", func() {
		It("should include Lease RBAC rules for interactive sessions", func() {
			Expect(helmTemplate).To(ContainSubstring("coordination.k8s.io"))
			Expect(helmTemplate).To(ContainSubstring("leases"))
		})
	})

	Describe("UT-KA-HELM-002: Leases RBAC is feature-gated on interactive.enabled", func() {
		It("should conditionally include Lease RBAC based on interactive.enabled value", func() {
			Expect(helmTemplate).To(ContainSubstring("interactive"))
			lines := strings.Split(helmTemplate, "\n")
			foundCoordination := false
			for i, line := range lines {
				if strings.Contains(line, "coordination.k8s.io") {
					foundCoordination = true
					// Verify there's an if-guard within 5 lines above
					contextStart := i - 5
					if contextStart < 0 {
						contextStart = 0
					}
					context := strings.Join(lines[contextStart:i], "\n")
					Expect(context).To(ContainSubstring("if"))
					break
				}
			}
			Expect(foundCoordination).To(BeTrue(), "coordination.k8s.io not found in template")
		})
	})
})
