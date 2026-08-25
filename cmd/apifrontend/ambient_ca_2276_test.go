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

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

func generateAmbientCATestCert(t *testing.T) string {
	t.Helper()
	caPath := filepath.Join(t.TempDir(), "ca.crt")

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "apifrontend-ambient-ca-2276-test"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(caPath, pemBytes, 0644); err != nil {
		t.Fatal(err)
	}
	return caPath
}

func resetAmbientCAEnv(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		_ = os.Unsetenv("SSL_CERT_FILE")
		_ = os.Unsetenv("TLS_CA_FILE")
		sharedtls.ResetSystemCertFileCandidatesForTesting()
	})
}

// IT-AF-2276-001: a dsTlsCaFile configured via YAML is wired through
// bootstrapAmbientCATrust -- the exact production function run() calls
// immediately after setupConfigAndLogger, before buildSARClient -- into
// ambient env vars as defense-in-depth (Issue #2276, SC-8). This does not
// replace AF's existing explicit per-connection CA wiring (kaTlsCaFile,
// dsTlsCaFile, prometheusTlsCaFile, etc.) -- it only backstops any code path
// not covered by one of those explicit fields.
func TestBootstrapAmbientCATrust_APIFrontend_InjectsFromConfig(t *testing.T) {
	resetAmbientCAEnv(t)
	realCA := generateAmbientCATestCert(t)
	sharedtls.SetSystemCertFileCandidatesForTesting([]string{"/nonexistent/system-bundle.pem"})

	cfg := &config.Config{Agent: config.AgentConfig{DSTLSCaFile: realCA}}

	if err := bootstrapAmbientCATrust(logr.Discard(), cfg); err != nil {
		t.Fatalf("bootstrapAmbientCATrust() error = %v, want nil", err)
	}

	if got := os.Getenv("TLS_CA_FILE"); got != realCA {
		t.Errorf("TLS_CA_FILE = %q, want %q", got, realCA)
	}
	if got := os.Getenv("SSL_CERT_FILE"); got == "" {
		t.Error("SSL_CERT_FILE must be set after bootstrapAmbientCATrust with a non-empty dsTlsCaFile")
	}
}

// IT-AF-2276-002: an unset dsTlsCaFile leaves ambient env vars untouched
// (fail-open parity).
func TestBootstrapAmbientCATrust_APIFrontend_NoopWhenUnset(t *testing.T) {
	resetAmbientCAEnv(t)
	cfg := &config.Config{}

	if err := bootstrapAmbientCATrust(logr.Discard(), cfg); err != nil {
		t.Fatalf("bootstrapAmbientCATrust() error = %v, want nil", err)
	}

	if got := os.Getenv("SSL_CERT_FILE"); got != "" {
		t.Errorf("SSL_CERT_FILE = %q, want empty", got)
	}
	if got := os.Getenv("TLS_CA_FILE"); got != "" {
		t.Errorf("TLS_CA_FILE = %q, want empty", got)
	}
}
