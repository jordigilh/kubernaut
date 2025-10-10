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

package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-RESOURCE-OPTIMIZATION-SUITE-001: Comprehensive Resource Optimization Business Test Suite Organization
// Business Impact: Ensures comprehensive testing of resource optimization business logic for production reliability
// Stakeholder Value: Operations teams can trust that resource optimization is thoroughly validated

func TestComprehensiveResourceOptimization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Comprehensive Resource Optimization Unit Tests Suite")
}
