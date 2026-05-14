/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

BR-STORAGE-024 / SOC2 Gap #9: Hash-chain hot path micro-benchmarks.
*/

package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func benchmarkAuditEventBaseline() *AuditEvent {
	ev := AuditEvent{
		EventID:        uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		EventTimestamp: time.Date(2025, 3, 14, 15, 9, 26, 0, time.UTC),
		EventDate:      DateOnly(time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)),
		EventType:      "gateway.signal.received",
		Version:        "1.0",
		EventCategory:  "gateway",
		EventAction:    "signal_received",
		EventOutcome:   "success",
		CorrelationID:  "bench-correlation-" + uuid.New().String(),
		EventHash:      "deadbeefcafebabedeadbeefcafebabedeadbeefcafebabedeadbeefcafebabe",
		PreviousEventHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde",
		EventData: map[string]interface{}{
			"signal_name":       "BenchSignal",
			"namespace":         "default",
			"payload_kb_hint":   16,
			"nested_dimensions": []string{"topology", "alert", "remediation"},
		},
	}
	return &ev
}

// BenchmarkPrepareEventForHashing measures deterministic field clearing ahead of hashing.
func BenchmarkPrepareEventForHashing(b *testing.B) {
	ev := benchmarkAuditEventBaseline()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PrepareEventForHashing(ev)
	}
}

// BenchmarkCalculateExpectedHash measures cumulative SHA256 chain recomputation costs.
//
// Uses the exported SHA256 hashing path through calculateEventHash (same linkage as ingest/export).
func BenchmarkCalculateExpectedHash(b *testing.B) {
	ev := benchmarkAuditEventBaseline()
	prev := "genesis-marker-for-benchmark-hash-chain--------------------------------------------"
	hash, err := calculateEventHash(prev, ev)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(hash)))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, benchErr := calculateEventHash(prev, ev)
		if benchErr != nil {
			b.Fatal(benchErr)
		}
	}
}
