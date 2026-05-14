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

BR-STORAGE-021 .. BR-STORAGE-025 / DD-STORAGE-010: Audit query planner benchmarks.
*/

package query

import (
	"testing"
	"time"
)

// BenchmarkAuditEventQueryBuild measures SQL compilation for richly filtered timelines.
func BenchmarkAuditEventQueryBuild(b *testing.B) {
	since := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	builder := NewAuditEventsQueryBuilder().
		WithCorrelationID("lifecycle-bench-correlation").
		WithEventType("gateway.signal.received").
		WithService("gateway").
		WithOutcome("success").
		WithSeverity("info").
		WithSince(since).
		WithUntil(since.Add(24 * time.Hour)).
		WithLimit(500).
		WithOffset(25)

	sql, _, err := builder.Build()
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(sql)))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, benchErr := builder.Build()
		if benchErr != nil {
			b.Fatal(benchErr)
		}
	}
}
