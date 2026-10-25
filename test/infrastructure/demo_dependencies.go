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

package infrastructure

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFleetGatewayVersionOverrides loads the optional dependency override file
// used by the fleet demo. Empty paths preserve the validated built-in matrix.
func LoadFleetGatewayVersionOverrides(path string) (FleetGatewayVersionOverrides, error) {
	if strings.TrimSpace(path) == "" {
		return FleetGatewayVersionOverrides{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return FleetGatewayVersionOverrides{}, fmt.Errorf("read dependency overrides file %q: %w", path, err)
	}
	return parseFleetGatewayVersionOverrides(data)
}

// parseFleetGatewayVersionOverrides decodes one strict YAML override document.
func parseFleetGatewayVersionOverrides(data []byte) (FleetGatewayVersionOverrides, error) {
	var overrides FleetGatewayVersionOverrides
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&overrides); err != nil {
		if errors.Is(err, io.EOF) {
			return FleetGatewayVersionOverrides{}, nil
		}
		return FleetGatewayVersionOverrides{}, fmt.Errorf("parse dependency overrides: %w", err)
	}
	return overrides, nil
}

// MergeFleetGatewayVersionOverrides applies explicit CLI/Make values over the
// optional file values. This keeps one-off compatibility experiments convenient
// without requiring a second file for every version change.
func MergeFleetGatewayVersionOverrides(file, explicit FleetGatewayVersionOverrides) FleetGatewayVersionOverrides {
	merged := file
	if strings.TrimSpace(explicit.EnvoyGatewayVersion) != "" {
		merged.EnvoyGatewayVersion = explicit.EnvoyGatewayVersion
	}
	if strings.TrimSpace(explicit.AgentRouterVersion) != "" {
		merged.AgentRouterVersion = explicit.AgentRouterVersion
	}
	return merged
}

// DemoDependencyOverrides is the user-facing release/profile file for both
// external Fleet dependencies and the prebuilt operator/application artifacts.
// It deliberately has no source-build default; an explicit source checkout is
// handled only through KUBERNAUT_OPERATOR_ROOT with dirty-tree validation.
type DemoDependencyOverrides struct {
	EnvoyGatewayVersion string                     `yaml:"envoyGatewayVersion,omitempty"`
	AgentRouterVersion  string                     `yaml:"agentRouterVersion,omitempty"`
	Dependency          DemoDependencyOverrideYAML `yaml:"dependency,omitempty"`
	Operator            DemoArtifactOverrideYAML   `yaml:"operator,omitempty"`
	Kubernaut           DemoArtifactOverrideYAML   `yaml:"kubernaut,omitempty"`
}

type DemoArtifactOverrideYAML struct {
	Version         string `yaml:"version,omitempty"`
	Image           string `yaml:"image,omitempty"`
	ImageRepository string `yaml:"imageRepository,omitempty"`
	ImageTag        string `yaml:"imageTag,omitempty"`
	ImageDigest     string `yaml:"imageDigest,omitempty"`
	Chart           string `yaml:"chart,omitempty"`
	ChartDigest     string `yaml:"chartDigest,omitempty"`
	Ref             string `yaml:"ref,omitempty"`
}

type DemoDependencyOverrideYAML struct {
	Chart       string `yaml:"chart,omitempty"`
	Version     string `yaml:"version,omitempty"`
	ChartDigest string `yaml:"chartDigest,omitempty"`
}

func LoadDemoDependencyOverrides(path string) (DemoDependencyOverrides, error) {
	if strings.TrimSpace(path) == "" {
		return DemoDependencyOverrides{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return DemoDependencyOverrides{}, fmt.Errorf("read dependency overrides file %q: %w", path, err)
	}
	var overrides DemoDependencyOverrides
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&overrides); err != nil {
		if errors.Is(err, io.EOF) {
			return DemoDependencyOverrides{}, nil
		}
		return DemoDependencyOverrides{}, fmt.Errorf("parse dependency overrides: %w", err)
	}
	return overrides, nil
}

func (o DemoDependencyOverrides) FleetGatewayVersions() FleetGatewayVersionOverrides {
	return FleetGatewayVersionOverrides{EnvoyGatewayVersion: o.EnvoyGatewayVersion, AgentRouterVersion: o.AgentRouterVersion}
}

func (o DemoDependencyOverrides) ArtifactOverrides() DemoArtifactOverrides {
	return DemoArtifactOverrides{
		OperatorVersion:         o.Operator.Version,
		OperatorImage:           o.Operator.Image,
		OperatorImageRepository: o.Operator.ImageRepository,
		OperatorImageTag:        o.Operator.ImageTag,
		OperatorDigest:          o.Operator.ImageDigest,
		OperatorChart:           o.Operator.Chart,
		OperatorChartDigest:     o.Operator.ChartDigest,
		OperatorRef:             o.Operator.Ref,
		DependencyChart:         o.Dependency.Chart,
		DependencyChartVersion:  o.Dependency.Version,
		DependencyChartDigest:   o.Dependency.ChartDigest,
		KubernautVersion:        o.Kubernaut.Version,
		KubernautImage:          o.Kubernaut.Image,
		KubernautDigest:         o.Kubernaut.ImageDigest,
	}
}

// MergeDemoArtifactOverrides applies explicit CLI values over the optional
// dependency override file. Empty CLI fields preserve file-backed defaults.
func MergeDemoArtifactOverrides(base, explicit DemoArtifactOverrides) DemoArtifactOverrides {
	merged := base
	if explicit.OperatorVersion != "" {
		merged.OperatorVersion = explicit.OperatorVersion
	}
	if explicit.OperatorImage != "" {
		merged.OperatorImage = explicit.OperatorImage
	}
	if explicit.OperatorImageRepository != "" {
		merged.OperatorImageRepository = explicit.OperatorImageRepository
	}
	if explicit.OperatorImageTag != "" {
		merged.OperatorImageTag = explicit.OperatorImageTag
	}
	if explicit.OperatorDigest != "" {
		merged.OperatorDigest = explicit.OperatorDigest
	}
	if explicit.OperatorChart != "" {
		merged.OperatorChart = explicit.OperatorChart
	}
	if explicit.OperatorChartDigest != "" {
		merged.OperatorChartDigest = explicit.OperatorChartDigest
	}
	if explicit.OperatorRef != "" {
		merged.OperatorRef = explicit.OperatorRef
	}
	if explicit.DependencyChart != "" {
		merged.DependencyChart = explicit.DependencyChart
	}
	if explicit.DependencyChartVersion != "" {
		merged.DependencyChartVersion = explicit.DependencyChartVersion
	}
	if explicit.DependencyChartDigest != "" {
		merged.DependencyChartDigest = explicit.DependencyChartDigest
	}
	if explicit.KubernautVersion != "" {
		merged.KubernautVersion = explicit.KubernautVersion
	}
	if explicit.KubernautImage != "" {
		merged.KubernautImage = explicit.KubernautImage
	}
	if explicit.KubernautDigest != "" {
		merged.KubernautDigest = explicit.KubernautDigest
	}
	return merged
}
