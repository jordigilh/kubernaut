# DD-004 v1.1 Format Triage - Design Decision Standards Compliance

**Status**: ⚠️ **ACTION REQUIRED** - DD-004 contains operational tracking (not allowed in DDs)
**Date**: December 18, 2025
**Priority**: 🟡 **MEDIUM** - Document hygiene, not functional issue
**Authority**: DD format standards (per docs/architecture/decisions/README.md)
**Confidence**: **100%** - Clear standards violation

---

## 📋 **EXECUTIVE SUMMARY**

DD-004 v1.1 contains an "Implementation Status by Service" section (lines 584-614) that tracks **operational status**, not design decisions. This violates DD document standards.

### **Problem**
- ❌ **Lines 584-614**: Implementation tracking table (which services are compliant)
- ❌ **Lines 586-597**: Gateway-specific implementation details and evidence
- ❌ **Lines 601-614**: Service status table with completion tracking

### **Standards Violation**
Design Decision (DD) documents should define:
- ✅ **What** the decision is (RFC 7807 standard)
- ✅ **Why** it was chosen (alternatives, rationale)
- ✅ **How** to implement it (patterns, examples, migration guide)
- ✅ **How** to validate compliance (test patterns, checklist)

Design Decision documents **should NOT** contain:
- ❌ **Who** has implemented it (service tracking)
- ❌ **When** services completed implementation (dates)
- ❌ **Status** of individual services (✅ Complete, 🔄 Pending)

---

## 🔍 **DETAILED ANALYSIS**

### **Violation 1: Lines 584-614 - "Implementation Status by Service"**

**Current Content** (❌ VIOLATES DD STANDARDS):
```markdown
### **Implementation Status by Service**

#### **Gateway Service** ✅ **COMPLETE**

**Status**: ✅ RFC 7807 fully implemented

**Evidence**:
- ✅ `pkg/gateway/errors/rfc7807.go` - Error types defined
- ✅ `pkg/gateway/server.go` - Helper functions implemented
- ✅ All error responses use RFC 7807 format
- ✅ Integration tests passing (115 specs)
- ✅ Readiness probe errors use RFC 7807

**Example**: See `docs/architecture/RFC7807_READINESS_UPDATE.md`

---

#### **Other Services** 🔄 **IN PROGRESS**

| Service | Status | Priority | Notes |
|---------|--------|----------|-------|
| **HolmesGPT API (HAPI)** | ✅ Complete (v1.1) | P0 | Dec 18, 2025 |
| **DataStorage** | 🔄 Pending (v1.1) | P0 | Triage complete |
| **Gateway** | 🔄 Pending (v1.1) | P0 | - |
| ~~**Context API**~~ | ❌ Removed | - | Service removed in v1.0 |
| ~~**Dynamic Toolset**~~ | ❌ Removed | - | Service removed in v1.0 |
| ~~**Effectiveness Monitor**~~ | ❌ Removed | - | Service removed in v1.0 |
| **CRD Controllers** | ✅ N/A | - | No HTTP APIs |
```

**Why This Violates Standards**:
1. ❌ **Operational Tracking**: Tracks which services have completed implementation
2. ❌ **Temporal Information**: Contains dates (Dec 18, 2025)
3. ❌ **Status Updates**: Uses status indicators (✅ Complete, 🔄 Pending, ❌ Removed)
4. ❌ **Service-Specific Evidence**: Lists specific files and test counts
5. ❌ **Project Management**: Tracks progress like a project board, not design documentation

**Proper Location**: This content belongs in:
- `docs/handoff/DD_004_V1_1_IMPLEMENTATION_TRACKER.md` (operational tracking)
- Or individual service handoff documents (e.g., `GATEWAY_DD_004_V1_1_TRIAGE_DEC_18_2025.md`)

---

### **What SHOULD Be In "Validation" Section**

**Correct "Validation" Section** (✅ FOLLOWS DD STANDARDS):
```markdown
## ✅ **Validation**

### **Compliance Verification**

**How to verify a service is DD-004 compliant:**

1. **Content-Type Header**: Verify all error responses use `application/problem+json`
2. **Required Fields**: Verify presence of type, title, detail, status, instance
3. **Error Type URIs**: Verify format matches `https://kubernaut.ai/problems/{type}` (v1.1)
4. **Status Code Mapping**: Verify correct error type for each HTTP status
5. **Request ID**: Verify request_id included when available

### **Compliance Tests**

**Required Tests** (per service):

[Test examples showing HOW to validate, not WHO has validated]
```

**Key Difference**:
- ✅ **Describes validation process** (how to check compliance)
- ❌ **Does not track validation results** (which services are compliant)

---

## 📊 **DD FORMAT STANDARDS**

### **From docs/architecture/decisions/README.md**

**DD Documents Should Contain**:
1. **Context**: Problem statement and business impact
2. **Requirements**: Functional and non-functional requirements
3. **Alternatives**: Options considered with pros/cons
4. **Decision**: Which option was chosen and why
5. **Implementation**: How to implement (patterns, examples, code)
6. **Migration Guide**: Step-by-step migration instructions
7. **Validation**: How to verify compliance (not who is compliant)
8. **References**: Related docs, RFCs, standards

**DD Documents Should NOT Contain**:
- ❌ Project tracking (service completion status)
- ❌ Operational status (which services are done)
- ❌ Temporal progress updates (dates of completion)
- ❌ Service-specific implementation evidence

---

## 🎯 **RECOMMENDED FIX**

### **Step 1: Remove Lines 584-614**

**Delete This Section**:
```markdown
### **Implementation Status by Service**

#### **Gateway Service** ✅ **COMPLETE**
[... entire section ...]

#### **Other Services** 🔄 **IN PROGRESS**
[... entire table ...]
```

---

### **Step 2: Replace With Proper "Validation" Section**

**Add This Instead**:
```markdown
### **Validation Strategy**

**How to Verify DD-004 Compliance**:

Services are DD-004 compliant when all HTTP error responses (4xx, 5xx) meet these criteria:

1. ✅ **Content-Type**: `application/problem+json` header present
2. ✅ **Required Fields**: type, title, detail, status, instance all populated
3. ✅ **Error Type URI**: Matches `https://kubernaut.ai/problems/{error-type}` format (v1.1)
4. ✅ **Status Codes**: HTTP status codes unchanged from service's existing behavior
5. ✅ **Extension Members**: Optional request_id field present when available

**Reference Implementation**: `pkg/gateway/errors/rfc7807.go` demonstrates compliant structure

**Note**: Success responses (2xx) are not affected by DD-004 and may use service-specific formats.
```

---

### **Step 3: Create Separate Tracking Document**

**New File**: `docs/handoff/DD_004_V1_1_IMPLEMENTATION_TRACKER.md`

**Content**:
```markdown
# DD-004 v1.1 Implementation Tracker - RFC 7807 Domain/Path Update

**Purpose**: Track DD-004 v1.1 implementation progress across services
**Note**: This is operational tracking, NOT part of DD-004 design document
**Authority**: DD-004 v1.1 (Dec 18, 2025)

---

## 📊 **Implementation Status**

| Service | v1.0 Status | v1.1 Status | Last Updated | Handoff Document |
|---------|-------------|-------------|--------------|------------------|
| **HolmesGPT API** | ✅ Complete | ✅ Complete (v1.1) | Dec 18, 2025 | [HAPI_DOMAIN_CORRECTION_KUBERNAUT_AI_DEC_18_2025.md](HAPI_DOMAIN_CORRECTION_KUBERNAUT_AI_DEC_18_2025.md) |
| **DataStorage** | ✅ Complete | 🔄 Triage Complete | Dec 18, 2025 | [DS_DOMAIN_CORRECTION_KUBERNAUT_AI_DEC_18_2025.md](DS_DOMAIN_CORRECTION_KUBERNAUT_AI_DEC_18_2025.md) |
| **Gateway** | ✅ Complete | 🔄 Triage Complete | Dec 18, 2025 | [GATEWAY_DD_004_V1_1_TRIAGE_DEC_18_2025.md](GATEWAY_DD_004_V1_1_TRIAGE_DEC_18_2025.md) |
| ~~**Context API**~~ | ~~✅ Complete~~ | ❌ Removed | - | Service removed in V1.0 |
| ~~**Dynamic Toolset**~~ | ~~✅ Complete~~ | ❌ Removed | - | Service removed in V1.0 |
| ~~**Effectiveness Monitor**~~ | ~~N/A~~ | ❌ Removed | - | Service removed in V1.0 |
| **CRD Controllers** | ✅ N/A | ✅ N/A | - | No HTTP APIs |

---

## 📝 **V1.1 Update Requirements**

**Domain**: `kubernaut.io` → `kubernaut.ai` (most services already compliant)
**Path**: `/errors/` → `/problems/` (primary change)

**Per Service Effort**: 5-10 minutes (update string constants only)
```

---

## 📋 **CORRECTED DD-004 STRUCTURE**

### **Sections That Are Correct** ✅
1. **Header** (Status, Version, Date, Confidence)
2. **Changelog** (version history)
3. **Overview** (context and problem)
4. **Requirements** (functional and non-functional)
5. **Alternatives Considered** (options with pros/cons)
6. **Decision** (rationale and trade-offs)
7. **Implementation** (error structure, URI convention, helper functions)
8. **Examples** (concrete code samples)
9. **Migration Guide** (step-by-step instructions)
10. **References** (RFC 7807, industry examples)
11. **Summary** (key decisions, confidence, production readiness)

### **Section That Violates Standards** ❌
- **Lines 584-614**: "Implementation Status by Service"
  - ❌ Operational tracking (not design decision)
  - ❌ Should be in handoff document, not DD

---

## ⏱️ **IMPLEMENTATION PLAN**

| Step | Task | Time |
|------|------|------|
| 1 | Remove lines 584-614 from DD-004.md | 1 min |
| 2 | Add proper "Validation Strategy" section | 3 min |
| 3 | Create `DD_004_V1_1_IMPLEMENTATION_TRACKER.md` | 5 min |
| 4 | Move tracking content to new tracker doc | 2 min |
| 5 | Update DD-004 changelog (v1.2 - format correction) | 2 min |
| 6 | Git commit changes | 1 min |
| **Total** | | **14 min** |

---

## 📊 **IMPACT ASSESSMENT**

| Factor | Assessment |
|--------|-----------|
| **Breaking Changes** | ❌ NONE (documentation only) |
| **Functional Impact** | ❌ NONE (no code changes) |
| **Reference Impact** | 🟡 **MINOR** (links to DD-004 still valid) |
| **Tracking Impact** | ✅ **IMPROVED** (tracking in proper location) |
| **Standards Compliance** | ✅ **FIXED** (DD follows proper format) |

**Overall Risk**: 🟢 **NONE** (documentation refactoring only)

---

## ✅ **VALIDATION CHECKLIST**

**After Implementing Fix**:
- [ ] DD-004 contains no service tracking tables
- [ ] DD-004 contains no service-specific implementation details
- [ ] DD-004 contains no temporal status updates (✅ Complete, 🔄 Pending)
- [ ] DD-004 "Validation" section describes HOW to validate, not WHO validated
- [ ] Tracking information moved to separate handoff document
- [ ] DD-004 changelog updated with v1.2 format correction entry
- [ ] All handoff documents reference the new tracker document

---

## 🎯 **RECOMMENDATION**

**Priority**: 🟡 **MEDIUM** - Document hygiene, improves standards compliance

**Action**: ✅ **APPROVED** - Implement format correction to align DD-004 with DD standards

**Rationale**:
1. ✅ **Standards Compliance**: DDs should be timeless design decisions, not status trackers
2. ✅ **Maintainability**: Operational tracking in handoff docs is easier to update
3. ✅ **Clarity**: Separates "what the decision is" from "who has implemented it"
4. ✅ **Low Risk**: Documentation-only change, no code impact

**Next Steps**:
1. Implement the recommended fix (14 minutes)
2. Git commit with descriptive message
3. Update related handoff documents to reference the new tracker

---

## 📚 **RELATED DOCUMENTS**

- **DD-004 v1.1**: [RFC7807 Error Response Standard](../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md)
- **DD Format Standards**: [docs/architecture/decisions/README.md](../architecture/decisions/README.md)
- **Gateway Triage**: [GATEWAY_DD_004_V1_1_TRIAGE_DEC_18_2025.md](GATEWAY_DD_004_V1_1_TRIAGE_DEC_18_2025.md)
- **HAPI Implementation**: [HAPI_DOMAIN_CORRECTION_KUBERNAUT_AI_DEC_18_2025.md](HAPI_DOMAIN_CORRECTION_KUBERNAUT_AI_DEC_18_2025.md)
- **DS Triage**: [DS_DOMAIN_CORRECTION_KUBERNAUT_AI_DEC_18_2025.md](DS_DOMAIN_CORRECTION_KUBERNAUT_AI_DEC_18_2025.md)

---

**END OF TRIAGE DOCUMENT**

