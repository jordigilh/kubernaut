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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Issue #1064: ApplySignalLabelOverrides — defense-in-depth for tool context", func() {

	Describe("UT-KA-1064-012: Valid target_resource_kind label overrides ResourceKind", func() {
		It("should return signal with ResourceKind set to the label value", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "Subscription",
				},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceKind).To(Equal("Subscription"),
				"ResourceKind should be overridden by target_resource_kind label")
		})
	})

	Describe("UT-KA-1064-013: Valid target_resource_name label overrides ResourceName", func() {
		It("should return signal with ResourceName set to the label value", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Subscription",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_name": "etcd",
				},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceName).To(Equal("etcd"),
				"ResourceName should be overridden by target_resource_name label")
		})
	})

	Describe("UT-KA-1064-014: Invalid label values rejected (enrichment fallback)", func() {
		It("should preserve original ResourceKind when label contains path traversal", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "../etc/passwd",
				},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"),
				"invalid label must be rejected; enrichment value preserved")
		})
	})

	Describe("UT-KA-1064-015: No-op when SignalLabels is nil", func() {
		It("should return identical signal when no labels present", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: nil,
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"))
			Expect(result.ResourceName).To(Equal("demo-operator"))
		})
	})

	Describe("UT-KA-1064-016: Original SignalContext not modified (value semantics)", func() {
		It("should not mutate the original signal passed to ApplySignalLabelOverrides", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "Subscription",
					"target_resource_name": "etcd",
				},
			}
			_ = investigator.ApplySignalLabelOverrides(signal)
			Expect(signal.ResourceKind).To(Equal("Namespace"),
				"original signal must not be modified")
			Expect(signal.ResourceName).To(Equal("demo-operator"),
				"original signal must not be modified")
		})
	})
})
