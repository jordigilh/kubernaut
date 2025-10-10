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

package testutil

import (
	"time"
)

// InternalTestDataFactory provides standardized test data creation for internal tests
type InternalTestDataFactory struct{}

// NewInternalTestDataFactory creates a new test data factory for internal tests
func NewInternalTestDataFactory() *InternalTestDataFactory {
	return &InternalTestDataFactory{}
}

// CreateValidConfigYAML creates a valid configuration YAML string
func (f *InternalTestDataFactory) CreateValidConfigYAML() string {
	return `
server:
  webhook_port: "8080"
  metrics_port: "9090"

slm:
  endpoint: "http://localhost:11434"
  model: "llama2"
  timeout: "30s"
  retry_count: 3
  provider: "localai"
  temperature: 0.3
  max_tokens: 500

kubernetes:
  context: "test-context"
  namespace: "default"

actions:
  dry_run: false
  max_concurrent: 5
  cooldown_period: "5m"

filters:
  - name: "production-filter"
    conditions:
      namespace:
        - "production"
        - "staging"
      severity:
        - "critical"
        - "warning"

logging:
  level: "info"
  format: "json"

webhook:
  port: "8080"
  path: "/webhook"
`
}

// CreateMinimalConfigYAML creates a minimal configuration YAML string
func (f *InternalTestDataFactory) CreateMinimalConfigYAML() string {
	return `
server:
  webhook_port: "8080"

slm:
  endpoint: "http://localhost:11434"
  model: "test-model"

kubernetes:
  namespace: "default"
`
}

// CreateInvalidConfigYAML creates an invalid configuration YAML string
func (f *InternalTestDataFactory) CreateInvalidConfigYAML() string {
	return `
invalid: yaml: content
  - this is not
    valid yaml syntax
`
}

// CreateEmptyConfigYAML creates an empty configuration YAML string
func (f *InternalTestDataFactory) CreateEmptyConfigYAML() string {
	return ""
}

// CreateDatabaseEnvVars creates database environment variables
func (f *InternalTestDataFactory) CreateDatabaseEnvVars() map[string]string {
	return map[string]string{
		"DB_HOST":     "testhost",
		"DB_PORT":     "3306",
		"DB_USER":     "testuser",
		"DB_PASSWORD": "testpass",
		"DB_NAME":     "testdb",
		"DB_SSL_MODE": "require",
	}
}

// CreateDefaultDatabaseEnvVars creates default database environment variables
func (f *InternalTestDataFactory) CreateDefaultDatabaseEnvVars() map[string]string {
	return map[string]string{
		"DB_HOST":     "localhost",
		"DB_PORT":     "5432",
		"DB_USER":     "slm_user",
		"DB_PASSWORD": "",
		"DB_NAME":     "action_history",
		"DB_SSL_MODE": "disable",
	}
}

// CreateInvalidDatabaseEnvVars creates invalid database environment variables
func (f *InternalTestDataFactory) CreateInvalidDatabaseEnvVars() map[string]string {
	return map[string]string{
		"DB_HOST":     "testhost",
		"DB_PORT":     "invalid-port",
		"DB_USER":     "testuser",
		"DB_PASSWORD": "testpass",
		"DB_NAME":     "testdb",
		"DB_SSL_MODE": "require",
	}
}

// CreateValidationTestData creates test data for validation tests
func (f *InternalTestDataFactory) CreateValidationTestData() map[string]interface{} {
	return map[string]interface{}{
		"name":        "test-resource",
		"namespace":   "test-namespace",
		"labels":      map[string]string{"app": "test-app"},
		"annotations": map[string]string{"description": "test resource"},
		"spec": map[string]interface{}{
			"replicas": 3,
			"selector": map[string]interface{}{
				"matchLabels": map[string]string{"app": "test-app"},
			},
		},
	}
}

// CreateInvalidValidationTestData creates invalid test data for validation tests
func (f *InternalTestDataFactory) CreateInvalidValidationTestData() map[string]interface{} {
	return map[string]interface{}{
		"name":      "", // Invalid: empty name
		"namespace": "test-namespace",
		"spec": map[string]interface{}{
			"replicas": -1, // Invalid: negative replicas
		},
	}
}

// CreateKubernetesResourceNames creates standard Kubernetes resource names
func (f *InternalTestDataFactory) CreateKubernetesResourceNames() []string {
	return []string{
		"valid-name",
		"valid-name-123",
		"valid-name-with-dashes",
		"123-valid-name",
	}
}

// CreateInvalidKubernetesResourceNames creates invalid Kubernetes resource names
func (f *InternalTestDataFactory) CreateInvalidKubernetesResourceNames() []string {
	return []string{
		"",              // Empty name
		"invalid_name",  // Underscores not allowed
		"Invalid-Name",  // Capital letters not allowed
		"invalid.name",  // Dots not allowed
		"invalid name",  // Spaces not allowed
		"-invalid-name", // Cannot start with dash
		"invalid-name-", // Cannot end with dash
		"this-name-is-way-too-long-to-be-a-valid-kubernetes-resource-name-and-exceeds-limits",
	}
}

// CreateValidNamespaces creates valid namespace names
func (f *InternalTestDataFactory) CreateValidNamespaces() []string {
	return []string{
		"default",
		"kube-system",
		"test-namespace",
		"production",
		"staging",
		"development",
	}
}

// CreateInvalidNamespaces creates invalid namespace names
func (f *InternalTestDataFactory) CreateInvalidNamespaces() []string {
	return []string{
		"",             // Empty namespace
		"Invalid-Name", // Capital letters not allowed
		"invalid_name", // Underscores not allowed
		"kube-system-", // Cannot end with dash
		"-kube-system", // Cannot start with dash
	}
}

// CreateValidLabels creates valid Kubernetes labels
func (f *InternalTestDataFactory) CreateValidLabels() map[string]string {
	return map[string]string{
		"app":         "test-app",
		"version":     "v1.0.0",
		"environment": "test",
		"team":        "platform",
		"component":   "backend",
	}
}

// CreateInvalidLabels creates invalid Kubernetes labels
func (f *InternalTestDataFactory) CreateInvalidLabels() map[string]string {
	return map[string]string{
		"":            "empty-key",
		"invalid key": "spaces-in-key",
		"app":         "", // Empty value is actually valid
		"toolongkey":  "this-value-is-way-too-long-to-be-a-valid-kubernetes-label-value-and-exceeds-the-maximum-allowed-length",
	}
}

// CreateTestTimestamps creates various timestamps for testing
func (f *InternalTestDataFactory) CreateTestTimestamps() map[string]time.Time {
	now := time.Now()
	return map[string]time.Time{
		"now":           now,
		"one_hour_ago":  now.Add(-time.Hour),
		"one_day_ago":   now.Add(-24 * time.Hour),
		"one_week_ago":  now.Add(-7 * 24 * time.Hour),
		"one_month_ago": now.Add(-30 * 24 * time.Hour),
		"future":        now.Add(time.Hour),
	}
}

// CreateTestDurations creates various durations for testing
func (f *InternalTestDataFactory) CreateTestDurations() map[string]time.Duration {
	return map[string]time.Duration{
		"short":     1 * time.Second,
		"medium":    30 * time.Second,
		"long":      5 * time.Minute,
		"very_long": 30 * time.Minute,
		"timeout":   10 * time.Second,
	}
}

// CreateConnectionStrings creates database connection strings for testing
func (f *InternalTestDataFactory) CreateConnectionStrings() map[string]string {
	return map[string]string{
		"local":    "host=localhost port=5432 user=slm_user dbname=action_history sslmode=disable",
		"remote":   "host=remote.db.com port=5432 user=app_user password=secret dbname=production sslmode=require",
		"with_ssl": "host=secure.db.com port=5432 user=secure_user password=secret dbname=secure_db sslmode=require",
		"minimal":  "host=localhost dbname=test",
	}
}

// CreateErrorMessages creates standard error messages for testing
func (f *InternalTestDataFactory) CreateErrorMessages() map[string]string {
	return map[string]string{
		"validation":   "validation failed: field 'name' is required",
		"not_found":    "resource not found",
		"unauthorized": "access denied: insufficient permissions",
		"bad_request":  "bad request: invalid input format",
		"internal":     "internal server error: database connection failed",
		"timeout":      "operation timed out after 30 seconds",
		"network":      "network error: connection refused",
		"config":       "configuration error: invalid YAML format",
	}
}

// CreateValidationRules creates validation rule test data
func (f *InternalTestDataFactory) CreateValidationRules() map[string]interface{} {
	return map[string]interface{}{
		"required_fields": []string{"name", "namespace", "spec"},
		"min_length": map[string]int{
			"name":      1,
			"namespace": 1,
		},
		"max_length": map[string]int{
			"name":      253,
			"namespace": 253,
		},
		"patterns": map[string]string{
			"name":      "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
			"namespace": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
		},
	}
}
