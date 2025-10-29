## 🧪 UNIT TEST EDGE CASE EXPANSION (DAYS 7.5: HIGH PRIORITY)

**Status**: ⚠️ **REQUIRED - Days 1-7 Complete, Unit Test Edge Cases Identified**
**Priority**: **HIGH - Production Risk Mitigation + Security Hardening**
**Estimated Effort**: 5-8 hours (split into 3 phases)
**Test Count**: **+35 edge case tests** (125 → 160 tests, +28% increase)
**Confidence**: 70% → 75% (after completion)

### **Why Unit Test Edge Cases Are Critical**

**Current Unit Test Status**: 125 tests covering common scenarios and basic error cases
**Gap Identified**: Missing edge cases that cause production incidents

**Production Risks Without Edge Case Testing**:
- **HIGH RISK**: Security vulnerabilities (SQL injection, log injection, null bytes)
- **HIGH RISK**: International support failures (Unicode, emoji, multi-byte characters)
- **MEDIUM RISK**: K8s compliance violations (DNS-1123, length limits, label/annotation limits)
- **MEDIUM RISK**: Consistency issues (non-deterministic fingerprints, nil vs empty)
- **MEDIUM RISK**: DoS attacks (deep nesting, large payloads, extreme values)

**See**: `UNIT_TEST_EDGE_CASE_EXPANSION.md` for detailed risk assessment and test specifications

---

### **Edge Case Expansion by Category**

#### **Category 1: Payload Validation Edge Cases (+10 tests)**

**Current**: 15 validation tests (basic malformed JSON, missing fields)
**Expanded**: 25 validation tests (+10 edge cases)
**File**: `test/unit/gateway/adapters/validation_test.go`

**NEW Test 1: Extremely Large Label Values**
```go
Entry("label value >10KB → should truncate or reject",
    "protects from memory exhaustion attacks",
    []byte(`{"alerts": [{"labels": {"alertname": "Test", "description": "`+strings.Repeat("A", 15000)+`"}}]}`),
    "label value too large"),
```

**Why This Matters**:
- **Production Risk**: Malicious/misconfigured alerts with huge annotations
- **Business Impact**: Gateway OOM from single large payload
- **BR Coverage**: BR-010 (payload size limits)

**NEW Test 2: Unicode and Emoji in Alert Names**
```go
Entry("alertname with emoji → should handle or reject gracefully",
    "international users may use Unicode characters",
    []byte(`{"alerts": [{"labels": {"alertname": "🚨 Production Down 中文", "namespace": "prod"}}]}`),
    ""), // Should either accept or reject with clear error
```

**Why This Matters**:
- **Production Risk**: Unicode handling bugs cause parsing failures
- **Business Impact**: International teams' alerts rejected
- **BR Coverage**: BR-001, BR-003 (international support)

**NEW Test 3: SQL Injection Attempt in Labels**
```go
Entry("SQL injection in label → should sanitize",
    "protects from injection attacks",
    []byte(`{"alerts": [{"labels": {"alertname": "Test'; DROP TABLE alerts;--", "namespace": "prod"}}]}`),
    ""), // Should sanitize or reject
```

**Why This Matters**:
- **Production Risk**: Malicious payloads attempt injection
- **Business Impact**: Security vulnerability if labels stored in DB
- **BR Coverage**: BR-010 (input sanitization)

**NEW Test 4: Null Bytes in Payload**
```go
Entry("null bytes in payload → should reject",
    "null bytes can cause parsing issues",
    []byte("{\x00\"alerts\": [{\"labels\": {\"alertname\": \"Test\"}}]}"),
    "invalid character"),
```

**Why This Matters**:
- **Production Risk**: Null bytes cause Go string handling issues
- **Business Impact**: Parsing failures, potential crashes
- **BR Coverage**: BR-003 (payload validation)

**NEW Test 5: Deeply Nested JSON (100+ levels)**
```go
Entry("deeply nested JSON → should reject to prevent stack overflow",
    "protects from algorithmic complexity attacks",
    generateDeeplyNestedJSON(150), // Helper function
    "nesting too deep"),
```

**Why This Matters**:
- **Production Risk**: Deeply nested JSON causes stack overflow
- **Business Impact**: Gateway crash from malicious payload
- **BR Coverage**: BR-010 (complexity limits)

**NEW Test 6: Duplicate Label Keys**
```go
Entry("duplicate label keys → should handle deterministically",
    "JSON parsers may handle duplicates differently",
    []byte(`{"alerts": [{"labels": {"alertname": "First", "alertname": "Second", "namespace": "prod"}}]}`),
    ""), // Should use first or last consistently
```

**Why This Matters**:
- **Production Risk**: Inconsistent duplicate handling
- **Business Impact**: Alert misidentification
- **BR Coverage**: BR-001 (deterministic parsing)

**NEW Test 7: Scientific Notation in Numeric Fields**
```go
Entry("scientific notation in timestamp → should parse correctly",
    "timestamps may use scientific notation",
    []byte(`{"alerts": [{"labels": {"alertname": "Test"}, "startsAt": "1.698e9"}]}`),
    ""), // Should parse or reject with clear error
```

**Why This Matters**:
- **Production Risk**: Timestamp parsing failures
- **Business Impact**: Incorrect alert timing
- **BR Coverage**: BR-001 (timestamp handling)

**NEW Test 8: Mixed Case in Required Fields**
```go
Entry("mixed case 'AlertName' instead of 'alertname' → should reject",
    "case sensitivity must be consistent",
    []byte(`{"alerts": [{"labels": {"AlertName": "Test", "namespace": "prod"}}]}`),
    "missing alertname"), // Case-sensitive field names
```

**Why This Matters**:
- **Production Risk**: Case sensitivity confusion
- **Business Impact**: Valid alerts rejected due to casing
- **BR Coverage**: BR-003 (field name validation)

**NEW Test 9: Negative Numeric Values in Unexpected Fields**
```go
Entry("negative replica count → should reject",
    "negative values in count fields are invalid",
    []byte(`{"alerts": [{"labels": {"alertname": "Test", "replicas": "-5"}}]}`),
    ""), // Should validate numeric constraints
```

**Why This Matters**:
- **Production Risk**: Negative values cause logic errors
- **Business Impact**: Invalid remediation actions
- **BR Coverage**: BR-006 (resource extraction validation)

**NEW Test 10: Control Characters in Strings**
```go
Entry("control characters (\\r\\n\\t) in alertname → should sanitize",
    "control characters can break log parsing",
    []byte(`{"alerts": [{"labels": {"alertname": "Test\r\nInjection\t", "namespace": "prod"}}]}`),
    ""), // Should sanitize or reject
```

**Why This Matters**:
- **Production Risk**: Control characters break logging/monitoring
- **Business Impact**: Log injection attacks
- **BR Coverage**: BR-024 (logging safety)

---

#### **Category 2: Fingerprint Generation Edge Cases (+8 tests)**

**Current**: 8 fingerprint tests (basic generation, uniqueness)
**Expanded**: 16 fingerprint tests (+8 edge cases)
**File**: `test/unit/gateway/deduplication_test.go`

**NEW Test 1: Fingerprint Collision Probability**
```go
It("should generate unique fingerprints for 10,000 similar alerts", func() {
    // BR-GATEWAY-008: Fingerprint uniqueness
    // BUSINESS OUTCOME: No false duplicates even with similar alerts

    fingerprints := make(map[string]bool)

    for i := 0; i < 10000; i++ {
        signal := &types.NormalizedSignal{
            AlertName: fmt.Sprintf("HighMemory-%d", i),
            Namespace: "production",
            Resource:  types.ResourceIdentifier{Kind: "Pod", Name: fmt.Sprintf("pod-%d", i)},
        }

        fingerprint := generateFingerprint(signal)

        Expect(fingerprints[fingerprint]).To(BeFalse(),
            "fingerprint collision detected at iteration %d", i)
        fingerprints[fingerprint] = true
    }

    // BUSINESS OUTCOME: 10,000 unique fingerprints generated
    Expect(len(fingerprints)).To(Equal(10000))
})
```

**Why This Matters**:
- **Production Risk**: Hash collisions cause false duplicates
- **Business Impact**: Different alerts treated as same incident
- **BR Coverage**: BR-008 (fingerprint uniqueness)

**NEW Test 2: Fingerprint Stability Across Restarts**
```go
It("should generate same fingerprint for same alert across service restarts", func() {
    // BR-GATEWAY-008: Fingerprint determinism
    // BUSINESS OUTCOME: Deduplication works across Gateway restarts

    signal := &types.NormalizedSignal{
        AlertName: "DatabaseDown",
        Namespace: "production",
        Resource:  types.ResourceIdentifier{Kind: "Pod", Name: "postgres-0"},
    }

    // Generate fingerprint multiple times
    fingerprint1 := generateFingerprint(signal)
    fingerprint2 := generateFingerprint(signal)

    // Simulate service restart (recreate objects)
    signal2 := &types.NormalizedSignal{
        AlertName: "DatabaseDown",
        Namespace: "production",
        Resource:  types.ResourceIdentifier{Kind: "Pod", Name: "postgres-0"},
    }
    fingerprint3 := generateFingerprint(signal2)

    // BUSINESS OUTCOME: Same fingerprint every time
    Expect(fingerprint1).To(Equal(fingerprint2))
    Expect(fingerprint1).To(Equal(fingerprint3))
})
```

**Why This Matters**:
- **Production Risk**: Non-deterministic fingerprints break deduplication
- **Business Impact**: Duplicates after Gateway restart
- **BR Coverage**: BR-008 (fingerprint determinism)

**NEW Test 3: Fingerprint with Unicode Characters**
```go
It("should handle Unicode characters in fingerprint generation", func() {
    // BR-GATEWAY-008: Unicode handling

    signal := &types.NormalizedSignal{
        AlertName: "数据库故障", // Chinese characters
        Namespace: "production",
        Resource:  types.ResourceIdentifier{Kind: "Pod", Name: "postgres-0"},
    }

    fingerprint := generateFingerprint(signal)

    // BUSINESS OUTCOME: Valid fingerprint generated
    Expect(fingerprint).ToNot(BeEmpty())
    Expect(len(fingerprint)).To(Equal(64)) // SHA256 hex length
})
```

**Why This Matters**:
- **Production Risk**: Unicode breaks hash generation
- **Business Impact**: International alerts fail
- **BR Coverage**: BR-008 (Unicode support)

**NEW Test 4: Fingerprint with Empty Optional Fields**
```go
It("should generate consistent fingerprint with empty optional fields", func() {
    // BR-GATEWAY-008: Optional field handling

    signal1 := &types.NormalizedSignal{
        AlertName: "Test",
        Namespace: "production",
        Resource:  types.ResourceIdentifier{Kind: "Pod", Name: "test"},
        Severity:  "", // Empty optional field
    }

    signal2 := &types.NormalizedSignal{
        AlertName: "Test",
        Namespace: "production",
        Resource:  types.ResourceIdentifier{Kind: "Pod", Name: "test"},
        // Severity not set (nil vs empty string)
    }

    fingerprint1 := generateFingerprint(signal1)
    fingerprint2 := generateFingerprint(signal2)

    // BUSINESS OUTCOME: Empty and nil treated consistently
    Expect(fingerprint1).To(Equal(fingerprint2))
})
```

**Why This Matters**:
- **Production Risk**: Nil vs empty string inconsistency
- **Business Impact**: Same alert generates different fingerprints
- **BR Coverage**: BR-008 (field normalization)

**NEW Test 5: Fingerprint with Extremely Long Resource Names**
```go
It("should handle extremely long resource names in fingerprint", func() {
    // BR-GATEWAY-008: Long name handling

    longName := strings.Repeat("very-long-pod-name-", 100) // 1900 chars

    signal := &types.NormalizedSignal{
        AlertName: "Test",
        Namespace: "production",
        Resource:  types.ResourceIdentifier{Kind: "Pod", Name: longName},
    }

    fingerprint := generateFingerprint(signal)

    // BUSINESS OUTCOME: Fingerprint generated without error
    Expect(fingerprint).ToNot(BeEmpty())
    Expect(len(fingerprint)).To(Equal(64))
})
```

**Why This Matters**:
- **Production Risk**: Long names cause hash failures
- **Business Impact**: Alerts with long names fail
- **BR Coverage**: BR-008 (extreme values)

**NEW Test 6-8: Additional Fingerprint Edge Cases**
- Fingerprint with special characters in namespace (`prod-us-west-2`)
- Fingerprint with numeric-only resource names (`12345`)
- Fingerprint order independence (labels in different order → same fingerprint)

---

#### **Category 3: Priority Classification Edge Cases (+7 tests)**

**Current**: 12 priority tests (basic severity + environment)
**Expanded**: 19 priority tests (+7 edge cases)
**File**: `test/unit/gateway/priority_classification_test.go`

**NEW Test 1: Conflicting Priority Indicators**
```go
It("should resolve conflicting priority indicators (critical severity + dev namespace)", func() {
    // BR-GATEWAY-020: Priority conflict resolution
    // BUSINESS SCENARIO: Critical alert in dev environment

    signal := &types.NormalizedSignal{
        AlertName: "DatabaseDown",
        Namespace: "dev-testing",
        Severity:  "critical", // Indicates P0
    }

    priority := classifyPriority(signal)

    // BUSINESS OUTCOME: Environment takes precedence (dev = P3)
    // Critical severity in dev is less urgent than warning in production
    Expect(priority).To(Equal("P3"),
        "dev environment should downgrade priority regardless of severity")
})
```

**Why This Matters**:
- **Production Risk**: Conflicting signals cause wrong priority
- **Business Impact**: Dev alerts escalated unnecessarily
- **BR Coverage**: BR-020 (priority resolution logic)

**NEW Test 2: Unknown/Custom Severity Levels**
```go
It("should handle unknown severity levels gracefully", func() {
    // BR-GATEWAY-020: Unknown severity handling

    signal := &types.NormalizedSignal{
        AlertName: "Test",
        Namespace: "production",
        Severity:  "super-critical-emergency", // Non-standard
    }

    priority := classifyPriority(signal)

    // BUSINESS OUTCOME: Default to safe priority (P2)
    Expect(priority).To(Or(Equal("P1"), Equal("P2")),
        "unknown severity should default to medium-high priority")
})
```

**Why This Matters**:
- **Production Risk**: Custom severity levels cause classification failures
- **Business Impact**: Alerts with custom severities ignored
- **BR Coverage**: BR-020 (fallback logic)

**NEW Test 3: Priority with Missing Namespace**
```go
It("should handle missing namespace in priority classification", func() {
    // BR-GATEWAY-020: Missing namespace handling

    signal := &types.NormalizedSignal{
        AlertName: "Test",
        Namespace: "", // Missing
        Severity:  "critical",
    }

    priority := classifyPriority(signal)

    // BUSINESS OUTCOME: Default to high priority (assume production)
    Expect(priority).To(Equal("P1"),
        "missing namespace should default to high priority (fail-safe)")
})
```

**Why This Matters**:
- **Production Risk**: Missing namespace causes classification failure
- **Business Impact**: Critical alerts deprioritized
- **BR Coverage**: BR-020 (fail-safe defaults)

**NEW Test 4-7: Additional Priority Edge Cases**
- Priority with ambiguous namespace patterns (`prod-test`, `staging-prod`)
- Priority with case-insensitive namespace matching (`Production` vs `production`)
- Priority with numeric namespaces (`ns-12345`)
- Priority with extremely long namespace names (>253 chars, K8s limit)

---

#### **Category 4: Storm Detection Edge Cases (+5 tests)**

**Current**: 12 storm tests (basic threshold, time windows)
**Expanded**: 17 storm tests (+5 edge cases)
**File**: `test/unit/gateway/storm_detection_test.go`

**NEW Test 1: Storm Detection with Identical Timestamps**
```go
It("should handle multiple alerts with identical timestamps", func() {
    // BR-GATEWAY-007: Timestamp collision handling
    // BUSINESS SCENARIO: Batch alerts arrive with same timestamp

    timestamp := time.Now()

    for i := 0; i < 15; i++ {
        signal := &types.NormalizedSignal{
            AlertName: fmt.Sprintf("Alert-%d", i),
            Namespace: "production",
            Timestamp: timestamp, // Same timestamp
        }

        isStorm, _, _ := stormDetector.Check(ctx, signal)

        if i >= 9 {
            // BUSINESS OUTCOME: Storm detected even with identical timestamps
            Expect(isStorm).To(BeTrue(),
                "storm should be detected based on count, not time spread")
        }
    }
})
```

**Why This Matters**:
- **Production Risk**: Batch alerts have same timestamp
- **Business Impact**: Storm detection fails for batch alerts
- **BR Coverage**: BR-007 (timestamp handling)

**NEW Test 2: Storm Detection Across Midnight**
```go
It("should handle storm detection across midnight boundary", func() {
    // BR-GATEWAY-007: Time boundary handling
    // BUSINESS SCENARIO: Storm starts before midnight, continues after

    // Send 5 alerts at 23:59:50
    beforeMidnight := time.Date(2025, 10, 22, 23, 59, 50, 0, time.UTC)
    for i := 0; i < 5; i++ {
        signal := &types.NormalizedSignal{
            AlertName: "Test",
            Namespace: "production",
            Timestamp: beforeMidnight.Add(time.Duration(i) * time.Second),
        }
        _, _, _ = stormDetector.Check(ctx, signal)
    }

    // Send 10 alerts at 00:00:05 (next day)
    afterMidnight := time.Date(2025, 10, 23, 0, 0, 5, 0, time.UTC)
    for i := 0; i < 10; i++ {
        signal := &types.NormalizedSignal{
            AlertName: "Test",
            Namespace: "production",
            Timestamp: afterMidnight.Add(time.Duration(i) * time.Second),
        }
        isStorm, _, _ := stormDetector.Check(ctx, signal)

        if i >= 4 { // Total 15 alerts
            // BUSINESS OUTCOME: Storm detected across day boundary
            Expect(isStorm).To(BeTrue())
        }
    }
})
```

**Why This Matters**:
- **Production Risk**: Time boundary bugs
- **Business Impact**: Storm detection resets at midnight
- **BR Coverage**: BR-007 (time window handling)

**NEW Test 3-5: Additional Storm Edge Cases**
- Storm detection with alerts arriving out of order (timestamp T+5 before T+2)
- Storm detection with future timestamps (clock skew)
- Storm detection with alerts spread exactly at threshold boundary (10 alerts in exactly 60s)

---

#### **Category 5: CRD Metadata Generation Edge Cases (+5 tests)**

**Current**: 8 CRD metadata tests (basic fields, annotations)
**Expanded**: 13 CRD metadata tests (+5 edge cases)
**File**: `test/unit/gateway/crd_metadata_test.go`

**NEW Test 1: CRD Name Length Limit (K8s 253 char limit)**
```go
It("should truncate CRD name if it exceeds K8s limit", func() {
    // BR-GATEWAY-015: K8s name length compliance

    longAlertName := strings.Repeat("very-long-alert-name-", 20) // >253 chars

    signal := &types.NormalizedSignal{
        AlertName: longAlertName,
        Namespace: "production",
        Resource:  types.ResourceIdentifier{Kind: "Pod", Name: "test"},
    }

    crdName := generateCRDName(signal)

    // BUSINESS OUTCOME: CRD name fits K8s limits
    Expect(len(crdName)).To(BeNumerically("<=", 253),
        "CRD name must comply with K8s DNS-1123 subdomain limit")

    // Should still be unique (include hash suffix)
    Expect(crdName).To(MatchRegexp(`-[a-f0-9]{8}$`),
        "truncated name should include hash for uniqueness")
})
```

**Why This Matters**:
- **Production Risk**: Long names cause K8s API rejection
- **Business Impact**: CRD creation fails
- **BR Coverage**: BR-015 (K8s compliance)

**NEW Test 2: CRD Name with Invalid DNS Characters**
```go
It("should sanitize CRD name to be DNS-1123 compliant", func() {
    // BR-GATEWAY-015: DNS-1123 compliance

    signal := &types.NormalizedSignal{
        AlertName: "Alert_With_Underscores & Spaces!",
        Namespace: "production",
        Resource:  types.ResourceIdentifier{Kind: "Pod", Name: "test"},
    }

    crdName := generateCRDName(signal)

    // BUSINESS OUTCOME: CRD name is K8s-compliant
    Expect(crdName).To(MatchRegexp(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`),
        "CRD name must be DNS-1123 compliant (lowercase alphanumeric + hyphens)")
})
```

**Why This Matters**:
- **Production Risk**: Invalid characters cause K8s API rejection
- **Business Impact**: CRD creation fails
- **BR Coverage**: BR-015 (name sanitization)

**NEW Test 3-5: Additional CRD Metadata Edge Cases**
- CRD labels with values >63 chars (K8s label value limit)
- CRD annotations with values >256KB (K8s annotation limit)
- CRD owner references with circular dependencies

---

### **Implementation Phases**

#### **Phase 1: High Priority** (15 tests, 2-3 hours)
**Focus**: Critical security, K8s compliance, and consistency edge cases

**Tests to Implement**:
- Payload validation extreme values (5 tests)
- Fingerprint collision/determinism (5 tests)
- Priority conflict resolution (3 tests)
- CRD name limits (2 tests)

**Deliverables**:
- ✅ Security hardening (injection attacks)
- ✅ K8s compliance (name/label limits)
- ✅ Consistency (fingerprint determinism)
- ✅ Confidence: 70% → 72%

#### **Phase 2: Medium Priority** (12 tests, 2-3 hours)
**Focus**: Unicode/encoding, storm boundaries, priority edge cases

**Tests to Implement**:
- Unicode/encoding handling (5 tests)
- Storm detection boundaries (3 tests)
- Priority edge cases (4 tests)

**Deliverables**:
- ✅ International support validated
- ✅ Time boundary bugs prevented
- ✅ Priority resolution comprehensive
- ✅ Confidence: 72% → 74%

#### **Phase 3: Lower Priority** (8 tests, 1-2 hours)
**Focus**: Additional malicious input handling, fingerprint edge cases, CRD metadata

**Tests to Implement**:
- Malicious input handling (3 tests)
- Additional fingerprint edge cases (3 tests)
- Additional CRD metadata edge cases (2 tests)

**Deliverables**:
- ✅ DoS protection tested
- ✅ Fingerprint edge cases covered
- ✅ CRD metadata comprehensive
- ✅ Confidence: 74% → 75%

**Total Effort**: 5-8 hours

---

### **Edge Case Expansion Summary**

#### **Test Count by Category**:

| Category | Current | Expanded | Added | Increase |
|----------|---------|----------|-------|----------|
| **Payload Validation** | 15 | 25 | +10 | +67% |
| **Fingerprint Generation** | 8 | 16 | +8 | +100% |
| **Priority Classification** | 12 | 19 | +7 | +58% |
| **Storm Detection** | 12 | 17 | +5 | +42% |
| **CRD Metadata** | 8 | 13 | +5 | +63% |
| **Other** | 70 | 70 | 0 | 0% |
| **TOTAL** | **125** | **160** | **+35** | **+28%** |

#### **Edge Case Categories Added**:

1. **Extreme Values** (10 tests): Large payloads, long strings, deep nesting, numeric limits
2. **Unicode & Encoding** (5 tests): Emoji, international characters, control characters
3. **Malicious Inputs** (5 tests): SQL injection, null bytes, log injection
4. **Boundary Conditions** (8 tests): Time boundaries, length limits, threshold edges
5. **Consistency** (7 tests): Determinism, case sensitivity, nil vs empty

#### **Production Risk Coverage**:

**Original Plan**: Covers common scenarios and basic error cases
**Expanded Plan**: Also covers edge cases that cause production incidents

**Risk Categories Added**:
- **Security**: SQL injection, log injection, null bytes
- **International**: Unicode, emoji, multi-byte characters
- **Limits**: K8s limits (253 chars, 63 char labels, 256KB annotations)
- **Consistency**: Deterministic behavior, restart stability
- **Malicious**: DoS attacks (deep nesting, large payloads)

---

### **Benefits**

#### **Improved Coverage**:
- **Before**: 125 tests covering common scenarios
- **After**: 160 tests covering common + edge cases
- **Increase**: +28% test count, +50% edge case coverage

#### **Production Readiness**:
- ✅ Security vulnerabilities tested (injection attacks)
- ✅ International support validated (Unicode)
- ✅ K8s compliance verified (DNS-1123, length limits)
- ✅ Consistency guaranteed (deterministic behavior)
- ✅ DoS protection tested (extreme values)

#### **Confidence Improvement**:
- **Current**: 70% confidence (Days 1-7 complete)
- **After Edge Case Expansion**: 75% confidence (comprehensive unit tests)
- **After Integration Tests (Days 8-10)**: 95% confidence (defense-in-depth complete)

---

### **Recommendation**

**Proceed with Phase 1 (High Priority) immediately**: 15 critical edge case tests that address the most likely production issues.

**Benefits**:
- ✅ Security hardening (injection attacks)
- ✅ K8s compliance (name/label limits)
- ✅ Consistency (fingerprint determinism)
- ✅ Minimal effort (2-3 hours)

**Confidence**: 95% (edge cases address real production risks)

---

