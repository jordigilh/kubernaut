// Package hash -- ConfigMap-aware composite hashing for #396 (BR-EM-004).
//
// ExtractConfigMapRefs walks a Kubernetes resource's unstructured .spec to find
// all ConfigMap references (volumes, projected volumes, envFrom, env valueFrom)
// across containers, initContainers, and ephemeralContainers. Kind-aware:
// resolves the pod template path based on the resource Kind.
//
// ConfigMapDataHash computes a deterministic SHA-256 of a ConfigMap's .data and
// .binaryData fields, suitable for inclusion in a composite spec hash.
//
// CompositeSpecHash combines a spec hash with sorted per-ConfigMap hashes into
// a single digest. Identity when no ConfigMap hashes are provided.
package hash

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

// ExtractConfigMapRefs returns a deduplicated, sorted list of ConfigMap names
// referenced by the given resource spec. The kind parameter determines how to
// locate the pod template within the spec:
//   - Deployment, StatefulSet, DaemonSet, ReplicaSet, Job: spec.template.spec
//   - Pod: spec directly
//   - CronJob: spec.jobTemplate.spec.template.spec
//   - All other kinds: returns nil (no pod template)
//
// Never panics on malformed input; uses defensive type assertions throughout.
func ExtractConfigMapRefs(spec map[string]interface{}, kind string) []string {
	podSpec := resolvePodSpec(spec, kind)
	if podSpec == nil {
		return nil
	}

	seen := map[string]bool{}

	extractFromVolumes(podSpec, seen)
	extractFromContainerSlice(podSpec, "containers", seen)
	extractFromContainerSlice(podSpec, "initContainers", seen)
	extractFromContainerSlice(podSpec, "ephemeralContainers", seen)

	if len(seen) == 0 {
		return nil
	}

	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// resolvePodSpec navigates to the pod spec within the resource spec based on Kind.
func resolvePodSpec(spec map[string]interface{}, kind string) map[string]interface{} {
	switch kind {
	case "Pod":
		return spec
	case "CronJob":
		return nestedMap(spec, "jobTemplate", "spec", "template", "spec")
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		return nestedMap(spec, "template", "spec")
	default:
		return nil
	}
}

// nestedMap traverses a chain of map keys, returning nil if any step fails.
func nestedMap(m map[string]interface{}, keys ...string) map[string]interface{} {
	current := m
	for _, k := range keys {
		val, ok := current[k]
		if !ok {
			return nil
		}
		next, ok := val.(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// extractFromVolumes collects ConfigMap names from volumes[].configMap.name and
// volumes[].projected.sources[].configMap.name.
func extractFromVolumes(podSpec map[string]interface{}, seen map[string]bool) {
	volumes, ok := podSpec["volumes"].([]interface{})
	if !ok {
		return
	}
	for _, v := range volumes {
		vol, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if cm, ok := vol["configMap"].(map[string]interface{}); ok {
			if name, ok := cm["name"].(string); ok && name != "" {
				seen[name] = true
			}
		}
		if proj, ok := vol["projected"].(map[string]interface{}); ok {
			if sources, ok := proj["sources"].([]interface{}); ok {
				for _, s := range sources {
					src, ok := s.(map[string]interface{})
					if !ok {
						continue
					}
					if cm, ok := src["configMap"].(map[string]interface{}); ok {
						if name, ok := cm["name"].(string); ok && name != "" {
							seen[name] = true
						}
					}
				}
			}
		}
	}
}

// extractFromContainerSlice collects ConfigMap names from envFrom[].configMapRef
// and env[].valueFrom.configMapKeyRef across all containers in the given key.
func extractFromContainerSlice(podSpec map[string]interface{}, key string, seen map[string]bool) {
	containers, ok := podSpec[key].([]interface{})
	if !ok {
		return
	}
	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if envFroms, ok := container["envFrom"].([]interface{}); ok {
			for _, ef := range envFroms {
				entry, ok := ef.(map[string]interface{})
				if !ok {
					continue
				}
				if ref, ok := entry["configMapRef"].(map[string]interface{}); ok {
					if name, ok := ref["name"].(string); ok && name != "" {
						seen[name] = true
					}
				}
			}
		}
		if envs, ok := container["env"].([]interface{}); ok {
			for _, e := range envs {
				env, ok := e.(map[string]interface{})
				if !ok {
					continue
				}
				if vf, ok := env["valueFrom"].(map[string]interface{}); ok {
					if ref, ok := vf["configMapKeyRef"].(map[string]interface{}); ok {
						if name, ok := ref["name"].(string); ok && name != "" {
							seen[name] = true
						}
					}
				}
			}
		}
	}
}

// ConfigMapDataHash computes a deterministic SHA-256 hash of a ConfigMap's
// content. String data keys are serialized as "key=value"; binary data keys
// are serialized as "key=base64(<bytes>)". All keys are sorted before hashing.
// Returns "sha256:<64-lowercase-hex>" (71 chars total).
func ConfigMapDataHash(data map[string]string, binaryData map[string][]byte) (string, error) {
	var parts []string

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("d:%s=%s", k, data[k]))
	}

	bkeys := make([]string, 0, len(binaryData))
	for k := range binaryData {
		bkeys = append(bkeys, k)
	}
	sort.Strings(bkeys)
	for _, k := range bkeys {
		parts = append(parts, fmt.Sprintf("b:%s=%s", k, base64.StdEncoding.EncodeToString(binaryData[k])))
	}

	serialized := strings.Join(parts, "\n")
	h := sha256.Sum256([]byte(serialized))
	return fmt.Sprintf("sha256:%x", h), nil
}

// CompositeSpecHash combines a spec hash with per-ConfigMap content hashes
// into a single composite digest. ConfigMap names are sorted before
// concatenation to ensure order independence.
//
// Identity: if configMapHashes is nil or empty, returns specHash unchanged.
// This preserves backward compatibility for resources with no ConfigMap refs.
//
// Deprecated: Use CompositeResourceFingerprint for new code (#765).
func CompositeSpecHash(specHash string, configMapHashes map[string]string) (string, error) {
	return CompositeResourceFingerprint(specHash, configMapHashes)
}

// CompositeResourceFingerprint combines a resource fingerprint with per-ConfigMap
// content hashes into a single composite digest (#765, DD-EM-002 v2.0).
//
// ConfigMap names are sorted before concatenation to ensure order independence.
// Identity: if configMapHashes is nil or empty, returns fingerprint unchanged.
//
// Note: Secrets are excluded from cascading per project policy (Vault-managed,
// rotational, not functional configuration state).
func CompositeResourceFingerprint(fingerprint string, configMapHashes map[string]string) (string, error) {
	if len(configMapHashes) == 0 {
		return fingerprint, nil
	}

	names := make([]string, 0, len(configMapHashes))
	for name := range configMapHashes {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(fingerprint)
	for _, name := range names {
		b.WriteString("\n")
		b.WriteString(name)
		b.WriteString(":")
		b.WriteString(configMapHashes[name])
	}

	h := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("sha256:%x", h), nil
}
