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

package query

import (
	"fmt"
	"strings"
	"time"
)

// ParseTimeParam parses time parameters that can be either:
// - Relative duration: "24h", "7d", "30m" (parsed with time.ParseDuration)
// - Absolute timestamp: "2025-01-15T10:00:00Z" (parsed with RFC3339)
//
// DD-STORAGE-010: Time parsing for query API
// BR-STORAGE-022: Query filtering by time range
func ParseTimeParam(param string) (time.Time, error) {
	if param == "" {
		return time.Time{}, fmt.Errorf("time parameter is empty")
	}

	// Try parsing as relative duration first (24h, 7d, 30m)
	// Convert "d" (days) to "h" (hours) for time.ParseDuration compatibility
	if strings.HasSuffix(param, "d") {
		daysStr := strings.TrimSuffix(param, "d")
		duration, err := time.ParseDuration(daysStr + "h")
		if err == nil {
			// Convert days to hours (1d = 24h)
			duration = duration * 24
			return time.Now().Add(-duration), nil
		}
	}

	// Try parsing as standard duration (24h, 30m, 1h30m)
	if duration, err := time.ParseDuration(param); err == nil {
		// Relative time: subtract duration from now
		return time.Now().Add(-duration), nil
	}

	// Try parsing as absolute RFC3339 timestamp
	if t, err := time.Parse(time.RFC3339, param); err == nil {
		return t, nil
	}

	// Try parsing as RFC3339 without timezone (assume UTC)
	if t, err := time.Parse("2006-01-02T15:04:05", param); err == nil {
		return t.UTC(), nil
	}

	// Try parsing as date only (assume start of day UTC)
	if t, err := time.Parse("2006-01-02", param); err == nil {
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("invalid time format: %s (expected: 24h, 7d, or RFC3339 timestamp)", param)
}

