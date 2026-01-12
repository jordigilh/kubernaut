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

package remediationorchestrator

import (
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // Ginkgo/Gomega convention
)

// GenerateUniqueNamespace creates a unique namespace for parallel test execution
// Format: ro-e2e-p{process}-{uuid}
// This enables parallel E2E tests by providing complete namespace isolation
//
// Pattern: Notification E2E Pattern (test/e2e/notification/05_retry_exponential_backoff_test.go)
// - Uses UUID for guaranteed uniqueness (more reliable than UnixNano which failed in past)
// - Each parallel process gets its own namespace
// - Complete test isolation (no CRD pollution between tests)
// - Automatic cleanup (delete namespace after test)
func GenerateUniqueNamespace() string {
	return fmt.Sprintf("ro-e2e-p%d-%s",
		GinkgoParallelProcess(),
		uuid.New().String()[:8]) // First 8 chars of UUID for brevity
}
