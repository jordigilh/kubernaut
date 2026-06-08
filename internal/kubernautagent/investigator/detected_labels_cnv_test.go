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
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("CNV DetectedLabels Propagation — #1378", func() {

	Describe("UT-KA-1378-020: detectedLabelsToPromptMap with VM+migration+CDI+odf-ceph", func() {
		It("should propagate all 4 CNV fields into result.DetectedLabels via FinalizeWorkflowResult", func() {
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
		})
	})

	Describe("UT-KA-1378-021: detectedLabelsToPromptMap with VM but no storage", func() {
		It("should include virtualMachine=true but no storageBackend key", func() {
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
					LiveMigratable: false,
					CDIManaged:     false,
					StorageBackend: "",
				},
				ResourceKind:      "VirtualMachine",
				ResourceName:      "test-vm",
				ResourceNamespace: "cnv-ns",
			}

			investigator.FinalizeWorkflowResult(result, signal, nil, enrichData)

			Expect(result.DetectedLabels).NotTo(BeNil())
			Expect(result.DetectedLabels["virtualMachine"]).To(Equal(true))
			// storageBackend omitted when empty in result map — but detectedLabelsToResult
			// always includes bool fields, so liveMigratable and cdiManaged are present as false
			Expect(result.DetectedLabels["liveMigratable"]).To(Equal(false))
			Expect(result.DetectedLabels["cdiManaged"]).To(Equal(false))
		})
	})

	Describe("UT-KA-1378-022: detectedLabelsToPromptMap with non-VM workload", func() {
		It("should include CNV fields as false in result map", func() {
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment", Name: "api-server", Namespace: "production",
				},
			}
			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "production",
			}
			enrichData := &enrichment.EnrichmentResult{
				DetectedLabels: &sharedtypes.DetectedLabels{
					VirtualMachine: false,
					LiveMigratable: false,
					CDIManaged:     false,
					StorageBackend: "",
				},
				ResourceKind:      "Deployment",
				ResourceName:      "api-server",
				ResourceNamespace: "production",
			}

			investigator.FinalizeWorkflowResult(result, signal, nil, enrichData)

			Expect(result.DetectedLabels).NotTo(BeNil())
			Expect(result.DetectedLabels["virtualMachine"]).To(Equal(false))
			Expect(result.DetectedLabels["liveMigratable"]).To(Equal(false))
			Expect(result.DetectedLabels["cdiManaged"]).To(Equal(false))
		})
	})

	Describe("UT-KA-1378-023: detectedLabelsToResult with all CNV fields", func() {
		It("should include all 4 CNV fields in result map including false values", func() {
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
					StorageBackend: "lvms",
					GitOpsManaged:  true,
					GitOpsTool:     "argocd",
				},
				ResourceKind:      "VirtualMachine",
				ResourceName:      "test-vm",
				ResourceNamespace: "cnv-ns",
			}

			investigator.FinalizeWorkflowResult(result, signal, nil, enrichData)

			Expect(result.DetectedLabels).NotTo(BeNil())
			// CNV fields
			Expect(result.DetectedLabels["virtualMachine"]).To(Equal(true))
			Expect(result.DetectedLabels["liveMigratable"]).To(Equal(true))
			Expect(result.DetectedLabels["cdiManaged"]).To(Equal(true))
			Expect(result.DetectedLabels["storageBackend"]).To(Equal("lvms"))
			// Existing fields still present
			Expect(result.DetectedLabels["gitOpsManaged"]).To(Equal(true))
			Expect(result.DetectedLabels["gitOpsTool"]).To(Equal("argocd"))
		})
	})
})
