#!/bin/bash

# Validate RunSpecs Coverage Script
# Ensures every integration test directory has a proper RunSpecs directive

set -euo pipefail

cd "$(dirname "$0")"

INTEGRATION_DIR="test/integration"
if [ "$#" -gt 0 ]; then
    INTEGRATION_DIR="$1"
fi

VALIDATION_LOG="runspecs_validation.log"

echo "🔍 Validating RunSpecs Coverage for Integration Tests"
echo "📁 Directory: $INTEGRATION_DIR"
echo "📄 Validation Log: $VALIDATION_LOG"
echo ""

# Initialize validation log
cat > "$VALIDATION_LOG" << EOF
RunSpecs Validation Log
Started: $(date)
Target Directory: $INTEGRATION_DIR

EOF

# Function to log with timestamp
log() {
    echo "[$(date '+%H:%M:%S')] $1" | tee -a "$VALIDATION_LOG"
}

# Function to check if directory has RunSpecs
check_runspecs() {
    local dir="$1"
    local dir_name=$(basename "$dir")

    # Skip if not a directory or excluded directories
    if [ ! -d "$dir" ]; then
        return 0
    fi

    # Skip excluded directories
    case "$dir_name" in
        "shared"|"fixtures"|"examples"|"scripts"|"init-vector-db.sql"|"init-vector-store.sql")
            return 0
            ;;
    esac

    # Check for test files in directory
    local test_files=$(find "$dir" -maxdepth 1 -name "*_test.go" -type f 2>/dev/null | wc -l)

    if [ "$test_files" -eq 0 ]; then
        log "⚠️  No test files found in $dir"
        return 0
    fi

    # Check for suite test files with RunSpecs
    local suite_files=$(find "$dir" -maxdepth 1 -name "*suite_test.go" -type f 2>/dev/null)
    local runspecs_found=false

    if [ -n "$suite_files" ]; then
        for suite_file in $suite_files; do
            if grep -q "RunSpecs(" "$suite_file" 2>/dev/null; then
                runspecs_found=true
                log "✅ $dir has RunSpecs in $(basename "$suite_file")"
                break
            fi
        done
    fi

    # Check for RunSpecs in any test file if not found in suite files
    if [ "$runspecs_found" = false ]; then
        local all_test_files=$(find "$dir" -maxdepth 1 -name "*_test.go" -type f 2>/dev/null)
        for test_file in $all_test_files; do
            if grep -q "RunSpecs(" "$test_file" 2>/dev/null; then
                runspecs_found=true
                log "✅ $dir has RunSpecs in $(basename "$test_file")"
                break
            fi
        done
    fi

    if [ "$runspecs_found" = false ]; then
        log "❌ MISSING: $dir lacks RunSpecs directive"
        echo "$dir" >> missing_runspecs.txt
        return 1
    fi

    return 0
}

# Function to create missing RunSpecs
create_missing_runspecs() {
    local dir="$1"
    local package_name=$(basename "$dir")

    # Create proper test function name (CamelCase)
    local test_func_name=$(echo "$package_name" | sed -e 's/_\([a-z]\)/\U\1/g' -e 's/^./\U&/')

    # Get a descriptive suite name
    local suite_name="$package_name"
    case "$package_name" in
        "ai") suite_name="AI Integration" ;;
        "orchestration") suite_name="Workflow Orchestration" ;;
        "vector_ai") suite_name="Vector AI Integration" ;;
        "infrastructure_integration") suite_name="Infrastructure Integration" ;;
        "external_services") suite_name="External Services Integration" ;;
        "health_monitoring") suite_name="Health Monitoring" ;;
        *) suite_name=$(echo "$package_name" | sed 's/_/ /g' | sed 's/\b\w/\U&/g') ;;
    esac

    local suite_file="$dir/${package_name}_suite_test.go"

    cat > "$suite_file" << EOF
//go:build integration
// +build integration

package $package_name

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-$(echo $package_name | tr '[:lower:]' '[:upper:]')-SUITE-001: $suite_name Test Suite Organization
// Business Impact: Ensures comprehensive validation of $suite_name business logic
// Stakeholder Value: Provides executive confidence in $suite_name testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in $suite_name capabilities
// Business Impact: Ensures all $suite_name components deliver measurable system reliability
// Business Outcome: Test suite framework enables $suite_name validation

func Test$test_func_name(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "$suite_name Suite")
}
EOF

    log "🔧 Created RunSpecs for $dir in $(basename "$suite_file")"
}

# Main validation logic
main() {
    log "Starting RunSpecs validation..."

    # Remove previous missing list
    [ -f missing_runspecs.txt ] && rm missing_runspecs.txt

    local total_dirs=0
    local valid_dirs=0
    local missing_dirs=0

    # Check all subdirectories
    for dir in "$INTEGRATION_DIR"/*; do
        if [ -d "$dir" ]; then
            total_dirs=$((total_dirs + 1))
            if check_runspecs "$dir"; then
                valid_dirs=$((valid_dirs + 1))
            else
                missing_dirs=$((missing_dirs + 1))
            fi
        fi
    done

    # Check nested subdirectories (for new structure)
    if [ -d "$INTEGRATION_DIR" ]; then
        for domain_dir in "$INTEGRATION_DIR"/*; do
            if [ -d "$domain_dir" ]; then
                for sub_dir in "$domain_dir"/*; do
                    if [ -d "$sub_dir" ]; then
                        total_dirs=$((total_dirs + 1))
                        if check_runspecs "$sub_dir"; then
                            valid_dirs=$((valid_dirs + 1))
                        else
                            missing_dirs=$((missing_dirs + 1))
                        fi
                    fi
                done
            fi
        done
    fi

    log ""
    log "📊 Validation Summary:"
    log "   Total directories checked: $total_dirs"
    log "   Directories with RunSpecs: $valid_dirs"
    log "   Directories missing RunSpecs: $missing_dirs"

    if [ "$missing_dirs" -gt 0 ]; then
        log ""
        log "❌ Missing RunSpecs in the following directories:"
        if [ -f missing_runspecs.txt ]; then
            while read -r missing_dir; do
                log "   - $missing_dir"
            done < missing_runspecs.txt
        fi

        echo ""
        read -p "Create missing RunSpecs files? (y/N): " create_missing

        if [[ $create_missing == [yY] ]]; then
            log ""
            log "🔧 Creating missing RunSpecs files..."

            if [ -f missing_runspecs.txt ]; then
                while read -r missing_dir; do
                    create_missing_runspecs "$missing_dir"
                done < missing_runspecs.txt
            fi

            log "✅ Missing RunSpecs files created"

            # Re-validate
            log ""
            log "🔄 Re-validating after creating missing files..."
            rm -f missing_runspecs.txt

            local new_valid=0
            local new_missing=0

            for dir in "$INTEGRATION_DIR"/*; do
                if [ -d "$dir" ]; then
                    if check_runspecs "$dir"; then
                        new_valid=$((new_valid + 1))
                    else
                        new_missing=$((new_missing + 1))
                    fi
                fi
            done

            log ""
            log "📊 Final Validation Summary:"
            log "   Directories with RunSpecs: $new_valid"
            log "   Directories still missing RunSpecs: $new_missing"
        fi
    else
        log ""
        log "🎉 All directories have proper RunSpecs directives!"
    fi

    # Test compilation
    log ""
    log "🧪 Testing Go compilation..."

    if cd "$INTEGRATION_DIR" && go test -tags=integration -list=. ./... >/dev/null 2>&1; then
        log "✅ Go compilation successful"
    else
        log "❌ Go compilation failed - check import paths and package declarations"
        cd - >/dev/null
        return 1
    fi

    cd - >/dev/null

    # Clean up
    [ -f missing_runspecs.txt ] && rm missing_runspecs.txt

    log ""
    log "✅ RunSpecs validation completed successfully!"

    return 0
}

# Run main function
main "$@"
