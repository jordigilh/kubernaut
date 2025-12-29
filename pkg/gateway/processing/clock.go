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

package processing

import "time"

// Clock provides time-related operations for dependency injection
//
// This interface enables:
// - Fast, deterministic tests (no time.Sleep() needed)
// - Time-based behavior testing without wall-clock dependency
// - Better test isolation and reliability
//
// Usage:
// - Production: Use RealClock for actual time
// - Tests: Use MockClock with controllable time
type Clock interface {
	// Now returns the current time
	Now() time.Time
}

// RealClock provides actual system time for production use
type RealClock struct{}

// NewRealClock creates a new RealClock instance
func NewRealClock() *RealClock {
	return &RealClock{}
}

// Now returns the current system time
func (c *RealClock) Now() time.Time {
	return time.Now()
}

// MockClock provides controllable time for testing
//
// Usage in tests:
//
//	clock := NewMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
//	crdCreator := NewCRDCreator(..., clock)
//
//	// First call
//	rr1, _ := crdCreator.CreateRemediationRequest(ctx, signal)
//
//	// Advance time for uniqueness testing
//	clock.Advance(1 * time.Second)
//
//	// Second call (different timestamp)
//	rr2, _ := crdCreator.CreateRemediationRequest(ctx, signal)
type MockClock struct {
	currentTime time.Time
}

// NewMockClock creates a new MockClock with the specified initial time
func NewMockClock(initialTime time.Time) *MockClock {
	return &MockClock{
		currentTime: initialTime,
	}
}

// Now returns the current mock time
func (c *MockClock) Now() time.Time {
	return c.currentTime
}

// Advance moves the mock clock forward by the specified duration
//
// This enables fast testing of time-dependent behavior without sleep:
//
//	clock.Advance(1 * time.Second)  // Instant, no actual wait
func (c *MockClock) Advance(d time.Duration) {
	c.currentTime = c.currentTime.Add(d)
}

// Set sets the mock clock to a specific time
func (c *MockClock) Set(t time.Time) {
	c.currentTime = t
}


