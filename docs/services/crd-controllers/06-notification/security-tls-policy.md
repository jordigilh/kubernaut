# Notification Service - TLS Certificate Security Policy

**Business Requirement**: BR-NOT-058 (Security Policy & Error Handling)
**Status**: ✅ Active
**Last Updated**: 2025-11-21
**Moved From**: `test/unit/notification/slack_delivery_test.go` (Test #2 - Documentation-only test)

---

## 🔒 **TLS Certificate Security Policy**

This document defines the mandatory security policy for TLS certificate validation in the Notification Service, specifically for Slack webhook delivery.

**Rationale**: TLS certificate errors indicate security issues or misconfigurations that should NOT be automatically retried, as retry would bypass critical security validation.

---

## 📋 **Policy Requirements (BR-NOT-058)**

### **1. Production Environment - MANDATORY TLS Validation**

**Policy**: ✅ **TLS validation MUST be enforced in production**

| Requirement | Status | Implementation |
|-------------|--------|----------------|
| **Valid TLS Certificates** | ✅ MANDATORY | Go's `http.Client` validates by default |
| **Certificate Chain Validation** | ✅ MANDATORY | Automatic (stdlib) |
| **Hostname Verification** | ✅ MANDATORY | Automatic (stdlib) |
| **Expiration Check** | ✅ MANDATORY | Automatic (stdlib) |
| **TLS Version Enforcement** | ✅ MANDATORY | TLS 1.2+ (stdlib default) |

**Production Webhook Requirements**:
- ✅ Webhooks MUST use HTTPS (not HTTP)
- ✅ Certificates MUST be issued by trusted CA
- ✅ Certificates MUST be valid (not expired)
- ✅ Certificate hostname MUST match webhook domain
- ✅ Certificate chain MUST be complete

---

### **2. TLS Error Handling - Non-Retryable**

**Policy**: ❌ **TLS errors MUST NOT be retried automatically**

| Error Type | Classification | Retry? | Rationale |
|------------|----------------|--------|-----------|
| **Expired Certificate** | Permanent Failure | ❌ NO | Security vulnerability - expired cert = no validation |
| **Self-Signed Certificate** | Permanent Failure | ❌ NO | Untrusted CA - potential MITM attack |
| **Invalid Certificate** | Permanent Failure | ❌ NO | Certificate validation failed |
| **Hostname Mismatch** | Permanent Failure | ❌ NO | Potential MITM attack or DNS misconfiguration |
| **Unknown Authority** | Permanent Failure | ❌ NO | Certificate not issued by trusted CA |
| **TLS Handshake Failure** | Permanent Failure | ❌ NO | Protocol incompatibility or configuration error |

**Security Rationale**:
- Automatic retry of TLS errors would **bypass security validation**
- TLS errors indicate **misconfiguration or active attacks**
- Operations team must be **alerted immediately** for manual investigation
- Retry logic could mask **ongoing security incidents**

---

### **3. Development Environment - Optional TLS Skip**

**Policy**: ⚠️ **TLS validation MAY be disabled in development (with explicit flag)**

**Use Cases for TLS Skip**:
- ✅ Local development with self-signed certificates
- ✅ Integration testing with test certificates
- ✅ Staging environments with internal CAs

**Requirements for TLS Skip**:
- ❌ **NEVER in production** (MANDATORY enforcement)
- ✅ Explicit configuration flag required (`SLACK_TLS_SKIP_VERIFY=true`)
- ⚠️ Must log clear warning when TLS validation disabled
- 📋 Must document security implications in configuration

**Example Configuration** (Development Only):
```yaml
# config/development.yaml
notification:
  slack:
    tls_skip_verify: true  # ⚠️ DEVELOPMENT ONLY - DO NOT USE IN PRODUCTION
```

**Configuration Validation**:
```go
// Production deployment check
if isProduction && config.TLSSkipVerify {
    return fmt.Errorf("TLS validation cannot be disabled in production (BR-NOT-058)")
}
```

---

## 🚨 **Operations & Monitoring**

### **1. TLS Error Alerting - REQUIRED**

**Policy**: ✅ **TLS errors MUST trigger immediate alerts**

**Alert Triggers**:
- ❌ `x509.CertificateInvalidError` → **ALERT: Certificate validation failed**
- ❌ `x509.UnknownAuthorityError` → **ALERT: Untrusted certificate authority**
- ❌ `x509.HostnameError` → **ALERT: Certificate hostname mismatch**
- ❌ `tls.RecordHeaderError` → **ALERT: TLS protocol error**

**Alert Severity**: 🔴 **CRITICAL**
- TLS errors indicate potential security incidents
- Require immediate investigation by operations team
- May indicate active MITM attack or infrastructure compromise

**Alert Information to Include**:
- Webhook URL (sanitized - don't expose full webhook token)
- TLS error type and details
- Timestamp of failure
- Notification ID for correlation
- Certificate details (if available)

---

### **2. Certificate Expiration Monitoring - RECOMMENDED**

**Policy**: ⚠️ **Monitor certificate expiration proactively**

**Monitoring Strategy**:
- ⚠️ Alert when certificates expire in <30 days
- ⚠️ Alert when certificates expire in <7 days (escalated)
- 🔴 Alert when certificates are already expired (critical)

**Prevention**:
- Set up automated certificate renewal
- Monitor certificate expiration dates
- Test certificate rotation procedures
- Document emergency certificate update process

---

### **3. Metrics & Observability**

**Policy**: ✅ **Track TLS-related metrics**

**Prometheus Metrics** (Recommended):
```prometheus
# TLS error count by type
notification_slack_tls_errors_total{error_type="expired|unknown_authority|hostname_mismatch|handshake_failure"} counter

# TLS validation success rate
notification_slack_tls_validation_success_rate gauge

# Certificate expiration days remaining (for monitored webhooks)
notification_slack_certificate_expiry_days{webhook_domain="hooks.slack.com"} gauge
```

**Grafana Alerts**:
- Alert when TLS error rate > 0 (any TLS error is critical)
- Alert when certificate expires in <30 days
- Alert when TLS validation success rate < 100%

---

## 🔧 **Implementation Details**

### **1. Go's stdlib Provides Secure Defaults**

**Implementation**: ✅ **No custom TLS code required**

**Standard Library Behavior**:
```go
// Go's http.Client automatically validates TLS certificates
client := &http.Client{
    Timeout: 30 * time.Second,
    // TLS validation is enabled by default
    // - Validates certificate chain
    // - Checks expiration
    // - Verifies hostname
    // - Enforces TLS 1.2+
}
```

**Secure by Default**:
- ✅ Certificate chain validation (automatic)
- ✅ Hostname verification (automatic)
- ✅ Expiration checking (automatic)
- ✅ TLS 1.2+ enforcement (automatic)
- ✅ Secure cipher suites (automatic)

**No Additional Code Needed**: Go's `http.Client` provides production-grade TLS security out of the box.

---

### **2. TLS Skip Configuration (Development Only)**

**Configuration** (if needed for development):
```go
// ⚠️ DEVELOPMENT ONLY - NEVER IN PRODUCTION
func newDevelopmentHTTPClient(config Config) *http.Client {
    if config.Environment == "production" && config.TLSSkipVerify {
        panic("TLS validation cannot be disabled in production (BR-NOT-058)")
    }

    transport := &http.Transport{}

    if config.TLSSkipVerify {
        logger.Warn("TLS certificate validation disabled - DEVELOPMENT ONLY",
            zap.Bool("tls_skip_verify", true),
            zap.String("security_risk", "MITM attacks possible"))

        transport.TLSClientConfig = &tls.Config{
            InsecureSkipVerify: true, // ⚠️ INSECURE
        }
    }

    return &http.Client{
        Timeout:   30 * time.Second,
        Transport: transport,
    }
}
```

---

### **3. Error Classification Implementation**

**Error Handling** (in `pkg/notification/delivery/slack.go`):
```go
func (s *SlackDeliveryService) Deliver(ctx context.Context, notification *Notification) error {
    // ... delivery logic ...

    if err != nil {
        // Classify TLS errors as permanent failures (non-retryable)
        if isTLSError(err) {
            // Log TLS error for alerting
            s.logger.Error("TLS certificate validation failed - permanent failure",
                zap.Error(err),
                zap.String("webhook_domain", extractDomain(s.webhookURL)),
                zap.String("notification_id", notification.Name),
                zap.String("br", "BR-NOT-058"))

            // Return permanent error (will not be retried)
            return NewPermanentError(err, "TLS certificate validation failed")
        }

        // ... other error handling ...
    }

    return nil
}

func isTLSError(err error) bool {
    // Check for various TLS error types
    var certInvalidErr *x509.CertificateInvalidError
    var unknownAuthorityErr *x509.UnknownAuthorityError
    var hostnameErr *x509.HostnameError

    return errors.As(err, &certInvalidErr) ||
           errors.As(err, &unknownAuthorityErr) ||
           errors.As(err, &hostnameErr) ||
           strings.Contains(err.Error(), "tls:") ||
           strings.Contains(err.Error(), "x509:")
}
```

---

## 📚 **References**

### **Related Business Requirements**
- **BR-NOT-058**: Security Error Handling & Policy (Primary)
- **BR-NOT-052**: Retry on Timeout (Excludes TLS errors)
- **BR-NOT-063**: Graceful Audit Degradation (Error handling framework)

### **Related Documentation**
- [Notification Service Security Configuration](./security-configuration.md)
- [Error Handling Philosophy](./implementation/design/ERROR_HANDLING_PHILOSOPHY.md)
- [Slack Delivery Implementation](../../pkg/notification/delivery/slack.go)

### **Testing**
- **Unit Tests**: `test/unit/notification/slack_delivery_test.go` (Network error handling)
- **Integration Tests**: `test/integration/notification/slack_tls_integration_test.go` (TLS validation scenarios)
- **E2E Tests**: `test/e2e/notification/` (Full notification lifecycle)

---

## 🔐 **Security Best Practices**

### **1. Certificate Management**

**Recommendations**:
- ✅ Use certificates from trusted CAs (Let's Encrypt, DigiCert, etc.)
- ✅ Implement automated certificate renewal (avoid expiration)
- ✅ Monitor certificate expiration proactively
- ✅ Test certificate rotation procedures regularly
- ✅ Document emergency certificate update process

**Slack Webhook Certificates**:
- Slack webhooks (`hooks.slack.com`) use valid TLS certificates from trusted CAs
- Certificates are automatically rotated by Slack
- No manual certificate management required for Slack webhooks
- Custom webhook proxies MUST use valid certificates

---

### **2. Incident Response**

**TLS Error Response Procedure**:

1. **Immediate Actions** (Within 5 minutes):
   - ✅ Acknowledge alert
   - ✅ Check if webhook URL is correct
   - ✅ Verify certificate is not expired
   - ✅ Check for recent infrastructure changes

2. **Investigation** (Within 30 minutes):
   - ✅ Examine certificate details (`openssl s_client -connect hooks.slack.com:443`)
   - ✅ Verify DNS resolution is correct
   - ✅ Check for MITM indicators
   - ✅ Review recent network/firewall changes

3. **Resolution**:
   - If certificate expired: Update certificate immediately
   - If self-signed in production: Replace with valid CA-signed certificate
   - If hostname mismatch: Correct webhook URL or certificate
   - If unknown authority: Investigate potential compromise, rotate credentials

4. **Post-Incident**:
   - ✅ Document root cause
   - ✅ Update runbooks if needed
   - ✅ Review monitoring/alerting effectiveness
   - ✅ Implement preventive measures

---

### **3. Configuration Validation**

**Deployment Validation** (in CI/CD):
```bash
# Validate TLS configuration before deployment
#!/bin/bash

# Check that TLS skip is NOT enabled for production
if [ "$ENVIRONMENT" = "production" ]; then
    if grep -q "tls_skip_verify: true" config/production.yaml; then
        echo "ERROR: TLS validation cannot be disabled in production (BR-NOT-058)"
        exit 1
    fi
fi

# Verify webhook URLs use HTTPS (not HTTP)
if grep -E "^[^#]*http://" config/*.yaml; then
    echo "ERROR: Webhook URLs must use HTTPS, not HTTP"
    exit 1
fi

echo "✅ TLS configuration validation passed"
```

---

## ✅ **Compliance Checklist**

**Production Deployment Checklist** (BR-NOT-058):

- [ ] ✅ TLS validation enabled (not disabled)
- [ ] ✅ All webhook URLs use HTTPS
- [ ] ✅ Certificates from trusted CAs
- [ ] ✅ Certificate expiration monitoring enabled
- [ ] ✅ TLS error alerting configured
- [ ] ✅ Incident response procedures documented
- [ ] ✅ No `InsecureSkipVerify` in production code
- [ ] ✅ Configuration validation in CI/CD pipeline

---

**Document Status**: ✅ Active
**Policy Effective Date**: 2025-11-21
**Review Frequency**: Quarterly or after security incidents
**Policy Owner**: Security Team + Notification Service Maintainers


