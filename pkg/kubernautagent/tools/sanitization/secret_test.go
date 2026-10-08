package sanitization_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	sharedsanitization "github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

var _ = Describe("Kubernaut Agent K8S-SECRET Sanitizer — #966", func() {

	var (
		stage sanitization.Stage
		ctx   context.Context
	)

	BeforeEach(func() {
		stage = sanitization.NewSecretSanitizer()
		ctx = context.Background()
	})

	Describe("UT-KA-966-001: Redacts JSON Secret data values", func() {
		It("should redact data map values in a Secret object", func() {
			secret := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]interface{}{"name": "db-creds", "namespace": "prod"},
				"data": map[string]interface{}{
					"username": "YWRtaW4=",
					"password": "czNjcjN0",
					"key":      "c2VjcmV0LWtleQ==",
					"tls.crt":  "Y2VydGlmaWNhdGU=",
				},
			}
			input, err := json.Marshal(secret)
			Expect(err).NotTo(HaveOccurred())

			result, sanitizeErr := stage.Sanitize(ctx, string(input))
			Expect(sanitizeErr).NotTo(HaveOccurred())

			Expect(result).NotTo(ContainSubstring("YWRtaW4="))
			Expect(result).NotTo(ContainSubstring("czNjcjN0"))
			Expect(result).NotTo(ContainSubstring("c2VjcmV0LWtleQ=="))
			Expect(result).NotTo(ContainSubstring("Y2VydGlmaWNhdGU="))
			Expect(result).To(ContainSubstring(sharedsanitization.RedactedPlaceholder))
			Expect(result).To(ContainSubstring("db-creds"))
		})
	})

	Describe("UT-KA-966-002: Redacts stringData values", func() {
		It("should redact stringData map values", func() {
			secret := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]interface{}{"name": "tls-cert"},
				"stringData": map[string]interface{}{
					"tls.key": "-----BEGIN RSA PRIVATE KEY-----\nfakekey\n-----END RSA PRIVATE KEY-----",
				},
			}
			input, err := json.Marshal(secret)
			Expect(err).NotTo(HaveOccurred())

			result, sanitizeErr := stage.Sanitize(ctx, string(input))
			Expect(sanitizeErr).NotTo(HaveOccurred())

			Expect(result).NotTo(ContainSubstring("fakekey"))
			Expect(result).To(ContainSubstring(sharedsanitization.RedactedPlaceholder))
		})
	})

	Describe("UT-KA-966-003: Handles SecretList", func() {
		It("should redact data in all Secrets within a SecretList", func() {
			list := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "SecretList",
				"items": []interface{}{
					map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata":   map[string]interface{}{"name": "secret-1"},
						"data":       map[string]interface{}{"key1": "dmFsdWUx"},
					},
					map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata":   map[string]interface{}{"name": "secret-2"},
						"data":       map[string]interface{}{"key2": "dmFsdWUy"},
					},
				},
			}
			input, err := json.Marshal(list)
			Expect(err).NotTo(HaveOccurred())

			result, sanitizeErr := stage.Sanitize(ctx, string(input))
			Expect(sanitizeErr).NotTo(HaveOccurred())

			Expect(result).NotTo(ContainSubstring("dmFsdWUx"))
			Expect(result).NotTo(ContainSubstring("dmFsdWUy"))
			Expect(result).To(ContainSubstring("secret-1"))
			Expect(result).To(ContainSubstring("secret-2"))
		})
	})

	Describe("UT-KA-966-004: Preserves non-Secret JSON", func() {
		It("should not modify ConfigMap JSON", func() {
			cm := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "my-config"},
				"data":       map[string]interface{}{"app.conf": "key=value"},
			}
			input, err := json.Marshal(cm)
			Expect(err).NotTo(HaveOccurred())

			result, sanitizeErr := stage.Sanitize(ctx, string(input))
			Expect(sanitizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(string(input)))
		})

		It("UT-KA-2485-008 (BR-KA-211 FR-1): should not modify Kubernetes metadata annotations", func() {
			input := `{"apiVersion":"v1","kind":"Node","metadata":{"annotations":{"key":"maintenance"}}}`

			result, sanitizeErr := stage.Sanitize(ctx, input)
			Expect(sanitizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(input))
		})
	})

	Describe("UT-KA-966-005: Non-JSON passthrough", func() {
		It("should pass non-JSON text unchanged", func() {
			input := "NAME   READY   STATUS    RESTARTS   AGE\nnginx  1/1     Running   0          5m"
			result, sanitizeErr := stage.Sanitize(ctx, input)
			Expect(sanitizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(input))
		})
	})

	Describe("UT-KA-966-006: Secret without data field", func() {
		It("should return unchanged when Secret has no data or stringData", func() {
			secret := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   map[string]interface{}{"name": "empty-secret"},
				"type":       "Opaque",
			}
			input, err := json.Marshal(secret)
			Expect(err).NotTo(HaveOccurred())

			result, sanitizeErr := stage.Sanitize(ctx, string(input))
			Expect(sanitizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(string(input)))
		})
	})

	Describe("UT-KA-966-007: Stage name", func() {
		It("should return K8S-SECRET as the stage name", func() {
			Expect(stage.Name()).To(Equal("K8S-SECRET"))
		})
	})

	Describe("UT-KA-2485-002 (BR-KA-211 FR-2): Redacts YAML Secret data and stringData", func() {
		It("should redact every Secret value, including data.key and arbitrary entries", func() {
			input := `apiVersion: v1
kind: Secret
metadata:
  name: demo
data:
  key: c2VjcmV0LWtleQ==
  tls.crt: Y2VydGlmaWNhdGU=
stringData:
  password: plaintext-password
`

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("c2VjcmV0LWtleQ=="))
			Expect(result).NotTo(ContainSubstring("Y2VydGlmaWNhdGU="))
			Expect(result).NotTo(ContainSubstring("plaintext-password"))
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring(sharedsanitization.RedactedPlaceholder))
			Expect(result).To(ContainSubstring("name: demo"))
		})
	})

	Describe("UT-KA-2485-003 (BR-KA-211 FR-2): Redacts YAML Secret collections", func() {
		It("should redact Secrets in SecretList and generic List documents", func() {
			input := `apiVersion: v1
kind: SecretList
items:
- apiVersion: v1
  kind: Secret
  metadata:
    name: first
  data:
    arbitrary: Zmlyc3Qtc2VjcmV0
---
apiVersion: v1
kind: List
items:
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config
  data:
    key: maintenance
- apiVersion: v1
  kind: Secret
  metadata:
    name: second
  stringData:
    tls.key: second-secret
`

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("Zmlyc3Qtc2VjcmV0"))
			Expect(result).NotTo(ContainSubstring("second-secret"))
			Expect(result).To(ContainSubstring("name: first"))
			Expect(result).To(ContainSubstring("name: second"))
			Expect(result).To(ContainSubstring("key: maintenance"),
				"non-Secret List items must retain their data")
		})
	})

	Describe("UT-KA-2485-005 (BR-KA-211 FR-1/FR-2): Handles non-Secret and invalid YAML", func() {
		It("should leave non-Secret YAML byte-for-byte unchanged", func() {
			input := "apiVersion: v1\nkind: Node\nspec:\n  taints:\n  - effect: NoSchedule\n    key: maintenance\n"

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(input))
		})

		It("should pass invalid YAML through for the generic stages", func() {
			input := "not: [valid: yaml"

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(input))
		})
	})

	Describe("UT-KA-2485-007 (BR-KA-211 FR-1/FR-2): Handles alternate YAML representations", func() {
		It("should preserve a non-Secret flow-style object", func() {
			input := "apiVersion: v1\nkind: Node\nspec: {taints: [{effect: NoSchedule, key: maintenance, value: scheduled}]}\n"

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(input))
		})

		It("should redact quoted Secret keys and values", func() {
			input := "\"apiVersion\": v1\n\"kind\": Secret\n\"data\":\n  \"key\": c2VjcmV0LWtleQ==\n"

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("c2VjcmV0LWtleQ=="))
			Expect(result).To(ContainSubstring(sharedsanitization.RedactedPlaceholder))
		})

		It("should redact anchored Secret values without exposing aliases", func() {
			input := "apiVersion: v1\nkind: Secret\ndata:\n  key: &secret c2VjcmV0LWtleQ==\n  copy: *secret\n"

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("c2VjcmV0LWtleQ=="))
			Expect(result).To(ContainSubstring(sharedsanitization.RedactedPlaceholder))
		})

		It("should preserve Secret-like data on an unknown custom resource", func() {
			input := "apiVersion: example.io/v1\nkind: Widget\ndata:\n  key: maintenance\n"

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(input))
		})

		It("should redact Secret and preserve ConfigMap items in a JSON generic List", func() {
			input := `{"apiVersion":"v1","kind":"List","items":[{"apiVersion":"v1","kind":"ConfigMap","data":{"key":"maintenance"}},{"apiVersion":"v1","kind":"Secret","data":{"key":"c2VjcmV0LWtleQ=="}}]}`

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("maintenance"))
			Expect(result).NotTo(ContainSubstring("c2VjcmV0LWtleQ=="))
			Expect(result).To(ContainSubstring(sharedsanitization.RedactedPlaceholder))
		})
	})

})
