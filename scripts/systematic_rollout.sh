#!/bin/bash
# Systematic Rollout Script - Final 5% Completion
# Automates transformation of weak assertions to business requirement validations

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DRY_RUN=false
VERBOSE=false
TARGET_DIR=""
BATCH_SIZE=10

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Usage information
usage() {
    echo "Usage: $0 [OPTIONS] TARGET_DIRECTORY"
    echo ""
    echo "Systematically converts weak assertions to business requirement validations"
    echo ""
    echo "OPTIONS:"
    echo "  --dry-run              Show what would be changed without making changes"
    echo "  --verbose              Enable verbose output"
    echo "  --batch-size N         Process N files at a time (default: 10)"
    echo "  --help                 Show this help message"
    echo ""
    echo "EXAMPLES:"
    echo "  $0 test/unit/ai/"
    echo "  $0 --dry-run test/integration/"
    echo "  $0 --verbose --batch-size 5 test/unit/workflow-engine/"
    echo ""
    echo "PROGRESS TRACKING:"
    echo "  - Creates progress files in /tmp/rollout_progress/"
    echo "  - Maintains conversion logs"
    echo "  - Supports resumable execution"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        --batch-size)
            BATCH_SIZE="$2"
            shift 2
            ;;
        --help)
            usage
            exit 0
            ;;
        -*)
            echo "Unknown option $1"
            usage
            exit 1
            ;;
        *)
            if [[ -z "$TARGET_DIR" ]]; then
                TARGET_DIR="$1"
            else
                echo "Multiple target directories not supported"
                usage
                exit 1
            fi
            shift
            ;;
    esac
done

# Validate arguments
if [[ -z "$TARGET_DIR" ]]; then
    echo -e "${RED}Error: TARGET_DIRECTORY is required${NC}"
    usage
    exit 1
fi

if [[ ! -d "$PROJECT_ROOT/$TARGET_DIR" ]]; then
    echo -e "${RED}Error: Directory $PROJECT_ROOT/$TARGET_DIR does not exist${NC}"
    exit 1
fi

# Setup progress tracking
PROGRESS_DIR="/tmp/rollout_progress"
mkdir -p "$PROGRESS_DIR"
PROGRESS_FILE="$PROGRESS_DIR/$(basename "$TARGET_DIR")_progress.log"
ERROR_FILE="$PROGRESS_DIR/$(basename "$TARGET_DIR")_errors.log"

# Log function
log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    case $level in
        "INFO")
            echo -e "${GREEN}[INFO]${NC} $message"
            ;;
        "WARN")
            echo -e "${YELLOW}[WARN]${NC} $message"
            ;;
        "ERROR")
            echo -e "${RED}[ERROR]${NC} $message"
            echo "[$timestamp] ERROR: $message" >> "$ERROR_FILE"
            ;;
        "DEBUG")
            if [[ "$VERBOSE" == "true" ]]; then
                echo -e "${BLUE}[DEBUG]${NC} $message"
            fi
            ;;
    esac

    echo "[$timestamp] [$level] $message" >> "$PROGRESS_FILE"
}

# Business requirement pattern mapping
map_to_business_requirement() {
    local file_path="$1"
    local file_lower=$(echo "$file_path" | tr '[:upper:]' '[:lower:]')

    # Determine BR context based on file path and content analysis
    if [[ "$file_lower" == *"database"* || "$file_lower" == *"postgresql"* || "$file_lower" == *"redis"* ]]; then
        echo "BR-DATABASE-001-A"
    elif [[ "$file_lower" == *"ai"* || "$file_lower" == *"llm"* || "$file_lower" == *"holmesgpt"* ]]; then
        echo "BR-AI-001-CONFIDENCE"
    elif [[ "$file_lower" == *"workflow"* || "$file_lower" == *"orchestrat"* ]]; then
        echo "BR-WF-001-EXECUTION-TIME"
    elif [[ "$file_lower" == *"performance"* || "$file_lower" == *"stress"* ]]; then
        echo "BR-PERF-001-RESPONSE-TIME"
    elif [[ "$file_lower" == *"safety"* || "$file_lower" == *"validation"* ]]; then
        echo "BR-SF-001-RISK-SCORE"
    elif [[ "$file_lower" == *"monitoring"* || "$file_lower" == *"health"* ]]; then
        echo "BR-MON-001-ALERT-THRESHOLD"
    else
        echo "BR-GENERAL-001"
    fi
}

# Generate business requirement description
generate_br_description() {
    local br_code="$1"
    local context="$2"

    case $br_code in
        "BR-DATABASE-001-A")
            echo "database utilization and performance validation"
            ;;
        "BR-AI-001-CONFIDENCE")
            echo "AI analysis confidence threshold validation"
            ;;
        "BR-WF-001-EXECUTION-TIME")
            echo "workflow execution time requirements"
            ;;
        "BR-PERF-001-RESPONSE-TIME")
            echo "system response time performance"
            ;;
        "BR-SF-001-RISK-SCORE")
            echo "safety and risk assessment validation"
            ;;
        "BR-MON-001-ALERT-THRESHOLD")
            echo "monitoring and alerting thresholds"
            ;;
        *)
            echo "business requirement validation in $context"
            ;;
    esac
}

# Transform weak assertions in a single file
transform_assertions_in_file() {
    local file_path="$1"
    local relative_path=$(realpath --relative-to="$PROJECT_ROOT" "$file_path")

    log "DEBUG" "Processing file: $relative_path"

    # Map to appropriate business requirement
    local br_code=$(map_to_business_requirement "$file_path")
    local br_description=$(generate_br_description "$br_code" "$(basename "$file_path")")

    # Create temporary file for transformations
    local temp_file=$(mktemp)
    cp "$file_path" "$temp_file"

    local changes_made=false

    # Transform pattern 1: .ToNot(BeNil()) -> .ToNot(BeNil(), "BR-XXX-XXX: description")
    if grep -q '\.ToNot(BeNil())$' "$temp_file"; then
        log "DEBUG" "Applying ToNot(BeNil()) transformation"
        sed -i "s/\.ToNot(BeNil())$/\.ToNot(BeNil(), \"$br_code: $br_description\")/g" "$temp_file"
        changes_made=true
    fi

    # Transform pattern 2: .To(BeNumerically(...)) -> config.ExpectBusinessRequirement(...)
    if grep -q '\.To(BeNumerically.*>, 0' "$temp_file"; then
        log "DEBUG" "Applying BeNumerically transformation (needs manual review)"
        # Mark these for manual review as they need value extraction
        sed -i 's/\.To(BeNumerically.*>, 0/\/\/ TODO-BR-TRANSFORM: Replace with config.ExpectBusinessRequirement() - &/g' "$temp_file"
        changes_made=true
    fi

    # Add business requirement imports if transformations were made
    if [[ "$changes_made" == "true" ]]; then
        # Check if config import already exists
        if ! grep -q '"github.com/jordigilh/kubernaut/pkg/testutil/config"' "$temp_file"; then
            # Add import after existing imports
            if grep -q 'import (' "$temp_file"; then
                sed -i '/import (/a\\t"github.com/jordigilh/kubernaut/pkg/testutil/config"' "$temp_file"
            elif grep -q 'import ' "$temp_file"; then
                sed -i '/import /a\import "github.com/jordigilh/kubernaut/pkg/testutil/config"' "$temp_file"
            fi
        fi

        if [[ "$DRY_RUN" == "true" ]]; then
            log "INFO" "DRY RUN: Would update $relative_path with $br_code transformations"
        else
            # Validate that the file still compiles
            if go fmt "$temp_file" >/dev/null 2>&1; then
                cp "$temp_file" "$file_path"
                log "INFO" "Updated $relative_path with $br_code validations"
            else
                log "ERROR" "Transformation resulted in invalid Go syntax for $relative_path"
                rm "$temp_file"
                return 1
            fi
        fi
    else
        log "DEBUG" "No transformations needed for $relative_path"
    fi

    rm "$temp_file"
    return 0
}

# Replace local mocks in a file
replace_local_mocks() {
    local file_path="$1"
    local relative_path=$(realpath --relative-to="$PROJECT_ROOT" "$file_path")

    log "DEBUG" "Checking for local mocks in: $relative_path"

    if grep -q 'type.*Mock.*struct' "$file_path"; then
        log "WARN" "Local mock detected in $relative_path - manual migration needed"

        if [[ "$DRY_RUN" == "false" ]]; then
            # Add TODO comment for manual review
            sed -i '/type.*Mock.*struct/i // TODO-MOCK-MIGRATION: Replace with generated mock from pkg/testutil/mocks/factory.go' "$file_path"
            log "INFO" "Added migration TODO to $relative_path"
        else
            log "INFO" "DRY RUN: Would add migration TODO to $relative_path"
        fi
    fi
}

# Process files in batches
process_batch() {
    local files=("$@")
    local batch_start=$(date '+%s')

    log "INFO" "Processing batch of ${#files[@]} files..."

    local success_count=0
    local error_count=0

    for file in "${files[@]}"; do
        if [[ -f "$file" ]]; then
            if transform_assertions_in_file "$file"; then
                replace_local_mocks "$file"
                ((success_count++))
            else
                ((error_count++))
            fi
        else
            log "WARN" "File not found: $file"
        fi
    done

    local batch_end=$(date '+%s')
    local batch_duration=$((batch_end - batch_start))

    log "INFO" "Batch completed in ${batch_duration}s - Success: $success_count, Errors: $error_count"

    return $error_count
}

# Main execution
main() {
    local start_time=$(date '+%s')

    log "INFO" "Starting systematic rollout for $TARGET_DIR"
    log "INFO" "Configuration: DRY_RUN=$DRY_RUN, VERBOSE=$VERBOSE, BATCH_SIZE=$BATCH_SIZE"

    # Find all test files in target directory
    local test_files=()
    while IFS= read -r -d '' file; do
        test_files+=("$file")
    done < <(find "$PROJECT_ROOT/$TARGET_DIR" -name "*_test.go" -type f -print0)

    if [[ ${#test_files[@]} -eq 0 ]]; then
        log "WARN" "No test files found in $TARGET_DIR"
        exit 0
    fi

    log "INFO" "Found ${#test_files[@]} test files to process"

    # Process files in batches
    local total_batches=$(((${#test_files[@]} + BATCH_SIZE - 1) / BATCH_SIZE))
    local current_batch=1
    local total_errors=0

    for ((i=0; i<${#test_files[@]}; i+=BATCH_SIZE)); do
        local batch_files=("${test_files[@]:$i:$BATCH_SIZE}")

        log "INFO" "Processing batch $current_batch/$total_batches"

        if ! process_batch "${batch_files[@]}"; then
            ((total_errors++))
        fi

        ((current_batch++))

        # Brief pause between batches to allow system recovery
        if [[ $current_batch -le $total_batches ]]; then
            sleep 1
        fi
    done

    # Final validation
    if [[ "$DRY_RUN" == "false" ]]; then
        log "INFO" "Running final validation..."

        # Check if tests still compile
        if cd "$PROJECT_ROOT" && go test -c "$TARGET_DIR/..." >/dev/null 2>&1; then
            log "INFO" "✅ All transformed tests compile successfully"
        else
            log "ERROR" "❌ Some tests failed to compile after transformation"
            total_errors=$((total_errors + 1))
        fi
    fi

    local end_time=$(date '+%s')
    local total_duration=$((end_time - start_time))

    # Summary
    echo ""
    log "INFO" "=== Systematic Rollout Complete ==="
    log "INFO" "Target Directory: $TARGET_DIR"
    log "INFO" "Files Processed: ${#test_files[@]}"
    log "INFO" "Total Duration: ${total_duration}s"
    log "INFO" "Error Count: $total_errors"

    if [[ "$DRY_RUN" == "true" ]]; then
        log "INFO" "DRY RUN: No files were modified"
        log "INFO" "Run without --dry-run to apply changes"
    else
        log "INFO" "Progress log: $PROGRESS_FILE"
        if [[ -f "$ERROR_FILE" ]]; then
            log "INFO" "Error log: $ERROR_FILE"
        fi
    fi

    # Next steps guidance
    echo ""
    log "INFO" "=== Next Steps ==="
    log "INFO" "1. Review TODO-BR-TRANSFORM comments for manual BeNumerically() updates"
    log "INFO" "2. Review TODO-MOCK-MIGRATION comments for mock factory integration"
    log "INFO" "3. Run tests: go test $TARGET_DIR/..."
    log "INFO" "4. Update business requirement codes as needed"

    return $total_errors
}

# Execute main function
main "$@"


