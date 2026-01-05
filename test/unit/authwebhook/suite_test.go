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

package authwebhook

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Authentication Webhook Unit Tests
//
// Business Requirement: BR-WE-013 (Audit-Tracked Execution Block Clearing)
// SOC2 Compliance: CC8.1 (Attribution), CC7.3 (Immutability), CC7.4 (Completeness)
//
// Per TESTING_GUIDELINES.md:
// - Unit tests focus on specific function/method behavior
// - Unit tests validate business behavior + implementation correctness
// - Shared infrastructure used by WorkflowExecution, RemediationApprovalRequest, NotificationRequest
//
// Defense-in-Depth Strategy (per DD-WEBHOOK-001):
// - Unit tests (70%+): Authenticator + Validator logic in isolation
// - Integration tests (50%): envtest + real CRD operations
// - E2E tests (50%): Kind cluster + full webhook flow
//
// TDD RED Phase: Tests written BEFORE implementation exists

func TestAuthWebhookUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook Unit Suite - BR-AUTH-001 SOC2 Operator Attribution")
}
