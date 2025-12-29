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

package testutil

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/onsi/ginkgo/v2"
)

// testCounter provides sequential uniqueness within a test run
// Uses atomic operations for thread-safety across parallel test processes
var testCounter uint64

// UniqueTestSuffix generates a unique suffix for test resource names.
//
// CRITICAL: Uses nanosecond precision to avoid collisions in parallel test execution.
//
// Pattern matches Gateway service (test/integration/gateway/adapter_interaction_test.go:51):
// - Nanosecond timestamp (primary uniqueness)
// - Ginkgo random seed (test run isolation)
// - Atomic counter (sequential safety)
//
// Why this three-way approach:
// - Nanosecond timestamp: Prevents same-moment collisions
// - Random seed: Isolates different test runs
// - Counter: Handles rapid sequential creation
//
// Usage:
//
//	name := fmt.Sprintf("test-resource-%s", testutil.UniqueTestSuffix())
//
// DO NOT USE: time.Now().Format("20060102150405") - causes parallel test failures
// USE INSTEAD: testutil.UniqueTestSuffix()
func UniqueTestSuffix() string {
	counter := atomic.AddUint64(&testCounter, 1)
	return fmt.Sprintf("%d-%d-%d",
		time.Now().UnixNano(),
		ginkgo.GinkgoRandomSeed(),
		counter,
	)
}

// UniqueTestName creates a unique test resource name with a given prefix.
//
// Pattern matches Gateway service for maximum collision avoidance.
//
// Usage:
//
//	analysis := &aianalysisv1alpha1.AIAnalysis{
//	    ObjectMeta: metav1.ObjectMeta{
//	        Name: testutil.UniqueTestName("integration-test"),
//	        Namespace: "default",
//	    },
//	}
func UniqueTestName(prefix string) string {
	counter := atomic.AddUint64(&testCounter, 1)
	return fmt.Sprintf("%s-%d-%d-%d",
		prefix,
		time.Now().UnixNano(),
		ginkgo.GinkgoRandomSeed(),
		counter,
	)
}

// UniqueTestNameWithProcess creates a unique test resource name including process ID.
//
// Use this when you need explicit process isolation (e.g., shared external resources).
//
// Pattern matches Gateway service (test/integration/gateway/adapter_interaction_test.go:148):
//
//	uniqueAlertName := fmt.Sprintf("PodCrashLoop-p%d-%d",
//	    GinkgoParallelProcess(),
//	    time.Now().UnixNano())
//
// Usage:
//
//	name := testutil.UniqueTestNameWithProcess("alert")
//	// Returns: "alert-p2-1765494131234567890-12345-42"
func UniqueTestNameWithProcess(prefix string) string {
	counter := atomic.AddUint64(&testCounter, 1)
	return fmt.Sprintf("%s-p%d-%d-%d-%d",
		prefix,
		ginkgo.GinkgoParallelProcess(),
		time.Now().UnixNano(),
		ginkgo.GinkgoRandomSeed(),
		counter,
	)
}
