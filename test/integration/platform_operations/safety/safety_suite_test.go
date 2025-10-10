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

package safety

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PLAT-SAFETY-001: Platform Safety Framework Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Platform Safety Framework business logic
// Stakeholder Value: Provides executive confidence in Platform Safety Framework testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Platform Safety Framework capabilities
// Business Impact: Ensures all Platform Safety Framework components deliver measurable system reliability
// Business Outcome: Test suite framework enables Platform Safety Framework validation

func TestUsafety(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Platform Safety Framework Suite")
}
