package infrastructure

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/cert"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-PLATFORM-001.6a: real DataStorage collector subprocess (SC-8, AC-6)", func() {
	It("IT-AF-2501-002/003: retrieves structured evidence with verified TLS, file credentials and supported time bounds", func() {
		const secret = "synthetic-read-only-query-token-2501"
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+secret {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if r.URL.Path != "/api/v1/audit/events" {
				http.NotFound(w, r)
				return
			}
			if r.URL.Query().Get("since") != "24h" || r.URL.Query().Get("until") == "" || r.URL.Query().Has("start_time") {
				http.Error(w, "unsupported audit time bounds", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"data":[{"event_id":"fresh-2501","event_type":"apifrontend.tool.executed"}],"pagination":{"total":1,"limit":1000,"offset":0}}`))
			Expect(err).ToNot(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		dir := GinkgoT().TempDir()
		caPath := filepath.Join(dir, "ca.crt")
		tokenPath := filepath.Join(dir, "query-token")
		Expect(os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)).To(Succeed())
		Expect(os.WriteFile(tokenPath, []byte(secret), 0600)).To(Succeed())
		output, err := runDataStorageCollector(dir, server.URL, caPath, tokenPath, "24h")
		Expect(err).ToNot(HaveOccurred(), "collector output: %s", output)
		body, err := os.ReadFile(filepath.Join(dir, "datastorage", "audit-events.json"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("fresh-2501"))
		Expect(output).ToNot(ContainSubstring(secret))
		Expect(string(body)).ToNot(ContainSubstring(secret))
	})

	It("IT-AF-2501-002 [UT-AF-2501-003]: the collector's frozen upper bound includes same-second persisted events", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/audit/events" {
				http.NotFound(w, r)
				return
			}
			if r.URL.Query().Get("until") != "2026-10-11T00:00:01Z" {
				http.Error(w, "upper bound excluded a persisted 00:00:00.500 event", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"data":[{"event_id":"same-second-2501"}],"pagination":{"total":1}}`))
			Expect(err).ToNot(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		dir := GinkgoT().TempDir()
		// External clock only; the production collector and real TLS/HTTP are unchanged.
		Expect(os.WriteFile(filepath.Join(dir, "date"), []byte("#!/bin/bash\ncase \"$*\" in *+%s*) printf '1791676800\\n';; *) printf '2026-10-11T00:00:00Z\\n';; esac\n"), 0755)).To(Succeed())
		GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		caPath, tokenPath := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "token")
		Expect(os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)).To(Succeed())
		Expect(os.WriteFile(tokenPath, []byte("test-owned-same-second-token"), 0600)).To(Succeed())
		output, err := runDataStorageCollector(dir, server.URL, caPath, tokenPath, "24h")
		Expect(err).NotTo(HaveOccurred(), "collector output: %s", output)
		body, err := os.ReadFile(filepath.Join(dir, "datastorage", "audit-events.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("same-second-2501"))
	})

	DescribeTable("IT-AF-2501-003/004: failures retain valid bounded JSON, never credential material",
		func(status int, duration string, wrongCA bool) {
			const secret = "synthetic-error-credential-2501"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "failure with \"quotes\"\nsecret="+secret, status)
			}))
			DeferCleanup(server.Close)
			dir := GinkgoT().TempDir()
			caPath := filepath.Join(dir, "ca.crt")
			caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
			if wrongCA {
				other, err := cert.GenerateSelfSigned(cert.CertificateOptions{
					CommonName: "untrusted-2501", DNSNames: []string{"localhost"},
					ValidityDuration: time.Hour, KeySize: 2048,
				})
				Expect(err).ToNot(HaveOccurred())
				caPEM = other.CertPEM
			}
			Expect(os.WriteFile(caPath, caPEM, 0600)).To(Succeed())
			tokenPath := filepath.Join(dir, "token")
			Expect(os.WriteFile(tokenPath, []byte(secret), 0600)).To(Succeed())
			output, err := runDataStorageCollector(dir, server.URL, caPath, tokenPath, duration)
			Expect(err).To(HaveOccurred(), "audit collection failure must not be silently successful")
			body, err := os.ReadFile(filepath.Join(dir, "datastorage", "error.json"))
			Expect(err).ToNot(HaveOccurred(), "collector output: %s", output)
			var record struct {
				HTTPStatus string `json:"http_status"`
				ExitCode   int    `json:"exit_code"`
				Message    string `json:"message"`
			}
			Expect(json.Unmarshal(body, &record)).To(Succeed(), "error JSON must survive quotes/newlines")
			if duration == "nonsense" {
				Expect(record.Message).To(ContainSubstring("duration"))
			} else if wrongCA {
				Expect(record.ExitCode).ToNot(BeZero())
			} else {
				Expect(record.HTTPStatus).To(Equal(strconv.Itoa(status)))
			}
			Expect(output).ToNot(ContainSubstring(secret))
			Expect(string(body)).ToNot(ContainSubstring(secret))
		},
		Entry("unauthorized", http.StatusUnauthorized, "24h", false),
		Entry("forbidden", http.StatusForbidden, "24h", false),
		Entry("quoted/newline error body", http.StatusServiceUnavailable, "24h", false),
		Entry("wrong CA", http.StatusUnauthorized, "24h", true),
		Entry("invalid timeframe", http.StatusUnauthorized, "nonsense", false),
	)

	DescribeTable("IT-AF-2501-003/004: external network and hostname failures retain curl exit/status/stderr",
		func(failure string, expectedExit int) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{"data":[],"pagination":{"total":0}}`))
				Expect(err).ToNot(HaveOccurred())
			}))
			DeferCleanup(server.Close)
			dir := GinkgoT().TempDir()
			caPath, tokenPath := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "token")
			const secret = "synthetic-network-credential-2501"
			Expect(os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)).To(Succeed())
			Expect(os.WriteFile(tokenPath, []byte(secret), 0600)).To(Succeed())
			GinkgoT().Setenv("HTTPS_PROXY", "")
			GinkgoT().Setenv("ALL_PROXY", "")
			GinkgoT().Setenv("NO_PROXY", "*")
			authority := server.URL
			switch failure {
			case "DNS":
				authority = "https://missing-2501.invalid:443"
			case "TCP":
				server.Close()
			case "hostname":
				address := strings.TrimPrefix(server.URL, "https://")
				authority = "https://wrong-2501.invalid:443"
				GinkgoT().Setenv("DATASTORAGE_CONNECT_TO", "wrong-2501.invalid:443:"+address)
			}
			output, err := runDataStorageCollector(dir, authority, caPath, tokenPath, "24h")
			Expect(err).To(HaveOccurred(), "collector output: %s", output)
			body, err := os.ReadFile(filepath.Join(dir, "datastorage", "error.json"))
			Expect(err).ToNot(HaveOccurred())
			var record struct {
				HTTPStatus string `json:"http_status"`
				ExitCode   int    `json:"exit_code"`
				Stderr     string `json:"stderr"`
			}
			Expect(json.Unmarshal(body, &record)).To(Succeed())
			Expect(record.ExitCode).To(Equal(expectedExit))
			Expect(record.HTTPStatus).To(Equal("000"))
			Expect(record.Stderr).ToNot(BeEmpty())
			Expect(len(record.Stderr)).To(BeNumerically("<=", 4096))
			Expect(string(body)).ToNot(ContainSubstring(secret))
			Expect(output).ToNot(ContainSubstring(secret))
		}, Entry("unresolvable authority", "DNS", 6), Entry("refused owned TCP endpoint", "TCP", 7), Entry("verified CA but mismatched SAN", "hostname", 60))
})

func runDataStorageCollector(dir, url, caPath, tokenPath, duration string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", filepath.Join(getProjectRoot(), "cmd", "must-gather", "collectors", "datastorage.sh"), dir)
	cmd.Env = append(os.Environ(), "DATASTORAGE_URL="+url, "DATASTORAGE_CA_FILE="+caPath,
		"DATASTORAGE_TOKEN_FILE="+tokenPath, "AUDIT_TIMEFRAME="+duration, "DATASTORAGE_TIMEOUT=1")
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
