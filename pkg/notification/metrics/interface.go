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

package metrics

// ========================================
// METRICS RECORDER INTERFACE (DD-METRICS-001)
// 📋 Design Decision: DD-METRICS-001 | ✅ Dependency Injection Pattern
// See: docs/architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md
// ========================================
//
// Recorder defines the interface for recording notification metrics.
// This enables dependency injection, testability, and compliance with DD-METRICS-001.
//
// WHY INTERFACE-BASED METRICS?
// - ✅ Testability: Controllers can use mock recorders in tests
// - ✅ Isolation: Tests don't pollute global Prometheus registry
// - ✅ Flexibility: Easy to add alternative implementations (e.g., StatsD, DataDog)
// - ✅ DD-METRICS-001 Compliance: Mandatory dependency injection pattern
//
// USAGE IN CONTROLLER:
//   type NotificationRequestReconciler struct {
//       client.Client
//       Metrics metrics.Recorder  // ← Injected dependency
//   }
//
//   func (r *NotificationRequestReconciler) Reconcile(...) {
//       r.Metrics.UpdatePhaseCount(namespace, "Pending", 1)  // ← Use interface
//   }
//
// WIRING IN MAIN:
//   metricsRecorder := metrics.NewPrometheusRecorder()
//   reconciler := &NotificationRequestReconciler{
//       Metrics: metricsRecorder,  // ← Inject implementation
//   }
// ========================================

type Recorder interface {
	// RecordDeliveryAttempt records a delivery attempt (success or failure)
	// Parameters:
	//   - namespace: Kubernetes namespace
	//   - channel: Delivery channel (console, slack, etc.)
	//   - status: Delivery status (success, failure)
	RecordDeliveryAttempt(namespace, channel, status string)

	// RecordDeliveryDuration records the time taken for a delivery
	// Parameters:
	//   - namespace: Kubernetes namespace
	//   - channel: Delivery channel
	//   - durationSeconds: Duration in seconds
	RecordDeliveryDuration(namespace, channel string, durationSeconds float64)

	// UpdateFailureRatio updates the failure ratio metric for a namespace (0-1 scale)
	// Parameters:
	//   - namespace: Kubernetes namespace
	//   - ratio: Failure ratio (0.0 = 0%, 1.0 = 100%)
	UpdateFailureRatio(namespace string, ratio float64)

	// RecordStuckDuration records time spent in Delivering phase
	// Parameters:
	//   - namespace: Kubernetes namespace
	//   - durationSeconds: Duration in seconds
	RecordStuckDuration(namespace string, durationSeconds float64)

	// UpdatePhaseCount updates the count of notifications in a specific phase
	// Parameters:
	//   - namespace: Kubernetes namespace
	//   - phase: Notification phase (Pending, Sending, Sent, Failed, PartiallySent)
	//   - count: Current count
	UpdatePhaseCount(namespace, phase string, count float64)

	// RecordDeliveryRetries records the number of retries for a notification
	// Parameters:
	//   - namespace: Kubernetes namespace
	//   - retries: Number of retries
	RecordDeliveryRetries(namespace string, retries float64)

	// RecordSlackRetry records a Slack API retry attempt
	// Parameters:
	//   - namespace: Kubernetes namespace
	//   - reason: Retry reason (rate_limit, timeout, etc.)
	RecordSlackRetry(namespace, reason string)

	// RecordSlackBackoff records the backoff duration for a Slack API retry
	// Parameters:
	//   - namespace: Kubernetes namespace
	//   - durationSeconds: Backoff duration in seconds
	RecordSlackBackoff(namespace string, durationSeconds float64)
}

// ========================================
// NO-OP RECORDER (Testing / Disabled Metrics)
// ========================================

// NoOpRecorder is a no-op implementation that does nothing.
// Useful for testing when you don't want metrics recorded.
type NoOpRecorder struct{}

// NewNoOpRecorder creates a new no-op metrics recorder
func NewNoOpRecorder() *NoOpRecorder {
	return &NoOpRecorder{}
}

func (n *NoOpRecorder) RecordDeliveryAttempt(namespace, channel, status string) {}
func (n *NoOpRecorder) RecordDeliveryDuration(namespace, channel string, durationSeconds float64) {
}
func (n *NoOpRecorder) UpdateFailureRatio(namespace string, ratio float64)            {}
func (n *NoOpRecorder) RecordStuckDuration(namespace string, durationSeconds float64) {}
func (n *NoOpRecorder) UpdatePhaseCount(namespace, phase string, count float64)       {}
func (n *NoOpRecorder) RecordDeliveryRetries(namespace string, retries float64)       {}
func (n *NoOpRecorder) RecordSlackRetry(namespace, reason string)                     {}
func (n *NoOpRecorder) RecordSlackBackoff(namespace string, durationSeconds float64)  {}

// Compile-time check that NoOpRecorder implements Recorder
var _ Recorder = (*NoOpRecorder)(nil)

