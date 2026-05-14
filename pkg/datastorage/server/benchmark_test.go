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

BR-AUDIT-007: Signed export query parsing benchmarks (parseExportFilters).
*/

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// BenchmarkParseExportFilters benchmarks GET audit/export query coercion into repository.ExportFilters.
func BenchmarkParseExportFilters(b *testing.B) {
	const (
		rawCorrelation = "et-ds-bench-export-1234567890abcdef0123456789abcdef"
		exportPathFmt  = "/api/v1/audit/export"
	)

	startTime := time.Date(2026, 1, 14, 8, 0, 5, 0, time.UTC).Format(time.RFC3339)
	endTime := startTime // identical bounds stress parser without widening query semantics

	rawURL := exportPathFmt + "?correlation_id=" + rawCorrelation +
		"&event_category=gateway&limit=742&offset=18&start_time=" + startTime +
		"&end_time=" + endTime + "&redact_pii=true"

	baseReq := httptest.NewRequest(http.MethodGet, rawURL, nil)
	if _, err := parseExportFilters(baseReq); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, rawURL, nil)
		_, benchErr := parseExportFilters(req)
		if benchErr != nil {
			b.Fatal(benchErr)
		}
	}
}
