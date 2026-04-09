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

package types_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("Signal Context Propagation — #433 Phase 1A", func() {

	Describe("UT-KA-433-101: WithSignalContext stores and retrieves signal", func() {
		It("should store signal in context and retrieve it", func() {
			signal := katypes.SignalContext{
				Name:        "test-pod",
				Namespace:   "default",
				Severity:    "critical",
				Environment: "production",
				Priority:    "P0",
			}

			ctx := katypes.WithSignalContext(context.Background(), signal)
			retrieved, ok := katypes.SignalContextFromContext(ctx)
			Expect(ok).To(BeTrue(), "signal context should be found")
			Expect(retrieved.Name).To(Equal("test-pod"))
			Expect(retrieved.Namespace).To(Equal("default"))
			Expect(retrieved.Severity).To(Equal("critical"))
			Expect(retrieved.Environment).To(Equal("production"))
			Expect(retrieved.Priority).To(Equal("P0"))
		})
	})

	Describe("UT-KA-433-102: SignalContextFromContext returns false for empty context", func() {
		It("should return false when context has no signal", func() {
			_, ok := katypes.SignalContextFromContext(context.Background())
			Expect(ok).To(BeFalse(), "should not find signal in bare context")
		})
	})

	Describe("UT-KA-433-103: Signal context carries all fields", func() {
		It("should propagate all SignalContext fields through context", func() {
			signal := katypes.SignalContext{
				Name:             "oom-pod",
				Namespace:        "apps",
				Severity:         "warning",
				Message:          "OOMKilled detected",
				ResourceKind:     "Deployment",
				ResourceName:     "api-server",
				ClusterName:      "prod-east",
				Environment:      "staging",
				Priority:         "P1",
				RiskTolerance:    "low",
				SignalSource:     "prometheus",
				BusinessCategory: "revenue",
				Description:      "Pod was OOM killed",
			}

			ctx := katypes.WithSignalContext(context.Background(), signal)
			retrieved, ok := katypes.SignalContextFromContext(ctx)
			Expect(ok).To(BeTrue())
			Expect(retrieved).To(Equal(signal))
		})
	})
})
