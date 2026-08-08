package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// Issue #2022: shared scope-checker context plumbing, reused by
// kubernaut_investigate_alert, kubernaut_remediate, and kubernaut_investigate
// (both AF transports) so a resource out of Kubernaut's management scope
// (ADR-053) is rejected before an RR/session is created, instead of only
// being caught downstream by RO after the waste #2022 reported.
var _ = Describe("ScopeChecker context plumbing (#2022)", func() {
	Describe("UT-AF-2022-040: ContextWithScopeChecker / ScopeCheckerFromContext round-trip", func() {
		It("returns the checker that was stored", func() {
			checker := &mocks.NeverManagedScopeChecker{}
			ctx := tools.ContextWithScopeChecker(context.Background(), checker)
			Expect(tools.ScopeCheckerFromContext(ctx)).To(BeIdenticalTo(checker))
		})

		It("returns nil when no checker was stored", func() {
			Expect(tools.ScopeCheckerFromContext(context.Background())).To(BeNil())
		})

		It("is a no-op when storing a nil checker (mirrors ContextWithRESTMapper)", func() {
			ctx := tools.ContextWithScopeChecker(context.Background(), nil)
			Expect(tools.ScopeCheckerFromContext(ctx)).To(BeNil())
		})
	})
})
