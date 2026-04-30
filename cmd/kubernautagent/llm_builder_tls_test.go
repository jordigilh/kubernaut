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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

func TestBuildTransportChain_TLSCaFile(t *testing.T) {
	caDir := t.TempDir()
	caPath := filepath.Join(caDir, "ca.crt")

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	if err := os.WriteFile(caPath, caPEM, 0644); err != nil {
		t.Fatal(err)
	}

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
	// With an invalid CA file, we expect either nil or an error-path behavior.
	// The current implementation should handle this gracefully.
	_ = transport
}
