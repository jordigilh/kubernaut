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

package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	fleetclient "github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

type recoveryAuditStore2518 struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *recoveryAuditStore2518) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

var _ = Describe("Fleet overlay recovery wiring (Issue #2518)", func() {
	It("IT-KA-2527-003 [BR-AUDIT-005, SI-10]: normalizes absent recovery failure classes to allowed values", func() {
		store := &recoveryAuditStore2518{}
		observer := newFleetRetryObserver(store, nil, logr.Discard())
		ctx := context.Background()

		observer.ObserveRetry(ctx, fleetclient.RetryObservation{
			Source:      "bridge_tool",
			ToolClass:   "read",
			LogicalTool: "resources_get",
			Outcome:     fleetclient.RetryOutcomeExhausted,
		})
		observer.ObserveRetry(ctx, fleetclient.RetryObservation{
			Source:      "bridge_tool",
			ToolClass:   "read",
			LogicalTool: "resources_get",
			Outcome:     fleetclient.RetryOutcomeSuccess,
		})

		store.mu.Lock()
		events := append([]*audit.AuditEvent(nil), store.events...)
		store.mu.Unlock()
		Expect(events).To(HaveLen(2))
		Expect(events[0].Data["failure_class"]).To(Equal("unknown"))
		Expect(events[0].Data["retry_outcome"]).To(Equal(fleetclient.RetryOutcomeExhausted))
		Expect(events[1].Data).NotTo(HaveKey("failure_class"),
			"successful recovery must not persist a synthetic failure enum")
	})

	It("IT-KA-FLEET-2518-001 [BR-HAPI-261]: production overlay re-resolves a read binding after the MCP session resets", func() {
		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
		DeferCleanup(gw.Close)

		cfg := kaconfig.DefaultConfig()
		cfg.Integrations.Fleet.Endpoint = gw.URL()
		cfg.Integrations.Fleet.GatewayType = string(fleet.GatewayEAIGW)
		cfg.Integrations.Fleet.Resilience = fleet.FleetResilienceConfig{MaxElapsedTime: 5 * time.Second}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		DeferCleanup(cancel)
		fleetClient, resolver := registerFleetTools(ctx, cfg, logr.Discard())
		Expect(fleetClient).ToNot(BeNil())
		DeferCleanup(func() { _ = fleetClient.Close() })

		overlay, err := resolver.Overlay(ctx, "prod-east")
		Expect(err).ToNot(HaveOccurred())
		tool, found := overlay["resources_get"]
		Expect(found).To(BeTrue())

		args := json.RawMessage(`{"kind":"Pod","apiVersion":"v1","namespace":"default","name":"nginx"}`)
		result, err := tool.Execute(ctx, args)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(ContainSubstring("nginx"))

		Expect(fleetClient.Session().Close()).To(Succeed())
		result, err = tool.Execute(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "the production overlay must follow the provider's fresh session")
		Expect(result).To(ContainSubstring("nginx"))

		var readCalls int
		for _, call := range gw.CallLog() {
			if call.ToolName == "prod-east__resources_get" {
				readCalls++
			}
		}
		Expect(readCalls).To(Equal(2), "one logical read before and one after recovery; no duplicate replay beyond the failed attempt")
	})

	It("IT-KA-FLEET-2518-012 [BR-KA-OBSERVABILITY-001]: production BridgeTool uses the injected bounded recovery observer", func() {
		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
		DeferCleanup(gw.Close)

		cfg := kaconfig.DefaultConfig()
		cfg.Integrations.Fleet.Endpoint = gw.URL()
		cfg.Integrations.Fleet.GatewayType = string(fleet.GatewayEAIGW)
		cfg.Integrations.Fleet.Resilience = fleet.FleetResilienceConfig{MaxElapsedTime: 5 * time.Second}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		DeferCleanup(cancel)
		store := &recoveryAuditStore2518{}
		metrics := kametrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		observer := newFleetRetryObserver(store, metrics, logr.Discard())
		fleetClient, resolver := registerFleetTools(ctx, cfg, logr.Discard(), observer)
		Expect(fleetClient).ToNot(BeNil())
		DeferCleanup(func() { _ = fleetClient.Close() })

		overlay, err := resolver.Overlay(ctx, "prod-east")
		Expect(err).ToNot(HaveOccurred())
		tool, found := overlay["resources_get"]
		Expect(found).To(BeTrue())
		Expect(fleetClient.Session().Close()).To(Succeed())

		result, err := tool.Execute(ctx, json.RawMessage(`{"kind":"Pod","apiVersion":"v1","namespace":"default","name":"nginx"}`))
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(ContainSubstring("nginx"))

		store.mu.Lock()
		events := append([]*audit.AuditEvent(nil), store.events...)
		store.mu.Unlock()
		Expect(events).NotTo(BeEmpty())
		for _, event := range events {
			Expect(event.EventID).NotTo(BeZero())
			Expect(event.Data["logical_tool"]).To(Equal("resources_get"))
			Expect(event.Data["cluster_id"]).To(Equal("prod-east"))
			Expect(event.Data["failure_code"]).NotTo(Equal("secret"))
		}
	})
})
