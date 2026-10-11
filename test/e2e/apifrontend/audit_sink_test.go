package e2e_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("DS Audit Sink (G8)", Label("e2e", "phase4", "g8"), func() {
	var (
		authToken         string
		mcpSessionID      string
		dsAuditURL        string
		dsClient          *http.Client
		actor             string
		queryClient       *ogenclient.Client
		clusterKubeconfig string
	)

	BeforeEach(func() {
		dsAuditURL = getEnvOrDefault("AF_E2E_DS_AUDIT_URL", e2eHostURL("https", 8089)+"/api/v1/audit/events")

		var err error
		// Use the worker's synchronized kubeconfig, including Fleet-only lanes.
		clusterKubeconfig = kubeconfigPath
		Expect(auditActorEmail).ToNot(BeEmpty(), "BeforeSuite must provision the test-owned OIDC identity")
		authToken, err = fetchDEXToken(dexURL, clientID, clientSecret, auditActorEmail, "password")
		Expect(err).NotTo(HaveOccurred())
		actor, err = auditOIDCSubject(authToken)
		Expect(err).NotTo(HaveOccurred())
		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())
		saToken, saErr := kinfra.GetServiceAccountToken(context.Background(), e2eNamespace, "apifrontend", kubeconfigPath)
		Expect(saErr).NotTo(HaveOccurred(), "SA token for DS audit auth")

		dsTransport, err := sharedtls.NewTLSTransport(kinfra.InterServiceCAPath(kubeconfigPath))
		Expect(err).NotTo(HaveOccurred())
		dsClient = &http.Client{
			Transport: testauth.NewServiceAccountTransportWithBase(saToken, dsTransport),
			Timeout:   10 * time.Second,
		}
		Expect(dsAuditURL).To(HaveSuffix("/api/v1/audit/events"))
		queryClient, err = ogenclient.NewClient(strings.TrimSuffix(dsAuditURL, "/api/v1/audit/events"), ogenclient.WithClient(dsClient))
		Expect(err).NotTo(HaveOccurred())
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

	It("TC-E2E-AUDIT-01 / E2E-AF-2501-001: real MCP call yields a new attributable structured DS success (BR-AUDIT-005)", func() {
		started := time.Now().UTC().Truncate(time.Second)
		params := ogenclient.QueryAuditEventsParams{
			EventType: ogenclient.NewOptString("apifrontend.tool.executed"), DetailKey: ogenclient.NewOptString("tool_name"),
			DetailValue: ogenclient.NewOptString("kubernaut_list_remediations"), Since: ogenclient.NewOptString(started.Format(time.RFC3339)),
		}
		var baselineRows []ogenclient.AuditEvent
		// DS is always deployed in AF E2E — verify reachability as a hard assertion.
		Eventually(func() error {
			queryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var err error
			baselineRows, err = kinfra.QueryAuditSnapshot(queryCtx, queryClient, params)
			return err
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"DS audit endpoint (%s) must be reachable in E2E", dsAuditURL)

		baseline := make(map[uuid.UUID]struct{}, len(baselineRows))
		for _, event := range baselineRows {
			baseline[event.EventID.Value] = struct{}{}
		}
		_, err := mcpToolCall("kubernaut_list_remediations", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred())

		var fresh ogenclient.AuditEvent
		Eventually(func() error {
			queryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			rows, err := kinfra.QueryAuditSnapshot(queryCtx, queryClient, params)
			if err != nil {
				return err
			}
			match, ok := kinfra.FindNewToolAudit(rows, kinfra.AuditObservation{Actor: actor, Tool: "kubernaut_list_remediations", Since: started, Baseline: baseline})
			if !ok {
				return fmt.Errorf("no fresh success: actor=%s baseline=%d rows=%d filters=%+v", actor, len(baseline), len(rows), params)
			}
			fresh = match
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed())
		correlated, err := kinfra.QueryAuditSnapshot(context.Background(), queryClient, ogenclient.QueryAuditEventsParams{CorrelationID: ogenclient.NewOptString(fresh.CorrelationID)})
		Expect(err).NotTo(HaveOccurred())
		Expect(correlated).To(ContainElement(HaveField("EventID", fresh.EventID)), "round-trip the actual persisted correlation, not MCP request/session IDs")
		AddReportEntry("persisted fresh audit UUID", fresh.EventID.Value.String())

		// E2E-AF-2501-002: exercise the same real external-image runner used by
		// failure collection, before removing the test-owned actor.
		collectorCtx, collectorCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer collectorCancel()
		image, err := kinfra.BuildMustGatherImageForE2E(collectorCtx, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		ownedClusters := apifrontendE2EClusterNames(afE2ELaneFromEnv(), fleetAFSetupAttempted)
		Expect(ownedClusters).ToNot(BeEmpty())
		opts := kinfra.RunMustGatherImageOptions{ClusterName: ownedClusters[0], Image: image,
			OutputDir: filepath.Join(os.TempDir(), "kubernaut-must-gather", "audit-proof", fresh.EventID.Value.String()), Namespace: e2eNamespace, UsePodman: true}
		// DS serves the inter-service leaf, not AF's separately generated leaf.
		cleanup, err := kinfra.PrepareMustGatherDataStorage(collectorCtx, &opts, clusterKubeconfig, kinfra.InterServiceCAPath(clusterKubeconfig))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(cleanup()).To(Succeed()) })
		Expect(kinfra.RunMustGatherImage(collectorCtx, opts, GinkgoWriter)).To(Succeed())
		paths, err := filepath.Glob(filepath.Join(opts.OutputDir, "*", "datastorage", "audit-events.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(paths).To(HaveLen(1))
		captured, err := os.ReadFile(paths[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(captured)).To(ContainSubstring(fresh.EventID.Value.String()))

		// Prove delivery in the packaged artifact too, and check every member
		// for the real short-lived collector credential without logging it.
		token, err := os.ReadFile(opts.DataStorage.TokenFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(token).ToNot(BeEmpty())
		archives, err := filepath.Glob(filepath.Join(opts.OutputDir, "*.tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		Expect(archives).To(HaveLen(1))
		archive, err := os.Open(archives[0])
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(archive.Close()).To(Succeed()) })
		compressed, err := gzip.NewReader(archive)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(compressed.Close()).To(Succeed()) })
		members := tar.NewReader(compressed)
		found := false
		for {
			header, err := members.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			body, err := io.ReadAll(members)
			Expect(err).NotTo(HaveOccurred())
			Expect(bytes.Contains(body, token)).To(BeFalse(), "collector credential leaked in archive member %s", header.Name)
			if strings.HasSuffix(header.Name, "/datastorage/audit-events.json") {
				Expect(string(body)).To(ContainSubstring(fresh.EventID.Value.String()))
				found = true
			}
		}
		Expect(found).To(BeTrue(), "packaged archive must deliver the actual persisted audit UUID")
	})

})
