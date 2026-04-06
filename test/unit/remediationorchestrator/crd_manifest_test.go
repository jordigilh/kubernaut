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

// Issue #644 supersedes #635: Revised column layout following pipeline flow.
var _ = Describe("Issue #644: CRD Manifest Printer Columns (DD-CRD-003)", func() {
	It("UT-RR-644-CRD-001: should define default columns: Phase, Outcome, Alert, RCA Target, Workflow, Confidence, Age", func() {
		crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_remediationrequests.yaml")
		data, err := os.ReadFile(crdPath)
		Expect(err).NotTo(HaveOccurred(), "CRD manifest should exist at config/crd/bases/")

		crdYAML := string(data)

		Expect(crdYAML).To(ContainSubstring("name: Phase"))
		Expect(crdYAML).To(ContainSubstring("name: Outcome"))
		Expect(crdYAML).To(ContainSubstring("name: Alert"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.signalName"))
		Expect(crdYAML).To(ContainSubstring("name: RCA Target"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.targetDisplay"))
		Expect(crdYAML).To(ContainSubstring("name: Workflow"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.workflowDisplayName"))
		Expect(crdYAML).To(ContainSubstring("name: Confidence"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.confidence"))
		Expect(crdYAML).To(ContainSubstring("name: Age"))
	})

	It("UT-RR-644-CRD-002: should define wide columns: Source, Signal NS, Signal Target, RCA NS (no Reason)", func() {
		crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_remediationrequests.yaml")
		data, err := os.ReadFile(crdPath)
		Expect(err).NotTo(HaveOccurred())
		crdYAML := string(data)

		Expect(crdYAML).To(ContainSubstring("name: Source"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.signalSource"))
		Expect(crdYAML).To(ContainSubstring("name: Signal NS"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.targetResource.namespace"))
		Expect(crdYAML).To(ContainSubstring("name: Signal Target"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.signalTargetDisplay"))
		Expect(crdYAML).To(ContainSubstring("name: RCA NS"))
		Expect(crdYAML).To(ContainSubstring("jsonPath: .status.remediationTarget.namespace"))
		Expect(crdYAML).NotTo(ContainSubstring("name: Reason"))
	})
})
