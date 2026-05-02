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

	transport := buildTransportChain(cfg, rt)
	if transport == nil {
		t.Fatal("expected non-nil transport when tlsCaFile is set, got nil")
	}
}

func TestBuildTransportChain_NoTLSCaFile(t *testing.T) {
	cfg := kaconfig.DefaultConfig()
	rt := &kaconfig.LLMRuntimeConfig{}

	transport := buildTransportChain(cfg, rt)
	if transport != nil {
		t.Fatalf("expected nil transport when no custom config, got %T", transport)
	}
}

func TestBuildTransportChain_InvalidCaFile(t *testing.T) {
	cfg := kaconfig.DefaultConfig()
	cfg.AI.LLM.TLSCaFile = "/nonexistent/ca.crt"

	rt := &kaconfig.LLMRuntimeConfig{}

	transport := buildTransportChain(cfg, rt)
	_ = transport
}
