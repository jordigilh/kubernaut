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

package infrastructure

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/shared/tlscontract"
)

// BR-SECURITY-2548 (issue #2548): upstream TLS deployment-contract
// guardrails. FedRAMP SC-8/SC-13, AC-6/CM-7; SOC 2 CC8.1; OWASP ASVS
// V12.3.1-V12.3.4, V15.2.5. The runtime no-write contract itself is proven
// by UT/IT-TLS-2545; these specs pin the repository and deployment
// surfaces so the contract cannot regress silently.
var _ = Describe("TLS ambient CA deployment contract [BR-SECURITY-2548]", func() {
	var repoRoot string

	BeforeEach(func() {
		repoRoot = getProjectRoot()
	})

	manifestRoots := []string{
		"test",
		"config",
		"hack",
	}

	// UT-TLS-2548-001: no repo-owned workload manifest statically injects
	// the ambient CA env vars; the runtime adapter owns SSL_CERT_FILE and
	// SSL_CERT_DIR (DD-TLS-001). TLS_CA_FILE remains a supported explicit
	// reloadable-transport input and is not flagged.
	It("UT-TLS-2548-001: repo manifests never statically inject ambient CA env", func() {
		result, err := tlscontract.ScanManifestFiles(repoRoot, manifestRoots)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.HasViolations()).To(BeFalse(), strings.Join(result.SortedViolations(), "\n"))
		// Anti-vacuity: the corpus must actually contain workload docs.
		Expect(result.WorkloadDocCount).To(BeNumerically(">", 10),
			"manifest corpus must contain a meaningful number of workload documents for BR-SECURITY-2548")
	})

	// UT-TLS-2548-002: CA/TLS material is only ever mounted read-only from
	// secret/configMap/projected volumes -- never emptyDir or hostPath, the
	// post-compromise staging locations this issue exists to eliminate.
	It("UT-TLS-2548-002: CA/TLS material mounts are read-only and never emptyDir", func() {
		result, err := tlscontract.ScanManifestFiles(repoRoot, manifestRoots)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.HasViolations()).To(BeFalse(), strings.Join(result.SortedViolations(), "\n"))
		// Anti-vacuity: at least the datastorage signing-cert mount
		// (test/fixtures/kubernetes/datastorage/deployment.yaml, /etc/certs)
		// must be matched by the CA-material heuristic.
		Expect(result.CertMountCount).To(BeNumerically(">", 0),
			"CA-material heuristic matched no mounts; the positive corpus anchor is missing for BR-SECURITY-2548")
	})

	// UT-TLS-2548-003: the detector itself is proven non-vacuous and
	// false-positive-free on adversarial fixtures: every planted violation
	// must be caught, and CA-like names that are not CA material
	// (replicas-cache) must NOT be flagged.
	It("UT-TLS-2548-003: detector catches planted violations and ignores lookalikes", func() {
		staticEnvFixture := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-static-env
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          env:
            - name: SSL_CERT_DIR
              value: /etc/custom-ca
`
		envResult, err := tlscontract.ScanManifestData("planted-static-env.yaml", []byte(staticEnvFixture))
		Expect(err).ToNot(HaveOccurred())
		Expect(envResult.Violations).To(HaveLen(1))
		Expect(envResult.Violations[0].String()).To(ContainSubstring("SSL_CERT_DIR"))

		writableCAMountFixture := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-writable-ca
spec:
  template:
    spec:
      volumes:
        - name: ca-cert
          emptyDir: {}
      containers:
        - name: app
          image: registry.example/app:1
          volumeMounts:
            - name: ca-cert
              mountPath: /etc/certs
`
		mountResult, err := tlscontract.ScanManifestData("planted-writable-ca.yaml", []byte(writableCAMountFixture))
		Expect(err).ToNot(HaveOccurred())
		Expect(mountResult.Violations).To(HaveLen(2))
		joined := strings.Join(mountResult.SortedViolations(), "\n")
		Expect(joined).To(ContainSubstring("readOnly"))
		Expect(joined).To(ContainSubstring("emptyDir"))

		envFromFixture := `apiVersion: v1
kind: ConfigMap
metadata:
  name: bad-env-source
data:
  SSL_CERT_DIR: /etc/custom-ca
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-envfrom
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          envFrom:
            - configMapRef:
                name: bad-env-source
`
		envFromResult, err := tlscontract.ScanManifestData("planted-envfrom.yaml", []byte(envFromFixture))
		Expect(err).ToNot(HaveOccurred())
		Expect(envFromResult.Violations).To(HaveLen(1))
		Expect(envFromResult.Violations[0].String()).To(ContainSubstring("envFrom"))

		lookalikeFixture := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: lookalike-not-ca
spec:
  template:
    spec:
      volumes:
        - name: replicas-cache
          emptyDir: {}
      containers:
        - name: app
          image: registry.example/app:1
          volumeMounts:
            - name: replicas-cache
              mountPath: /var/cache/replicas-cache
`
		lookalikeResult, err := tlscontract.ScanManifestData("planted-lookalike.yaml", []byte(lookalikeFixture))
		Expect(err).ToNot(HaveOccurred())
		Expect(lookalikeResult.HasViolations()).To(BeFalse(),
			"segment-based heuristic must not flag replicas-cache for BR-SECURITY-2548")

		malformedFixture := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: broken\nspec:\n  template:\n    spec:\n      containers:\n        - name: app\n          image: [unclosed\n"
		_, err = tlscontract.ScanManifestData("planted-malformed.yaml", []byte(malformedFixture))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("planted-malformed.yaml"),
			"a YAML syntax error must abort the scan naming the file, never loop or silently pass, for BR-SECURITY-2548")

		configMapEmbeddedFixture := `apiVersion: v1
kind: ConfigMap
metadata:
  name: embedded-doc-trap
data:
  deployment.yaml: |
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: trap
    spec:
      template:
        spec:
          containers:
            - name: app
              image: registry.example/app:1
              env:
                - name: SSL_CERT_FILE
                  value: /etc/ssl/custom.pem
`
		trapResult, err := tlscontract.ScanManifestData("planted-configmap-trap.yaml", []byte(configMapEmbeddedFixture))
		Expect(err).ToNot(HaveOccurred())
		Expect(trapResult.HasViolations()).To(BeFalse(),
			"ConfigMap-embedded workload text must be skipped by document-level kind filtering for BR-SECURITY-2548")

		// envFrom via secretRef with an ambient key must be flagged
		// (same contract as configMapRef).
		secretEnvFromFixture := `apiVersion: v1
kind: Secret
metadata:
  name: bad-secret-env
stringData:
  SSL_CERT_FILE: /etc/ssl/custom.pem
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-secret-envfrom
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          envFrom:
            - secretRef:
                name: bad-secret-env
`
		secretEnvFromResult, err := tlscontract.ScanManifestData("planted-secret-envfrom.yaml", []byte(secretEnvFromFixture))
		Expect(err).ToNot(HaveOccurred())
		Expect(secretEnvFromResult.Violations).To(HaveLen(1),
			"envFrom secretRef carrying SSL_CERT_FILE via stringData must be flagged for BR-SECURITY-2548")
		Expect(secretEnvFromResult.Violations[0].String()).To(ContainSubstring("envFrom"))
		Expect(secretEnvFromResult.Violations[0].String()).To(ContainSubstring("SSL_CERT_FILE"))

		// Violations planted in initContainers and ephemeralContainers
		// must be caught: the container scan chain covers all three
		// lists, not just main containers.
		initContainerFixture := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-init-ephemeral
spec:
  template:
    spec:
      initContainers:
        - name: bootstrap
          image: registry.example/init:1
          env:
            - name: SSL_CERT_DIR
              value: /etc/custom-ca
      containers:
        - name: app
          image: registry.example/app:1
      ephemeralContainers:
        - name: debug
          image: registry.example/debug:1
          env:
            - name: SSL_CERT_FILE
              value: /etc/ssl/custom.pem
`
		initContainerResult, err := tlscontract.ScanManifestData("planted-init-ephemeral.yaml", []byte(initContainerFixture))
		Expect(err).ToNot(HaveOccurred())
		Expect(initContainerResult.Violations).To(HaveLen(2),
			"ambient CA env in initContainers and ephemeralContainers must both be flagged for BR-SECURITY-2548")
		joinedInit := strings.Join(initContainerResult.SortedViolations(), "\n")
		Expect(joinedInit).To(ContainSubstring("bootstrap"))
		Expect(joinedInit).To(ContainSubstring("debug"))

		// A mount whose only CA-material signal is a .pem-suffixed
		// name must be matched by the suffix heuristic.
		suffixOnlyFixture := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-suffix-only
spec:
  template:
    spec:
      volumes:
        - name: leaf.pem
          emptyDir: {}
      containers:
        - name: app
          image: registry.example/app:1
          volumeMounts:
            - name: leaf.pem
              mountPath: /data
`
		suffixOnlyResult, err := tlscontract.ScanManifestData("planted-suffix-only.yaml", []byte(suffixOnlyFixture))
		Expect(err).ToNot(HaveOccurred())
		Expect(suffixOnlyResult.Violations).To(HaveLen(2),
			"a .pem-suffixed volume name alone must trigger the CA-material heuristic for BR-SECURITY-2548")
		joinedSuffix := strings.Join(suffixOnlyResult.SortedViolations(), "\n")
		Expect(joinedSuffix).To(ContainSubstring("readOnly"))
		Expect(joinedSuffix).To(ContainSubstring("emptyDir"))
	})

	// UT-TLS-2548-006: Go-template placeholders in one document of a
	// multi-doc stream must never mask a YAML syntax error or a planted
	// violation in another document of the same stream (fail-closed
	// decode: template documents are skipped individually, by their own
	// bytes, and every other document is still decoded and scanned).
	It("UT-TLS-2548-006: template placeholders in one doc do not mask errors or violations in others", func() {
		mixedStream := `apiVersion: v1
kind: ConfigMap
metadata:
  name: template-bearing
data:
  notice: "{{ .Values.something }}"
---
bad: [unclosed
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-after-template
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          env:
            - name: SSL_CERT_DIR
              value: /etc/custom-ca
`
		_, err := tlscontract.ScanManifestData("planted-mixed-template.yaml", []byte(mixedStream))
		Expect(err).To(HaveOccurred(),
			"a genuine YAML syntax error must abort the scan even when another document carries template placeholders, for BR-SECURITY-2548")
		Expect(err.Error()).To(ContainSubstring("planted-mixed-template.yaml"))

		templateDocSkipped := `values: "{{ .Values.render-time }}"
extra: [unclosed
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-after-template-doc
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          env:
            - name: SSL_CERT_DIR
              value: /etc/custom-ca
`
		skipResult, err := tlscontract.ScanManifestData("planted-template-doc.yaml", []byte(templateDocSkipped))
		Expect(err).ToNot(HaveOccurred(),
			"a malformed template document is skipped individually without failing the whole stream, for BR-SECURITY-2548")
		Expect(skipResult.Violations).To(HaveLen(1),
			"the workload document after a skipped template document must still be scanned for BR-SECURITY-2548")
		Expect(skipResult.Violations[0].String()).To(ContainSubstring("SSL_CERT_DIR"))

		// CRLF streams must not silently drop documents after the
		// first: every separator must split, and a violating workload
		// in document 2 must still be found.
		crlfStream := strings.ReplaceAll(`apiVersion: v1
kind: ConfigMap
metadata:
  name: crlf-first
data:
  key: value
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: crlf-bad-second
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          env:
            - name: SSL_CERT_DIR
              value: /etc/custom-ca
`, "\n", "\r\n")
		crlfResult, err := tlscontract.ScanManifestData("planted-crlf.yaml", []byte(crlfStream))
		Expect(err).ToNot(HaveOccurred(),
			"CRLF line endings must not break document splitting for BR-SECURITY-2548")
		Expect(crlfResult.Violations).To(HaveLen(1),
			"the second document of a CRLF stream must still be scanned for BR-SECURITY-2548")
		Expect(crlfResult.Violations[0].String()).To(ContainSubstring("SSL_CERT_DIR"))
	})

	// UT-TLS-2548-007: envFrom references resolve across files of the
	// scanned corpus, not just within one stream: a ConfigMap carrying
	// SSL_CERT_DIR in file A and a Deployment envFrom-ing it in file B
	// must be flagged.
	It("UT-TLS-2548-007: envFrom ambient env sources are resolved across corpus files", func() {
		corpusRoot := GinkgoT().TempDir()
		// The pinned Tekton exclusion directory must exist for the
		// exclusion pin check; recreate the pinned marker so the scan
		// runs against a self-contained corpus.
		tektonDir := filepath.Join(corpusRoot, "test", "fixtures", "tekton", "releases")
		Expect(os.MkdirAll(tektonDir, 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tektonDir, "tekton-v0.59.0.yaml"), []byte("# pinned upstream fixture\n"), 0o640)).To(Succeed())

		envSourceDir := filepath.Join(corpusRoot, "envsource")
		workloadDir := filepath.Join(corpusRoot, "workloads")
		Expect(os.MkdirAll(envSourceDir, 0o750)).To(Succeed())
		Expect(os.MkdirAll(workloadDir, 0o750)).To(Succeed())

		envSource := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cross-file-env
data:
  SSL_CERT_DIR: /etc/custom-ca
`
		workload := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: cross-file-envfrom
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          envFrom:
            - configMapRef:
                name: cross-file-env
`
		Expect(os.WriteFile(filepath.Join(envSourceDir, "configmap.yaml"), []byte(envSource), 0o640)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(workloadDir, "deployment.yaml"), []byte(workload), 0o640)).To(Succeed())

		result, err := tlscontract.ScanManifestFiles(corpusRoot, []string{"envsource", "workloads"})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Violations).To(HaveLen(1),
			"envFrom must resolve a same-corpus ConfigMap defined in a different file for BR-SECURITY-2548")
		Expect(result.Violations[0].String()).To(ContainSubstring("envFrom"))

		// Same-named sources in different files (e.g. different
		// namespaces) must union-merge: a clean same-named ConfigMap in
		// a later file must never shadow one carrying an ambient env
		// key -- the error direction is a possible false positive,
		// never a missed violation.
		shadowSource := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cross-file-env
  namespace: other
data:
  UNRELATED: value
`
		Expect(os.WriteFile(filepath.Join(envSourceDir, "configmap-shadow.yaml"), []byte(shadowSource), 0o640)).To(Succeed())
		shadowResult, err := tlscontract.ScanManifestFiles(corpusRoot, []string{"envsource", "workloads"})
		Expect(err).ToNot(HaveOccurred())
		Expect(shadowResult.Violations).To(HaveLen(1),
			"a same-named clean ConfigMap must not shadow one carrying SSL_CERT_DIR for BR-SECURITY-2548")
		Expect(shadowResult.Violations[0].String()).To(ContainSubstring("envFrom"))

		// Same-name shadowing within a single stream must also
		// union-merge: a clean same-named ConfigMap later in the same
		// stream must never shadow one carrying an ambient env key.
		inStreamShadow := `apiVersion: v1
kind: ConfigMap
metadata:
  name: in-stream-env
data:
  SSL_CERT_DIR: /etc/custom-ca
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: in-stream-env
  namespace: other
data:
  UNRELATED: value
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: in-stream-envfrom
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          envFrom:
            - configMapRef:
                name: in-stream-env
`
		inStreamResult, err := tlscontract.ScanManifestData("planted-instream-shadow.yaml", []byte(inStreamShadow))
		Expect(err).ToNot(HaveOccurred())
		Expect(inStreamResult.Violations).To(HaveLen(1),
			"a same-named clean ConfigMap in the same stream must not shadow one carrying SSL_CERT_DIR for BR-SECURITY-2548")
		Expect(inStreamResult.Violations[0].String()).To(ContainSubstring("envFrom"))
	})

	// UT-TLS-2548-008: the rendered-bundle entry point
	// (ScanManifestCorpus) resolves envFrom sources across separate
	// streams of one deployment surface, and aborts fail-closed when any
	// stream is malformed.
	It("UT-TLS-2548-008: rendered corpus scan resolves envFrom across streams and fails closed", func() {
		bundleA := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cross-bundle-env
data:
  SSL_CERT_DIR: /etc/custom-ca
`
		bundleB := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: cross-bundle-envfrom
spec:
  template:
    spec:
      containers:
        - name: app
          image: registry.example/app:1
          envFrom:
            - configMapRef:
                name: cross-bundle-env
`
		result, err := tlscontract.ScanManifestCorpus(map[string][]byte{
			"operator-dependencies-rendered.yaml": []byte(bundleA),
			"operator-bootstrap-rendered.yaml":    []byte(bundleB),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Violations).To(HaveLen(1),
			"envFrom must resolve a ConfigMap defined in a different stream of the same rendered corpus for BR-SECURITY-2548")
		Expect(result.Violations[0].String()).To(ContainSubstring("envFrom"))

		_, err = tlscontract.ScanManifestCorpus(map[string][]byte{
			"operator-bootstrap-rendered.yaml": []byte("bad: [unclosed\n"),
		})
		Expect(err).To(HaveOccurred(),
			"a malformed stream must abort the whole rendered-corpus scan for BR-SECURITY-2548")
		Expect(err.Error()).To(ContainSubstring("operator-bootstrap-rendered.yaml"))
	})

	// UT-TLS-2548-009: the documented init-only certificate-generation
	// exception (issue #2548 acceptance criterion 3): a Job may stage
	// CA material in a memory-backed emptyDir (bounded tmpfs) when its
	// root filesystem is read-only; disk-backed staging and long-running
	// kinds never qualify.
	It("UT-TLS-2548-009: init-only Job cert staging allows memory-backed emptyDir only", func() {
		jobMemoryBacked := `apiVersion: batch/v1
kind: Job
metadata:
  name: cert-bootstrap-memory
spec:
  template:
    spec:
      restartPolicy: Never
      volumes:
        - name: cert-bootstrap-tmp
          emptyDir:
            medium: Memory
      containers:
        - name: certificate-bootstrap
          image: registry.example/bootstrap:1
          securityContext:
            readOnlyRootFilesystem: true
          volumeMounts:
            - name: cert-bootstrap-tmp
              mountPath: /tmp
`
		memoryResult, err := tlscontract.ScanManifestData("job-memory-backed.yaml", []byte(jobMemoryBacked))
		Expect(err).ToNot(HaveOccurred())
		Expect(memoryResult.HasViolations()).To(BeFalse(),
			"a memory-backed emptyDir cert staging volume in an init-only Job is the documented exception for BR-SECURITY-2548")
		Expect(memoryResult.CertMountCount).To(BeNumerically(">", 0),
			"the exception fixture must still be counted as a CA-material mount for BR-SECURITY-2548")

		jobDiskBacked := strings.Replace(jobMemoryBacked, "name: cert-bootstrap-memory", "name: cert-bootstrap-disk", 1)
		jobDiskBacked = strings.Replace(jobDiskBacked, "            medium: Memory\n", "", 1)
		diskResult, err := tlscontract.ScanManifestData("job-disk-backed.yaml", []byte(jobDiskBacked))
		Expect(err).ToNot(HaveOccurred())
		Expect(diskResult.Violations).To(HaveLen(2),
			"disk-backed emptyDir cert staging must remain a violation even in an init-only Job for BR-SECURITY-2548")

		deploymentMemoryBacked := strings.Replace(jobMemoryBacked, "kind: Job", "kind: Deployment", 1)
		deployResult, err := tlscontract.ScanManifestData("deployment-memory-backed.yaml", []byte(deploymentMemoryBacked))
		Expect(err).ToNot(HaveOccurred())
		Expect(deployResult.Violations).To(HaveLen(2),
			"memory-backed cert staging must remain a violation in long-running workload kinds for BR-SECURITY-2548")
	})

	// UT-TLS-2548-004: the ambient CA owner package stays no-write
	// (DD-TLS-001): no file creation or mutation APIs in non-test sources.
	It("UT-TLS-2548-004: pkg/shared/tls sources contain no write or temp-file APIs", func() {
		tlsPkgDir := filepath.Join(repoRoot, "pkg", "shared", "tls")
		violations, err := tlscontract.ScanTLSSourceWrites(tlsPkgDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(violations).To(BeEmpty(),
			"no-write ambient CA contract violated for BR-SECURITY-2548: %s", sourceViolationLines(violations))
	})

	// UT-TLS-2548-005: ambient trust env ownership cannot drift out of
	// pkg/shared/tls: no other non-test Go source may reference
	// SSL_CERT_FILE / SSL_CERT_DIR as a string literal.
	It("UT-TLS-2548-005: ambient CA env ownership stays in pkg/shared/tls", func() {
		ownerDir := filepath.Join(repoRoot, "pkg", "shared", "tls")
		scanRoots := []string{
			filepath.Join(repoRoot, "pkg"),
			filepath.Join(repoRoot, "cmd"),
			filepath.Join(repoRoot, "internal"),
		}
		violations, err := tlscontract.ScanAmbientEnvOwnership(scanRoots, ownerDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(violations).To(BeEmpty(),
			"ambient env ownership drifted for BR-SECURITY-2548: %s", sourceViolationLines(violations))
	})
})

func sourceViolationLines(violations []tlscontract.SourceViolation) string {
	lines := make([]string, 0, len(violations))
	for _, v := range violations {
		lines = append(lines, v.String())
	}
	return strings.Join(lines, "\n")
}
