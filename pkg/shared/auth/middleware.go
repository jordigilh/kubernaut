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

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

// ContextKey is the type for context keys used in the auth middleware.
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated user identity (string).
	UserContextKey ContextKey = "user"

	// UserInfoContextKey is the context key for the full authenticated user info
	// including group memberships. Required for interactive MCP sessions (#703)
	// where impersonation needs both username and groups.
	UserInfoContextKey ContextKey = "userInfo"
)

// failureReasonKey is an unexported context key for the mutable holder
// WithFailureReasonCapture installs. Unexported so only this package can
// write to it (via emitSecurityEvent); consumers only ever see it through
// the accessor function WithFailureReasonCapture returns.
type failureReasonKey struct{}

// WithFailureReasonCapture returns a derived context carrying a mutable
// recorder, plus an accessor that returns whatever security-event reason
// (e.g. "invalid_token_audience", "missing_auth_header",
// "authorization_denied") Handler classified for the request, once it
// returns. Empty if the request succeeded or Handler was never invoked with
// the returned context.
//
// BR-SECURITY-1900 (AU-3, extending BR-AUDIT-005): an outer HTTP wrapper
// that persists audit-table events by observing only the response status
// code (e.g. KubernautAgent's AuditAuthMiddleware) cannot otherwise tell an
// audience-bound TokenReview mismatch apart from a routine
// missing/malformed/expired token -- both produce an identical 401. Callers
// must derive the request's context from the value returned here (e.g. via
// r.WithContext) before invoking Handler, then call the accessor after
// Handler returns.
func WithFailureReasonCapture(ctx context.Context) (context.Context, func() string) {
	holder := new(string)
	return context.WithValue(ctx, failureReasonKey{}, holder), func() string { return *holder }
}

// Middleware provides authentication and authorization for HTTP requests.
//
// Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)
//
// This middleware implements a secure, testable auth framework using dependency injection:
// 1. Extracts Bearer token from Authorization header
// 2. Validates token using Authenticator interface (TokenReview)
// 3. Checks authorization using Authorizer interface (SAR)
// 4. Injects user identity into request context (for audit logging)
// 5. Injects X-Auth-Request-User header (for SOC2 user attribution)
//
// Security: No runtime disable flags — auth is always enforced via interface implementations.
//
// Used by: DataStorage service (BR-DS-*), Gateway service (BR-GATEWAY-036/037)
type Middleware struct {
	authenticator Authenticator
	authorizer    Authorizer
	config        MiddlewareConfig
	logger        logr.Logger
}

// MiddlewareConfig contains the SAR configuration for authorization checks.
type MiddlewareConfig struct {
	// Namespace is the Kubernetes namespace for the SAR check
	Namespace string

	// Resource is the Kubernetes resource type (e.g., "services", "pods")
	Resource string

	// ResourceName is the specific resource name (e.g., "data-storage-service", "gateway-service")
	ResourceName string

	// Verb is the RBAC verb to check (e.g., "create", "get", "list", "update", "delete")
	// Authority: DD-AUTH-011 (Granular RBAC with SAR verb mapping)
	Verb string
}

// NewMiddleware creates a new authentication middleware with dependency injection.
//
// Example (Production):
//
//	k8sClient, _ := kubernetes.NewForConfig(config)
//	authenticator := auth.NewK8sAuthenticator(k8sClient)
//	authorizer := auth.NewK8sAuthorizer(k8sClient)
//	mw := auth.NewMiddleware(
//	    authenticator,
//	    authorizer,
//	    auth.MiddlewareConfig{
//	        Namespace:    "kubernaut-system",
//	        Resource:     "services",
//	        ResourceName: "gateway-service",
//	        Verb:         "create",
//	    },
//	    logger,
//	)
func NewMiddleware(
	authenticator Authenticator,
	authorizer Authorizer,
	config MiddlewareConfig,
	logger logr.Logger,
) *Middleware {
	return &Middleware{
		authenticator: authenticator,
		authorizer:    authorizer,
		config:        config,
		logger:        logger.WithName("auth-middleware"),
	}
}

// Handler returns a chi-compatible middleware handler.
//
// HTTP Status Codes (Authority: DD-AUTH-013):
// - 401 Unauthorized: Missing/invalid/expired token
// - 403 Forbidden: Valid token but insufficient RBAC permissions
// - 500 Internal Server Error: TokenReview/SAR API call failures
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Issue #673 H-2: Strip client-supplied identity header before authentication.
		// This header is a SOC2 trust anchor used by downstream services (DataStorage).
		r.Header.Del("X-Auth-Request-User")

		// Issue #703: Strip Kubernetes impersonation headers to prevent privilege escalation.
		stripImpersonationHeaders(r)

		userInfo, ok := m.authenticateRequest(w, r)
		if !ok {
			return
		}
		user := userInfo.Username

		if !m.authorizeRequest(w, r, user) {
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		ctx = context.WithValue(ctx, UserInfoContextKey, userInfo)
		r.Header.Set("X-Auth-Request-User", user)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authenticateRequest extracts the Bearer token from r and validates it via
// m.authenticator. On failure it writes the appropriate error response (401
// for missing/malformed/invalid tokens, 500 for authenticator failures) and
// returns ok=false; the caller must return immediately without proceeding.
func (m *Middleware) authenticateRequest(w http.ResponseWriter, r *http.Request) (userInfo UserInfo, ok bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		m.logAuthenticationFailure(r, "missing_auth_header")
		m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing Authorization header")
		return UserInfo{}, false
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		m.logAuthenticationFailure(r, "invalid_auth_format")
		m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Invalid Authorization header format")
		return UserInfo{}, false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		m.logAuthenticationFailure(r, "empty_bearer_token")
		m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Empty Bearer token")
		return UserInfo{}, false
	}

	userInfo, err := m.authenticator.ValidateTokenFull(r.Context(), token)
	if err != nil {
		if errors.Is(err, ErrTokenInvalid) {
			m.logAuthenticationFailure(r, invalidTokenReason(err))
			m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Invalid or expired token")
			return UserInfo{}, false
		}
		m.logger.Error(err, "Token validation failed",
			"path", r.URL.Path,
			"method", r.Method,
		)
		// Issue #2664 (AU-2/AU-3): a TokenReview outage is a security-relevant,
		// fail-closed decision and must be auditable, not only logged as an error.
		m.logAuthenticationOutage(r, err)
		// Issue #673 C-2: Generic error message; details logged server-side only
		m.writeError(w, http.StatusInternalServerError, "Internal Server Error", "Authentication service unavailable")
		return UserInfo{}, false
	}

	m.logger.V(2).Info("Token validated",
		"user", userInfo.Username,
		"providerType", userInfo.ProviderType,
		"path", r.URL.Path,
	)
	return userInfo, true
}

// authorizeRequest performs the SAR authorization check for user via
// m.authorizer. On failure or denial it writes the appropriate error response
// (500 for authorizer failures, 403 for denial) and returns false; the caller
// must return immediately without proceeding.
func (m *Middleware) authorizeRequest(w http.ResponseWriter, r *http.Request, user string) bool {
	allowed, err := m.authorizer.CheckAccess(
		r.Context(),
		user,
		m.config.Namespace,
		m.config.Resource,
		m.config.ResourceName,
		m.config.Verb,
	)
	if err != nil {
		m.logger.Error(err, "Authorization check failed",
			"user", user,
			"path", r.URL.Path,
		)
		// Issue #2664 (AU-2/AU-3): a SAR outage is a security-relevant,
		// fail-closed decision and must be auditable, not only logged as an error.
		m.logAuthorizationOutage(r, user, err)
		// Issue #673 C-2: Generic error message; details logged server-side only
		m.writeError(w, http.StatusInternalServerError, "Internal Server Error", "Authorization service unavailable")
		return false
	}

	if !allowed {
		m.logAuthorizationDenial(r, user)
		m.writeError(w, http.StatusForbidden, "Forbidden", "Insufficient permissions")
		return false
	}

	m.logger.V(2).Info("Authorization granted",
		"user", user,
		"path", r.URL.Path,
	)
	m.logSecurityGrant(r, user)
	return true
}

// GetUserFromContext extracts the authenticated user identity from the request context.
// Returns empty string if the context is nil or the user is not in context.
func GetUserFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if user, ok := ctx.Value(UserContextKey).(string); ok {
		return user
	}
	return ""
}

// GetUserInfoFromContext extracts the full authenticated user info (username + groups)
// from the request context. Returns zero-value UserInfo if not present.
// SEC-CRIT-02 (#703): Required for impersonation to propagate group memberships.
func GetUserInfoFromContext(ctx context.Context) UserInfo {
	if ctx == nil {
		return UserInfo{}
	}
	if info, ok := ctx.Value(UserInfoContextKey).(UserInfo); ok {
		return info
	}
	return UserInfo{}
}

// stripImpersonationHeaders removes all Kubernetes impersonation headers from the
// request. This prevents clients from injecting Impersonate-User/Group/Uid/Extra-*
// headers that could escalate privileges when KA constructs impersonating K8s
// clients (#703, #895). Covers KEP-1513 Impersonate-Uid (K8s 1.22+).
func stripImpersonationHeaders(r *http.Request) {
	r.Header.Del("Impersonate-User")
	r.Header.Del("Impersonate-Group")
	r.Header.Del("Impersonate-Uid")
	for key := range r.Header {
		if strings.HasPrefix(strings.ToLower(key), "impersonate-extra-") {
			r.Header.Del(key)
		}
	}
}

// invalidTokenReason classifies an ErrTokenInvalid failure for the
// security_event audit log (BR-SECURITY-1900, AU-3/CC7.2) using an
// "audience mismatch" substring convention, so an authenticator that wraps a
// cross-service token-replay detection in ErrTokenInvalid this way is
// distinguishable in audit review from a routine expired/malformed/
// unauthenticated token, without introducing a second sentinel error that
// callers would need to match on. No authenticator anywhere in the codebase
// currently produces this substring: audience-bound TokenReview validation
// was implemented for both KA and AF under #1900, then fully reverted for
// both (see BR-SECURITY-1900 -- KA's real AF traffic flows exclusively
// through the dual-purpose MCP endpoint that DD-AUTH-MCP-001 requires to
// also serve audience-unaware direct in-cluster clients, and AF's variant
// was descoped as defense-in-depth of unclear incremental value given SAR
// already denies unauthorized tool access regardless of the authentication
// layer). This classifier ships anyway as generic, forward-compatible
// plumbing for any future authenticator that adopts the convention; the
// tests exercising "invalid_token_audience" (pkg/shared/auth) construct a
// synthetic error string by hand for exactly this reason -- there is no
// live authenticator to exercise it against today.
func invalidTokenReason(err error) string {
	if strings.Contains(err.Error(), "audience mismatch") {
		return "invalid_token_audience"
	}
	return "invalid_token"
}

// ADR-034 event_type values used by the shared auth middleware.
const (
	securityEventTypeAuthentication = "authentication"
	securityEventTypeAuthorization  = "authorization"
)

// securityEvent is one ADR-034 security decision to record. It carries only
// what differs between the decision kinds; emitSecurityEvent owns the common
// field shape.
type securityEvent struct {
	reason       string
	user         string
	statusCode   int
	outcome      string
	eventType    string
	action       string
	errorDetails *SecurityErrorDetails
}

// logAuthenticationFailure records a request rejected before identity was
// established (401). FedRAMP AU-2: every rejected request is traceable.
func (m *Middleware) logAuthenticationFailure(r *http.Request, reason string) {
	m.emitSecurityEvent(r, securityEvent{
		reason:     reason,
		statusCode: http.StatusUnauthorized,
		outcome:    "failure",
	})
}

// logAuthorizationDenial records an authenticated user denied by SAR (403).
func (m *Middleware) logAuthorizationDenial(r *http.Request, user string) {
	m.emitSecurityEvent(r, securityEvent{
		reason:     "authorization_denied",
		user:       user,
		statusCode: http.StatusForbidden,
		outcome:    "failure",
	})
}

// logAuthenticationOutage records a fail-closed TokenReview dependency outage
// (500) with DD-ERROR-001 error_details. No identity was established.
func (m *Middleware) logAuthenticationOutage(r *http.Request, cause error) {
	const reason = "authentication_service_unavailable"
	details := newSecurityErrorDetails(reason, cause)
	m.emitSecurityEvent(r, securityEvent{
		reason:       reason,
		statusCode:   http.StatusInternalServerError,
		outcome:      "failure",
		errorDetails: &details,
	})
}

// logAuthorizationOutage records a fail-closed SAR dependency outage (500) with
// DD-ERROR-001 error_details; the authenticated user is already known.
func (m *Middleware) logAuthorizationOutage(r *http.Request, user string, cause error) {
	const reason = "authorization_service_unavailable"
	details := newSecurityErrorDetails(reason, cause)
	m.emitSecurityEvent(r, securityEvent{
		reason:       reason,
		user:         user,
		statusCode:   http.StatusInternalServerError,
		outcome:      "failure",
		errorDetails: &details,
	})
}

// logSecurityGrant emits the positive authorization decision required by
// BR-SECURITY-016 ("every authorization decision (grant or deny) is observable
// as a structured security-event log entry", FedRAMP AU-2).
//
// Volume: one Info line per authorization decision (SAR check), i.e. one per
// authorized read request and two per DS write/mutate request because those
// routes run both the base "get" and the route-level "create"/"update"
// middleware. That roughly doubles the per-request log line count of a service
// that already writes an access-log line per request.
//
// This method applies no sampling and no level gate of its own, but it logs
// through the host service's logger: that logger's configured level and zap
// sampling still apply (zap's production config samples repeated message keys).
// Guaranteeing delivery under that configuration requires a dedicated
// unsampled security-event logger in the service wiring -- a host-logger
// change outside this middleware, tracked in issue #2694 rather than done here.
// What this middleware guarantees is that every decision calls the emitter.
//
// The event is emitted as soon as the SAR check allows the request, before the
// downstream handler runs: status_code=200 records the authorization decision
// (allowed), not the eventual HTTP response, which the handler may still turn
// into a 4xx/5xx.
func (m *Middleware) logSecurityGrant(r *http.Request, user string) {
	m.emitSecurityEvent(r, securityEvent{
		reason:     "authorization_granted",
		user:       user,
		statusCode: http.StatusOK,
		outcome:    "success",
		eventType:  securityEventTypeAuthorization,
		action:     "grant",
	})
}

// SecurityEventRecord is the typed form of the security_event log record.
// Its fields mirror the subset of pkg/audit.AuditEvent (ADR-034, DD-AUDIT-002)
// that this middleware emits on the log channel: event identity, action,
// outcome, actor and resource context. Keeping one struct means field names
// and types are refactorable and cannot drift between call sites.
type SecurityEventRecord struct {
	EventID       string                `json:"event_id"`
	EventType     string                `json:"event_type"`
	EventAction   string                `json:"event_action"`
	EventOutcome  string                `json:"event_outcome"`
	Reason        string                `json:"reason"`
	CorrelationID string                `json:"correlation_id"`
	ActorType     string                `json:"actor_type"`
	ActorID       string                `json:"actor_id"`
	User          string                `json:"user"`
	StatusCode    int                   `json:"status_code"`
	Method        string                `json:"method"`
	Path          string                `json:"path"`
	RemoteAddr    string                `json:"remote_addr"`
	Resource      string                `json:"resource"`
	ResourceName  string                `json:"resource_name"`
	Verb          string                `json:"verb"`
	ErrorDetails  *SecurityErrorDetails `json:"error_details,omitempty"`
}

// logFields expands the record into logr key/value pairs. This is the single
// place the wire field names are written.
func (r SecurityEventRecord) logFields() []interface{} {
	fields := []interface{}{
		"event_id", r.EventID,
		"event_type", r.EventType,
		"event_action", r.EventAction,
		"event_outcome", r.EventOutcome,
		"reason", r.Reason,
		"correlation_id", r.CorrelationID,
		"actor_type", r.ActorType,
		"actor_id", r.ActorID,
		"user", r.User,
		"status_code", r.StatusCode,
		"method", r.Method,
		"path", r.Path,
		"remote_addr", r.RemoteAddr,
		"resource", r.Resource,
		"resource_name", r.ResourceName,
		"verb", r.Verb,
	}
	// DD-ERROR-001 / DD-AUDIT-003: error events carry standardized
	// error_details. Denials and grants have no dependency error and omit it.
	if r.ErrorDetails != nil {
		fields = append(fields, "error_details", *r.ErrorDetails)
	}
	return fields
}

// emitSecurityEvent writes one structured security_event record. event_type and
// event_action are derived from the reason when a caller does not set them
// explicitly, so a fail-closed outage is never reported as a denial.
func (m *Middleware) emitSecurityEvent(r *http.Request, evt securityEvent) {
	if evt.outcome == "failure" {
		if holder, ok := r.Context().Value(failureReasonKey{}).(*string); ok {
			*holder = evt.reason
		}
	}
	eventType, action := evt.eventType, evt.action
	if eventType == "" {
		eventType, action = securityEventClassification(evt.reason)
	}
	record := SecurityEventRecord{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		EventAction:   action,
		EventOutcome:  evt.outcome,
		Reason:        evt.reason,
		CorrelationID: requestCorrelationID(r),
		ActorType:     securityActorType(evt.user),
		ActorID:       evt.user,
		User:          evt.user,
		StatusCode:    evt.statusCode,
		Method:        r.Method,
		Path:          r.URL.Path,
		RemoteAddr:    r.RemoteAddr,
		Resource:      m.config.Resource,
		ResourceName:  m.config.ResourceName,
		Verb:          m.config.Verb,
		ErrorDetails:  evt.errorDetails,
	}
	m.logger.Info("security_event", record.logFields()...)
}

// ParseSecurityEventRecord decodes a captured security_event log payload
// (flat logr key/value fields) into the typed record, so tests and downstream
// consumers assert against the schema instead of string keys.
func ParseSecurityEventRecord(fields map[string]interface{}) (SecurityEventRecord, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return SecurityEventRecord{}, fmt.Errorf("marshal security event fields: %w", err)
	}
	var record SecurityEventRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return SecurityEventRecord{}, fmt.Errorf("unmarshal security event fields: %w", err)
	}
	return record, nil
}

// securityActorType classifies who made the decision for ADR-034 actor_type,
// whose enum is 'service', 'external', 'user' (ADR-034 schema). A request
// rejected before identity resolution has no authenticated principal, so it is
// classified as 'external' with an empty actor_id -- matching the audit-table
// precedent for unauthenticated callers (test/e2e/gateway/23_audit_emission_test.go).
func securityActorType(user string) string {
	if user == "" {
		return "external"
	}
	return "user"
}

// securityEventClassification derives the ADR-034 event_type and event_action
// for a security_event reason in one place, so the two fields can never
// disagree about the decision domain. A denial is "deny"; a fail-closed
// dependency outage is the operation that could not be completed
// ("authenticate"/"authorize"), never a deny decision. The complete reason
// vocabulary is pinned by UT-AUTH-2664-011.
func securityEventClassification(reason string) (eventType, action string) {
	switch reason {
	case "authorization_denied":
		return securityEventTypeAuthorization, "deny"
	case "authorization_service_unavailable":
		return securityEventTypeAuthorization, "authorize"
	case "authentication_service_unavailable",
		"missing_auth_header",
		"invalid_auth_format",
		"empty_bearer_token",
		"invalid_token",
		"invalid_token_audience":
		return securityEventTypeAuthentication, "authenticate"
	default:
		// Forward-compatible fail-safe: a newly introduced authorization reason
		// must never be classified as an authentication attempt or a denial.
		if strings.HasPrefix(reason, "authorization_") {
			return securityEventTypeAuthorization, "authorize"
		}
		return securityEventTypeAuthentication, "authenticate"
	}
}

// requestCorrelationID resolves the request-scoped correlation identifier.
//
// ADR-034's canonical correlation_id is the remediation ID, which the payload
// handler owns and which does not exist yet when authentication runs. The auth
// middleware therefore records the standard request correlation ID: the
// X-Request-ID request header when the service's request-ID middleware set it
// (Gateway's RequestIDMiddleware rewrites it to the ID it returns and logs),
// otherwise chi's RequestID context value (see
// docs/architecture/LOG_CORRELATION_ID_STANDARD.md). It groups every security
// decision made for one request (e.g. DS's base "get" and route-level "create"
// grant lines). It is empty only when the caller is not behind a correlation-ID
// middleware and sent no header.
func requestCorrelationID(r *http.Request) string {
	// The request header is authoritative when a service's request-ID
	// middleware has set it (Gateway's RequestIDMiddleware overwrites it with the
	// canonical ID it returns and logs). chi's RequestID reads an incoming header
	// and otherwise generates one, so the fallbacks agree when no service
	// middleware rewrote it.
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return chimiddleware.GetReqID(r.Context())
}

// SecurityErrorDetails is the DD-ERROR-001 standardized error payload attached
// to fail-closed outage events (DD-AUDIT-003: every error event carries it).
type SecurityErrorDetails struct {
	Message       string `json:"message"`
	Code          string `json:"code"`
	Component     string `json:"component"`
	RetryPossible bool   `json:"retry_possible"`
}

// securityErrorDetails maps a TokenReview/SAR dependency failure to the
// DD-ERROR-001 contract. Both dependencies are Kubernetes API reads, so the
// failure is transient and retryable.
func newSecurityErrorDetails(reason string, cause error) SecurityErrorDetails {
	code := "ERR_AUTH_SERVICE_UNAVAILABLE"
	if reason == "authorization_service_unavailable" {
		code = "ERR_AUTHZ_SERVICE_UNAVAILABLE"
	}
	return SecurityErrorDetails{
		Message:       cause.Error(),
		Code:          code,
		Component:     "auth-middleware",
		RetryPossible: true,
	}
}

// writeError writes an RFC 7807 Problem Details JSON error response.
func (m *Middleware) writeError(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	problem := map[string]any{
		"type":   "about:blank",
		"title":  title,
		"status": status,
		"detail": detail,
	}

	_ = json.NewEncoder(w).Encode(problem)
}
