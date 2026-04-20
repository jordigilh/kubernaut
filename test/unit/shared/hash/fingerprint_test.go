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

var _ = Describe("CanonicalResourceFingerprint (#765)", func() {

	It("UT-HASH-765-001: should strip metadata/status/apiVersion/kind from hash input", func() {
		objWithMeta := map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "web",
				"namespace": "default",
				"uid":       "abc-123",
			},
			"status": map[string]interface{}{
				"readyReplicas": float64(3),
			},
			"spec": map[string]interface{}{
				"replicas": float64(3),
			},
		}

		objWithoutMeta := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": float64(3),
			},
		}

		h1, err1 := hash.CanonicalResourceFingerprint(objWithMeta)
		h2, err2 := hash.CanonicalResourceFingerprint(objWithoutMeta)

		Expect(err1).NotTo(HaveOccurred())
		Expect(err2).NotTo(HaveOccurred())
		Expect(h1).To(Equal(h2), "metadata/status/apiVersion/kind must be stripped before hashing")
	})

	It("UT-HASH-765-002: should produce stable hash for Deployment object", func() {
		obj := map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "web", "namespace": "default"},
			"spec": map[string]interface{}{
				"replicas": float64(3),
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{"app": "web"},
				},
			},
		}

		h, err := hash.CanonicalResourceFingerprint(obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(h).To(HavePrefix("sha256:"))
		Expect(h).To(HaveLen(71))
	})

	It("UT-HASH-765-003: should capture ConfigMap .data/.binaryData in fingerprint", func() {
		cmObj := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "app-config", "namespace": "default"},
			"data": map[string]interface{}{
				"config.yaml": "server:\n  port: 8080",
			},
		}

		h, err := hash.CanonicalResourceFingerprint(cmObj)
		Expect(err).NotTo(HaveOccurred())
		Expect(h).NotTo(BeEmpty())

		cmObjChanged := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "app-config", "namespace": "default"},
			"data": map[string]interface{}{
				"config.yaml": "server:\n  port: 9090",
			},
		}

		hChanged, errChanged := hash.CanonicalResourceFingerprint(cmObjChanged)
		Expect(errChanged).NotTo(HaveOccurred())
		Expect(hChanged).NotTo(Equal(h), "different ConfigMap data must produce different fingerprint")
	})

	It("UT-HASH-765-004: should capture ClusterRole .rules in fingerprint", func() {
		crObj := map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata":   map[string]interface{}{"name": "admin"},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{""},
					"resources": []interface{}{"pods"},
					"verbs":     []interface{}{"get", "list", "watch"},
				},
			},
		}

		h, err := hash.CanonicalResourceFingerprint(crObj)
		Expect(err).NotTo(HaveOccurred())
		Expect(h).NotTo(BeEmpty())

		crObjDifferent := map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata":   map[string]interface{}{"name": "admin"},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{""},
					"resources": []interface{}{"pods"},
					"verbs":     []interface{}{"get", "list", "watch", "delete"},
				},
			},
		}

		hDiff, errDiff := hash.CanonicalResourceFingerprint(crObjDifferent)
		Expect(errDiff).NotTo(HaveOccurred())
		Expect(hDiff).NotTo(Equal(h), "different ClusterRole rules must produce different fingerprint")
	})

	It("UT-HASH-765-005: should be idempotent (1000 iterations)", func() {
		obj := map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "test"},
			"spec": map[string]interface{}{
				"replicas": float64(2),
			},
		}

		first, err := hash.CanonicalResourceFingerprint(obj)
		Expect(err).NotTo(HaveOccurred())

		for i := 0; i < 999; i++ {
			h, e := hash.CanonicalResourceFingerprint(obj)
			Expect(e).NotTo(HaveOccurred())
			Expect(h).To(Equal(first), "iteration %d produced different hash", i+1)
		}
	})

	It("UT-HASH-765-006: should be map-order and slice-order independent", func() {
		objA := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "a"},
			"data": map[string]interface{}{
				"b": "2",
				"a": "1",
			},
		}

		objB := map[string]interface{}{
			"kind":       "ConfigMap",
			"apiVersion": "v1",
			"metadata":   map[string]interface{}{"name": "a"},
			"data": map[string]interface{}{
				"a": "1",
				"b": "2",
			},
		}

		hA, errA := hash.CanonicalResourceFingerprint(objA)
		hB, errB := hash.CanonicalResourceFingerprint(objB)

		Expect(errA).NotTo(HaveOccurred())
		Expect(errB).NotTo(HaveOccurred())
		Expect(hA).To(Equal(hB))
	})
})

var _ = Describe("CompositeResourceFingerprint (#765)", func() {

	It("UT-HASH-765-007: should return fingerprint unchanged when no ConfigMap hashes", func() {
		fingerprint := "sha256:abc123def456789012345678901234567890123456789012345678901234"

		composite, err := hash.CompositeResourceFingerprint(fingerprint, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(composite).To(Equal(fingerprint))
	})

	It("UT-HASH-765-008: should include ConfigMap content in composite digest", func() {
		fingerprint := "sha256:abc123def456789012345678901234567890123456789012345678901234"
		cmHashes := map[string]string{
			"app-config": "sha256:111111111111111111111111111111111111111111111111111111111111",
		}

		composite, err := hash.CompositeResourceFingerprint(fingerprint, cmHashes)
		Expect(err).NotTo(HaveOccurred())
		Expect(composite).NotTo(Equal(fingerprint), "ConfigMap hash must change the composite")
		Expect(composite).To(HavePrefix("sha256:"))
		Expect(composite).To(HaveLen(71))
	})
})
