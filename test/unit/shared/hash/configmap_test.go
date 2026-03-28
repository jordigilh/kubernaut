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

package hash_test

import (
	hash "github.com/jordigilh/kubernaut/pkg/shared/hash"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// ConfigMap Reference Extraction Tests (#396, BR-EM-004)
//
// Contract: ExtractConfigMapRefs(spec map[string]interface{}, kind string) []string
//
// Walks 5 reference paths in the pod spec:
//   - volumes[].configMap.name
//   - volumes[].projected.sources[].configMap.name
//   - containers[].envFrom[].configMapRef.name
//   - containers[].env[].valueFrom.configMapKeyRef.name
//   - initContainers[] (same paths as containers)
//
// Kind-aware pod template resolution:
//   - Deployment/StatefulSet/DaemonSet/ReplicaSet/Job: spec.template.spec
//   - Pod: spec (direct)
//   - CronJob: spec.jobTemplate.spec.template.spec
//   - Non-workload (HPA, Service): empty slice
//
// Returns deduplicated, sorted []string. Never panics on malformed input.
// ========================================
var _ = Describe("ExtractConfigMapRefs (#396)", func() {

	It("UT-SH-396-001: should extract ConfigMap names from volumes[].configMap.name", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "app-config-vol",
							"configMap": map[string]interface{}{
								"name": "my-app-config",
							},
						},
						map[string]interface{}{
							"name": "secret-vol",
							"secret": map[string]interface{}{
								"secretName": "my-secret",
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{"name": "app", "image": "nginx:1.25"},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Deployment")
		Expect(refs).To(Equal([]string{"my-app-config"}))
	})

	It("UT-SH-396-002: should extract ConfigMap names from initContainers[].envFrom[].configMapRef.name", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"initContainers": []interface{}{
						map[string]interface{}{
							"name":  "init",
							"image": "busybox:1.36",
							"envFrom": []interface{}{
								map[string]interface{}{
									"configMapRef": map[string]interface{}{
										"name": "init-config",
									},
								},
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{"name": "app", "image": "nginx:1.25"},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Deployment")
		Expect(refs).To(Equal([]string{"init-config"}))
	})

	It("UT-SH-396-003: should extract ConfigMap names from volumes[].projected.sources[].configMap.name", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "combined",
							"projected": map[string]interface{}{
								"sources": []interface{}{
									map[string]interface{}{
										"configMap": map[string]interface{}{
											"name": "app-config",
										},
									},
									map[string]interface{}{
										"configMap": map[string]interface{}{
											"name": "logging-config",
										},
									},
									map[string]interface{}{
										"secret": map[string]interface{}{
											"name": "tls-cert",
										},
									},
								},
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{"name": "app", "image": "nginx:1.25"},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Deployment")
		Expect(refs).To(Equal([]string{"app-config", "logging-config"}))
	})

	It("UT-SH-396-004: should extract ConfigMap names from containers[].envFrom[].configMapRef.name", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.25",
							"envFrom": []interface{}{
								map[string]interface{}{
									"configMapRef": map[string]interface{}{
										"name": "env-config",
									},
								},
							},
						},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Deployment")
		Expect(refs).To(Equal([]string{"env-config"}))
	})

	It("UT-SH-396-005: should extract ConfigMap names from containers[].env[].valueFrom.configMapKeyRef.name", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.25",
							"env": []interface{}{
								map[string]interface{}{
									"name": "FEATURE_FLAGS",
									"valueFrom": map[string]interface{}{
										"configMapKeyRef": map[string]interface{}{
											"name": "feature-flags",
											"key":  "flags.json",
										},
									},
								},
								map[string]interface{}{
									"name":  "STATIC_VAR",
									"value": "static-value",
								},
							},
						},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Deployment")
		Expect(refs).To(Equal([]string{"feature-flags"}))
	})

	It("UT-SH-396-006: should deduplicate and sort ConfigMap names across all paths", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "shared-vol",
							"configMap": map[string]interface{}{
								"name": "shared-config",
							},
						},
						map[string]interface{}{
							"name": "another-vol",
							"configMap": map[string]interface{}{
								"name": "alpha-config",
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.25",
							"envFrom": []interface{}{
								map[string]interface{}{
									"configMapRef": map[string]interface{}{
										"name": "shared-config",
									},
								},
							},
						},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Deployment")
		Expect(refs).To(Equal([]string{"alpha-config", "shared-config"}),
			"should be deduplicated (shared-config appears once) and sorted alphabetically")
	})

	It("UT-SH-396-007: should return empty slice when no ConfigMap references exist", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.25",
							"env": []interface{}{
								map[string]interface{}{
									"name":  "VAR",
									"value": "static",
								},
							},
						},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Deployment")
		Expect(refs).To(BeEmpty())
	})

	It("UT-SH-396-008: should return empty slice for malformed spec without panic", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes":    "not-a-slice",
					"containers": float64(42),
				},
			},
		}

		Expect(func() {
			refs := hash.ExtractConfigMapRefs(spec, "Deployment")
			Expect(refs).To(BeEmpty())
		}).ToNot(Panic(), "malformed spec must not cause a panic")
	})

	It("UT-SH-396-009: should return empty slice for non-workload kind (HPA)", func() {
		spec := map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "my-app",
			},
			"minReplicas": float64(2),
			"maxReplicas": float64(10),
		}

		refs := hash.ExtractConfigMapRefs(spec, "HorizontalPodAutoscaler")
		Expect(refs).To(BeEmpty())
	})

	It("UT-SH-396-010: should extract from Pod spec directly (no .template nesting)", func() {
		spec := map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"name": "pod-config-vol",
					"configMap": map[string]interface{}{
						"name": "pod-config",
					},
				},
			},
			"containers": []interface{}{
				map[string]interface{}{"name": "app", "image": "nginx:1.25"},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Pod")
		Expect(refs).To(Equal([]string{"pod-config"}))
	})

	It("UT-SH-396-011: should extract from CronJob nested jobTemplate path", func() {
		spec := map[string]interface{}{
			"schedule": "*/5 * * * *",
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "cron-vol",
									"configMap": map[string]interface{}{
										"name": "cron-config",
									},
								},
							},
							"containers": []interface{}{
								map[string]interface{}{"name": "worker", "image": "worker:1.0"},
							},
						},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "CronJob")
		Expect(refs).To(Equal([]string{"cron-config"}))
	})

	It("UT-SH-396-023: should extract ConfigMap names from ephemeralContainers[].envFrom[].configMapRef.name", func() {
		spec := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{"name": "app", "image": "nginx:1.25"},
					},
					"ephemeralContainers": []interface{}{
						map[string]interface{}{
							"name":  "debugger",
							"image": "busybox",
							"envFrom": []interface{}{
								map[string]interface{}{
									"configMapRef": map[string]interface{}{
										"name": "debug-config",
									},
								},
							},
						},
					},
				},
			},
		}

		refs := hash.ExtractConfigMapRefs(spec, "Deployment")
		Expect(refs).To(Equal([]string{"debug-config"}))
	})
})

// ========================================
// ConfigMap Data Hashing Tests (#396, BR-EM-004)
//
// Contract: ConfigMapDataHash(data map[string]string, binaryData map[string][]byte) (string, error)
//
// Produces a deterministic SHA-256 hash of ConfigMap content.
// Keys are sorted, string values serialized directly, binary values base64-encoded.
// Format: "sha256:<64-lowercase-hex>" (71 chars).
// ========================================
var _ = Describe("ConfigMapDataHash (#396)", func() {

	It("UT-SH-396-012: should produce deterministic sha256 for sorted .data key-value pairs", func() {
		data := map[string]string{
			"config.yaml":   "server:\n  port: 8080",
			"settings.json": `{"debug": true}`,
		}

		h1, err1 := hash.ConfigMapDataHash(data, nil)
		h2, err2 := hash.ConfigMapDataHash(data, nil)

		Expect(err1).ToNot(HaveOccurred())
		Expect(err2).ToNot(HaveOccurred())
		Expect(h1).To(HavePrefix("sha256:"))
		Expect(h1).To(HaveLen(71))
		Expect(h1).To(Equal(h2), "same input must produce identical hash")
	})

	It("UT-SH-396-013: should include .binaryData in hash (base64-encoded)", func() {
		data := map[string]string{"key": "value"}
		binaryData := map[string][]byte{"cert.pem": []byte("binary-cert-content")}

		hWithBinary, err1 := hash.ConfigMapDataHash(data, binaryData)
		hWithoutBinary, err2 := hash.ConfigMapDataHash(data, nil)

		Expect(err1).ToNot(HaveOccurred())
		Expect(err2).ToNot(HaveOccurred())
		Expect(hWithBinary).To(HavePrefix("sha256:"))
		Expect(hWithBinary).To(HaveLen(71))
		Expect(hWithBinary).ToNot(Equal(hWithoutBinary),
			"binaryData must contribute to the hash")
	})

	It("UT-SH-396-014: should be key-order independent", func() {
		dataA := map[string]string{
			"z-last":  "value-z",
			"a-first": "value-a",
			"m-mid":   "value-m",
		}
		dataB := map[string]string{
			"a-first": "value-a",
			"m-mid":   "value-m",
			"z-last":  "value-z",
		}

		hA, errA := hash.ConfigMapDataHash(dataA, nil)
		hB, errB := hash.ConfigMapDataHash(dataB, nil)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errB).ToNot(HaveOccurred())
		Expect(hA).To(Equal(hB), "map iteration order must not affect hash")
	})

	It("UT-SH-396-015: should produce deterministic hash for empty data", func() {
		h1, err1 := hash.ConfigMapDataHash(map[string]string{}, nil)
		h2, err2 := hash.ConfigMapDataHash(map[string]string{}, nil)

		Expect(err1).ToNot(HaveOccurred())
		Expect(err2).ToNot(HaveOccurred())
		Expect(h1).To(HavePrefix("sha256:"))
		Expect(h1).To(HaveLen(71))
		Expect(h1).To(Equal(h2))
	})

	It("UT-SH-396-016: should produce distinct hash for absent sentinel vs empty data", func() {
		absentData := map[string]string{"__sentinel__": "__absent:my-config__"}
		emptyData := map[string]string{}

		hAbsent, errA := hash.ConfigMapDataHash(absentData, nil)
		hEmpty, errE := hash.ConfigMapDataHash(emptyData, nil)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errE).ToNot(HaveOccurred())
		Expect(hAbsent).ToNot(Equal(hEmpty),
			"sentinel must produce a distinct hash from empty ConfigMap data")
	})
})

// ========================================
// Composite Spec Hash Tests (#396, BR-EM-004)
//
// Contract: CompositeSpecHash(specHash string, configMapHashes map[string]string) (string, error)
//
// Combines spec hash with sorted ConfigMap hashes into a single digest.
// Identity: nil/empty configMapHashes returns specHash unchanged.
// Format: "sha256:<64-lowercase-hex>" (71 chars) when ConfigMaps present.
// ========================================
var _ = Describe("CompositeSpecHash (#396)", func() {

	var specHash string

	BeforeEach(func() {
		var err error
		specHash, err = hash.CanonicalSpecHash(map[string]interface{}{
			"replicas": float64(3),
		})
		Expect(err).ToNot(HaveOccurred())
	})

	It("UT-SH-396-017: should equal CanonicalSpecHash when no ConfigMaps (backward compat)", func() {
		composite, err := hash.CompositeSpecHash(specHash, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(composite).To(Equal(specHash),
			"no ConfigMaps means composite hash is the spec hash itself")

		compositeEmpty, err := hash.CompositeSpecHash(specHash, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		Expect(compositeEmpty).To(Equal(specHash),
			"empty map should also return spec hash unchanged")
	})

	It("UT-SH-396-018: should differ from spec-only hash when ConfigMap data present", func() {
		configMapHashes := map[string]string{
			"my-config": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}

		composite, err := hash.CompositeSpecHash(specHash, configMapHashes)
		Expect(err).ToNot(HaveOccurred())
		Expect(composite).To(HavePrefix("sha256:"))
		Expect(composite).To(HaveLen(71))
		Expect(composite).ToNot(Equal(specHash),
			"ConfigMap data must change the composite hash")
	})

	It("UT-SH-396-019: should be deterministic (same inputs -> same output)", func() {
		configMapHashes := map[string]string{
			"config-a": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}

		h1, err1 := hash.CompositeSpecHash(specHash, configMapHashes)
		h2, err2 := hash.CompositeSpecHash(specHash, configMapHashes)

		Expect(err1).ToNot(HaveOccurred())
		Expect(err2).ToNot(HaveOccurred())
		Expect(h1).To(Equal(h2))
	})

	It("UT-SH-396-020: should detect ConfigMap appearance (absent sentinel -> real data)", func() {
		absentHashes := map[string]string{
			"my-config": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		}
		realHashes := map[string]string{
			"my-config": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		}

		hAbsent, err1 := hash.CompositeSpecHash(specHash, absentHashes)
		hReal, err2 := hash.CompositeSpecHash(specHash, realHashes)

		Expect(err1).ToNot(HaveOccurred())
		Expect(err2).ToNot(HaveOccurred())
		Expect(hAbsent).ToNot(Equal(hReal),
			"ConfigMap appearance must be detectable via hash change")
	})

	It("UT-SH-396-021: should detect ConfigMap data change", func() {
		hashesV1 := map[string]string{
			"my-config": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		hashesV2 := map[string]string{
			"my-config": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}

		hV1, err1 := hash.CompositeSpecHash(specHash, hashesV1)
		hV2, err2 := hash.CompositeSpecHash(specHash, hashesV2)

		Expect(err1).ToNot(HaveOccurred())
		Expect(err2).ToNot(HaveOccurred())
		Expect(hV1).ToNot(Equal(hV2),
			"ConfigMap data change must produce a different composite hash")
	})

	It("UT-SH-396-022: should be name-sorted and order-independent for multiple ConfigMaps", func() {
		hashesAB := map[string]string{
			"config-a": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"config-b": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}
		hashesBA := map[string]string{
			"config-b": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"config-a": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}

		hAB, err1 := hash.CompositeSpecHash(specHash, hashesAB)
		hBA, err2 := hash.CompositeSpecHash(specHash, hashesBA)

		Expect(err1).ToNot(HaveOccurred())
		Expect(err2).ToNot(HaveOccurred())
		Expect(hAB).To(Equal(hBA),
			"ConfigMap name ordering in the map must not affect the composite hash")
	})
})
