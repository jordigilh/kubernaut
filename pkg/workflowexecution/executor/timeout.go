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

package executor

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// remainingUntilDeadline converts an absolute deadline (DD-TIMEOUT-002 /
// Issue #2176 -- WorkflowExecutionSpec.TimesOutAt) into a duration remaining
// as of now, shared by job.go's ActiveDeadlineSeconds computation and
// ansible.go's TokenRequest TTL sizing. Returns ok=false when deadline is
// nil (no authoritative timeout propagated by RO). The returned duration may
// be zero or negative when the deadline has already passed -- callers are
// responsible for flooring/clamping to whatever positive value their target
// field requires.
func remainingUntilDeadline(deadline *metav1.Time) (remaining time.Duration, ok bool) {
	if deadline == nil {
		return 0, false
	}
	return time.Until(deadline.Time), true
}
