package launcher_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

var _ = Describe("enrichRRDetail (AC 12)", func() {
	It("UT-AF-1189-040: adds rr_name and rr_namespace when CreateContext has RR fields", func() {
		sc := &session.CreateContext{
			TaskID:      "task-001",
			RRName:      "rr-oom-web",
			RRNamespace: "production",
		}
		ctx := session.WithCreateContext(context.Background(), sc)

		detail := map[string]string{"task_id": "task-001"}
		launcher.EnrichRRDetailForTest(ctx, detail)

		Expect(detail).To(HaveKeyWithValue("rr_name", "rr-oom-web"))
		Expect(detail).To(HaveKeyWithValue("rr_namespace", "production"))
		Expect(detail).To(HaveKeyWithValue("task_id", "task-001"))
	})

	It("UT-AF-1189-041: no-op when CreateContext has empty RRName", func() {
		sc := &session.CreateContext{TaskID: "task-002"}
		ctx := session.WithCreateContext(context.Background(), sc)

		detail := map[string]string{"task_id": "task-002"}
		launcher.EnrichRRDetailForTest(ctx, detail)

		Expect(detail).NotTo(HaveKey("rr_name"))
		Expect(detail).NotTo(HaveKey("rr_namespace"))
	})

	It("UT-AF-1189-042: no-op when no CreateContext in context", func() {
		detail := map[string]string{"task_id": "task-003"}
		launcher.EnrichRRDetailForTest(context.Background(), detail)

		Expect(detail).NotTo(HaveKey("rr_name"))
		Expect(detail).NotTo(HaveKey("rr_namespace"))
	})

	It("UT-AF-1189-043: pointer mutation by tool callback is visible", func() {
		sc := &session.CreateContext{TaskID: "task-004"}
		ctx := session.WithCreateContext(context.Background(), sc)

		// Simulate kubernaut_remediate tool callback mutating the shared pointer
		sc.RRName = "rr-crash-api"
		sc.RRNamespace = "staging"

		detail := map[string]string{"task_id": "task-004"}
		launcher.EnrichRRDetailForTest(ctx, detail)

		Expect(detail).To(HaveKeyWithValue("rr_name", "rr-crash-api"))
		Expect(detail).To(HaveKeyWithValue("rr_namespace", "staging"))
	})
})
