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

package audit

// REFACTOR-RO-AUD-002: Actor determination helper for audit events
// Reference: BR-AUTH-001 (SOC 2 CC8.1 User Attribution), ADR-034 v1.7

// DetermineActor determines the actor type and actor ID based on the decidedBy field.
//
// Per ADR-034 v1.7 Two-Event Pattern:
// - Webhook events (category=webhook): Always actor_type="user", actor_id=authenticated username
// - Orchestration events (category=orchestration): Can be "user" or "service" (system expiration)
//
// Parameters:
//   - decidedBy: The DecidedBy field from RAR status (e.g., "kubernetes-admin", "system")
//   - serviceName: The name of the service (used for system actors)
//
// Returns:
//   - actorType: "user" or "service"
//   - actorID: The specific user or service identifier
func DetermineActor(decidedBy, serviceName string) (actorType, actorID string) {
	if decidedBy == "system" {
		// System-initiated decision (e.g., expiration timeout)
		return "service", serviceName
	}

	// User-initiated decision (authenticated by AuthWebhook)
	return "user", decidedBy
}
