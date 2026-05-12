package e2e_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %s is not set", key))
	}
	return v
}

func newTLSClient(caCertPath string) *http.Client {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caCertPath != "" {
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			panic(fmt.Sprintf("read CA cert: %v", err))
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			panic("failed to add CA cert to pool")
		}
		tlsCfg.RootCAs = pool
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
}

// fetchDEXToken performs an OAuth2 Resource Owner Password Credentials grant
// against DEX to obtain a valid access token for E2E testing.
func fetchDEXToken(dexURL, clientID, clientSecret, username, password string) (string, error) {
	tokenURL := dexURL + "/token"
	data := url.Values{
		"grant_type":    {"password"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"username":      {username},
		"password":      {password},
		"scope":         {"openid email profile groups"},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, body)
	}

	// Extract access_token from JSON response
	bodyStr := string(body)
	start := strings.Index(bodyStr, `"id_token":"`)
	if start == -1 {
		return "", fmt.Errorf("id_token not found in response: %s", bodyStr)
	}
	start += len(`"id_token":"`)
	end := strings.Index(bodyStr[start:], `"`)
	if end == -1 {
		return "", fmt.Errorf("malformed token response")
	}
	return bodyStr[start : start+end], nil
}
