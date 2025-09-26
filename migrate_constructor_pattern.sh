#!/bin/bash

# Script to migrate NewIntelligentWorkflowBuilder calls to new config pattern
# Usage: ./migrate_constructor_pattern.sh [group_number]

set -e

echo "🔄 Constructor Pattern Migration Script"
echo "======================================"

# Define file groups for incremental migration
declare -a GROUP1=(
    "test/unit/workflow-engine/resource_optimization_integration_test.go"
    "test/unit/workflow-engine/ai_enhancement_integration_test.go"
    "test/unit/workflow-engine/validation_enhancement_integration_test.go"
    "test/unit/workflow-engine/advanced_analytics_integration_test.go"
    "test/unit/workflow-engine/advanced_scheduling_integration_test.go"
)

declare -a GROUP2=(
    "test/unit/workflow-engine/environment_adaptation_integration_test.go"
    "test/unit/workflow-engine/filter_executions_by_criteria_integration_test.go"
    "test/unit/workflow-engine/objective_analysis_activation_test.go"
    "test/unit/workflow-engine/analytics_integration_test.go"
    "test/unit/workflow-engine/performance_monitoring_integration_test.go"
)

declare -a GROUP3=(
    "test/unit/workflow-engine/pattern_discovery_integration_test.go"
    "test/unit/workflow-engine/security_enhancement_integration_test.go"
    "test/unit/workflow-engine/workflow_generation_validation_test.go"
    "test/unit/workflow-engine/advanced_analytics_unit_test.go"
    "test/unit/workflow-engine/resource_management_unit_test.go"
)

# Function to migrate a single file
migrate_file() {
    local file="$1"
    echo "  📝 Migrating: $file"
    
    if [[ ! -f "$file" ]]; then
        echo "    ⚠️  File not found: $file"
        return 1
    fi
    
    # Create backup
    cp "$file" "${file}.backup"
    
    # Pattern 1: Simple 7-parameter call (most common)
    # engine.NewIntelligentWorkflowBuilder(nil, mockVectorDB, nil, nil, nil, nil, log)
    sed -i '' 's/engine\.NewIntelligentWorkflowBuilder(\([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^)]*\))/\
		config := \&engine.IntelligentWorkflowBuilderConfig{\
			LLMClient:       \1,\
			VectorDB:        \2,\
			AnalyticsEngine: \3,\
			PatternStore:    \4,\
			ExecutionRepo:   \5,\
			Logger:          \7,\
		}\
		\
		var err error\
		builder, err = engine.NewIntelligentWorkflowBuilder(config)\
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")/g' "$file"
    
    # Check if migration was successful
    if grep -q "IntelligentWorkflowBuilderConfig" "$file"; then
        echo "    ✅ Migration successful"
        rm "${file}.backup"
        return 0
    else
        echo "    ❌ Migration failed, restoring backup"
        mv "${file}.backup" "$file"
        return 1
    fi
}

# Function to validate files compile
validate_group() {
    local -n files=$1
    echo "🔍 Validating group compilation..."
    
    for file in "${files[@]}"; do
        if ! go vet "$file" >/dev/null 2>&1; then
            echo "  ❌ Validation failed for: $file"
            return 1
        fi
    done
    
    echo "  ✅ All files in group validate successfully"
    return 0
}

# Main migration logic
GROUP_NUM=${1:-1}

case $GROUP_NUM in
    1)
        echo "📦 Migrating Group 1 (5 files)..."
        FILES=("${GROUP1[@]}")
        ;;
    2)
        echo "📦 Migrating Group 2 (5 files)..."
        FILES=("${GROUP2[@]}")
        ;;
    3)
        echo "📦 Migrating Group 3 (5 files)..."
        FILES=("${GROUP3[@]}")
        ;;
    *)
        echo "❌ Invalid group number. Use 1, 2, or 3"
        exit 1
        ;;
esac

# Migrate files in the group
SUCCESS_COUNT=0
TOTAL_COUNT=${#FILES[@]}

for file in "${FILES[@]}"; do
    if migrate_file "$file"; then
        ((SUCCESS_COUNT++))
    fi
done

echo ""
echo "📊 Migration Summary:"
echo "  ✅ Successful: $SUCCESS_COUNT/$TOTAL_COUNT files"
echo "  📁 Group: $GROUP_NUM"

if [[ $SUCCESS_COUNT -eq $TOTAL_COUNT ]]; then
    echo ""
    echo "🎉 Group $GROUP_NUM migration completed successfully!"
    echo "💡 Next steps:"
    echo "  1. Run: go build ./test/unit/workflow-engine/..."
    echo "  2. Run: make test (if desired)"
    echo "  3. If successful, run: ./migrate_constructor_pattern.sh $((GROUP_NUM + 1))"
else
    echo ""
    echo "⚠️  Some migrations failed. Please review and fix manually before proceeding."
fi
