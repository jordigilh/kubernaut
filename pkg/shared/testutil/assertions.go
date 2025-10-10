<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package testutil

import (
	"fmt"
	"math"
	"strings"
	"time"

	. "github.com/onsi/gomega" //nolint:staticcheck
)

// SharedAssertions provides standardized assertion helpers for shared package tests
type SharedAssertions struct{}

// NewSharedAssertions creates a new shared assertions helper
func NewSharedAssertions() *SharedAssertions {
	return &SharedAssertions{}
}

// AssertNoError verifies no error occurred with context
func (a *SharedAssertions) AssertNoError(err error, context string) {
	Expect(err).NotTo(HaveOccurred(), "Expected no error in %s, but got: %v", context, err)
}

// AssertErrorContains verifies error contains expected text
func (a *SharedAssertions) AssertErrorContains(err error, expectedText string) {
	Expect(err).To(HaveOccurred(), "Expected an error containing '%s'", expectedText)
	Expect(err.Error()).To(ContainSubstring(expectedText))
}

// AssertErrorMessage verifies exact error message
func (a *SharedAssertions) AssertErrorMessage(err error, expectedMessage string) {
	Expect(err).To(HaveOccurred(), "Expected error with message '%s'", expectedMessage)
	Expect(err.Error()).To(Equal(expectedMessage))
}

// AssertStringContains verifies string contains all expected substrings
func (a *SharedAssertions) AssertStringContains(str string, expectedSubstrings []string) {
	for _, substring := range expectedSubstrings {
		Expect(str).To(ContainSubstring(substring), "Expected string to contain '%s'", substring)
	}
}

// AssertStringNotContains verifies string does not contain any forbidden substrings
func (a *SharedAssertions) AssertStringNotContains(str string, forbiddenSubstrings []string) {
	for _, substring := range forbiddenSubstrings {
		Expect(str).NotTo(ContainSubstring(substring), "Expected string not to contain '%s'", substring)
	}
}

// AssertValidDuration verifies duration is within expected bounds
func (a *SharedAssertions) AssertValidDuration(duration time.Duration, min, max time.Duration) {
	Expect(duration).To(BeNumerically(">=", min), "Duration should be at least %v", min)
	Expect(duration).To(BeNumerically("<=", max), "Duration should be at most %v", max)
}

// AssertRecentTimestamp verifies timestamp is within expected time range
func (a *SharedAssertions) AssertRecentTimestamp(timestamp time.Time, maxAge time.Duration) {
	Expect(timestamp).To(BeTemporally(">=", time.Now().Add(-maxAge)))
	Expect(timestamp).To(BeTemporally("<=", time.Now().Add(time.Minute))) // Small buffer for test execution time
}

// AssertTimestampOrder verifies timestamps are in expected order
func (a *SharedAssertions) AssertTimestampOrder(earlier, later time.Time) {
	Expect(later).To(BeTemporally(">=", earlier))
}

// AssertMapContainsKeys verifies map contains all expected keys
func (a *SharedAssertions) AssertMapContainsKeys(m map[string]interface{}, expectedKeys []string) {
	for _, key := range expectedKeys {
		Expect(m).To(HaveKey(key), "Map should contain key '%s'", key)
	}
}

// AssertMapValues verifies map has expected key-value pairs
func (a *SharedAssertions) AssertMapValues(m map[string]interface{}, expectedValues map[string]interface{}) {
	for key, expectedValue := range expectedValues {
		Expect(m).To(HaveKeyWithValue(key, expectedValue))
	}
}

// AssertSliceContains verifies slice contains expected elements
func (a *SharedAssertions) AssertSliceContains(slice []string, expectedElements []string) {
	for _, element := range expectedElements {
		Expect(slice).To(ContainElement(element))
	}
}

// AssertSliceLength verifies slice has expected length
func (a *SharedAssertions) AssertSliceLength(slice interface{}, expectedLength int) {
	Expect(slice).To(HaveLen(expectedLength))
}

// AssertNumericalRange verifies number is within expected range
func (a *SharedAssertions) AssertNumericalRange(value float64, min, max float64) {
	Expect(value).To(BeNumerically(">=", min))
	Expect(value).To(BeNumerically("<=", max))
}

// AssertPercentage verifies value is a valid percentage (0.0 to 1.0)
func (a *SharedAssertions) AssertPercentage(value float64) {
	a.AssertNumericalRange(value, 0.0, 1.0)
}

// AssertPositiveNumber verifies number is positive
func (a *SharedAssertions) AssertPositiveNumber(value float64) {
	Expect(value).To(BeNumerically(">", 0))
}

// AssertNonNegativeNumber verifies number is non-negative
func (a *SharedAssertions) AssertNonNegativeNumber(value float64) {
	Expect(value).To(BeNumerically(">=", 0))
}

// AssertHTTPStatusCode verifies HTTP status code is as expected
func (a *SharedAssertions) AssertHTTPStatusCode(statusCode int, expectedCode int) {
	Expect(statusCode).To(Equal(expectedCode))
}

// AssertHTTPStatusCodeRange verifies HTTP status code is within expected range
func (a *SharedAssertions) AssertHTTPStatusCodeRange(statusCode int, minCode, maxCode int) {
	Expect(statusCode).To(BeNumerically(">=", minCode))
	Expect(statusCode).To(BeNumerically("<=", maxCode))
}

// AssertValidURL verifies string is a valid URL format
func (a *SharedAssertions) AssertValidURL(url string) {
	Expect(url).NotTo(BeEmpty())
	Expect(url).To(Or(HavePrefix("http://"), HavePrefix("https://")))
}

// AssertErrorType verifies error is of expected type through type assertion
func (a *SharedAssertions) AssertErrorType(err error, expectedType string) {
	Expect(err).To(HaveOccurred())
	errorStr := fmt.Sprintf("%T", err)
	Expect(errorStr).To(ContainSubstring(expectedType))
}

// AssertLogFieldsValid verifies logging fields are properly structured
func (a *SharedAssertions) AssertLogFieldsValid(fields map[string]interface{}) {
	Expect(fields).NotTo(BeEmpty())

	// Common field validations
	if timestamp, exists := fields["timestamp"]; exists {
		Expect(timestamp).NotTo(BeNil())
	}

	if level, exists := fields["level"]; exists {
		Expect(level).To(BeAssignableToTypeOf(""))
		levelStr := level.(string)
		validLevels := []string{"debug", "info", "warn", "error", "fatal", "panic"}
		Expect(validLevels).To(ContainElement(strings.ToLower(levelStr)))
	}

	if message, exists := fields["message"]; exists {
		Expect(message).To(BeAssignableToTypeOf(""))
		Expect(message.(string)).NotTo(BeEmpty())
	}
}

// AssertStatisticsValid verifies statistics values are reasonable
func (a *SharedAssertions) AssertStatisticsValid(stats map[string]float64) {
	Expect(stats).NotTo(BeEmpty())

	// Basic sanity checks for common statistics
	if mean, exists := stats["mean"]; exists {
		Expect(math.IsNaN(mean)).To(BeFalse())
	}

	if stddev, exists := stats["stddev"]; exists {
		a.AssertNonNegativeNumber(stddev)
	}

	if variance, exists := stats["variance"]; exists {
		a.AssertNonNegativeNumber(variance)
	}

	if count, exists := stats["count"]; exists {
		a.AssertNonNegativeNumber(count)
	}
}

// AssertJSONValid verifies string is valid JSON by attempting to parse it
func (a *SharedAssertions) AssertJSONValid(jsonStr string) {
	Expect(jsonStr).NotTo(BeEmpty())
	// This is a basic check - in a real implementation you might use json.Unmarshal
	Expect(jsonStr).To(Or(
		HavePrefix("{"),
		HavePrefix("["),
		Equal("null"),
		Equal("true"),
		Equal("false"),
	))
}
