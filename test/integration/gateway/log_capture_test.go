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

package gateway

import (
	"encoding/json"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// LOG CAPTURE INFRASTRUCTURE FOR BR-109 TESTING
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// BUSINESS OUTCOME: Enable operators to trace requests and debug issues
// via structured logging with request context.
//
// TEST STRATEGY: Capture Zap logs in-memory during tests, parse structured
// fields, and verify business outcomes (request tracing, security auditing,
// performance analysis).
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// LogCapture provides in-memory log capture for testing
type LogCapture struct {
	Core     zapcore.Core
	Observer *observer.ObservedLogs
	Logger   *zap.Logger
}

// NewLogCapture creates a new log capture instance for testing
//
// BUSINESS OUTCOME: Enable testing of structured logging business capabilities
// without requiring external log aggregation infrastructure.
//
// Parameters:
//   - level: Minimum log level to capture (e.g., zapcore.DebugLevel, zapcore.InfoLevel)
//
// Returns:
//   - *LogCapture: Configured log capture instance with in-memory observer
func NewLogCapture(level zapcore.Level) *LogCapture {
	// Create in-memory observer core
	core, logs := observer.New(level)

	// Create logger with observer core
	logger := zap.New(core)

	return &LogCapture{
		Core:     core,
		Observer: logs,
		Logger:   logger,
	}
}

// LogEntry represents a parsed structured log entry
//
// BUSINESS OUTCOME: Structured representation of log data for test assertions.
// Enables verification of business requirements (request_id, source_ip, etc.)
type LogEntry struct {
	Level      string                 `json:"level"`
	Timestamp  string                 `json:"timestamp"`
	Message    string                 `json:"message"`
	RequestID  string                 `json:"request_id,omitempty"`
	SourceIP   string                 `json:"source_ip,omitempty"`
	Endpoint   string                 `json:"endpoint,omitempty"`
	DurationMS float64                `json:"duration_ms,omitempty"`
	Fields     map[string]interface{} `json:"-"` // All fields for flexible assertions
}

// GetAllLogs returns all captured log entries
//
// BUSINESS OUTCOME: Retrieve all logs for comprehensive test validation
// (e.g., verify request_id appears in ALL log entries for a request)
func (lc *LogCapture) GetAllLogs() []observer.LoggedEntry {
	return lc.Observer.All()
}

// GetLogsByLevel returns logs filtered by level
//
// BUSINESS OUTCOME: Enable testing of log level control (BR-109 requirement)
// Operators can verify DEBUG logs are present/absent based on configuration.
func (lc *LogCapture) GetLogsByLevel(level zapcore.Level) []observer.LoggedEntry {
	var filtered []observer.LoggedEntry
	for _, log := range lc.Observer.All() {
		if log.Level == level {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

// GetLogsContaining returns logs with messages containing the given substring
//
// BUSINESS OUTCOME: Enable targeted log validation for specific operations
// (e.g., find all logs related to "CRD creation" or "Redis operation")
func (lc *LogCapture) GetLogsContaining(substring string) []observer.LoggedEntry {
	var filtered []observer.LoggedEntry
	for _, log := range lc.Observer.All() {
		if strings.Contains(log.Message, substring) {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

// ParseStructuredLogs converts captured logs to structured LogEntry objects
//
// BUSINESS OUTCOME: Enable business outcome assertions on log structure
// (e.g., verify request_id, source_ip, duration_ms fields are present)
func (lc *LogCapture) ParseStructuredLogs() []LogEntry {
	var entries []LogEntry

	for _, log := range lc.Observer.All() {
		entry := LogEntry{
			Level:   log.Level.String(),
			Message: log.Message,
			Fields:  make(map[string]interface{}),
		}

		// Extract structured fields
		for _, field := range log.Context {
			switch field.Key {
			case "request_id":
				entry.RequestID = field.String
			case "source_ip":
				entry.SourceIP = field.String
			case "endpoint":
				entry.Endpoint = field.String
			case "duration_ms":
				// Duration is stored as Integer (milliseconds) or Interface (float64)
				if field.Type == zapcore.Int64Type {
					entry.DurationMS = float64(field.Integer)
				} else if field.Interface != nil {
					if v, ok := field.Interface.(float64); ok {
						entry.DurationMS = v
					}
				}
			case "timestamp":
				entry.Timestamp = field.String
			default:
				// Store all fields for flexible assertions
				entry.Fields[field.Key] = field.Interface
			}
		}

		entries = append(entries, entry)
	}

	return entries
}

// GetLogEntriesWithField returns logs that contain a specific field
//
// BUSINESS OUTCOME: Verify business requirements like "all logs include request_id"
// or "security logs include source_ip"
func (lc *LogCapture) GetLogEntriesWithField(fieldName string) []LogEntry {
	var filtered []LogEntry

	for _, entry := range lc.ParseStructuredLogs() {
		switch fieldName {
		case "request_id":
			if entry.RequestID != "" {
				filtered = append(filtered, entry)
			}
		case "source_ip":
			if entry.SourceIP != "" {
				filtered = append(filtered, entry)
			}
		case "endpoint":
			if entry.Endpoint != "" {
				filtered = append(filtered, entry)
			}
		case "duration_ms":
			if entry.DurationMS > 0 {
				filtered = append(filtered, entry)
			}
		default:
			if _, exists := entry.Fields[fieldName]; exists {
				filtered = append(filtered, entry)
			}
		}
	}

	return filtered
}

// ToJSON converts a log entry to JSON format for validation
//
// BUSINESS OUTCOME: Verify logs are machine-readable (BR-109 requirement)
// Log aggregation systems (ELK, Splunk) can parse Gateway logs.
func (lc *LogCapture) ToJSON() ([]byte, error) {
	entries := lc.ParseStructuredLogs()
	return json.Marshal(entries)
}

// ContainsSensitiveData checks if logs contain unsanitized sensitive data
//
// BUSINESS OUTCOME: Verify logs don't leak sensitive information (BR-109)
// Security compliance requirement: passwords, tokens, secrets must be redacted.
//
// Returns:
//   - bool: true if sensitive data found (test should fail)
//   - []string: List of log messages containing sensitive data
func (lc *LogCapture) ContainsSensitiveData(sensitivePatterns []string) (bool, []string) {
	var violations []string

	for _, log := range lc.Observer.All() {
		logText := log.Message
		for _, field := range log.Context {
			logText += " " + field.String
		}

		for _, pattern := range sensitivePatterns {
			if strings.Contains(strings.ToLower(logText), strings.ToLower(pattern)) {
				violations = append(violations, log.Message)
				break
			}
		}
	}

	return len(violations) > 0, violations
}

// Reset clears all captured logs
//
// BUSINESS OUTCOME: Enable clean test state between test cases
// Prevents log pollution across tests.
func (lc *LogCapture) Reset() {
	lc.Observer.TakeAll()
}

// Count returns the total number of captured logs
//
// BUSINESS OUTCOME: Enable assertions on log volume
// (e.g., verify exactly N log entries for a single request)
func (lc *LogCapture) Count() int {
	return lc.Observer.Len()
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HELPER FUNCTIONS FOR COMMON TEST SCENARIOS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// VerifyAllLogsHaveRequestID checks if all logs include request_id field
//
// BUSINESS OUTCOME: BR-109 requirement - operators can trace requests
// across Gateway components using request_id.
func VerifyAllLogsHaveRequestID(capture *LogCapture) (bool, int, int) {
	allLogs := capture.ParseStructuredLogs()
	logsWithRequestID := capture.GetLogEntriesWithField("request_id")

	return len(allLogs) == len(logsWithRequestID), len(logsWithRequestID), len(allLogs)
}

// VerifyLogsHaveSourceIP checks if security-relevant logs include source_ip
//
// BUSINESS OUTCOME: BR-109 requirement - operators can audit webhook sources
// for security compliance and suspicious activity detection.
func VerifyLogsHaveSourceIP(capture *LogCapture) (bool, int) {
	logsWithSourceIP := capture.GetLogEntriesWithField("source_ip")
	return len(logsWithSourceIP) > 0, len(logsWithSourceIP)
}

// VerifyLogsHavePerformanceMetrics checks if logs include endpoint and duration_ms
//
// BUSINESS OUTCOME: BR-109 requirement - operators can identify slow requests
// via log analysis without requiring Prometheus metrics.
func VerifyLogsHavePerformanceMetrics(capture *LogCapture) (bool, int) {
	logsWithEndpoint := capture.GetLogEntriesWithField("endpoint")
	logsWithDuration := capture.GetLogEntriesWithField("duration_ms")

	// Both fields should be present for performance analysis
	return len(logsWithEndpoint) > 0 && len(logsWithDuration) > 0,
		min(len(logsWithEndpoint), len(logsWithDuration))
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
