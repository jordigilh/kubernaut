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
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Ogen enum type converters for audit payloads.
// These cast CRD string values to ogen-generated enum types used in
// structured audit event payloads (DD-AUDIT-004).

func toRemediationApprovalAuditPayloadDecision(s string) api.RemediationApprovalAuditPayloadDecision {
	return api.RemediationApprovalAuditPayloadDecision(s)
}

func toNotificationAuditPayloadType(s string) api.NotificationAuditPayloadType {
	return api.NotificationAuditPayloadType(s)
}

func toNotificationAuditPayloadPriority(s string) api.NotificationAuditPayloadPriority {
	return api.NotificationAuditPayloadPriority(s)
}

func toNotificationAuditPayloadNotificationType(s string) api.NotificationAuditPayloadNotificationType {
	return api.NotificationAuditPayloadNotificationType(s)
}

func toNotificationAuditPayloadFinalStatus(s string) api.NotificationAuditPayloadFinalStatus {
	return api.NotificationAuditPayloadFinalStatus(s)
}
