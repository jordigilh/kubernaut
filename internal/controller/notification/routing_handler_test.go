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
	"reflect"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// TestCollectAllCredentialRefs is a characterization test for the credential
// ref collectors used by ReloadRoutingFromContent (BR-NOT-067, BR-NOT-104).
// It pins the aggregation order (Slack, then PagerDuty, then Teams) and the
// exact set of refs returned, so that the prealloc capacity-hint refactor in
// collectAllCredentialRefs cannot silently change behavior.
// Run with: go test ./internal/controller/notification/ -run TestCollectAllCredentialRefs
func TestCollectAllCredentialRefs(t *testing.T) {
	config := &routing.Config{
		Receivers: []*routing.Receiver{
			{
				Name: "slack-receiver",
				SlackConfigs: []routing.SlackConfig{
					{Channel: "#alerts", CredentialRef: "slack-cred-1"},
					{Channel: "#ops", CredentialRef: ""}, // empty refs must be skipped
				},
			},
			{
				Name: "pagerduty-receiver",
				PagerDutyConfigs: []routing.PagerDutyConfig{
					{CredentialRef: "pd-cred-1"},
				},
			},
			{
				Name: "teams-receiver",
				TeamsConfigs: []routing.TeamsConfig{
					{CredentialRef: "teams-cred-1"},
					{CredentialRef: "teams-cred-2"},
				},
			},
		},
	}

	got := collectAllCredentialRefs(config)
	want := []string{"slack-cred-1", "pd-cred-1", "teams-cred-1", "teams-cred-2"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectAllCredentialRefs() = %v, want %v", got, want)
	}
}

// TestCollectAllCredentialRefs_Empty pins the behavior for a config with no
// credential-bound channels: an empty, non-nil slice.
func TestCollectAllCredentialRefs_Empty(t *testing.T) {
	config := &routing.Config{Receivers: []*routing.Receiver{{Name: "console-only"}}}

	got := collectAllCredentialRefs(config)

	if len(got) != 0 {
		t.Errorf("collectAllCredentialRefs() = %v, want empty slice", got)
	}
}
