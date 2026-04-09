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
package config

import (
	"errors"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// ScenarioOverride defines optional per-scenario overrides from a YAML config file.
type ScenarioOverride struct {
	WorkflowID string   `yaml:"workflow_id,omitempty"`
	Confidence *float64 `yaml:"confidence,omitempty"`
}

// Overrides holds the parsed YAML override configuration.
type Overrides struct {
	Mode      string                      `yaml:"mode"`
	Scenarios map[string]ScenarioOverride `yaml:"scenarios"`
}

// LoadYAMLOverrides reads a YAML overrides file. If the path is empty or the
// file does not exist, it returns an empty Overrides struct and no error,
// enabling graceful fallback to deterministic defaults.
func LoadYAMLOverrides(path string) (*Overrides, error) {
	if path == "" {
		return &Overrides{Scenarios: map[string]ScenarioOverride{}}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Overrides{Scenarios: map[string]ScenarioOverride{}}, nil
		}
		return nil, err
	}

	var o Overrides
	if err := yaml.Unmarshal(data, &o); err != nil {
		return nil, err
	}

	if o.Scenarios == nil {
		o.Scenarios = map[string]ScenarioOverride{}
	}

	return &o, nil
}
