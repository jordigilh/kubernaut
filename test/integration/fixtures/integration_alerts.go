package fixtures

// IntegrationTestAlerts contains all test cases for integration testing
var IntegrationTestAlerts []TestCase

func init() {
	// Combine all test alert categories
	IntegrationTestAlerts = append(IntegrationTestAlerts, HighPriorityTestAlerts...)
	IntegrationTestAlerts = append(IntegrationTestAlerts, CoreTestAlerts...)
}

// GetAllEdgeCaseAlerts returns all edge case test scenarios combined
func GetAllEdgeCaseAlerts() []TestCase {
	var allEdgeCases []TestCase

	// Edge case categories are currently commented out for the following reasons:
	// 1. ChaosEngineeringAlerts - Requires complex multi-node failure simulation
	// 2. SecurityComplianceAlerts - Needs security scanning integration
	// 3. ResourceExhaustionAlerts - Requires actual resource stress testing
	// 4. CascadingFailureAlerts - Complex dependency chain testing
	//
	// These will be implemented in future iterations when:
	// - The core functionality is stable
	// - Proper test infrastructure is available
	// - Resource-intensive testing can be isolated to dedicated test environments
	//
	// For now, we focus on core alert processing and database isolation testing

	return allEdgeCases
}
