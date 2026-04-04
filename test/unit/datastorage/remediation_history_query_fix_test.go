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

// Issue #616: Regression gates for CorrelateTier1Chain post-hash matching.
//
// These tests validate that CorrelateTier1Chain correctly handles the
// post-hash match scenario. They are regression gates — expected to PASS
// immediately because the correlation logic already handles post-hash via
// ComputeHashMatch. The actual bug is in QueryROEventsBySpecHash (SQL layer),
// tested by integration tests IT-DS-616-*.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// TP-616-v1.1: Test Plan for Issue #616.
package datastorage

import (
	"time"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Issue #616: CorrelateTier1Chain Post-Hash Matching (Regression Gates)", Label("unit", "issue-616"), func() {

	var (
		fixedTime = time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
		laterTime = time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC)
	)

	makeROEvent := func(correlationID, preHash string, ts time.Time) repository.RawAuditRow {
		return repository.RawAuditRow{
			EventType:      "remediation.workflow_created",
			CorrelationID:  correlationID,
			EventTimestamp: ts,
			EventData: map[string]interface{}{
				"pre_remediation_spec_hash": preHash,
				"outcome":                  "success",
				"signal_type":              "HighCPULoad",
				"signal_fingerprint":       "fp-616",
				"action_type":              "RestartPod",
				"target_resource":          "default/Deployment/nginx",
			},
		}
	}

	makeHashEMEvent := func(preHash, postHash string) *server.EffectivenessEvent {
		return &server.EffectivenessEvent{
			EventData: map[string]interface{}{
				"event_type":                 "effectiveness.hash.computed",
				"pre_remediation_spec_hash":  preHash,
				"post_remediation_spec_hash": postHash,
				"hash_match":                 false,
			},
		}
	}

	It("UT-DS-616-001: should produce hashMatch=postRemediation when currentSpecHash matches EM post-hash", func() {
		roEvents := []repository.RawAuditRow{
			makeROEvent("rr-001", "sha256:aaa", fixedTime),
		}
		emEvents := map[string][]*server.EffectivenessEvent{
			"rr-001": {makeHashEMEvent("sha256:aaa", "sha256:bbb")},
		}

		entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:bbb")

		Expect(entries).To(HaveLen(1))
		entry := entries[0]
		Expect(entry.RemediationUID).To(Equal("rr-001"))
		Expect(entry.HashMatch.Set).To(BeTrue())
		Expect(entry.HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchPostRemediation))
		Expect(entry.PostRemediationSpecHash.Set).To(BeTrue())
		Expect(entry.PostRemediationSpecHash.Value).To(Equal("sha256:bbb"))
	})

	It("UT-DS-616-002: should produce hashMatch=preRemediation for pre-hash match (regression gate)", func() {
		roEvents := []repository.RawAuditRow{
			makeROEvent("rr-002", "sha256:aaa", fixedTime),
		}
		emEvents := map[string][]*server.EffectivenessEvent{
			"rr-002": {makeHashEMEvent("sha256:aaa", "sha256:bbb")},
		}

		entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:aaa")

		Expect(entries).To(HaveLen(1))
		Expect(entries[0].HashMatch.Set).To(BeTrue())
		Expect(entries[0].HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchPreRemediation))
	})

	It("UT-DS-616-003: should produce correct entries when both pre-hash and post-hash paths contribute RO events", func() {
		roEvents := []repository.RawAuditRow{
			makeROEvent("rr-pre", "sha256:target", fixedTime),
			makeROEvent("rr-post", "sha256:other", laterTime),
		}
		emEvents := map[string][]*server.EffectivenessEvent{
			"rr-pre":  {makeHashEMEvent("sha256:target", "sha256:xxx")},
			"rr-post": {makeHashEMEvent("sha256:other", "sha256:target")},
		}

		entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:target")

		Expect(entries).To(HaveLen(2))

		// Sorted descending by completedAt: rr-post (laterTime) first
		Expect(entries[0].RemediationUID).To(Equal("rr-post"))
		Expect(entries[0].HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchPostRemediation))

		Expect(entries[1].RemediationUID).To(Equal("rr-pre"))
		Expect(entries[1].HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchPreRemediation))
	})

	It("UT-DS-616-004: should produce hashMatch=none when neither pre nor post hash matches", func() {
		roEvents := []repository.RawAuditRow{
			makeROEvent("rr-none", "sha256:unrelated", fixedTime),
		}
		emEvents := map[string][]*server.EffectivenessEvent{
			"rr-none": {makeHashEMEvent("sha256:unrelated", "sha256:also-unrelated")},
		}

		entries := server.CorrelateTier1Chain(roEvents, emEvents, "sha256:target")

		Expect(entries).To(HaveLen(1))
		Expect(entries[0].HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchNone))
	})
})
