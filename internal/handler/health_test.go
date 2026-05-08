package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// UT-AF-038-060
func TestReadyz_Returns503WhenOneCheckerFails(t *testing.T) {
	checker := AllReady(
		func() bool { return true },
		func() bool { return false }, // Simulates KA CB open
	)
	h := readyzHandler(checker)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

// UT-AF-038-063
func TestReadyz_Returns200WhenAllCheckersPass(t *testing.T) {
	checker := AllReady(
		func() bool { return true }, // JWKS
		func() bool { return true }, // KA CB
		func() bool { return true }, // DS CB
		func() bool { return true }, // K8s CB
	)
	h := readyzHandler(checker)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// UT-AF-038-064
func TestReadyz_Returns200WhenHalfOpen(t *testing.T) {
	// half-open means the CB is probing — it should still be "ready" for traffic
	checker := AllReady(
		func() bool { return true }, // half-open returns true per Healthy() semantics
	)
	h := readyzHandler(checker)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// UT-AF-038-060/061/062 — individual CB failures
func TestReadyz_Returns503ForEachCBOpen(t *testing.T) {
	tests := []struct {
		name string
		ka   bool
		ds   bool
		k8s  bool
	}{
		{"KA CB open", false, true, true},
		{"DS CB open", true, false, true},
		{"K8s CB open", true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := AllReady(
				func() bool { return tt.ka },
				func() bool { return tt.ds },
				func() bool { return tt.k8s },
			)
			h := readyzHandler(checker)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", rec.Code)
			}
		})
	}
}

func TestAllReady_EmptyCheckers(t *testing.T) {
	checker := AllReady()
	if !checker() {
		t.Error("AllReady() with no checkers should return true")
	}
}
