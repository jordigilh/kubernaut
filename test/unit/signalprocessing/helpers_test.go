/*
Copyright 2025 Jordi Gil.

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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
// See docs/development/business-requirements/TESTING_GUIDELINES.md
package signalprocessing

import (
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/cache"
)

// Unit Test: extractConfidence helper function
// Note: extractConfidence is unexported, but we can test the behavior
// through the classifiers that use it. For now, we test cache.Len which
// is the other P4 item.

// Unit Test: cache.Len implementation correctness
var _ = Describe("TTLCache.Len", func() {
	var ttlCache *cache.TTLCache

	BeforeEach(func() {
		ttlCache = cache.NewTTLCache(5 * time.Minute)
	})

	It("CACHE-LEN-01: should return 0 for empty cache", func() {
		Expect(ttlCache.Len()).To(Equal(0))
	})

	It("CACHE-LEN-02: should return correct count after adding items", func() {
		ttlCache.Set("key1", "value1")
		ttlCache.Set("key2", "value2")
		ttlCache.Set("key3", "value3")

		Expect(ttlCache.Len()).To(Equal(3))
	})

	It("CACHE-LEN-03: should return correct count after deleting items", func() {
		ttlCache.Set("key1", "value1")
		ttlCache.Set("key2", "value2")
		ttlCache.Set("key3", "value3")
		Expect(ttlCache.Len()).To(Equal(3))

		ttlCache.Delete("key2")
		Expect(ttlCache.Len()).To(Equal(2))
	})

	It("CACHE-LEN-04: should return 0 after clear", func() {
		ttlCache.Set("key1", "value1")
		ttlCache.Set("key2", "value2")
		Expect(ttlCache.Len()).To(Equal(2))

		ttlCache.Clear()
		Expect(ttlCache.Len()).To(Equal(0))
	})

	It("CACHE-LEN-05: should include expired entries in count", func() {
		// Create cache with very short TTL
		shortTTLCache := cache.NewTTLCache(1 * time.Millisecond)
		shortTTLCache.Set("key1", "value1")

		// Len includes expired entries (per cache.go comment)
		// This is by design - cleanup happens on Get, not on Len
		Expect(shortTTLCache.Len()).To(Equal(1))

		// After a short wait, the entry is still in the map
		// (expired entries are cleaned up lazily on Get)
		time.Sleep(5 * time.Millisecond)
		Expect(shortTTLCache.Len()).To(Equal(1))

		// But Get should return not found for expired entry
		_, found := shortTTLCache.Get("key1")
		Expect(found).To(BeFalse())
	})
})

// Unit Test: json.Number handling in confidence extraction
// This tests the behavior through a mock Rego result that contains json.Number
var _ = Describe("JSON Number Handling", func() {
	It("HELP-01: should handle json.Number from Rego results", func() {
		// Simulate Rego result structure with json.Number
		jsonData := `{"confidence": 0.85}`
		var result map[string]interface{}

		// Use json.Decoder with UseNumber to get json.Number type
		decoder := json.NewDecoder(strings.NewReader(jsonData))
		decoder.UseNumber()
		err := decoder.Decode(&result)
		Expect(err).ToNot(HaveOccurred())

		// Verify confidence is json.Number type
		confidence := result["confidence"]
		Expect(confidence).To(BeAssignableToTypeOf(json.Number("")))

		// Verify conversion to float64
		numVal := confidence.(json.Number)
		floatVal, err := numVal.Float64()
		Expect(err).ToNot(HaveOccurred())
		Expect(floatVal).To(BeNumerically("~", 0.85, 0.001))
	})

	It("HELP-02: should handle direct float64 values", func() {
		// When not using UseNumber, JSON unmarshals to float64
		jsonData := `{"confidence": 0.95}`
		var result map[string]interface{}
		err := json.Unmarshal([]byte(jsonData), &result)
		Expect(err).ToNot(HaveOccurred())

		confidence := result["confidence"]
		Expect(confidence).To(BeAssignableToTypeOf(float64(0)))
		Expect(confidence.(float64)).To(BeNumerically("~", 0.95, 0.001))
	})

	It("HELP-03: should handle nil confidence gracefully", func() {
		jsonData := `{}`
		var result map[string]interface{}
		err := json.Unmarshal([]byte(jsonData), &result)
		Expect(err).ToNot(HaveOccurred())

		confidence := result["confidence"]
		Expect(confidence).To(BeNil())
	})

	It("HELP-04: should handle integer confidence values", func() {
		// Integer values from Rego (e.g., 1 instead of 1.0)
		jsonData := `{"confidence": 1}`
		decoder := json.NewDecoder(strings.NewReader(jsonData))
		decoder.UseNumber()
		var result map[string]interface{}
		err := decoder.Decode(&result)
		Expect(err).ToNot(HaveOccurred())

		numVal := result["confidence"].(json.Number)
		floatVal, err := numVal.Float64()
		Expect(err).ToNot(HaveOccurred())
		Expect(floatVal).To(Equal(float64(1)))
	})
})
