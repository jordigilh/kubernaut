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

// Package infrastructure provides E2E test helpers for fleet testing.
package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

// FleetE2EConfig holds configuration for fleet E2E test infrastructure.
type FleetE2EConfig struct {
	ValkeyAddr         string
	MockClusterID      string
	MockClusterName    string
	MCPGatewayEndpoint string
}

// DefaultFleetE2EConfig returns defaults for fleet E2E testing.
func DefaultFleetE2EConfig() FleetE2EConfig {
	return FleetE2EConfig{
		ValkeyAddr:      "localhost:6379",
		MockClusterID:   "mock-spoke",
		MockClusterName: "Mock Spoke Cluster",
	}
}

// FleetE2EHelper provides utilities for fleet E2E test setup and teardown.
type FleetE2EHelper struct {
	config FleetE2EConfig
	logger logr.Logger
	valkey *redis.Client
}

// NewFleetE2EHelper creates a new fleet E2E helper.
func NewFleetE2EHelper(config FleetE2EConfig, logger logr.Logger) *FleetE2EHelper {
	return &FleetE2EHelper{
		config: config,
		logger: logger,
		valkey: redis.NewClient(&redis.Options{Addr: config.ValkeyAddr}),
	}
}

// SeedValkeyScopeCache pre-seeds Valkey with managed resource keys for the mock cluster.
// This simulates what the FMC Writer would do in production.
func (h *FleetE2EHelper) SeedValkeyScopeCache(ctx context.Context, resources []SeedResource) error {
	for _, res := range resources {
		key := scopecache.BuildKey(h.config.MockClusterID, res.Group, res.Version, res.Kind, res.Namespace, res.Name)
		if err := h.valkey.Set(ctx, key, "1", 5*time.Minute).Err(); err != nil {
			return fmt.Errorf("seed key %s: %w", key, err)
		}
		h.logger.V(1).Info("Seeded scope cache", "key", key)
	}
	return nil
}

// CleanupValkeyKeys removes all kubernaut:managed: keys for the mock cluster.
func (h *FleetE2EHelper) CleanupValkeyKeys(ctx context.Context) error {
	pattern := fmt.Sprintf("kubernaut:managed:%s:*", h.config.MockClusterID)
	iter := h.valkey.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := h.valkey.Del(ctx, iter.Val()).Err(); err != nil {
			h.logger.Error(err, "Failed to delete key", "key", iter.Val())
		}
	}
	return iter.Err()
}

// Close terminates the Valkey connection.
func (h *FleetE2EHelper) Close() error {
	return h.valkey.Close()
}

// SeedResource defines a resource to seed in the Valkey scope cache.
type SeedResource struct {
	Group     string
	Version   string
	Kind      string
	Namespace string
	Name      string
}
