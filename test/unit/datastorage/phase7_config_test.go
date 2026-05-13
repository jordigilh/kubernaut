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
			// RED: Stub returns 0.

			cfg := config.ServerConfig{
				EndpointPropagationDelay: "60s",
			}

			Expect(cfg.GetEndpointPropagationDelay()).To(BeNumerically("<=", 30*time.Second),
				"Endpoint propagation delay must not exceed 30s to prevent slow shutdown")
		})
	})
})
