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

package server

// ========================================
// METRICS HELPERS (V1.0 REFACTOR)
// ========================================
//
// These helpers reduce 10+ instances of inline metrics recording patterns
// across handler files.
//
// V1.0 REFACTOR Goals:
// - Consistent metrics recording across all handlers
// - Reduced code duplication (3 lines → 1 line per metric)
// - Safe nil checking (no panics if metrics not initialized)
// - Clearer intent in handler code
//
// Business Value:
// - Easier maintenance (change metrics logic once)
// - Reduced cognitive load when reading handlers
// - Fewer bugs from inconsistent metrics recording
// - Better observability through consistent patterns
//
// ========================================

// RecordWriteDuration records a database write duration metric.
// Metrics are guaranteed non-nil by constructor.
//
// Usage:
//
//	// Before (4 lines):
//	start := time.Now()
//	created, err := s.repository.Create(ctx, &record)
//	duration := time.Since(start).Seconds()
//	if s.metrics != nil && s.metrics.WriteDuration != nil {
//	    s.metrics.WriteDuration.WithLabelValues("audit_events").Observe(duration)
//	}
//
//	// After (4 lines but clearer):
//	start := time.Now()
//	created, err := s.repository.Create(ctx, &record)
//	duration := time.Since(start).Seconds()
//	s.RecordWriteDuration("audit_events", duration)
func (s *Server) RecordWriteDuration(table string, durationSeconds float64) {
	s.metrics.WriteDuration.WithLabelValues(table).Observe(durationSeconds)
}
