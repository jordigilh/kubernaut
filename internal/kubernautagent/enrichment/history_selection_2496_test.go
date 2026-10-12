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

package enrichment_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("DD-KA-2496 history adapter (BR-KA-016, SI-10, AC-4)", func() {
	DescribeTable("UT-KA-2496-001: generated HTTP response retains identity and known/unknown selection confidence in both tiers",
		func(confidenceJSON string, expected interface{}) {
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("clusterId") != "origin-a" || r.URL.Query().Get("targetName") != "api" {
					http.Error(w, "incorrect target scope", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				entry := fmt.Sprintf(`{"remediationUID":"rr-prior","workflowId":"wf-catalog-a","workflowName":"Restore memory",%s"completedAt":"2026-10-08T10:00:00Z","effectivenessScore":0,"signalResolved":false,"assessmentReason":"Full"}`, confidenceJSON)
				if _, err := fmt.Fprintf(w, `{"targetResource":"prod/Deployment/api","currentSpecHash":"sha256:current","regressionDetected":false,"tier1":{"window":"24h","chain":[%s]},"tier2":{"window":"2160h","chain":[%s]}}`, entry, entry); err != nil {
					GinkgoWriter.Printf("history fixture response failed: %v\n", err)
				}
			}))
			DeferCleanup(endpoint.Close)
			client, err := api.NewClient(endpoint.URL)
			Expect(err).NotTo(HaveOccurred())
			result, err := enrichment.NewDSAdapter(client).GetRemediationHistory(context.Background(), "Deployment", "api", "prod", "origin-a", "sha256:current")
			Expect(err).NotTo(HaveOccurred())
			raw, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())
			var mapped map[string]json.RawMessage
			Expect(json.Unmarshal(raw, &mapped)).To(Succeed())
			for _, tier := range []string{"tier1", "tier2"} {
				var entries []map[string]interface{}
				Expect(json.Unmarshal(mapped[tier], &entries)).To(Succeed())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0]).To(HaveKeyWithValue("workflow_id", "wf-catalog-a"))
				Expect(entries[0]).To(HaveKeyWithValue("workflow_name", "Restore memory"))
				if expected == nil {
					Expect(entries[0]).NotTo(HaveKey("workflow_selection_confidence"))
				} else {
					Expect(entries[0]).To(HaveKeyWithValue("workflow_selection_confidence", expected))
				}
				Expect(entries[0]).To(HaveKeyWithValue("effectiveness_score", 0.0))
				Expect(entries[0]).To(HaveKeyWithValue("signal_resolved", false))
				Expect(entries[0]).To(HaveKeyWithValue("assessment_reason", "Full"))
			}
		},
		Entry("known score", `"workflowSelectionConfidence":0.94,`, 0.94),
		Entry("known zero", `"workflowSelectionConfidence":0,`, 0.0),
		Entry("unknown", "", nil),
	)
})
