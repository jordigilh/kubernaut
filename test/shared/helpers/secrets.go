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

package helpers

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ========================================
// KUBERNETES SECRET TEST HELPERS
// ========================================
//
// SCOPE: Reusable test utilities for creating K8s Secrets in integration tests
// AUTHORITY: Defined in GW_INTEGRATION_TEST_PLAN_V1.0.md Phase 4
// USED BY: Gateway, and potentially other services needing secret loading tests

// CreateRedisSecret creates a test Kubernetes Secret for Redis configuration
//
// Parameters:
//   - name: Secret name (e.g., "gateway-redis-secret")
//   - namespace: Target namespace (e.g., "gateway-test")
//   - password: Redis password (will be stored in "redis-password" key)
//   - host: Redis host (will be stored in "redis-host" key)
//   - port: Redis port (will be stored in "redis-port" key)
//
// Returns:
//   - *corev1.Secret ready to be created in K8s cluster
//
// Example:
//
//	secret := helpers.CreateRedisSecret("test-secret", "default", "pass123", "redis.svc.local", "6379")
//	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
func CreateRedisSecret(name, namespace, password, host, port string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"redis-password": []byte(password),
			"redis-host":     []byte(host),
			"redis-port":     []byte(port),
		},
	}
}

// CreateDataStorageSecret creates a test Kubernetes Secret for DataStorage configuration
//
// Parameters:
//   - name: Secret name (e.g., "gateway-datastorage-secret")
//   - namespace: Target namespace (e.g., "gateway-test")
//   - url: DataStorage API URL (will be stored in "datastorage-url" key)
//   - apiKey: DataStorage API key (will be stored in "datastorage-api-key" key)
//   - timeout: Request timeout (will be stored in "datastorage-timeout" key, e.g., "30s")
//
// Returns:
//   - *corev1.Secret ready to be created in K8s cluster
//
// Example:
//
//	secret := helpers.CreateDataStorageSecret("ds-secret", "default",
//	    "http://datastorage:8080", "api-key-123", "30s")
//	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
func CreateDataStorageSecret(name, namespace, url, apiKey, timeout string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"datastorage-url":     []byte(url),
			"datastorage-api-key": []byte(apiKey),
			"datastorage-timeout": []byte(timeout),
		},
	}
}

// CreateIncompleteRedisSecret creates a test Kubernetes Secret with missing required fields
// Useful for testing validation error handling.
//
// Parameters:
//   - name: Secret name
//   - namespace: Target namespace
//   - host: Redis host (included)
//   - port: Redis port (included)
//
// Returns:
//   - *corev1.Secret with MISSING redis-password field
//
// Example:
//
//	secret := helpers.CreateIncompleteRedisSecret("incomplete-secret", "default",
//	    "redis.svc.local", "6379")
//	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
//	// Expect validation error when loading this secret
func CreateIncompleteRedisSecret(name, namespace, host, port string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"redis-host": []byte(host),
			"redis-port": []byte(port),
			// Missing: redis-password - intentionally omitted for validation testing
		},
	}
}

// CreateGenericSecret creates a test Kubernetes Secret with custom key-value pairs
// Useful for testing scenarios not covered by the specific helpers above.
//
// Parameters:
//   - name: Secret name
//   - namespace: Target namespace
//   - data: Map of key-value pairs to store in the secret
//
// Returns:
//   - *corev1.Secret with custom data
//
// Example:
//
//	secret := helpers.CreateGenericSecret("custom-secret", "default", map[string]string{
//	    "username": "admin",
//	    "password": "secret123",
//	    "endpoint": "https://api.example.com",
//	})
//	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
func CreateGenericSecret(name, namespace string, data map[string]string) *corev1.Secret {
	secretData := make(map[string][]byte)
	for key, value := range data {
		secretData[key] = []byte(value)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}
}
