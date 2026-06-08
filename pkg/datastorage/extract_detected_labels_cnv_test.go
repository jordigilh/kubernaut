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

package datastorage_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
)

var _ = Describe("ExtractDetectedLabels CNV — #1378", func() {
	var p *schema.Parser

	BeforeEach(func() {
		p = schema.NewParser()
	})

	Describe("UT-DS-1378-026: ExtractDetectedLabels converts 'true' to bool for CNV booleans", func() {
		It("should convert string 'true' to bool true for virtualMachine, liveMigratable, cdiManaged", func() {
			ws := &models.WorkflowSchema{
				DetectedLabels: &models.DetectedLabelsSchema{
					VirtualMachine: "true",
					LiveMigratable: "true",
					CDIManaged:     "true",
				},
			}
			dl, err := p.ExtractDetectedLabels(ws)
			Expect(err).NotTo(HaveOccurred())
			Expect(dl.VirtualMachine).To(BeTrue(), "virtualMachine 'true' should convert to bool true")
			Expect(dl.LiveMigratable).To(BeTrue(), "liveMigratable 'true' should convert to bool true")
			Expect(dl.CDIManaged).To(BeTrue(), "cdiManaged 'true' should convert to bool true")
		})
	})

	Describe("UT-DS-1378-027: ExtractDetectedLabels preserves storageBackend string", func() {
		It("should preserve storageBackend string value through extraction", func() {
			ws := &models.WorkflowSchema{
				DetectedLabels: &models.DetectedLabelsSchema{
					StorageBackend: "odf-ceph",
				},
			}
			dl, err := p.ExtractDetectedLabels(ws)
			Expect(err).NotTo(HaveOccurred())
			Expect(dl.StorageBackend).To(Equal("odf-ceph"), "storageBackend should preserve string value")
		})
	})
})
