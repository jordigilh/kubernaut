/*
Copyright 2026 Jordi Gil.

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
	"fmt"
	"sync"
	"testing"
)

// TestAuditIdempotencyRace validates that concurrent calls to markAuditEventEmitted
// do not trigger a data race. Run with: go test -race ./internal/controller/notification/ -run TestAuditIdempotencyRace
// Ref: NT-H4 (#1356) — nested *sync.Map replaces raw map[string]bool to eliminate race.
func TestAuditIdempotencyRace(t *testing.T) {
	r := &NotificationRequestReconciler{}

	const goroutines = 50
	const key = "race-test-notification-uid"

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			eventType := fmt.Sprintf("event.type.%d", idx)
			r.markAuditEventEmitted(key, eventType)
		}(i)
	}
	wg.Wait()

	for i := 0; i < goroutines; i++ {
		eventType := fmt.Sprintf("event.type.%d", i)
		if r.shouldEmitAuditEvent(key, eventType) {
			t.Errorf("expected event %q to be marked as emitted, but shouldEmitAuditEvent returned true", eventType)
		}
	}

	// Verify a never-emitted event type still returns true
	if !r.shouldEmitAuditEvent(key, "never.emitted") {
		t.Error("expected shouldEmitAuditEvent to return true for a never-emitted event type")
	}

	// Verify cleanup removes all tracking
	r.cleanupAuditEventTracking(key)
	if !r.shouldEmitAuditEvent(key, "event.type.0") {
		t.Error("expected shouldEmitAuditEvent to return true after cleanup")
	}
}
