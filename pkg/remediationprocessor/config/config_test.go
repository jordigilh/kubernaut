// Package config_test provides unit tests for remediationprocessor configuration.
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jordigilh/kubernaut/pkg/remediationprocessor/config"
)

// TestLoadConfig tests loading configuration from YAML file.
func TestLoadConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
namespace: test-namespace
metrics_address: ":9090"
health_address: ":9091"
leader_election: true
log_level: debug
max_concurrency: 20

kubernetes:
  qps: 50.0
  burst: 75

data_storage:
  postgres_host: localhost
  postgres_port: 5432
  postgres_user: test_user
  postgres_password: test_pass
  postgres_database: test_db
  ssl_mode: require
  max_connections: 25
  max_idle_conns: 5

context:
  endpoint: http://context-api:8080
  timeout: 30
  max_retries: 3
  retry_backoff_ms: 100

classification:
  semantic_threshold: 0.85
  time_window_minutes: 60
  similarity_engine: cosine
  batch_size: 100
`

	require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0644))

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify common fields
	assert.Equal(t, "test-namespace", cfg.Namespace)
	assert.Equal(t, ":9090", cfg.MetricsAddress)
	assert.Equal(t, ":9091", cfg.HealthAddress)
	assert.True(t, cfg.LeaderElection)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 20, cfg.MaxConcurrency)

	// Verify Kubernetes config
	assert.Equal(t, float32(50.0), cfg.Kubernetes.QPS)
	assert.Equal(t, 75, cfg.Kubernetes.Burst)

	// Verify DataStorage config
	assert.Equal(t, "localhost", cfg.DataStorage.PostgresHost)
	assert.Equal(t, 5432, cfg.DataStorage.PostgresPort)
	assert.Equal(t, "test_user", cfg.DataStorage.PostgresUser)
	assert.Equal(t, "test_pass", cfg.DataStorage.PostgresPassword)
	assert.Equal(t, "test_db", cfg.DataStorage.PostgresDatabase)
	assert.Equal(t, "require", cfg.DataStorage.SSLMode)
	assert.Equal(t, 25, cfg.DataStorage.MaxConnections)
	assert.Equal(t, 5, cfg.DataStorage.MaxIdleConns)

	// Verify Context API config
	assert.Equal(t, "http://context-api:8080", cfg.Context.Endpoint)
	assert.Equal(t, 30, cfg.Context.Timeout)
	assert.Equal(t, 3, cfg.Context.MaxRetries)
	assert.Equal(t, 100, cfg.Context.RetryBackoffMs)

	// Verify Classification config
	assert.Equal(t, 0.85, cfg.Classification.SemanticThreshold)
	assert.Equal(t, 60, cfg.Classification.TimeWindowMinutes)
	assert.Equal(t, "cosine", cfg.Classification.SimilarityEngine)
	assert.Equal(t, 100, cfg.Classification.BatchSize)
}

// TestLoadConfigInvalidPath tests loading config from non-existent file.
func TestLoadConfigInvalidPath(t *testing.T) {
	cfg, err := config.LoadConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to read config file")
}

// TestLoadConfigInvalidYAML tests loading invalid YAML.
func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	invalidYAML := `
namespace: test
invalid yaml here: [
`

	require.NoError(t, os.WriteFile(configPath, []byte(invalidYAML), 0644))

	cfg, err := config.LoadConfig(configPath)
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to parse config file")
}

// TestValidateConfig tests configuration validation.
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: `
namespace: test-namespace
metrics_address: ":8080"
health_address: ":8081"
log_level: info
max_concurrency: 10
kubernetes:
  qps: 20.0
  burst: 30
data_storage:
  postgres_host: localhost
  postgres_port: 5432
  postgres_user: test_user
  postgres_database: test_db
context:
  endpoint: http://context-api:8080
  timeout: 30
classification:
  semantic_threshold: 0.85
  time_window_minutes: 60
  batch_size: 100
`,
			wantErr: false,
		},
		{
			name: "missing postgres host",
			config: `
namespace: test-namespace
metrics_address: ":8080"
health_address: ":8081"
log_level: info
max_concurrency: 10
kubernetes:
  qps: 20.0
  burst: 30
data_storage:
  postgres_host: ""
  postgres_port: 5432
  postgres_user: test_user
  postgres_database: test_db
context:
  endpoint: http://context-api:8080
classification:
  semantic_threshold: 0.85
`,
			wantErr: true,
			errMsg:  "data_storage.postgres_host is required",
		},
		{
			name: "missing context endpoint",
			config: `
namespace: test-namespace
metrics_address: ":8080"
health_address: ":8081"
log_level: info
max_concurrency: 10
kubernetes:
  qps: 20.0
  burst: 30
data_storage:
  postgres_host: localhost
  postgres_port: 5432
  postgres_user: test_user
  postgres_database: test_db
context:
  endpoint: ""
classification:
  semantic_threshold: 0.85
`,
			wantErr: true,
			errMsg:  "context.endpoint is required",
		},
		{
			name: "invalid semantic threshold",
			config: `
namespace: test-namespace
metrics_address: ":8080"
health_address: ":8081"
log_level: info
max_concurrency: 10
kubernetes:
  qps: 20.0
  burst: 30
data_storage:
  postgres_host: localhost
  postgres_port: 5432
  postgres_user: test_user
  postgres_password: test_pass
  postgres_database: test_db
context:
  endpoint: http://context-api:8080
  timeout: 30
classification:
  semantic_threshold: 1.5
  time_window_minutes: 60
  batch_size: 100
`,
			wantErr: true,
			errMsg:  "classification.semantic_threshold must be between 0 and 1",
		},
		{
			name: "invalid time window",
			config: `
namespace: test-namespace
metrics_address: ":8080"
health_address: ":8081"
log_level: info
max_concurrency: 10
kubernetes:
  qps: 20.0
  burst: 30
data_storage:
  postgres_host: localhost
  postgres_port: 5432
  postgres_user: test_user
  postgres_password: test_pass
  postgres_database: test_db
context:
  endpoint: http://context-api:8080
  timeout: 30
classification:
  semantic_threshold: 0.85
  time_window_minutes: -1
  batch_size: 100
`,
			wantErr: true,
			errMsg:  "classification.time_window_minutes must be greater than 0",
		},
		{
			name: "invalid batch size",
			config: `
namespace: test-namespace
metrics_address: ":8080"
health_address: ":8081"
log_level: info
max_concurrency: 10
kubernetes:
  qps: 20.0
  burst: 30
data_storage:
  postgres_host: localhost
  postgres_port: 5432
  postgres_user: test_user
  postgres_password: test_pass
  postgres_database: test_db
context:
  endpoint: http://context-api:8080
  timeout: 30
classification:
  semantic_threshold: 0.85
  time_window_minutes: 60
  batch_size: -1
`,
			wantErr: true,
			errMsg:  "classification.batch_size must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(tt.config), 0644))

			cfg, err := config.LoadConfig(configPath)
			require.NoError(t, err)

			err = cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestLoadFromEnv tests environment variable overrides.
func TestLoadFromEnv(t *testing.T) {
	// Create base config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	baseConfig := `
namespace: base-namespace
metrics_address: ":8080"
health_address: ":8081"
log_level: info
max_concurrency: 10
kubernetes:
  qps: 20.0
  burst: 30
data_storage:
  postgres_host: base-host
  postgres_port: 5432
  postgres_user: base_user
  postgres_database: base_db
context:
  endpoint: http://base-context:8080
  timeout: 30
classification:
  semantic_threshold: 0.85
  time_window_minutes: 60
`

	require.NoError(t, os.WriteFile(configPath, []byte(baseConfig), 0644))

	// Set environment variables
	os.Setenv("CONTROLLER_NAMESPACE", "env-namespace")
	os.Setenv("METRICS_ADDRESS", ":9090")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("MAX_CONCURRENCY", "50")
	os.Setenv("KUBERNETES_QPS", "100.0")
	os.Setenv("KUBERNETES_BURST", "150")
	os.Setenv("POSTGRES_HOST", "env-postgres-host")
	os.Setenv("POSTGRES_PORT", "5433")
	os.Setenv("POSTGRES_USER", "env_user")
	os.Setenv("POSTGRES_PASSWORD", "env_password")
	os.Setenv("POSTGRES_DATABASE", "env_db")
	os.Setenv("POSTGRES_SSL_MODE", "disable")
	os.Setenv("CONTEXT_API_ENDPOINT", "http://env-context:8080")
	os.Setenv("CONTEXT_API_TIMEOUT", "60")
	os.Setenv("SEMANTIC_THRESHOLD", "0.90")
	os.Setenv("TIME_WINDOW_MINUTES", "120")

	defer func() {
		os.Unsetenv("CONTROLLER_NAMESPACE")
		os.Unsetenv("METRICS_ADDRESS")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("MAX_CONCURRENCY")
		os.Unsetenv("KUBERNETES_QPS")
		os.Unsetenv("KUBERNETES_BURST")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_DATABASE")
		os.Unsetenv("POSTGRES_SSL_MODE")
		os.Unsetenv("CONTEXT_API_ENDPOINT")
		os.Unsetenv("CONTEXT_API_TIMEOUT")
		os.Unsetenv("SEMANTIC_THRESHOLD")
		os.Unsetenv("TIME_WINDOW_MINUTES")
	}()

	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)

	require.NoError(t, cfg.LoadFromEnv())

	// Verify environment overrides
	assert.Equal(t, "env-namespace", cfg.Namespace)
	assert.Equal(t, ":9090", cfg.MetricsAddress)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 50, cfg.MaxConcurrency)
	assert.Equal(t, float32(100.0), cfg.Kubernetes.QPS)
	assert.Equal(t, 150, cfg.Kubernetes.Burst)

	// Verify DataStorage overrides
	assert.Equal(t, "env-postgres-host", cfg.DataStorage.PostgresHost)
	assert.Equal(t, 5433, cfg.DataStorage.PostgresPort)
	assert.Equal(t, "env_user", cfg.DataStorage.PostgresUser)
	assert.Equal(t, "env_password", cfg.DataStorage.PostgresPassword)
	assert.Equal(t, "env_db", cfg.DataStorage.PostgresDatabase)
	assert.Equal(t, "disable", cfg.DataStorage.SSLMode)

	// Verify Context API overrides
	assert.Equal(t, "http://env-context:8080", cfg.Context.Endpoint)
	assert.Equal(t, 60, cfg.Context.Timeout)

	// Verify Classification overrides
	assert.Equal(t, 0.90, cfg.Classification.SemanticThreshold)
	assert.Equal(t, 120, cfg.Classification.TimeWindowMinutes)
}

// TestLoadFromEnvInvalidValues tests invalid environment variable values.
func TestLoadFromEnvInvalidValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	baseConfig := `
namespace: test-namespace
metrics_address: ":8080"
health_address: ":8081"
log_level: info
max_concurrency: 10
kubernetes:
  qps: 20.0
  burst: 30
data_storage:
  postgres_host: localhost
  postgres_port: 5432
  postgres_user: test_user
  postgres_database: test_db
context:
  endpoint: http://context-api:8080
classification:
  semantic_threshold: 0.85
`

	require.NoError(t, os.WriteFile(configPath, []byte(baseConfig), 0644))

	tests := []struct {
		name   string
		envVar string
		value  string
		errMsg string
	}{
		{
			name:   "invalid max_concurrency",
			envVar: "MAX_CONCURRENCY",
			value:  "invalid",
			errMsg: "invalid MAX_CONCURRENCY",
		},
		{
			name:   "invalid kubernetes qps",
			envVar: "KUBERNETES_QPS",
			value:  "invalid",
			errMsg: "invalid KUBERNETES_QPS",
		},
		{
			name:   "invalid kubernetes burst",
			envVar: "KUBERNETES_BURST",
			value:  "invalid",
			errMsg: "invalid KUBERNETES_BURST",
		},
		{
			name:   "invalid postgres port",
			envVar: "POSTGRES_PORT",
			value:  "invalid",
			errMsg: "invalid POSTGRES_PORT",
		},
		{
			name:   "invalid context timeout",
			envVar: "CONTEXT_API_TIMEOUT",
			value:  "invalid",
			errMsg: "invalid CONTEXT_API_TIMEOUT",
		},
		{
			name:   "invalid semantic threshold",
			envVar: "SEMANTIC_THRESHOLD",
			value:  "invalid",
			errMsg: "invalid SEMANTIC_THRESHOLD",
		},
		{
			name:   "invalid time window",
			envVar: "TIME_WINDOW_MINUTES",
			value:  "invalid",
			errMsg: "invalid TIME_WINDOW_MINUTES",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(tt.envVar, tt.value)
			defer os.Unsetenv(tt.envVar)

			cfg, err := config.LoadConfig(configPath)
			require.NoError(t, err)

			err = cfg.LoadFromEnv()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestDefaults tests default values are set correctly.
func TestDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	minimalConfig := `
data_storage:
  postgres_host: localhost
  postgres_user: test_user
  postgres_database: test_db
context:
  endpoint: http://context-api:8080
`

	require.NoError(t, os.WriteFile(configPath, []byte(minimalConfig), 0644))

	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)

	// Verify defaults
	assert.Equal(t, "kubernaut-system", cfg.Namespace)
	assert.Equal(t, ":8080", cfg.MetricsAddress)
	assert.Equal(t, ":8081", cfg.HealthAddress)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 10, cfg.MaxConcurrency)
	assert.Equal(t, float32(20.0), cfg.Kubernetes.QPS)
	assert.Equal(t, 30, cfg.Kubernetes.Burst)

	// Verify DataStorage defaults
	assert.Equal(t, 5432, cfg.DataStorage.PostgresPort)
	assert.Equal(t, "require", cfg.DataStorage.SSLMode)
	assert.Equal(t, 25, cfg.DataStorage.MaxConnections)
	assert.Equal(t, 5, cfg.DataStorage.MaxIdleConns)

	// Verify Context API defaults
	assert.Equal(t, 30, cfg.Context.Timeout)
	assert.Equal(t, 3, cfg.Context.MaxRetries)
	assert.Equal(t, 100, cfg.Context.RetryBackoffMs)

	// Verify Classification defaults
	assert.Equal(t, 0.85, cfg.Classification.SemanticThreshold)
	assert.Equal(t, 60, cfg.Classification.TimeWindowMinutes)
	assert.Equal(t, "cosine", cfg.Classification.SimilarityEngine)
	assert.Equal(t, 100, cfg.Classification.BatchSize)
}
