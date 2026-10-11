package apifrontend_test

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-AUDIT-005: same-second PostgreSQL/DS observation", func() {
	It("IT-AF-2501-001: returns a deterministic later-page target and rejects complete pre-call baseline", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := sql.Open("pgx", "host=localhost port=15448 user=slm_user password=test_password dbname=action_history sslmode=disable")
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(db.Close()).To(Succeed()) })
		repo := repository.NewAuditEventsRepository(db, logr.Discard())
		actor, correlation := "observation-"+uuid.NewString(), uuid.NewString()
		timestamp := time.Now().UTC().Truncate(time.Second)
		insert := func(firstByte byte) uuid.UUID {
			id := uuid.New()
			id[0] = firstByte
			_, err := repo.Create(ctx, &repository.AuditEvent{EventID: id, Version: "1.0", EventTimestamp: timestamp,
				EventType: "apifrontend.tool.executed", EventCategory: "apifrontend", EventAction: "executed", EventOutcome: "success",
				ActorType: "user", ActorID: actor, CorrelationID: correlation, EventData: map[string]interface{}{
					"event_type": "apifrontend.tool.executed", "tool_name": "kubernaut_list_remediations", "tool_outcome": "success", "session_id": "", "execution_duration_ms": 0,
				},
			})
			Expect(err).ToNot(HaveOccurred())
			return id
		}
		for i := 0; i < 85; i++ {
			insert(255)
		}
		transport, err := sharedtls.NewTLSTransport(infrastructure.BootstrapTLSCAFile("https://localhost:18096"))
		Expect(err).ToNot(HaveOccurred())
		client, err := ogenclient.NewClient("https://localhost:18096", ogenclient.WithClient(&http.Client{
			Transport: &bearerTransport{token: serviceAccountToken, base: transport}, Timeout: 2 * time.Second,
		}))
		Expect(err).ToNot(HaveOccurred())
		params := ogenclient.QueryAuditEventsParams{EventType: ogenclient.NewOptString("apifrontend.tool.executed"),
			DetailKey: ogenclient.NewOptString("tool_name"), DetailValue: ogenclient.NewOptString("kubernaut_list_remediations"),
			CorrelationID: ogenclient.NewOptString(correlation), Since: ogenclient.NewOptString(timestamp.Format(time.RFC3339)), Limit: ogenclient.NewOptInt(50)}
		baselineRows, err := infrastructure.QueryAuditSnapshot(ctx, client, params)
		Expect(err).ToNot(HaveOccurred())
		Expect(baselineRows).To(HaveLen(85), "baseline must cover every page, not just fifty newest UUIDs")
		baseline := make(map[uuid.UUID]struct{}, len(baselineRows))
		for _, event := range baselineRows {
			baseline[event.EventID.Value] = struct{}{}
		}
		target := insert(1)
		firstPage, err := client.QueryAuditEvents(ctx, params)
		Expect(err).ToNot(HaveOccurred())
		Expect(firstPage.Data).ToNot(ContainElement(HaveField("EventID", ogenclient.NewOptUUID(target))))
		rows, err := infrastructure.QueryAuditSnapshot(ctx, client, params)
		Expect(err).ToNot(HaveOccurred())
		match, ok := infrastructure.FindNewToolAudit(rows, infrastructure.AuditObservation{
			Actor: actor, Tool: "kubernaut_list_remediations", Since: timestamp.Add(750 * time.Millisecond), Baseline: baseline})
		Expect(ok).To(BeTrue())
		Expect(match.EventID.Value).To(Equal(target))
		Expect(match.CorrelationID).To(Equal(correlation))
	})
})
