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

package config

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ========================================
// KUBERNETES SECRET LOADER
// 📋 Business Requirement: BR-GATEWAY-052
// Authority: GW_INTEGRATION_TEST_PLAN_V1.0.md Phase 4
// ========================================
//
// SCOPE: Load Gateway secrets from Kubernetes Secret API
// PATTERN: Direct K8s client access (integration test pattern)
// SECURITY: Secrets must not be logged or exposed in error messages
//
// DESIGN DECISION: K8s API vs File-based
// - Gateway uses K8s client API for secret loading (this file)
// - DataStorage uses file-based loading (pkg/datastorage/config/config.go)
// - Both patterns are valid; choice depends on deployment model

// SecretLoader loads secrets from Kubernetes Secret API.
// BR-GATEWAY-052: Gateway must load secrets from K8s Secrets, not environment variables.
type SecretLoader struct {
	k8sClient client.Client
}

// NewSecretLoader creates a new SecretLoader with K8s client.
//
// Parameters:
//   - k8sClient: Kubernetes client for reading Secrets
//
// Returns:
//   - *SecretLoader ready to load secrets
//
// Example:
//
//	loader := config.NewSecretLoader(k8sClient)
//	redisConfig, err := loader.LoadRedisConfig(ctx, "kubernaut-system", "gateway-redis-secret")
func NewSecretLoader(k8sClient client.Client) *SecretLoader {
	return &SecretLoader{
		k8sClient: k8sClient,
	}
}

// RedisConfig contains Redis connection configuration loaded from K8s Secret.
// BR-GATEWAY-052: Credentials loaded from K8s Secrets.
type RedisConfig struct {
	Password string // Redis password (from redis-password key)
	Host     string // Redis host (from redis-host key)
	Port     string // Redis port (from redis-port key)
}

// String returns a safe string representation with redacted password.
// BR-GATEWAY-052: Secrets must not appear in logs.
func (r *RedisConfig) String() string {
	return fmt.Sprintf("RedisConfig{Host: %s, Port: %s, Password: [REDACTED]}", r.Host, r.Port)
}

// DataStorageConfig contains DataStorage API configuration loaded from K8s Secret.
// BR-GATEWAY-052: API credentials loaded from K8s Secrets.
type DataStorageConfig struct {
	URL     string        // DataStorage API URL (from datastorage-url key)
	APIKey  string        // DataStorage API key (from datastorage-api-key key)
	Timeout time.Duration // Request timeout (from datastorage-timeout key)
}

// String returns a safe string representation with redacted API key.
// BR-GATEWAY-052: Secrets must not appear in logs.
func (d *DataStorageConfig) String() string {
	return fmt.Sprintf("DataStorageConfig{URL: %s, Timeout: %s, APIKey: [REDACTED]}", d.URL, d.Timeout)
}

// LoadRedisConfig loads Redis configuration from a Kubernetes Secret.
// BR-GATEWAY-052: Load Redis credentials from K8s Secret, not environment variables.
//
// Parameters:
//   - ctx: Context for K8s API call
//   - namespace: Namespace where Secret exists
//   - secretName: Name of the Secret
//
// Returns:
//   - *RedisConfig with credentials loaded
//   - error if Secret not found, missing required fields, or validation fails
//
// Required Secret Keys:
//   - redis-password: Redis password (required)
//   - redis-host: Redis host (required)
//   - redis-port: Redis port (required)
//
// Example:
//
//	redisConfig, err := loader.LoadRedisConfig(ctx, "kubernaut-system", "gateway-redis-secret")
//	if err != nil {
//	    return fmt.Errorf("failed to load Redis secret: %w", err)
//	}
//	// Use redisConfig.Password, redisConfig.Host, redisConfig.Port
func (s *SecretLoader) LoadRedisConfig(ctx context.Context, namespace, secretName string) (*RedisConfig, error) {
	// Fetch Secret from K8s API
	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}

	if err := s.k8sClient.Get(ctx, secretKey, &secret); err != nil {
		return nil, fmt.Errorf("secret not found: %s/%s: %w", namespace, secretName, err)
	}

	// Extract required fields
	password, ok := secret.Data["redis-password"]
	if !ok {
		return nil, fmt.Errorf("redis-password required field missing in secret %s/%s", namespace, secretName)
	}

	host, ok := secret.Data["redis-host"]
	if !ok {
		return nil, fmt.Errorf("redis-host required field missing in secret %s/%s", namespace, secretName)
	}

	port, ok := secret.Data["redis-port"]
	if !ok {
		return nil, fmt.Errorf("redis-port required field missing in secret %s/%s", namespace, secretName)
	}

	return &RedisConfig{
		Password: string(password),
		Host:     string(host),
		Port:     string(port),
	}, nil
}

// LoadDataStorageConfig loads DataStorage API configuration from a Kubernetes Secret.
// BR-GATEWAY-052: Load DataStorage credentials from K8s Secret, not environment variables.
//
// Parameters:
//   - ctx: Context for K8s API call
//   - namespace: Namespace where Secret exists
//   - secretName: Name of the Secret
//
// Returns:
//   - *DataStorageConfig with API credentials loaded
//   - error if Secret not found, missing required fields, or validation fails
//
// Required Secret Keys:
//   - datastorage-url: DataStorage API URL (required)
//   - datastorage-api-key: DataStorage API key (required)
//   - datastorage-timeout: Request timeout duration (required, e.g., "30s")
//
// Example:
//
//	dsConfig, err := loader.LoadDataStorageConfig(ctx, "kubernaut-system", "gateway-ds-secret")
//	if err != nil {
//	    return fmt.Errorf("failed to load DataStorage secret: %w", err)
//	}
//	// Use dsConfig.URL, dsConfig.APIKey, dsConfig.Timeout
func (s *SecretLoader) LoadDataStorageConfig(ctx context.Context, namespace, secretName string) (*DataStorageConfig, error) {
	// Fetch Secret from K8s API
	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}

	if err := s.k8sClient.Get(ctx, secretKey, &secret); err != nil {
		return nil, fmt.Errorf("secret not found: %s/%s: %w", namespace, secretName, err)
	}

	// Extract required fields
	url, ok := secret.Data["datastorage-url"]
	if !ok {
		return nil, fmt.Errorf("datastorage-url required field missing in secret %s/%s", namespace, secretName)
	}

	apiKey, ok := secret.Data["datastorage-api-key"]
	if !ok {
		return nil, fmt.Errorf("datastorage-api-key required field missing in secret %s/%s", namespace, secretName)
	}

	timeoutStr, ok := secret.Data["datastorage-timeout"]
	if !ok {
		return nil, fmt.Errorf("datastorage-timeout required field missing in secret %s/%s", namespace, secretName)
	}

	// Parse timeout duration
	timeout, err := time.ParseDuration(string(timeoutStr))
	if err != nil {
		return nil, fmt.Errorf("invalid datastorage-timeout format in secret %s/%s: %w", namespace, secretName, err)
	}

	return &DataStorageConfig{
		URL:     string(url),
		APIKey:  string(apiKey),
		Timeout: timeout,
	}, nil
}
