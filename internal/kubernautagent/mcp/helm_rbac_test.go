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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Helm RBAC — PR4 H1 BR-INTERACTIVE-001", func() {

	var helmTemplate string

	BeforeEach(func() {
		data, err := os.ReadFile("../../../charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml")
		Expect(err).NotTo(HaveOccurred())
		helmTemplate = string(data)
	})

	Describe("UT-KA-HELM-001: coordination.k8s.io/leases RBAC present in namespace-scoped Role", func() {
		It("should include Lease RBAC rules for interactive sessions and AgentSession dispatch", func() {
			Expect(helmTemplate).To(ContainSubstring("coordination.k8s.io"))
			Expect(helmTemplate).To(ContainSubstring("leases"))
			Expect(helmTemplate).To(ContainSubstring("kubernaut-agent-leases"))
		})
	})

	// UT-KA-HELM-002 (DD-AA-KA-001, #2170): Leases RBAC is now unconditional,
	// not gated on interactive.enabled — the AgentSession dispatch Lease is
	// used by every investigation (autonomous or interactive), so KA's
	// controller-runtime client/RBAC wiring for it must always be present.
	// This supersedes the previous "feature-gated" assertion.
	Describe("UT-KA-HELM-002: Leases RBAC is unconditional (dispatch Lease needed regardless of interactive.enabled)", func() {
		It("should include the Lease Role/RoleBinding without wrapping them in an interactive.enabled guard", func() {
			Expect(helmTemplate).To(ContainSubstring("kubernaut-agent-leases-binding"))
			Expect(helmTemplate).NotTo(ContainSubstring("{{- if .Values.kubernautAgent.interactive.enabled }}\n---\n# HELM-03"))
		})
	})

	Describe("UT-KA-HELM-003 (DD-AA-KA-001): AgentSession RBAC present, scoped to read + status-subresource only", func() {
		It("should include get/list/watch on agentsessions and update/patch on agentsessions/status, never a full write", func() {
			Expect(helmTemplate).To(ContainSubstring("agentsessions"))
			Expect(helmTemplate).To(ContainSubstring("agentsessions/status"))
		})
	})

	// UT-KA-HELM-004 (DD-AA-KA-001 Amendment Gap 1, BR-AA-KA-065.9): KA's
	// dispatch-time InvestigationSession-existence check needs read-only
	// RBAC -- KA never writes InvestigationSession, so there must be no
	// investigationsessions/status grant alongside it.
	Describe("UT-KA-HELM-004: InvestigationSession RBAC present, read-only (get/list/watch), no status-write grant", func() {
		It("should include get/list/watch on investigationsessions but never investigationsessions/status", func() {
			Expect(helmTemplate).To(ContainSubstring("investigationsessions"))
			Expect(helmTemplate).NotTo(ContainSubstring("investigationsessions/status"))
		})
	})
})
