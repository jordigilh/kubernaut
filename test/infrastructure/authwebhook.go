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
	"io"
)

// AuthWebhookInfrastructure wraps the shared DSBootstrapInfra
// Per DD-TEST-001 v2.2: PostgreSQL:15442, Redis:16386, DataStorage:18099
// Per DD-TEST-002: Uses shared sequential startup pattern from datastorage_bootstrap.go
type AuthWebhookInfrastructure struct {
	*DSBootstrapInfra // Embed shared infrastructure
}

// NewAuthWebhookInfrastructure creates infrastructure manager using shared DSBootstrap
// This eliminates 95% code duplication by reusing proven Gateway/RO/NT patterns.
//
// Pattern: DD-TEST-002 Sequential Startup (via DSBootstrap)
// Benefits:
//   - Proven reliability: DataStorage (818/818 tests), Gateway (7/7 tests)
//   - Eliminates podman-compose race conditions
//   - Consistent behavior across all integration test suites
//   - Single source of truth for infrastructure setup
func NewAuthWebhookInfrastructure() *AuthWebhookInfrastructure {
	return &AuthWebhookInfrastructure{}
}

// Setup starts all infrastructure using shared DSBootstrap
// Sequential Order: Cleanup → Network → PostgreSQL → Migrations → Redis → DataStorage
func (i *AuthWebhookInfrastructure) Setup(writer io.Writer) error {
	cfg := DSBootstrapConfig{
		ServiceName:     "authwebhook",
		PostgresPort:    15442, // DD-TEST-001 v2.2
		RedisPort:       16386, // DD-TEST-001 v2.2
		DataStoragePort: 18099, // DD-TEST-001 v2.2
		MetricsPort:     19099, // DD-TEST-001 v2.2
		ConfigDir:       "test/integration/authwebhook/config",
	}

	infra, err := StartDSBootstrap(cfg, writer)
	if err != nil {
		return err
	}

	i.DSBootstrapInfra = infra
	return nil
}

// Teardown stops and cleans up all infrastructure using shared DSBootstrap
// Cleanup Order: Stop containers → Remove images → Remove network
func (i *AuthWebhookInfrastructure) Teardown(writer io.Writer) error {
	if i.DSBootstrapInfra == nil {
		return nil
	}
	return StopDSBootstrap(i.DSBootstrapInfra, writer)
}

// GetDataStorageURL returns the Data Storage service URL (convenience method)
func (i *AuthWebhookInfrastructure) GetDataStorageURL() string {
	if i.DSBootstrapInfra == nil {
		return ""
	}
	return i.ServiceURL
}
