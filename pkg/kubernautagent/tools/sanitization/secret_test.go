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
				},
			}
			input, err := json.Marshal(secret)
			Expect(err).NotTo(HaveOccurred())

			result, sanitizeErr := stage.Sanitize(ctx, string(input))
			Expect(sanitizeErr).NotTo(HaveOccurred())

			Expect(result).NotTo(ContainSubstring("YWRtaW4="))
			Expect(result).NotTo(ContainSubstring("czNjcjN0"))
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
})
