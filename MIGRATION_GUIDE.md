# Type Migration Guide

This document outlines the migration of duplicate types to consolidated shared types.

## Overview

We have identified and consolidated duplicate types across the codebase to improve maintainability, reduce confusion, and prevent type casting errors.

## Phase 1: Exact Duplicates (COMPLETED)

### Moved to `pkg/shared/types/common.go`:

#### `UtilizationTrend`
- **Previously in**:
  - `pkg/intelligence/patterns/pattern_discovery_helpers.go`
  - `pkg/intelligence/learning/time_series_analyzer.go`
- **Now in**: `pkg/shared/types.UtilizationTrend`
- **Status**: ✅ MIGRATED

#### `ConfidenceInterval`
- **Previously in**:
  - `pkg/intelligence/patterns/pattern_discovery_helpers.go`
  - `pkg/intelligence/learning/time_series_analyzer.go`
- **Now in**: `pkg/shared/types.ConfidenceInterval`
- **Status**: ✅ MIGRATED

#### `ResourceUsageData`
- **Previously in**:
  - `pkg/intelligence/patterns/pattern_discovery_helpers.go`
  - `pkg/workflow/types/core.go`
- **Now in**: `pkg/shared/types.ResourceUsageData`
- **Status**: ✅ MIGRATED

## Phase 3: Conflicting Definitions (IN PROGRESS)

### Moved to `pkg/shared/types/workflow.go`:

#### `WorkflowTemplate`
- **Previously in**:
  - `pkg/intelligence/patterns/pattern_discovery_helpers.go` - had Version, Variables, Tags
  - `pkg/workflow/types/core.go` - had Metadata, more complete structure
- **Now in**: `pkg/shared/types.WorkflowTemplate`
- **Consolidated features**: Includes all fields from both versions
- **Status**: ✅ CANONICAL VERSION CREATED, DEPRECATION COMMENTS ADDED

#### `WorkflowStep`
- **Previously in**:
  - `pkg/intelligence/patterns/pattern_discovery_helpers.go` - had Timeout
  - `pkg/workflow/types/core.go` - had Action, Conditions, Parameters
- **Now in**: `pkg/shared/types.WorkflowStep`
- **Consolidated features**: Includes all fields from both versions
- **Status**: ✅ CANONICAL VERSION CREATED, DEPRECATION COMMENTS ADDED

#### `OptimizationSuggestion`
- **Previously in**:
  - `pkg/intelligence/patterns/pattern_discovery_helpers.go` - had Impact (string), Effort (string)
  - `pkg/workflow/types/core.go` - had ExpectedImprovement (float64), ImplementationEffort (string)
- **Now in**: `pkg/shared/types.OptimizationSuggestion`
- **Consolidated features**: Includes both qualitative and quantitative assessment fields
- **Status**: ✅ CANONICAL VERSION CREATED, DEPRECATION COMMENTS ADDED

## Migration Steps for Developers

### Immediate Actions Required

1. **Update Imports**: Add `sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"` to any file using these types

2. **Replace Type References**:
   ```go
   // Old
   var trend UtilizationTrend

   // New
   var trend sharedtypes.UtilizationTrend
   ```

3. **Update Function Signatures**:
   ```go
   // Old
   func analyze(data ResourceUsageData) error

   // New
   func analyze(data sharedtypes.ResourceUsageData) error
   ```

### Type Conversion Examples

#### WorkflowTemplate Migration
```go
// Old pattern_discovery_helpers.go style
oldTemplate := WorkflowTemplate{
    ID:          "test",
    Name:        "Test Template",
    Version:     "1.0",
    Variables:   map[string]interface{}{"key": "value"},
    Tags:        []string{"test"},
}

// New canonical version
newTemplate := sharedtypes.WorkflowTemplate{
    ID:          "test",
    Name:        "Test Template",
    Version:     "1.0",              // Preserved from helpers version
    Variables:   map[string]interface{}{"key": "value"}, // Preserved
    Tags:        []string{"test"},    // Preserved
    Metadata:    map[string]interface{}{},               // Added from core version
}
```

#### OptimizationSuggestion Migration
```go
// Old helpers version (qualitative)
oldSuggestion := OptimizationSuggestion{
    Type:        "performance",
    Description: "Improve caching",
    Impact:      "high",       // qualitative
    Effort:      "medium",     // qualitative
    Priority:    1,
}

// New canonical version (supports both)
newSuggestion := sharedtypes.OptimizationSuggestion{
    Type:                 "performance",
    Description:          "Improve caching",
    Impact:               "high",                    // Qualitative (preserved)
    ExpectedImprovement:  0.25,                     // Quantitative (added)
    Effort:               "medium",                  // Qualitative (preserved)
    ImplementationEffort: "2-3 days",               // Detailed (added)
    Priority:             1,
}
```

## Breaking Changes

### WorkflowStep.Conditions Type Change
- **Old**: `Conditions []*WorkflowCondition`
- **New**: `Conditions []WorkflowCondition`
- **Action**: Remove pointer indirection when migrating

### New Required Fields
Some consolidated types have new optional fields that weren't in all original versions. These are marked with `omitempty` so existing code should continue working.

## Testing

After migration:
1. Run `go build ./pkg/shared/types/...` to test new types compile
2. Run tests for packages using migrated types
3. Check for any unused import warnings

## Timeline

- ✅ **Phase 1**: Exact duplicates consolidated
- 🔄 **Phase 3**: Conflicting definitions resolved (IN PROGRESS)
- 📋 **Phase 4**: Clean up deprecated types (PENDING)

## Support

If you encounter issues during migration:
1. Check this guide for examples
2. Ensure you're using the correct import alias
3. Verify field mappings for conflicting types
4. Test compilation of affected packages

## Future Cleanup

Once all code has been migrated to use shared types:
- Remove deprecated type definitions
- Remove deprecation warnings
- Update documentation to reference canonical types
