# REQUEST: Migrate to Shared Sanitization Library

**From**: Data Storage Team
**To**: Gateway Team, Notification Team
**Date**: December 9, 2025
**Priority**: 🟡 P2 (MEDIUM) - Technical Debt Reduction
**Status**: 🟡 ACTION REQUESTED

---

## 📋 Summary

A shared sanitization library now exists at `pkg/shared/sanitization/` that consolidates DD-005 compliant log sanitization logic. Services currently using service-specific implementations should migrate to the shared library for:

- **Code consistency** across the codebase
- **Single source of truth** for sanitization patterns
- **Reduced maintenance** (one fix benefits all services)
- **DD-005 compliance** with authoritative patterns

---

## 🎯 Current State

### Service-Specific Implementations (DUPLICATE CODE)

| Service | Current Implementation | Lines of Code | Status |
|---------|------------------------|---------------|--------|
| **Gateway** | `pkg/gateway/middleware/log_sanitization.go` | ~203 | 🟡 Migrate to shared |
| **Notification** | `pkg/notification/sanitization/sanitizer.go` | ~varies | 🟡 Migrate to shared |
| **Shared Library** | `pkg/shared/sanitization/` | ~410 | ✅ **AUTHORITATIVE** |
| **Data Storage** | N/A (structured logging) | N/A | ✅ Compliant via design |

### Shared Library Location

```
pkg/shared/sanitization/
├── doc.go          # Package documentation
├── sanitizer.go    # Core sanitization logic (Sanitizer struct, Rules)
└── headers.go      # HTTP header sanitization
```

---

## 📚 Shared Library API

### Basic Usage

```go
import "github.com/jordigilh/kubernaut/pkg/shared/sanitization"

// Simple string sanitization
sanitized := sanitization.SanitizeForLog(sensitiveData)
logger.Info("Processing request", "payload", sanitized)
```

### With Fallback (Recommended for Production)

```go
import "github.com/jordigilh/kubernaut/pkg/shared/sanitization"

// Create sanitizer instance
sanitizer := sanitization.NewSanitizer()

// Sanitize with graceful degradation (BR-NOT-055)
clean, err := sanitizer.SanitizeWithFallback(content)
if err != nil {
    // Fallback sanitization was used - log the error
    logger.Error(err, "Sanitization used fallback mode")
}
```

### HTTP Middleware

```go
import "github.com/jordigilh/kubernaut/pkg/shared/sanitization"

// Create sanitizing middleware
middleware := sanitization.NewLoggingMiddleware(logger)
router.Use(middleware)
```

### Custom Rules

```go
import "github.com/jordigilh/kubernaut/pkg/shared/sanitization"

// Service-specific patterns
customRules := []*sanitization.Rule{
    {
        Name:        "Custom Secret",
        Pattern:     regexp.MustCompile(`my-secret-pattern`),
        Replacement: "[REDACTED]",
        Description: "Service-specific secret pattern",
    },
}

// Append to default rules
allRules := append(sanitization.DefaultRules(), customRules...)
sanitizer := sanitization.NewSanitizerWithRules(allRules)
```

---

## 🔄 Migration Guide

### Gateway Service

**Current**: `pkg/gateway/middleware/log_sanitization.go`

**Step 1**: Update imports
```go
// BEFORE
import "github.com/jordigilh/kubernaut/pkg/gateway/middleware"

// AFTER
import "github.com/jordigilh/kubernaut/pkg/shared/sanitization"
```

**Step 2**: Replace function calls
```go
// BEFORE
middleware.SanitizeForLog(data)
middleware.NewSanitizingLogger(writer)

// AFTER
sanitization.SanitizeForLog(data)
sanitization.NewLoggingMiddleware(logger)
```

**Step 3**: Deprecate service-specific file
- Add deprecation notice to `pkg/gateway/middleware/log_sanitization.go`
- Keep for backwards compatibility during transition
- Remove after migration is verified

### Notification Service

**Current**: `pkg/notification/sanitization/sanitizer.go`

**Step 1**: Update imports
```go
// BEFORE
import "github.com/jordigilh/kubernaut/pkg/notification/sanitization"

// AFTER
import sharedsanitization "github.com/jordigilh/kubernaut/pkg/shared/sanitization"
```

**Step 2**: Replace sanitizer instantiation
```go
// BEFORE
sanitizer := notification.NewSanitizer()

// AFTER
sanitizer := sharedsanitization.NewSanitizer()
```

**Step 3**: Update test imports accordingly

---

## ✅ Patterns Covered by Shared Library

The shared library covers all DD-005 required patterns:

### Passwords
- `password`, `passwd`, `pwd` (JSON, URL, plain text)

### API Keys
- `api_key`, `apikey`
- OpenAI keys (`sk-*`)
- AWS keys

### Tokens
- `token`, `access_token`
- Bearer tokens
- GitHub tokens (`ghp_*`)

### Secrets
- `secret`, `client_secret`, `credential`

### Database URLs
- PostgreSQL connection strings
- MySQL connection strings
- MongoDB connection strings

### Certificates
- PEM certificates
- Private keys

### Kubernetes
- Secret data (base64 encoded)

### HTTP Headers
- `Authorization`
- `Bearer`
- `X-API-Key`

---

## 📋 Action Items

| # | Action | Service | Owner | Timeline |
|---|--------|---------|-------|----------|
| 1 | Review shared library API | All | DS Team | Done |
| 2 | Migrate Gateway to shared library | Gateway | Gateway Team | V1.1 |
| 3 | Migrate Notification to shared library | Notification | Notification Team | V1.1 |
| 4 | Deprecate service-specific implementations | All | Respective teams | V1.1 |
| 5 | Update tests to use shared library | All | Respective teams | V1.1 |
| 6 | Remove deprecated code | All | Respective teams | V1.2 |

---

## 🧪 Testing Requirements

After migration, ensure:

1. **Unit Tests**: All existing sanitization unit tests pass with shared library
2. **Integration Tests**: No change in sanitized output format
3. **Regression**: No sensitive data leaks in logs

### Test Files to Update

**Gateway**:
- `test/unit/gateway/middleware/log_sanitization_test.go` → Use shared library

**Notification**:
- `test/unit/notification/sanitization_test.go` → Use shared library
- `test/unit/notification/sanitization/sanitizer_fallback_test.go` → Use shared library

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [DD-005-OBSERVABILITY-STANDARDS.md](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) | Authoritative DD-005 specification |
| [pkg/shared/sanitization/doc.go](../../pkg/shared/sanitization/doc.go) | Shared library documentation |
| [NOTICE_DD005_DOCUMENTATION_CODE_DISCREPANCY.md](./NOTICE_DD005_DOCUMENTATION_CODE_DISCREPANCY.md) | Discovery of duplication issue |

---

## 📞 Response Section

### Gateway Team Response

```
⏳ AWAITING RESPONSE

Please confirm:
1. Acknowledgment of migration request
2. Timeline for migration
3. Any concerns with shared library API
```

### Notification Team Response

```
⏳ AWAITING RESPONSE

Please confirm:
1. Acknowledgment of migration request
2. Timeline for migration
3. Any concerns with shared library API
```

---

**Document Version**: 1.0
**Created**: December 9, 2025
**Last Updated**: December 9, 2025
**Maintained By**: Data Storage Team

