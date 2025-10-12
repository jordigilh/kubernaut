package toolset_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset/configmap"
)

var _ = Describe("BR-TOOLSET-029: ConfigMap Builder", func() {
	var (
		builder configmap.ConfigMapBuilder
		ctx     context.Context
	)

	BeforeEach(func() {
		builder = configmap.NewConfigMapBuilder("holmesgpt-toolset", "holmesgpt")
		ctx = context.Background()
	})

	// BR-TOOLSET-038: Namespace must be kubernaut-system
	Describe("TDD-RED: Correct Namespace Configuration", func() {
		It("should use kubernaut-system namespace per api-specification.md", func() {
			correctBuilder := configmap.NewConfigMapBuilder("kubernaut-toolset-config", "kubernaut-system")

			toolsetJSON := `{
				"tools": [
					{
						"name": "prometheus",
						"type": "prometheus",
						"endpoint": "http://prometheus:9090",
						"description": "Prometheus monitoring",
						"metadata": {}
					}
				]
			}`

			cm, err := correctBuilder.BuildConfigMap(ctx, toolsetJSON)
			Expect(err).ToNot(HaveOccurred())
			Expect(cm).ToNot(BeNil())

			// TDD-RED: These will fail until implementation is fixed
			Expect(cm.Namespace).To(Equal("kubernaut-system"), "ConfigMap must use kubernaut-system namespace")
			Expect(cm.Name).To(Equal("kubernaut-toolset-config"), "ConfigMap must be named kubernaut-toolset-config")
		})
	})

	Describe("BuildConfigMap", func() {
		Context("with toolset JSON", func() {
			It("should create ConfigMap with toolset data", func() {
				toolsetJSON := `{
					"tools": [
						{
							"name": "prometheus",
							"type": "prometheus",
							"endpoint": "http://prometheus:9090",
							"description": "Prometheus monitoring",
							"metadata": {}
						}
					]
				}`

				cm, err := builder.BuildConfigMap(ctx, toolsetJSON)

				Expect(err).ToNot(HaveOccurred())
				Expect(cm).ToNot(BeNil())
				Expect(cm.Name).To(Equal("holmesgpt-toolset"))
				Expect(cm.Namespace).To(Equal("holmesgpt"))
				Expect(cm.Data).To(HaveKey("toolset.json"))
				Expect(cm.Data["toolset.json"]).To(Equal(toolsetJSON))
			})

			It("should add standard labels", func() {
				toolsetJSON := `{"tools": []}`

				cm, err := builder.BuildConfigMap(ctx, toolsetJSON)

				Expect(err).ToNot(HaveOccurred())
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "holmesgpt-toolset"))
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "dynamic-toolset"))
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "kubernaut"))
			})

			It("should add generation timestamp annotation", func() {
				toolsetJSON := `{"tools": []}`

				cm, err := builder.BuildConfigMap(ctx, toolsetJSON)

				Expect(err).ToNot(HaveOccurred())
				Expect(cm.Annotations).To(HaveKey("kubernaut.io/generated-at"))
				Expect(cm.Annotations["kubernaut.io/generated-at"]).ToNot(BeEmpty())
			})

			It("should handle empty toolset", func() {
				toolsetJSON := `{"tools": []}`

				cm, err := builder.BuildConfigMap(ctx, toolsetJSON)

				Expect(err).ToNot(HaveOccurred())
				Expect(cm.Data["toolset.json"]).To(Equal(toolsetJSON))
			})
		})

		Context("BR-TOOLSET-030: with existing ConfigMap overrides", func() {
			It("should preserve manual overrides in existing ConfigMap", func() {
				existingCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "holmesgpt-toolset",
						Namespace: "holmesgpt",
						Annotations: map[string]string{
							"kubernaut.io/manual-override": "true",
							"user-annotation":              "keep-this",
						},
					},
					Data: map[string]string{
						"toolset.json":    `{"tools": []}`,
						"custom-config":   "manual-value",
						"override-config": "user-modified",
					},
				}

				newToolsetJSON := `{
					"tools": [
						{
							"name": "prometheus",
							"type": "prometheus",
							"endpoint": "http://prometheus:9090",
							"description": "Prometheus",
							"metadata": {}
						}
					]
				}`

				cm, err := builder.BuildConfigMapWithOverrides(ctx, newToolsetJSON, existingCM)

				Expect(err).ToNot(HaveOccurred())

				// Should update toolset.json
				Expect(cm.Data["toolset.json"]).To(Equal(newToolsetJSON))

				// Should preserve custom data keys
				Expect(cm.Data["custom-config"]).To(Equal("manual-value"))
				Expect(cm.Data["override-config"]).To(Equal("user-modified"))

				// Should preserve custom annotations
				Expect(cm.Annotations["user-annotation"]).To(Equal("keep-this"))
				Expect(cm.Annotations["kubernaut.io/manual-override"]).To(Equal("true"))
			})

			It("should merge labels from existing ConfigMap", func() {
				existingCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "holmesgpt-toolset",
						Namespace: "holmesgpt",
						Labels: map[string]string{
							"custom-label": "keep-this",
							"team":         "platform",
						},
					},
					Data: map[string]string{
						"toolset.json": `{"tools": []}`,
					},
				}

				toolsetJSON := `{"tools": []}`

				cm, err := builder.BuildConfigMapWithOverrides(ctx, toolsetJSON, existingCM)

				Expect(err).ToNot(HaveOccurred())

				// Should have standard labels
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "kubernaut"))

				// Should preserve custom labels
				Expect(cm.Labels).To(HaveKeyWithValue("custom-label", "keep-this"))
				Expect(cm.Labels).To(HaveKeyWithValue("team", "platform"))
			})

			It("should handle nil existing ConfigMap", func() {
				toolsetJSON := `{"tools": []}`

				cm, err := builder.BuildConfigMapWithOverrides(ctx, toolsetJSON, nil)

				Expect(err).ToNot(HaveOccurred())
				Expect(cm.Data["toolset.json"]).To(Equal(toolsetJSON))
			})

			It("should not override managed annotations", func() {
				existingCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "holmesgpt-toolset",
						Namespace: "holmesgpt",
						Annotations: map[string]string{
							"kubernaut.io/generated-at": "2024-01-01T00:00:00Z",
						},
					},
					Data: map[string]string{
						"toolset.json": `{"tools": []}`,
					},
				}

				toolsetJSON := `{"tools": []}`

				cm, err := builder.BuildConfigMapWithOverrides(ctx, toolsetJSON, existingCM)

				Expect(err).ToNot(HaveOccurred())

				// Should update generated-at timestamp (not preserve old one)
				Expect(cm.Annotations["kubernaut.io/generated-at"]).ToNot(Equal("2024-01-01T00:00:00Z"))
			})
		})

		Context("with validation", func() {
			It("should reject invalid JSON", func() {
				invalidJSON := `{invalid json`

				_, err := builder.BuildConfigMap(ctx, invalidJSON)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid JSON"))
			})

			It("should reject empty toolset JSON", func() {
				emptyJSON := ""

				_, err := builder.BuildConfigMap(ctx, emptyJSON)

				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("BR-TOOLSET-031: DetectDrift", func() {
		It("should detect changes in toolset data", func() {
			currentCM := &corev1.ConfigMap{
				Data: map[string]string{
					"toolset.json": `{"tools": [{"name": "old"}]}`,
				},
			}

			newToolsetJSON := `{"tools": [{"name": "new"}]}`

			hasDrift := builder.DetectDrift(ctx, currentCM, newToolsetJSON)

			Expect(hasDrift).To(BeTrue())
		})

		It("should not detect drift for identical content", func() {
			toolsetJSON := `{"tools": [{"name": "prometheus"}]}`

			currentCM := &corev1.ConfigMap{
				Data: map[string]string{
					"toolset.json": toolsetJSON,
				},
			}

			hasDrift := builder.DetectDrift(ctx, currentCM, toolsetJSON)

			Expect(hasDrift).To(BeFalse())
		})

		It("should handle missing ConfigMap", func() {
			newToolsetJSON := `{"tools": []}`

			hasDrift := builder.DetectDrift(ctx, nil, newToolsetJSON)

			Expect(hasDrift).To(BeTrue())
		})

		It("should ignore whitespace differences", func() {
			currentCM := &corev1.ConfigMap{
				Data: map[string]string{
					"toolset.json": `{"tools":[{"name":"test"}]}`,
				},
			}

			// Same content, different formatting
			newToolsetJSON := `{
				"tools": [
					{
						"name": "test"
					}
				]
			}`

			hasDrift := builder.DetectDrift(ctx, currentCM, newToolsetJSON)

			Expect(hasDrift).To(BeFalse())
		})

		It("should not consider manual data keys as drift", func() {
			currentCM := &corev1.ConfigMap{
				Data: map[string]string{
					"toolset.json":  `{"tools": []}`,
					"custom-config": "manual-value",
				},
			}

			newToolsetJSON := `{"tools": []}`

			hasDrift := builder.DetectDrift(ctx, currentCM, newToolsetJSON)

			Expect(hasDrift).To(BeFalse())
		})
	})
})
