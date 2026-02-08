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

package classifier

import (
	"fmt"
	"os"
	"sync"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
)

// SignalModeResult contains the classification outcome for a signal type.
// BR-SP-106: Predictive Signal Mode Classification
// ADR-054: Predictive Signal Mode Classification and Prompt Strategy
type SignalModeResult struct {
	// SignalMode is "reactive" (default) or "predictive"
	SignalMode string
	// NormalizedType is the base signal type for workflow catalog matching.
	// For predictive signals, this is the mapped base type (e.g., "OOMKilled").
	// For reactive signals, this is the original type unchanged.
	NormalizedType string
	// OriginalSignalType is preserved for audit trail (SOC2 CC7.4).
	// Only populated for predictive signals; empty for reactive.
	OriginalSignalType string
}

// signalModeConfig is the YAML structure for predictive signal mappings.
type signalModeConfig struct {
	PredictiveSignalMappings map[string]string `yaml:"predictive_signal_mappings"`
}

// SignalModeClassifier classifies signals as reactive or predictive
// using a YAML-based lookup table.
//
// Design Decision: YAML config (not Rego) because signal mode classification
// is a simple key-value lookup, unlike severity/environment/priority which
// evaluate complex multi-input policies via Rego.
//
// BR-SP-106: Predictive Signal Mode Classification
// ADR-054: Predictive Signal Mode Classification and Prompt Strategy
type SignalModeClassifier struct {
	logger   logr.Logger
	mu       sync.RWMutex
	mappings map[string]string // predictive type -> base type
}

// NewSignalModeClassifier creates a new signal mode classifier.
// The classifier starts with empty mappings (all signals default to reactive)
// until LoadConfig is called.
func NewSignalModeClassifier(logger logr.Logger) *SignalModeClassifier {
	return &SignalModeClassifier{
		logger:   logger.WithName("signalmode"),
		mappings: make(map[string]string),
	}
}

// LoadConfig loads predictive signal mappings from a YAML config file.
// This method is safe for concurrent use and supports hot-reload
// (BR-SP-072 pattern): call it again to update mappings at runtime.
func (c *SignalModeClassifier) LoadConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg signalModeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Build new mappings (nil map from empty YAML is fine — make a clean map)
	newMappings := make(map[string]string, len(cfg.PredictiveSignalMappings))
	for k, v := range cfg.PredictiveSignalMappings {
		newMappings[k] = v
	}

	c.mu.Lock()
	c.mappings = newMappings
	c.mu.Unlock()

	c.logger.Info("Signal mode config loaded",
		"mappings", len(newMappings),
		"path", configPath)

	return nil
}

// Classify determines the signal mode and normalized type for a given signal type.
//
// - If the signal type is in the predictive mappings, it returns mode "predictive"
//   with the normalized (base) type and preserves the original for audit.
// - Otherwise, it returns mode "reactive" with the type unchanged.
//
// This is a pure function (no I/O) after config is loaded. Safe for concurrent use.
func (c *SignalModeClassifier) Classify(signalType string) SignalModeResult {
	c.mu.RLock()
	baseType, found := c.mappings[signalType]
	c.mu.RUnlock()

	if found {
		return SignalModeResult{
			SignalMode:         "predictive",
			NormalizedType:     baseType,
			OriginalSignalType: signalType,
		}
	}

	return SignalModeResult{
		SignalMode:         "reactive",
		NormalizedType:     signalType,
		OriginalSignalType: "",
	}
}
