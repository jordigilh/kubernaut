# Gateway Unit Test Plan v1.0

**Service**: Gateway
**Version**: v1.0
**Date**: January 17, 2026
**Owner**: Gateway Team
**Status**: 📋 **PLANNED** | P0 Security & Resilience Gaps

---

## 🎯 **Executive Summary**

### **Objective**
Address 3 critical P0 business requirements with no test coverage through focused unit tests for security and resilience features.

**Reference**: `GW_TEST_COVERAGE_GAP_ANALYSIS_JAN17_2026.md`

### **Scope**
- **BR-042**: Log Sanitization (P0 Critical - Security)
- **BR-107**: Memory Management (P0 Critical - Resilience)
- **BR-108**: Goroutine Management (P0 Critical - Resilience)

### **Current State**
| BR | Requirement | Priority | Current Coverage | Gap Impact |
|----|-------------|----------|------------------|------------|
| **BR-042** | Log Sanitization | P0 | ❌ No tests | **HIGH** - PII/secret leakage risk |
| **BR-107** | Memory Management | P0 | ❌ No tests | **HIGH** - Memory leak crash risk |
| **BR-108** | Goroutine Management | P0 | ❌ No tests | **HIGH** - Goroutine leak resource exhaustion |

### **Plan Summary**
| Category | Tests | Effort | Priority |
|----------|-------|--------|----------|
| **Log Sanitization** | 8 tests | 2-4h | 🔴 **IMMEDIATE** (Security) |
| **Memory Management** | 5 tests | 4-6h | 🟠 **HIGH** (Reliability) |
| **Goroutine Management** | 6 tests | 3-5h | 🟠 **HIGH** (Reliability) |
| **TOTAL** | **19 tests** | **9-15h** | 🔴 **1-2 Sprints** |

### **Why Unit Tests?**
**Per TESTING_GUIDELINES.md**:
- ✅ **BR-042**: Pure function validation, no external dependencies
- ✅ **BR-107**: Memory profiling, no infrastructure required
- ✅ **BR-108**: Goroutine tracking, no infrastructure required
- ✅ **Fast Execution**: <100ms per test (unit tier requirement)
- ✅ **No Infrastructure**: No K8s, PostgreSQL, Redis needed

---

## 📋 **Test Case Registry**

### Quick Reference Table

| Test ID | Test Name | Category | BR | Priority | Status | Section |
|---------|-----------|----------|-----|----------|--------|---------|
| **Log Sanitization (BR-042)** |
| GW-UNIT-SEC-001 | Redact Email Addresses | Security | 042 | P0 | ⏳ Planned | 1.1.1 |
| GW-UNIT-SEC-002 | Redact IP Addresses | Security | 042 | P0 | ⏳ Planned | 1.1.2 |
| GW-UNIT-SEC-003 | Redact API Keys/Tokens | Security | 042 | P0 | ⏳ Planned | 1.1.3 |
| GW-UNIT-SEC-004 | Redact Passwords | Security | 042 | P0 | ⏳ Planned | 1.1.4 |
| GW-UNIT-SEC-005 | Redact Credit Card Numbers | Security | 042 | P0 | ⏳ Planned | 1.1.5 |
| GW-UNIT-SEC-006 | Preserve Non-PII Data | Security | 042 | P0 | ⏳ Planned | 1.1.6 |
| GW-UNIT-SEC-007 | Multiple PII in One Message | Security | 042 | P0 | ⏳ Planned | 1.1.7 |
| GW-UNIT-SEC-008 | Unicode/Special Characters | Security | 042 | P1 | ⏳ Planned | 1.1.8 |
| **Memory Management (BR-107)** |
| GW-UNIT-RES-001 | No Memory Leak on Startup | Resilience | 107 | P0 | ⏳ Planned | 2.1.1 |
| GW-UNIT-RES-002 | No Memory Leak on Signal Processing | Resilience | 107 | P0 | ⏳ Planned | 2.1.2 |
| GW-UNIT-RES-003 | Memory Cleanup on Shutdown | Resilience | 107 | P0 | ⏳ Planned | 2.1.3 |
| GW-UNIT-RES-004 | Memory Usage Under Load | Resilience | 107 | P0 | ⏳ Planned | 2.1.4 |
| GW-UNIT-RES-005 | Memory Growth Bounds | Resilience | 107 | P1 | ⏳ Planned | 2.1.5 |
| **Goroutine Management (BR-108)** |
| GW-UNIT-RES-006 | No Goroutine Leak on Startup | Resilience | 108 | P0 | ⏳ Planned | 3.1.1 |
| GW-UNIT-RES-007 | No Goroutine Leak on Processing | Resilience | 108 | P0 | ⏳ Planned | 3.1.2 |
| GW-UNIT-RES-008 | Goroutine Cleanup on Shutdown | Resilience | 108 | P0 | ⏳ Planned | 3.1.3 |
| GW-UNIT-RES-009 | Goroutine Pool Management | Resilience | 108 | P0 | ⏳ Planned | 3.1.4 |
| GW-UNIT-RES-010 | Goroutine Leak Detection | Resilience | 108 | P1 | ⏳ Planned | 3.1.5 |
| GW-UNIT-RES-011 | Goroutine Count Bounds | Resilience | 108 | P1 | ⏳ Planned | 3.1.6 |

### Category Summary

| Category | Count | Test ID Range | Priority Distribution | Implementation Priority |
|----------|-------|---------------|----------------------|------------------------|
| **SEC** (Security - Log Sanitization) | 8 | GW-UNIT-SEC-001 to 008 | P0: 7, P1: 1 | 🔴 **IMMEDIATE** |
| **RES** (Resilience - Memory/Goroutines) | 11 | GW-UNIT-RES-001 to 011 | P0: 9, P1: 2 | 🟠 **HIGH** |
| **TOTAL** | **19** | - | **P0: 16, P1: 3** | - |

---

## 🔐 **CATEGORY 1: Log Sanitization (BR-042)**

### **Objective**: Ensure PII and secrets never appear in logs

**Priority**: 🔴 **P0 IMMEDIATE** (Security Risk)
**Effort**: 2-4 hours
**Impact**: HIGH - Prevents sensitive data leakage

---

### **Scenario 1.1: Log Message Sanitization**
**BR**: BR-GATEWAY-042
**Priority**: P0 (Critical)
**Business Value**: Prevent PII/secret exposure in logs (SOC2/GDPR compliance)

**Test Specifications**:

```go
package logging_test

import (
	"testing"

	"github.com/jordigilh/kubernaut/pkg/gateway/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Logging Unit Test Suite")
}

var _ = Describe("BR-GATEWAY-042: Log Sanitization", func() {
	var sanitizer *logging.Sanitizer

	BeforeEach(func() {
		sanitizer = logging.NewSanitizer()
	})

	// Test 1.1.1: GW-UNIT-SEC-001
	It("should redact email addresses from log messages", func() {
		// Given: Log message containing email
		input := "User john.doe@example.com attempted login from 192.168.1.100"

		// When: Message sanitized
		output := sanitizer.Sanitize(input)

		// Then: Email redacted
		Expect(output).ToNot(ContainSubstring("john.doe@example.com"),
			"BR-GATEWAY-042: Email must be redacted")
		Expect(output).To(ContainSubstring("[EMAIL_REDACTED]"),
			"BR-GATEWAY-042: Redaction marker must be present")
		Expect(output).To(ContainSubstring("192.168.1.100"),
			"BR-GATEWAY-042: Non-PII data must be preserved")

		GinkgoWriter.Printf("✅ Sanitized: %s\n", output)
	})

	// Test 1.1.2: GW-UNIT-SEC-002
	It("should redact IP addresses from log messages", func() {
		// Given: Log message containing IP addresses
		testCases := []struct {
			input    string
			redacted string
		}{
			{
				input:    "Request from 192.168.1.100",
				redacted: "192.168.1.100",
			},
			{
				input:    "Connection to 10.0.0.5:8080",
				redacted: "10.0.0.5",
			},
			{
				input:    "IPv6 address 2001:0db8:85a3::8a2e:0370:7334",
				redacted: "2001:0db8:85a3::8a2e:0370:7334",
			},
		}

		for _, tc := range testCases {
			// When: Message sanitized
			output := sanitizer.Sanitize(tc.input)

			// Then: IP address redacted
			Expect(output).ToNot(ContainSubstring(tc.redacted),
				"BR-GATEWAY-042: IP address must be redacted")
			Expect(output).To(ContainSubstring("[IP_REDACTED]"),
				"BR-GATEWAY-042: Redaction marker must be present")

			GinkgoWriter.Printf("✅ Sanitized IP: %s → %s\n", tc.input, output)
		}
	})

	// Test 1.1.3: GW-UNIT-SEC-003
	It("should redact API keys and tokens from log messages", func() {
		// Given: Log messages containing API keys/tokens
		testCases := []struct {
			name     string
			input    string
			secret   string
		}{
			{
				name:   "Bearer token",
				input:  "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
				secret: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			},
			{
				name:   "API key",
				input:  "API Key: sk_test_FAKE_KEY_FOR_TESTING_ONLY",
				secret: "sk_test_FAKE_KEY_FOR_TESTING_ONLY",
			},
			{
				name:   "Generic token",
				input:  "token=abc123def456ghi789",
				secret: "abc123def456ghi789",
			},
		}

		for _, tc := range testCases {
			// When: Message sanitized
			output := sanitizer.Sanitize(tc.input)

			// Then: Secret redacted
			Expect(output).ToNot(ContainSubstring(tc.secret),
				"BR-GATEWAY-042: %s must be redacted", tc.name)
			Expect(output).To(ContainSubstring("[TOKEN_REDACTED]"),
				"BR-GATEWAY-042: Redaction marker must be present")

			GinkgoWriter.Printf("✅ Sanitized %s: %s → %s\n", tc.name, tc.input, output)
		}
	})

	// Test 1.1.4: GW-UNIT-SEC-004
	It("should redact passwords from log messages", func() {
		// Given: Log messages containing passwords
		testCases := []string{
			"password=SuperSecret123!",
			"pwd: MyP@ssw0rd",
			"user_password='complex_P@ss123'",
			"redis_password: \"redis-secret-456\"",
		}

		for _, input := range testCases {
			// When: Message sanitized
			output := sanitizer.Sanitize(input)

			// Then: Password redacted
			Expect(output).To(ContainSubstring("[PASSWORD_REDACTED]"),
				"BR-GATEWAY-042: Password must be redacted")
			Expect(output).ToNot(MatchRegexp(`(SuperSecret|MyP@ssw0rd|complex_P@ss|redis-secret)`),
				"BR-GATEWAY-042: Actual password value must not appear")

			GinkgoWriter.Printf("✅ Sanitized password: %s → %s\n", input, output)
		}
	})

	// Test 1.1.5: GW-UNIT-SEC-005
	It("should redact credit card numbers from log messages", func() {
		// Given: Log messages containing credit card numbers
		testCases := []struct {
			name   string
			input  string
			ccNum  string
		}{
			{
				name:   "Visa",
				input:  "Payment with card 4532-1234-5678-9010",
				ccNum:  "4532-1234-5678-9010",
			},
			{
				name:   "Mastercard",
				input:  "CC: 5425233430109903",
				ccNum:  "5425233430109903",
			},
			{
				name:   "Amex",
				input:  "Amex card: 374245455400126",
				ccNum:  "374245455400126",
			},
		}

		for _, tc := range testCases {
			// When: Message sanitized
			output := sanitizer.Sanitize(tc.input)

			// Then: Credit card redacted
			Expect(output).ToNot(ContainSubstring(tc.ccNum),
				"BR-GATEWAY-042: Credit card number must be redacted")
			Expect(output).To(ContainSubstring("[CC_REDACTED]"),
				"BR-GATEWAY-042: Redaction marker must be present")

			GinkgoWriter.Printf("✅ Sanitized %s: %s → %s\n", tc.name, tc.input, output)
		}
	})

	// Test 1.1.6: GW-UNIT-SEC-006
	It("should preserve non-PII data while redacting PII", func() {
		// Given: Log message with mix of PII and non-PII
		input := "User admin@example.com from IP 10.0.0.1 processed alert HighMemoryUsage " +
			"in namespace production with severity critical at 2026-01-17T10:30:00Z"

		// When: Message sanitized
		output := sanitizer.Sanitize(input)

		// Then: PII redacted, business data preserved
		Expect(output).To(ContainSubstring("HighMemoryUsage"),
			"BR-GATEWAY-042: Alert name must be preserved")
		Expect(output).To(ContainSubstring("production"),
			"BR-GATEWAY-042: Namespace must be preserved")
		Expect(output).To(ContainSubstring("critical"),
			"BR-GATEWAY-042: Severity must be preserved")
		Expect(output).To(ContainSubstring("2026-01-17T10:30:00Z"),
			"BR-GATEWAY-042: Timestamp must be preserved")
		Expect(output).ToNot(ContainSubstring("admin@example.com"),
			"BR-GATEWAY-042: Email must be redacted")
		Expect(output).ToNot(ContainSubstring("10.0.0.1"),
			"BR-GATEWAY-042: IP must be redacted")

		GinkgoWriter.Printf("✅ Mixed content sanitized correctly\n")
	})

	// Test 1.1.7: GW-UNIT-SEC-007
	It("should handle multiple PII instances in single message", func() {
		// Given: Log message with multiple PII types
		input := "User john@example.com with token sk_live_abc123 from IP 192.168.1.1 " +
			"and backup email jane@example.com accessed system"

		// When: Message sanitized
		output := sanitizer.Sanitize(input)

		// Then: All PII redacted
		Expect(output).ToNot(ContainSubstring("john@example.com"),
			"BR-GATEWAY-042: First email must be redacted")
		Expect(output).ToNot(ContainSubstring("jane@example.com"),
			"BR-GATEWAY-042: Second email must be redacted")
		Expect(output).ToNot(ContainSubstring("sk_live_abc123"),
			"BR-GATEWAY-042: Token must be redacted")
		Expect(output).ToNot(ContainSubstring("192.168.1.1"),
			"BR-GATEWAY-042: IP must be redacted")

		// Count redaction markers
		emailRedactions := strings.Count(output, "[EMAIL_REDACTED]")
		tokenRedactions := strings.Count(output, "[TOKEN_REDACTED]")
		ipRedactions := strings.Count(output, "[IP_REDACTED]")

		Expect(emailRedactions).To(Equal(2), "BR-GATEWAY-042: Both emails must be redacted")
		Expect(tokenRedactions).To(Equal(1), "BR-GATEWAY-042: Token must be redacted")
		Expect(ipRedactions).To(Equal(1), "BR-GATEWAY-042: IP must be redacted")

		GinkgoWriter.Printf("✅ Multiple PII redacted: %d emails, %d tokens, %d IPs\n",
			emailRedactions, tokenRedactions, ipRedactions)
	})

	// Test 1.1.8: GW-UNIT-SEC-008
	It("should handle unicode and special characters in PII", func() {
		// Given: Log messages with unicode/special characters
		testCases := []struct {
			name  string
			input string
		}{
			{
				name:  "Unicode email",
				input: "User françois@exämple.com logged in",
			},
			{
				name:  "URL encoded",
				input: "Email: user%40example.com",
			},
			{
				name:  "JSON escaped",
				input: `{"email":"user@example.com","token":"abc123"}`,
			},
		}

		for _, tc := range testCases {
			// When: Message sanitized
			output := sanitizer.Sanitize(tc.input)

			// Then: PII patterns detected and redacted
			Expect(output).To(ContainSubstring("REDACTED"),
				"BR-GATEWAY-042: %s PII must be detected and redacted", tc.name)

			GinkgoWriter.Printf("✅ Sanitized %s: %s → %s\n", tc.name, tc.input, output)
		}
	})
})
```

**Implementation Required**:
- `pkg/gateway/logging/sanitizer.go` - Sanitization business logic
- `pkg/gateway/logging/sanitizer_test.go` - Unit tests (this file)
- `pkg/gateway/logging/patterns.go` - Regex patterns for PII detection

**Timeline**: 2-4 hours

---

## 💾 **CATEGORY 2: Memory Management (BR-107)**

### **Objective**: Detect and prevent memory leaks

**Priority**: 🟠 **P0 HIGH** (Reliability Risk)
**Effort**: 4-6 hours
**Impact**: HIGH - Prevents memory leak crashes

---

### **Scenario 2.1: Memory Leak Detection**
**BR**: BR-GATEWAY-107
**Priority**: P0 (Critical)
**Business Value**: Prevent memory exhaustion leading to pod crashes

**Test Specifications**:

```go
package memory_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/gateway/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Memory Management Unit Test Suite")
}

var _ = Describe("BR-GATEWAY-107: Memory Management", func() {
	// Helper to get current memory stats
	getMemStats := func() runtime.MemStats {
		runtime.GC() // Force garbage collection
		time.Sleep(10 * time.Millisecond) // Let GC finish
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m
	}

	// Test 2.1.1: GW-UNIT-RES-001
	It("should not leak memory on repeated server initialization", func() {
		// Given: Initial memory baseline
		initialStats := getMemStats()
		initialAlloc := initialStats.Alloc

		// When: Create and destroy server multiple times
		iterations := 100
		for i := 0; i < iterations; i++ {
			srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
			Expect(err).ToNot(HaveOccurred())

			// Simulate work
			_ = srv

			// Explicit cleanup
			srv = nil
		}

		// Force garbage collection
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		// Then: Memory should return to baseline (within tolerance)
		finalStats := getMemStats()
		finalAlloc := finalStats.Alloc
		memoryGrowth := finalAlloc - initialAlloc

		// Allow 10% growth tolerance (for GC overhead)
		maxAllowedGrowth := initialAlloc / 10
		Expect(memoryGrowth).To(BeNumerically("<", maxAllowedGrowth),
			"BR-GATEWAY-107: Memory leak detected on server initialization "+
				"(grew by %d bytes, max allowed: %d bytes)",
			memoryGrowth, maxAllowedGrowth)

		GinkgoWriter.Printf("✅ Memory stable after %d iterations: initial=%d, final=%d, growth=%d bytes (%.2f%%)\n",
			iterations, initialAlloc, finalAlloc, memoryGrowth,
			float64(memoryGrowth)/float64(initialAlloc)*100)
	})

	// Test 2.1.2: GW-UNIT-RES-002
	It("should not leak memory during signal processing", func() {
		// Given: Server and initial memory baseline
		srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		initialStats := getMemStats()
		initialAlloc := initialStats.Alloc

		// When: Process many signals
		ctx := context.Background()
		iterations := 1000

		for i := 0; i < iterations; i++ {
			signal := createTestPrometheusAlert(PrometheusAlertOptions{
				AlertName: "HighMemoryUsage",
				Namespace: "production",
				Severity:  "critical",
			})

			_, err := srv.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
		}

		// Force garbage collection
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		// Then: Memory growth bounded
		finalStats := getMemStats()
		finalAlloc := finalStats.Alloc
		memoryGrowth := finalAlloc - initialAlloc

		// Allow 5MB growth for 1000 signals (5KB per signal average)
		maxAllowedGrowth := uint64(5 * 1024 * 1024)
		Expect(memoryGrowth).To(BeNumerically("<", maxAllowedGrowth),
			"BR-GATEWAY-107: Memory leak detected during signal processing "+
				"(grew by %d bytes, max allowed: %d bytes)",
			memoryGrowth, maxAllowedGrowth)

		GinkgoWriter.Printf("✅ Memory stable after %d signals: growth=%d bytes (%.2f MB)\n",
			iterations, memoryGrowth, float64(memoryGrowth)/(1024*1024))
	})

	// Test 2.1.3: GW-UNIT-RES-003
	It("should clean up memory on server shutdown", func() {
		// Given: Server running with active operations
		srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		for i := 0; i < 50; i++ {
			signal := createTestPrometheusAlert(PrometheusAlertOptions{
				AlertName: "HighMemoryUsage",
				Namespace: "production",
			})
			_, _ = srv.ProcessSignal(ctx, signal)
		}

		beforeShutdown := getMemStats()

		// When: Server shutdown
		err = srv.Shutdown(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Force garbage collection
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		// Then: Memory released
		afterShutdown := getMemStats()

		// Memory should decrease or stay within 10% of before shutdown
		memoryDelta := int64(afterShutdown.Alloc) - int64(beforeShutdown.Alloc)
		maxAllowedGrowth := int64(beforeShutdown.Alloc / 10)

		Expect(memoryDelta).To(BeNumerically("<", maxAllowedGrowth),
			"BR-GATEWAY-107: Memory not released after shutdown "+
				"(grew by %d bytes, max allowed: %d bytes)",
			memoryDelta, maxAllowedGrowth)

		GinkgoWriter.Printf("✅ Memory cleaned up after shutdown: delta=%d bytes\n", memoryDelta)
	})

	// Test 2.1.4: GW-UNIT-RES-004
	It("should maintain stable memory under sustained load", func() {
		// Given: Server processing signals continuously
		srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		samples := 10
		sampleInterval := 100 * time.Millisecond
		signalsPerSample := 100

		memoryReadings := make([]uint64, samples)

		// When: Process signals and sample memory periodically
		for i := 0; i < samples; i++ {
			for j := 0; j < signalsPerSample; j++ {
				signal := createTestPrometheusAlert(PrometheusAlertOptions{
					AlertName: "HighMemoryUsage",
					Namespace: "production",
				})
				_, _ = srv.ProcessSignal(ctx, signal)
			}

			stats := getMemStats()
			memoryReadings[i] = stats.Alloc
			time.Sleep(sampleInterval)
		}

		// Then: Memory growth should stabilize (not continuously increase)
		// Check last 3 samples are within 10% of each other
		lastThreeSamples := memoryReadings[samples-3:]
		minMem := lastThreeSamples[0]
		maxMem := lastThreeSamples[0]

		for _, mem := range lastThreeSamples {
			if mem < minMem {
				minMem = mem
			}
			if mem > maxMem {
				maxMem = mem
			}
		}

		variance := maxMem - minMem
		allowedVariance := minMem / 10 // 10% tolerance

		Expect(variance).To(BeNumerically("<", allowedVariance),
			"BR-GATEWAY-107: Memory not stable under load "+
				"(variance: %d bytes, max allowed: %d bytes)",
			variance, allowedVariance)

		GinkgoWriter.Printf("✅ Memory stable under load: min=%d, max=%d, variance=%d bytes (%.2f%%)\n",
			minMem, maxMem, variance, float64(variance)/float64(minMem)*100)
	})

	// Test 2.1.5: GW-UNIT-RES-005
	It("should not exceed memory growth bounds over time", func() {
		// Given: Server and baseline memory
		srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		initialStats := getMemStats()
		initialAlloc := initialStats.Alloc

		// When: Process signals over extended period
		ctx := context.Background()
		duration := 5 * time.Second
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		timeout := time.After(duration)
		signalsProcessed := 0

	processLoop:
		for {
			select {
			case <-timeout:
				break processLoop
			case <-ticker.C:
				signal := createTestPrometheusAlert(PrometheusAlertOptions{
					AlertName: "HighMemoryUsage",
					Namespace: "production",
				})
				_, _ = srv.ProcessSignal(ctx, signal)
				signalsProcessed++
			}
		}

		// Force garbage collection
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		// Then: Total memory growth bounded
		finalStats := getMemStats()
		finalAlloc := finalStats.Alloc
		memoryGrowth := finalAlloc - initialAlloc

		// Allow 10MB max growth regardless of signals processed
		maxAllowedGrowth := uint64(10 * 1024 * 1024)
		Expect(memoryGrowth).To(BeNumerically("<", maxAllowedGrowth),
			"BR-GATEWAY-107: Memory growth exceeded bounds "+
				"(grew by %d bytes, max allowed: %d bytes after processing %d signals)",
			memoryGrowth, maxAllowedGrowth, signalsProcessed)

		GinkgoWriter.Printf("✅ Memory growth bounded: processed %d signals, growth=%d bytes (%.2f MB)\n",
			signalsProcessed, memoryGrowth, float64(memoryGrowth)/(1024*1024))
	})
})
```

**Implementation Required**:
- `pkg/gateway/server/server.go` - Ensure proper cleanup in Shutdown()
- `pkg/gateway/server/memory_test.go` - Memory leak tests (this file)
- May require refactoring to ensure proper resource cleanup

**Timeline**: 4-6 hours

---

## 🔄 **CATEGORY 3: Goroutine Management (BR-108)**

### **Objective**: Detect and prevent goroutine leaks

**Priority**: 🟠 **P0 HIGH** (Reliability Risk)
**Effort**: 3-5 hours
**Impact**: HIGH - Prevents resource exhaustion

---

### **Scenario 3.1: Goroutine Leak Detection**
**BR**: BR-GATEWAY-108
**Priority**: P0 (Critical)
**Business Value**: Prevent goroutine exhaustion leading to resource starvation

**Test Specifications**:

```go
package goroutine_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/gateway/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGoroutine(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Goroutine Management Unit Test Suite")
}

var _ = Describe("BR-GATEWAY-108: Goroutine Management", func() {
	// Helper to get current goroutine count
	getGoroutineCount := func() int {
		runtime.GC() // Force garbage collection
		time.Sleep(50 * time.Millisecond) // Let goroutines finish
		return runtime.NumGoroutine()
	}

	// Test 3.1.1: GW-UNIT-RES-006
	It("should not leak goroutines on repeated server initialization", func() {
		// Given: Initial goroutine count
		initialCount := getGoroutineCount()

		// When: Create and destroy server multiple times
		iterations := 50
		for i := 0; i < iterations; i++ {
			srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
			Expect(err).ToNot(HaveOccurred())

			// Simulate work
			ctx, cancel := context.WithCancel(context.Background())
			_ = srv
			cancel() // Cancel context to trigger cleanup

			// Explicit cleanup
			srv = nil
		}

		// Wait for goroutines to finish
		time.Sleep(500 * time.Millisecond)

		// Then: Goroutine count should return to baseline
		finalCount := getGoroutineCount()
		goroutineGrowth := finalCount - initialCount

		// Allow 10 goroutine tolerance (for GC, timers, etc.)
		Expect(goroutineGrowth).To(BeNumerically("<=", 10),
			"BR-GATEWAY-108: Goroutine leak detected on server initialization "+
				"(grew by %d goroutines, max allowed: 10)",
			goroutineGrowth)

		GinkgoWriter.Printf("✅ Goroutines stable after %d iterations: initial=%d, final=%d, growth=%d\n",
			iterations, initialCount, finalCount, goroutineGrowth)
	})

	// Test 3.1.2: GW-UNIT-RES-007
	It("should not leak goroutines during signal processing", func() {
		// Given: Server and initial goroutine count
		srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		initialCount := getGoroutineCount()

		// When: Process many signals
		ctx := context.Background()
		iterations := 500

		for i := 0; i < iterations; i++ {
			signal := createTestPrometheusAlert(PrometheusAlertOptions{
				AlertName: "HighMemoryUsage",
				Namespace: "production",
			})

			_, err := srv.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
		}

		// Wait for goroutines to finish
		time.Sleep(500 * time.Millisecond)

		// Then: Goroutine count bounded
		finalCount := getGoroutineCount()
		goroutineGrowth := finalCount - initialCount

		// Allow 20 goroutine growth (worker pool, async operations)
		Expect(goroutineGrowth).To(BeNumerically("<=", 20),
			"BR-GATEWAY-108: Goroutine leak detected during processing "+
				"(grew by %d goroutines, max allowed: 20)",
			goroutineGrowth)

		GinkgoWriter.Printf("✅ Goroutines stable after %d signals: initial=%d, final=%d, growth=%d\n",
			iterations, initialCount, finalCount, goroutineGrowth)
	})

	// Test 3.1.3: GW-UNIT-RES-008
	It("should clean up goroutines on server shutdown", func() {
		// Given: Server running with active operations
		srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		for i := 0; i < 50; i++ {
			signal := createTestPrometheusAlert(PrometheusAlertOptions{
				AlertName: "HighMemoryUsage",
				Namespace: "production",
			})
			_, _ = srv.ProcessSignal(ctx, signal)
		}

		beforeShutdown := getGoroutineCount()

		// When: Server shutdown
		err = srv.Shutdown(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Wait for goroutines to finish
		time.Sleep(500 * time.Millisecond)

		// Then: Goroutines cleaned up
		afterShutdown := getGoroutineCount()

		// Goroutines should decrease or stay within tolerance
		goroutineDelta := afterShutdown - beforeShutdown

		Expect(goroutineDelta).To(BeNumerically("<=", 5),
			"BR-GATEWAY-108: Goroutines not cleaned up after shutdown "+
				"(delta: %d goroutines, max allowed: 5)",
			goroutineDelta)

		GinkgoWriter.Printf("✅ Goroutines cleaned up: before=%d, after=%d, delta=%d\n",
			beforeShutdown, afterShutdown, goroutineDelta)
	})

	// Test 3.1.4: GW-UNIT-RES-009
	It("should manage goroutine pool with bounded size", func() {
		// Given: Server with worker pool configuration
		configWithPool := *testConfig
		configWithPool.WorkerPoolSize = 10

		srv, err := server.New(&configWithPool, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		initialCount := getGoroutineCount()

		// When: Process many concurrent signals
		ctx := context.Background()
		iterations := 100

		for i := 0; i < iterations; i++ {
			signal := createTestPrometheusAlert(PrometheusAlertOptions{
				AlertName: "HighMemoryUsage",
				Namespace: "production",
			})

			// Process asynchronously
			go srv.ProcessSignal(ctx, signal)
		}

		// Wait for processing to complete
		time.Sleep(1 * time.Second)

		// Then: Goroutine count should not exceed pool size + baseline
		finalCount := getGoroutineCount()
		goroutineGrowth := finalCount - initialCount

		// Pool size (10) + tolerance (10) = max 20 new goroutines
		maxExpectedGrowth := configWithPool.WorkerPoolSize + 10
		Expect(goroutineGrowth).To(BeNumerically("<=", maxExpectedGrowth),
			"BR-GATEWAY-108: Worker pool not bounded "+
				"(grew by %d goroutines, max allowed: %d)",
			goroutineGrowth, maxExpectedGrowth)

		GinkgoWriter.Printf("✅ Worker pool bounded: pool_size=%d, goroutine_growth=%d\n",
			configWithPool.WorkerPoolSize, goroutineGrowth)
	})

	// Test 3.1.5: GW-UNIT-RES-010
	It("should detect goroutine leak with leak detector utility", func() {
		// Given: Leak detector and initial state
		detector := runtime.NewLeakDetector()
		detector.Snapshot() // Take initial snapshot

		// When: Create resources that might leak
		srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		for i := 0; i < 20; i++ {
			signal := createTestPrometheusAlert(PrometheusAlertOptions{
				AlertName: "HighMemoryUsage",
				Namespace: "production",
			})
			_, _ = srv.ProcessSignal(ctx, signal)
		}

		// Cleanup
		_ = srv.Shutdown(ctx)
		time.Sleep(500 * time.Millisecond)

		// Then: No leaks detected
		leaks := detector.CheckLeaks()
		Expect(leaks).To(BeEmpty(),
			"BR-GATEWAY-108: Goroutine leaks detected:\n%s",
			detector.FormatLeaks(leaks))

		GinkgoWriter.Printf("✅ No goroutine leaks detected\n")
	})

	// Test 3.1.6: GW-UNIT-RES-011
	It("should maintain bounded goroutine count under sustained load", func() {
		// Given: Server processing signals continuously
		srv, err := server.New(testConfig, testK8sClient, testRedis, testAudit, testLogger)
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		initialCount := getGoroutineCount()

		// When: Process signals continuously for duration
		duration := 5 * time.Second
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		timeout := time.After(duration)
		signalsProcessed := 0
		maxGoroutines := initialCount

	processLoop:
		for {
			select {
			case <-timeout:
				break processLoop
			case <-ticker.C:
				signal := createTestPrometheusAlert(PrometheusAlertOptions{
					AlertName: "HighMemoryUsage",
					Namespace: "production",
				})
				_, _ = srv.ProcessSignal(ctx, signal)
				signalsProcessed++

				// Track max goroutines
				currentCount := runtime.NumGoroutine()
				if currentCount > maxGoroutines {
					maxGoroutines = currentCount
				}
			}
		}

		// Wait for goroutines to settle
		time.Sleep(500 * time.Millisecond)
		finalCount := getGoroutineCount()

		// Then: Goroutines stayed bounded
		maxGrowth := maxGoroutines - initialCount
		finalGrowth := finalCount - initialCount

		Expect(maxGrowth).To(BeNumerically("<=", 50),
			"BR-GATEWAY-108: Goroutine count unbounded under load "+
				"(peak growth: %d goroutines, max allowed: 50)",
			maxGrowth)

		Expect(finalGrowth).To(BeNumerically("<=", 20),
			"BR-GATEWAY-108: Goroutines not released after load "+
				"(final growth: %d goroutines, max allowed: 20)",
			finalGrowth)

		GinkgoWriter.Printf("✅ Goroutines bounded: processed %d signals, initial=%d, max=%d, final=%d\n",
			signalsProcessed, initialCount, maxGoroutines, finalCount)
	})
})
```

**Implementation Required**:
- `pkg/gateway/server/server.go` - Ensure goroutine cleanup in Shutdown()
- `pkg/gateway/server/worker_pool.go` - Bounded worker pool implementation
- `test/unit/gateway/goroutine_test.go` - Goroutine leak tests (this file)
- `test/unit/gateway/helpers/runtime.go` - Goroutine leak detection helper (if reusable)

**Timeline**: 3-5 hours

---

## 📊 **Test Plan Summary**

| Category | Tests | P0 Tests | P1 Tests | Effort | Priority |
|----------|-------|----------|----------|--------|----------|
| **Log Sanitization** | 8 | 7 | 1 | 2-4h | 🔴 **IMMEDIATE** |
| **Memory Management** | 5 | 4 | 1 | 4-6h | 🟠 **HIGH** |
| **Goroutine Management** | 6 | 5 | 1 | 3-5h | 🟠 **HIGH** |
| **TOTAL** | **19** | **16** | **3** | **9-15h** | - |

---

## ✅ **Success Criteria**

### **Coverage Targets**:
- ✅ **16 P0 tests** addressing critical security and resilience gaps
- ✅ **Fast Execution**: All tests <100ms (unit tier requirement)
- ✅ **No Infrastructure**: Zero external dependencies

### **Quality Targets**:
- ✅ All tests map to P0 business requirements
- ✅ All tests use pure functions or runtime inspection
- ✅ All tests validate business outcomes (not implementation)
- ✅ Reusable test helpers created for other services

### **Business Value Targets**:
- ✅ **Security**: Prevent PII/secret leakage (BR-042)
- ✅ **Reliability**: Prevent memory leak crashes (BR-107)
- ✅ **Reliability**: Prevent goroutine exhaustion (BR-108)
- ✅ **Compliance**: SOC2/GDPR log sanitization requirements

---

## 📝 **Implementation Checklist**

### **Phase 1: Log Sanitization** (Week 1 - 2-4 hours)
- [ ] Verify `pkg/shared/sanitization/sanitizer.go` exists (shared library)
- [ ] Create unit tests in `test/unit/gateway/logging/sanitization_test.go`
- [ ] Create fallback tests in `test/unit/gateway/logging/sanitization_fallback_test.go`
- [ ] Run tests: `go test ./test/unit/gateway/logging/... -v`
- [ ] Verify all 8 tests passing

### **Phase 2: Memory Management** (Week 1-2 - 4-6 hours)
- [ ] Review `pkg/gateway/server/server.go` for cleanup patterns
- [ ] Implement proper resource cleanup in `Shutdown()`
- [ ] Create memory tests in `test/unit/gateway/memory_test.go`
- [ ] Run tests: `go test ./test/unit/gateway/... -run Memory -v`
- [ ] Verify all 5 tests passing

### **Phase 3: Goroutine Management** (Week 2 - 3-5 hours)
- [ ] Review `pkg/gateway/server/server.go` for goroutine lifecycle
- [ ] Implement worker pool in `pkg/gateway/server/worker_pool.go`
- [ ] Create goroutine tests in `test/unit/gateway/goroutine_test.go`
- [ ] Create runtime helpers in `test/unit/gateway/helpers/runtime.go` (if needed)
- [ ] Run tests: `go test ./test/unit/gateway/... -run Goroutine -v`
- [ ] Verify all 6 tests passing

### **Final Validation**
- [ ] Run all unit tests: `go test ./test/unit/gateway/... -v`
- [ ] Verify 70%+ unit test coverage
- [ ] No lint errors
- [ ] All P0 gaps addressed

---

## 📚 **References**

- **Gap Analysis**: `GW_TEST_COVERAGE_GAP_ANALYSIS_JAN17_2026.md`
- **Business Requirements**: `BUSINESS_REQUIREMENTS.md` v1.6
- **Testing Guidelines**: `TESTING_GUIDELINES.md` (Unit test patterns)
- **Integration Plan**: `GW_INTEGRATION_TEST_PLAN_V1.0.md` (Complementary tests)

---

## 📝 **Change Log**

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| v1.0.1 | 2026-01-17 | **Corrected test paths**: Fixed incorrect `pkg/testutil/` references to use correct `test/unit/gateway/` and `test/shared/` patterns. Clarified that BR-042 uses existing `pkg/shared/sanitization/` library (no custom implementation needed). | Gateway Team |
| v1.0 | 2026-01-17 | **Initial unit test plan created**: 19 unit tests across 3 P0 security and resilience gaps (BR-042, BR-107, BR-108). Includes comprehensive test scenarios for log sanitization, memory leak detection, and goroutine leak detection. Timeline: 9-15 hours (1-2 sprints). Status: Planned. | Gateway Team |

---

**Status**: 📋 **PLANNED** | P0 Security & Resilience Gaps
**Created**: January 17, 2026
**Timeline**: 1-2 sprints (9-15 hours)
**Priority**: 🔴 **IMMEDIATE** (Security) + 🟠 **HIGH** (Resilience)
**Compliance**: SOC2, GDPR, V1.0 production readiness
