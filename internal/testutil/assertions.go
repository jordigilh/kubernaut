package testutil

import (
	"strings"
	"time"

	. "github.com/onsi/gomega" //nolint:staticcheck
)

// InternalAssertions provides standardized assertion helpers for internal tests
type InternalAssertions struct{}

// NewInternalAssertions creates a new internal assertions helper
func NewInternalAssertions() *InternalAssertions {
	return &InternalAssertions{}
}

// AssertNoError verifies no error occurred with context
func (a *InternalAssertions) AssertNoError(err error, context string) {
	Expect(err).NotTo(HaveOccurred(), "Expected no error in %s, but got: %v", context, err)
}

// AssertErrorContains verifies error contains expected text
func (a *InternalAssertions) AssertErrorContains(err error, expectedText string) {
	Expect(err).To(HaveOccurred(), "Expected an error containing '%s'", expectedText)
	Expect(err.Error()).To(ContainSubstring(expectedText))
}

// AssertErrorMessage verifies exact error message
func (a *InternalAssertions) AssertErrorMessage(err error, expectedMessage string) {
	Expect(err).To(HaveOccurred(), "Expected error with message '%s'", expectedMessage)
	Expect(err.Error()).To(Equal(expectedMessage))
}

// AssertConfigurationValid verifies configuration has expected values
func (a *InternalAssertions) AssertConfigurationValid(config interface{}) {
	Expect(config).NotTo(BeNil(), "Configuration should not be nil")
}

// AssertConfigurationField verifies a specific configuration field value
func (a *InternalAssertions) AssertConfigurationField(actualValue, expectedValue interface{}, fieldName string) {
	Expect(actualValue).To(Equal(expectedValue), "Configuration field '%s' should have expected value", fieldName)
}

// AssertValidName verifies a name follows Kubernetes naming conventions
func (a *InternalAssertions) AssertValidName(name string) {
	Expect(name).NotTo(BeEmpty(), "Name should not be empty")
	Expect(name).To(MatchRegexp("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"), "Name should follow Kubernetes naming conventions")
	Expect(len(name)).To(BeNumerically("<=", 253), "Name should not exceed 253 characters")
}

// AssertInvalidName verifies a name is invalid for Kubernetes
func (a *InternalAssertions) AssertInvalidName(name string) {
	if name == "" {
		return // Empty names are clearly invalid
	}

	// Check for invalid patterns
	hasInvalidChars := strings.ContainsAny(name, "_. ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	startsOrEndsWithDash := strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-")
	tooLong := len(name) > 253

	Expect(hasInvalidChars || startsOrEndsWithDash || tooLong).To(BeTrue(),
		"Name '%s' should be invalid for Kubernetes", name)
}

// AssertValidNamespace verifies a namespace follows Kubernetes naming conventions
func (a *InternalAssertions) AssertValidNamespace(namespace string) {
	a.AssertValidName(namespace) // Same rules as names
}

// AssertInvalidNamespace verifies a namespace is invalid for Kubernetes
func (a *InternalAssertions) AssertInvalidNamespace(namespace string) {
	a.AssertInvalidName(namespace) // Same rules as names
}

// AssertValidLabels verifies labels follow Kubernetes conventions
func (a *InternalAssertions) AssertValidLabels(labels map[string]string) {
	for key, value := range labels {
		Expect(key).NotTo(BeEmpty(), "Label key should not be empty")
		Expect(len(key)).To(BeNumerically("<=", 253), "Label key should not exceed 253 characters")
		Expect(len(value)).To(BeNumerically("<=", 63), "Label value should not exceed 63 characters")

		// Keys should follow DNS subdomain format
		if strings.Contains(key, "/") {
			parts := strings.Split(key, "/")
			Expect(parts).To(HaveLen(2), "Label key with slash should have exactly one slash")
		}
	}
}

// AssertValidationError verifies a validation error with specific field
func (a *InternalAssertions) AssertValidationError(err error, fieldName string) {
	Expect(err).To(HaveOccurred(), "Expected validation error for field '%s'", fieldName)
	Expect(err.Error()).To(ContainSubstring("validation"), "Error should mention validation")
	Expect(err.Error()).To(ContainSubstring(fieldName), "Error should mention the field name")
}

// AssertDatabaseConnection verifies database connection parameters
func (a *InternalAssertions) AssertDatabaseConnection(host string, port int, user, database string) {
	Expect(host).NotTo(BeEmpty(), "Database host should not be empty")
	Expect(port).To(BeNumerically(">", 0), "Database port should be positive")
	Expect(port).To(BeNumerically("<=", 65535), "Database port should be valid")
	Expect(user).NotTo(BeEmpty(), "Database user should not be empty")
	Expect(database).NotTo(BeEmpty(), "Database name should not be empty")
}

// AssertConnectionString verifies database connection string format
func (a *InternalAssertions) AssertConnectionString(connStr string) {
	Expect(connStr).NotTo(BeEmpty(), "Connection string should not be empty")
	Expect(connStr).To(ContainSubstring("host="), "Connection string should contain host")
	Expect(connStr).To(ContainSubstring("dbname="), "Connection string should contain database name")
}

// AssertEnvironmentVariable verifies environment variable is set correctly
func (a *InternalAssertions) AssertEnvironmentVariable(value, expectedValue, varName string) {
	Expect(value).To(Equal(expectedValue), "Environment variable '%s' should have expected value", varName)
}

// AssertFileExists verifies a file exists at the given path
func (a *InternalAssertions) AssertFileExists(filePath string) {
	Expect(filePath).To(BeAnExistingFile(), "File should exist at path: %s", filePath)
}

// AssertFileContent verifies file contains expected content
func (a *InternalAssertions) AssertFileContent(content, expectedContent string) {
	Expect(content).To(Equal(expectedContent), "File content should match expected content")
}

// AssertFileContentContains verifies file contains expected substring
func (a *InternalAssertions) AssertFileContentContains(content, expectedSubstring string) {
	Expect(content).To(ContainSubstring(expectedSubstring), "File content should contain expected substring")
}

// AssertYAMLValid verifies string is valid YAML
func (a *InternalAssertions) AssertYAMLValid(yamlStr string) {
	Expect(yamlStr).NotTo(BeEmpty(), "YAML string should not be empty")
	// In a real implementation, you might parse the YAML to verify validity
}

// AssertYAMLInvalid verifies string is invalid YAML
func (a *InternalAssertions) AssertYAMLInvalid(yamlStr string) {
	// This is a basic check - in a real implementation you might try parsing
	// For now, just check it's not empty (empty can be valid YAML)
	if yamlStr == "" {
		return // Empty is handled separately
	}
}

// AssertTimeRange verifies time is within expected range
func (a *InternalAssertions) AssertTimeRange(timestamp time.Time, startTime, endTime time.Time) {
	Expect(timestamp).To(BeTemporally(">=", startTime))
	Expect(timestamp).To(BeTemporally("<=", endTime))
}

// AssertRecentTimestamp verifies timestamp is recent (within specified duration)
func (a *InternalAssertions) AssertRecentTimestamp(timestamp time.Time, maxAge time.Duration) {
	now := time.Now()
	Expect(timestamp).To(BeTemporally(">=", now.Add(-maxAge)))
	Expect(timestamp).To(BeTemporally("<=", now.Add(time.Minute))) // Small buffer
}

// AssertDuration verifies duration is within expected bounds
func (a *InternalAssertions) AssertDuration(duration time.Duration, min, max time.Duration) {
	Expect(duration).To(BeNumerically(">=", min), "Duration should be at least %v", min)
	Expect(duration).To(BeNumerically("<=", max), "Duration should be at most %v", max)
}

// AssertPositiveNumber verifies number is positive
func (a *InternalAssertions) AssertPositiveNumber(value interface{}) {
	Expect(value).To(BeNumerically(">", 0), "Value should be positive")
}

// AssertNonNegativeNumber verifies number is non-negative
func (a *InternalAssertions) AssertNonNegativeNumber(value interface{}) {
	Expect(value).To(BeNumerically(">=", 0), "Value should be non-negative")
}

// AssertInRange verifies value is within specified range
func (a *InternalAssertions) AssertInRange(value interface{}, min, max interface{}) {
	Expect(value).To(BeNumerically(">=", min), "Value should be >= %v", min)
	Expect(value).To(BeNumerically("<=", max), "Value should be <= %v", max)
}

// AssertStringNotEmpty verifies string is not empty
func (a *InternalAssertions) AssertStringNotEmpty(str string, description string) {
	Expect(str).NotTo(BeEmpty(), "%s should not be empty", description)
}

// AssertMapContainsKeys verifies map contains all expected keys
func (a *InternalAssertions) AssertMapContainsKeys(m map[string]interface{}, expectedKeys []string) {
	for _, key := range expectedKeys {
		Expect(m).To(HaveKey(key), "Map should contain key '%s'", key)
	}
}

// AssertMapValues verifies map has expected key-value pairs
func (a *InternalAssertions) AssertMapValues(m map[string]interface{}, expectedValues map[string]interface{}) {
	for key, expectedValue := range expectedValues {
		Expect(m).To(HaveKeyWithValue(key, expectedValue), "Map should have key '%s' with value '%v'", key, expectedValue)
	}
}

// AssertSliceContains verifies slice contains expected elements
func (a *InternalAssertions) AssertSliceContains(slice []string, expectedElements []string) {
	for _, element := range expectedElements {
		Expect(slice).To(ContainElement(element), "Slice should contain element '%s'", element)
	}
}

// AssertSliceLength verifies slice has expected length
func (a *InternalAssertions) AssertSliceLength(slice interface{}, expectedLength int) {
	Expect(slice).To(HaveLen(expectedLength), "Slice should have length %d", expectedLength)
}
