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
	"strings"

	hash "github.com/jordigilh/kubernaut/pkg/shared/hash"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// Canonical Spec Hash Tests (DD-EM-002)
//
// Contract: CanonicalSpecHash(spec map[string]interface{}) -> (string, error)
//
// Guarantees from DD-EM-002:
//   - Idempotent: same content always produces same hash
//   - Map-order independent
//   - Slice-order independent
//   - SHA-256 strength
//   - Human-readable hex format: "sha256:<64-char-hex>" (71 chars total)
//   - Cross-process portable
//
// Test IDs from DD-EM-002 Testing Requirements table.
// ========================================
var _ = Describe("CanonicalSpecHash (DD-EM-002)", func() {

	// UT-HASH-001: Map key order independence
	It("UT-HASH-001: should produce identical hash regardless of map key iteration order", func() {
		// Go maps have non-deterministic iteration order.
		// We create two maps with the same content and verify they hash identically.
		specA := map[string]interface{}{
			"replicas": float64(3),
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "nginx",
				},
			},
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{"name": "nginx", "image": "nginx:1.21"},
					},
				},
			},
		}

		specB := map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{"image": "nginx:1.21", "name": "nginx"},
					},
				},
			},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "nginx",
				},
			},
			"replicas": float64(3),
		}

		hashA, errA := hash.CanonicalSpecHash(specA)
		hashB, errB := hash.CanonicalSpecHash(specB)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errB).ToNot(HaveOccurred())
		Expect(hashA).To(Equal(hashB), "Same content with different key order should produce identical hash")
	})

	// UT-HASH-002: Slice order independence
	It("UT-HASH-002: should produce identical hash regardless of slice element order", func() {
		specA := map[string]interface{}{
			"items": []interface{}{"alpha", "bravo", "charlie"},
		}
		specB := map[string]interface{}{
			"items": []interface{}{"charlie", "alpha", "bravo"},
		}

		hashA, errA := hash.CanonicalSpecHash(specA)
		hashB, errB := hash.CanonicalSpecHash(specB)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errB).ToNot(HaveOccurred())
		Expect(hashA).To(Equal(hashB), "Same slice content in different order should produce identical hash")
	})

	// UT-HASH-003: Nested map + slice normalization
	It("UT-HASH-003: should normalize deeply nested structures correctly", func() {
		specA := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{"z": "last", "a": "first"},
						map[string]interface{}{"m": "middle"},
					},
				},
			},
		}
		specB := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{"m": "middle"},
						map[string]interface{}{"a": "first", "z": "last"},
					},
				},
			},
		}

		hashA, errA := hash.CanonicalSpecHash(specA)
		hashB, errB := hash.CanonicalSpecHash(specB)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errB).ToNot(HaveOccurred())
		Expect(hashA).To(Equal(hashB))
	})

	// UT-HASH-004: Real K8s Deployment spec
	It("UT-HASH-004: should produce stable hash for a real Deployment spec", func() {
		spec := map[string]interface{}{
			"replicas": float64(3),
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "web-server",
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "web-server",
					},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:1.21",
							"ports": []interface{}{
								map[string]interface{}{"containerPort": float64(80)},
							},
							"resources": map[string]interface{}{
								"limits": map[string]interface{}{
									"cpu":    "500m",
									"memory": "128Mi",
								},
							},
						},
					},
				},
			},
		}

		h, err := hash.CanonicalSpecHash(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(h).To(HavePrefix("sha256:"), "Hash must have sha256: prefix")
		Expect(h).To(HaveLen(71), "sha256: (7) + 64 hex chars = 71")
	})

	// UT-HASH-005: Real K8s Pod spec with reordered containers
	It("UT-HASH-005: should produce identical hash for Pod spec with containers in different order", func() {
		specA := map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{"name": "sidecar", "image": "envoy:1.0"},
				map[string]interface{}{"name": "app", "image": "myapp:2.0"},
			},
		}
		specB := map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{"name": "app", "image": "myapp:2.0"},
				map[string]interface{}{"name": "sidecar", "image": "envoy:1.0"},
			},
		}

		hashA, errA := hash.CanonicalSpecHash(specA)
		hashB, errB := hash.CanonicalSpecHash(specB)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errB).ToNot(HaveOccurred())
		Expect(hashA).To(Equal(hashB))
	})

	// UT-HASH-006: Empty spec, nil spec, empty map
	It("UT-HASH-006: should handle edge cases gracefully", func() {
		// Empty map
		h1, err1 := hash.CanonicalSpecHash(map[string]interface{}{})
		Expect(err1).ToNot(HaveOccurred())
		Expect(h1).To(HavePrefix("sha256:"))

		// Nil map
		h2, err2 := hash.CanonicalSpecHash(nil)
		Expect(err2).ToNot(HaveOccurred())
		Expect(h2).To(HavePrefix("sha256:"))

		// Empty and nil should produce the same hash (both are empty maps)
		Expect(h1).To(Equal(h2), "Empty map and nil should produce identical hash")
	})

	// UT-HASH-007: Float precision (replicas: 3 as float64)
	It("UT-HASH-007: should handle JSON number representation stably", func() {
		// When K8s unstructured returns numbers, they are always float64
		spec := map[string]interface{}{
			"replicas": float64(3),
			"minReady": float64(0),
			"ratio":    float64(0.75),
		}

		h1, err1 := hash.CanonicalSpecHash(spec)
		h2, err2 := hash.CanonicalSpecHash(spec)

		Expect(err1).ToNot(HaveOccurred())
		Expect(err2).ToNot(HaveOccurred())
		Expect(h1).To(Equal(h2), "Same float64 values should produce identical hash")
	})

	// UT-HASH-008: Unicode string handling
	It("UT-HASH-008: should handle non-ASCII characters correctly", func() {
		spec := map[string]interface{}{
			"description": "日本語テスト",
			"label":       "café résumé",
			"emoji":       "🚀",
		}

		h, err := hash.CanonicalSpecHash(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(h).To(HavePrefix("sha256:"))
		Expect(h).To(HaveLen(71))
	})

	// UT-HASH-009: Large spec (10KB+)
	It("UT-HASH-009: should handle large specs correctly", func() {
		// Build a spec with many entries to exceed 10KB
		containers := make([]interface{}, 50)
		for i := 0; i < 50; i++ {
			containers[i] = map[string]interface{}{
				"name":  strings.Repeat("container-name-padding-", 5) + string(rune('a'+i%26)),
				"image": strings.Repeat("registry.example.com/very/long/image/path/", 3) + "app:latest",
				"env": []interface{}{
					map[string]interface{}{"name": "VAR1", "value": strings.Repeat("x", 200)},
					map[string]interface{}{"name": "VAR2", "value": strings.Repeat("y", 200)},
				},
			}
		}
		spec := map[string]interface{}{
			"replicas":   float64(3),
			"containers": containers,
		}

		h, err := hash.CanonicalSpecHash(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(h).To(HavePrefix("sha256:"))
		Expect(h).To(HaveLen(71))
	})

	// UT-HASH-010: Idempotency (1000 iterations)
	It("UT-HASH-010: should produce identical hash across 1000 iterations", func() {
		spec := map[string]interface{}{
			"replicas": float64(2),
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "test"},
			},
		}

		first, err := hash.CanonicalSpecHash(spec)
		Expect(err).ToNot(HaveOccurred())

		for i := 0; i < 999; i++ {
			h, err := hash.CanonicalSpecHash(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(h).To(Equal(first), "Iteration %d produced different hash", i+1)
		}
	})

	// UT-HASH-011: Nested slices of maps (containers[].volumeMounts[])
	It("UT-HASH-011: should normalize multi-level slice nesting", func() {
		specA := map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name": "app",
					"volumeMounts": []interface{}{
						map[string]interface{}{"name": "data", "mountPath": "/data"},
						map[string]interface{}{"name": "config", "mountPath": "/config"},
					},
				},
			},
		}
		specB := map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name": "app",
					"volumeMounts": []interface{}{
						map[string]interface{}{"mountPath": "/config", "name": "config"},
						map[string]interface{}{"mountPath": "/data", "name": "data"},
					},
				},
			},
		}

		hashA, errA := hash.CanonicalSpecHash(specA)
		hashB, errB := hash.CanonicalSpecHash(specB)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errB).ToNot(HaveOccurred())
		Expect(hashA).To(Equal(hashB))
	})

	// UT-HASH-012: Mixed types in slices
	It("UT-HASH-012: should sort heterogeneous slices correctly", func() {
		specA := map[string]interface{}{
			"mixed": []interface{}{
				"string_value",
				float64(42),
				true,
				nil,
				map[string]interface{}{"key": "value"},
			},
		}
		specB := map[string]interface{}{
			"mixed": []interface{}{
				map[string]interface{}{"key": "value"},
				nil,
				true,
				float64(42),
				"string_value",
			},
		}

		hashA, errA := hash.CanonicalSpecHash(specA)
		hashB, errB := hash.CanonicalSpecHash(specB)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errB).ToNot(HaveOccurred())
		Expect(hashA).To(Equal(hashB), "Same mixed-type slice in different order should hash identically")
	})

	// Additional: Different content must produce different hash
	It("should produce different hashes for different content", func() {
		specA := map[string]interface{}{"replicas": float64(1)}
		specB := map[string]interface{}{"replicas": float64(2)}

		hashA, errA := hash.CanonicalSpecHash(specA)
		hashB, errB := hash.CanonicalSpecHash(specB)

		Expect(errA).ToNot(HaveOccurred())
		Expect(errB).ToNot(HaveOccurred())
		Expect(hashA).ToNot(Equal(hashB), "Different content must produce different hashes")
	})

	// Hash format verification (DD-EM-002 Hash Format section)
	It("should return hash in sha256:<64-hex> format (71 chars total)", func() {
		spec := map[string]interface{}{"key": "value"}

		h, err := hash.CanonicalSpecHash(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(h).To(HavePrefix("sha256:"))
		Expect(h).To(HaveLen(71))

		// Verify hex characters after prefix
		hexPart := h[7:]
		Expect(hexPart).To(MatchRegexp("^[0-9a-f]{64}$"), "Hash should be lowercase hex")
	})
})
