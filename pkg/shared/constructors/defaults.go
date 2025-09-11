package constructors

import (
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
)

// ConfigDefaults defines default configuration patterns used across the codebase.
// This consolidates the common "if config == nil" pattern found in 20+ constructors.

// DefaultConfigBuilder provides a fluent interface for building default configurations
type DefaultConfigBuilder[T any] struct {
	defaults T
}

// NewDefaultConfigBuilder creates a new default config builder for type T
func NewDefaultConfigBuilder[T any]() *DefaultConfigBuilder[T] {
	return &DefaultConfigBuilder[T]{}
}

// WithDefaults sets the default values for the configuration
func (b *DefaultConfigBuilder[T]) WithDefaults(defaults T) *DefaultConfigBuilder[T] {
	b.defaults = defaults
	return b
}

// BuildOrDefault returns the provided config if not nil, otherwise returns defaults
func (b *DefaultConfigBuilder[T]) BuildOrDefault(config *T) *T {
	if config == nil {
		// Create a copy of defaults
		defaultsCopy := b.defaults
		return &defaultsCopy
	}
	return config
}

// Common timeout defaults used across multiple services
var (
	DefaultExecutionTimeout = 30 * time.Minute
	DefaultStepTimeout      = 60 * time.Second
	DefaultHealthTimeout    = 10 * time.Second
	DefaultRecoveryTimeout  = 10 * time.Minute
	DefaultRetryBackoff     = 30 * time.Second
)

// CommonConfigDefaults provides default values for common configuration fields
type CommonConfigDefaults struct {
	MaxConcurrentExecutions int           `yaml:"max_concurrent_executions" default:"10"`
	DefaultTimeout          time.Duration `yaml:"default_timeout" default:"30m"`
	EnableSafetyChecks      bool          `yaml:"enable_safety_checks" default:"true"`
	DefaultRetries          int           `yaml:"default_retries" default:"3"`
	RetryBackoffBase        time.Duration `yaml:"retry_backoff_base" default:"30s"`
	EnableAutoRecovery      bool          `yaml:"enable_auto_recovery" default:"true"`
	MaxRecoveryAttempts     int           `yaml:"max_recovery_attempts" default:"3"`
	RecoveryTimeout         time.Duration `yaml:"recovery_timeout" default:"10m"`
	EnableDetailedLogging   bool          `yaml:"enable_detailed_logging" default:"false"`
	HealthCheckInterval     time.Duration `yaml:"health_check_interval" default:"5m"`
}

// GetCommonDefaults returns common default configuration values
func GetCommonDefaults() CommonConfigDefaults {
	return CommonConfigDefaults{
		MaxConcurrentExecutions: 10,
		DefaultTimeout:          DefaultExecutionTimeout,
		EnableSafetyChecks:      true,
		DefaultRetries:          3,
		RetryBackoffBase:        DefaultRetryBackoff,
		EnableAutoRecovery:      true,
		MaxRecoveryAttempts:     3,
		RecoveryTimeout:         DefaultRecoveryTimeout,
		EnableDetailedLogging:   false,
		HealthCheckInterval:     5 * time.Minute,
	}
}

// ApplyCommonDefaults applies common default values to a config struct using reflection
func ApplyCommonDefaults(config interface{}) {
	if config == nil {
		return
	}

	v := reflect.ValueOf(config)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	defaults := GetCommonDefaults()

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Apply defaults for common field names
		switch fieldType.Name {
		case "MaxConcurrentExecutions":
			if field.Int() == 0 {
				field.SetInt(int64(defaults.MaxConcurrentExecutions))
			}
		case "DefaultTimeout":
			if field.Int() == 0 {
				field.SetInt(int64(defaults.DefaultTimeout))
			}
		case "EnableSafetyChecks":
			if !field.Bool() {
				field.SetBool(defaults.EnableSafetyChecks)
			}
		case "DefaultRetries", "MaxRetries":
			if field.Int() == 0 {
				field.SetInt(int64(defaults.DefaultRetries))
			}
		case "RetryBackoffBase":
			if field.Int() == 0 {
				field.SetInt(int64(defaults.RetryBackoffBase))
			}
		case "EnableAutoRecovery":
			if !field.Bool() {
				field.SetBool(defaults.EnableAutoRecovery)
			}
		case "MaxRecoveryAttempts":
			if field.Int() == 0 {
				field.SetInt(int64(defaults.MaxRecoveryAttempts))
			}
		case "RecoveryTimeout":
			if field.Int() == 0 {
				field.SetInt(int64(defaults.RecoveryTimeout))
			}
		case "HealthCheckInterval":
			if field.Int() == 0 {
				field.SetInt(int64(defaults.HealthCheckInterval))
			}
		}
	}
}

// NewWithDefaults is a generic constructor helper that applies default configuration
func NewWithDefaults[T any, ConfigT any](
	constructor func(ConfigT, *logrus.Logger) T,
	config *ConfigT,
	defaultConfig ConfigT,
	logger *logrus.Logger,
) T {
	if config == nil {
		config = &defaultConfig
	}
	return constructor(*config, logger)
}

// LogConstructorInfo logs standard constructor information
func LogConstructorInfo(logger *logrus.Logger, componentName string, config interface{}) {
	if logger == nil {
		return
	}

	configFields := logrus.Fields{
		"component": componentName,
	}

	// Use reflection to add key config fields to log
	if config != nil {
		v := reflect.ValueOf(config)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		if v.Kind() == reflect.Struct {
			t := v.Type()
			for i := 0; i < v.NumField() && i < 5; i++ { // Limit to 5 fields to avoid verbose logs
				field := v.Field(i)
				fieldType := t.Field(i)

				if field.CanInterface() {
					configFields[fieldType.Name] = field.Interface()
				}
			}
		}
	}

	logger.WithFields(configFields).Info("Component initialized with configuration")
}
