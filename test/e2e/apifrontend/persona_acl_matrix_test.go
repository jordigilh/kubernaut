package e2e_test

import (
	"fmt"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
)

// persona ACL-matrix E2E coverage (#1827).
//
// Prior E2E coverage exercised individual persona/tool pairs (e.g.
// TC-E2E-RAR-03 proves sre may kubernaut_approve, E2E-AF-WP-003 proves cicd
// may not kubernaut_discover_workflows), but no test confirmed the full
// values.yaml apifrontend.config.rbac.personas matrix is actually enforced
// end-to-end through AF's real SAR-backed Authorizer (pkg/apifrontend/auth/sar.go)
// for more than one or two personas/tools at a time.
//
// This suite derives its expectations directly from values.yaml (via
// kinfra.LoadPersonaToolsFromValuesYAML, the same source of truth
// PersonaToolClusterRolesYAML renders from -- see UT-INFRA-RBAC-002) rather
// than hand-declaring a second "expected" table that could itself drift, as
// happened to the AF-only E2E suite's RBAC bootstrap (2026-08-03 incident).
//
// probeTools is a small set of read-only, side-effect-free tools chosen
// because their persona coverage in values.yaml varies (granted to some
// personas, not others), so each one yields both positive and negative
// cases across the 6 personas without needing per-tool scenario data setup.
var probeTools = []string{
	"kubernaut_get_effectiveness",
	"kubernaut_list_workflows",
	"kubernaut_get_audit_trail",
	"kubernaut_list_approval_requests",
	"kubernaut_get_remediation_history",
}

// probeArgs returns harmless, always-optional-or-fake arguments for a probe
// tool so the call reaches AF's RBAC check and (if allowed) executes without
// tripping input validation first. The exact result content doesn't matter --
// only whether AF's response is an RBAC denial.
func probeArgs(tool string) map[string]interface{} {
	switch tool {
	case "kubernaut_get_audit_trail":
		return map[string]interface{}{"rr_id": "e2e-acl-matrix-probe"}
	default:
		return map[string]interface{}{}
	}
}

// personaSession caches one Dex JWT + MCP session per persona so the 5 probe
// tools for that persona share a single auth round-trip instead of each
// re-authenticating from scratch (see cachedPersonaSession's doc comment).
type personaSession struct {
	token     string
	sessionID string
}

// cachedPersonaSession lazily authenticates and opens an MCP session for
// persona, memoizing the result in cache for reuse across every probeTools
// entry for that persona.
//
// Fix for CI incident (2026-08-03, run against PR adding this suite): the
// original version called fetchDEXTokenForPersona + initMCPSession fresh in
// every one of the 30 (persona x tool) DescribeTable entries. Ginkgo
// distributes DescribeTable entries as independent specs across
// --procs=12, so this suite alone could put up to 30 concurrent fresh Dex
// authentications on the single Dex pod's single Kind hostPort -> NodePort
// path (already a known contention point per issue #1807, mitigated there
// with a 4-attempt retry+backoff for the *existing*, smaller call volume).
// That existing mitigation was not sized for this suite's 5x-larger burst:
// the run saw sustained "connection reset by peer"/EOF errors on
// https://localhost:5556/dex/token cascading into unrelated specs (in other
// files, running concurrently) also failing to obtain their own tokens, and
// the suite hit its 18-minute Ginkgo timeout with 81 failures.
//
// Caching one token+session per persona (in an Ordered container below, so
// this map is never accessed concurrently) cuts this suite's own Dex/MCP
// auth round-trips from 30 to 6 -- back in line with the per-suite load the
// #1807 retry logic was actually designed to absorb.
func cachedPersonaSession(cache map[string]personaSession, persona string) (personaSession, error) {
	if sess, ok := cache[persona]; ok {
		return sess, nil
	}
	tok, err := fetchDEXTokenForPersona(persona)
	if err != nil {
		return personaSession{}, fmt.Errorf("fetch token for %s: %w", persona, err)
	}
	sid, err := initMCPSession(tok)
	if err != nil {
		return personaSession{}, fmt.Errorf("init MCP session for %s: %w", persona, err)
	}
	sess := personaSession{token: tok, sessionID: sid}
	cache[persona] = sess
	return sess, nil
}

// callProbeTool invokes the probe tool using an already-authenticated
// persona session, reporting whether AF's RBAC layer denied it.
func callProbeTool(sess personaSession, persona, tool string) (text string, denied bool, err error) {
	body := buildJSONRPC(fmt.Sprintf("acl-%s-%s", persona, tool), "tools/call", map[string]interface{}{
		"name":      tool,
		"arguments": probeArgs(tool),
	})
	raw, code, err := mcpPOST(sess.token, sess.sessionID, body)
	if err != nil {
		return "", false, err
	}
	if code >= 400 {
		return string(raw), true, nil
	}

	text, toolIsError, err := parseMCPToolPayload(unwrapSSEDataLine(raw))
	if err != nil {
		return text, false, err
	}
	denied = toolIsError && strings.Contains(strings.ToLower(text), "permission denied")
	return text, denied, nil
}

// Ordered: forces all specs in this container to run sequentially on a
// single Ginkgo process (never parallelized against each other), which is
// what makes the sequential, unsynchronized tokenCache map below safe and
// also caps this suite's own Dex load to one in-flight auth at a time --
// see cachedPersonaSession's doc comment for the incident this fixes.
var _ = Describe("Persona ACL matrix enforcement (#1827)", Ordered, Label("e2e", "phase2", "rbac"), func() {
	var (
		personaTools map[string][]string
		tokenCache   map[string]personaSession
	)

	BeforeAll(func() {
		var err error
		personaTools, err = kinfra.LoadPersonaToolsFromValuesYAML()
		Expect(err).NotTo(HaveOccurred(), "values.yaml apifrontend.config.rbac.personas must be parseable")
		Expect(personaTools).NotTo(BeEmpty())
		tokenCache = make(map[string]personaSession, len(e2ePersonas))
	})

	// Sorted persona names so table entries are generated deterministically.
	personaNames := make([]string, 0, len(e2ePersonas))
	for name := range e2ePersonas {
		personaNames = append(personaNames, name)
	}
	sort.Strings(personaNames)

	entries := []TableEntry{}
	for _, persona := range personaNames {
		for _, tool := range probeTools {
			entries = append(entries, Entry(fmt.Sprintf("%s x %s", persona, tool), persona, tool))
		}
	}

	DescribeTable("AF grants or denies each probe tool exactly as values.yaml declares",
		func(persona, tool string) {
			tools, ok := personaTools[persona]
			Expect(ok).To(BeTrue(), "values.yaml apifrontend.config.rbac.personas must define persona %q", persona)
			allowed := false
			for _, t := range tools {
				if t == tool {
					allowed = true
					break
				}
			}

			sess, err := cachedPersonaSession(tokenCache, persona)
			Expect(err).NotTo(HaveOccurred(), "authentication for persona %s should not fail", persona)

			text, denied, err := callProbeTool(sess, persona, tool)
			Expect(err).NotTo(HaveOccurred(), "probe call for %s/%s should not error at the transport level", persona, tool)

			if allowed {
				Expect(denied).To(BeFalse(),
					"persona %q has %q in values.yaml but AF's /mcp endpoint denied it: %s", persona, tool, text)
			} else {
				Expect(denied).To(BeTrue(),
					"persona %q lacks %q in values.yaml but AF's /mcp endpoint allowed it: %s", persona, tool, text)
			}
		},
		entries,
	)
})
