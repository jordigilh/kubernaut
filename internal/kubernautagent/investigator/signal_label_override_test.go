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
	"sync"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
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

var _ = Describe("Issue #1061: logLabelOverrideOrRejection audit logging", func() {

	newCapturingLogger := func() (func() []string, logr.Logger) {
		var mu sync.Mutex
		var lines []string
		logger := funcr.New(func(prefix, args string) {
			mu.Lock()
			defer mu.Unlock()
			lines = append(lines, prefix+" "+args)
		}, funcr.Options{Verbosity: 10})
		return func() []string {
			mu.Lock()
			defer mu.Unlock()
			dst := make([]string, len(lines))
			copy(dst, lines)
			return dst
		}, logger
	}

	Describe("UT-KA-1061-017: logs override event when label override is applied", func() {
		It("should emit a structured log with original and override values", func() {
			getLines, logger := newCapturingLogger()

			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "Subscription",
					"target_resource_name": "etcd",
				},
			}
			result := investigator.SignalToPrompt(signal)
			investigator.LogLabelOverrideOrRejection(logger, signal, result, "corr-123", "RCA")

			lines := getLines()
			Expect(lines).To(HaveLen(1),
				"BR-AI-1061: exactly one override log line expected")
			Expect(lines[0]).To(ContainSubstring("signal label override applied to RCA prompt"),
				"BR-AI-1061: log must identify the phase")
			Expect(lines[0]).To(ContainSubstring("original_kind"),
				"BR-AI-1061/FED-1: log must include original_kind for audit trail")
			Expect(lines[0]).To(ContainSubstring("corr-123"),
				"BR-AI-1061/FED-2: log must include correlation_id for trace linking")
		})
	})

	Describe("UT-KA-1061-018: logs rejection event when label fails validation", func() {
		It("should emit a rejection log for each invalid label", func() {
			getLines, logger := newCapturingLogger()

			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: map[string]string{
					"target_resource_kind": "../../etc/passwd",
					"target_resource_name": "foo/bar",
				},
			}
			result := investigator.SignalToPrompt(signal)
			investigator.LogLabelOverrideOrRejection(logger, signal, result, "corr-456", "workflow selection")

			lines := getLines()
			Expect(lines).To(HaveLen(2),
				"BR-AI-1061/SEC-6: one rejection log per invalid label expected")
			Expect(lines[0]).To(ContainSubstring("signal label override rejected"),
				"BR-AI-1061/SEC-6: rejection log must use consistent message prefix")
			Expect(lines[0]).To(ContainSubstring("../../etc/passwd"),
				"BR-AI-1061/SEC-6: rejection log must include the rejected value")
			Expect(lines[1]).To(ContainSubstring("foo/bar"))
		})
	})

	Describe("UT-KA-1061-019: no log emitted when SignalLabels is nil", func() {
		It("should not emit any log when labels map is nil", func() {
			getLines, logger := newCapturingLogger()

			signal := katypes.SignalContext{
				ResourceKind: "Namespace",
				ResourceName: "demo-operator",
				SignalLabels: nil,
			}
			result := investigator.SignalToPrompt(signal)
			investigator.LogLabelOverrideOrRejection(logger, signal, result, "corr-789", "RCA")

			lines := getLines()
			Expect(lines).To(BeEmpty(),
				"BR-AI-1061: no log expected when SignalLabels is nil (no override attempted)")
		})
	})

	Describe("UT-KA-1061-020: no log emitted when label matches enrichment value", func() {
		It("should not log override or rejection when label equals enrichment value", func() {
			getLines, logger := newCapturingLogger()

			signal := katypes.SignalContext{
				ResourceKind: "Subscription",
				ResourceName: "etcd",
				SignalLabels: map[string]string{
					"target_resource_kind": "Subscription",
					"target_resource_name": "etcd",
				},
			}
			result := investigator.SignalToPrompt(signal)
			investigator.LogLabelOverrideOrRejection(logger, signal, result, "corr-noop", "RCA")

			lines := getLines()
			Expect(lines).To(BeEmpty(),
				"BR-AI-1061: no log when label value matches enrichment value (no-op override)")
		})
	})
})

// Issue #1051: target_resource_api_version label override and isValidAPIVersion tests.
// These exercise the ApplySignalLabelOverrides consistency guard that clears
// ResourceAPIVersion when kind is overridden without an explicit api_version,
// and validate the isValidAPIVersion regex indirectly through the public API.
var _ = Describe("TP-1051: target_resource_api_version label override and isValidAPIVersion", func() {

	DescribeTable("UT-KA-1051-020: isValidAPIVersion accepts/rejects via ApplySignalLabelOverrides (Issue #1051 / FedRAMP SI-10)",
		func(apiVersion string, shouldAccept bool) {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceName:       "my-app",
				ResourceAPIVersion: "apps/v1",
				SignalLabels: map[string]string{
					"target_resource_kind":        "StatefulSet",
					"target_resource_api_version": apiVersion,
				},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			if shouldAccept {
				Expect(result.ResourceAPIVersion).To(Equal(apiVersion),
					"Issue #1051: valid apiVersion %q should be accepted", apiVersion)
			} else {
				Expect(result.ResourceAPIVersion).To(BeEmpty(),
					"Issue #1051: invalid apiVersion %q should be rejected; kind override clears api_version", apiVersion)
			}
		},
		Entry("empty string", "", false),
		Entry("valid core v1", "v1", true),
		Entry("valid apps/v1", "apps/v1", true),
		Entry("valid apps/v1beta1", "apps/v1beta1", true),
		Entry("valid apps/v2alpha1", "apps/v2alpha1", true),
		Entry("valid CRD route.openshift.io/v1", "route.openshift.io/v1", true),
		Entry("valid CRD networking.k8s.io/v1", "networking.k8s.io/v1", true),
		Entry("valid CRD batch/v1", "batch/v1", true),
		Entry("valid policy/v1", "policy/v1", true),
		Entry("invalid: uppercase group", "Apps/v1", false),
		Entry("invalid: missing v prefix", "apps/1", false),
		Entry("invalid: plain kind", "Deployment", false),
		Entry("invalid: trailing slash", "apps/v1/", false),
		Entry("invalid: double slash", "apps//v1", false),
		Entry("invalid: spaces", "apps/ v1", false),
		Entry("invalid: control characters", "apps/v1\x00", false),
		Entry("boundary: exactly 253 bytes", strings.Repeat("a", 250)+"/v1", true),
		Entry("invalid: 254 bytes (overlong)", strings.Repeat("a", 251)+"/v1", false),
	)

	Describe("UT-KA-1051-021: kind override without api_version clears ResourceAPIVersion (consistency guard)", func() {
		It("should clear ResourceAPIVersion when kind is overridden but api_version is absent (Issue #1051)", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceName:       "my-app",
				ResourceAPIVersion: "apps/v1",
				SignalLabels: map[string]string{
					"target_resource_kind": "StatefulSet",
				},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceKind).To(Equal("StatefulSet"),
				"Issue #1051: kind should be overridden")
			Expect(result.ResourceAPIVersion).To(BeEmpty(),
				"Issue #1051: api_version must be cleared when kind is overridden without explicit api_version")
		})
	})

	Describe("UT-KA-1051-022: kind override with valid api_version updates both", func() {
		It("should set both ResourceKind and ResourceAPIVersion when both labels are valid (Issue #1051)", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceName:       "my-app",
				ResourceAPIVersion: "apps/v1",
				SignalLabels: map[string]string{
					"target_resource_kind":        "DaemonSet",
					"target_resource_api_version": "apps/v1",
				},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceKind).To(Equal("DaemonSet"),
				"Issue #1051: kind should be overridden")
			Expect(result.ResourceAPIVersion).To(Equal("apps/v1"),
				"Issue #1051: api_version should be set from label")
		})
	})

	Describe("UT-KA-1051-023: invalid api_version with kind override clears api_version", func() {
		It("should clear ResourceAPIVersion when kind is overridden and api_version is invalid (Issue #1051)", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceName:       "my-app",
				ResourceAPIVersion: "apps/v1",
				SignalLabels: map[string]string{
					"target_resource_kind":        "StatefulSet",
					"target_resource_api_version": "INVALID",
				},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceKind).To(Equal("StatefulSet"),
				"Issue #1051: kind should be overridden")
			Expect(result.ResourceAPIVersion).To(BeEmpty(),
				"Issue #1051: invalid api_version + kind override must clear api_version")
		})
	})

	Describe("UT-KA-1051-024: api_version override alone (no kind override) is ignored (SEC-1 guard)", func() {
		It("should NOT update ResourceAPIVersion when kind label is absent (Issue #1051 / SEC-1)", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceName:       "my-app",
				ResourceAPIVersion: "apps/v1",
				SignalLabels: map[string]string{
					"target_resource_api_version": "apps/v1beta1",
				},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceKind).To(Equal("Deployment"),
				"Issue #1051: kind should remain unchanged")
			Expect(result.ResourceAPIVersion).To(Equal("apps/v1"),
				"Issue #1051 / SEC-1: api_version alone must be ignored to prevent inconsistent GVK")
		})
	})

	Describe("UT-KA-1051-025: no api_version label and no kind override preserves original", func() {
		It("should preserve ResourceAPIVersion when neither kind nor api_version labels are set (Issue #1051)", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceName:       "my-app",
				ResourceAPIVersion: "apps/v1",
				SignalLabels:       map[string]string{},
			}
			result := investigator.ApplySignalLabelOverrides(signal)
			Expect(result.ResourceAPIVersion).To(Equal("apps/v1"),
				"Issue #1051: api_version should be preserved when no labels override it")
		})
	})
})

var _ = Describe("SyncSignalFromRCA — signal/RCA target reconciliation (#1374)", func() {

	Describe("UT-KA-WD-001: signal missing apiVersion, RCA target has it", func() {
		It("should copy RCA target apiVersion to signal [BR-WORKFLOW-004]", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
			}
			target := katypes.RemediationTarget{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceAPIVersion).To(Equal("apps/v1"),
				"signal must inherit RCA target apiVersion for workflow GVK matching")
			Expect(result.ComponentGVK()).To(Equal("apps/v1/Deployment"),
				"ComponentGVK must produce full GVK after sync")
		})
	})

	Describe("UT-KA-WD-002: signal already has apiVersion, RCA target overrides", func() {
		It("should use RCA target apiVersion (parity with autonomous L505) [BR-WORKFLOW-004]", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceAPIVersion: "apps/v1",
			}
			target := katypes.RemediationTarget{
				Kind:       "Deployment",
				APIVersion: "extensions/v1beta1",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceAPIVersion).To(Equal("extensions/v1beta1"),
				"RCA target apiVersion is authoritative for workflow discovery")
		})
	})

	Describe("UT-KA-WD-003: both signal and RCA target missing apiVersion", func() {
		It("should leave signal apiVersion empty [BR-WORKFLOW-004]", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
			}
			target := katypes.RemediationTarget{
				Kind: "Deployment",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceAPIVersion).To(BeEmpty(),
				"no apiVersion to sync from RCA target")
			Expect(result.ComponentGVK()).To(BeEmpty(),
				"ComponentGVK must be empty without apiVersion")
		})
	})

	Describe("UT-KA-WD-004: OCP ambiguous kind (Node) — apiVersion from RCA is critical", func() {
		It("should sync apiVersion for Node to avoid cross-API-group mismatch [BR-WORKFLOW-004]", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Node",
			}
			target := katypes.RemediationTarget{
				Kind:       "Node",
				APIVersion: "v1",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceAPIVersion).To(Equal("v1"))
			Expect(result.ComponentGVK()).To(Equal("v1/Node"),
				"must distinguish core/v1 Node from other API groups with Node kind")
		})
	})

	Describe("UT-KA-1374-001: cross-resource RCA — Kind synced from RCA target", func() {
		It("should produce correct GVK for AuthorizationPolicy [BR-WORKFLOW-004, SI-10]", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceName:       "api-server",
				Namespace:          "demo-mesh",
				ResourceAPIVersion: "apps/v1",
			}
			target := katypes.RemediationTarget{
				Kind:       "AuthorizationPolicy",
				Name:       "deny-all-traffic",
				Namespace:  "demo-mesh",
				APIVersion: "security.istio.io/v1",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceKind).To(Equal("AuthorizationPolicy"),
				"Kind must be synced from RCA target for cross-resource RCA")
			Expect(result.ResourceAPIVersion).To(Equal("security.istio.io/v1"))
			Expect(result.ComponentGVK()).To(Equal("security.istio.io/v1/AuthorizationPolicy"),
				"ComponentGVK must reflect RCA target, not original alert target")
		})
	})

	Describe("UT-KA-1374-002: cross-resource RCA — Name and Namespace synced", func() {
		It("should sync Name and Namespace from RCA target [BR-WORKFLOW-004]", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "production",
			}
			target := katypes.RemediationTarget{
				Kind:       "AuthorizationPolicy",
				Name:       "deny-all-traffic",
				Namespace:  "demo-mesh",
				APIVersion: "security.istio.io/v1",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceName).To(Equal("deny-all-traffic"),
				"Name must be synced from RCA target")
			Expect(result.Namespace).To(Equal("demo-mesh"),
				"Namespace must be synced from RCA target")
		})
	})

	Describe("UT-KA-1374-003: stale GVK guard — Kind changes without apiVersion", func() {
		It("should clear apiVersion to prevent invalid GVK [BR-WORKFLOW-004, SI-10]", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceAPIVersion: "apps/v1",
			}
			target := katypes.RemediationTarget{
				Kind: "AuthorizationPolicy",
				Name: "deny-all-traffic",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceKind).To(Equal("AuthorizationPolicy"))
			Expect(result.ResourceAPIVersion).To(BeEmpty(),
				"stale apiVersion must be cleared when Kind changes but RCA has no apiVersion")
			Expect(result.ComponentGVK()).To(BeEmpty(),
				"ComponentGVK must be empty to prevent nonsensical GVK")
		})
	})

	Describe("UT-KA-1374-004: same-resource RCA — apiVersion always synced from RCA", func() {
		It("should override signal apiVersion with RCA apiVersion [BR-WORKFLOW-004]", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceAPIVersion: "apps/v1",
			}
			target := katypes.RemediationTarget{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceAPIVersion).To(Equal("apps/v1"))
			Expect(result.ResourceKind).To(Equal("Deployment"))
		})
	})

	Describe("UT-KA-1374-005: RCA target with empty Kind — signal unchanged", func() {
		It("should not modify signal when RCA target Kind is empty [BR-WORKFLOW-004]", func() {
			signal := katypes.SignalContext{
				ResourceKind:       "Deployment",
				ResourceName:       "api-server",
				Namespace:          "production",
				ResourceAPIVersion: "apps/v1",
			}
			target := katypes.RemediationTarget{}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceKind).To(Equal("Deployment"))
			Expect(result.ResourceName).To(Equal("api-server"))
			Expect(result.Namespace).To(Equal("production"))
			Expect(result.ResourceAPIVersion).To(Equal("apps/v1"))
		})
	})

	Describe("UT-KA-1374-006: cluster-scoped kind — Namespace cleared", func() {
		It("should clear Namespace for cluster-scoped RCA target [BR-WORKFLOW-004]", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
				Namespace:    "production",
			}
			target := katypes.RemediationTarget{
				Kind:       "Node",
				Name:       "worker-1",
				APIVersion: "v1",
			}
			result := investigator.SyncSignalFromRCA(signal, target)
			Expect(result.ResourceKind).To(Equal("Node"))
			Expect(result.ResourceName).To(Equal("worker-1"))
			Expect(result.Namespace).To(BeEmpty(),
				"cluster-scoped kind must have empty namespace")
			Expect(result.ComponentGVK()).To(Equal("v1/Node"))
		})
	})
})

var _ = Describe("FinalizeWorkflowResult — post-Phase 3 processing parity (#1374 F8)", func() {

	Describe("UT-KA-1374-F8-001: severity backfill from signal when result has none", func() {
		It("should backfill severity from signal [BR-INTERACTIVE-010]", func() {
			result := &katypes.InvestigationResult{
				RCASummary: "test rca",
			}
			signal := katypes.SignalContext{
				Severity:     "critical",
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "production",
			}
			rcaResult := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
			}
			investigator.FinalizeWorkflowResult(result, signal, rcaResult, nil)
			Expect(result.Severity).To(Equal("critical"),
				"severity must be backfilled from signal when result has none")
		})
	})

	Describe("UT-KA-1374-F8-002: severity preserved when result already has it", func() {
		It("should not override existing severity [BR-INTERACTIVE-010]", func() {
			result := &katypes.InvestigationResult{
				Severity:   "high",
				RCASummary: "test rca",
			}
			signal := katypes.SignalContext{
				Severity:     "critical",
				ResourceKind: "Deployment",
			}
			investigator.FinalizeWorkflowResult(result, signal, &katypes.InvestigationResult{}, nil)
			Expect(result.Severity).To(Equal("high"),
				"existing severity must be preserved")
		})
	})

	Describe("UT-KA-1374-F8-003: InjectRemediationTarget sets target from signal", func() {
		It("should set RemediationTarget from signal when enrichData is nil [BR-INTERACTIVE-010]", func() {
			result := &katypes.InvestigationResult{
				RCASummary: "test rca",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment",
				},
			}
			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "production",
			}
			investigator.FinalizeWorkflowResult(result, signal, &katypes.InvestigationResult{}, nil)
			Expect(result.RemediationTarget.Name).To(Equal("api-server"),
				"RemediationTarget.Name must be set from signal")
			Expect(result.RemediationTarget.Namespace).To(Equal("production"),
				"RemediationTarget.Namespace must be set from signal")
		})
	})

	Describe("UT-KA-1374-F8-004: InjectTargetResourceParameters populates TARGET_RESOURCE_* params", func() {
		It("should inject TARGET_RESOURCE_* parameters [BR-INTERACTIVE-010]", func() {
			result := &katypes.InvestigationResult{
				RCASummary: "test rca",
				RemediationTarget: katypes.RemediationTarget{
					Kind:       "Deployment",
					Name:       "api-server",
					Namespace:  "production",
					APIVersion: "apps/v1",
				},
			}
			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "production",
			}
			investigator.FinalizeWorkflowResult(result, signal, &katypes.InvestigationResult{}, nil)
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"))
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "api-server"))
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", "production"))
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_API_VERSION", "apps/v1"))
		})
	})

	Describe("UT-KA-1374-F8-005: apiVersion propagation from RCA when result lacks it", func() {
		It("should copy apiVersion from rcaResult when workflowResult has none [BR-WORKFLOW-004]", func() {
			result := &katypes.InvestigationResult{
				RCASummary: "test rca",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment",
					Name: "api-server",
				},
			}
			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "production",
			}
			rcaResult := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
			}
			investigator.FinalizeWorkflowResult(result, signal, rcaResult, nil)
			Expect(result.RemediationTarget.APIVersion).To(Equal("apps/v1"),
				"apiVersion must be propagated from rcaResult when workflowResult lacks it")
		})
	})
})
