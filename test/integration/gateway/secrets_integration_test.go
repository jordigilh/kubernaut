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

package gateway

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BR-GATEWAY-052: Secret Management Integration", func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Use unique namespace per test run (DD-TEST-001: Parallel execution pattern)
		namespace = helpers.CreateTestNamespace(ctx, k8sClient, "gw-secrets")
	})

	AfterEach(func() {
		// Simple cleanup - namespace is unique per test
		helpers.DeleteTestNamespace(ctx, k8sClient, namespace)
	})

	// ========================================
	// GW-INT-SEC-001: Load Redis Secret from K8s
	// ========================================
	It("GW-INT-SEC-001: should load Redis password from Kubernetes Secret", func() {
		// Given: Kubernetes Secret with Redis credentials
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-redis-secret",
				Namespace: namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"redis-password": []byte("test-redis-password-123"),
				"redis-host":     []byte("redis.default.svc.cluster.local"),
				"redis-port":     []byte("6379"),
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		// Wait for secret to be available
		Eventually(func() error {
			var retrieved corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &retrieved)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		// When: Config loader reads secret
		secretLoader := config.NewSecretLoader(k8sClient)
		redisConfig, err := secretLoader.LoadRedisConfig(ctx, namespace, "gateway-redis-secret")

		// Then: Credentials loaded correctly
		Expect(err).ToNot(HaveOccurred(), "BR-GATEWAY-052: Must load secrets from K8s")
		Expect(redisConfig.Password).To(Equal("test-redis-password-123"))
		Expect(redisConfig.Host).To(Equal("redis.default.svc.cluster.local"))
		Expect(redisConfig.Port).To(Equal("6379"))

		GinkgoWriter.Printf("✅ GW-INT-SEC-001: Secret loaded: host=%s, password=[REDACTED]\n",
			redisConfig.Host)
	})

	// ========================================
	// GW-INT-SEC-002: Load DataStorage Secret from K8s
	// ========================================
	It("GW-INT-SEC-002: should load DataStorage credentials from Kubernetes Secret", func() {
		// Given: Kubernetes Secret with DataStorage API credentials
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-datastorage-secret",
				Namespace: namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"datastorage-url":     []byte("http://datastorage.kubernaut-system.svc.cluster.local:8080"),
				"datastorage-api-key": []byte("test-api-key-abc123"),
				"datastorage-timeout": []byte("30s"),
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		// Wait for secret to be available
		Eventually(func() error {
			var retrieved corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &retrieved)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		// When: Config loader reads secret
		secretLoader := config.NewSecretLoader(k8sClient)
		dsConfig, err := secretLoader.LoadDataStorageConfig(ctx, namespace, "gateway-datastorage-secret")

		// Then: Credentials loaded correctly
		Expect(err).ToNot(HaveOccurred(), "BR-GATEWAY-052: Must load DataStorage credentials from K8s Secret")
		Expect(dsConfig.URL).To(Equal("http://datastorage.kubernaut-system.svc.cluster.local:8080"))
		Expect(dsConfig.APIKey).To(Equal("test-api-key-abc123"))
		Expect(dsConfig.Timeout).To(Equal(30 * time.Second))

		GinkgoWriter.Printf("✅ GW-INT-SEC-002: DataStorage secret loaded: url=%s, api-key=[REDACTED]\n",
			dsConfig.URL)
	})

	// ========================================
	// GW-INT-SEC-003: Missing Secret Error Handling
	// ========================================
	It("GW-INT-SEC-003: should return error when secret does not exist", func() {
		// Given: No secret exists
		nonExistentSecretName := "gateway-missing-secret"

		// When: Config loader attempts to read non-existent secret
		secretLoader := config.NewSecretLoader(k8sClient)
		_, err := secretLoader.LoadRedisConfig(ctx, namespace, nonExistentSecretName)

		// Then: Error returned with clear message
		Expect(err).To(HaveOccurred(), "BR-GATEWAY-052: Must fail gracefully when secret missing")
		Expect(err.Error()).To(ContainSubstring("secret not found"))
		Expect(err.Error()).To(ContainSubstring(nonExistentSecretName))

		GinkgoWriter.Printf("✅ GW-INT-SEC-003: Correctly failed with missing secret: %v\n", err)
	})

	// ========================================
	// GW-INT-SEC-004: Incomplete Secret Validation
	// ========================================
	It("GW-INT-SEC-004: should return error when secret is missing required field", func() {
		// Given: Secret missing required 'redis-password' field
		incompleteSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-incomplete-secret",
				Namespace: namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"redis-host": []byte("redis.default.svc.cluster.local"),
				"redis-port": []byte("6379"),
				// Missing: redis-password
			},
		}
		Expect(k8sClient.Create(ctx, incompleteSecret)).To(Succeed())

		// Wait for secret to be available
		Eventually(func() error {
			var retrieved corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(incompleteSecret), &retrieved)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		// When: Config loader reads incomplete secret
		secretLoader := config.NewSecretLoader(k8sClient)
		_, err := secretLoader.LoadRedisConfig(ctx, namespace, "gateway-incomplete-secret")

		// Then: Validation error returned
		Expect(err).To(HaveOccurred(), "BR-GATEWAY-052: Must validate required secret fields")
		Expect(err.Error()).To(ContainSubstring("redis-password"))
		Expect(err.Error()).To(ContainSubstring("required field missing"))

		GinkgoWriter.Printf("✅ GW-INT-SEC-004: Correctly detected missing field: %v\n", err)
	})

	// ========================================
	// GW-INT-SEC-005: Secret Rotation Without Restart
	// ========================================
	It("GW-INT-SEC-005: should handle secret update without service restart", func() {
		// Given: Initial secret with credentials
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-redis-secret-rotation",
				Namespace: namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"redis-password": []byte("initial-password"),
				"redis-host":     []byte("redis.default.svc.cluster.local"),
				"redis-port":     []byte("6379"),
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		// Wait for secret to be available
		Eventually(func() error {
			var retrieved corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &retrieved)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		// When: Load initial config
		secretLoader := config.NewSecretLoader(k8sClient)
		initialConfig, err := secretLoader.LoadRedisConfig(ctx, namespace, "gateway-redis-secret-rotation")
		Expect(err).ToNot(HaveOccurred())
		Expect(initialConfig.Password).To(Equal("initial-password"))

		// And: Secret is rotated (updated)
		var updatedSecret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &updatedSecret)).To(Succeed())
		updatedSecret.Data["redis-password"] = []byte("rotated-password-456")
		Expect(k8sClient.Update(ctx, &updatedSecret)).To(Succeed())

		// Wait for update to propagate
		Eventually(func() string {
			var retrieved corev1.Secret
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &retrieved)
			return string(retrieved.Data["redis-password"])
		}, 10*time.Second, 500*time.Millisecond).Should(Equal("rotated-password-456"))

		// Then: Reloading config returns new credentials
		rotatedConfig, err := secretLoader.LoadRedisConfig(ctx, namespace, "gateway-redis-secret-rotation")
		Expect(err).ToNot(HaveOccurred(), "BR-GATEWAY-052: Must support secret rotation")
		Expect(rotatedConfig.Password).To(Equal("rotated-password-456"))

		GinkgoWriter.Printf("✅ GW-INT-SEC-005: Secret rotation successful: initial=[REDACTED] → rotated=[REDACTED]\n")
	})

	// ========================================
	// GW-INT-SEC-006: Secret Redaction in Logs
	// ========================================
	It("GW-INT-SEC-006: should not expose secrets in logs or error messages", func() {
		// Given: Secret with sensitive data
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-sensitive-secret",
				Namespace: namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"redis-password": []byte("super-secret-password-xyz"),
				"redis-host":     []byte("redis.default.svc.cluster.local"),
				"redis-port":     []byte("6379"),
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		// Wait for secret to be available
		Eventually(func() error {
			var retrieved corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &retrieved)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		// When: Config loaded and logged
		secretLoader := config.NewSecretLoader(k8sClient)
		redisConfig, err := secretLoader.LoadRedisConfig(ctx, namespace, "gateway-sensitive-secret")
		Expect(err).ToNot(HaveOccurred())

		// Then: String representation does not contain password
		configString := redisConfig.String() // Should use custom String() method
		Expect(configString).ToNot(ContainSubstring("super-secret-password-xyz"),
			"BR-GATEWAY-052: Secrets must not appear in logs")
		Expect(configString).To(ContainSubstring("[REDACTED]"),
			"BR-GATEWAY-052: Secrets must be redacted in output")

		GinkgoWriter.Printf("✅ GW-INT-SEC-006: Secret properly redacted in output: %s\n", configString)
	})
})
