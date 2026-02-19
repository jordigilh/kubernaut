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

package aianalysis

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aiv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Cycle 3.1: PostRCAContext CRD type API contract tests
// Authority: ADR-056, DD-HAPI-018
// Business Requirement: BR-HAPI-102
var _ = Describe("PostRCAContext CRD Type [ADR-056]", func() {

	Context("UT-AA-056-001: PostRCAContext JSON serialization", func() {
		It("should serialize to correct camelCase JSON", func() {
			now := metav1.Now()
			ctx := aiv1alpha1.PostRCAContext{
				DetectedLabels: &sharedtypes.DetectedLabels{
					GitOpsManaged:    true,
					GitOpsTool:       "flux",
					PDBProtected:     false,
					Stateful:         true,
					FailedDetections: []string{"networkIsolated"},
				},
				SetAt: &now,
			}

			data, err := json.Marshal(ctx)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			err = json.Unmarshal(data, &parsed)
			Expect(err).NotTo(HaveOccurred())

			Expect(parsed).To(HaveKey("detectedLabels"))
			Expect(parsed).To(HaveKey("setAt"))

			labels := parsed["detectedLabels"].(map[string]interface{})
			Expect(labels["gitOpsManaged"]).To(BeTrue())
			Expect(labels["gitOpsTool"]).To(Equal("flux"))
			Expect(labels["stateful"]).To(BeTrue())
		})
	})

	Context("UT-AA-056-002: PostRCAContext omitempty contract", func() {
		It("should omit PostRCAContext from JSON when nil", func() {
			status := aiv1alpha1.AIAnalysisStatus{
				Phase: "Investigating",
			}

			Expect(status.PostRCAContext).To(BeNil())

			data, err := json.Marshal(status)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			err = json.Unmarshal(data, &parsed)
			Expect(err).NotTo(HaveOccurred())

			_, hasKey := parsed["postRCAContext"]
			Expect(hasKey).To(BeFalse(), "postRCAContext should be omitted when nil")
		})
	})
})
