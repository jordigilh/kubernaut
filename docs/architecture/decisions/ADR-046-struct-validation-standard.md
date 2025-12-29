# ADR-046: Struct Validation Standard (go-playground/validator)

**Status**: Approved
**Date**: 2025-11-28
**Deciders**: Architecture Team
**Related**: ADR-043, DD-STORAGE-008, DD-WORKFLOW-005
**Version**: 1.1

---

## Changelog

### Version 1.1 (2025-12-04)
- Renumbered from ADR-044 to ADR-046 to resolve conflict with ADR-044 (Workflow Execution Engine Delegation)

### Version 1.0 (2025-11-28)
- Initial ADR creation
- Approved go-playground/validator as the standard validation library
- Defined exception documentation requirements

---

## Context

Kubernaut requires consistent struct validation across all services. The codebase currently has:

1. **Validation tags defined** - Extensive use of `validate:"required,max=255"` tags in model structs
2. **Manual validation** - Custom `Validate()` methods with hand-written field checks
3. **Custom validators** - Service-specific validators like `NotificationAuditValidator`

This creates inconsistency:
- Tags are defined but not activated (decorative only)
- Manual validation duplicates tag logic
- Different validation patterns across services

### Requirements

1. Single validation approach across all services
2. Reduce boilerplate validation code
3. Leverage existing validation tags
4. Support custom validation rules when needed
5. Provide clear error messages for API consumers

---

## Decision

**APPROVED**: Use **go-playground/validator/v10** as the standard validation library for all struct validation in Kubernaut.

**Confidence**: 92%

---

## Rationale

### Why go-playground/validator

| Criteria | Assessment |
|----------|------------|
| **Industry adoption** | ✅ 16k+ GitHub stars, de-facto Go standard |
| **Existing codebase alignment** | ✅ Validation tags already defined in models |
| **Built-in rules** | ✅ 100+ validation rules (required, min, max, oneof, email, etc.) |
| **Custom validators** | ✅ Register custom validation functions |
| **Framework integration** | ✅ Native support in Gin, Echo, Fiber |
| **Error handling** | ✅ Field-level error messages |
| **Performance** | ✅ Optimized reflection, caching |

### Alternatives Considered

| Library | Decision | Reason |
|---------|----------|--------|
| **ozzo-validation** | ❌ Rejected | Would require removing existing tags, less adoption |
| **govalidator** | ❌ Rejected | Less active maintenance, fewer features |
| **Manual validation** | ❌ Rejected | Inconsistent, verbose, error-prone |

---

## Specification

### Standard Validation Pattern

#### 1. Add Validation Tags to Structs

```go
// pkg/datastorage/models/workflow.go
type CreateWorkflowRequest struct {
    WorkflowID  string          `json:"workflow_id" validate:"required,max=255"`
    Version     string          `json:"version" validate:"required,max=50,semver"`
    Name        string          `json:"name" validate:"required,max=255"`
    Description string          `json:"description" validate:"required,min=1"`
    Content     string          `json:"content" validate:"required"`
    Labels      json.RawMessage `json:"labels" validate:"required"`
}
```

#### 2. Create Shared Validator Instance

```go
// pkg/validation/validator.go
package validation

import (
    "sync"
    "github.com/go-playground/validator/v10"
)

var (
    validate *validator.Validate
    once     sync.Once
)

// Get returns the singleton validator instance
func Get() *validator.Validate {
    once.Do(func() {
        validate = validator.New()
        registerCustomValidators(validate)
    })
    return validate
}

// ValidateStruct validates any struct with validate tags
func ValidateStruct(s interface{}) error {
    return Get().Struct(s)
}

// registerCustomValidators registers Kubernaut-specific validators
func registerCustomValidators(v *validator.Validate) {
    // Example: Custom semver validator
    v.RegisterValidation("semver", validateSemver)

    // Example: Custom uppercase_snake_case validator
    v.RegisterValidation("upper_snake", validateUpperSnakeCase)
}
```

#### 3. Use in Handlers

```go
// pkg/datastorage/server/workflow_handlers.go
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
    var req models.CreateWorkflowRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // Handle JSON decode error
        return
    }

    // Validate using shared validator
    if err := validation.ValidateStruct(&req); err != nil {
        // Convert to RFC 7807 error response
        h.writeValidationError(w, err)
        return
    }

    // Proceed with business logic
}
```

#### 4. Error Response Format

```go
// Convert validator errors to RFC 7807 format
func (h *Handler) writeValidationError(w http.ResponseWriter, err error) {
    if validationErrors, ok := err.(validator.ValidationErrors); ok {
        fields := make(map[string]string)
        for _, e := range validationErrors {
            fields[e.Field()] = formatValidationError(e)
        }

        h.writeRFC7807Error(w, http.StatusBadRequest,
            "validation-error",
            "Validation Failed",
            fmt.Sprintf("Invalid request: %d field(s) failed validation", len(fields)),
        )
    }
}
```

---

## Common Validation Tags

| Tag | Description | Example |
|-----|-------------|---------|
| `required` | Field must be present and non-zero | `validate:"required"` |
| `max=N` | Maximum length/value | `validate:"max=255"` |
| `min=N` | Minimum length/value | `validate:"min=1"` |
| `oneof=a b c` | Value must be one of listed | `validate:"oneof=active disabled"` |
| `email` | Valid email format | `validate:"email"` |
| `url` | Valid URL format | `validate:"url"` |
| `uuid` | Valid UUID format | `validate:"uuid"` |
| `len=N` | Exact length | `validate:"len=64"` |
| `omitempty` | Skip validation if empty | `validate:"omitempty,max=255"` |

---

## Custom Validators (Kubernaut-Specific)

| Validator | Purpose | Example |
|-----------|---------|---------|
| `semver` | Semantic version format | `validate:"semver"` |
| `upper_snake` | UPPER_SNAKE_CASE format | `validate:"upper_snake"` |
| `workflow_id` | Valid workflow ID format | `validate:"workflow_id"` |
| `signal_type` | Valid signal type per DD-WORKFLOW-001 | `validate:"signal_type"` |
| `severity` | Valid severity per DD-LLM-001 | `validate:"severity"` |

---

## Exceptions

### When Manual Validation is Required

Document exceptions here when go-playground/validator cannot handle a validation case:

| Exception | Reason | Location | Date Added |
|-----------|--------|----------|------------|
| *None yet* | - | - | - |

### Exception Documentation Template

When adding an exception:

```markdown
#### Exception: [Name]

**Reason**: [Why go-playground/validator cannot handle this]
**Location**: [File path]
**Validation Logic**: [Brief description]
**Date Added**: YYYY-MM-DD
**Approved By**: [Name]
```

---

## Migration Plan

### Phase 1: New Code (Immediate)
- All new structs use go-playground/validator tags
- New handlers use `validation.ValidateStruct()`

### Phase 2: Existing Code (Deferred)
- Migrate `NotificationAuditValidator` to tag-based
- Migrate `DisableWorkflowRequest.Validate()` to tag-based
- Remove manual validation methods

### Priority
- **Phase 1**: Implement with ADR-043 (WorkflowSchema)
- **Phase 2**: Separate task after Data Storage CRUD completion

---

## Consequences

### Positive

1. ✅ **Reduced boilerplate** - 80%+ reduction in validation code
2. ✅ **Consistency** - Same validation pattern across all services
3. ✅ **Maintainability** - Add validations via tags, not code
4. ✅ **Error messages** - Automatic field-level errors
5. ✅ **Activates existing tags** - Zero migration for models with tags

### Negative

1. ⚠️ **New dependency** - Adds go-playground/validator/v10 to go.mod
2. ⚠️ **Learning curve** - Team must learn tag syntax
3. ⚠️ **Complex validation** - May need custom validators for edge cases

### Mitigations

- **Dependency**: Well-maintained, widely used library
- **Learning**: Comprehensive documentation, common patterns
- **Complex validation**: Custom validator registration supported

---

## Implementation Checklist

- [ ] Add `github.com/go-playground/validator/v10` to go.mod
- [ ] Create `pkg/validation/validator.go` with shared instance
- [ ] Register custom validators (semver, upper_snake, etc.)
- [ ] Update handlers to use `validation.ValidateStruct()`
- [ ] Create error response helper for validation errors
- [ ] Document any exceptions in this ADR

---

## References

- [go-playground/validator Documentation](https://pkg.go.dev/github.com/go-playground/validator/v10)
- [Validation Tag Reference](https://github.com/go-playground/validator#baked-in-validations)
- [Custom Validators Guide](https://github.com/go-playground/validator#custom-validation-functions)

---

## Approval

**Status**: ✅ Approved
**Date**: 2025-11-28
**Authority**: Authoritative

**Next Steps**:
1. Implement with ADR-043 WorkflowSchema struct
2. Queue migration of existing validators as separate task


**Status**: Approved
**Date**: 2025-11-28
**Deciders**: Architecture Team
**Related**: ADR-043, DD-STORAGE-008, DD-WORKFLOW-005
**Version**: 1.0

---

## Changelog

### Version 1.0 (2025-11-28)
- Initial ADR creation
- Approved go-playground/validator as the standard validation library
- Defined exception documentation requirements

---

## Context

Kubernaut requires consistent struct validation across all services. The codebase currently has:

1. **Validation tags defined** - Extensive use of `validate:"required,max=255"` tags in model structs
2. **Manual validation** - Custom `Validate()` methods with hand-written field checks
3. **Custom validators** - Service-specific validators like `NotificationAuditValidator`

This creates inconsistency:
- Tags are defined but not activated (decorative only)
- Manual validation duplicates tag logic
- Different validation patterns across services

### Requirements

1. Single validation approach across all services
2. Reduce boilerplate validation code
3. Leverage existing validation tags
4. Support custom validation rules when needed
5. Provide clear error messages for API consumers

---

## Decision

**APPROVED**: Use **go-playground/validator/v10** as the standard validation library for all struct validation in Kubernaut.

**Confidence**: 92%

---

## Rationale

### Why go-playground/validator

| Criteria | Assessment |
|----------|------------|
| **Industry adoption** | ✅ 16k+ GitHub stars, de-facto Go standard |
| **Existing codebase alignment** | ✅ Validation tags already defined in models |
| **Built-in rules** | ✅ 100+ validation rules (required, min, max, oneof, email, etc.) |
| **Custom validators** | ✅ Register custom validation functions |
| **Framework integration** | ✅ Native support in Gin, Echo, Fiber |
| **Error handling** | ✅ Field-level error messages |
| **Performance** | ✅ Optimized reflection, caching |

### Alternatives Considered

| Library | Decision | Reason |
|---------|----------|--------|
| **ozzo-validation** | ❌ Rejected | Would require removing existing tags, less adoption |
| **govalidator** | ❌ Rejected | Less active maintenance, fewer features |
| **Manual validation** | ❌ Rejected | Inconsistent, verbose, error-prone |

---

## Specification

### Standard Validation Pattern

#### 1. Add Validation Tags to Structs

```go
// pkg/datastorage/models/workflow.go
type CreateWorkflowRequest struct {
    WorkflowID  string          `json:"workflow_id" validate:"required,max=255"`
    Version     string          `json:"version" validate:"required,max=50,semver"`
    Name        string          `json:"name" validate:"required,max=255"`
    Description string          `json:"description" validate:"required,min=1"`
    Content     string          `json:"content" validate:"required"`
    Labels      json.RawMessage `json:"labels" validate:"required"`
}
```

#### 2. Create Shared Validator Instance

```go
// pkg/validation/validator.go
package validation

import (
    "sync"
    "github.com/go-playground/validator/v10"
)

var (
    validate *validator.Validate
    once     sync.Once
)

// Get returns the singleton validator instance
func Get() *validator.Validate {
    once.Do(func() {
        validate = validator.New()
        registerCustomValidators(validate)
    })
    return validate
}

// ValidateStruct validates any struct with validate tags
func ValidateStruct(s interface{}) error {
    return Get().Struct(s)
}

// registerCustomValidators registers Kubernaut-specific validators
func registerCustomValidators(v *validator.Validate) {
    // Example: Custom semver validator
    v.RegisterValidation("semver", validateSemver)

    // Example: Custom uppercase_snake_case validator
    v.RegisterValidation("upper_snake", validateUpperSnakeCase)
}
```

#### 3. Use in Handlers

```go
// pkg/datastorage/server/workflow_handlers.go
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
    var req models.CreateWorkflowRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // Handle JSON decode error
        return
    }

    // Validate using shared validator
    if err := validation.ValidateStruct(&req); err != nil {
        // Convert to RFC 7807 error response
        h.writeValidationError(w, err)
        return
    }

    // Proceed with business logic
}
```

#### 4. Error Response Format

```go
// Convert validator errors to RFC 7807 format
func (h *Handler) writeValidationError(w http.ResponseWriter, err error) {
    if validationErrors, ok := err.(validator.ValidationErrors); ok {
        fields := make(map[string]string)
        for _, e := range validationErrors {
            fields[e.Field()] = formatValidationError(e)
        }

        h.writeRFC7807Error(w, http.StatusBadRequest,
            "validation-error",
            "Validation Failed",
            fmt.Sprintf("Invalid request: %d field(s) failed validation", len(fields)),
        )
    }
}
```

---

## Common Validation Tags

| Tag | Description | Example |
|-----|-------------|---------|
| `required` | Field must be present and non-zero | `validate:"required"` |
| `max=N` | Maximum length/value | `validate:"max=255"` |
| `min=N` | Minimum length/value | `validate:"min=1"` |
| `oneof=a b c` | Value must be one of listed | `validate:"oneof=active disabled"` |
| `email` | Valid email format | `validate:"email"` |
| `url` | Valid URL format | `validate:"url"` |
| `uuid` | Valid UUID format | `validate:"uuid"` |
| `len=N` | Exact length | `validate:"len=64"` |
| `omitempty` | Skip validation if empty | `validate:"omitempty,max=255"` |

---

## Custom Validators (Kubernaut-Specific)

| Validator | Purpose | Example |
|-----------|---------|---------|
| `semver` | Semantic version format | `validate:"semver"` |
| `upper_snake` | UPPER_SNAKE_CASE format | `validate:"upper_snake"` |
| `workflow_id` | Valid workflow ID format | `validate:"workflow_id"` |
| `signal_type` | Valid signal type per DD-WORKFLOW-001 | `validate:"signal_type"` |
| `severity` | Valid severity per DD-LLM-001 | `validate:"severity"` |

---

## Exceptions

### When Manual Validation is Required

Document exceptions here when go-playground/validator cannot handle a validation case:

| Exception | Reason | Location | Date Added |
|-----------|--------|----------|------------|
| *None yet* | - | - | - |

### Exception Documentation Template

When adding an exception:

```markdown
#### Exception: [Name]

**Reason**: [Why go-playground/validator cannot handle this]
**Location**: [File path]
**Validation Logic**: [Brief description]
**Date Added**: YYYY-MM-DD
**Approved By**: [Name]
```

---

## Migration Plan

### Phase 1: New Code (Immediate)
- All new structs use go-playground/validator tags
- New handlers use `validation.ValidateStruct()`

### Phase 2: Existing Code (Deferred)
- Migrate `NotificationAuditValidator` to tag-based
- Migrate `DisableWorkflowRequest.Validate()` to tag-based
- Remove manual validation methods

### Priority
- **Phase 1**: Implement with ADR-043 (WorkflowSchema)
- **Phase 2**: Separate task after Data Storage CRUD completion

---

## Consequences

### Positive

1. ✅ **Reduced boilerplate** - 80%+ reduction in validation code
2. ✅ **Consistency** - Same validation pattern across all services
3. ✅ **Maintainability** - Add validations via tags, not code
4. ✅ **Error messages** - Automatic field-level errors
5. ✅ **Activates existing tags** - Zero migration for models with tags

### Negative

1. ⚠️ **New dependency** - Adds go-playground/validator/v10 to go.mod
2. ⚠️ **Learning curve** - Team must learn tag syntax
3. ⚠️ **Complex validation** - May need custom validators for edge cases

### Mitigations

- **Dependency**: Well-maintained, widely used library
- **Learning**: Comprehensive documentation, common patterns
- **Complex validation**: Custom validator registration supported

---

## Implementation Checklist

- [ ] Add `github.com/go-playground/validator/v10` to go.mod
- [ ] Create `pkg/validation/validator.go` with shared instance
- [ ] Register custom validators (semver, upper_snake, etc.)
- [ ] Update handlers to use `validation.ValidateStruct()`
- [ ] Create error response helper for validation errors
- [ ] Document any exceptions in this ADR

---

## References

- [go-playground/validator Documentation](https://pkg.go.dev/github.com/go-playground/validator/v10)
- [Validation Tag Reference](https://github.com/go-playground/validator#baked-in-validations)
- [Custom Validators Guide](https://github.com/go-playground/validator#custom-validation-functions)

---

## Approval

**Status**: ✅ Approved
**Date**: 2025-11-28
**Authority**: Authoritative

**Next Steps**:
1. Implement with ADR-043 WorkflowSchema struct
2. Queue migration of existing validators as separate task



