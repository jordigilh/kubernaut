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

package notification

import (
	. "github.com/onsi/ginkgo/v2"
)

// ========================================
// Retry and Exponential Backoff E2E Tests
// ========================================
// BUSINESS REQUIREMENT: BR-NOT-054 - Automatic Retry with Exponential Backoff
//
// 📋 **TEST MIGRATION NOTICE** (2026-01-10)
// ========================================
// All retry logic tests have been **MIGRATED TO INTEGRATION TIER**.
//
// **Why Integration Instead of E2E?**
// - ✅ Mock services provide deterministic failure simulation
// - ✅ Faster execution (~seconds vs ~minutes)
// - ✅ Better coverage of edge cases
// - ✅ No infrastructure dependencies (Kind, file system, etc.)
//
// **What Was Migrated?**
// - ❌ E2E Pending Test: Retry with exponential backoff (PIt)
// - ✅ Integration Test: `test/integration/notification/controller_retry_logic_test.go`
//
// **Coverage Status**:
// - ✅ Retry logic: Integration tier (mock file service failures)
// - ✅ Successful delivery: E2E tier (03_file_delivery_validation_test.go, 06_multi_channel_fanout_test.go)
// - ✅ Multi-channel: E2E tier (06_multi_channel_fanout_test.go, 07_priority_routing_test.go)
//
// **Related**:
// - Integration Test: `test/integration/notification/controller_retry_logic_test.go`
// - Partial Failure: `test/integration/notification/controller_partial_failure_test.go`
// - Design Decision: DD-NOT-006 v2 (FileDeliveryConfig removal)
	// ========================================

var _ = Describe("Retry and Exponential Backoff E2E (BR-NOT-054)", func() {
	// All tests migrated to integration tier
	// See: test/integration/notification/controller_retry_logic_test.go
})
