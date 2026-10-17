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

package audit

import (
	"fmt"

	"github.com/google/uuid"
)

// canonicalizeEventID establishes the one event identifier used by the
// internal event, the top-level DataStorage request, and typed event data.
// Legacy hand-built events may carry an old non-UUID payload value; the
// DataStorage contract is server-owned, so that value is replaced by a fresh
// canonical UUID rather than being allowed to create two identifiers.
func canonicalizeEventID(event *AuditEvent) error {
	if event == nil {
		return fmt.Errorf("audit event must not be nil")
	}
	legacyID := dataString(event.Data, "event_id")
	if event.EventID == uuid.Nil {
		event.EventID = canonicalEventID(legacyID)
	}
	if event.Data == nil {
		event.Data = make(map[string]interface{})
	}
	event.Data["event_id"] = event.EventID.String()
	return nil
}

func canonicalEventID(legacyID string) uuid.UUID {
	if legacyID == "" {
		return uuid.New()
	}
	parsed, err := uuid.Parse(legacyID)
	if err != nil {
		return uuid.New()
	}
	return parsed
}
