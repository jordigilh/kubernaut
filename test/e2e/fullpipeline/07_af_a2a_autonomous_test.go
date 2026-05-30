package fullpipeline

import (
	. "github.com/onsi/ginkgo/v2"
)

// E2E-FP-1189-002: A2A Autonomous — Blocked on #1332 (intent-based tool redesign).
// The current af_create_rr unconditionally creates an IS CRD, forcing the AA into
// interactive mode. #1332 introduces kubernaut_remediate (no IS, autonomous) vs
// kubernaut_investigate (IS, interactive) to resolve this.
var _ = Describe("AF A2A Autonomous Full Pipeline [E2E-FP-1189-002]", Label("fp", "af", "a2a", "issue-1189"), func() {

	It("should create RR via A2A and trigger full pipeline execution", func() {
		Skip("Blocked on #1332: autonomous A2A requires kubernaut_remediate (no IS creation)")
	})
})
