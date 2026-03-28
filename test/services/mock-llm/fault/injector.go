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
package fault

import "sync"

// Config defines a fault injection configuration.
type Config struct {
	Enabled    bool   `json:"enabled"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	DelayMs    int    `json:"delay_ms,omitempty"`
	Count      int    `json:"count,omitempty"` // 0 = unlimited; >0 = auto-disable after N faults
}

// Injector manages configurable fault injection for the Mock LLM.
type Injector struct {
	mu     sync.RWMutex
	config Config
}

// NewInjector creates a new fault injector (disabled by default).
func NewInjector() *Injector {
	return &Injector{}
}

// Configure sets the fault injection configuration.
func (fi *Injector) Configure(cfg Config) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	fi.config = cfg
}

// IsActive returns true if fault injection is enabled.
// When Count > 0, decrements the remaining count and auto-disables when exhausted.
func (fi *Injector) IsActive() bool {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	if !fi.config.Enabled {
		return false
	}
	if fi.config.Count > 0 {
		fi.config.Count--
		if fi.config.Count == 0 {
			fi.config.Enabled = false
		}
	}
	return true
}

// StatusCode returns the configured HTTP status code.
func (fi *Injector) StatusCode() int {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	if fi.config.StatusCode == 0 {
		return 500
	}
	return fi.config.StatusCode
}

// Message returns the configured error message.
func (fi *Injector) Message() string {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	return fi.config.Message
}

// DelayMs returns the configured delay in milliseconds.
func (fi *Injector) DelayMs() int {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	return fi.config.DelayMs
}

// GetConfig returns a copy of the current configuration.
func (fi *Injector) GetConfig() Config {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	return fi.config
}

// Reset disables fault injection.
func (fi *Injector) Reset() {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	fi.config = Config{}
}
