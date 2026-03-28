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

package ogenconv_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/shared/types/ogenconv"
)

// BR-WORKFLOW-004, DD-WORKFLOW-016: Structured description round-trip through ogen wire format.
var _ = Describe("OgenConv Description Converters", func() {

	Context("round-trip conversion", func() {
		It("UT-SHARED-OGENCONV-001: SharedDescriptionToOgen -> OgenDescriptionToShared preserves all fields", func() {
			original := types.StructuredDescription{
				What:          "Increase memory limits for OOMKilled pods",
				WhenToUse:     "When pods are OOMKilled repeatedly",
				WhenNotToUse:  "When memory usage is within normal bounds",
				Preconditions: "Pod must be managed by a Deployment or StatefulSet",
			}

			ogenDesc := ogenconv.SharedDescriptionToOgen(original)
			roundTripped := ogenconv.OgenDescriptionToShared(ogenDesc)

			Expect(roundTripped).To(Equal(original))
		})

		It("UT-SHARED-OGENCONV-002: round-trip preserves empty optional fields", func() {
			original := types.StructuredDescription{
				What:      "Restart crashed pods",
				WhenToUse: "When a pod is in CrashLoopBackOff",
			}

			ogenDesc := ogenconv.SharedDescriptionToOgen(original)
			roundTripped := ogenconv.OgenDescriptionToShared(ogenDesc)

			Expect(roundTripped).To(Equal(original))
		})
	})

	Context("SharedDescriptionToOgen", func() {
		It("UT-SHARED-OGENCONV-003: sets optional ogen fields only when non-empty", func() {
			desc := types.StructuredDescription{
				What:          "Test action",
				WhenToUse:     "Always",
				WhenNotToUse:  "Never",
				Preconditions: "",
			}

			ogenDesc := ogenconv.SharedDescriptionToOgen(desc)

			Expect(ogenDesc.What).To(Equal("Test action"))
			Expect(ogenDesc.WhenToUse).To(Equal("Always"))
			Expect(ogenDesc.WhenNotToUse.Value).To(Equal("Never"))
			Expect(ogenDesc.WhenNotToUse.Set).To(BeTrue())
			Expect(ogenDesc.Preconditions.Set).To(BeFalse())
		})
	})
})
