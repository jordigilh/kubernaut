#!/bin/bash

# Bulk update script to replace infrastructure/types imports with shared/types
# This script aggressively removes backward compatibility and migrates everything to the new structure

echo "Starting bulk import update for Alert type migration..."

# List of files to update (from the find command)
files=(
    "cmd/test-slm/main.go"
    "cmd/test-context-performance/main.go"
    "test/unit/orchestration/ml_analyzer_test.go"
    "test/integration/core_integration/integration_test.go"
    "test/integration/end_to_end/end_to_end_recurring_alerts_test.go"
    "test/integration/performance_scale/stress_production_test.go"
    "test/integration/performance_scale/context_performance_test.go"
    "test/integration/shared/fake_slm_enhanced_decision.go"
    "test/integration/shared/enhanced_capabilities_demo.go"
    "test/integration/shared/fake_slm_enhancements.go"
    "test/integration/shared/database_test_utils.go"
    "test/integration/shared/example_enhanced_isolation_test.go"
    "test/integration/shared/integration_test_utils.go"
    "test/integration/shared/realistic_test_data.go"
    "test/integration/shared/fake_slm_client.go"
    "test/integration/validation_quality/confidence_consistency_test.go"
    "test/integration/validation_quality/prompt_validation_test.go"
    "test/integration/ai/system_integration_test.go"
    "test/integration/ai/alert_correlation_test.go"
    "test/integration/ai/shared_test_utils.go"
    "test/integration/ai/ai_pipeline_test.go"
    "test/integration/ai/workflow_orchestration_test.go"
    "test/integration/workflow_engine/template_factory_integration_test.go"
    "test/integration/workflow_engine/intelligent_workflow_builder_integration_support.go"
    "test/integration/fixtures/types.go"
    "test/integration/fixtures/core_alerts.go"
    "test/integration/fixtures/high_priority_alerts.go"
    "test/integration/fixtures/test_helpers.go"
    "test/integration/production_readiness/production_readiness_test.go"
    "scripts/context-performance-test.go"
    "debug_test.go"
    "pkg/intelligence/patterns/pattern_discovery_helpers.go"
    "pkg/intelligence/patterns/pattern_discovery_engine.go"
    "pkg/intelligence/patterns/pattern_discovery_data_collector_simple.go"
    "pkg/intelligence/learning/feature_extractor.go"
    "pkg/integration/webhook/handler_test.go"
    "pkg/integration/processor/processor_test.go"
    "pkg/integration/processor/processor.go"
    "pkg/integration/notifications/service.go"
    "pkg/integration/notifications/interfaces.go"
    "pkg/integration/notifications/builder.go"
    "pkg/platform/executor/executor_test.go"
    "pkg/platform/executor/registry.go"
    "pkg/platform/executor/registry_test.go"
    "pkg/platform/monitoring/interfaces.go"
    "pkg/platform/monitoring/stub_clients.go"
    "pkg/platform/monitoring/side_effect_detector_test.go"
    "pkg/platform/monitoring/side_effect_detector.go"
    "pkg/platform/monitoring/prometheus_client.go"
    "pkg/platform/monitoring/prometheus_client_test.go"
    "pkg/ai/insights/assessor.go"
    "pkg/ai/llm/prompt_engineering_test.go"
    "pkg/ai/llm/ai_response_processing_test.go"
    "pkg/ai/llm/enhanced_client.go"
    "pkg/ai/llm/client_comprehensive_test.go"
    "pkg/ai/llm/ai_response_processor.go"
    "pkg/ai/llm/ai_response_parser.go"
    "pkg/ai/llm/ai_enhanced_client_test.go"
    "pkg/ai/llm/client_test.go"
    "pkg/ai/llm/ai_response_processor_impl.go"
    "pkg/ai/conditions/ai_condition_evaluator_test.go"
    "pkg/ai/conditions/ai_condition_parsers.go"
    "pkg/ai/conditions/ai_condition_impl.go"
    "pkg/workflow/templates/template_factory_test.go"
    "pkg/workflow/templates/template_factory.go"
    "pkg/workflow/engine/intelligent_workflow_builder_helpers.go"
)

# Counter for tracking updates
updated_count=0
error_count=0

# Function to update imports in a file
update_file() {
    local file="$1"
    local full_path="/Users/jgil/go/src/github.com/jordigilh/kubernaut/$file"

    if [[ ! -f "$full_path" ]]; then
        echo "WARNING: File not found: $full_path"
        ((error_count++))
        return 1
    fi

    # Check if file contains the import we want to replace
    if grep -q '"github.com/jordigilh/kubernaut/pkg/infrastructure/types"' "$full_path"; then
        echo "Updating: $file"

        # Create a backup
        cp "$full_path" "${full_path}.backup"

        # Replace the import
        sed -i '' 's|"github.com/jordigilh/kubernaut/pkg/infrastructure/types"|"github.com/jordigilh/kubernaut/pkg/shared/types"|g' "$full_path"

        # Check if the replacement was successful
        if [[ $? -eq 0 ]]; then
            ((updated_count++))
            echo "  ✅ Successfully updated"
        else
            echo "  ❌ Failed to update, restoring backup"
            mv "${full_path}.backup" "$full_path"
            ((error_count++))
            return 1
        fi

        # Remove backup if successful
        rm -f "${full_path}.backup"
    else
        echo "Skipping: $file (no matching import found)"
    fi
}

# Update all files
echo "Updating ${#files[@]} files..."
echo

for file in "${files[@]}"; do
    update_file "$file"
done

echo
echo "Update Summary:"
echo "==============="
echo "Total files processed: ${#files[@]}"
echo "Successfully updated: $updated_count"
echo "Errors encountered: $error_count"
echo

if [[ $error_count -eq 0 ]]; then
    echo "🎉 All imports updated successfully!"
    echo "Next steps:"
    echo "1. Run 'go mod tidy' to clean up dependencies"
    echo "2. Run tests to ensure everything works"
    echo "3. Remove the old Alert type definitions completely"
else
    echo "⚠️  Some files had errors. Check the output above for details."
    echo "Backup files (.backup) have been preserved for failed updates."
fi

echo "Done!"
