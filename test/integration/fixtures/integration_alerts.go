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
