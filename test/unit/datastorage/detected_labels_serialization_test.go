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

package datastorage

import (
	"database/sql/driver"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-AI-056 / #1052: DetectedLabels domain-specific serialization.
// CRD domain requires full JSON (all boolean fields present for schema validation).
// DB domain requires sparse JSON (false booleans omitted for JSONB efficiency).

// Compile-time interface assertions: both types implement SerializeLabels() ([]byte, error).
type detectedLabelsSerializer interface {
	SerializeLabels() ([]byte, error)
}

var _ detectedLabelsSerializer = (*sharedtypes.DetectedLabels)(nil)
var _ detectedLabelsSerializer = (sharedtypes.DBDetectedLabels{})

// Compile-time: DBDetectedLabels satisfies driver.Valuer via value receiver.
var _ driver.Valuer = sharedtypes.DBDetectedLabels{}
var _ driver.Valuer = (*sharedtypes.DetectedLabels)(nil)

// Compile-time: DBDetectedLabels satisfies json.Marshaler via value receiver.
var _ json.Marshaler = sharedtypes.DBDetectedLabels{}

var _ = Describe("DetectedLabels domain-specific serialization (BR-AI-056, #1052)", func() {

	Describe("CRD full-JSON contract (DetectedLabels)", func() {
		It("UT-DL-SERIAL-001: SerializeLabels includes all 7 boolean fields even when false", func() {
			dl := &sharedtypes.DetectedLabels{}
			raw, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())

			var m map[string]interface{}
			Expect(json.Unmarshal(raw, &m)).To(Succeed())

			for _, field := range []string{
				"gitOpsManaged", "pdbProtected", "hpaEnabled",
				"stateful", "helmManaged", "networkIsolated",
				"resourceQuotaConstrained",
			} {
				Expect(m).To(HaveKey(field), "CRD JSON must include required field: "+field)
				Expect(m[field]).To(BeFalse(), field+" should be false when zero-valued")
			}
		})

		It("UT-DL-SERIAL-002: SerializeLabels preserves true booleans and string fields", func() {
			dl := &sharedtypes.DetectedLabels{
				GitOpsManaged:            true,
				GitOpsTool:               "argocd",
				PDBProtected:             true,
				HPAEnabled:               false,
				Stateful:                 true,
				HelmManaged:              false,
				NetworkIsolated:          true,
				ServiceMesh:              "istio",
				ResourceQuotaConstrained: true,
				FailedDetections:         []string{"hpaEnabled"},
			}
			raw, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())

			var m map[string]interface{}
			Expect(json.Unmarshal(raw, &m)).To(Succeed())
			Expect(m["gitOpsManaged"]).To(BeTrue())
			Expect(m["gitOpsTool"]).To(Equal("argocd"))
			Expect(m["hpaEnabled"]).To(BeFalse())
			Expect(m["serviceMesh"]).To(Equal("istio"))
			Expect(m["failedDetections"]).To(ConsistOf("hpaEnabled"))
		})

		It("UT-DL-SERIAL-003: nil DetectedLabels SerializeLabels returns nil", func() {
			var dl *sharedtypes.DetectedLabels
			raw, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())
			Expect(raw).To(BeNil())
		})

		It("UT-DL-SERIAL-004: json.Marshal on DetectedLabels includes all required boolean fields", func() {
			dl := sharedtypes.DetectedLabels{}
			raw, err := json.Marshal(dl)
			Expect(err).ToNot(HaveOccurred())

			var m map[string]interface{}
			Expect(json.Unmarshal(raw, &m)).To(Succeed())
			for _, field := range []string{
				"gitOpsManaged", "pdbProtected", "hpaEnabled",
				"stateful", "helmManaged", "networkIsolated",
				"resourceQuotaConstrained",
			} {
				Expect(m).To(HaveKey(field), "json.Marshal must include CRD-required field: "+field)
			}
		})
	})

	Describe("DB sparse-JSON contract (DBDetectedLabels / models.DetectedLabels)", func() {
		It("UT-DL-SERIAL-010: SerializeLabels omits false booleans from output", func() {
			dl := sharedtypes.DBDetectedLabels{}
			raw, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())

			var m map[string]interface{}
			Expect(json.Unmarshal(raw, &m)).To(Succeed())
			Expect(m).ToNot(HaveKey("gitOpsManaged"))
			Expect(m).ToNot(HaveKey("pdbProtected"))
			Expect(m).ToNot(HaveKey("hpaEnabled"))
			Expect(m).ToNot(HaveKey("stateful"))
			Expect(m).ToNot(HaveKey("helmManaged"))
			Expect(m).ToNot(HaveKey("networkIsolated"))
			Expect(m).ToNot(HaveKey("resourceQuotaConstrained"))
		})

		It("UT-DL-SERIAL-011: SerializeLabels includes only true booleans and non-empty strings", func() {
			dl := sharedtypes.DBDetectedLabels{
				GitOpsManaged:            true,
				GitOpsTool:               "flux",
				HPAEnabled:               false,
				NetworkIsolated:          true,
				ResourceQuotaConstrained: true,
				FailedDetections:         []string{"stateful"},
			}
			raw, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())

			var m map[string]interface{}
			Expect(json.Unmarshal(raw, &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("gitOpsManaged", true))
			Expect(m).To(HaveKeyWithValue("gitOpsTool", "flux"))
			Expect(m).To(HaveKeyWithValue("networkIsolated", true))
			Expect(m).To(HaveKeyWithValue("resourceQuotaConstrained", true))
			Expect(m["failedDetections"]).To(ConsistOf("stateful"))
			Expect(m).ToNot(HaveKey("hpaEnabled"))
			Expect(m).ToNot(HaveKey("pdbProtected"))
			Expect(m).ToNot(HaveKey("stateful"))
			Expect(m).ToNot(HaveKey("helmManaged"))
			Expect(m).ToNot(HaveKey("serviceMesh"))
		})

		It("UT-DL-SERIAL-012: MarshalJSON delegates to SerializeLabels for sparse output", func() {
			dl := sharedtypes.DBDetectedLabels{GitOpsManaged: true}
			fromMarshal, err := json.Marshal(dl)
			Expect(err).ToNot(HaveOccurred())
			fromSerialize, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())
			Expect(fromMarshal).To(Equal(fromSerialize))
		})

		It("UT-DL-SERIAL-013: Value delegates to SerializeLabels for driver.Valuer", func() {
			dl := sharedtypes.DBDetectedLabels{PDBProtected: true, ServiceMesh: "istio"}
			driverVal, err := dl.Value()
			Expect(err).ToNot(HaveOccurred())
			serialized, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())
			Expect(driverVal).To(Equal(serialized))
		})

		It("UT-DL-SERIAL-014: models.DetectedLabels alias resolves to DBDetectedLabels", func() {
			dl := models.NewDetectedLabels()
			Expect(dl.FailedDetections).To(BeEmpty())
			dl.GitOpsManaged = true
			raw, err := dl.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())
			var m map[string]interface{}
			Expect(json.Unmarshal(raw, &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("gitOpsManaged", true))
			Expect(m).ToNot(HaveKey("hpaEnabled"), "sparse: false booleans omitted")
		})
	})

	Describe("Round-trip Scan/Value (DB domain)", func() {
		It("UT-DL-SERIAL-020: DBDetectedLabels Value -> Scan round-trip preserves all fields", func() {
			original := sharedtypes.DBDetectedLabels{
				GitOpsManaged:            true,
				GitOpsTool:               "argocd",
				PDBProtected:             false,
				HPAEnabled:               true,
				Stateful:                 false,
				HelmManaged:              true,
				NetworkIsolated:          false,
				ServiceMesh:              "linkerd",
				ResourceQuotaConstrained: true,
				FailedDetections:         []string{"pdbProtected"},
			}
			dv, err := original.Value()
			Expect(err).ToNot(HaveOccurred())

			var restored sharedtypes.DBDetectedLabels
			Expect(restored.Scan(dv)).To(Succeed())
			Expect(restored.GitOpsManaged).To(Equal(original.GitOpsManaged))
			Expect(restored.GitOpsTool).To(Equal(original.GitOpsTool))
			Expect(restored.PDBProtected).To(Equal(original.PDBProtected))
			Expect(restored.HPAEnabled).To(Equal(original.HPAEnabled))
			Expect(restored.Stateful).To(Equal(original.Stateful))
			Expect(restored.HelmManaged).To(Equal(original.HelmManaged))
			Expect(restored.NetworkIsolated).To(Equal(original.NetworkIsolated))
			Expect(restored.ServiceMesh).To(Equal(original.ServiceMesh))
			Expect(restored.ResourceQuotaConstrained).To(Equal(original.ResourceQuotaConstrained))
			Expect(restored.FailedDetections).To(Equal(original.FailedDetections))
		})

		It("UT-DL-SERIAL-021: Scan(nil) is a no-op", func() {
			dl := sharedtypes.DBDetectedLabels{GitOpsManaged: true}
			Expect(dl.Scan(nil)).To(Succeed())
			Expect(dl.GitOpsManaged).To(BeTrue(), "should not reset on nil scan")
		})

		It("UT-DL-SERIAL-022: Scan rejects non-byte input", func() {
			var dl sharedtypes.DBDetectedLabels
			Expect(dl.Scan(12345)).To(HaveOccurred())
		})

		It("UT-DL-SERIAL-023: Scan handles sparse JSON correctly (missing booleans default to false)", func() {
			sparseJSON := []byte(`{"gitOpsManaged":true,"gitOpsTool":"argocd"}`)
			var dl sharedtypes.DBDetectedLabels
			Expect(dl.Scan(sparseJSON)).To(Succeed())
			Expect(dl.GitOpsManaged).To(BeTrue())
			Expect(dl.GitOpsTool).To(Equal("argocd"))
			Expect(dl.PDBProtected).To(BeFalse())
			Expect(dl.HPAEnabled).To(BeFalse())
			Expect(dl.Stateful).To(BeFalse())
		})
	})

	Describe("CRD Value/Scan (canonical type)", func() {
		It("UT-DL-SERIAL-030: DetectedLabels Value produces full JSON with all fields", func() {
			dl := &sharedtypes.DetectedLabels{GitOpsManaged: true}
			dv, err := dl.Value()
			Expect(err).ToNot(HaveOccurred())
			raw, ok := dv.([]byte)
			Expect(ok).To(BeTrue())

			var m map[string]interface{}
			Expect(json.Unmarshal(raw, &m)).To(Succeed())
			Expect(m).To(HaveKey("pdbProtected"), "CRD Value must include all boolean fields")
			Expect(m).To(HaveKey("stateful"))
		})

		It("UT-DL-SERIAL-031: nil DetectedLabels Value returns nil", func() {
			var dl *sharedtypes.DetectedLabels
			dv, err := dl.Value()
			Expect(err).ToNot(HaveOccurred())
			Expect(dv).To(BeNil())
		})
	})

	Describe("IsEmpty parity", func() {
		It("UT-DL-SERIAL-040: DBDetectedLabels.IsEmpty matches DetectedLabels.IsEmpty contract", func() {
			Expect((&sharedtypes.DBDetectedLabels{}).IsEmpty()).To(BeTrue())
			Expect((&sharedtypes.DBDetectedLabels{GitOpsManaged: true}).IsEmpty()).To(BeFalse())
			Expect((&sharedtypes.DBDetectedLabels{ServiceMesh: "istio"}).IsEmpty()).To(BeFalse())
			Expect((&sharedtypes.DBDetectedLabels{ResourceQuotaConstrained: true}).IsEmpty()).To(BeTrue(),
				"IsEmpty ignores resourceQuotaConstrained (pre-existing contract)")
		})
	})
})
