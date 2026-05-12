package infrastructure

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// WaitForDeployment blocks until a Kubernetes deployment reaches Available status.
func WaitForDeployment(ctx context.Context, kubeContext, namespace, name string, timeout time.Duration) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--context", kubeContext,
		"rollout", "status", "deployment/"+name, "-n", namespace, fmt.Sprintf("--timeout=%ds", int(timeout.Seconds()))) // #nosec G204 -- test infrastructure
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WaitForEndpoint polls a URL until it returns 200 OK or the timeout expires.
// If caCertPath is non-empty, it is used as the TLS CA for HTTPS endpoints.
func WaitForEndpoint(ctx context.Context, url, caCertPath string, timeout time.Duration) error {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if caCertPath != "" {
		caCert, err := os.ReadFile(filepath.Clean(caCertPath)) // #nosec G304 -- test infrastructure, path from env
		if err != nil {
			return fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to add CA cert to pool")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: transport}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("endpoint %s not ready after %v: %w", url, timeout, lastErr)
}

// PortForward starts a kubectl port-forward process for the given service.
func PortForward(ctx context.Context, kubeContext, namespace, service string, localPort, remotePort int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "--context", kubeContext,
		"port-forward", "-n", namespace, "svc/"+service,
		fmt.Sprintf("%d:%d", localPort, remotePort)) // #nosec G204 -- test infrastructure
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("port-forward: %w", err)
	}
	return cmd, nil
}
