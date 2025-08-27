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

	// Add edge case categories (these would be defined in separate files)
	// allEdgeCases = append(allEdgeCases, ChaosEngineeringAlerts...)
	// allEdgeCases = append(allEdgeCases, SecurityComplianceAlerts...)
	// allEdgeCases = append(allEdgeCases, ResourceExhaustionAlerts...)
	// allEdgeCases = append(allEdgeCases, CascadingFailureAlerts...)

	return allEdgeCases
}
