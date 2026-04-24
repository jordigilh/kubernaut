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

package conversation

// LifecycleManager handles session state transitions based on RAR status.decision
// and RR completion status.
type LifecycleManager struct{}

// NewLifecycleManager creates a lifecycle manager.
func NewLifecycleManager() *LifecycleManager {
	return &LifecycleManager{}
}

// ApplyRARDecision updates session state based on the RAR status.decision field.
//   - "Approved" or "Rejected" -> read-only (decision has been made, review only)
//   - "Expired" -> closed (no further interaction possible)
//   - "" (pending) -> no change (session remains interactive)
func (l *LifecycleManager) ApplyRARDecision(session *Session, decision string) {
	switch decision {
	case "Approved", "Rejected":
		session.State = SessionReadOnly
	case "Expired":
		session.State = SessionClosed
	}
}

// ApplyRRCompletion updates session state based on RR completion/failure.
// Both "Completed" and "Failed" transitions put the session in read-only mode
// so users can review the outcome.
func (l *LifecycleManager) ApplyRRCompletion(session *Session, status string) {
	switch status {
	case "Completed", "Failed":
		session.State = SessionReadOnly
	}
}
