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

// Package tlscontract implements the upstream TLS deployment-contract
// guardrails for BR-SECURITY-2548 (issue #2548): no statically injected
// ambient CA environment (SSL_CERT_FILE / SSL_CERT_DIR) and no writable
// volume backing CA/TLS certificate material in any workload manifest
// owned by this repository or rendered by its CI. It is a test-owned
// support package (mirroring test/shared/helpers) and is never linked
// into production binaries.
//
// Control mapping: FedRAMP SC-8/SC-13, AC-6/CM-7; SOC 2 CC8.1 (change
// management guardrails); OWASP ASVS V12.3.1-V12.3.4, V15.2.5.
package tlscontract

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// AmbientEnvVars are the process-wide ambient trust variables that only
// pkg/shared/tls (the ambient CA owner per DD-TLS-001) may set. TLS_CA_FILE
// is deliberately absent: it remains a supported deployer-declared input for
// explicit reloadable transports (PR #2546).
var AmbientEnvVars = []string{
	"SSL_CERT_FILE",
	"SSL_CERT_DIR",
}

// ForbiddenWorkloadEnv reports whether an env var name is forbidden in
// workload manifests: static ambient CA overrides are not part of the
// deployment contract (the runtime adapter owns them).
func ForbiddenWorkloadEnv(name string) bool {
	for _, v := range AmbientEnvVars {
		if name == v {
			return true
		}
	}
	return false
}

// workloadKinds are the manifest kinds whose pod specs are scanned.
var workloadKinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
	"ReplicaSet":  true,
	"Pod":         true,
	"Job":         true,
	"CronJob":     true,
}

// envSourceKinds are the object kinds whose data keys can inject env vars
// through envFrom references.
var envSourceKinds = map[string]bool{
	"ConfigMap": true,
	"Secret":    true,
}

// certPathTokens drive the CA/TLS-material heuristic. Matching is
// token-based (whole path segment, whole file name, or '-'/'_'/'.'-separated
// token within a segment): "replicas-cache" must NOT be treated as CA
// material, while "signing-cert", "webhook-certs", and "tls-ca" must.
var certPathTokens = map[string]bool{
	"ca":    true,
	"cert":  true,
	"certs": true,
	"pki":   true,
	"tls":   true,
	"x509":  true,
}

// certFileSuffixes mark file names that are certificate/key material
// regardless of their stem.
var certFileSuffixes = []string{
	".crt",
	".pem",
	".key",
}

// IsCertMaterialPath reports whether a mount path, subPath, volume name, or
// volume source name indicates CA/TLS certificate material. Detection is a
// documented heuristic; the residual under-detection envelope (arbitrary
// names like a secret "cache" mounted at /var/lib/app) is recorded as an
// accepted risk in docs/security/TLS_WRITABLE_DIRECTORY_AUDIT_2548.md.
func IsCertMaterialPath(path string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	for _, segment := range strings.Split(cleaned, "/") {
		lower := strings.ToLower(segment)
		if certPathTokens[lower] {
			return true
		}
		for _, suffix := range certFileSuffixes {
			if strings.HasSuffix(lower, suffix) {
				return true
			}
		}
		for _, token := range strings.FieldsFunc(lower, func(r rune) bool {
			return r == '-' || r == '_' || r == '.'
		}) {
			if certPathTokens[token] {
				return true
			}
		}
	}
	return false
}

// readOnlyVolumeSources are the volume sources acceptable for CA/TLS
// material. emptyDir and hostPath are rejected (writable, persistent
// staging locations after compromise -- the threat model of issue #2548).
// Read-only PVC/CSI mounts are also rejected by design (fail-closed): the
// supported contract mounts trust material from secret/configMap/projected
// sources only.
var readOnlyVolumeSources = map[string]bool{
	"secret":    true,
	"configMap": true,
	"projected": true,
}

// Violation is a single deployment-contract breach found by a scan.
type Violation struct {
	File    string
	Kind    string
	Message string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s (%s): %s", v.File, v.Kind, v.Message)
}

// ScanResult carries scan findings plus corpus evidence used by the
// guardrail specs to prove they are not passing vacuously.
type ScanResult struct {
	Violations []Violation
	// WorkloadDocCount is the number of workload-kind YAML documents
	// actually inspected across the corpus.
	WorkloadDocCount int
	// CertMountCount is the number of CA/TLS-material volume mounts
	// matched in the corpus (the positive anchor for UT-TLS-2548-002).
	CertMountCount int
}

// HasViolations reports whether any contract breach was found.
func (r ScanResult) HasViolations() bool {
	return len(r.Violations) > 0
}

// SortedViolations returns violations in stable file order for assertions.
func (r ScanResult) SortedViolations() []string {
	out := make([]string, 0, len(r.Violations))
	for _, v := range r.Violations {
		out = append(out, v.String())
	}
	sort.Strings(out)
	return out
}

// manifestObject is one decoded top-level YAML document.
type manifestObject struct {
	kind    string
	name    string
	doc     map[string]any
	dataKey map[string]bool
}

// splitYAMLDocuments splits a multi-document YAML stream into per-document
// byte segments at "^---" and "..." separator lines (optional trailing
// comment allowed). CRLF line endings are normalized first so separators
// are not missed. Empty segments are preserved so document numbering in
// error messages matches the visible separator count.
func splitYAMLDocuments(data []byte) [][]byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	sep := regexp.MustCompile(`(?m)^(?:---|\.\.\.)[ \t]*(?:#.*)?$`)
	locs := sep.FindAllIndex(data, -1)
	if len(locs) == 0 {
		return [][]byte{data}
	}
	var segments [][]byte
	prev := 0
	for _, loc := range locs {
		segments = append(segments, data[prev:loc[0]])
		prev = loc[1]
	}
	segments = append(segments, data[prev:])
	return segments
}

// decodeSegment decodes exactly one YAML document from a pre-split
// segment. It fails closed if the segment still contains more than one
// document (an unrecognized separator form): yaml.Unmarshal would
// silently decode only the first document and drop the rest, which would
// hide violations, so residual documents are an error, never a skip.
func decodeSegment(path string, docNum int, seg []byte) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(seg))
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil // empty segment
		}
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("segment %d in %s contains multiple yaml documents after splitting (unrecognized document separator)", docNum, path)
	}
	return raw, nil
}

// decodeManifestDocs decodes every document of a multi-doc YAML stream,
// one document at a time. A document that fails to parse aborts the scan
// with the file path, EXCEPT when that document's own bytes contain
// Go-template placeholders ("{{"): a runtime-substituted template document
// cannot be applied by kubectl/helm as-is and only that document is
// skipped. Template placeholders in one document never mask a parse error
// or a violation in any other document of the same stream.
func decodeManifestDocs(path string, data []byte) ([]manifestObject, error) {
	segments := splitYAMLDocuments(data)
	var docs []manifestObject
	for i, seg := range segments {
		if len(bytes.TrimSpace(seg)) == 0 {
			continue
		}
		raw, err := decodeSegment(path, i+1, seg)
		if err != nil {
			if strings.Contains(string(seg), "{{") {
				continue // template document: skip this document only
			}
			return nil, fmt.Errorf("failed to parse yaml document %d in %s: %w", i+1, path, err)
		}
		if raw == nil {
			continue
		}
		docMap, ok := raw.(map[string]any)
		if !ok {
			continue // empty or non-mapping document
		}
		obj := manifestObject{doc: docMap}
		obj.kind, _ = docMap["kind"].(string)
		if meta, ok := docMap["metadata"].(map[string]any); ok {
			obj.name, _ = meta["name"].(string)
		}
		if envSourceKinds[obj.kind] {
			obj.dataKey = decodeDataKeys(docMap)
		}
		docs = append(docs, obj)
	}
	return docs, nil
}

func decodeDataKeys(docMap map[string]any) map[string]bool {
	keys := map[string]bool{}
	data, _ := docMap["data"].(map[string]any)
	for key := range data {
		keys[key] = true
	}
	stringData, _ := docMap["stringData"].(map[string]any)
	for key := range stringData {
		keys[key] = true
	}
	return keys
}

// buildEnvSourceIndex maps object name -> data keys for ConfigMap/Secret
// documents, so envFrom references can be checked for ambient env keys.
// Same-named sources within one stream union-merge: a clean source can
// never shadow one carrying an ambient env key (possible false positive,
// never a missed violation).
func buildEnvSourceIndex(docs []manifestObject) map[string]map[string]bool {
	index := map[string]map[string]bool{}
	for _, doc := range docs {
		if envSourceKinds[doc.kind] && doc.name != "" {
			mergeEnvSource(index, doc.name, doc.dataKey)
		}
	}
	return index
}

// mergeEnvSource union-merges data keys for one named envFrom source
// into an index, never overwriting existing keys.
func mergeEnvSource(index map[string]map[string]bool, name string, keys map[string]bool) {
	if existing, ok := index[name]; ok {
		for key := range keys {
			existing[key] = true
		}
		return
	}
	index[name] = keys
}

// ScanManifestData scans one multi-document YAML manifest file (the
// rendered operator bundles and repo fixtures use multi-doc layout) for
// ambient-CA contract violations. Non-workload documents (CRDs, Services,
// ...) are skipped, which also prevents ConfigMap-embedded
// `kind: Deployment` text from being misparsed. envFrom references are
// resolved against ConfigMap/Secret documents in the same stream; sources
// not present in the stream are not verifiable here (documented envelope).
func ScanManifestData(path string, data []byte) (ScanResult, error) {
	result := ScanResult{}
	docs, err := decodeManifestDocs(path, data)
	if err != nil {
		return result, err
	}
	return scanDocs(path, docs, buildEnvSourceIndex(docs)), nil
}

// ScanManifestCorpus scans a set of named manifest streams as one corpus:
// envFrom references resolve across every stream (union-merged index), and
// a parse failure in any stream aborts the whole scan (fail-closed). This
// is the entry point for multi-file rendered deployment surfaces such as
// the CI-rendered operator bundles.
func ScanManifestCorpus(streams map[string][]byte) (ScanResult, error) {
	result := ScanResult{}
	names := make([]string, 0, len(streams))
	for name := range streams {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic order for stable violation output

	type decodedStream struct {
		path string
		docs []manifestObject
	}
	var all []decodedStream
	corpusIndex := map[string]map[string]bool{}
	for _, name := range names {
		docs, err := decodeManifestDocs(name, streams[name])
		if err != nil {
			return result, err
		}
		all = append(all, decodedStream{path: name, docs: docs})
		for source, keys := range buildEnvSourceIndex(docs) {
			mergeEnvSource(corpusIndex, source, keys)
		}
	}
	for _, ds := range all {
		fileResult := scanDocs(ds.path, ds.docs, corpusIndex)
		result.Violations = append(result.Violations, fileResult.Violations...)
		result.WorkloadDocCount += fileResult.WorkloadDocCount
		result.CertMountCount += fileResult.CertMountCount
	}
	return result, nil
}

// scanDocs inspects decoded workload documents of one file for contract
// violations, resolving envFrom references against envSourceIndex (which
// may span the whole scanned corpus, not just this file).
func scanDocs(path string, docs []manifestObject, envSourceIndex map[string]map[string]bool) ScanResult {
	result := ScanResult{}
	for _, doc := range docs {
		if !workloadKinds[doc.kind] {
			continue
		}
		result.WorkloadDocCount++

		podSpec, ok := extractPodSpec(doc.kind, doc.doc)
		if !ok {
			result.Violations = append(result.Violations, Violation{
				File:    path,
				Kind:    doc.kind,
				Message: fmt.Sprintf("workload %q has no pod spec/template to scan (fail-closed for malformed workloads)", doc.name),
			})
			continue
		}
		scanPodSpec(path, doc.kind, podSpec, envSourceIndex, &result)
	}
	return result
}

// extractPodSpec navigates the workload-kind-specific path to the pod spec.
// Template-bearing kinds without a template are reported as malformed
// (ok=false), never silently skipped.
func extractPodSpec(kind string, docMap map[string]any) (map[string]any, bool) {
	spec, _ := docMap["spec"].(map[string]any)
	if spec == nil {
		return nil, false
	}
	// CronJob: spec.jobTemplate.spec.template.spec
	if kind == "CronJob" {
		jobTemplate, ok := spec["jobTemplate"].(map[string]any)
		if !ok {
			return nil, false
		}
		jtSpec, ok := jobTemplate["spec"].(map[string]any)
		if !ok {
			return nil, false
		}
		tpl, ok := jtSpec["template"].(map[string]any)
		if !ok {
			return nil, false
		}
		tplSpec, ok := tpl["spec"].(map[string]any)
		if !ok {
			return nil, false
		}
		return tplSpec, true
	}
	// Pod: spec itself
	if kind == "Pod" {
		return spec, true
	}
	// Deployment/StatefulSet/DaemonSet/ReplicaSet/Job: spec.template.spec
	tpl, ok := spec["template"].(map[string]any)
	if !ok {
		return nil, false
	}
	tplSpec, ok := tpl["spec"].(map[string]any)
	if !ok {
		return nil, false
	}
	return tplSpec, true
}

func scanPodSpec(path, kind string, podSpec map[string]any, envSourceIndex map[string]map[string]bool, result *ScanResult) {
	volumes := mapSlice(podSpec, "volumes")
	certVolumes := certMaterialVolumes(volumes)
	containers := append(append(
		mapSlice(podSpec, "containers"),
		mapSlice(podSpec, "initContainers")...),
		mapSlice(podSpec, "ephemeralContainers")...,
	)

	for _, container := range containers {
		containerName, _ := container["name"].(string)
		for _, env := range mapSlice(container, "env") {
			envName, _ := env["name"].(string)
			if ForbiddenWorkloadEnv(envName) {
				result.Violations = append(result.Violations, Violation{
					File:    path,
					Kind:    kind,
					Message: fmt.Sprintf("container %q statically injects ambient CA env %q (runtime-owned per DD-TLS-001)", containerName, envName),
				})
			}
		}
		for _, envFrom := range mapSlice(container, "envFrom") {
			scanEnvFrom(path, kind, containerName, envFrom, envSourceIndex, result)
		}
		for _, mount := range mapSlice(container, "volumeMounts") {
			mountPath, _ := mount["mountPath"].(string)
			subPath, _ := mount["subPath"].(string)
			volumeName := volumeNameFor(mount)
			if !IsCertMaterialPath(mountPath) && !IsCertMaterialPath(subPath) &&
				!IsCertMaterialPath(volumeName) && !certVolumes[volumeName] {
				continue
			}
			result.CertMountCount++
			if readOnly, ok := mount["readOnly"].(bool); !ok || !readOnly {
				result.Violations = append(result.Violations, Violation{
					File:    path,
					Kind:    kind,
					Message: fmt.Sprintf("container %q mounts CA/TLS material at %q without readOnly: true", containerName, mountPath),
				})
			}
			if !hasReadOnlyVolumeSource(volumes, volumeName) {
				result.Violations = append(result.Violations, Violation{
					File:    path,
					Kind:    kind,
					Message: fmt.Sprintf("CA/TLS material volume %q is not backed by secret/configMap/projected (emptyDir and hostPath are forbidden)", volumeName),
				})
			}
		}
	}
}

// scanEnvFrom checks that a configMapRef/secretRef source does not carry
// ambient CA env keys. Sources not present in the scanned stream cannot be
// verified and are skipped (documented detection envelope).
func scanEnvFrom(path, kind, containerName string, envFrom map[string]any, envSourceIndex map[string]map[string]bool, result *ScanResult) {
	for _, refField := range []string{"configMapRef", "secretRef"} {
		ref, ok := envFrom[refField].(map[string]any)
		if !ok {
			continue
		}
		refName, _ := ref["name"].(string)
		dataKeys, found := envSourceIndex[refName]
		if !found {
			continue
		}
		for _, envVar := range AmbientEnvVars {
			if dataKeys[envVar] {
				result.Violations = append(result.Violations, Violation{
					File:    path,
					Kind:    kind,
					Message: fmt.Sprintf("container %q envFrom source %q carries ambient CA env key %q (runtime-owned per DD-TLS-001)", containerName, refName, envVar),
				})
			}
		}
	}
}

// certMaterialVolumes returns the set of volume names whose source name
// (secret.secretName, configMap.name, or a projected source) indicates
// CA/TLS material.
func certMaterialVolumes(volumes []map[string]any) map[string]bool {
	certVolumes := map[string]bool{}
	for _, volume := range volumes {
		name, _ := volume["name"].(string)
		sourceNames := volumeSourceNames(volume)
		for _, sourceName := range sourceNames {
			if IsCertMaterialPath(sourceName) {
				certVolumes[name] = true
				break
			}
		}
	}
	return certVolumes
}

func volumeSourceNames(volume map[string]any) []string {
	var names []string
	if secret, ok := volume["secret"].(map[string]any); ok {
		if n, _ := secret["secretName"].(string); n != "" {
			names = append(names, n)
		}
	}
	if configMap, ok := volume["configMap"].(map[string]any); ok {
		if n, _ := configMap["name"].(string); n != "" {
			names = append(names, n)
		}
	}
	if projected, ok := volume["projected"].(map[string]any); ok {
		for _, source := range mapSlice(projected, "sources") {
			if secret, ok := source["secret"].(map[string]any); ok {
				if n, _ := secret["name"].(string); n != "" {
					names = append(names, n)
				}
			}
			if configMap, ok := source["configMap"].(map[string]any); ok {
				if n, _ := configMap["name"].(string); n != "" {
					names = append(names, n)
				}
			}
		}
	}
	return names
}

func volumeNameFor(mount map[string]any) string {
	name, _ := mount["name"].(string)
	return name
}

func hasReadOnlyVolumeSource(volumes []map[string]any, volumeName string) bool {
	for _, volume := range volumes {
		name, _ := volume["name"].(string)
		if name != volumeName {
			continue
		}
		for source := range readOnlyVolumeSources {
			if _, ok := volume[source]; ok {
				return true
			}
		}
		return false
	}
	return false
}

// mapSlice extracts a list-of-maps field (volumes, containers, env,
// volumeMounts) from a decoded YAML mapping.
func mapSlice(parent map[string]any, field string) []map[string]any {
	raw, ok := parent[field].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Exclusion is an explicitly pinned skip-list entry for manifest scanning.
// Exclusions are loud (asserted in the guardrail spec), never silent root
// omissions.
type Exclusion struct {
	// Path is the repository-relative directory prefix to skip.
	Path string
	// PinnedVersion must appear in the excluded file names (vendored
	// upstream pin, e.g. the Tekton release fixture).
	PinnedVersion string
	// Reason documents why the location is out of contract scope.
	Reason string
}

// ManifestExclusions lists the pinned exclusions for the repository corpus.
var ManifestExclusions = []Exclusion{
	{
		Path:          filepath.Join("test", "fixtures", "tekton", "releases"),
		PinnedVersion: "v0.59.0",
		Reason:        "vendored upstream Tekton release manifest, pinned; its SSL_CERT_FILE/SSL_CERT_DIR point at configmap-mounted read-only paths",
	},
}

// corpusFile is one decoded manifest file of a scanned corpus.
type corpusFile struct {
	path string
	docs []manifestObject
}

// collectCorpusDocs walks roots, decoding every scanned YAML file and
// building a corpus-wide envFrom source index (ConfigMap/Secret name ->
// data keys) across all files.
func collectCorpusDocs(repoRoot string, roots []string, excluded map[string]bool) ([]corpusFile, map[string]map[string]bool, error) {
	corpusIndex := map[string]map[string]bool{}
	var decoded []corpusFile
	for _, root := range roots {
		absRoot := filepath.Join(repoRoot, root)
		if _, err := os.Stat(absRoot); err != nil {
			return nil, nil, fmt.Errorf("scan root %s is missing: %w", root, err)
		}
		err := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if excluded[path] {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}
			data, err := os.ReadFile(path) //nolint:gosec // G304: repository-relative walk
			if err != nil {
				return fmt.Errorf("failed to read manifest %s: %w", path, err)
			}
			docs, err := decodeManifestDocs(path, data)
			if err != nil {
				return err
			}
			decoded = append(decoded, corpusFile{path: path, docs: docs})
			// Union-merge same-named sources across files (and, via
			// buildEnvSourceIndex, within each stream): Kubernetes
			// allows the same ConfigMap/Secret name in different
			// namespaces. Union (never overwrite) so a same-named
			// clean source cannot shadow one carrying an ambient env
			// key -- the error direction is a possible false
			// positive, never a missed violation.
			for name, keys := range buildEnvSourceIndex(docs) {
				mergeEnvSource(corpusIndex, name, keys)
			}
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to walk scan root %s: %w", root, err)
		}
	}
	return decoded, corpusIndex, nil
}

// ScanManifestFiles scans every YAML file under roots (repository-relative,
// resolved against repoRoot) for ambient-CA deployment-contract violations,
// applying the pinned exclusion list. envFrom references are resolved
// against a corpus-wide index of ConfigMap/Secret documents, so a source
// defined in a different file of the same corpus is still verified.
func ScanManifestFiles(repoRoot string, roots []string) (ScanResult, error) {
	result := ScanResult{}
	excluded := map[string]bool{}
	for _, ex := range ManifestExclusions {
		exDir := filepath.Join(repoRoot, ex.Path)
		excluded[exDir] = true
		// Pin check: the excluded directory must exist and contain the
		// pinned version, so a future fixture move fails loudly here.
		pinned, err := containsPinnedVersion(exDir, ex.PinnedVersion)
		if err != nil {
			return result, fmt.Errorf("exclusion pin check failed for %s: %w", ex.Path, err)
		}
		if !pinned {
			return result, fmt.Errorf("exclusion pin %s no longer matches any file under %s", ex.PinnedVersion, ex.Path)
		}
	}

	decoded, corpusIndex, err := collectCorpusDocs(repoRoot, roots, excluded)
	if err != nil {
		return result, err
	}

	for _, fd := range decoded {
		fileResult := scanDocs(fd.path, fd.docs, corpusIndex)
		result.Violations = append(result.Violations, fileResult.Violations...)
		result.WorkloadDocCount += fileResult.WorkloadDocCount
		result.CertMountCount += fileResult.CertMountCount
	}
	return result, nil
}

func containsPinnedVersion(dir, version string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.Contains(d.Name(), version) {
			found = true
		}
		return nil
	})
	return found, err
}

// bannedWriteAPIs are the file-mutation APIs forbidden in the ambient CA
// owner package's non-test sources (DD-TLS-001 no-write contract).
var bannedWriteAPIs = map[string]bool{
	"CreateTemp":  true,
	"MkdirTemp":   true,
	"Create":      true,
	"OpenFile":    true,
	"WriteFile":   true,
	"Mkdir":       true,
	"MkdirAll":    true,
	"Symlink":     true,
	"Rename":      true,
	"Remove":      true,
	"RemoveAll":   true,
	"Truncate":    true,
	"Chmod":       true,
	"Chown":       true,
}

// SourceViolation is a source-level guardrail finding.
type SourceViolation struct {
	File    string
	Line    int
	Message string
}

func (v SourceViolation) String() string {
	return fmt.Sprintf("%s:%d: %s", v.File, v.Line, v.Message)
}

// ScanTLSSourceWrites verifies pkg/shared/tls non-test sources contain no
// file-creation/mutation APIs: the ambient CA trust path must stay no-write
// (DD-TLS-001). Comment mentions are immune because this is AST-based, and
// aliased imports (e.g. `ose "os"`) are resolved through the file's import
// list.
func ScanTLSSourceWrites(tlsPkgDir string) ([]SourceViolation, error) {
	return scanGoSources(tlsPkgDir, func(file *ast.File, fset *token.FileSet, path string) []SourceViolation {
		writePackageAliases := importAliases(file, "os", "io/ioutil")
		var found []SourceViolation
		ast.Inspect(file, func(n ast.Node) bool {
			selector, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if writePackageAliases[ident.Name] && bannedWriteAPIs[selector.Sel.Name] {
				found = append(found, SourceViolation{
					File:    path,
					Line:    fset.Position(selector.Pos()).Line,
					Message: fmt.Sprintf("%s is forbidden in the no-write ambient CA package (aliased import of os/ioutil)", ident.Name),
				})
			}
			return true
		})
		return found
	})
}

// importAliases returns the identifiers a file uses for the given import
// paths: the explicit alias if present, otherwise the package's base name.
func importAliases(file *ast.File, paths ...string) map[string]bool {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")
		for _, want := range paths {
			if importPath != want {
				continue
			}
			if imp.Name != nil {
				aliases[imp.Name.Name] = true
			} else {
				aliases[filepath.Base(importPath)] = true
			}
		}
	}
	return aliases
}

// ScanAmbientEnvOwnership verifies that no Go source outside the ambient CA
// owner package (pkg/shared/tls) references the ambient trust env vars
// SSL_CERT_FILE / SSL_CERT_DIR as string literals -- covering both direct
// os.Setenv calls and variable-indirected ones. Env ownership cannot drift
// to another package unnoticed.
func ScanAmbientEnvOwnership(scanRoots []string, ownerDir string) ([]SourceViolation, error) {
	var found []SourceViolation
	for _, root := range scanRoots {
		violations, err := scanGoSources(root, func(file *ast.File, fset *token.FileSet, path string) []SourceViolation {
			var hits []SourceViolation
			if strings.HasPrefix(filepath.Clean(path), filepath.Clean(ownerDir)+string(filepath.Separator)) {
				return nil
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value := strings.Trim(lit.Value, "\"'`")
				for _, envVar := range AmbientEnvVars {
					if value == envVar {
						hits = append(hits, SourceViolation{
							File:    path,
							Line:    fset.Position(lit.Pos()).Line,
							Message: fmt.Sprintf("ambient CA env var %s may only be referenced inside pkg/shared/tls (the runtime owner per DD-TLS-001)", envVar),
						})
					}
				}
				return true
			})
			return hits
		})
		if err != nil {
			return nil, err
		}
		found = append(found, violations...)
	}
	return found, nil
}

func scanGoSources(root string, inspect func(*ast.File, *token.FileSet, string) []SourceViolation) ([]SourceViolation, error) {
	var found []SourceViolation
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip testdata and hidden directories.
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
		found = append(found, inspect(file, fset, path)...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk go sources under %s: %w", root, err)
	}
	return found, nil
}
