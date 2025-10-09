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

package failover

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PERF-FAILOVER-001: Performance Failover Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Performance Failover business logic
// Stakeholder Value: Provides executive confidence in Performance Failover testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Performance Failover capabilities
// Business Impact: Ensures all Performance Failover components deliver measurable system reliability
// Business Outcome: Test suite framework enables Performance Failover validation

func TestUfailover(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Failover Suite")
}
