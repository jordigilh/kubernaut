package infrastructure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/go-faster/jx"
	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-AUDIT-005: fresh structured tool evidence (AU-3, CC7.2)", func() {
	It("UT-AF-2501-001/002/003: rejects baseline/foreign/wrong-type/failed rows but accepts same-second fresh success", func() {
		start := time.Now().UTC().Truncate(time.Second).Add(750 * time.Millisecond)
		fresh := toolObservationEvent("owned-actor", start.Truncate(time.Second))
		baseline := toolObservationEvent("owned-actor", start.Truncate(time.Second))
		foreign := toolObservationEvent("another-actor", start)
		failed := toolObservationEvent("owned-actor", start)
		failed.EventOutcome = "failure"
		wrongType := toolObservationEvent("owned-actor", start)
		wrongType.EventType = "another.tool.executed"
		wrongAction := toolObservationEvent("owned-actor", start)
		wrongAction.EventAction = "denied"
		old := toolObservationEvent("owned-actor", start.Add(-time.Hour))
		missingID := fresh
		missingID.EventID = ogenclient.OptNilUUID{}
		missingCorrelation := fresh
		missingCorrelation.CorrelationID = ""
		wrongTool := fresh
		payload, ok := wrongTool.EventData.GetApifrontendToolExecutedPayload()
		Expect(ok).To(BeTrue())
		payload.ToolName = "another-tool"
		wrongTool.EventData = ogenclient.NewApifrontendToolExecutedPayloadAuditEventEventData(payload)
		failedPayload := fresh
		payload.ToolName = "kubernaut_list_remediations"
		payload.ToolOutcome = "failure"
		failedPayload.EventData = ogenclient.NewApifrontendToolExecutedPayloadAuditEventEventData(payload)
		observation := AuditObservation{Actor: "owned-actor", Tool: "kubernaut_list_remediations", Since: start,
			Baseline: map[uuid.UUID]struct{}{baseline.EventID.Value: {}}}
		for _, event := range []ogenclient.AuditEvent{baseline, foreign, failed, wrongType, wrongAction, old, missingID, missingCorrelation, wrongTool, failedPayload} {
			_, ok := FindNewToolAudit([]ogenclient.AuditEvent{event}, observation)
			Expect(ok).To(BeFalse(), "row %s must not satisfy the current invocation", event.EventID.Value)
		}
		match, ok := FindNewToolAudit([]ogenclient.AuditEvent{baseline, foreign, fresh}, observation)
		Expect(ok).To(BeTrue())
		Expect(match.EventID).To(Equal(fresh.EventID))
	})

	It("UT-AF-2501-004: traverses complete filtered pages and preserves inclusive same-second bounds", func() {
		start := time.Now().UTC().Truncate(time.Second)
		target := toolObservationEvent("owned-actor", start)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.URL.Query().Get("since")).To(Equal(start.Format(time.RFC3339)))
			Expect(r.URL.Query().Get("event_type")).To(Equal("apifrontend.tool.executed"))
			Expect(r.URL.Query().Get("detail_key")).To(Equal("tool_name"))
			Expect(r.URL.Query().Get("detail_value")).To(Equal("kubernaut_list_remediations"))
			offsetText := r.URL.Query().Get("offset")
			if offsetText == "" {
				offsetText = "0"
			}
			offset, err := strconv.Atoi(offsetText)
			Expect(err).ToNot(HaveOccurred())
			rows := make([]ogenclient.AuditEvent, 50)
			for i := range rows {
				rows[i] = toolObservationEvent("another-actor", start)
			}
			if offset == 50 {
				rows = []ogenclient.AuditEvent{target}
			}
			w.Header().Set("Content-Type", "application/json")
			page := ogenclient.AuditEventsQueryResponse{Data: rows}
			var encoder jx.Encoder
			page.Encode(&encoder)
			_, err = w.Write(encoder.Bytes())
			Expect(err).ToNot(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		client, err := ogenclient.NewClient(server.URL)
		Expect(err).ToNot(HaveOccurred())
		events, err := QueryAuditSnapshot(context.Background(), client, ogenclient.QueryAuditEventsParams{
			EventType: ogenclient.NewOptString("apifrontend.tool.executed"),
			DetailKey: ogenclient.NewOptString("tool_name"), DetailValue: ogenclient.NewOptString("kubernaut_list_remediations"),
			Since: ogenclient.NewOptString(start.Format(time.RFC3339)), Limit: ogenclient.NewOptInt(50),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(events).To(HaveLen(51))
		match, ok := FindNewToolAudit(events, AuditObservation{Actor: "owned-actor", Tool: "kubernaut_list_remediations", Since: start})
		Expect(ok).To(BeTrue())
		Expect(match.EventID).To(Equal(target.EventID))
	})

	It("UT-AF-2501-004: changing totals/full pages terminate with explicit exhaustion", func() {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defer GinkgoRecover()
			calls++
			w.Header().Set("Content-Type", "application/json")
			page := ogenclient.AuditEventsQueryResponse{Data: []ogenclient.AuditEvent{
				toolObservationEvent("other", time.Now().UTC()),
			}}
			var encoder jx.Encoder
			page.Encode(&encoder)
			_, err := w.Write(encoder.Bytes())
			Expect(err).ToNot(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		client, err := ogenclient.NewClient(server.URL)
		Expect(err).ToNot(HaveOccurred())
		_, err = QueryAuditSnapshot(context.Background(), client, ogenclient.QueryAuditEventsParams{Limit: ogenclient.NewOptInt(1)})
		Expect(err).To(MatchError(ContainSubstring("page budget")))
		Expect(calls).To(Equal(10))
	})

	DescribeTable("UT-AF-2501-004: rejects invalid page sizes without issuing a query",
		func(limit int) {
			_, err := QueryAuditSnapshot(context.Background(), nil, ogenclient.QueryAuditEventsParams{Limit: ogenclient.NewOptInt(limit)})
			Expect(err).To(MatchError("invalid audit snapshot page size"))
		}, Entry("zero", 0), Entry("over API maximum", 1001))

	It("UT-AF-2501-004: deduplicates persisted UUIDs across pages and keeps explicit upper bounds", func() {
		event := toolObservationEvent("owned-actor", time.Now().UTC().Truncate(time.Second))
		until := event.EventTimestamp.Add(time.Minute).Format(time.RFC3339)
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			calls++
			Expect(r.URL.Query().Get("until")).To(Equal(until))
			rows := []ogenclient.AuditEvent{event}
			if calls == 1 {
				rows = append(rows, event)
			}
			var encoder jx.Encoder
			page := ogenclient.AuditEventsQueryResponse{Data: rows}
			page.Encode(&encoder)
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write(encoder.Bytes())
			Expect(err).ToNot(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		client, err := ogenclient.NewClient(server.URL)
		Expect(err).ToNot(HaveOccurred())
		rows, err := QueryAuditSnapshot(context.Background(), client, ogenclient.QueryAuditEventsParams{Limit: ogenclient.NewOptInt(2), Until: ogenclient.NewOptString(until)})
		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(ConsistOf(event))
		Expect(calls).To(Equal(2))
	})

	It("UT-AF-2501-004: a failed API query is observable, never an empty successful observation", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "test-owned query unavailable", http.StatusServiceUnavailable)
		}))
		DeferCleanup(server.Close)
		client, err := ogenclient.NewClient(server.URL)
		Expect(err).ToNot(HaveOccurred())
		rows, err := QueryAuditSnapshot(context.Background(), client, ogenclient.QueryAuditEventsParams{})
		Expect(err).To(MatchError(ContainSubstring("audit snapshot offset 0")))
		Expect(rows).To(BeNil())
	})

	It("UT-AF-2501-004: rows without persisted UUIDs cannot form a complete baseline", func() {
		event := toolObservationEvent("owned-actor", time.Now().UTC())
		event.EventID = ogenclient.OptNilUUID{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defer GinkgoRecover()
			var encoder jx.Encoder
			page := ogenclient.AuditEventsQueryResponse{Data: []ogenclient.AuditEvent{event}}
			page.Encode(&encoder)
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write(encoder.Bytes())
			Expect(err).ToNot(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		client, err := ogenclient.NewClient(server.URL)
		Expect(err).ToNot(HaveOccurred())
		_, err = QueryAuditSnapshot(context.Background(), client, ogenclient.QueryAuditEventsParams{})
		Expect(err).To(MatchError("audit snapshot row missing persisted event uuid"))
	})
})

func toolObservationEvent(actor string, timestamp time.Time) ogenclient.AuditEvent {
	return ogenclient.AuditEvent{Version: "1.0", EventType: "apifrontend.tool.executed", EventCategory: "apifrontend",
		EventAction: "executed", EventOutcome: "success", EventTimestamp: timestamp, EventID: ogenclient.NewOptNilUUID(uuid.New()),
		ActorType: ogenclient.NewOptString("user"), ActorID: ogenclient.NewOptString(actor), CorrelationID: uuid.NewString(),
		EventData: ogenclient.NewApifrontendToolExecutedPayloadAuditEventEventData(ogenclient.ApifrontendToolExecutedPayload{
			EventType: "apifrontend.tool.executed", ToolName: "kubernaut_list_remediations", ToolOutcome: "success", SessionID: "",
		}),
	}
}
