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

package remediationapprovalrequest

import "time"

// ComputeTimeRemaining returns the human-readable time remaining until requiredBy.
// Uses Go's time.Duration.String() format (e.g. "1m30s", "45s", "0s").
// When requiredBy is in the past, returns "0s".
func ComputeTimeRemaining(requiredBy time.Time, now time.Time) string {
	remaining := requiredBy.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	return remaining.Round(time.Second).String()
}
