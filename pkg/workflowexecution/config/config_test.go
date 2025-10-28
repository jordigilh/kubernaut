// Package config_test provides unit tests for workflowexecution configuration.
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
)

// TestLoadConfig tests loading configuration from YAML file.
func TestLoadConfig(t *testing.T) {
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

kubernetes_api:
  timeout: 60
  retry_attempts: 5
  retry_backoff_ms: 200
  max_retry_delay: 20000
  watch_timeout: 600
  list_chunk_size: 1000
  namespace: workflows

parallel_limits:
  max_concurrent: 10
  complexity_threshold: 15
  approval_required: true
  max_steps_per_workflow: 200
  max_depth_level: 15
  enable_auto_scaling: true
  auto_scaling_threshold: 12

validation:
  rego_policy_configmap: custom-policies
  enabled: true
  default_action: deny
  strict_mode: true
  fail_on_warnings: false
  validation_timeout: 20

complexity:
  max_complexity_score: 150
  step_weight_multiplier: 1.5
  depth_weight_multiplier: 3.0
  enable_auto_reject: true
  reject_threshold: 120
  warn_threshold: 90
`

	require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0644))

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

	// Verify KubernetesAPI config
	assert.Equal(t, 60, cfg.KubernetesAPI.Timeout)
	assert.Equal(t, 5, cfg.KubernetesAPI.RetryAttempts)
	assert.Equal(t, 200, cfg.KubernetesAPI.RetryBackoffMs)
	assert.Equal(t, 20000, cfg.KubernetesAPI.MaxRetryDelay)
	assert.Equal(t, 600, cfg.KubernetesAPI.WatchTimeout)
	assert.Equal(t, int64(1000), cfg.KubernetesAPI.ListChunkSize)
	assert.Equal(t, "workflows", cfg.KubernetesAPI.Namespace)

	// Verify ParallelLimits config
	assert.Equal(t, 10, cfg.ParallelLimits.MaxConcurrent)
	assert.Equal(t, 15, cfg.ParallelLimits.ComplexityThreshold)
	assert.True(t, cfg.ParallelLimits.ApprovalRequired)
	assert.Equal(t, 200, cfg.ParallelLimits.MaxStepsPerWorkflow)
	assert.Equal(t, 15, cfg.ParallelLimits.MaxDepthLevel)
	assert.True(t, cfg.ParallelLimits.EnableAutoScaling)
	assert.Equal(t, 12, cfg.ParallelLimits.AutoScalingThreshold)

	// Verify Validation config
	assert.Equal(t, "custom-policies", cfg.Validation.RegoPolicyConfigMap)
	assert.True(t, cfg.Validation.Enabled)
	assert.Equal(t, "deny", cfg.Validation.DefaultAction)
	assert.True(t, cfg.Validation.StrictMode)
	assert.False(t, cfg.Validation.FailOnWarnings)
	assert.Equal(t, 20, cfg.Validation.ValidationTimeout)

	// Verify Complexity config
	assert.Equal(t, 150, cfg.Complexity.MaxComplexityScore)
	assert.Equal(t, 1.5, cfg.Complexity.StepWeightMultiplier)
	assert.Equal(t, 3.0, cfg.Complexity.DepthWeightMultiplier)
	assert.True(t, cfg.Complexity.EnableAutoReject)
	assert.Equal(t, 120, cfg.Complexity.RejectThreshold)
	assert.Equal(t, 90, cfg.Complexity.WarnThreshold)
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
kubernetes_api:
  timeout: 30
  retry_attempts: 3
  list_chunk_size: 500
parallel_limits:
  max_concurrent: 5
  complexity_threshold: 10
  max_steps_per_workflow: 100
  max_depth_level: 10
validation:
  rego_policy_configmap: policies
  enabled: true
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 80
  warn_threshold: 60
`,
			wantErr: false,
		},
		{
			name: "invalid parallel_limits max_concurrent",
			config: `
namespace: test
kubernetes:
  qps: 20.0
  burst: 30
kubernetes_api:
  timeout: 30
  list_chunk_size: 500
parallel_limits:
  max_concurrent: -1
validation:
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 80
  warn_threshold: 60
`,
			wantErr: true,
			errMsg:  "parallel_limits.max_concurrent must be greater than 0",
		},
		{
			name: "invalid validation default_action",
			config: `
namespace: test
kubernetes:
  qps: 20.0
  burst: 30
kubernetes_api:
  timeout: 30
  list_chunk_size: 500
parallel_limits:
  max_concurrent: 5
  complexity_threshold: 10
  max_steps_per_workflow: 100
  max_depth_level: 10
validation:
  enabled: true
  default_action: invalid
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 80
  warn_threshold: 60
`,
			wantErr: true,
			errMsg:  "validation.default_action must be 'allow' or 'deny'",
		},
		{
			name: "invalid complexity warn_threshold >= reject_threshold",
			config: `
namespace: test
kubernetes:
  qps: 20.0
  burst: 30
kubernetes_api:
  timeout: 30
  list_chunk_size: 500
parallel_limits:
  max_concurrent: 5
  complexity_threshold: 10
  max_steps_per_workflow: 100
  max_depth_level: 10
validation:
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 60
  warn_threshold: 80
`,
			wantErr: true,
			errMsg:  "complexity.warn_threshold must be between 1 and reject_threshold",
		},
		{
			name: "invalid kubernetes_api list_chunk_size",
			config: `
namespace: test
kubernetes:
  qps: 20.0
  burst: 30
kubernetes_api:
  timeout: 30
  list_chunk_size: -100
parallel_limits:
  max_concurrent: 5
  complexity_threshold: 10
  max_steps_per_workflow: 100
  max_depth_level: 10
validation:
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 80
  warn_threshold: 60
`,
			wantErr: true,
			errMsg:  "kubernetes_api.list_chunk_size must be greater than 0",
		},
		{
			name: "invalid kubernetes_api timeout",
			config: `
namespace: test
kubernetes:
  qps: 20.0
  burst: 30
kubernetes_api:
  timeout: -10
  list_chunk_size: 500
parallel_limits:
  max_concurrent: 5
validation:
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 80
  warn_threshold: 60
`,
			wantErr: true,
			errMsg:  "kubernetes_api.timeout must be greater than 0",
		},
		{
			name: "invalid parallel_limits max_steps",
			config: `
namespace: test
kubernetes:
  qps: 20.0
  burst: 30
kubernetes_api:
  timeout: 30
  list_chunk_size: 500
parallel_limits:
  max_concurrent: 5
  complexity_threshold: 10
  max_steps_per_workflow: -100
  max_depth_level: 10
validation:
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 80
  warn_threshold: 60
`,
			wantErr: true,
			errMsg:  "parallel_limits.max_steps_per_workflow must be greater than 0",
		},
		{
			name: "invalid complexity reject_threshold > max_score",
			config: `
namespace: test
kubernetes:
  qps: 20.0
  burst: 30
kubernetes_api:
  timeout: 30
  list_chunk_size: 500
parallel_limits:
  max_concurrent: 5
  complexity_threshold: 10
  max_steps_per_workflow: 100
  max_depth_level: 10
validation:
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 150
  warn_threshold: 60
`,
			wantErr: true,
			errMsg:  "complexity.reject_threshold must be between 1 and max_complexity_score",
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
kubernetes_api:
  timeout: 30
  list_chunk_size: 500
parallel_limits:
  max_concurrent: 5
  complexity_threshold: 10
  max_steps_per_workflow: 100
validation:
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 80
  warn_threshold: 60
`

	require.NoError(t, os.WriteFile(configPath, []byte(baseConfig), 0644))

	// Set environment variables
	os.Setenv("CONTROLLER_NAMESPACE", "env-namespace")
	os.Setenv("METRICS_ADDRESS", ":9090")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("MAX_CONCURRENCY", "50")
	os.Setenv("KUBERNETES_QPS", "100.0")
	os.Setenv("KUBERNETES_BURST", "150")
	os.Setenv("KUBERNETES_API_TIMEOUT", "120")
	os.Setenv("KUBERNETES_API_RETRY_ATTEMPTS", "10")
	os.Setenv("PARALLEL_MAX_CONCURRENT", "15")
	os.Setenv("PARALLEL_COMPLEXITY_THRESHOLD", "20")
	os.Setenv("PARALLEL_MAX_STEPS", "300")
	os.Setenv("VALIDATION_ENABLED", "true")
	os.Setenv("VALIDATION_DEFAULT_ACTION", "allow")
	os.Setenv("VALIDATION_REGO_CONFIGMAP", "env-policies")
	os.Setenv("COMPLEXITY_MAX_SCORE", "200")
	os.Setenv("COMPLEXITY_REJECT_THRESHOLD", "160")
	os.Setenv("COMPLEXITY_WARN_THRESHOLD", "120")

	defer func() {
		os.Unsetenv("CONTROLLER_NAMESPACE")
		os.Unsetenv("METRICS_ADDRESS")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("MAX_CONCURRENCY")
		os.Unsetenv("KUBERNETES_QPS")
		os.Unsetenv("KUBERNETES_BURST")
		os.Unsetenv("KUBERNETES_API_TIMEOUT")
		os.Unsetenv("KUBERNETES_API_RETRY_ATTEMPTS")
		os.Unsetenv("PARALLEL_MAX_CONCURRENT")
		os.Unsetenv("PARALLEL_COMPLEXITY_THRESHOLD")
		os.Unsetenv("PARALLEL_MAX_STEPS")
		os.Unsetenv("VALIDATION_ENABLED")
		os.Unsetenv("VALIDATION_DEFAULT_ACTION")
		os.Unsetenv("VALIDATION_REGO_CONFIGMAP")
		os.Unsetenv("COMPLEXITY_MAX_SCORE")
		os.Unsetenv("COMPLEXITY_REJECT_THRESHOLD")
		os.Unsetenv("COMPLEXITY_WARN_THRESHOLD")
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

	// Verify KubernetesAPI overrides
	assert.Equal(t, 120, cfg.KubernetesAPI.Timeout)
	assert.Equal(t, 10, cfg.KubernetesAPI.RetryAttempts)

	// Verify ParallelLimits overrides
	assert.Equal(t, 15, cfg.ParallelLimits.MaxConcurrent)
	assert.Equal(t, 20, cfg.ParallelLimits.ComplexityThreshold)
	assert.Equal(t, 300, cfg.ParallelLimits.MaxStepsPerWorkflow)

	// Verify Validation overrides
	assert.True(t, cfg.Validation.Enabled)
	assert.Equal(t, "allow", cfg.Validation.DefaultAction)
	assert.Equal(t, "env-policies", cfg.Validation.RegoPolicyConfigMap)

	// Verify Complexity overrides
	assert.Equal(t, 200, cfg.Complexity.MaxComplexityScore)
	assert.Equal(t, 160, cfg.Complexity.RejectThreshold)
	assert.Equal(t, 120, cfg.Complexity.WarnThreshold)
}

// TestLoadFromEnvInvalidValues tests invalid environment variable values.
func TestLoadFromEnvInvalidValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	baseConfig := `
namespace: test
kubernetes:
  qps: 20.0
  burst: 30
kubernetes_api:
  timeout: 30
  list_chunk_size: 500
parallel_limits:
  max_concurrent: 5
validation:
  default_action: deny
  validation_timeout: 10
complexity:
  max_complexity_score: 100
  step_weight_multiplier: 1.0
  depth_weight_multiplier: 2.0
  reject_threshold: 80
  warn_threshold: 60
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
			name:   "invalid kubernetes_api timeout",
			envVar: "KUBERNETES_API_TIMEOUT",
			value:  "invalid",
			errMsg: "invalid KUBERNETES_API_TIMEOUT",
		},
		{
			name:   "invalid parallel max_concurrent",
			envVar: "PARALLEL_MAX_CONCURRENT",
			value:  "invalid",
			errMsg: "invalid PARALLEL_MAX_CONCURRENT",
		},
		{
			name:   "invalid complexity max_score",
			envVar: "COMPLEXITY_MAX_SCORE",
			value:  "invalid",
			errMsg: "invalid COMPLEXITY_MAX_SCORE",
		},
		{
			name:   "invalid complexity reject_threshold",
			envVar: "COMPLEXITY_REJECT_THRESHOLD",
			value:  "invalid",
			errMsg: "invalid COMPLEXITY_REJECT_THRESHOLD",
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
namespace: kubernaut-system
`

	require.NoError(t, os.WriteFile(configPath, []byte(minimalConfig), 0644))

	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)

	// Verify common defaults
	assert.Equal(t, "kubernaut-system", cfg.Namespace)
	assert.Equal(t, ":8080", cfg.MetricsAddress)
	assert.Equal(t, ":8081", cfg.HealthAddress)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 10, cfg.MaxConcurrency)
	assert.Equal(t, float32(20.0), cfg.Kubernetes.QPS)
	assert.Equal(t, 30, cfg.Kubernetes.Burst)

	// Verify KubernetesAPI defaults
	assert.Equal(t, 30, cfg.KubernetesAPI.Timeout)
	assert.Equal(t, 3, cfg.KubernetesAPI.RetryAttempts)
	assert.Equal(t, 100, cfg.KubernetesAPI.RetryBackoffMs)
	assert.Equal(t, 10000, cfg.KubernetesAPI.MaxRetryDelay)
	assert.Equal(t, 300, cfg.KubernetesAPI.WatchTimeout)
	assert.Equal(t, int64(500), cfg.KubernetesAPI.ListChunkSize)

	// Verify ParallelLimits defaults
	assert.Equal(t, 5, cfg.ParallelLimits.MaxConcurrent)
	assert.Equal(t, 10, cfg.ParallelLimits.ComplexityThreshold)
	assert.Equal(t, 100, cfg.ParallelLimits.MaxStepsPerWorkflow)
	assert.Equal(t, 10, cfg.ParallelLimits.MaxDepthLevel)
	assert.Equal(t, 8, cfg.ParallelLimits.AutoScalingThreshold)

	// Verify Validation defaults
	assert.Equal(t, "workflow-validation-policies", cfg.Validation.RegoPolicyConfigMap)
	assert.Equal(t, "deny", cfg.Validation.DefaultAction)
	assert.Equal(t, 10, cfg.Validation.ValidationTimeout)

	// Verify Complexity defaults
	assert.Equal(t, 100, cfg.Complexity.MaxComplexityScore)
	assert.Equal(t, 1.0, cfg.Complexity.StepWeightMultiplier)
	assert.Equal(t, 2.0, cfg.Complexity.DepthWeightMultiplier)
	assert.Equal(t, 80, cfg.Complexity.RejectThreshold)
	assert.Equal(t, 60, cfg.Complexity.WarnThreshold)
}

