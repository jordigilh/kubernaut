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

package audit

import (
	"encoding/json"

	"github.com/go-faster/jx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// UT-AF-2658-009 / IT-AF-2658-106: severity_triage.ambiguous must be a
// first-class persisted event, and both severity payloads must carry the API
// group (#2658 D1.9) -- before #2658 the ambiguous event was dropped by the
// "no typed payload" guard and the group never reached the audit trail
// (AU-3, SOC2 CC7.2).
var _ = Describe("severity_triage audit payloads (#2658) [BR-SEVERITY-001 AC-8, AU-3, CC7.2]", func() {
	It("persists severity_triage.ambiguous with api_group and ambiguity_reason", func() {
		Expect(hasTypedPayload(EventSeverityTriageAmbiguous)).To(BeTrue(),
			"an event without a typed payload is logged only, never persisted")

		union := buildSeverityTriageAmbiguousPayload(&Event{
			Type: EventSeverityTriageAmbiguous,
			Detail: map[string]string{
				"namespace":            "prod",
				"kind":                 "Route",
				"name":                 "web",
				"api_group":            "route.openshift.io",
				"ambiguity_reason":     "group_unscopable",
				"candidate_alert":      "RouteDegraded",
				"candidate_severity":   "critical",
				"candidate_api_groups": `["serving.knative.dev"]`,
			},
		})
		Expect(union.Validate()).To(Succeed())

		payload, ok := union.GetApifrontendSeverityTriageAmbiguousPayload()
		Expect(ok).To(BeTrue())
		Expect(string(payload.EventType)).To(Equal("apifrontend.severity_triage.ambiguous"))
		Expect(string(payload.AmbiguityReason)).To(Equal("group_unscopable"))
		group, present := payload.APIGroup.Get()
		Expect(present).To(BeTrue())
		Expect(group).To(Equal("route.openshift.io"))
		Expect(payload.CandidateAPIGroups).To(Equal([]string{"serving.knative.dev"}))

		var encoder jx.Encoder
		union.Encode(&encoder)
		var serialized map[string]json.RawMessage
		Expect(json.Unmarshal(encoder.Bytes(), &serialized)).To(Succeed())
		Expect(string(serialized["ambiguity_reason"])).To(MatchJSON(`"group_unscopable"`))
		Expect(string(serialized["api_group"])).To(MatchJSON(`"route.openshift.io"`))
	})

	It("serializes each ambiguity reason the gate can emit", func() {
		for _, reason := range []string{"cluster_scoped_only", "group_unscopable", "alert_belongs_to_sibling"} {
			union := buildSeverityTriageAmbiguousPayload(&Event{
				Type:   EventSeverityTriageAmbiguous,
				Detail: map[string]string{"ambiguity_reason": reason, "api_group": "apps"},
			})
			Expect(union.Validate()).To(Succeed(), "reason %q must be a schema value", reason)
			payload, ok := union.GetApifrontendSeverityTriageAmbiguousPayload()
			Expect(ok).To(BeTrue())
			Expect(string(payload.AmbiguityReason)).To(Equal(reason))
		}
	})

	It("carries api_group and the reconciled source_tier on severity_triage.completed", func() {
		union := buildSeverityTriageCompletedPayload(&Event{
			Type: EventSeverityTriageCompleted,
			Detail: map[string]string{
				"namespace":   "prod",
				"kind":        "Route",
				"name":        "web",
				"severity":    "critical",
				"source_tier": "firing_alert",
				"api_group":   "route.openshift.io",
			},
		})
		Expect(union.Validate()).To(Succeed())
		payload, ok := union.GetApifrontendSeverityTriageCompletedPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.SourceTier).To(Equal("firing_alert"),
			"the builder must read the key the triager actually emits (source/source_tier reconciliation)")
		group, present := payload.APIGroup.Get()
		Expect(present).To(BeTrue())
		Expect(group).To(Equal("route.openshift.io"))
	})

	It("maps the ambiguous event to a stable audit action", func() {
		Expect(eventAction(EventSeverityTriageAmbiguous)).To(Equal("ambiguous"))
	})

	It("exposes the ambiguous payload as a discriminated union member", func() {
		union := buildSeverityTriageAmbiguousPayload(&Event{
			Type:   EventSeverityTriageAmbiguous,
			Detail: map[string]string{"ambiguity_reason": "cluster_scoped_only"},
		})
		var encoder jx.Encoder
		union.Encode(&encoder)
		var serialized struct {
			Type string `json:"event_type"`
		}
		Expect(json.Unmarshal(encoder.Bytes(), &serialized)).To(Succeed())
		Expect(serialized.Type).To(Equal("apifrontend.severity_triage.ambiguous"))

		// The union must dispatch on the same discriminator DS validates.
		var discriminator ogenclient.AuditEventRequestEventData
		Expect(discriminator.Decode(jx.DecodeBytes(encoder.Bytes()))).To(Succeed())
		_, ok := discriminator.GetApifrontendSeverityTriageAmbiguousPayload()
		Expect(ok).To(BeTrue())
	})
})
