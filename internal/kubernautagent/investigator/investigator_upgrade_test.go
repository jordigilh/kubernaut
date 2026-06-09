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

package investigator_test

import (
	"context"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Fix #1390: Investigator Upgrade Flag — BR-INTERACTIVE-004", func() {

	newInvestigator := func() *investigator.Investigator {
		client := &interactiveHoldMockClient{}
		logger := logr.Discard()
		builder, _ := prompt.NewBuilder()
		rp := parser.NewResultParser()
		enricher := enrichment.NewEnricher(nopK8sClient{}, nopDSClient{}, audit.NopAuditStore{}, logger)

		return investigator.New(investigator.Config{
			Client:       client,
			Builder:      builder,
			ResultParser: rp,
			Enricher:     enricher,
			AuditStore:   audit.NopAuditStore{},
			Logger:       logger,
			MaxTurns:     15,
			PhaseTools:   investigator.DefaultPhaseToolMap(),
		})
	}

	Describe("UT-KA-1390-010 [SC-24]: Investigator sets InteractiveHold=true when upgrade flag is true and signal.Interactive=false", func() {
		It("should return InteractiveHold=true using context upgrade flag for originally-autonomous sessions", func() {
			inv := newInvestigator()

			flag := &atomic.Bool{}
			flag.Store(true)
			ctx := session.WithInteractiveUpgrade(context.Background(), flag)

			signal := katypes.SignalContext{
				Name:          "api-server",
				Namespace:     "production",
				Severity:      "critical",
				Message:       "OOMKilled",
				RemediationID: "rem-upgrade-010",
				Interactive:   false,
			}

			result, err := inv.Investigate(ctx, signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.InteractiveHold).To(BeTrue(),
				"InteractiveHold must be true when upgrade flag is set, even if signal.Interactive=false")
			Expect(result.WorkflowID).To(BeEmpty(),
				"Phase 2/3 must be skipped when InteractiveHold fires")
		})
	})

	Describe("UT-KA-1390-011 [SC-24]: Investigator still respects signal.Interactive=true (no regression)", func() {
		It("should return InteractiveHold=true via the original signal.Interactive path", func() {
			inv := newInvestigator()

			signal := katypes.SignalContext{
				Name:          "api-server",
				Namespace:     "production",
				Severity:      "critical",
				Message:       "OOMKilled",
				RemediationID: "rem-regression-011",
				Interactive:   true,
			}

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.InteractiveHold).To(BeTrue(),
				"InteractiveHold must remain true for originally-interactive sessions")
		})
	})
})
