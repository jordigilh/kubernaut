package e2e_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("DS Audit Sink (G8)", Label("e2e", "phase4", "g8"), func() {
	var (
		authToken    string
		mcpSessionID string
		dsAuditURL   string
		dsClient     *http.Client
	)

	BeforeEach(func() {
		dsAuditURL = getEnvOrDefault("AF_E2E_DS_AUDIT_URL", "https://localhost:8089/api/v1/audit/events")

		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())

		kubeconfigPath := os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
		saToken, saErr := kinfra.GetServiceAccountToken(context.Background(), e2eNamespace, "apifrontend", kubeconfigPath)
		Expect(saErr).NotTo(HaveOccurred(), "SA token for DS audit auth")

		dsClient = &http.Client{
			Transport: testauth.NewServiceAccountTransportWithBase(saToken, &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // E2E test only
			}),
			Timeout: 10 * time.Second,
		}
	})

	mcpToolCall := func(toolName string, args map[string]interface{}) (string, error) {
		callBody := buildJSONRPC(fmt.Sprintf("g8-%s-%d", toolName, time.Now().UnixNano()),
			"tools/call", map[string]interface{}{
				"name":      toolName,
				"arguments": args,
			})
		raw, code, err := mcpPOST(authToken, mcpSessionID, callBody)
		if err != nil {
			return "", err
		}
		if code >= http.StatusBadRequest {
			return "", fmt.Errorf("HTTP %d: %s", code, string(raw))
		}
		payload := unwrapSSEDataLine(raw)
		text, toolErr, parseErr := parseMCPToolPayload(payload)
		if parseErr != nil {
			return text, parseErr
		}
		if toolErr {
			return text, fmt.Errorf("%s", text)
		}
		return text, nil
	}

	fetchAuditBody := func() ([]byte, int, error) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, dsAuditURL, http.NoBody)
		if err != nil {
			return nil, 0, err
		}
		resp, err := dsClient.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		b, rerr := io.ReadAll(resp.Body)
		return b, resp.StatusCode, rerr
	}

	auditBodyContainsTool := func(body []byte, toolSubstring string) bool {
		if len(body) == 0 {
			return false
		}
		s := strings.ToLower(string(body))
		return strings.Contains(s, strings.ToLower(toolSubstring))
	}

	It("TC-E2E-AUDIT-01: After A2A tool call -> DS QueryAuditEvents returns matching entry", func() {
		// DS is always deployed in AF E2E — verify reachability as a hard assertion.
		Eventually(func() error {
			_, code, rerr := fetchAuditBody()
			if rerr != nil {
				return fmt.Errorf("DS audit endpoint not reachable: %w", rerr)
			}
			if code == http.StatusUnauthorized || code == http.StatusForbidden ||
				code == http.StatusNotFound || code == http.StatusBadGateway || code == http.StatusServiceUnavailable {
				return fmt.Errorf("DS audit endpoint returned %d", code)
			}
			return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"DS audit endpoint (%s) must be reachable in E2E", dsAuditURL)

		_, err := mcpToolCall("kubernaut_list_remediations", map[string]interface{}{
			"namespace": "default",
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			body, code, rerr := fetchAuditBody()
			if rerr != nil || code != http.StatusOK {
				return false
			}
			return auditBodyContainsTool(body, "kubernaut_list_remediations")
		}, 60*time.Second, 2*time.Second).Should(BeTrue(), "DS audit API should list an event referencing kubernaut_list_remediations")
	})

})
