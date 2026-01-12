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

package shared_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSharedUtilities(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Utilities Suite")
}

// ========================================
// SHARED UTILITIES TEST SUITE
// ========================================
//
// Test Organization:
// - backoff/       - Exponential backoff calculations (BR-WE-012, BR-NOT-052)
// - conditions/    - Kubernetes condition helpers
// - hotreload/     - Configuration hot-reload utilities (BR-NOT-051)
// - sanitization/  - Input sanitization (security) [TO BE CREATED]
// - types/         - Shared type utilities (deduplication, enrichment) [TO BE CREATED]
//
// Testing Strategy (per 03-testing-strategy.mdc):
// - **Unit Tests (70%+)**: Pure utilities with no external dependencies
// - **Integration Tests**: Tested by services using these utilities
// - **E2E Tests**: Tested by end-to-end service validation
//
// Rationale:
// Shared utilities are foundational building blocks used across multiple services.
// They MUST have comprehensive unit test coverage because:
// - Pure functions with deterministic behavior
// - No external dependencies to mock
// - Changes affect multiple services
// - High confidence required for production use
//
// Services Using These Utilities:
// - WorkflowExecution: backoff (BR-WE-009, BR-WE-012), conditions (BR-WE-006)
// - Notification: backoff (BR-NOT-052), hotreload (BR-NOT-051), conditions
// - SignalProcessing: conditions
// - RemediationOrchestrator: conditions
// - DataStorage: sanitization (security)
// - Gateway: sanitization (security)
