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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Issue #1061: signalToPrompt target_resource label override", func() {

	Describe("UT-KA-1061-001: Labels override enrichment-resolved kind/name", func() {
		It("should use target_resource_kind and target_resource_name from signal labels", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				Namespace:    "demo-operator",
				Name:         "operator-health",
				Severity:     "critical",
				Message:      "OLM Subscription unhealthy",
				SignalLabels: map[string]string{
					"target_resource_kind": "Subscription",
					"target_resource_name": "etcd",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Subscription"))
			Expect(result.ResourceName).To(Equal("etcd"))
		})
	})

	Describe("UT-KA-1061-002: No labels, enrichment-resolved values preserved", func() {
		It("should keep original ResourceKind and ResourceName when no override labels present", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				Namespace:    "demo-operator",
				Name:         "operator-health",
				Severity:     "critical",
				Message:      "OLM Subscription unhealthy",
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"))
			Expect(result.ResourceName).To(Equal("demo-operator"))
		})
	})

	Describe("UT-KA-1061-003: Partial label override — only kind", func() {
		It("should override only ResourceKind when target_resource_name is absent", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				Namespace:    "demo-operator",
				Name:         "operator-health",
				Severity:     "critical",
				Message:      "OLM Subscription unhealthy",
				SignalLabels: map[string]string{
					"target_resource_kind": "Subscription",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Subscription"))
			Expect(result.ResourceName).To(Equal("demo-operator"))
		})
	})

	Describe("UT-KA-1061-004: Partial label override — only name", func() {
		It("should override only ResourceName when target_resource_kind is absent", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				Namespace:    "demo-operator",
				Name:         "operator-health",
				Severity:     "critical",
				Message:      "OLM Subscription unhealthy",
				SignalLabels: map[string]string{
					"target_resource_name": "etcd",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"))
			Expect(result.ResourceName).To(Equal("etcd"))
		})
	})

	Describe("UT-KA-1061-005: Empty label values are ignored", func() {
		It("should not override when label values are empty strings", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				Namespace:    "demo-operator",
				Name:         "operator-health",
				Severity:     "critical",
				Message:      "OLM Subscription unhealthy",
				SignalLabels: map[string]string{
					"target_resource_kind": "",
					"target_resource_name": "",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"))
			Expect(result.ResourceName).To(Equal("demo-operator"))
		})
	})

	Describe("UT-KA-1061-006: All other fields propagate unchanged", func() {
		It("should pass through all non-overridden fields identically", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				Namespace:    "demo-operator",
				Name:         "operator-health",
				Severity:     "critical",
				Message:      "OLM Subscription unhealthy",
				ClusterName:  "prod-1",
				Environment:  "production",
				Description:  "etcd operator subscription is degraded",
				SignalLabels: map[string]string{
					"target_resource_kind": "Subscription",
					"target_resource_name": "etcd",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.Namespace).To(Equal("demo-operator"))
			Expect(result.Name).To(Equal("operator-health"))
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.Message).To(Equal("OLM Subscription unhealthy"))
			Expect(result.ClusterName).To(Equal("prod-1"))
			Expect(result.Environment).To(Equal("production"))
			Expect(result.Description).To(Equal("etcd operator subscription is degraded"))
		})
	})

	Describe("UT-KA-1061-007: Nil SignalLabels map is safe", func() {
		It("should not panic and should preserve enrichment values when SignalLabels is nil", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				Namespace:    "demo-operator",
				Name:         "operator-health",
				Severity:     "critical",
				Message:      "OLM Subscription unhealthy",
				SignalLabels: nil,
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"),
				"BR-AI-1061: nil SignalLabels must not alter enrichment-resolved kind")
			Expect(result.ResourceName).To(Equal("demo-operator"),
				"BR-AI-1061: nil SignalLabels must not alter enrichment-resolved name")
		})
	})

	Describe("UT-KA-1061-008: Path traversal label values are rejected (FedRAMP SI-10)", func() {
		It("should ignore label values containing path separators", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "../../etc/passwd",
					"target_resource_name": "foo/bar",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"),
				"BR-AI-1061: path traversal in kind label must be rejected")
			Expect(result.ResourceName).To(Equal("demo-operator"),
				"BR-AI-1061: slash in name label must be rejected")
		})
	})

	Describe("UT-KA-1061-009: Control characters in label values are rejected (FedRAMP SI-10)", func() {
		It("should ignore label values containing control characters", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "Sub\x00scription",
					"target_resource_name": "etcd\nnewline",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"),
				"BR-AI-1061: null byte in kind label must be rejected")
			Expect(result.ResourceName).To(Equal("demo-operator"),
				"BR-AI-1061: newline in name label must be rejected")
		})
	})

	Describe("UT-KA-1061-010: Overlong label values are rejected (FedRAMP SI-10)", func() {
		It("should ignore label values exceeding 253 characters", func() {
			longValue := strings.Repeat("a", 254)
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": longValue,
					"target_resource_name": longValue,
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"),
				"BR-AI-1061: overlong kind label must be rejected")
			Expect(result.ResourceName).To(Equal("demo-operator"),
				"BR-AI-1061: overlong name label must be rejected")
		})
	})

	Describe("UT-KA-1061-011: Unicode label values within bounds are accepted", func() {
		It("should accept valid Unicode values that pass validation", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "Deployment",
					"target_resource_name": "app-名前",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Deployment"),
				"BR-AI-1061: valid ASCII kind must be accepted")
			Expect(result.ResourceName).To(Equal("app-名前"),
				"BR-AI-1061: valid Unicode name must be accepted")
		})
	})

	Describe("UT-KA-1061-012: Exactly 253 char label value is accepted", func() {
		It("should accept label values at the boundary (253 chars)", func() {
			boundaryValue := strings.Repeat("a", 253)
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": boundaryValue,
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal(boundaryValue),
				"BR-AI-1061: 253-char kind label is at boundary and must be accepted")
		})
	})

	Describe("UT-KA-1061-013: Backslash in label values is rejected (FedRAMP SI-10)", func() {
		It("should ignore label values containing backslash", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "Sub\\scription",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"),
				"BR-AI-1061: backslash in kind label must be rejected")
		})
	})

	Describe("UT-KA-1061-015: Both labels invalid simultaneously (FedRAMP SI-10)", func() {
		It("should ignore both labels when kind has path traversal and name has control chars", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "../etc/passwd",
					"target_resource_name": "etcd\x00injected",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"),
				"BR-AI-1061: path traversal kind must be rejected")
			Expect(result.ResourceName).To(Equal("demo-operator"),
				"BR-AI-1061: control-char name must be rejected")
		})
	})

	Describe("UT-KA-1061-016: Whitespace-only label values are rejected (FedRAMP SI-10)", func() {
		It("should ignore label values that are only whitespace", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "   ",
					"target_resource_name": "\t\n",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Namespace"),
				"BR-AI-1061: whitespace-only kind label must be rejected")
			Expect(result.ResourceName).To(Equal("demo-operator"),
				"BR-AI-1061: whitespace-only name label must be rejected")
		})
	})

	Describe("UT-KA-1061-014: sameKindValidationGate uses signal.ResourceKind, not overridden kind (ARCH-5)", func() {
		It("should confirm that signal.ResourceKind is unmodified after SignalToPrompt", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				Namespace:    "demo-operator",
				Name:         "operator-health",
				Severity:     "critical",
				Message:      "OLM Subscription unhealthy",
				SignalLabels: map[string]string{
					"target_resource_kind": "Subscription",
					"target_resource_name": "etcd",
				},
			}

			result := investigator.SignalToPrompt(signal)
			Expect(result.ResourceKind).To(Equal("Subscription"),
				"prompt should use overridden kind")
			Expect(signal.ResourceKind).To(Equal("Namespace"),
				"ARCH-5: signal.ResourceKind must remain unchanged after SignalToPrompt "+
					"so sameKindValidationGate compares against enrichment-layer identity")
		})
	})
})
