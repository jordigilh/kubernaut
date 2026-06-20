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

// Package fleet provides shared types and factories for multi-cluster federation.
// Services that participate in fleet operations (GW, RO, FMC Writer) import this
// package for consistent configuration and connection management.
package fleet

import "fmt"

// FleetConfig holds multi-cluster federation settings shared across all services.
// Previously duplicated in pkg/gateway/config and internal/config/remediationorchestrator.
//
// References: ADR-065, ADR-068, BR-INTEGRATION-065
type FleetConfig struct {
	// Enabled activates federated scope checking via Valkey cache.
	Enabled bool `yaml:"enabled"`
	// ValkeyAddr is the Valkey/Redis address for the fleet metadata cache.
	ValkeyAddr string `yaml:"valkeyAddr"`
}

// Validate checks that FleetConfig has all required fields when enabled.
func (c FleetConfig) Validate() error {
	if c.Enabled && c.ValkeyAddr == "" {
		return fmt.Errorf("fleet: valkeyAddr must not be empty when fleet is enabled")
	}
	return nil
}
