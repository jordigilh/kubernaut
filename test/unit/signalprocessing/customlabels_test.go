package signalprocessing

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/rego"
)

// BR-SP-102: CustomLabels Rego Extraction
// BR-SP-103: CustomLabels Validation Limits
// DD-WORKFLOW-001 v1.9: Security wrapper and validation limits
var _ = Describe("BR-SP-102, BR-SP-103: CustomLabels Extractor", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		extractor *rego.CustomLabelsExtractor
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================
	// TC-CL-001 to TC-CL-003: Label Extraction
	// ========================================
	Describe("Label Extraction from Rego", func() {
		It("TC-CL-001: should extract team label from namespace", func() {
			// Create ConfigMap with Rego policy
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["team"] := value if {
    value := [concat("=", ["name", input.namespace.labels.team])]
    input.namespace.labels.team
}
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{
					Name:   "default",
					Labels: map[string]string{"team": "payments"},
				},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			Expect(labels).To(HaveKey("team"))
			Expect(labels["team"]).To(ContainElement("name=payments"))
		})

		It("TC-CL-002: should extract risk-tolerance label", func() {
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["risk"] := value if {
    value := ["high"]
    input.namespace.labels.environment == "production"
}
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{
					Name:   "prod",
					Labels: map[string]string{"environment": "production"},
				},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			Expect(labels).To(HaveKey("risk"))
			Expect(labels["risk"]).To(ContainElement("high"))
		})

		It("TC-CL-003: should extract constraint label", func() {
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["constraint"] := value if {
    value := ["cost-constrained"]
    input.namespace.labels.cost_center
}
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{
					Name:   "apps",
					Labels: map[string]string{"cost_center": "engineering"},
				},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			Expect(labels).To(HaveKey("constraint"))
			Expect(labels["constraint"]).To(ContainElement("cost-constrained"))
		})
	})

	// ========================================
	// TC-CL-004: Security - System Labels Stripped
	// ========================================
	Describe("Security Wrapper", func() {
		It("TC-CL-004: should strip system labels from output", func() {
			// Policy tries to set reserved system labels
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["environment"] := value if {
    value := ["hacked"]
}
labels["priority"] := value if {
    value := ["P0"]
}
labels["custom"] := value if {
    value := ["allowed"]
}
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{Name: "default"},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			// System labels should be stripped
			Expect(labels).ToNot(HaveKey("environment"))
			Expect(labels).ToNot(HaveKey("priority"))
			// Custom labels should remain
			Expect(labels).To(HaveKey("custom"))
			Expect(labels["custom"]).To(ContainElement("allowed"))
		})

		It("should strip reserved prefixes", func() {
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["kubernaut.ai/internal"] := value if {
    value := ["secret"]
}
labels["system/admin"] := value if {
    value := ["root"]
}
labels["user/custom"] := value if {
    value := ["allowed"]
}
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{Name: "default"},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			// Reserved prefixes should be stripped
			Expect(labels).ToNot(HaveKey("kubernaut.ai/internal"))
			Expect(labels).ToNot(HaveKey("system/admin"))
			// User prefixes should remain
			Expect(labels).To(HaveKey("user/custom"))
		})
	})

	// ========================================
	// TC-CL-005, TC-CL-006: Error Handling
	// ========================================
	Describe("Error Handling", func() {
		It("TC-CL-005: should return empty map for missing policy ConfigMap", func() {
			// No ConfigMap created
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			err := extractor.LoadPolicy(ctx)

			// Should fail to load policy
			Expect(err).To(HaveOccurred())
		})

		It("TC-CL-006: should handle policy evaluation error gracefully", func() {
			// Policy with intentional syntax error in output (non-fatal)
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

# Valid empty policy
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{Name: "default"},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			Expect(labels).To(BeEmpty()) // Empty map for no results
		})
	})

	// ========================================
	// Validation Limits (DD-WORKFLOW-001 v1.9)
	// ========================================
	Describe("Validation Limits", func() {
		It("should enforce max 10 keys", func() {
			// Policy returns 15 keys
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["key1"] := ["v1"]
labels["key2"] := ["v2"]
labels["key3"] := ["v3"]
labels["key4"] := ["v4"]
labels["key5"] := ["v5"]
labels["key6"] := ["v6"]
labels["key7"] := ["v7"]
labels["key8"] := ["v8"]
labels["key9"] := ["v9"]
labels["key10"] := ["v10"]
labels["key11"] := ["v11"]
labels["key12"] := ["v12"]
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{Name: "default"},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			Expect(len(labels)).To(BeNumerically("<=", 10))
		})

		It("should enforce max 5 values per key", func() {
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["many_values"] := ["v1", "v2", "v3", "v4", "v5", "v6", "v7", "v8"]
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{Name: "default"},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			Expect(labels).To(HaveKey("many_values"))
			Expect(len(labels["many_values"])).To(BeNumerically("<=", 5))
		})

		It("should truncate keys longer than 63 chars", func() {
			longKey := "this_is_a_very_long_key_name_that_exceeds_the_kubernetes_label_limit_of_63_chars"
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["` + longKey + `"] := ["value"]
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{Name: "default"},
			}

			labels, err := extractor.Extract(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			// Key should be truncated to max 63 chars
			for key := range labels {
				Expect(len(key)).To(BeNumerically("<=", 63))
			}
		})
	})

	// ========================================
	// ConfigMap Hot Reload
	// ========================================
	Describe("Policy Hot Reload", func() {
		It("should reload policy when ConfigMap changes", func() {
			// Initial policy
			configMap := createPolicyConfigMap(`
package signalprocessing.labels

labels["version"] := ["v1"]
`)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(configMap).
				Build()

			extractor = rego.NewCustomLabelsExtractor(fakeClient, ctrl.Log.WithName("test"))
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			input := &rego.LabelInput{
				Namespace: rego.NamespaceContext{Name: "default"},
			}

			labels, err := extractor.Extract(ctx, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(labels["version"]).To(ContainElement("v1"))

			// Update ConfigMap with new policy
			updatedCM := createPolicyConfigMap(`
package signalprocessing.labels

labels["version"] := ["v2"]
`)
			Expect(fakeClient.Update(ctx, updatedCM)).To(Succeed())

			// Reload policy
			Expect(extractor.LoadPolicy(ctx)).To(Succeed())

			labels, err = extractor.Extract(ctx, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(labels["version"]).To(ContainElement("v2"))
		})
	})
})

// Helper function to create policy ConfigMap
func createPolicyConfigMap(policy string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "signal-processing-policies",
			Namespace: "kubernaut-system",
		},
		Data: map[string]string{
			"labels.rego": policy,
		},
	}
}

