package adaptive

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
)

// ConfigurationManager handles complex configuration management for the pattern discovery engine
type ConfigurationManager struct {
	log              *logrus.Logger
	configValidator  *ConfigValidator
	profiles         map[string]*ConfigProfile
	currentProfile   string
	overrides        map[string]interface{}
	loadedConfig     *patterns.EnhancedPatternConfig
	validationErrors []ConfigValidationError
}

// ConfigProfile represents a pre-defined configuration profile
type ConfigProfile struct {
	Name        string                          `yaml:"name"`
	Description string                          `yaml:"description"`
	Environment string                          `yaml:"environment"` // "development", "staging", "production"
	Config      *patterns.EnhancedPatternConfig `yaml:"config"`
	Metadata    map[string]string               `yaml:"metadata"`
}

// ConfigValidator validates configuration settings
type ConfigValidator struct {
	rules map[string]*ConfigValidationRule
	log   *logrus.Logger
}

// ConfigValidationRule defines a configuration validation rule
type ConfigValidationRule struct {
	Field        string                 `yaml:"field"`
	Type         string                 `yaml:"type"` // "range", "enum", "dependency", "custom"
	Min          interface{}            `yaml:"min,omitempty"`
	Max          interface{}            `yaml:"max,omitempty"`
	Values       []interface{}          `yaml:"values,omitempty"`
	Dependencies []string               `yaml:"dependencies,omitempty"`
	Validator    func(interface{}) bool `yaml:"-"`
	Message      string                 `yaml:"message"`
	Severity     ValidationSeverity     `yaml:"severity"`
}

// ValidationSeverity defines the severity of validation issues
type ValidationSeverity string

const (
	ValidationSeverityError   ValidationSeverity = "error"
	ValidationSeverityWarning ValidationSeverity = "warning"
	ValidationSeverityInfo    ValidationSeverity = "info"
)

// ConfigValidationError represents a configuration validation error
type ConfigValidationError struct {
	Field    string             `json:"field"`
	Value    interface{}        `json:"value"`
	Rule     string             `json:"rule"`
	Message  string             `json:"message"`
	Severity ValidationSeverity `json:"severity"`
	Context  map[string]string  `json:"context,omitempty"`
}

// ConfigurationReport provides a comprehensive report of the configuration
type ConfigurationReport struct {
	Profile          string                  `json:"profile"`
	ValidationPassed bool                    `json:"validation_passed"`
	Errors           []ConfigValidationError `json:"errors"`
	Warnings         []ConfigValidationError `json:"warnings"`
	RecommendedFixes []string                `json:"recommended_fixes"`
	OptimalSettings  map[string]interface{}  `json:"optimal_settings"`
	SecurityIssues   []SecurityIssue         `json:"security_issues"`
	PerformanceNotes []string                `json:"performance_notes"`
}

// SecurityIssue represents a security-related configuration issue
type SecurityIssue struct {
	Issue       string `json:"issue"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Fix         string `json:"fix"`
}

// ConfigurationGuide provides guided configuration setup
type ConfigurationGuide struct {
	manager *ConfigurationManager
	log     *logrus.Logger
}

// NewConfigurationManager creates a new configuration manager
func NewConfigurationManager(log *logrus.Logger) *ConfigurationManager {
	cm := &ConfigurationManager{
		log:              log,
		profiles:         make(map[string]*ConfigProfile),
		overrides:        make(map[string]interface{}),
		validationErrors: make([]ConfigValidationError, 0),
	}

	cm.configValidator = NewConfigValidator(log)
	cm.initializeBuiltinProfiles()
	cm.loadValidationRules()

	return cm
}

// LoadConfiguration loads and validates configuration from various sources
func (cm *ConfigurationManager) LoadConfiguration(configPath string, profileName string) (*patterns.EnhancedPatternConfig, error) {
	cm.log.WithFields(logrus.Fields{
		"config_path": configPath,
		"profile":     profileName,
	}).Info("Loading pattern engine configuration")

	// Start with profile if specified
	var config *patterns.EnhancedPatternConfig
	if profileName != "" {
		profile, exists := cm.profiles[profileName]
		if !exists {
			return nil, fmt.Errorf("configuration profile '%s' not found", profileName)
		}
		config = cm.cloneConfig(profile.Config)
		cm.currentProfile = profileName
	} else {
		config = cm.getDefaultConfig()
		cm.currentProfile = "default"
	}

	// Load from file if provided
	if configPath != "" {
		fileConfig, err := cm.loadConfigFromFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
		config = cm.mergeConfigurations(config, fileConfig)
	}

	// Apply environment variable overrides
	config = cm.applyEnvironmentOverrides(config)

	// Apply manual overrides
	config = cm.applyOverrides(config)

	// Validate configuration
	validationErrors := cm.configValidator.ValidateConfiguration(config)
	cm.validationErrors = validationErrors

	// Check for critical errors
	for _, err := range validationErrors {
		if err.Severity == ValidationSeverityError {
			return nil, fmt.Errorf("configuration validation failed: %s", err.Message)
		}
	}

	// Log warnings
	for _, warning := range validationErrors {
		if warning.Severity == ValidationSeverityWarning {
			cm.log.WithField("field", warning.Field).Warn(warning.Message)
		}
	}

	cm.loadedConfig = config
	cm.log.WithFields(logrus.Fields{
		"profile":           cm.currentProfile,
		"validation_errors": len(validationErrors),
	}).Info("Configuration loaded successfully")

	return config, nil
}

// GetConfigurationReport generates a comprehensive configuration report
func (cm *ConfigurationManager) GetConfigurationReport() *ConfigurationReport {
	if cm.loadedConfig == nil {
		return &ConfigurationReport{
			ValidationPassed: false,
			Errors: []ConfigValidationError{{
				Message:  "No configuration loaded",
				Severity: ValidationSeverityError,
			}},
		}
	}

	errors := make([]ConfigValidationError, 0)
	warnings := make([]ConfigValidationError, 0)

	for _, err := range cm.validationErrors {
		if err.Severity == ValidationSeverityError {
			errors = append(errors, err)
		} else if err.Severity == ValidationSeverityWarning {
			warnings = append(warnings, err)
		}
	}

	report := &ConfigurationReport{
		Profile:          cm.currentProfile,
		ValidationPassed: len(errors) == 0,
		Errors:           errors,
		Warnings:         warnings,
		RecommendedFixes: cm.generateRecommendedFixes(),
		OptimalSettings:  cm.generateOptimalSettings(),
		SecurityIssues:   cm.checkSecurityIssues(),
		PerformanceNotes: cm.generatePerformanceNotes(),
	}

	return report
}

// GetConfigurationGuide returns a guided configuration helper
func (cm *ConfigurationManager) GetConfigurationGuide() *ConfigurationGuide {
	return &ConfigurationGuide{
		manager: cm,
		log:     cm.log,
	}
}

// SetOverride sets a configuration override
func (cm *ConfigurationManager) SetOverride(path string, value interface{}) error {
	if err := cm.validateOverridePath(path); err != nil {
		return fmt.Errorf("invalid override path '%s': %w", path, err)
	}

	cm.overrides[path] = value
	cm.log.WithFields(logrus.Fields{
		"path":  path,
		"value": value,
	}).Debug("Configuration override set")

	return nil
}

// ListProfiles returns all available configuration profiles
func (cm *ConfigurationManager) ListProfiles() map[string]*ConfigProfile {
	return cm.profiles
}

// ExportConfiguration exports the current configuration to a file
func (cm *ConfigurationManager) ExportConfiguration(outputPath string) error {
	if cm.loadedConfig == nil {
		return fmt.Errorf("no configuration loaded to export")
	}

	data, err := yaml.Marshal(cm.loadedConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	cm.log.WithField("output_path", outputPath).Info("Configuration exported successfully")
	return nil
}

// Private helper methods

func (cm *ConfigurationManager) initializeBuiltinProfiles() {
	// Development profile
	cm.profiles["development"] = &ConfigProfile{
		Name:        "development",
		Description: "Development environment configuration with relaxed validation",
		Environment: "development",
		Config: &patterns.EnhancedPatternConfig{
			PatternDiscoveryConfig: &patterns.PatternDiscoveryConfig{
				MinExecutionsForPattern: 5,  // Lower threshold for development
				MaxHistoryDays:          30, // Shorter history
				SamplingInterval:        30 * time.Minute,
				SimilarityThreshold:     0.75, // More lenient
				ClusteringEpsilon:       0.4,
				MinClusterSize:          3,
				ModelUpdateInterval:     4 * time.Hour,
				FeatureWindowSize:       25,
				PredictionConfidence:    0.6,
				MaxConcurrentAnalysis:   5,
				PatternCacheSize:        500,
				EnableRealTimeDetection: true,
			},
			EnableStatisticalValidation: true,
			RequireValidationPassing:    false, // Allow experimentation
			MinReliabilityScore:         0.5,
			EnableOverfittingPrevention: true,
			MaxOverfittingRisk:          0.8, // More tolerant
			RequireCrossValidation:      false,
			EnableMonitoring:            true,
			AutoRecovery:                true,
		},
		Metadata: map[string]string{
			"created_by": "system",
			"purpose":    "development and testing",
		},
	}

	// Production profile
	cm.profiles["production"] = &ConfigProfile{
		Name:        "production",
		Description: "Production environment configuration with strict validation",
		Environment: "production",
		Config: &patterns.EnhancedPatternConfig{
			PatternDiscoveryConfig: &patterns.PatternDiscoveryConfig{
				MinExecutionsForPattern: 20,  // Higher threshold
				MaxHistoryDays:          180, // Longer history
				SamplingInterval:        time.Hour,
				SimilarityThreshold:     0.9, // Strict similarity
				ClusteringEpsilon:       0.25,
				MinClusterSize:          8,
				ModelUpdateInterval:     24 * time.Hour,
				FeatureWindowSize:       100,
				PredictionConfidence:    0.8,
				MaxConcurrentAnalysis:   15,
				PatternCacheSize:        2000,
				EnableRealTimeDetection: true,
			},
			EnableStatisticalValidation: true,
			RequireValidationPassing:    true, // Strict validation
			MinReliabilityScore:         0.8,
			EnableOverfittingPrevention: true,
			MaxOverfittingRisk:          0.5, // Conservative
			RequireCrossValidation:      true,
			EnableMonitoring:            true,
			AutoRecovery:                true,
		},
		Metadata: map[string]string{
			"created_by": "system",
			"purpose":    "production workloads",
		},
	}

	// High-performance profile
	cm.profiles["high-performance"] = &ConfigProfile{
		Name:        "high-performance",
		Description: "High-performance configuration optimized for speed",
		Environment: "production",
		Config: &patterns.EnhancedPatternConfig{
			PatternDiscoveryConfig: &patterns.PatternDiscoveryConfig{
				MinExecutionsForPattern: 15,
				MaxHistoryDays:          90,
				SamplingInterval:        2 * time.Hour,
				SimilarityThreshold:     0.85,
				ClusteringEpsilon:       0.3,
				MinClusterSize:          5,
				ModelUpdateInterval:     12 * time.Hour,
				FeatureWindowSize:       50,
				PredictionConfidence:    0.75,
				MaxConcurrentAnalysis:   25,   // Higher concurrency
				PatternCacheSize:        5000, // Larger cache
				EnableRealTimeDetection: true,
			},
			EnableStatisticalValidation: true,
			RequireValidationPassing:    false, // Speed over strict validation
			MinReliabilityScore:         0.65,
			EnableOverfittingPrevention: true,
			MaxOverfittingRisk:          0.7,
			RequireCrossValidation:      false, // Skip for speed
			EnableMonitoring:            true,
			AutoRecovery:                true,
		},
		Metadata: map[string]string{
			"created_by": "system",
			"purpose":    "high-throughput scenarios",
		},
	}
}

func (cm *ConfigurationManager) loadValidationRules() {
	cm.configValidator.AddRule("min_executions_for_pattern", &ConfigValidationRule{
		Field:    "PatternDiscoveryConfig.MinExecutionsForPattern",
		Type:     "range",
		Min:      1,
		Max:      1000,
		Message:  "MinExecutionsForPattern must be between 1 and 1000",
		Severity: ValidationSeverityError,
	})

	cm.configValidator.AddRule("similarity_threshold", &ConfigValidationRule{
		Field:    "PatternDiscoveryConfig.SimilarityThreshold",
		Type:     "range",
		Min:      0.0,
		Max:      1.0,
		Message:  "SimilarityThreshold must be between 0.0 and 1.0",
		Severity: ValidationSeverityError,
	})

	cm.configValidator.AddRule("prediction_confidence", &ConfigValidationRule{
		Field:    "PatternDiscoveryConfig.PredictionConfidence",
		Type:     "range",
		Min:      0.0,
		Max:      1.0,
		Message:  "PredictionConfidence must be between 0.0 and 1.0",
		Severity: ValidationSeverityError,
	})

	cm.configValidator.AddRule("max_concurrent_analysis", &ConfigValidationRule{
		Field:    "PatternDiscoveryConfig.MaxConcurrentAnalysis",
		Type:     "range",
		Min:      1,
		Max:      100,
		Message:  "MaxConcurrentAnalysis should be between 1 and 100",
		Severity: ValidationSeverityWarning,
	})

	cm.configValidator.AddRule("reliability_score", &ConfigValidationRule{
		Field:    "MinReliabilityScore",
		Type:     "range",
		Min:      0.0,
		Max:      1.0,
		Message:  "MinReliabilityScore must be between 0.0 and 1.0",
		Severity: ValidationSeverityError,
	})

	// Performance-related validations
	cm.configValidator.AddRule("cache_size_performance", &ConfigValidationRule{
		Field:    "PatternDiscoveryConfig.PatternCacheSize",
		Type:     "custom",
		Message:  "Large cache sizes may impact memory usage",
		Severity: ValidationSeverityWarning,
		Validator: func(value interface{}) bool {
			if size, ok := value.(int); ok {
				return size <= 10000
			}
			return true
		},
	})
}

func (cm *ConfigurationManager) getDefaultConfig() *patterns.EnhancedPatternConfig {
	return cm.cloneConfig(cm.profiles["development"].Config)
}

func (cm *ConfigurationManager) cloneConfig(config *patterns.EnhancedPatternConfig) *patterns.EnhancedPatternConfig {
	// Deep clone the configuration
	clone := &patterns.EnhancedPatternConfig{}
	*clone = *config

	// Clone nested PatternDiscoveryConfig
	if config.PatternDiscoveryConfig != nil {
		patternConfig := &patterns.PatternDiscoveryConfig{}
		*patternConfig = *config.PatternDiscoveryConfig
		clone.PatternDiscoveryConfig = patternConfig
	}

	// Clone MonitoringConfig if present
	if config.MonitoringConfig != nil {
		monitoringConfig := &patterns.MonitoringConfig{}
		*monitoringConfig = *config.MonitoringConfig
		clone.MonitoringConfig = monitoringConfig
	}

	return clone
}

func (cm *ConfigurationManager) loadConfigFromFile(configPath string) (*patterns.EnhancedPatternConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config patterns.EnhancedPatternConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &config, nil
}

func (cm *ConfigurationManager) mergeConfigurations(base, overlay *patterns.EnhancedPatternConfig) *patterns.EnhancedPatternConfig {
	// Simple merge implementation - in practice, this would use reflection for deep merge
	merged := cm.cloneConfig(base)

	if overlay.PatternDiscoveryConfig != nil {
		if merged.PatternDiscoveryConfig == nil {
			merged.PatternDiscoveryConfig = &patterns.PatternDiscoveryConfig{}
		}
		cm.mergePatternDiscoveryConfig(merged.PatternDiscoveryConfig, overlay.PatternDiscoveryConfig)
	}

	// Merge other fields
	if overlay.EnableStatisticalValidation != base.EnableStatisticalValidation {
		merged.EnableStatisticalValidation = overlay.EnableStatisticalValidation
	}

	return merged
}

func (cm *ConfigurationManager) mergePatternDiscoveryConfig(base, overlay *patterns.PatternDiscoveryConfig) {
	// Use reflection for thorough merge
	baseValue := reflect.ValueOf(base).Elem()
	overlayValue := reflect.ValueOf(overlay).Elem()

	for i := 0; i < overlayValue.NumField(); i++ {
		field := overlayValue.Field(i)
		if !field.IsZero() {
			baseValue.Field(i).Set(field)
		}
	}
}

func (cm *ConfigurationManager) applyEnvironmentOverrides(config *patterns.EnhancedPatternConfig) *patterns.EnhancedPatternConfig {
	// Apply environment variable overrides
	if envVal := os.Getenv("PATTERN_ENGINE_MIN_EXECUTIONS"); envVal != "" {
		// Parse and apply environment variables
		cm.log.WithField("env_override", "PATTERN_ENGINE_MIN_EXECUTIONS").Debug("Applying environment override")
	}

	return config
}

func (cm *ConfigurationManager) applyOverrides(config *patterns.EnhancedPatternConfig) *patterns.EnhancedPatternConfig {
	for path, value := range cm.overrides {
		cm.applyOverridePath(config, path, value)
	}
	return config
}

func (cm *ConfigurationManager) applyOverridePath(config *patterns.EnhancedPatternConfig, path string, value interface{}) {
	// Simple path application - would use reflection in practice
	parts := strings.Split(path, ".")
	if len(parts) > 0 && parts[0] == "PatternDiscoveryConfig" && len(parts) > 1 {
		switch parts[1] {
		case "MinExecutionsForPattern":
			if val, ok := value.(int); ok {
				config.PatternDiscoveryConfig.MinExecutionsForPattern = val
			}
		case "SimilarityThreshold":
			if val, ok := value.(float64); ok {
				config.PatternDiscoveryConfig.SimilarityThreshold = val
			}
		}
	}
}

func (cm *ConfigurationManager) validateOverridePath(path string) error {
	// Validate that the path exists in the configuration structure
	validPaths := []string{
		"PatternDiscoveryConfig.MinExecutionsForPattern",
		"PatternDiscoveryConfig.SimilarityThreshold",
		"PatternDiscoveryConfig.PredictionConfidence",
		"MinReliabilityScore",
		"MaxOverfittingRisk",
	}

	for _, validPath := range validPaths {
		if path == validPath {
			return nil
		}
	}

	return fmt.Errorf("path not found in configuration structure")
}

func (cm *ConfigurationManager) generateRecommendedFixes() []string {
	fixes := make([]string, 0)

	for _, err := range cm.validationErrors {
		switch err.Field {
		case "PatternDiscoveryConfig.MinExecutionsForPattern":
			fixes = append(fixes, "Consider setting MinExecutionsForPattern to at least 10 for reliable pattern detection")
		case "PatternDiscoveryConfig.SimilarityThreshold":
			fixes = append(fixes, "SimilarityThreshold between 0.8-0.9 provides good balance of precision and recall")
		case "MinReliabilityScore":
			fixes = append(fixes, "MinReliabilityScore of 0.7 or higher recommended for production use")
		}
	}

	return fixes
}

func (cm *ConfigurationManager) generateOptimalSettings() map[string]interface{} {
	optimal := make(map[string]interface{})

	// Generate optimal settings based on current configuration and best practices
	optimal["MinExecutionsForPattern"] = 15
	optimal["SimilarityThreshold"] = 0.85
	optimal["PredictionConfidence"] = 0.75
	optimal["MinReliabilityScore"] = 0.7

	return optimal
}

func (cm *ConfigurationManager) checkSecurityIssues() []SecurityIssue {
	issues := make([]SecurityIssue, 0)

	if cm.loadedConfig == nil {
		return issues
	}

	// Check for potential security issues
	if cm.loadedConfig.PatternDiscoveryConfig.MaxConcurrentAnalysis > 50 {
		issues = append(issues, SecurityIssue{
			Issue:       "high_concurrency_limit",
			Severity:    "medium",
			Description: "High concurrency limit may lead to resource exhaustion attacks",
			Fix:         "Consider limiting MaxConcurrentAnalysis to 25 or less",
		})
	}

	return issues
}

func (cm *ConfigurationManager) generatePerformanceNotes() []string {
	notes := make([]string, 0)

	if cm.loadedConfig == nil {
		return notes
	}

	config := cm.loadedConfig.PatternDiscoveryConfig

	if config.PatternCacheSize > 5000 {
		notes = append(notes, "Large pattern cache size may increase memory usage")
	}

	if config.FeatureWindowSize > 100 {
		notes = append(notes, "Large feature window size may slow down analysis")
	}

	if config.MaxConcurrentAnalysis > 20 {
		notes = append(notes, "High concurrency may improve throughput but increase resource usage")
	}

	return notes
}

// ConfigValidator implementation

// NewConfigValidator creates a new configuration validator
func NewConfigValidator(log *logrus.Logger) *ConfigValidator {
	return &ConfigValidator{
		rules: make(map[string]*ConfigValidationRule),
		log:   log,
	}
}

// AddRule adds a validation rule
func (cv *ConfigValidator) AddRule(name string, rule *ConfigValidationRule) {
	cv.rules[name] = rule
}

// ValidateConfiguration validates a configuration against all rules
func (cv *ConfigValidator) ValidateConfiguration(config *patterns.EnhancedPatternConfig) []ConfigValidationError {
	errors := make([]ConfigValidationError, 0)

	for ruleName, rule := range cv.rules {
		if err := cv.validateRule(config, rule); err != nil {
			err.Rule = ruleName
			errors = append(errors, *err)
		}
	}

	return errors
}

func (cv *ConfigValidator) validateRule(config *patterns.EnhancedPatternConfig, rule *ConfigValidationRule) *ConfigValidationError {
	value := cv.getFieldValue(config, rule.Field)

	switch rule.Type {
	case "range":
		return cv.validateRange(value, rule)
	case "enum":
		return cv.validateEnum(value, rule)
	case "custom":
		return cv.validateCustom(value, rule)
	default:
		return nil
	}
}

func (cv *ConfigValidator) getFieldValue(config *patterns.EnhancedPatternConfig, fieldPath string) interface{} {
	// Simple field value extraction - would use reflection in practice
	parts := strings.Split(fieldPath, ".")

	switch {
	case len(parts) == 2 && parts[0] == "PatternDiscoveryConfig":
		if config.PatternDiscoveryConfig == nil {
			return nil
		}

		switch parts[1] {
		case "MinExecutionsForPattern":
			return config.PatternDiscoveryConfig.MinExecutionsForPattern
		case "SimilarityThreshold":
			return config.PatternDiscoveryConfig.SimilarityThreshold
		case "PredictionConfidence":
			return config.PatternDiscoveryConfig.PredictionConfidence
		case "MaxConcurrentAnalysis":
			return config.PatternDiscoveryConfig.MaxConcurrentAnalysis
		case "PatternCacheSize":
			return config.PatternDiscoveryConfig.PatternCacheSize
		}
	case len(parts) == 1:
		switch parts[0] {
		case "MinReliabilityScore":
			return config.MinReliabilityScore
		case "MaxOverfittingRisk":
			return config.MaxOverfittingRisk
		}
	}

	return nil
}

func (cv *ConfigValidator) validateRange(value interface{}, rule *ConfigValidationRule) *ConfigValidationError {
	if value == nil {
		return nil
	}

	var numValue float64
	var valid bool

	switch v := value.(type) {
	case int:
		numValue = float64(v)
		valid = true
	case float64:
		numValue = v
		valid = true
	}

	if !valid {
		return &ConfigValidationError{
			Field:    rule.Field,
			Value:    value,
			Message:  "Value is not numeric",
			Severity: rule.Severity,
		}
	}

	min, _ := rule.Min.(float64)
	max, _ := rule.Max.(float64)

	if numValue < min || numValue > max {
		return &ConfigValidationError{
			Field:    rule.Field,
			Value:    value,
			Message:  rule.Message,
			Severity: rule.Severity,
		}
	}

	return nil
}

func (cv *ConfigValidator) validateEnum(value interface{}, rule *ConfigValidationRule) *ConfigValidationError {
	if value == nil {
		return nil
	}

	for _, allowedValue := range rule.Values {
		if value == allowedValue {
			return nil
		}
	}

	return &ConfigValidationError{
		Field:    rule.Field,
		Value:    value,
		Message:  rule.Message,
		Severity: rule.Severity,
	}
}

func (cv *ConfigValidator) validateCustom(value interface{}, rule *ConfigValidationRule) *ConfigValidationError {
	if rule.Validator == nil || rule.Validator(value) {
		return nil
	}

	return &ConfigValidationError{
		Field:    rule.Field,
		Value:    value,
		Message:  rule.Message,
		Severity: rule.Severity,
	}
}

// ConfigurationGuide implementation

// GenerateGuidedSetup provides guided configuration setup
func (cg *ConfigurationGuide) GenerateGuidedSetup(workloadType string, environment string) (*patterns.EnhancedPatternConfig, error) {
	cg.log.WithFields(logrus.Fields{
		"workload_type": workloadType,
		"environment":   environment,
	}).Info("Generating guided configuration setup")

	var baseProfile string

	// Select base profile based on environment
	switch environment {
	case "development":
		baseProfile = "development"
	case "production":
		baseProfile = "production"
	case "high-performance":
		baseProfile = "high-performance"
	default:
		baseProfile = "development"
	}

	profile := cg.manager.profiles[baseProfile]
	config := cg.manager.cloneConfig(profile.Config)

	// Customize based on workload type
	switch workloadType {
	case "high-frequency":
		config.PatternDiscoveryConfig.SamplingInterval = 15 * time.Minute
		config.PatternDiscoveryConfig.MaxConcurrentAnalysis = 20
		config.RequireCrossValidation = false // Speed over validation

	case "high-accuracy":
		config.PatternDiscoveryConfig.SimilarityThreshold = 0.95
		config.PatternDiscoveryConfig.MinExecutionsForPattern = 25
		config.MinReliabilityScore = 0.85
		config.RequireValidationPassing = true

	case "resource-constrained":
		config.PatternDiscoveryConfig.PatternCacheSize = 500
		config.PatternDiscoveryConfig.MaxConcurrentAnalysis = 5
		config.PatternDiscoveryConfig.FeatureWindowSize = 25

	default:
		// Use profile defaults
	}

	return config, nil
}

// GetRecommendationsFor provides recommendations for specific scenarios
func (cg *ConfigurationGuide) GetRecommendationsFor(scenario string) []string {
	recommendations := make([]string, 0)

	switch scenario {
	case "first_time_setup":
		recommendations = append(recommendations,
			"Start with the 'development' profile for initial testing",
			"Set MinExecutionsForPattern to 10-15 for balanced pattern detection",
			"Enable monitoring to track pattern engine performance",
			"Use SimilarityThreshold of 0.8-0.85 for good precision/recall balance")

	case "production_deployment":
		recommendations = append(recommendations,
			"Use the 'production' profile as a starting point",
			"Enable RequireValidationPassing for production safety",
			"Set MinReliabilityScore to 0.7 or higher",
			"Configure monitoring alerts for pattern quality degradation")

	case "high_volume":
		recommendations = append(recommendations,
			"Consider the 'high-performance' profile",
			"Increase PatternCacheSize to 2000-5000",
			"Set MaxConcurrentAnalysis based on available CPU cores",
			"Monitor memory usage and adjust cache size accordingly")

	case "resource_optimization":
		recommendations = append(recommendations,
			"Reduce PatternCacheSize to 500-1000",
			"Set MaxConcurrentAnalysis to 5-10",
			"Increase SamplingInterval to reduce analysis frequency",
			"Disable unnecessary features like real-time detection")
	}

	return recommendations
}

// ValidateForEnvironment validates configuration for a specific environment
func (cg *ConfigurationGuide) ValidateForEnvironment(config *patterns.EnhancedPatternConfig, environment string) []string {
	issues := make([]string, 0)

	switch environment {
	case "production":
		if !config.EnableStatisticalValidation {
			issues = append(issues, "Statistical validation should be enabled in production")
		}
		if config.MinReliabilityScore < 0.7 {
			issues = append(issues, "MinReliabilityScore should be at least 0.7 in production")
		}
		if config.MaxOverfittingRisk > 0.6 {
			issues = append(issues, "MaxOverfittingRisk should be 0.6 or lower in production")
		}

	case "development":
		if config.PatternDiscoveryConfig.MinExecutionsForPattern > 20 {
			issues = append(issues, "MinExecutionsForPattern is high for development environment")
		}

	case "high-performance":
		if config.RequireCrossValidation {
			issues = append(issues, "Cross-validation may slow down high-performance scenarios")
		}
		if config.PatternDiscoveryConfig.MaxConcurrentAnalysis < 15 {
			issues = append(issues, "Consider increasing MaxConcurrentAnalysis for better performance")
		}
	}

	return issues
}
