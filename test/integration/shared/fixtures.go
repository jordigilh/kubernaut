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
package shared

import (
	"github.com/jordigilh/kubernaut/test/integration/fixtures"
)

// Re-export types and variables for compatibility

var IntegrationTestAlerts = fixtures.IntegrationTestAlerts

// Re-export helper functions
var (
	PerformanceTestAlert      = fixtures.PerformanceTestAlert
	ConcurrentTestAlert       = fixtures.ConcurrentTestAlert
	MalformedAlert            = fixtures.MalformedAlert
	ChaosEngineeringTestAlert = fixtures.ChaosEngineeringTestAlert
	SecurityIncidentAlert     = fixtures.SecurityIncidentAlert
	ResourceExhaustionAlert   = fixtures.ResourceExhaustionAlert
	CascadingFailureAlert     = fixtures.CascadingFailureAlert
	GetAllEdgeCaseAlerts      = fixtures.GetAllEdgeCaseAlerts
)
