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

package models

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// ========================================
// CNV DetectedLabels Schema/Serialization Tests — #1378
// ========================================

var _ = Describe("CNV DetectedLabels Schema/Serialization (#1378)", func() {

	Describe("UT-DS-1378-020: DetectedLabelsSchema validates virtualMachine=true", func() {
		It("accepts virtualMachine=\"true\"", func() {
			yamlData := "virtualMachine: \"true\""
			var schema DetectedLabelsSchema
			err := yaml.Unmarshal([]byte(yamlData), &schema)
			Expect(err).ToNot(HaveOccurred(), "CNV fields not yet implemented")
			Expect(schema.ValidateDetectedLabels()).To(Succeed())
		})
	})

	Describe("UT-DS-1378-021: DetectedLabelsSchema validates storageBackend=odf-ceph", func() {
		It("accepts storageBackend=odf-ceph", func() {
			yamlData := "storageBackend: odf-ceph"
			var schema DetectedLabelsSchema
			err := yaml.Unmarshal([]byte(yamlData), &schema)
			Expect(err).ToNot(HaveOccurred(), "CNV fields not yet implemented")
			Expect(schema.ValidateDetectedLabels()).To(Succeed())
		})
	})

	Describe("UT-DS-1378-022: schema rejects storageBackend=invalid", func() {
		It("rejects invalid storageBackend", func() {
			yamlData := "storageBackend: invalid"
			var schema DetectedLabelsSchema
			err := yaml.Unmarshal([]byte(yamlData), &schema)
			Expect(err).ToNot(HaveOccurred(), "CNV fields not yet implemented")
			Expect(schema.ValidateDetectedLabels()).ToNot(Succeed())
		})
	})

	Describe("UT-DS-1378-023: schema rejects virtualMachine=false", func() {
		It("rejects virtualMachine=\"false\" (booleans only accept \"true\")", func() {
			yamlData := "virtualMachine: \"false\""
			var schema DetectedLabelsSchema
			err := yaml.Unmarshal([]byte(yamlData), &schema)
			Expect(err).ToNot(HaveOccurred(), "CNV fields not yet implemented")
			Expect(schema.ValidateDetectedLabels()).ToNot(Succeed())
		})
	})

	Describe("UT-DS-1378-024: SerializeLabels includes CNV fields when set", func() {
		It("includes all CNV fields in JSON output", func() {
			dl := DetectedLabels{
				VirtualMachine: true,
				LiveMigratable: true,
				CDIManaged:     true,
				StorageBackend: "odf-ceph",
			}
			data, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())
			str := string(data)
			for _, key := range []string{"virtualMachine", "liveMigratable", "cdiManaged", "storageBackend"} {
				Expect(str).To(ContainSubstring(key), "expected JSON to contain %q", key)
			}
		})
	})

	Describe("UT-DS-1378-025: SerializeLabels omits CNV fields when false/empty", func() {
		It("omits zero-valued CNV fields from JSON output", func() {
			dl := DetectedLabels{
				VirtualMachine: false,
				LiveMigratable: false,
				CDIManaged:     false,
				StorageBackend: "",
			}
			data, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())
			str := string(data)
			for _, key := range []string{"virtualMachine", "liveMigratable", "cdiManaged", "storageBackend"} {
				Expect(str).NotTo(ContainSubstring(key), "JSON should not contain %q when zero", key)
			}
		})
	})

	Describe("IsEmpty with CNV fields", func() {
		It("returns false when VirtualMachine=true", func() {
			dl := &DetectedLabels{VirtualMachine: true}
			Expect(dl.IsEmpty()).To(BeFalse())
		})

		It("returns false when StorageBackend=odf-ceph", func() {
			dl := &DetectedLabels{StorageBackend: "odf-ceph"}
			Expect(dl.IsEmpty()).To(BeFalse())
		})
	})

	Describe("ValidDetectedLabelFields contains CNV fields", func() {
		It("includes all CNV field names", func() {
			expected := []string{"virtualMachine", "liveMigratable", "cdiManaged", "storageBackend"}
			fieldSet := make(map[string]bool, len(ValidDetectedLabelFields))
			for _, f := range ValidDetectedLabelFields {
				fieldSet[f] = true
			}
			for _, f := range expected {
				Expect(fieldSet).To(HaveKey(f), "ValidDetectedLabelFields missing CNV field %q", f)
			}
		})
	})
})
