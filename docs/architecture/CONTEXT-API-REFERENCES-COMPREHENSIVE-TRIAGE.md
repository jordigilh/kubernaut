# Context API References - Comprehensive Triage (Makefile, READMEs, Documentation)

**Date**: November 13, 2025
**Status**: 🚨 **CRITICAL - COMPREHENSIVE CLEANUP REQUIRED**
**Scope**: Makefile, README.md files, HolmesGPT API docs, Data Storage docs
**Version**: 1.0

---

## 🎯 **Executive Summary**

After deleting Context API code and updating authoritative architecture documents, **84 Context API references remain** in:
1. **Makefile**: 84 lines (build targets, test targets, Docker targets)
2. **HolmesGPT API Documentation**: 142 lines (BR-HAPI-046 to BR-HAPI-050, implementation plans)
3. **Data Storage Documentation**: 302 lines (mostly historical references, some active integration points)

**Required Actions**:
- **Makefile**: Remove all Context API targets (build, test, docker, deploy)
- **HolmesGPT API**: Replace "Context API" with "Data Storage Service" for semantic search
- **Data Storage**: Update integration points to reflect direct HolmesGPT API access

---

## 📋 **1. Makefile Context API References**

### **Status**: 🚨 **84 LINES TO REMOVE**

### **Affected Targets**

| Target | Lines | Action |
|--------|-------|--------|
| `test-integration-contextapi` | 113-131 | ❌ **DELETE** - No Context API tests exist |
| `build-all-services` | 418 | ✅ **UPDATE** - Remove `build-context-api` |
| `docker-build-microservices` | 511 | ✅ **UPDATE** - Remove `docker-build-context-api` |
| `docker-push-microservices` | 533 | ✅ **UPDATE** - Remove `docker-push-context-api` |
| `build-context-api` | 612-615 | ❌ **DELETE** - Binary no longer exists |
| `run-context-api` | 618-620 | ❌ **DELETE** - Binary no longer exists |
| `test-context-api` | 623-625 | ❌ **DELETE** - Tests deleted |
| `test-context-api-integration` | 628-630 | ❌ **DELETE** - Tests deleted |
| `docker-build-context-api` | 633-638 | ❌ **DELETE** - Dockerfile deleted |
| `docker-push-context-api` | 641-644 | ❌ **DELETE** - Image no longer built |
| `docker-build-context-api-single` | 647-651 | ❌ **DELETE** - Debug image no longer needed |
| `docker-run-context-api` | 654-671 | ❌ **DELETE** - Service no longer exists |
| `docker-run-context-api-with-config` | 674-685 | ❌ **DELETE** - Config file deleted |
| `docker-stop-context-api` | 688-691 | ❌ **DELETE** - Service no longer exists |
| `docker-logs-context-api` | 694-695 | ❌ **DELETE** - Service no longer exists |
| `deploy-context-api` | 698-703 | ❌ **DELETE** - K8s manifests deleted |
| `undeploy-context-api` | 706-709 | ❌ **DELETE** - K8s manifests deleted |
| `validate-context-api-build` | 712-728 | ❌ **DELETE** - Build pipeline no longer exists |
| `test-e2e-contextapi` | 733-735 | ❌ **DELETE** - E2E tests deleted |
| `test-contextapi-all` | 763-779 | ❌ **DELETE** - All test tiers deleted |
| `test-all-services` | 873 | ✅ **UPDATE** - Remove `test-contextapi-all` |

### **Recommended Makefile Changes**

```diff
--- a/Makefile
+++ b/Makefile
@@ -110,25 +110,6 @@ test-integration-datastorage: ## Run Data Storage integration tests (PostgreSQL
 	fi
 
-.PHONY: test-integration-contextapi
-test-integration-contextapi: ## Run Context API integration tests (Redis via Podman + PostgreSQL, ~45s)
-	@echo "🔧 Starting Redis for Context API..."
-	@podman run -d --name contextapi-redis-test -p 6379:6379 quay.io/jordigilh/redis:7-alpine > /dev/null 2>&1 || \
-		(echo "⚠️  Redis container already exists or failed to start" && \
-		 podman start contextapi-redis-test > /dev/null 2>&1) || true
-	@echo "⏳ Waiting for Redis to be ready..."
-	@sleep 2
-	@podman exec contextapi-redis-test redis-cli ping > /dev/null 2>&1 || \
-		(echo "❌ Redis not ready" && exit 1)
-	@echo "✅ Redis ready"
-	@echo "📝 NOTE: PostgreSQL required - run 'make bootstrap-dev' if not running"
-	@echo "🧪 Running Context API integration tests..."
-	@TEST_RESULT=0; \
-	go test ./test/integration/contextapi/... -v -timeout 5m || TEST_RESULT=$$?; \
-	echo "🧹 Cleaning up Redis container..."; \
-	podman stop contextapi-redis-test > /dev/null 2>&1 || true; \
-	podman rm contextapi-redis-test > /dev/null 2>&1 || true; \
-	echo "✅ Cleanup complete"; \
-	exit $$TEST_RESULT
-
 ##@ Microservices Build
 
 .PHONY: build-all-services
-build-all-services: build-gateway-service build-context-api build-datastorage build-dynamictoolset build-notification ## Build all Go services
+build-all-services: build-gateway-service build-datastorage build-dynamictoolset build-notification ## Build all Go services
 
 .PHONY: build-microservices
 build-microservices: build-all-services ## Build all microservices (alias for build-all-services)
@@ -508,7 +489,7 @@ build-microservices: build-all-services ## Build all microservices (alias for b
 
 ##@ Microservices Container Build
 .PHONY: docker-build-microservices
-docker-build-microservices: docker-build-gateway-service docker-build-context-api ## Build all microservice container images
+docker-build-microservices: docker-build-gateway-service ## Build all microservice container images
 
 .PHONY: docker-push-microservices
-docker-push-microservices: docker-push-gateway-service docker-push-context-api ## Push all microservice container images
+docker-push-microservices: docker-push-gateway-service ## Push all microservice container images
 
-##@ Context API Service
-
-# Context API Image Configuration
-CONTEXT_API_IMG ?= quay.io/jordigilh/context-api:v0.1.0
-
-.PHONY: build-context-api
-build-context-api: ## Build Context API binary locally
-	@echo "🔨 Building Context API binary..."
-	go build -o bin/context-api cmd/contextapi/main.go
-	@echo "✅ Binary: bin/context-api"
-
-[... DELETE ALL Context API targets through line 728 ...]
-
-##@ Context API E2E Tests
-
-.PHONY: test-e2e-contextapi
-test-e2e-contextapi: ## Run Context API E2E tests (Kind bootstrapped via Go)
-	@echo "🧪 Running Context API E2E tests..."
-	@cd test/e2e/contextapi && ginkgo -v
-
 ##@ Per-Service Test Suites (All Tiers)
 
-.PHONY: test-contextapi-all
-test-contextapi-all: ## Run ALL Context API tests (unit + integration + e2e)
-	@echo "════════════════════════════════════════════════════════════════════════"
-	@echo "🧪 Context API - Complete Test Suite"
-	@echo "════════════════════════════════════════════════════════════════════════"
-	@FAILED=0; \
-	echo ""; \
-	echo "1️⃣  Unit Tests..."; \
-	go test ./test/unit/contextapi/... -v -timeout=5m || FAILED=$$((FAILED + 1)); \
-	echo ""; \
-	echo "2️⃣  Integration Tests..."; \
-	$(MAKE) test-integration-contextapi || FAILED=$$((FAILED + 1)); \
-	echo ""; \
-	echo "3️⃣  E2E Tests..."; \
-	$(MAKE) test-e2e-contextapi || FAILED=$$((FAILED + 1)); \
-	echo ""; \
-	if [ $$FAILED -eq 0 ]; then \
-		echo "✅ Context API: ALL tests passed (3/3 tiers)"; \
-	else \
-		echo "❌ Context API: $$FAILED test tier(s) failed"; \
-		exit 1; \
-	fi
-
 .PHONY: test-all-services
 test-all-services: ## Run ALL tests for ALL services (unit + integration + e2e)
 	@echo "════════════════════════════════════════════════════════════════════════"
@@ -870,7 +851,6 @@ test-all-services: ## Run ALL tests for ALL services (unit + integration + e2e)
 	@FAILED=0; \
 	$(MAKE) test-gateway-all || FAILED=$$((FAILED + 1)); \
 	echo ""; \
-	$(MAKE) test-contextapi-all || FAILED=$$((FAILED + 1)); \
-	echo ""; \
 	$(MAKE) test-datastorage-all || FAILED=$$((FAILED + 1)); \
 	echo ""; \
```

**Confidence**: 100% - All Context API code deleted, targets are orphaned

---

## 📋 **2. HolmesGPT API Documentation**

### **Status**: 🚨 **142 LINES TO UPDATE**

### **Affected Files**

| File | Lines | Issue | Action |
|------|-------|-------|--------|
| `BR-HAPI-046-050-CONTEXT-API-TOOL.md` | 142 | BR-HAPI-046 to BR-HAPI-050 reference Context API | ✅ **UPDATE** - Replace with Data Storage Service |
| `IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md` | 4 | References Context API as example | ✅ **UPDATE** - Remove Context API reference |
| `testing-strategy.md` | 1 | Cross-service coordination example | ✅ **UPDATE** - Update example |

### **Critical Business Requirement Updates**

**BR-HAPI-046 to BR-HAPI-050** are **STILL VALID** but need to be updated:

| BR | Current Name | Updated Name | Status |
|----|--------------|--------------|--------|
| BR-HAPI-046 | Define `get_context` Tool | Define `get_playbooks` Tool | ✅ **UPDATE** |
| BR-HAPI-047 | Implement Context API Client | Implement Data Storage Client | ✅ **UPDATE** |
| BR-HAPI-048 | Tool Call Handler | Tool Call Handler | ✅ **UPDATE** (endpoint change) |
| BR-HAPI-049 | Tool Call Observability | Tool Call Observability | ✅ **UPDATE** (metric names) |
| BR-HAPI-050 | Tool Call Testing | Tool Call Testing | ✅ **UPDATE** (test names) |

### **Recommended Changes**

#### **File**: `docs/services/stateless/holmesgpt-api/BR-HAPI-046-050-CONTEXT-API-TOOL.md`

**Rename to**: `BR-HAPI-046-050-DATA-STORAGE-PLAYBOOK-TOOL.md`

**Key Changes**:
1. **Tool Name**: `get_context` → `get_playbooks`
2. **Endpoint**: `/api/v1/context/enrich` → `/api/v1/playbooks/search`
3. **Client Class**: `ContextAPIClient` → `DataStorageClient`
4. **Service Reference**: "Context API" → "Data Storage Service"
5. **Metrics**: `holmesgpt_context_tool_*` → `holmesgpt_playbook_tool_*`

**Example Update**:
```diff
-# BR-HAPI-046 to BR-HAPI-050: Context API Tool Integration
+# BR-HAPI-046 to BR-HAPI-050: Data Storage Playbook Tool Integration

-These 5 business requirements define the Context API tool integration for HolmesGPT API Service.
+These 5 business requirements define the Data Storage playbook search tool integration for HolmesGPT API Service.

-## BR-HAPI-046: Define `get_context` Tool
+## BR-HAPI-046: Define `get_playbooks` Tool

-**Category**: Context API Tool Integration
+**Category**: Data Storage Playbook Tool Integration

-System must define a `get_context` tool that allows the LLM to request historical context on-demand.
+System must define a `get_playbooks` tool that allows the LLM to request relevant playbooks via semantic search.

-### Tool Definition
+### Tool Definition

 {
-  "name": "get_context",
-  "description": "Get historical context for similar incidents",
+  "name": "get_playbooks",
+  "description": "Search for relevant remediation playbooks via semantic search",
   "parameters": {
     "type": "object",
     "properties": {
-      "alert_fingerprint": {
+      "query": {
         "type": "string",
-        "description": "Alert fingerprint to find similar incidents"
+        "description": "Natural language query describing the incident"
       },
       "similarity_threshold": {
         "type": "number",
-        "description": "Minimum similarity score (0.0-1.0, default 0.70)"
+        "description": "Minimum cosine similarity score (0.0-1.0, default 0.70)"
       }
     },
-    "required": ["alert_fingerprint"]
+    "required": ["query"]
   }
 }

-## BR-HAPI-047: Implement Context API Client
+## BR-HAPI-047: Implement Data Storage Client

-System must implement a robust HTTP client for Context API with retry logic, circuit breaker, and caching.
+System must implement a robust HTTP client for Data Storage Service with retry logic, circuit breaker, and caching.

-### Client Requirements
+### Client Requirements

-- HTTP client for Context API REST endpoint (`/api/v1/context/enrich`)
+- HTTP client for Data Storage REST endpoint (`/api/v1/playbooks/search`)
 - Retry logic with exponential backoff (max 3 retries)
 - Circuit breaker (opens after 50% failure rate in 5-minute window)
-- Caching of context results within investigation session (1h TTL)
-- Timeout: 2s per request (Context API p95 latency is <500ms)
+- Caching of playbook search results within investigation session (1h TTL)
+- Timeout: 2s per request (Data Storage p95 latency is <500ms)

-class ContextAPIClient:
+class DataStorageClient:
     def __init__(self, base_url: str, timeout: int = 2, max_retries: int = 3):
         self.base_url = base_url
         self.timeout = timeout
@@ -210,7 +210,7 @@ class ContextAPIClient:
         similarity_threshold: float = 0.70,
-        context_types: Optional[List[str]] = None
+        limit: int = 5
     ) -> Dict[str, Any]:
-        """Get context from Context API with retry logic and circuit breaker"""
+        """Search playbooks from Data Storage with retry logic and circuit breaker"""
```

**Confidence**: 95% - Clear 1:1 mapping from Context API to Data Storage Service

---

## 📋 **3. Data Storage Documentation**

### **Status**: ⚠️ **302 LINES TO REVIEW**

### **Affected Files**

| File | Lines | Issue | Action |
|------|-------|-------|--------|
| `BUSINESS_REQUIREMENTS.md` | ~10 | References Context API as consumer | ✅ **UPDATE** - Replace with HolmesGPT API |
| `api-specification.md` | ~5 | Context API aggregation proxy example | ✅ **UPDATE** - Direct HolmesGPT API access |
| `integration-points.md` | ~15 | Context API as downstream reader | ✅ **UPDATE** - Replace with HolmesGPT API |
| `performance-requirements.md` | ~10 | Context API connection pool sizing | ✅ **UPDATE** - HolmesGPT API connection pool |
| `DD-STORAGE-006-V1-NO-CACHE-DECISION.md` | ~10 | Context API query flow diagrams | ✅ **UPDATE** - HolmesGPT API query flow |
| `DD-STORAGE-008-PLAYBOOK-CATALOG-SCHEMA.md` | ~5 | Context API integration note | ✅ **UPDATE** - HolmesGPT API integration |
| `DATA-STORAGE-V1.0-MVP-IMPLEMENTATION-PLAN.md` | ~10 | Context API references in next steps | ✅ **UPDATE** - Remove obsolete next steps |
| `TEMPLATE-TEST-PACKAGE-TRIAGE-CORRECTED.md` | ~10 | Context API test examples | ℹ️ **KEEP** - Historical reference only |
| `testing-strategy.md` | ~5 | Context API container sharing | ✅ **UPDATE** - Remove Context API reference |

### **Recommended Changes**

#### **File**: `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md`

```diff
-**Use Case**: Context API queries Data Storage to find playbooks semantically similar to incident description
+**Use Case**: HolmesGPT API queries Data Storage to find playbooks semantically similar to incident description
```

#### **File**: `docs/services/stateless/data-storage/api-specification.md`

```diff
-**Context API** (Aggregation Proxy):
+**HolmesGPT API** (Direct Consumer):
 ```go
-// Context API exposes this endpoint to other services
-func (s *Server) HandleGetMultiDimensionalSuccessRate(w http.ResponseWriter, r *http.Request) {
-    // Forward to Data Storage with authentication
-    resp, err := s.dataStorageClient.GetMultiDimensionalSuccessRate(r.Context(), r.URL.Query())
+// HolmesGPT API calls Data Storage directly
+func (s *HolmesGPTService) GetPlaybookSuccessRate(ctx context.Context, playbookID string) (*SuccessRate, error) {
+    // Direct call to Data Storage with authentication
+    resp, err := s.dataStorageClient.GetMultiDimensionalSuccessRate(ctx, playbookID)
```

#### **File**: `docs/services/stateless/data-storage/integration-points.md`

```diff
-### **1. Context API Service (Planned Integration)**
+### **1. HolmesGPT API Service (Active Integration)**

-**Purpose**: Query historical incident data for AI context enrichment
+**Purpose**: Query playbook catalog for semantic search during incident investigation

-// In Context API Service (planned)
+// In HolmesGPT API Service (active)
 import (
     "encoding/json"
     "fmt"
```

#### **File**: `docs/services/stateless/data-storage/performance-requirements.md`

```diff
 | Parameter | Value | Rationale |
 |-----------|-------|-----------|
-| **max_connections** | 100 | 20 (Data Storage) + 50 (Context API) + 30 (other services) |
+| **max_connections** | 100 | 20 (Data Storage) + 50 (HolmesGPT API) + 30 (other services) |

-2. **Read Replicas**: Add PostgreSQL read replica for Context API queries - **Reduces write contention**
+2. **Read Replicas**: Add PostgreSQL read replica for HolmesGPT API queries - **Reduces write contention**
```

#### **File**: `docs/services/stateless/data-storage/implementation/DD-STORAGE-006-V1-NO-CACHE-DECISION.md`

```diff
 **Query Flow**:
 ```
-Incident arrives → Context API
+Incident arrives → HolmesGPT API
     ↓
 Data Storage Service: GET /api/v1/playbooks/search
     ↓
```

#### **File**: `docs/services/stateless/data-storage/implementation/DD-STORAGE-008-PLAYBOOK-CATALOG-SCHEMA.md`

```diff
-**Affects**: Data Storage Service V1.0 MVP, Context API integration
+**Affects**: Data Storage Service V1.0 MVP, HolmesGPT API integration
```

#### **File**: `docs/services/stateless/data-storage/implementation/DATA-STORAGE-V1.0-MVP-IMPLEMENTATION-PLAN.md`

```diff
 ## 📋 **Next Steps**
 
 1. ✅ **DD-STORAGE-010 Approved** (this document)
-2. 🚧 **Create DD-CONTEXT-006-MIGRATION**: Context API migration plan (salvage patterns)
-3. 🚧 **Create DD-STORAGE-011**: Data Storage V1.1 Implementation Plan (high-level)
+2. ✅ **DD-CONTEXT-006 Complete**: Context API deprecated (no salvageable patterns)
+3. 🚧 **Create DD-STORAGE-011**: Data Storage V1.1 Implementation Plan (high-level)
```

#### **File**: `docs/services/stateless/data-storage/testing-strategy.md`

```diff
 **Test Setup Helper**: `testcontainers-go` (Podman/Docker testcontainers)
 
-**Container Sharing**: Same containers used by Context API Service (efficiency)
+**Container Sharing**: Shared PostgreSQL/Redis containers for integration tests (efficiency)
```

**Confidence**: 90% - Most references are historical/documentation, low risk

---

## 📋 **4. Implementation Plan**

### **Phase 1: Makefile Cleanup** (30 minutes)
- [ ] Remove all Context API build targets (lines 612-728)
- [ ] Remove `test-integration-contextapi` (lines 113-131)
- [ ] Remove `test-contextapi-all` (lines 763-779)
- [ ] Update `build-all-services` to remove `build-context-api`
- [ ] Update `docker-build-microservices` to remove `docker-build-context-api`
- [ ] Update `docker-push-microservices` to remove `docker-push-context-api`
- [ ] Update `test-all-services` to remove `test-contextapi-all`
- [ ] Test: `make help` (verify no Context API targets listed)

### **Phase 2: HolmesGPT API Documentation** (1 hour)
- [ ] Rename `BR-HAPI-046-050-CONTEXT-API-TOOL.md` → `BR-HAPI-046-050-DATA-STORAGE-PLAYBOOK-TOOL.md`
- [ ] Update BR-HAPI-046: `get_context` → `get_playbooks`
- [ ] Update BR-HAPI-047: `ContextAPIClient` → `DataStorageClient`
- [ ] Update BR-HAPI-048: Endpoint `/api/v1/context/enrich` → `/api/v1/playbooks/search`
- [ ] Update BR-HAPI-049: Metrics `holmesgpt_context_tool_*` → `holmesgpt_playbook_tool_*`
- [ ] Update BR-HAPI-050: Test names `test_context_*` → `test_playbook_*`
- [ ] Update `IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md` (remove Context API reference)
- [ ] Update `testing-strategy.md` (update cross-service example)
- [ ] Bump version to v1.1, add changelog

### **Phase 3: Data Storage Documentation** (45 minutes)
- [ ] Update `BUSINESS_REQUIREMENTS.md` (Context API → HolmesGPT API)
- [ ] Update `api-specification.md` (aggregation proxy example)
- [ ] Update `integration-points.md` (downstream reader)
- [ ] Update `performance-requirements.md` (connection pool sizing)
- [ ] Update `DD-STORAGE-006-V1-NO-CACHE-DECISION.md` (query flow diagrams)
- [ ] Update `DD-STORAGE-008-PLAYBOOK-CATALOG-SCHEMA.md` (integration note)
- [ ] Update `DATA-STORAGE-V1.0-MVP-IMPLEMENTATION-PLAN.md` (next steps)
- [ ] Update `testing-strategy.md` (container sharing note)
- [ ] Bump versions, add changelogs

### **Phase 4: Verification** (15 minutes)
- [ ] `grep -r "context-api\|contextapi\|context_api" docs/ Makefile --include="*.md"` (expect 0 active references)
- [ ] `make help | grep -i context` (expect 0 results)
- [ ] Review all updated files for consistency
- [ ] Commit with message: `docs: remove Context API references from Makefile and documentation`

---

## 📊 **Risk Assessment**

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Broken Makefile targets | Low | High | Test `make help` after changes |
| HolmesGPT API BR invalidation | Low | Medium | BRs remain valid, only service name changes |
| Data Storage integration confusion | Low | Low | Clear documentation updates |
| Missed references in other docs | Medium | Low | Final grep verification |

---

## ✅ **Success Criteria**

1. **Makefile**: Zero Context API targets, `make help` shows no Context API commands
2. **HolmesGPT API**: All BRs updated to reference Data Storage Service
3. **Data Storage**: All integration points reference HolmesGPT API directly
4. **Verification**: `grep -r "context-api\|contextapi\|context_api" docs/ Makefile` returns 0 active references (historical changelogs OK)

---

## 📚 **References**

- **Context API Deletion Summary**: `docs/services/stateless/context-api/CONTEXT-API-DELETION-SUMMARY.md`
- **Context API BR Migration**: `docs/services/stateless/context-api/CONTEXT-API-BR-MIGRATION-TRIAGE.md`
- **Authoritative Docs Triage**: `docs/architecture/CONTEXT-API-AUTHORITATIVE-DOCS-TRIAGE.md`
- **Data Storage V1.0 Plan**: `docs/services/stateless/data-storage/implementation/DATA-STORAGE-V1.0-MVP-IMPLEMENTATION-PLAN.md`

---

**Confidence**: 95% - Clear scope, low risk, straightforward replacements

