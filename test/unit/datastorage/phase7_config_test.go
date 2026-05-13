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

package datastorage

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
)

// ========================================
// PHASE 7: CONFIGURABLE ENDPOINT PROPAGATION DELAY (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 7 / SRE-L1
// File Under Test: pkg/datastorage/config/config.go
// TDD Phase: RED — GetEndpointPropagationDelay() returns 0 (stub)
//
// Current state: endpointRemovalPropagationDelay is hardcoded as a const (5s)
// in pkg/datastorage/server/server.go. SRE teams need to tune this per cluster.
//
// Expected: ServerConfig.GetEndpointPropagationDelay() returns 5s default,
// parses the string field if provided, and clamps to [1s, 30s].
//
// ========================================

var _ = Describe("Phase 7: Configurable Endpoint Propagation Delay (TP-1088-P1)", func() {

	Describe("ServerConfig.GetEndpointPropagationDelay", func() {
		It("UT-DS-1088-P7-007a: default must be 5s when field is empty", func() {
			// RED: Stub returns 0, not 5s.

			cfg := config.ServerConfig{}

			Expect(cfg.GetEndpointPropagationDelay()).To(Equal(5*time.Second),
				"Default endpoint propagation delay must be 5s (industry best practice)")
		})

		It("UT-DS-1088-P7-007b: must parse valid duration string", func() {
			// RED: Stub returns 0, ignores the field value.

			cfg := config.ServerConfig{
				EndpointPropagationDelay: "10s",
			}

			Expect(cfg.GetEndpointPropagationDelay()).To(Equal(10*time.Second),
				"Must parse the configured duration string")
		})

		It("UT-DS-1088-P7-007c: must clamp to minimum 1s", func() {
			// RED: Stub returns 0.

			cfg := config.ServerConfig{
				EndpointPropagationDelay: "100ms",
			}

			Expect(cfg.GetEndpointPropagationDelay()).To(BeNumerically(">=", 1*time.Second),
				"Endpoint propagation delay must be at least 1s to allow K8s propagation")
		})

		It("UT-DS-1088-P7-007d: must clamp to maximum 30s", func() {
			cfg := config.ServerConfig{
				EndpointPropagationDelay: "60s",
			}

			Expect(cfg.GetEndpointPropagationDelay()).To(BeNumerically("<=", 30*time.Second),
				"Endpoint propagation delay must not exceed 30s to prevent slow shutdown")
		})
	})

	Describe("ServerConfig.GetShutdownTimeout", func() {
		It("UT-DS-1088-P7-020a: default must be 60s when field is empty", func() {
			cfg := config.ServerConfig{}
			Expect(cfg.GetShutdownTimeout()).To(Equal(60 * time.Second))
		})

		It("UT-DS-1088-P7-020b: must parse valid duration string", func() {
			cfg := config.ServerConfig{ShutdownTimeout: "90s"}
			Expect(cfg.GetShutdownTimeout()).To(Equal(90 * time.Second))
		})

		It("UT-DS-1088-P7-020c: must clamp to minimum 30s", func() {
			cfg := config.ServerConfig{ShutdownTimeout: "5s"}
			Expect(cfg.GetShutdownTimeout()).To(Equal(30 * time.Second))
		})

		It("UT-DS-1088-P7-020d: must clamp to maximum 120s", func() {
			cfg := config.ServerConfig{ShutdownTimeout: "300s"}
			Expect(cfg.GetShutdownTimeout()).To(Equal(120 * time.Second))
		})

		It("UT-DS-1088-P7-020e: must return default for invalid duration", func() {
			cfg := config.ServerConfig{ShutdownTimeout: "not-a-duration"}
			Expect(cfg.GetShutdownTimeout()).To(Equal(60 * time.Second))
		})
	})

	Describe("ServerConfig.GetMaxBodySize", func() {
		It("UT-DS-1088-P7-021a: default must be 5 MiB when field is empty", func() {
			cfg := config.ServerConfig{}
			Expect(cfg.GetMaxBodySize()).To(Equal(int64(5 << 20)))
		})

		It("UT-DS-1088-P7-021b: must parse integer byte value", func() {
			cfg := config.ServerConfig{MaxBodySize: "10485760"}
			Expect(cfg.GetMaxBodySize()).To(Equal(int64(10 << 20)))
		})

		It("UT-DS-1088-P7-021c: must clamp to minimum 1 MiB", func() {
			cfg := config.ServerConfig{MaxBodySize: "100"}
			Expect(cfg.GetMaxBodySize()).To(Equal(int64(1 << 20)))
		})

		It("UT-DS-1088-P7-021d: must clamp to maximum 50 MiB", func() {
			cfg := config.ServerConfig{MaxBodySize: "104857600"}
			Expect(cfg.GetMaxBodySize()).To(Equal(int64(50 << 20)))
		})

		It("UT-DS-1088-P7-021e: must return default for invalid string", func() {
			cfg := config.ServerConfig{MaxBodySize: "invalid"}
			Expect(cfg.GetMaxBodySize()).To(Equal(int64(5 << 20)))
		})
	})

	Describe("ServerConfig.GetCORSAllowedOrigins", func() {
		It("UT-DS-1088-P7-022a: default must be wildcard when field is empty", func() {
			cfg := config.ServerConfig{}
			Expect(cfg.GetCORSAllowedOrigins()).To(Equal([]string{"*"}))
		})

		It("UT-DS-1088-P7-022b: must return configured origins", func() {
			cfg := config.ServerConfig{
				CORSAllowedOrigins: []string{"https://kubernaut.ai", "https://console.kubernaut.ai"},
			}
			Expect(cfg.GetCORSAllowedOrigins()).To(Equal([]string{"https://kubernaut.ai", "https://console.kubernaut.ai"}))
		})
	})
})
