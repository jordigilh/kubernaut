#!/bin/bash
# gopls Refactoring Helper Script
# Simplifies common gopls refactoring operations with safety checks

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
function print_error() {
    echo -e "${RED}❌ Error: $1${NC}" >&2
}

function print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

function print_warning() {
    echo -e "${YELLOW}⚠️  Warning: $1${NC}"
}

function print_info() {
    echo "ℹ️  $1"
}

# Check prerequisites
function check_prerequisites() {
    if ! command -v gopls &> /dev/null; then
        print_error "gopls not found. Install with: go install golang.org/x/tools/gopls@latest"
        exit 1
    fi
    
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        print_warning "Not in a git repository. Reverting changes will be harder."
    fi
}

# Validate build after refactoring
function validate_build() {
    print_info "Validating build..."
    
    if go build ./... 2>&1 >/dev/null; then
        print_success "Build successful"
        return 0
    else
        print_error "Build failed after refactoring"
        return 1
    fi
}

# Quick test compile
function validate_tests() {
    print_info "Quick test compile check..."
    
    if go test ./... -run='^$' -timeout=30s 2>&1 >/dev/null; then
        print_success "Test compilation successful"
        return 0
    else
        print_error "Test compilation failed"
        return 1
    fi
}

# Show usage
function usage() {
    cat << EOF
gopls Refactoring Helper

Usage: $0 <command> [options]

Commands:
  rename <file> <line> <col> <newname>
      Rename a symbol at the specified position
      Example: $0 rename pkg/calc/math.go 10 5 Add

  move-package <file> <newpackage>
      Move/rename a package (position assumed at line 1, col 8)
      Example: $0 move-package pkg/oldpkg/file.go newpkg

  inline-call <file> <line> <col>
      Inline a function call at the specified position
      Example: $0 inline-call pkg/calc/math.go 20 10

  extract-function <file> <startline> <startcol> <endline> <endcol>
      Extract code into a new function
      Example: $0 extract-function pkg/calc/math.go 10 0 20 5

  remove-param <file> <line> <col>
      Remove unused parameter at position
      Example: $0 remove-param pkg/calc/math.go 10 15

  list-actions <file> <line> <col>
      List available code actions at position
      Example: $0 list-actions pkg/calc/math.go 10 5

  validate
      Run build and test validation (without refactoring)
      Example: $0 validate

Options:
  --no-validate    Skip build validation after refactoring
  --help           Show this help message

Safety Features:
  - Checks for gopls installation
  - Warns if not in git repository
  - Validates build after refactoring
  - Runs quick test compilation
  - Shows git diff after changes

EOF
}

# Parse global options
SKIP_VALIDATION=false
for arg in "$@"; do
    case $arg in
        --no-validate)
            SKIP_VALIDATION=true
            shift
            ;;
        --help|-h)
            usage
            exit 0
            ;;
    esac
done

# Main command dispatch
COMMAND=${1:-}
shift || true

case "$COMMAND" in
    rename)
        check_prerequisites
        
        if [ $# -ne 4 ]; then
            print_error "Usage: $0 rename <file> <line> <col> <newname>"
            exit 1
        fi
        
        FILE=$1
        LINE=$2
        COL=$3
        NEWNAME=$4
        
        if [ ! -f "$FILE" ]; then
            print_error "File not found: $FILE"
            exit 1
        fi
        
        print_info "Renaming symbol at $FILE:$LINE:$COL to '$NEWNAME'"
        
        if gopls rename -w "$FILE:$LINE:$COL" "$NEWNAME"; then
            print_success "Rename completed"
            
            if [ "$SKIP_VALIDATION" = false ]; then
                if ! validate_build; then
                    exit 1
                fi
                if ! validate_tests; then
                    exit 1
                fi
            fi
            
            print_info "Changes made:"
            git diff --stat 2>/dev/null || echo "(git not available)"
        else
            print_error "Rename failed"
            exit 1
        fi
        ;;
        
    move-package)
        check_prerequisites
        
        if [ $# -ne 2 ]; then
            print_error "Usage: $0 move-package <file> <newpackage>"
            exit 1
        fi
        
        FILE=$1
        NEWPACKAGE=$2
        
        if [ ! -f "$FILE" ]; then
            print_error "File not found: $FILE"
            exit 1
        fi
        
        print_warning "Moving package to '$NEWPACKAGE' - this moves all files and updates imports"
        print_info "Position assumed: $FILE:1:8 (package declaration)"
        
        if gopls rename -w "$FILE:1:8" "$NEWPACKAGE"; then
            print_success "Package move completed"
            
            if [ "$SKIP_VALIDATION" = false ]; then
                if ! validate_build; then
                    exit 1
                fi
                if ! validate_tests; then
                    exit 1
                fi
            fi
            
            print_info "Changes made:"
            git diff --stat 2>/dev/null || echo "(git not available)"
        else
            print_error "Package move failed"
            exit 1
        fi
        ;;
        
    inline-call)
        check_prerequisites
        
        if [ $# -ne 3 ]; then
            print_error "Usage: $0 inline-call <file> <line> <col>"
            exit 1
        fi
        
        FILE=$1
        LINE=$2
        COL=$3
        
        if [ ! -f "$FILE" ]; then
            print_error "File not found: $FILE"
            exit 1
        fi
        
        print_info "Inlining function call at $FILE:$LINE:$COL"
        
        if gopls codeaction -exec -kind refactor.inline.call "$FILE:$LINE:$COL"; then
            print_success "Inline completed"
            
            if [ "$SKIP_VALIDATION" = false ]; then
                if ! validate_build; then
                    exit 1
                fi
                if ! validate_tests; then
                    exit 1
                fi
            fi
        else
            print_error "Inline failed - may not be available at this position"
            exit 1
        fi
        ;;
        
    extract-function)
        check_prerequisites
        
        if [ $# -ne 5 ]; then
            print_error "Usage: $0 extract-function <file> <startline> <startcol> <endline> <endcol>"
            exit 1
        fi
        
        FILE=$1
        STARTLINE=$2
        STARTCOL=$3
        ENDLINE=$4
        ENDCOL=$5
        
        if [ ! -f "$FILE" ]; then
            print_error "File not found: $FILE"
            exit 1
        fi
        
        RANGE="$FILE:$STARTLINE:$STARTCOL-$ENDLINE:$ENDCOL"
        print_info "Extracting function from $RANGE"
        
        if gopls codeaction -exec -kind refactor.extract.function "$RANGE"; then
            print_success "Extract completed"
            print_warning "Extracted function may have default name 'newFunction' - consider renaming"
            
            if [ "$SKIP_VALIDATION" = false ]; then
                if ! validate_build; then
                    exit 1
                fi
                if ! validate_tests; then
                    exit 1
                fi
            fi
        else
            print_error "Extract failed - may not be available for this range"
            exit 1
        fi
        ;;
        
    remove-param)
        check_prerequisites
        
        if [ $# -ne 3 ]; then
            print_error "Usage: $0 remove-param <file> <line> <col>"
            exit 1
        fi
        
        FILE=$1
        LINE=$2
        COL=$3
        
        if [ ! -f "$FILE" ]; then
            print_error "File not found: $FILE"
            exit 1
        fi
        
        print_info "Removing unused parameter at $FILE:$LINE:$COL"
        
        if gopls codeaction -exec -kind refactor.rewrite.removeUnusedParam "$FILE:$LINE:$COL"; then
            print_success "Parameter removed - all call sites updated"
            
            if [ "$SKIP_VALIDATION" = false ]; then
                if ! validate_build; then
                    exit 1
                fi
                if ! validate_tests; then
                    exit 1
                fi
            fi
        else
            print_error "Remove parameter failed - parameter may be in use or not unused"
            exit 1
        fi
        ;;
        
    list-actions)
        check_prerequisites
        
        if [ $# -ne 3 ]; then
            print_error "Usage: $0 list-actions <file> <line> <col>"
            exit 1
        fi
        
        FILE=$1
        LINE=$2
        COL=$3
        
        if [ ! -f "$FILE" ]; then
            print_error "File not found: $FILE"
            exit 1
        fi
        
        print_info "Available code actions at $FILE:$LINE:$COL"
        gopls codeaction "$FILE:$LINE:$COL"
        ;;
        
    validate)
        print_info "Running validation..."
        if ! validate_build; then
            exit 1
        fi
        if ! validate_tests; then
            exit 1
        fi
        print_success "All validation checks passed"
        ;;
        
    *)
        if [ -n "$COMMAND" ]; then
            print_error "Unknown command: $COMMAND"
            echo ""
        fi
        usage
        exit 1
        ;;
esac
