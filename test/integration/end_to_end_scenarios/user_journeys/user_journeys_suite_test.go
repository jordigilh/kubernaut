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

//go:build integration
// +build integration

package user_journeys

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-E2E-JOURNEY-001: E2E User Journeys Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of E2E User Journeys business logic
// Stakeholder Value: Provides executive confidence in E2E User Journeys testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in E2E User Journeys capabilities
// Business Impact: Ensures all E2E User Journeys components deliver measurable system reliability
// Business Outcome: Test suite framework enables E2E User Journeys validation

func TestUuserUjourneys(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E User Journeys Suite")
}
