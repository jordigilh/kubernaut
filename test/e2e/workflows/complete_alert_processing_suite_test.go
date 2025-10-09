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

package workflows

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-E2E-001: Complete Alert-to-Remediation Workflow Validation
// Business Impact: Prevents 3-5 production incidents per month through complete workflow validation
// Stakeholder Value: Operations teams can rely on complete automation from alert to resolution
// Success Metrics: Complete resolution within 5-minute SLA, memory utilization reduced below 80%
func TestCompleteAlertProcessingWorkflows(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Complete Alert Processing E2E Workflow Tests Suite")
}

