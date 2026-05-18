package apifrontend_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tlswiring"
)

var _ = Describe("Integration: TLS wiring", func() {
	var certDir string

	BeforeEach(func() {
		certDir = generateCerts(GinkgoT())
	})

	It("completes a full TLS handshake end-to-end", func() {
		caFile := filepath.Join(certDir, "tls.crt")

		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "ok")
		})

		srv := &http.Server{Handler: mux}
		enabled, _, err := tlswiring.ConfigureServer(srv, certDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		tlsLn := tls.NewListener(ln, srv.TLSConfig)
		defer func() { _ = srv.Close() }()
		go func() { _ = srv.Serve(tlsLn) }()

		caCert, err := os.ReadFile(caFile)
		Expect(err).NotTo(HaveOccurred())
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)

		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
			Timeout:   5 * time.Second,
		}

		resp, err := client.Get(fmt.Sprintf("https://%s/healthz", ln.Addr().String()))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, _ := io.ReadAll(resp.Body)
		Expect(string(body)).To(Equal("ok"))
	})

	It("OutboundTransport connects to a TLS server", func() {
		caFile := filepath.Join(certDir, "tls.crt")

		mux := http.NewServeMux()
		mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, "pong")
		})

		srv := &http.Server{Handler: mux}
		enabled, _, err := tlswiring.ConfigureServer(srv, certDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		tlsLn := tls.NewListener(ln, srv.TLSConfig)
		defer func() { _ = srv.Close() }()
		go func() { _ = srv.Serve(tlsLn) }()

		rt, err := tlswiring.OutboundTransport(caFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(rt).NotTo(BeNil())

		client := &http.Client{Transport: rt, Timeout: 5 * time.Second}
		resp, err := client.Get(fmt.Sprintf("https://%s/ping", ln.Addr().String()))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		Expect(string(body)).To(Equal("pong"))
	})

	It("OutboundTransport rejects untrusted server", func() {
		differentCA := generateCA(GinkgoT())

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, "should not reach")
		})

		srv := &http.Server{Handler: mux}
		enabled, _, err := tlswiring.ConfigureServer(srv, certDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		tlsLn := tls.NewListener(ln, srv.TLSConfig)
		defer func() { _ = srv.Close() }()
		go func() { _ = srv.Serve(tlsLn) }()

		rt, err := tlswiring.OutboundTransport(differentCA)
		Expect(err).NotTo(HaveOccurred())

		client := &http.Client{Transport: rt, Timeout: 5 * time.Second}
		_, err = client.Get(fmt.Sprintf("https://%s/", ln.Addr().String()))
		Expect(err).To(HaveOccurred())
	})

	It("StartCertFileWatcher returns nil with nil reloader", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		watcher, err := tlswiring.StartCertFileWatcher(ctx, "/some/dir", nil, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(watcher).To(BeNil())
	})
})

func generateCerts(t GinkgoTInterface) string {
	dir := GinkgoT().TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	writePEM(filepath.Join(dir, "tls.crt"), "CERTIFICATE", caDER)
	appendPEM(filepath.Join(dir, "tls.crt"), "CERTIFICATE", serverDER)

	keyBytes, err := x509.MarshalECPrivateKey(serverKey)
	Expect(err).NotTo(HaveOccurred())
	writePEM(filepath.Join(dir, "tls.key"), "EC PRIVATE KEY", keyBytes)

	return dir
}

func generateCA(t GinkgoTInterface) string {
	dir := GinkgoT().TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(99),
		Subject:               pkix.Name{CommonName: "different-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())

	caFile := filepath.Join(dir, "ca.crt")
	writePEM(caFile, "CERTIFICATE", der)
	return caFile
}

func writePEM(path, blockType string, data []byte) {
	f, err := os.Create(path) //nolint:gosec
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = f.Close() }()
	Expect(pem.Encode(f, &pem.Block{Type: blockType, Bytes: data})).To(Succeed())
}

func appendPEM(path, blockType string, data []byte) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644) //nolint:gosec
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = f.Close() }()
	Expect(pem.Encode(f, &pem.Block{Type: blockType, Bytes: data})).To(Succeed())
}
