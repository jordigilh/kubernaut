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

package creator

import (
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Extension key constants for NotificationRequest.Spec.Extensions.
// Issue #416: Label-based notification routing
const (
	ExtKeyNotificationTarget = "notification-target"
	ExtKeyTargetKind         = "target-kind"
	ExtKeyNamespace          = "namespace"
	ExtKeyTeam               = "team"
	ExtKeyOwner              = "owner"
)

// statusPriority defines the worst-wins ordering for notification statuses.
// Lower index = worse status.
var statusPriority = []string{"Failed", "Pending", "InProgress", "Sent"}

// DetermineNotificationTarget decides whether dual NRs are needed based on
// signal vs RCA target comparison. Returns the notification-target value for
// the first NR and whether a second (RCA) NR is required.
//
// Rules:
//   - rcaTarget nil → ("signal", false): signal-only, no RCA
//   - signal == rca → ("both", false): single NR covers both
//   - signal != rca → ("signal", true): two NRs needed
func DetermineNotificationTarget(signalTarget remediationv1.ResourceIdentifier, rcaTarget *remediationv1.ResourceIdentifier) (string, bool) {
	if rcaTarget == nil {
		return "signal", false
	}
	if signalTarget.Kind == rcaTarget.Kind &&
		signalTarget.Name == rcaTarget.Name &&
		signalTarget.Namespace == rcaTarget.Namespace {
		return "both", false
	}
	return "signal", true
}

// BuildNotificationExtensions creates the Extensions map for a NotificationRequest
// carrying all routing-relevant keys from the target resource and owner labels.
func BuildNotificationExtensions(target remediationv1.ResourceIdentifier, notifTarget string, ownerLabels map[string]string) map[string]string {
	ext := map[string]string{
		ExtKeyNotificationTarget: notifTarget,
		ExtKeyTargetKind:         target.Kind,
	}
	if target.Namespace != "" {
		ext[ExtKeyNamespace] = target.Namespace
	}
	if team, ok := ownerLabels[LabelTeam]; ok {
		ext[ExtKeyTeam] = team
	}
	if owner, ok := ownerLabels[LabelOwner]; ok {
		ext[ExtKeyOwner] = owner
	}
	return ext
}

// AggregateNotificationStatus returns the worst status from a list of NR statuses
// using the worst-wins strategy: Failed > Pending > InProgress > Sent.
// Returns empty string for an empty input slice.
func AggregateNotificationStatus(statuses []string) string {
	if len(statuses) == 0 {
		return ""
	}
	worst := statuses[0]
	for _, s := range statuses[1:] {
		if priorityIndex(s) < priorityIndex(worst) {
			worst = s
		}
	}
	return worst
}

func priorityIndex(status string) int {
	for i, s := range statusPriority {
		if s == status {
			return i
		}
	}
	return len(statusPriority)
}
