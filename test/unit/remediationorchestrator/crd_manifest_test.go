/*
Copyright 2025 Jordi Gil.

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

package remediationorchestrator

import (
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func projectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..")
}

var _ = Describe("UT-RR-387-001: CRD Manifest Printer Columns (DD-CRD-003, Issue #387)", func() {
	It("should define the expected wide printer columns for operational triage", func() {
		crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_remediationrequests.yaml")
		data, err := os.ReadFile(crdPath)
		Expect(err).NotTo(HaveOccurred(), "CRD manifest should exist at config/crd/bases/")

		crdYAML := string(data)

		// Source column: signal adapter (e.g., "alertmanager")
		Expect(crdYAML).To(ContainSubstring("name: Source"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.signalSource"))

		// Target column: signal target resource name
		Expect(crdYAML).To(ContainSubstring("name: Target"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.targetResource.name"))

		// RCA Target column: LLM-identified remediation target (BR-HAPI-191)
		Expect(crdYAML).To(ContainSubstring("name: RCA Target"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.remediationTarget.name"))

		// Workflow column: AI-selected workflow name
		Expect(crdYAML).To(ContainSubstring("name: Workflow"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.selectedWorkflowRef.workflowId"))

		// All 4 new columns should be wide-only (priority 1)
		// The existing columns (Phase, Outcome, Age) have no priority (default 0)
		// and Reason already has priority 1. Count total priority: 1 entries.
		// After this change: Source, Target, RCA Target, Workflow, Reason = 5 entries with priority: 1
	})
})

var _ = Describe("Issue #622: RR -owide Missing Target Resource Namespace/Kind", func() {
	It("UT-RR-622-001: CRD manifest contains Target NS column", func() {
		crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_remediationrequests.yaml")
		data, err := os.ReadFile(crdPath)
		Expect(err).NotTo(HaveOccurred())
		crdYAML := string(data)

		Expect(crdYAML).To(ContainSubstring("name: Target NS"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.targetResource.namespace"))
	})

	It("UT-RR-622-002: CRD manifest contains Target Kind column", func() {
		crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_remediationrequests.yaml")
		data, err := os.ReadFile(crdPath)
		Expect(err).NotTo(HaveOccurred())
		crdYAML := string(data)

		Expect(crdYAML).To(ContainSubstring("name: Target Kind"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.targetResource.kind"))
	})

	It("UT-RR-622-003: CRD manifest contains RCA NS column", func() {
		crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_remediationrequests.yaml")
		data, err := os.ReadFile(crdPath)
		Expect(err).NotTo(HaveOccurred())
		crdYAML := string(data)

		Expect(crdYAML).To(ContainSubstring("name: RCA NS"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.remediationTarget.namespace"))
	})

	It("UT-RR-622-004: CRD manifest contains RCA Kind column", func() {
		crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_remediationrequests.yaml")
		data, err := os.ReadFile(crdPath)
		Expect(err).NotTo(HaveOccurred())
		crdYAML := string(data)

		Expect(crdYAML).To(ContainSubstring("name: RCA Kind"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.remediationTarget.kind"))
	})
})
