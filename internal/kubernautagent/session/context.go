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

package session

import katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"

// SessionContext holds typed request-level context for an investigation session.
// Replaces the untyped Metadata map[string]string with structured fields.
// Signal carries the full SignalContext from the original IncidentRequest,
// enabling interactive takeover to inherit security gate parameters.
type SessionContext struct {
	IncidentID    string
	RemediationID string
	CreatedBy     string
	Signal        katypes.SignalContext
}

// ToMap returns a string map for backward compatibility with audit events
// and the SessionSnapshot API response (agentclient.SessionSnapshotMetadata).
// Only non-empty fields are included.
func (c *SessionContext) ToMap() map[string]string {
	m := make(map[string]string)
	if c.IncidentID != "" {
		m["incident_id"] = c.IncidentID
	}
	if c.RemediationID != "" {
		m["remediation_id"] = c.RemediationID
	}
	if c.CreatedBy != "" {
		m["created_by"] = c.CreatedBy
	}
	if c.Signal.Name != "" {
		m["signal_name"] = c.Signal.Name
	}
	if c.Signal.Severity != "" {
		m["severity"] = c.Signal.Severity
	}
	return m
}
