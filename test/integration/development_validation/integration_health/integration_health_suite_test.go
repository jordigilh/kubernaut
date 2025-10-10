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
//go:build integration
// +build integration

package integration_health

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DEV-HEALTH-001: Development Integration Health Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Development Integration Health business logic
// Stakeholder Value: Provides executive confidence in Development Integration Health testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Development Integration Health capabilities
// Business Impact: Ensures all Development Integration Health components deliver measurable system reliability
// Business Outcome: Test suite framework enables Development Integration Health validation

func TestUintegrationUhealth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Development Integration Health Suite")
}
