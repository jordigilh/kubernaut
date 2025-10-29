// Generic config package unit test template for CRD controllers
//
// CUSTOMIZATION INSTRUCTIONS:
// 1. Update package name to match your controller
// 2. Update import path for config package
// 3. Update test YAML fixtures with controller-specific fields
// 4. Add validation tests for controller-specific constraints
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/remediationprocessor/config"
	"github.com/stretchr/testify/require"
	// TODO: Update import path
	// "github.com/jordigilh/kubernaut/pkg/{{CONTROLLER_NAME}}/config"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid config with all fields",
			yaml: `
namespace: test-namespace
metrics_address: :9090
health_address: :9091
leader_election: true
log_level: debug
max_concurrency: 20
kubernetes:
  qps: 30.0
  burst: 40
# TODO: Add controller-specific fields here
`,
			wantErr: false,
		},
		{
			name: "valid config with defaults",
			yaml: `
# Minimal config, should use defaults
log_level: info
# TODO: Add required controller-specific fields here
`,
			wantErr: false,
		},
		{
			name: "invalid yaml",
			yaml: `
namespace: test
invalid yaml: [
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.yaml), 0644)
			require.NoError(t, err)

			// Load config
			cfg, err := config.LoadConfig(configPath)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)

			// Verify defaults are set
			if tt.name == "valid config with defaults" {
				require.Equal(t, "kubernaut-system", cfg.Namespace)
				require.Equal(t, ":8080", cfg.MetricsAddress)
				require.Equal(t, ":8081", cfg.HealthAddress)
				require.Equal(t, 10, cfg.MaxConcurrency)
				require.Equal(t, float32(20.0), cfg.Kubernetes.QPS)
				require.Equal(t, 30, cfg.Kubernetes.Burst)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid configuration",
			config: &config.Config{
				Namespace:      "test-namespace",
				MetricsAddress: ":8080",
				HealthAddress:  ":8081",
				LogLevel:       "info",
				MaxConcurrency: 10,
				Kubernetes: config.KubernetesConfig{
					QPS:   20.0,
					Burst: 30,
				},
				// TODO: Add controller-specific fields here
			},
			wantErr: false,
		},
		{
			name: "missing namespace",
			config: &config.Config{
				MetricsAddress: ":8080",
				HealthAddress:  ":8081",
				LogLevel:       "info",
				MaxConcurrency: 10,
			},
			wantErr: true,
			errMsg:  "namespace is required",
		},
		{
			name: "invalid max_concurrency",
			config: &config.Config{
				Namespace:      "test-namespace",
				MetricsAddress: ":8080",
				HealthAddress:  ":8081",
				LogLevel:       "info",
				MaxConcurrency: 0,
			},
			wantErr: true,
			errMsg:  "max_concurrency must be greater than 0",
		},
		{
			name: "invalid kubernetes qps",
			config: &config.Config{
				Namespace:      "test-namespace",
				MetricsAddress: ":8080",
				HealthAddress:  ":8081",
				LogLevel:       "info",
				MaxConcurrency: 10,
				Kubernetes: config.KubernetesConfig{
					QPS:   0,
					Burst: 30,
				},
			},
			wantErr: true,
			errMsg:  "kubernetes.qps must be greater than 0",
		},
		// TODO: Add controller-specific validation tests here
		// Example:
		// {
		// 	name: "missing required field",
		// 	config: &config.Config{
		// 		Namespace:      "test-namespace",
		// 		MetricsAddress: ":8080",
		// 		HealthAddress:  ":8081",
		// 		LogLevel:       "info",
		// 		MaxConcurrency: 10,
		// 		// DataStorage.PostgresHost missing
		// 	},
		// 	wantErr: true,
		// 	errMsg:  "data_storage.postgres_host is required",
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		verify  func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "override namespace",
			envVars: map[string]string{
				"CONTROLLER_NAMESPACE": "env-namespace",
			},
			verify: func(t *testing.T, cfg *config.Config) {
				require.Equal(t, "env-namespace", cfg.Namespace)
			},
		},
		{
			name: "override metrics address",
			envVars: map[string]string{
				"METRICS_ADDRESS": ":9090",
			},
			verify: func(t *testing.T, cfg *config.Config) {
				require.Equal(t, ":9090", cfg.MetricsAddress)
			},
		},
		{
			name: "override max concurrency",
			envVars: map[string]string{
				"MAX_CONCURRENCY": "50",
			},
			verify: func(t *testing.T, cfg *config.Config) {
				require.Equal(t, 50, cfg.MaxConcurrency)
			},
		},
		{
			name: "override kubernetes qps and burst",
			envVars: map[string]string{
				"KUBERNETES_QPS":   "50.0",
				"KUBERNETES_BURST": "75",
			},
			verify: func(t *testing.T, cfg *config.Config) {
				require.Equal(t, float32(50.0), cfg.Kubernetes.QPS)
				require.Equal(t, 75, cfg.Kubernetes.Burst)
			},
		},
		// TODO: Add controller-specific environment variable tests here
		// Example:
		// {
		// 	name: "override postgres host",
		// 	envVars: map[string]string{
		// 		"POSTGRES_HOST": "custom-postgres.example.com",
		// 	},
		// 	verify: func(t *testing.T, cfg *config.Config) {
		// 		require.Equal(t, "custom-postgres.example.com", cfg.DataStorage.PostgresHost)
		// 	},
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			// Create minimal config
			cfg := &config.Config{
				Namespace:      "default-namespace",
				MetricsAddress: ":8080",
				HealthAddress:  ":8081",
				LogLevel:       "info",
				MaxConcurrency: 10,
				Kubernetes: config.KubernetesConfig{
					QPS:   20.0,
					Burst: 30,
				},
			}

			// Load environment overrides
			err := cfg.LoadFromEnv()
			require.NoError(t, err)

			// Verify overrides
			tt.verify(t, cfg)
		})
	}
}

func TestLoadFromEnvInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		errMsg  string
	}{
		{
			name: "invalid max_concurrency",
			envVars: map[string]string{
				"MAX_CONCURRENCY": "invalid",
			},
			wantErr: true,
			errMsg:  "invalid MAX_CONCURRENCY",
		},
		{
			name: "invalid kubernetes_qps",
			envVars: map[string]string{
				"KUBERNETES_QPS": "invalid",
			},
			wantErr: true,
			errMsg:  "invalid KUBERNETES_QPS",
		},
		{
			name: "invalid kubernetes_burst",
			envVars: map[string]string{
				"KUBERNETES_BURST": "invalid",
			},
			wantErr: true,
			errMsg:  "invalid KUBERNETES_BURST",
		},
		// TODO: Add controller-specific invalid value tests here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			cfg := &config.Config{}
			err := cfg.LoadFromEnv()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}








