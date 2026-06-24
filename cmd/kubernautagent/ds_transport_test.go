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

package main

import (
	"net/http"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

func TestBuildDSBaseTransport_WithCAFile(t *testing.T) {
	caPath := generateTestCACert(t, "DS Test CA")

	transport, err := buildDSBaseTransport(caPath, types.LLMCircuitBreaker{})
	if err != nil {
		t.Fatalf("expected no error with valid CA file, got: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport when caFile is set")
	}
	if transport == http.DefaultTransport {
		t.Fatal("expected custom transport, got http.DefaultTransport")
	}
}

func TestBuildDSBaseTransport_EmptyCAFile(t *testing.T) {
	transport, err := buildDSBaseTransport("", types.LLMCircuitBreaker{})
	if err != nil {
		t.Fatalf("expected no error with empty caFile, got: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport (default fallback)")
	}
}

func TestBuildDSBaseTransport_InvalidCAFile(t *testing.T) {
	_, err := buildDSBaseTransport("/nonexistent/ca.crt", types.LLMCircuitBreaker{})
	if err == nil {
		t.Fatal("expected error with invalid CA file path, got nil")
	}
}
