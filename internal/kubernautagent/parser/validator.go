/*
Copyright 2026 Jordi Gil.

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

package parser

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ValidationError captures a specific validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// WorkflowMeta holds catalog metadata for a workflow.
type WorkflowMeta struct {
	ExecutionEngine       string
	ExecutionBundle       string
	ExecutionBundleDigest string
	ServiceAccountName    string
	Version               string
	Component             []string // MandatoryLabels.Component: GVK scope (apiVersion/Kind, e.g. apps/v1/Deployment), plain Kind legacy, or ["*"]
	Parameters            []models.WorkflowParameter
	CompiledPatterns      map[string]*regexp.Regexp

	// ActionType/WorkflowName are catalog-authoritative identifiers (Issue
	// #1661 Change 12, DD-WORKFLOW-018) copied verbatim from the DS
	// catalog response (both are required, non-optional fields there).
	// enrichFromCatalog copies these onto InvestigationResult so KA's wire
	// response carries them through to AIAnalysis.Status.SelectedWorkflow.
	ActionType   string
	WorkflowName string

	// Dependencies declares the Secrets/ConfigMaps the workflow's schema
	// requires in the execution namespace (DD-WE-006). Nil when the schema
	// declares no dependencies section. Issue #1661 Change 11a
	// (DD-WORKFLOW-018): sourced here so enrichFromCatalog can place it on
	// InvestigationResult, letting AA embed it in the CRD execution snapshot
	// instead of WorkflowExecution re-fetching it from DataStorage.
	Dependencies *models.WorkflowDependencies

	// Resources declares the per-workflow Job container CPU/memory
	// requests/limits (BR-WE-019 / DD-WE-008). Nil when the schema's
	// execution.resources section is absent (BestEffort QoS preserved).
	Resources *corev1.ResourceRequirements

	// DeclaredParameterNames is the parameter-name allowlist WorkflowExecution
	// uses for defense-in-depth stripping of undeclared parameters (#243).
	// nil means no schema content was parsed; empty means the schema
	// declares zero parameters -- both are distinct from "unfiltered".
	DeclaredParameterNames map[string]bool
}

// kaManagedParams are parameters injected by KA (not provided by the LLM).
// These are excluded from schema validation and never stripped.
var kaManagedParams = map[string]bool{
	"TARGET_RESOURCE_NAME":        true,
	"TARGET_RESOURCE_KIND":        true,
	"TARGET_RESOURCE_NAMESPACE":   true,
	"TARGET_RESOURCE_API_VERSION": true,
}

// ParameterValidationResult captures the outcome of parameter schema validation.
type ParameterValidationResult struct {
	IsValid        bool
	Errors         []string
	Warnings       []string
	StrippedParams []string
	SchemaHint     string
}

// ParameterValidationError wraps a ParameterValidationResult as an error,
// allowing correctionFn to access the structured result for template rendering.
type ParameterValidationError struct {
	Result *ParameterValidationResult
}

func (e *ParameterValidationError) Error() string {
	return fmt.Sprintf("parameter validation failed: %s", strings.Join(e.Result.Errors, "; "))
}

// MatchesTargetKind reports whether the workflow's component scope includes
// the given Kubernetes resource kind. Returns true when:
//   - kind is empty (no constraint to check)
//   - Component is nil or empty (unconstrained — backward compat)
//   - Component contains "*" (wildcard)
//   - Component equals kind case-insensely (legacy plain-kind labels)
//   - Component is GVK-shaped (Issue #1051: "apiVersion/Kind"); the trailing Kind segment
//     is matched case-insensely against remediation target Kind
func (m WorkflowMeta) MatchesTargetKind(kind string) bool {
	if kind == "" || len(m.Component) == 0 {
		return true
	}
	for _, c := range m.Component {
		if workflowComponentMatchesTargetKind(c, kind) {
			return true
		}
	}
	return false
}

func workflowComponentMatchesTargetKind(component, targetKind string) bool {
	if component == "*" {
		return true
	}
	if strings.Contains(component, "/") {
		parts := strings.Split(component, "/")
		last := parts[len(parts)-1]
		return strings.EqualFold(last, targetKind)
	}
	return strings.EqualFold(component, targetKind)
}

// Validator checks InvestigationResult against session-specific constraints.
type Validator struct {
	allowedWorkflows map[string]struct{}
	catalogMeta      map[string]WorkflowMeta
}

// NewValidator creates a result validator with the given workflow allowlist.
func NewValidator(allowedWorkflows []string) *Validator {
	allowed := make(map[string]struct{}, len(allowedWorkflows))
	for _, w := range allowedWorkflows {
		allowed[w] = struct{}{}
	}
	return &Validator{
		allowedWorkflows: allowed,
		catalogMeta:      make(map[string]WorkflowMeta),
	}
}

// SetWorkflowMeta stores catalog metadata for a workflow ID (UUID).
// Pre-compiles regex patterns from parameter schema to avoid per-call compilation.
func (v *Validator) SetWorkflowMeta(workflowID string, meta WorkflowMeta) {
	if len(meta.Parameters) > 0 && meta.CompiledPatterns == nil {
		meta.CompiledPatterns = compileParameterPatterns(meta.Parameters)
	}
	v.catalogMeta[workflowID] = meta
}

// maxPatternLength caps regex pattern length to prevent ReDoS from excessively
// long patterns in workflow schemas. Patterns exceeding this are treated as
// invalid and skipped (warning emitted at validation time).
const maxPatternLength = 1024

// compileParameterPatterns pre-compiles regex patterns from workflow parameter
// definitions. Invalid or oversized patterns are silently skipped — they'll
// produce a warning at validation time via the fallback path.
func compileParameterPatterns(params []models.WorkflowParameter) map[string]*regexp.Regexp {
	compiled := make(map[string]*regexp.Regexp)
	for _, p := range params {
		if p.Pattern == "" || len(p.Pattern) > maxPatternLength {
			continue
		}
		if re, err := regexp.Compile(p.Pattern); err == nil {
			compiled[p.Name] = re
		}
	}
	if len(compiled) == 0 {
		return nil
	}
	return compiled
}

// GetWorkflowMeta returns catalog metadata for a workflow ID, if available.
func (v *Validator) GetWorkflowMeta(workflowID string) (WorkflowMeta, bool) {
	m, ok := v.catalogMeta[workflowID]
	return m, ok
}

// IsAllowed reports whether workflowID is present in the session allowlist
// (i.e. it resolves against the DS catalog fetched for this request). This
// is the same membership test Validate applies to workflow_id, exposed here
// so callers outside Validate (e.g. enrichFromCatalog's Issue #1711 guard)
// can distinguish "unresolvable ID" from "resolvable ID with no registered
// WorkflowMeta" -- the latter occurs only in test fixtures that populate the
// allowlist without a matching SetWorkflowMeta call; production always sets
// both from the same catalog-fetch loop (see dsCatalogFetcher.FetchValidator).
func (v *Validator) IsAllowed(workflowID string) bool {
	_, ok := v.allowedWorkflows[workflowID]
	return ok
}

// Validate checks the result against the allowlist, confidence bounds, and
// parameter schema constraints. Returns a ParameterValidationError when
// parameter validation fails (so correctionFn can access the structured result),
// or a ValidationError for allowlist/confidence failures.
func (v *Validator) Validate(result *katypes.InvestigationResult) error {
	if result.HumanReviewNeeded {
		return nil
	}

	if result.WorkflowID != "" {
		if _, ok := v.allowedWorkflows[result.WorkflowID]; !ok {
			return &ValidationError{
				Field:   "workflow_id",
				Message: fmt.Sprintf("workflow %q not in session allowlist", result.WorkflowID),
			}
		}
	}

	if result.Confidence < 0 || result.Confidence > 1 {
		return &ValidationError{
			Field:   "confidence",
			Message: fmt.Sprintf("confidence %.2f out of [0, 1] range", result.Confidence),
		}
	}

	if result.WorkflowID != "" && result.Parameters != nil {
		if meta, ok := v.catalogMeta[result.WorkflowID]; ok {
			pvr := v.validateParameters(result.Parameters, meta.Parameters, meta.CompiledPatterns)
			if !pvr.IsValid {
				return &ParameterValidationError{Result: pvr}
			}
		}
	}

	return nil
}

// ValidateParameters checks parameters against the workflow schema.
// Mutates params in-place: strips undeclared parameters (except KA-managed).
// BR-HAPI-191: 8 constraint checks + undeclared stripping.
func (v *Validator) ValidateParameters(params map[string]interface{}, schema []models.WorkflowParameter) *ParameterValidationResult {
	return v.validateParameters(params, schema, nil)
}

func (v *Validator) validateParameters(params map[string]interface{}, schema []models.WorkflowParameter, compiledPatterns map[string]*regexp.Regexp) *ParameterValidationResult {
	result := &ParameterValidationResult{IsValid: true}

	stripUndeclaredParameters(params, schema, result)

	// Validate each declared parameter (BR-HAPI-191: 8 constraint checks).
	for _, p := range schema {
		if kaManagedParams[p.Name] {
			continue
		}

		val, exists := params[p.Name]

		// 1. Required check
		if !exists {
			if p.Required {
				result.Errors = append(result.Errors,
					fmt.Sprintf("%s: required parameter is missing", p.Name))
			}
			continue
		}

		// 2. Type check
		if !validateType(val, p.Type) {
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: expected type %s, got %T", p.Name, p.Type, val))
			continue // skip further checks if type is wrong
		}

		validateNumericBounds(val, p, result)                  // 3-4. Minimum / Maximum
		validateEnumMembership(val, p, result)                 // 5. Enum check
		validatePatternMatch(val, p, compiledPatterns, result) // 6. Pattern check
		validateDependsOn(params, p, result)                   // 7. DependsOn check
	}

	if len(result.Errors) > 0 {
		result.IsValid = false
		result.SchemaHint = FormatSchemaHint(schema)
	}

	return result
}

// stripUndeclaredParameters removes any params entry not present in schema
// (KA-managed parameters are always preserved), recording each stripped key
// on result for LLM feedback.
func stripUndeclaredParameters(params map[string]interface{}, schema []models.WorkflowParameter, result *ParameterValidationResult) {
	declared := make(map[string]bool, len(schema))
	for _, p := range schema {
		declared[p.Name] = true
	}

	var toDelete []string
	for k := range params {
		if kaManagedParams[k] {
			continue
		}
		if !declared[k] {
			toDelete = append(toDelete, k)
		}
	}
	for _, k := range toDelete {
		delete(params, k)
		result.StrippedParams = append(result.StrippedParams, k)
	}
}

// validateNumericBounds checks p.Minimum/p.Maximum for numeric-typed values.
func validateNumericBounds(val interface{}, p models.WorkflowParameter, result *ParameterValidationResult) {
	numVal, ok := toFloat64(val)
	if !ok {
		return
	}
	if p.Minimum != nil && numVal < *p.Minimum {
		result.Errors = append(result.Errors,
			fmt.Sprintf("%s: value %v is below minimum %v", p.Name, numVal, *p.Minimum))
	}
	if p.Maximum != nil && numVal > *p.Maximum {
		result.Errors = append(result.Errors,
			fmt.Sprintf("%s: value %v exceeds maximum %v", p.Name, numVal, *p.Maximum))
	}
}

// validateEnumMembership checks p.Enum membership for string-typed values.
func validateEnumMembership(val interface{}, p models.WorkflowParameter, result *ParameterValidationResult) {
	if len(p.Enum) == 0 {
		return
	}
	strVal, ok := val.(string)
	if !ok {
		return
	}
	for _, allowed := range p.Enum {
		if strVal == allowed {
			return
		}
	}
	result.Errors = append(result.Errors,
		fmt.Sprintf("%s: value %q not in enum [%s]", p.Name, strVal, strings.Join(p.Enum, ", ")))
}

// validatePatternMatch checks p.Pattern against string-typed values,
// compiling (but, matching pre-refactor behavior, not caching back into)
// compiledPatterns on demand.
func validatePatternMatch(val interface{}, p models.WorkflowParameter, compiledPatterns map[string]*regexp.Regexp, result *ParameterValidationResult) {
	if p.Pattern == "" {
		return
	}
	strVal, ok := val.(string)
	if !ok {
		return
	}
	if len(p.Pattern) > maxPatternLength {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%s: pattern exceeds %d chars, skipping pattern validation", p.Name, maxPatternLength))
		return
	}
	re := compiledPatterns[p.Name]
	if re == nil {
		var err error
		re, err = regexp.Compile(p.Pattern)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: invalid regex pattern %q, skipping pattern validation", p.Name, p.Pattern))
			return
		}
	}
	if !re.MatchString(strVal) {
		result.Errors = append(result.Errors,
			fmt.Sprintf("%s: value %q does not match pattern %q", p.Name, strVal, p.Pattern))
	}
}

// validateDependsOn checks that every parameter p.DependsOn names is present
// in params.
func validateDependsOn(params map[string]interface{}, p models.WorkflowParameter, result *ParameterValidationResult) {
	for _, dep := range p.DependsOn {
		if _, depExists := params[dep]; !depExists {
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: depends on %q which is not present", p.Name, dep))
		}
	}
}

// FormatSchemaHint produces a human-readable schema description for LLM feedback.
// KA-managed parameters are excluded from the hint.
func FormatSchemaHint(schema []models.WorkflowParameter) string {
	if len(schema) == 0 {
		return "No parameter schema available."
	}

	var sb strings.Builder
	sb.WriteString("Expected parameters:\n")
	count := 0
	for _, p := range schema {
		if kaManagedParams[p.Name] {
			continue
		}
		count++
		sb.WriteString(fmt.Sprintf("  - %s (%s", p.Name, p.Type))
		if p.Required {
			sb.WriteString(", required")
		}
		if p.Minimum != nil {
			sb.WriteString(fmt.Sprintf(", min=%v", *p.Minimum))
		}
		if p.Maximum != nil {
			sb.WriteString(fmt.Sprintf(", max=%v", *p.Maximum))
		}
		if len(p.Enum) > 0 {
			sb.WriteString(fmt.Sprintf(", enum=[%s]", strings.Join(p.Enum, ", ")))
		}
		if p.Pattern != "" {
			sb.WriteString(fmt.Sprintf(", pattern=%q", p.Pattern))
		}
		sb.WriteString(")\n")
	}
	if count == 0 {
		return "No parameter schema available."
	}
	return sb.String()
}

func validateType(val interface{}, expectedType string) bool {
	switch expectedType {
	case "string":
		_, ok := val.(string)
		return ok
	case "integer":
		f, ok := val.(float64)
		if !ok {
			return false
		}
		return f == math.Trunc(f)
	case "float":
		_, ok := val.(float64)
		return ok
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "array":
		_, ok := val.([]interface{})
		return ok
	default:
		return true
	}
}

func toFloat64(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

// SelfCorrect runs a validation-correction loop up to maxAttempts times.
// Returns the corrected result with ValidationAttemptsHistory populated.
// If exhausted, sets HumanReviewNeeded + HumanReviewReason and clears WorkflowID
// per DD-HAPI-002 v1.2 (invalid workflows must not propagate to execution).
//
// The loop performs exactly maxAttempts validation checks. For each failed check
// except the last, it invokes correctionFn to request a new LLM response.
// History contains exactly maxAttempts entries when exhausted.
func (v *Validator) SelfCorrect(result *katypes.InvestigationResult, maxAttempts int,
	correctionFn func(result *katypes.InvestigationResult, err error) (*katypes.InvestigationResult, error),
) (*katypes.InvestigationResult, error) {
	current := result
	var history []katypes.ValidationAttemptRecord
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		validationErr := v.Validate(current)
		if validationErr == nil {
			history = append(history, katypes.ValidationAttemptRecord{
				Attempt:    attempt + 1,
				WorkflowID: current.WorkflowID,
				IsValid:    true,
				Timestamp:  time.Now().UTC().Format(time.RFC3339),
			})
			current.ValidationAttemptsHistory = history
			return current, nil
		}

		lastErr = validationErr

		var errStrings []string
		var paramErr *ParameterValidationError
		if errors.As(validationErr, &paramErr) {
			errStrings = paramErr.Result.Errors
		} else {
			errStrings = []string{validationErr.Error()}
		}

		history = append(history, katypes.ValidationAttemptRecord{
			Attempt:    attempt + 1,
			WorkflowID: current.WorkflowID,
			IsValid:    false,
			Errors:     errStrings,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		})

		if attempt < maxAttempts-1 {
			corrected, corrErr := correctionFn(current, validationErr)
			if corrErr != nil {
				return nil, fmt.Errorf("correction function failed on attempt %d: %w", attempt+1, corrErr)
			}
			current = corrected
		}
	}

	current.ValidationAttemptsHistory = history
	current.HumanReviewNeeded = true
	current.HumanReviewReason = "llm_parsing_error"
	current.Reason = fmt.Sprintf("self-correction exhausted after %d attempts: %s", maxAttempts, lastErr)
	current.WorkflowID = ""
	current.ExecutionBundle = ""
	return current, nil
}
