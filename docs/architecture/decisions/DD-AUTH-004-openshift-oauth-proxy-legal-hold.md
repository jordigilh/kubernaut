# DD-AUTH-004: OpenShift OAuth-Proxy for SOC2 Legal Hold Authentication

**Date**: January 7, 2026  
**Status**: ✅ **APPROVED** - Authoritative V1.0 Pattern  
**Builds On**: DD-AUTH-003 (Externalized Authorization Sidecar)  
**Confidence**: 95%  
**Last Reviewed**: January 7, 2026

---

## 🎯 **DECISION**

**The Data Storage service SHALL use OpenShift oauth-proxy as a sidecar to authenticate legal hold operations (POST/DELETE `/api/v1/audit/legal-hold`), while keeping read operations unauthenticated. Handlers SHALL extract user identity from `X-Auth-Request-User` header and implement defense-in-depth validation (401 if header missing).**

**Scope**:
- Data Storage service (SOC2 Gap #8 legal hold endpoints)
- POST `/api/v1/audit/legal-hold` (place hold)
- DELETE `/api/v1/audit/legal-hold/{correlation_id}` (release hold)
- GET `/api/v1/audit/legal-hold` (list holds - NO AUTH)

**Pattern**: OpenShift-native zero-trust authentication with Subject Access Review (SAR)

---

## 📊 **Context & Problem**

### **Business Requirements**

1. **SOC2 CC6.1 (Access Controls)**: Legal hold operations require **unique user identification**
2. **SOC2 CC7.2 (System Monitoring)**: Multiple layers of access controls (defense-in-depth)
3. **SOX/HIPAA Compliance**: Audit trail of WHO placed/released legal holds
4. **Enterprise Adoption**: Use OpenShift-native auth (not external OAuth providers)
5. **Testing Simplicity**: Integration tests mock headers, E2E tests use real oauth-proxy

### **Problem Statement**

**Current Implementation (X-User-ID header)**:
- Handlers accept `X-User-ID` header from any caller
- No authentication enforcement (trust-based model)
- Suitable for integration tests, NOT for production

**Why This Doesn't Work for Production**:
- ❌ **No Authentication**: Any client can set X-User-ID header
- ❌ **SOC2 Failure**: No cryptographic proof of user identity
- ❌ **Audit Trail Weakness**: Cannot prove WHO actually performed action
- ❌ **Enterprise Security Gap**: No integration with identity providers

**SOC2 Auditor Question**: "How do you prove that `legal_hold_placed_by='alice@company.com'` was actually Alice, not an attacker?"

**Answer**: OpenShift oauth-proxy validates JWT tokens, performs Subject Access Review (SAR), and injects trusted headers.

---

## 🔍 **Alternatives Considered**

### **Alternative 1: Application-Level JWT Validation** ❌ REJECTED

**Approach**: Data Storage service validates JWT tokens directly

**Why Rejected** (from DD-AUTH-003):
- ❌ Auth logic in business code (violates separation of concerns)
- ❌ Testing complexity (need JWT mocking in unit tests)
- ❌ Hard to change (auth updates require service redeploy)

**Confidence**: 95% rejection (superseded by DD-AUTH-003)

---

### **Alternative 2: Generic OAuth2-Proxy** ⚠️ PARTIAL SOLUTION

**Approach**: Use `quay.io/oauth2-proxy/oauth2-proxy` (generic OAuth provider)

**Pros**:
- ✅ Supports multiple OAuth providers (GitHub, Google, etc.)
- ✅ Well-documented, widely used

**Cons**:
- ❌ **Not OpenShift-native**: Doesn't integrate with OpenShift RBAC
- ❌ **External dependencies**: Requires external OAuth provider (GitHub, Auth0, etc.)
- ❌ **No SAR support**: Cannot check OpenShift permissions
- ❌ **Enterprise adoption barrier**: Companies prefer native K8s/OpenShift auth

**Why Rejected**: Not suitable for enterprise K8s environments that expect native auth

**Confidence**: 80% rejection (external dependencies)

---

### **Alternative 3: OpenShift oauth-proxy with SAR** ✅ APPROVED

**Approach**: Use `quay.io/openshift/oauth-proxy` with zero-configuration OpenShift auth

**Source**: https://github.com/openshift/oauth-proxy/blob/master/README.md

**Pros**:
- ✅ **Zero-configuration**: Reads ServiceAccount token automatically
- ✅ **OpenShift-native**: Integrates with OpenShift OAuth server
- ✅ **SAR support**: Validates user has required permissions
- ✅ **No external dependencies**: Uses in-cluster OAuth
- ✅ **Enterprise-ready**: Standard OpenShift authentication pattern
- ✅ **Kubernetes-compatible**: Works on vanilla K8s with service accounts
- ✅ **Defense-in-depth**: Network policy + sidecar + handler validation

**Cons**:
- ⚠️ **OpenShift-specific**: Optimized for OpenShift (but works on vanilla K8s)
- ⚠️ **Additional sidecar**: ~50MB memory per pod
- ⚠️ **Configuration per service**: Each service needs sidecar config

**Why Approved**: Best balance of security, enterprise adoption, and simplicity

**Confidence**: 95% approval (proven OpenShift pattern)

---

## 🏗️ **Implementation Architecture**

### **Component 1: OpenShift oauth-proxy Sidecar**

```yaml
# deploy/datastorage/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: kubernaut
spec:
  replicas: 2
  selector:
    matchLabels:
      app: datastorage
  template:
    metadata:
      labels:
        app: datastorage
    spec:
      serviceAccountName: datastorage
      containers:
      # ========================================
      # MAIN APPLICATION CONTAINER
      # ========================================
      - name: datastorage
        image: quay.io/jordigilh/kubernaut-datastorage:v1.0.0
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        env:
        # Trust headers from localhost (sidecar only)
        - name: TRUSTED_PROXY_ENABLED
          value: "true"
        - name: TRUSTED_PROXY_CIDRS
          value: "127.0.0.1/32"
        # Database configuration
        - name: POSTGRES_HOST
          value: postgresql
        - name: POSTGRES_PORT
          value: "5432"
        - name: POSTGRES_DB
          value: datastorage
        - name: POSTGRES_USER
          value: datastorage
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: datastorage-postgresql
              key: password
        # Redis configuration
        - name: REDIS_HOST
          value: redis
        - name: REDIS_PORT
          value: "6379"
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10

      # ========================================
      # OPENSHIFT OAUTH-PROXY SIDECAR
      # SOC2 Gap #8: Legal Hold Authentication
      # ========================================
      - name: oauth-proxy
        image: quay.io/openshift/oauth-proxy:latest
        ports:
        - containerPort: 8443
          name: proxy
          protocol: TCP
        args:
        # ========================================
        # PROVIDER CONFIGURATION
        # ========================================
        - --provider=openshift
        - --openshift-service-account=datastorage
        - --openshift-ca=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt

        # ========================================
        # UPSTREAM CONFIGURATION
        # ========================================
        - --upstream=http://localhost:8080
        - --https-address=:8443
        - --http-address=

        # ========================================
        # AUTHORIZATION: Subject Access Review (SAR)
        # Requires user to have 'update' permission on 'services/datastorage' resource
        # This allows cluster admins to control legal hold access via RBAC
        # ========================================
        - --openshift-sar={"namespace":"kubernaut","resource":"services","resourceName":"datastorage","verb":"update"}

        # ========================================
        # SKIP AUTHENTICATION FOR READ-ONLY ENDPOINTS
        # Legal hold GET (list) does not require authentication
        # All other audit endpoints (batch write, query) do not require authentication
        # ONLY POST/DELETE /api/v1/audit/legal-hold require authentication
        # ========================================
        - --skip-auth-regex=^/(healthz|metrics|api/v1/audit/(?!legal-hold$|legal-hold/)).*

        # ========================================
        # HEADER INJECTION
        # Inject X-Auth-Request-User and X-Auth-Request-Email headers
        # DataStorage handlers will extract user identity from these headers
        # ========================================
        - --set-xauthrequest=true

        # ========================================
        # SECURITY CONFIGURATION
        # ========================================
        - --cookie-secret=$(OAUTH2_COOKIE_SECRET)
        - --cookie-secure=true
        - --cookie-samesite=lax
        - --cookie-httponly=true

        # TLS configuration (OpenShift service serving certs)
        - --tls-cert=/etc/tls/private/tls.crt
        - --tls-key=/etc/tls/private/tls.key

        env:
        - name: OAUTH2_COOKIE_SECRET
          valueFrom:
            secretKeyRef:
              name: datastorage-oauth-proxy
              key: cookie-secret

        volumeMounts:
        # OpenShift service serving certificates (automatic TLS)
        - mountPath: /etc/tls/private
          name: tls-cert
          readOnly: true

        resources:
          requests:
            memory: "50Mi"
            cpu: "50m"
          limits:
            memory: "100Mi"
            cpu: "200m"

      volumes:
      # OpenShift service serving certificate (automatic TLS)
      - name: tls-cert
        secret:
          secretName: datastorage-tls
          defaultMode: 0640
```

**Key Configuration Explained**:

1. **`--provider=openshift`**: Use OpenShift OAuth server (zero-configuration)
2. **`--openshift-service-account=datastorage`**: Use ServiceAccount token for authentication
3. **`--openshift-sar={...}`**: Require user to have `update` permission on `services/datastorage`
4. **`--skip-auth-regex=^/(healthz|metrics|api/v1/audit/(?!legal-hold$|legal-hold/)).*`**:
   - Skip auth for: `/healthz`, `/metrics`, `/api/v1/audit/events`, `/api/v1/audit/query`
   - Require auth for: POST `/api/v1/audit/legal-hold`, DELETE `/api/v1/audit/legal-hold/{id}`
   - **Regex breakdown**: Match everything EXCEPT exact `/api/v1/audit/legal-hold` endpoints
5. **`--set-xauthrequest=true`**: Inject `X-Auth-Request-User` and `X-Auth-Request-Email` headers
6. **`--upstream=http://localhost:8080`**: Forward authenticated requests to main app

---

### **Component 2: Network Policy (Critical Security Layer)**

```yaml
# deploy/datastorage/network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: datastorage-auth-enforcement
  namespace: kubernaut
spec:
  podSelector:
    matchLabels:
      app: datastorage
  policyTypes:
  - Ingress

  ingress:
  # ========================================
  # RULE 1: External traffic MUST use sidecar port (8443)
  # This is the ONLY port exposed to other pods/services
  # ========================================
  - from:
    - namespaceSelector: {}  # From any namespace
    - podSelector: {}        # From any pod
    ports:
    - protocol: TCP
      port: 8443  # oauth-proxy sidecar port (authenticated)

  # ========================================
  # RULE 2: Application port (8080) is NOT exposed externally
  # Only accessible from localhost (sidecar)
  # Pod-local networking enforces this (127.0.0.1)
  # ========================================
  # (No rule for port 8080 = external access blocked)
```

**Security Guarantee**:
- ✅ External traffic CANNOT bypass oauth-proxy (port 8080 not exposed)
- ✅ Only sidecar (localhost) can reach application
- ✅ Network policy enforces single entry point
- ✅ Defense-in-depth: Network + Sidecar + Handler validation

---

### **Component 3: Handler Logic (Defense-in-Depth)**

```go
// pkg/datastorage/server/legal_hold_handler.go

// PlaceLegalHold handles POST /api/v1/audit/legal-hold
// SOC2 Gap #8: Legal Hold Placement with User Attribution
//
// SECURITY: Defense-in-depth validation
// - Layer 1: oauth-proxy validates JWT and SAR (returns 401 if unauthorized)
// - Layer 2: Handler validates X-Auth-Request-User is present (this function)
// - Layer 3: Network policy enforces sidecar path
//
// DD-AUTH-004: OpenShift oauth-proxy sidecar authentication
func (s *Server) PlaceLegalHold(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1. Parse request body
	var req PlaceLegalHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.metrics.LegalHoldFailures.WithLabelValues("invalid_request").Inc()
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-request", "Invalid Request",
			fmt.Sprintf("Invalid request body: %v", err), s.logger)
		return
	}

	// 2. Validate required fields
	if req.CorrelationID == "" || req.Reason == "" {
		s.metrics.LegalHoldFailures.WithLabelValues("missing_required_field").Inc()
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-required-field", "Missing Required Field",
			"correlation_id and reason are required", s.logger)
		return
	}

	// ========================================
	// 3. DEFENSE-IN-DEPTH: Extract and validate user identity
	//    Primary source: X-Auth-Request-User (injected by oauth-proxy)
	// ========================================
	// SOC2 CC7.2: Multiple layers of access controls
	//
	// This validation should NEVER fail if oauth-proxy is correctly configured.
	// If it does fail, it indicates:
	// - oauth-proxy misconfiguration (--skip-auth-regex typo)
	// - oauth-proxy failure (sidecar crashed)
	// - Security bypass attempt (direct access to port 8080)
	//
	// DD-AUTH-004: Trust but verify - oauth-proxy should have validated,
	// but we check again as defense-in-depth (SOC2 CC7.2 requirement)
	// ========================================
	placedBy := r.Header.Get("X-Auth-Request-User")
	if placedBy == "" {
		// CRITICAL SECURITY EVENT: User header missing
		// This should NEVER happen in production if oauth-proxy is working
		s.metrics.LegalHoldFailures.WithLabelValues("missing_user_header").Inc()
		s.logger.Error(nil, "SECURITY: Missing X-Auth-Request-User header - oauth-proxy bypass detected",
			"remote_addr", r.RemoteAddr,
			"user_agent", r.Header.Get("User-Agent"),
			"correlation_id", req.CorrelationID)
		response.WriteRFC7807Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized",
			"User authentication required for legal hold operations", s.logger)
		return
	}

	// 4. Optional: Extract email for enriched audit logging (future enhancement)
	// email := r.Header.Get("X-Auth-Request-Email")

	// 5. Check if correlation_id exists
	var eventCount int
	checkQuery := `SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1`
	err := s.db.QueryRowContext(ctx, checkQuery, req.CorrelationID).Scan(&eventCount)
	if err != nil {
		s.logger.Error(err, "Failed to check correlation_id existence", "correlation_id", req.CorrelationID)
		s.metrics.LegalHoldFailures.WithLabelValues("db_error").Inc()
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to validate correlation ID", s.logger)
		return
	}

	if eventCount == 0 {
		s.metrics.LegalHoldFailures.WithLabelValues("missing_correlation_id").Inc()
		response.WriteRFC7807Error(w, http.StatusNotFound, "correlation-id-not-found", "Correlation ID Not Found",
			fmt.Sprintf("No audit events found for correlation_id: %s", req.CorrelationID), s.logger)
		return
	}

	// 6. Place legal hold on all events with correlation_id
	placedAt := time.Now()
	updateQuery := `
		UPDATE audit_events
		SET legal_hold = TRUE,
		    legal_hold_reason = $1,
		    legal_hold_placed_by = $2,
		    legal_hold_placed_at = $3
		WHERE correlation_id = $4
	`
	result, err := s.db.ExecContext(ctx, updateQuery, req.Reason, placedBy, placedAt, req.CorrelationID)
	if err != nil {
		s.logger.Error(err, "Failed to place legal hold", "correlation_id", req.CorrelationID)
		s.metrics.LegalHoldFailures.WithLabelValues("db_error").Inc()
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to place legal hold", s.logger)
		return
	}

	rowsAffected, _ := result.RowsAffected()

	// 7. Success response
	s.metrics.LegalHoldSuccesses.WithLabelValues("place").Inc()
	s.logger.Info("Legal hold placed successfully",
		"correlation_id", req.CorrelationID,
		"placed_by", placedBy,  // ← Username from oauth-proxy (SOC2 attribution)
		"events_affected", rowsAffected,
		"reason", req.Reason)

	resp := PlaceLegalHoldResponse{
		CorrelationID:   req.CorrelationID,
		EventsAffected:  int(rowsAffected),
		PlacedBy:        placedBy,  // ← Authenticated user from oauth-proxy
		PlacedAt:        placedAt.Format(time.RFC3339),
		Reason:          req.Reason,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
```

**Key Security Points**:
1. **Layer 1 (Network)**: Network policy blocks direct access to port 8080
2. **Layer 2 (Sidecar)**: oauth-proxy validates JWT and SAR before forwarding
3. **Layer 3 (Handler)**: Handler validates `X-Auth-Request-User` is present (defense-in-depth)
4. **SOC2 CC7.2**: Multiple independent layers of controls

---

## 🧪 **Testing Strategy**

### **Unit Tests (70%+ Coverage)** ✅ NO CHANGES NEEDED

Unit tests already pass `X-User-ID` header directly - no authentication needed.

```go
// test/unit/datastorage/legal_hold_test.go
// NO CHANGES NEEDED - unit tests mock headers directly
```

---

### **Integration Tests** ✅ MOCK HEADERS

```go
// test/integration/datastorage/legal_hold_integration_test.go

var _ = Describe("SOC2 Gap #8: Legal Hold Integration Tests", func() {
	BeforeEach(func() {
		// ... existing setup ...
	})

	It("should place legal hold with authenticated user", func() {
		// Create audit events
		auditRepo := repository.NewAuditEventsRepository(db.DB, logger)
		// ... create events ...

		// ========================================
		// INTEGRATION TEST: Mock X-Auth-Request-User header
		// (oauth-proxy not running in integration tests)
		// ========================================
		req, err := http.NewRequest("POST", datastorageURL+"/api/v1/audit/legal-hold", strings.NewReader(`{
			"correlation_id": "`+correlationID+`",
			"reason": "Integration test legal hold"
		}`))
		Expect(err).ToNot(HaveOccurred())

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Auth-Request-User", "test-operator@kubernaut.ai")  // ← Mock header

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// Verify placed_by field
		var legalHoldResp PlaceLegalHoldResponse
		json.NewDecoder(resp.Body).Decode(&legalHoldResp)
		Expect(legalHoldResp.PlacedBy).To(Equal("test-operator@kubernaut.ai"))
	})

	It("should return 401 if X-Auth-Request-User header is missing", func() {
		// ========================================
		// DEFENSE-IN-DEPTH TEST: Verify handler rejects missing header
		// ========================================
		req, err := http.NewRequest("POST", datastorageURL+"/api/v1/audit/legal-hold", strings.NewReader(`{
			"correlation_id": "test-correlation-id",
			"reason": "Test"
		}`))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		// NO X-Auth-Request-User header

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})
})
```

**Integration Test Pattern**: Mock `X-Auth-Request-User` header directly (no oauth-proxy)

---

### **E2E Tests** ✅ REAL OAUTH-PROXY SIDECAR

```go
// test/e2e/datastorage/legal_hold_e2e_test.go

var _ = Describe("SOC2 Gap #8: Legal Hold E2E with OAuth-Proxy", func() {
	It("should authenticate via oauth-proxy sidecar", func() {
		// ========================================
		// E2E TEST: Real oauth-proxy sidecar running in Kind cluster
		// ========================================

		// Get ServiceAccount token (simulates authenticated user)
		token := getServiceAccountToken("datastorage-test-user")

		// Connect to external port (oauth-proxy: 8443)
		req, err := http.NewRequest("POST", "https://datastorage.kubernaut.svc.cluster.local:8443/api/v1/audit/legal-hold", strings.NewReader(`{
			"correlation_id": "e2e-test-correlation-id",
			"reason": "E2E test legal hold"
		}`))
		Expect(err).ToNot(HaveOccurred())

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)  // ← Real JWT token

		// oauth-proxy will:
		// 1. Validate JWT token
		// 2. Perform Subject Access Review (check RBAC permissions)
		// 3. Inject X-Auth-Request-User header
		// 4. Forward to DataStorage service

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// Verify audit trail has correct user
		events := queryAuditEvents("e2e-test-correlation-id")
		Expect(events[0].LegalHoldPlacedBy).To(ContainSubstring("datastorage-test-user"))
	})
})
```

**E2E Test Pattern**: Real oauth-proxy sidecar with JWT token validation

---

## ✅ **Consequences**

### **Positive Impacts**

1. **SOC2 CC6.1 Compliance** ✅
   - Cryptographic proof of user identity
   - Cannot forge user attribution
   - Audit trail integrity guaranteed

2. **Enterprise-Ready Authentication** ✅
   - OpenShift-native pattern (no external OAuth providers)
   - Integrates with existing RBAC
   - Subject Access Review (SAR) for authorization

3. **Defense-in-Depth (SOC2 CC7.2)** ✅
   - Network policy (Layer 1)
   - oauth-proxy validation (Layer 2)
   - Handler validation (Layer 3)
   - Observable security incidents via metrics

4. **Simple Testing** ✅
   - Integration tests: Mock headers (no oauth-proxy)
   - E2E tests: Real oauth-proxy sidecar
   - No changes to unit tests

5. **Zero Application Auth Code** ✅
   - Just read `X-Auth-Request-User` header
   - No JWT decoding, no token validation
   - Clean separation of concerns

---

### **Negative Impacts** (Mitigated)

1. **Additional Sidecar Memory** ⚠️
   - **Impact**: ~50MB per DataStorage pod
   - **Mitigation**: Negligible (1% of typical 4GB pod memory)
   - **Severity**: LOW

2. **E2E Test Complexity** ⚠️
   - **Impact**: E2E tests need real oauth-proxy sidecar in Kind
   - **Mitigation**: Standard K8s pattern, reusable across services
   - **Severity**: LOW

3. **OpenShift-Specific** ⚠️
   - **Impact**: Optimized for OpenShift OAuth
   - **Mitigation**: Also works on vanilla K8s with service accounts
   - **Severity**: LOW

---

## 📊 **Implementation Checklist**

- [ ] **Task 1**: Create DataStorage K8s deployment with oauth-proxy sidecar
- [ ] **Task 2**: Create Network Policy for datastorage pod
- [ ] **Task 3**: Update handlers to extract `X-Auth-Request-User` (replace `X-User-ID`)
- [ ] **Task 4**: Update integration tests to mock `X-Auth-Request-User` header
- [ ] **Task 5**: Create E2E infrastructure to deploy oauth-proxy in Kind
- [ ] **Task 6**: Update OpenAPI spec to document authentication flow
- [ ] **Task 7**: Run integration and E2E tests to verify

---

## 🔗 **Related Decisions**

- **Builds On**: [DD-AUTH-003](mdc:docs/architecture/decisions/DD-AUTH-003-externalized-authorization-sidecar.md) (Externalized Authorization Sidecar)
- **Supports**: SOC2 Gap #8 (Legal Hold & Retention Policies)
- **Enables**: SOC2 CC6.1 (Access Controls), SOC2 CC7.2 (System Monitoring)
- **Authoritative Reference**: https://github.com/openshift/oauth-proxy/blob/master/README.md

---

## 📈 **Success Metrics**

| Metric | Target | Status |
|--------|--------|--------|
| Legal hold operations authenticated | 100% | ⬜ Not Started |
| Integration tests with mocked headers | 7/7 passing | ⬜ Not Started |
| E2E tests with real oauth-proxy | 1+ passing | ⬜ Not Started |
| Handler defense-in-depth validation | 401 on missing header | ⬜ Not Started |
| SOC2 CC6.1 compliance | Auditor approval | ⬜ Not Started |

---

**Document Status**: ✅ APPROVED  
**Implementation Status**: ⬜ NOT STARTED  
**Target V1.0**: Yes (Data Storage service)  
**Confidence**: 95%  
**SOC2 Gap**: Gap #8 (Legal Hold Authentication)

