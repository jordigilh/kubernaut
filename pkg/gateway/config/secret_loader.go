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
//	dsConfig, err := loader.LoadDataStorageConfig(ctx, "kubernaut-system", "gateway-ds-secret")
func NewSecretLoader(k8sClient client.Client) *SecretLoader {
	return &SecretLoader{
		k8sClient: k8sClient,
	}
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
	// REFACTOR: Extract common secret fetching logic
	secret, err := s.getSecret(ctx, namespace, secretName)
	if err != nil {
		return nil, err
	}

	// REFACTOR: Extract field extraction with helper
	url, err := s.extractRequiredField(secret, "datastorage-url", namespace, secretName)
	if err != nil {
		return nil, err
	}

	apiKey, err := s.extractRequiredField(secret, "datastorage-api-key", namespace, secretName)
	if err != nil {
		return nil, err
	}

	timeoutStr, err := s.extractRequiredField(secret, "datastorage-timeout", namespace, secretName)
	if err != nil {
		return nil, err
	}

	// Parse timeout duration
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid datastorage-timeout format in secret %s/%s: %w", namespace, secretName, err)
	}

	return &DataStorageConfig{
		URL:     url,
		APIKey:  apiKey,
		Timeout: timeout,
	}, nil
}

// ========================================
// REFACTOR: Private helper methods
// ========================================

// getSecret fetches a Secret from the Kubernetes API.
// REFACTOR: Eliminates duplication in secret fetching logic.
func (s *SecretLoader) getSecret(ctx context.Context, namespace, secretName string) (*corev1.Secret, error) {
	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}

	if err := s.k8sClient.Get(ctx, secretKey, &secret); err != nil {
		return nil, fmt.Errorf("secret not found: %s/%s: %w", namespace, secretName, err)
	}

	return &secret, nil
}

// extractRequiredField extracts a required field from a Secret's data.
// REFACTOR: Eliminates duplication in field extraction logic.
// Returns the field value as string, or error if field is missing.
func (s *SecretLoader) extractRequiredField(secret *corev1.Secret, fieldName, namespace, secretName string) (string, error) {
	value, ok := secret.Data[fieldName]
	if !ok {
		return "", fmt.Errorf("%s required field missing in secret %s/%s", fieldName, namespace, secretName)
	}
	return string(value), nil
}
