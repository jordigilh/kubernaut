// Package config provides configuration management for workflowexecution controller.
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config represents the controller configuration.
type Config struct {
	// Common controller configuration
	Namespace      string `yaml:"namespace"`
	MetricsAddress string `yaml:"metrics_address"`
	HealthAddress  string `yaml:"health_address"`
	LeaderElection bool   `yaml:"leader_election"`
	LogLevel       string `yaml:"log_level"`
	MaxConcurrency int    `yaml:"max_concurrency"`

	// Kubernetes API configuration
	Kubernetes KubernetesConfig `yaml:"kubernetes"`

	// WorkflowExecution-specific configuration
	KubernetesAPI  KubernetesAPIConfig  `yaml:"kubernetes_api"`
	ParallelLimits ParallelLimitsConfig `yaml:"parallel_limits"`
	Validation     ValidationConfig     `yaml:"validation"`
	Complexity     ComplexityConfig     `yaml:"complexity"`
}

// KubernetesConfig holds basic Kubernetes API client configuration.
type KubernetesConfig struct {
	QPS   float32 `yaml:"qps"`
	Burst int     `yaml:"burst"`
}

// KubernetesAPIConfig holds advanced Kubernetes API client configuration for workflow execution.
type KubernetesAPIConfig struct {
	Timeout        int    `yaml:"timeout"`
	RetryAttempts  int    `yaml:"retry_attempts"`
	RetryBackoffMs int    `yaml:"retry_backoff_ms"`
	MaxRetryDelay  int    `yaml:"max_retry_delay"`
	WatchTimeout   int    `yaml:"watch_timeout"`
	ListChunkSize  int64  `yaml:"list_chunk_size"`
	Namespace      string `yaml:"namespace"`
}

// ParallelLimitsConfig holds configuration for parallel workflow execution limits.
type ParallelLimitsConfig struct {
	MaxConcurrent         int  `yaml:"max_concurrent"`
	ComplexityThreshold   int  `yaml:"complexity_threshold"`
	ApprovalRequired      bool `yaml:"approval_required"`
	MaxStepsPerWorkflow   int  `yaml:"max_steps_per_workflow"`
	MaxDepthLevel         int  `yaml:"max_depth_level"`
	EnableAutoScaling     bool `yaml:"enable_auto_scaling"`
	AutoScalingThreshold  int  `yaml:"auto_scaling_threshold"`
}

// ValidationConfig holds configuration for Rego policy validation framework.
type ValidationConfig struct {
	RegoPolicyConfigMap string `yaml:"rego_policy_configmap"`
	Enabled             bool   `yaml:"enabled"`
	DefaultAction       string `yaml:"default_action"`
	StrictMode          bool   `yaml:"strict_mode"`
	FailOnWarnings      bool   `yaml:"fail_on_warnings"`
	ValidationTimeout   int    `yaml:"validation_timeout"`
}

// ComplexityConfig holds configuration for workflow complexity analysis.
type ComplexityConfig struct {
	MaxComplexityScore    int     `yaml:"max_complexity_score"`
	StepWeightMultiplier  float64 `yaml:"step_weight_multiplier"`
	DepthWeightMultiplier float64 `yaml:"depth_weight_multiplier"`
	EnableAutoReject      bool    `yaml:"enable_auto_reject"`
	RejectThreshold       int     `yaml:"reject_threshold"`
	WarnThreshold         int     `yaml:"warn_threshold"`
}

// LoadConfig loads configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	return &cfg, nil
}

// setDefaults sets default values for unspecified configuration.
func (c *Config) setDefaults() {
	if c.Namespace == "" {
		c.Namespace = "kubernaut-system"
	}
	if c.MetricsAddress == "" {
		c.MetricsAddress = ":8080"
	}
	if c.HealthAddress == "" {
		c.HealthAddress = ":8081"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.MaxConcurrency == 0 {
		c.MaxConcurrency = 10
	}
	if c.Kubernetes.QPS == 0 {
		c.Kubernetes.QPS = 20.0
	}
	if c.Kubernetes.Burst == 0 {
		c.Kubernetes.Burst = 30
	}

	// KubernetesAPI defaults
	if c.KubernetesAPI.Timeout == 0 {
		c.KubernetesAPI.Timeout = 30
	}
	if c.KubernetesAPI.RetryAttempts == 0 {
		c.KubernetesAPI.RetryAttempts = 3
	}
	if c.KubernetesAPI.RetryBackoffMs == 0 {
		c.KubernetesAPI.RetryBackoffMs = 100
	}
	if c.KubernetesAPI.MaxRetryDelay == 0 {
		c.KubernetesAPI.MaxRetryDelay = 10000
	}
	if c.KubernetesAPI.WatchTimeout == 0 {
		c.KubernetesAPI.WatchTimeout = 300
	}
	if c.KubernetesAPI.ListChunkSize == 0 {
		c.KubernetesAPI.ListChunkSize = 500
	}

	// ParallelLimits defaults
	if c.ParallelLimits.MaxConcurrent == 0 {
		c.ParallelLimits.MaxConcurrent = 5
	}
	if c.ParallelLimits.ComplexityThreshold == 0 {
		c.ParallelLimits.ComplexityThreshold = 10
	}
	if c.ParallelLimits.MaxStepsPerWorkflow == 0 {
		c.ParallelLimits.MaxStepsPerWorkflow = 100
	}
	if c.ParallelLimits.MaxDepthLevel == 0 {
		c.ParallelLimits.MaxDepthLevel = 10
	}
	if c.ParallelLimits.AutoScalingThreshold == 0 {
		c.ParallelLimits.AutoScalingThreshold = 8
	}

	// Validation defaults
	if c.Validation.RegoPolicyConfigMap == "" {
		c.Validation.RegoPolicyConfigMap = "workflow-validation-policies"
	}
	if c.Validation.DefaultAction == "" {
		c.Validation.DefaultAction = "deny"
	}
	if c.Validation.ValidationTimeout == 0 {
		c.Validation.ValidationTimeout = 10
	}

	// Complexity defaults
	if c.Complexity.MaxComplexityScore == 0 {
		c.Complexity.MaxComplexityScore = 100
	}
	if c.Complexity.StepWeightMultiplier == 0 {
		c.Complexity.StepWeightMultiplier = 1.0
	}
	if c.Complexity.DepthWeightMultiplier == 0 {
		c.Complexity.DepthWeightMultiplier = 2.0
	}
	if c.Complexity.RejectThreshold == 0 {
		c.Complexity.RejectThreshold = 80
	}
	if c.Complexity.WarnThreshold == 0 {
		c.Complexity.WarnThreshold = 60
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if c.MetricsAddress == "" {
		return fmt.Errorf("metrics_address is required")
	}
	if c.HealthAddress == "" {
		return fmt.Errorf("health_address is required")
	}
	if c.LogLevel == "" {
		return fmt.Errorf("log_level is required")
	}
	if c.MaxConcurrency <= 0 {
		return fmt.Errorf("max_concurrency must be greater than 0")
	}

	// Validate Kubernetes config
	if c.Kubernetes.QPS <= 0 {
		return fmt.Errorf("kubernetes.qps must be greater than 0")
	}
	if c.Kubernetes.Burst <= 0 {
		return fmt.Errorf("kubernetes.burst must be greater than 0")
	}

	// Validate KubernetesAPI config
	if c.KubernetesAPI.Timeout <= 0 {
		return fmt.Errorf("kubernetes_api.timeout must be greater than 0")
	}
	if c.KubernetesAPI.RetryAttempts < 0 {
		return fmt.Errorf("kubernetes_api.retry_attempts must be non-negative")
	}
	if c.KubernetesAPI.ListChunkSize <= 0 {
		return fmt.Errorf("kubernetes_api.list_chunk_size must be greater than 0")
	}

	// Validate ParallelLimits config
	if c.ParallelLimits.MaxConcurrent <= 0 {
		return fmt.Errorf("parallel_limits.max_concurrent must be greater than 0")
	}
	if c.ParallelLimits.ComplexityThreshold <= 0 {
		return fmt.Errorf("parallel_limits.complexity_threshold must be greater than 0")
	}
	if c.ParallelLimits.MaxStepsPerWorkflow <= 0 {
		return fmt.Errorf("parallel_limits.max_steps_per_workflow must be greater than 0")
	}
	if c.ParallelLimits.MaxDepthLevel <= 0 {
		return fmt.Errorf("parallel_limits.max_depth_level must be greater than 0")
	}

	// Validate Validation config
	if c.Validation.Enabled && c.Validation.RegoPolicyConfigMap == "" {
		return fmt.Errorf("validation.rego_policy_configmap is required when validation is enabled")
	}
	if c.Validation.DefaultAction != "allow" && c.Validation.DefaultAction != "deny" {
		return fmt.Errorf("validation.default_action must be 'allow' or 'deny'")
	}
	if c.Validation.ValidationTimeout <= 0 {
		return fmt.Errorf("validation.validation_timeout must be greater than 0")
	}

	// Validate Complexity config
	if c.Complexity.MaxComplexityScore <= 0 {
		return fmt.Errorf("complexity.max_complexity_score must be greater than 0")
	}
	if c.Complexity.StepWeightMultiplier <= 0 {
		return fmt.Errorf("complexity.step_weight_multiplier must be greater than 0")
	}
	if c.Complexity.DepthWeightMultiplier <= 0 {
		return fmt.Errorf("complexity.depth_weight_multiplier must be greater than 0")
	}
	if c.Complexity.RejectThreshold <= 0 || c.Complexity.RejectThreshold > c.Complexity.MaxComplexityScore {
		return fmt.Errorf("complexity.reject_threshold must be between 1 and max_complexity_score")
	}
	if c.Complexity.WarnThreshold <= 0 || c.Complexity.WarnThreshold >= c.Complexity.RejectThreshold {
		return fmt.Errorf("complexity.warn_threshold must be between 1 and reject_threshold")
	}

	return nil
}

// LoadFromEnv loads environment variable overrides.
func (c *Config) LoadFromEnv() error {
	// Common environment variables
	if ns := os.Getenv("CONTROLLER_NAMESPACE"); ns != "" {
		c.Namespace = ns
	}
	if addr := os.Getenv("METRICS_ADDRESS"); addr != "" {
		c.MetricsAddress = addr
	}
	if addr := os.Getenv("HEALTH_ADDRESS"); addr != "" {
		c.HealthAddress = addr
	}
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		c.LogLevel = level
	}
	if concurrency := os.Getenv("MAX_CONCURRENCY"); concurrency != "" {
		val, err := strconv.Atoi(concurrency)
		if err != nil {
			return fmt.Errorf("invalid MAX_CONCURRENCY: %w", err)
		}
		c.MaxConcurrency = val
	}

	// Kubernetes API overrides
	if qps := os.Getenv("KUBERNETES_QPS"); qps != "" {
		val, err := strconv.ParseFloat(qps, 32)
		if err != nil {
			return fmt.Errorf("invalid KUBERNETES_QPS: %w", err)
		}
		c.Kubernetes.QPS = float32(val)
	}
	if burst := os.Getenv("KUBERNETES_BURST"); burst != "" {
		val, err := strconv.Atoi(burst)
		if err != nil {
			return fmt.Errorf("invalid KUBERNETES_BURST: %w", err)
		}
		c.Kubernetes.Burst = val
	}

	// KubernetesAPI overrides
	if timeout := os.Getenv("KUBERNETES_API_TIMEOUT"); timeout != "" {
		val, err := strconv.Atoi(timeout)
		if err != nil {
			return fmt.Errorf("invalid KUBERNETES_API_TIMEOUT: %w", err)
		}
		c.KubernetesAPI.Timeout = val
	}
	if retries := os.Getenv("KUBERNETES_API_RETRY_ATTEMPTS"); retries != "" {
		val, err := strconv.Atoi(retries)
		if err != nil {
			return fmt.Errorf("invalid KUBERNETES_API_RETRY_ATTEMPTS: %w", err)
		}
		c.KubernetesAPI.RetryAttempts = val
	}

	// ParallelLimits overrides
	if maxConcurrent := os.Getenv("PARALLEL_MAX_CONCURRENT"); maxConcurrent != "" {
		val, err := strconv.Atoi(maxConcurrent)
		if err != nil {
			return fmt.Errorf("invalid PARALLEL_MAX_CONCURRENT: %w", err)
		}
		c.ParallelLimits.MaxConcurrent = val
	}
	if threshold := os.Getenv("PARALLEL_COMPLEXITY_THRESHOLD"); threshold != "" {
		val, err := strconv.Atoi(threshold)
		if err != nil {
			return fmt.Errorf("invalid PARALLEL_COMPLEXITY_THRESHOLD: %w", err)
		}
		c.ParallelLimits.ComplexityThreshold = val
	}
	if maxSteps := os.Getenv("PARALLEL_MAX_STEPS"); maxSteps != "" {
		val, err := strconv.Atoi(maxSteps)
		if err != nil {
			return fmt.Errorf("invalid PARALLEL_MAX_STEPS: %w", err)
		}
		c.ParallelLimits.MaxStepsPerWorkflow = val
	}

	// Validation overrides
	if enabled := os.Getenv("VALIDATION_ENABLED"); enabled != "" {
		c.Validation.Enabled = enabled == "true"
	}
	if action := os.Getenv("VALIDATION_DEFAULT_ACTION"); action != "" {
		c.Validation.DefaultAction = action
	}
	if configmap := os.Getenv("VALIDATION_REGO_CONFIGMAP"); configmap != "" {
		c.Validation.RegoPolicyConfigMap = configmap
	}

	// Complexity overrides
	if maxScore := os.Getenv("COMPLEXITY_MAX_SCORE"); maxScore != "" {
		val, err := strconv.Atoi(maxScore)
		if err != nil {
			return fmt.Errorf("invalid COMPLEXITY_MAX_SCORE: %w", err)
		}
		c.Complexity.MaxComplexityScore = val
	}
	if rejectThreshold := os.Getenv("COMPLEXITY_REJECT_THRESHOLD"); rejectThreshold != "" {
		val, err := strconv.Atoi(rejectThreshold)
		if err != nil {
			return fmt.Errorf("invalid COMPLEXITY_REJECT_THRESHOLD: %w", err)
		}
		c.Complexity.RejectThreshold = val
	}
	if warnThreshold := os.Getenv("COMPLEXITY_WARN_THRESHOLD"); warnThreshold != "" {
		val, err := strconv.Atoi(warnThreshold)
		if err != nil {
			return fmt.Errorf("invalid COMPLEXITY_WARN_THRESHOLD: %w", err)
		}
		c.Complexity.WarnThreshold = val
	}

	return nil
}










