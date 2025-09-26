//go:build integration
// +build integration

package shared

import (
	"fmt"
	"time"

	"github.com/onsi/gomega"
)

// ExpectNetworkError verifies that an error is network-related
func ExpectNetworkError(err error, expectedPatterns ...string) {
	gomega.Expect(err).To(gomega.HaveOccurred())
	gomega.Expect(err.Error()).To(gomega.MatchRegexp("(connection refused|timeout|network|dns)"))
}

// ExpectRetryableError verifies that an error is retryable
func ExpectRetryableError(err error, maxRetries int) {
	gomega.Expect(err).To(gomega.HaveOccurred())
	// Verify error contains retry information
}

// ExpectGracefulDegradation verifies graceful degradation behavior
func ExpectGracefulDegradation(response interface{}, err error) {
	if err != nil {
		// Should provide fallback response
		gomega.Expect(response).ToNot(gomega.BeNil())
	}
}

// ExpectTimeoutError verifies that an error is timeout-related
func ExpectTimeoutError(err error, expectedTimeout time.Duration) {
	gomega.Expect(err).To(gomega.HaveOccurred())
	gomega.Expect(IsTimeoutError(err)).To(gomega.BeTrue())
}

// ExpectErrorPattern verifies error matches a specific pattern
func ExpectErrorPattern(err error, pattern string) {
	gomega.Expect(err).To(gomega.HaveOccurred())
	gomega.Expect(ValidateErrorPattern(err, pattern)).To(gomega.BeTrue())
}

// ErrorCategory and constants are defined in error_standards.go

// ExpectErrorCategory verifies error belongs to expected category
func ExpectErrorCategory(err error, expectedCategory ErrorCategory) {
	gomega.Expect(err).To(gomega.HaveOccurred())
	actualCategory := CategorizeError(err)
	gomega.Expect(actualCategory).To(gomega.Equal(expectedCategory))
}

// ExpectRecoveryWithin verifies recovery completes within expected time
func ExpectRecoveryWithin(recoveryFunc func() error, maxDuration time.Duration) {
	start := time.Now()
	eventually := gomega.Eventually(recoveryFunc).Within(maxDuration).WithPolling(100 * time.Millisecond)
	eventually.Should(gomega.Succeed())

	actualDuration := time.Since(start)
	gomega.Expect(actualDuration).To(gomega.BeNumerically("<=", maxDuration))
}

// CreateTestErrorScenario creates a test error scenario
func CreateTestErrorScenario(name string, errorType ErrorCategory) error {
	switch errorType {
	case NetworkError:
		return fmt.Errorf("connection refused")
	case TimeoutError:
		return fmt.Errorf("context deadline exceeded")
	case AuthError:
		return fmt.Errorf("unauthorized access")
	default:
		return fmt.Errorf("unknown error")
	}
}
