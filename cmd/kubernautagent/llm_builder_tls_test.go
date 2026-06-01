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
	"testing"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

func TestBuildTransportChain_TLSCaFile(t *testing.T) {
	caPath := generateTestCACert(t, "Test CA")

	cfg := kaconfig.DefaultConfig()
	cfg.AI.LLM.TLSCaFile = caPath

	rt := &kaconfig.LLMRuntimeConfig{}

	transport, err := buildTransportChain(cfg, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport when tlsCaFile is set, got nil")
	}
}

func TestBuildTransportChain_NoTLSCaFile(t *testing.T) {
	cfg := kaconfig.DefaultConfig()
	rt := &kaconfig.LLMRuntimeConfig{}

	transport, err := buildTransportChain(cfg, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport != nil {
		t.Fatalf("expected nil transport when no custom config, got %T", transport)
	}
}

// UT-KA-1342-030: buildTransportChain returns error for invalid CA file (fail-hard per SC-8)
func TestBuildTransportChain_InvalidCaFile(t *testing.T) {
	cfg := kaconfig.DefaultConfig()
	cfg.AI.LLM.TLSCaFile = "/nonexistent/ca.crt"

	rt := &kaconfig.LLMRuntimeConfig{}

	_, err := buildTransportChain(cfg, rt)
	if err == nil {
		t.Fatal("expected error for invalid CA file, got nil")
	}
}

// UT-KA-1342-020: buildTransportChain passes WithClientCert when cert fields are set
func TestBuildTransportChain_mTLS(t *testing.T) {
	caPath := generateTestCACert(t, "Test CA")

	certPath, keyPath := generateTestClientCert(t, caPath)

	cfg := kaconfig.DefaultConfig()
	cfg.AI.LLM.TLSCaFile = caPath
	cfg.AI.LLM.TLSCertFile = certPath
	cfg.AI.LLM.TLSKeyFile = keyPath

	rt := &kaconfig.LLMRuntimeConfig{}

	chain, err := buildTransportChain(cfg, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain == nil {
		t.Fatal("expected non-nil transport for mTLS config")
	}
}
