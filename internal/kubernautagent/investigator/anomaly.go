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

package investigator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
)

// AnomalyConfig holds configurable thresholds for the anomaly detector (I7).
type AnomalyConfig struct {
	MaxToolCallsPerTool int `yaml:"maxToolCallsPerTool"`
	MaxTotalToolCalls   int `yaml:"maxTotalToolCalls"`
	MaxRepeatedFailures int `yaml:"maxRepeatedFailures"`
}

// DefaultAnomalyConfig returns production defaults per DD-HAPI-019-003.
func DefaultAnomalyConfig() AnomalyConfig {
	return AnomalyConfig{
		MaxToolCallsPerTool: 5,
		MaxTotalToolCalls:   30,
		MaxRepeatedFailures: 3,
	}
}

// AnomalyResult indicates the outcome of an anomaly check.
type AnomalyResult struct {
	Allowed bool
	Reason  string
}

// AnomalyDetector tracks tool call patterns and aborts on anomalous behavior (I7).
// Not safe for concurrent use — designed for a single investigation goroutine.
type AnomalyDetector struct {
	config             AnomalyConfig
	suspiciousPatterns []*regexp.Regexp
	toolCallCounts     map[string]int
	totalCallCount     int
	failureTracker     map[string]int
}

// NewAnomalyDetector creates an I7 anomaly detector with the given config.
func NewAnomalyDetector(config AnomalyConfig, suspiciousPatterns []*regexp.Regexp) *AnomalyDetector {
	return &AnomalyDetector{
		config:             config,
		suspiciousPatterns: suspiciousPatterns,
		toolCallCounts:     make(map[string]int),
		failureTracker:     make(map[string]int),
	}
}

// CheckToolCall validates a tool call against anomaly thresholds.
// Returns Allowed=false if the call should be rejected.
func (d *AnomalyDetector) CheckToolCall(name string, args json.RawMessage) AnomalyResult {
	if r := d.checkSuspiciousArgs(name, args); !r.Allowed {
		return r
	}

	d.totalCallCount++
	if d.totalCallCount > d.config.MaxTotalToolCalls {
		return AnomalyResult{
			Allowed: false,
			Reason:  fmt.Sprintf("total tool call limit exceeded (%d > %d)", d.totalCallCount, d.config.MaxTotalToolCalls),
		}
	}

	d.toolCallCounts[name]++
	if d.toolCallCounts[name] > d.config.MaxToolCallsPerTool {
		return AnomalyResult{
			Allowed: false,
			Reason:  fmt.Sprintf("per-tool call limit exceeded for %s (%d > %d)", name, d.toolCallCounts[name], d.config.MaxToolCallsPerTool),
		}
	}

	return AnomalyResult{Allowed: true}
}

// RecordFailure records a tool execution failure for repeated-failure detection.
// The key is tool name + args hash, so different arguments are tracked independently.
func (d *AnomalyDetector) RecordFailure(name string, args json.RawMessage) AnomalyResult {
	key := failureKey(name, args)
	d.failureTracker[key]++
	if d.failureTracker[key] >= d.config.MaxRepeatedFailures {
		return AnomalyResult{
			Allowed: false,
			Reason:  fmt.Sprintf("repeated identical failure for %s (%d >= %d)", name, d.failureTracker[key], d.config.MaxRepeatedFailures),
		}
	}
	return AnomalyResult{Allowed: true}
}

// TotalExceeded returns true when the total tool call count has exceeded the configured limit.
// Used by runLLMLoop to abort early.
func (d *AnomalyDetector) TotalExceeded() bool {
	return d.totalCallCount > d.config.MaxTotalToolCalls
}

func (d *AnomalyDetector) checkSuspiciousArgs(name string, args json.RawMessage) AnomalyResult {
	if len(d.suspiciousPatterns) == 0 || len(args) == 0 {
		return AnomalyResult{Allowed: true}
	}
	argsStr := string(args)
	for _, p := range d.suspiciousPatterns {
		if p.MatchString(argsStr) {
			return AnomalyResult{
				Allowed: false,
				Reason:  fmt.Sprintf("suspicious argument pattern in %s: %s", name, p.String()),
			}
		}
	}
	return AnomalyResult{Allowed: true}
}

func failureKey(name string, args json.RawMessage) string {
	h := sha256.Sum256(args)
	return fmt.Sprintf("%s:%x", name, h[:8])
}
