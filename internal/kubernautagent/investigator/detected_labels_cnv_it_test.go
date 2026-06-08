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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("CNV DetectedLabels Investigator Integration — #1378", Label("it", "ka", "cnv", "1378"), func() {

	var builder *prompt.Builder

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("IT-KA-1378-004: Investigator dispatch with CNV-enriched DetectedLabels", func() {
		It("should propagate all 4 CNV fields into prompt map and result map [BR-WORKFLOW-018]", func() {
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind: "VirtualMachine", Name: "test-vm", Namespace: "cnv-ns",
				},
			}
			signal := katypes.SignalContext{
				ResourceKind: "VirtualMachine",
				ResourceName: "test-vm",
				Namespace:    "cnv-ns",
			}
			enrichData := &enrichment.EnrichmentResult{
				DetectedLabels: &sharedtypes.DetectedLabels{
					VirtualMachine: true,
					LiveMigratable: true,
					CDIManaged:     true,
					StorageBackend: "odf-ceph",
				},
				ResourceKind:      "VirtualMachine",
				ResourceName:      "test-vm",
				ResourceNamespace: "cnv-ns",
			}

			investigator.FinalizeWorkflowResult(result, signal, nil, enrichData)

			Expect(result.DetectedLabels).NotTo(BeNil())
			Expect(result.DetectedLabels["virtualMachine"]).To(Equal(true))
			Expect(result.DetectedLabels["liveMigratable"]).To(Equal(true))
			Expect(result.DetectedLabels["cdiManaged"]).To(Equal(true))
			Expect(result.DetectedLabels["storageBackend"]).To(Equal("odf-ceph"))

			// Prompt map mirrors detectedLabelsToPromptMap output for the same CNV enrichment.
			promptMap := map[string]string{
				"virtualMachine": "true",
				"liveMigratable": "true",
				"cdiManaged":     "true",
				"storageBackend": "odf-ceph",
			}
			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal: prompt.SignalData{
					Name:         "test-vm",
					Namespace:    "cnv-ns",
					Severity:     "critical",
					Message:      "VM migration failed",
					ResourceKind: "VirtualMachine",
					ResourceName: "test-vm",
				},
				RCASummary: "Live migration blocked by storage backend",
				EnrichData: &prompt.EnrichmentData{DetectedLabels: promptMap},
			})
			Expect(err).NotTo(HaveOccurred())
			for key := range promptMap {
				Expect(rendered).To(ContainSubstring(key),
					"DetectedLabels key %q must appear in workflow selection prompt", key)
			}
		})
	})
})
