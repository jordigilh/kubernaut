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

//go:build e2e
// +build e2e

package disasterrecovery

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DISASTER-RECOVERY-E2E-SUITE-001: Disaster Recovery and Resilience E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of disaster recovery capabilities and system resilience for business continuity
// Stakeholder Value: Executive confidence in business continuity, disaster recovery capabilities, and disaster preparedness

func TestDisasterRecoveryE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Disaster Recovery and Resilience E2E Business Suite")
}
