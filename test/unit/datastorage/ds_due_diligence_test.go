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

// DS Due Diligence: F7 — queryEffectivenessEvents event_type merge.
//
// BR-EM-001: Effectiveness scoring correctness.
// The bug: queryEffectivenessEvents only selects event_data (no event_type column).
// When event_data JSONB lacks the "event_type" key, BuildEffectivenessResponse
// cannot route events, producing score=nil and status="no_data".
//
// This test demonstrates the bug at the symptom level: when events lack event_type
// in their EventData, BuildEffectivenessResponse treats them as unrecognized.
package datastorage

import (
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("F7: event_type merge in effectiveness scoring", Label("unit", "due-diligence", "F7"), func() {

	It("UT-DS-F7-001: BuildEffectivenessResponse should return no_data when events lack event_type key", func() {
		events := []*server.EffectivenessEvent{
			{
				EventData: map[string]interface{}{
					"assessed": true,
					"score":    0.85,
					"details":  "Pod running, readiness passing",
				},
			},
			{
				EventData: map[string]interface{}{
					"assessed": true,
					"score":    1.0,
					"details":  "Alert resolved",
				},
			},
			{
				EventData: map[string]interface{}{
					"pre_remediation_spec_hash":  "sha256:aaa",
					"post_remediation_spec_hash": "sha256:bbb",
					"hash_match":                 false,
				},
			},
		}

		resp := server.BuildEffectivenessResponse("rr-f7-001", events)

		Expect(resp.Score).To(BeNil(), "Score should be nil when events cannot be routed")
		Expect(resp.AssessmentStatus).To(Equal("no_data"),
			"Status should be no_data when events lack event_type for routing")
	})
})
