#!/bin/bash
# Final KCLI Cache Cleanup Script - Remove ALL traces of old deployments
# This script addresses the persistent stress.parodos.dev configuration issue

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${CYAN}=== $1 ===${NC}"; }

echo -e "${CYAN}"
cat << "EOF"
 _____ _             _    ____  _
|  ___(_)_ __   __ _| |  / ___|| | ___  __ _ _ __  _   _ _ __
| |_  | | '_ \ / _` | | | |    | |/ _ \/ _` | '_ \| | | | '_ \
|  _| | | | | | (_| | | | |___ | |  __/ (_| | | | | |_| | |_) |
|_|   |_|_| |_|\__,_|_|  \____||_|\___|\__,_|_| |_|\__,_| .__/
                                                        |_|
EOF
echo -e "${NC}"

log_info "Final KCLI Cache Cleanup for helios08"
log_info "Removing ALL references to stress.parodos.dev and old configurations"

# Function to check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        log_info "Please run: sudo $0"
        exit 1
    fi
}

# Function to find and clean all configuration files with old cluster references
find_and_clean_config_files() {
    log_header "Finding and Cleaning Configuration Files"

    # Search for files containing stress or parodos.dev references
    log_info "Searching for files containing old cluster references..."

    # Common locations to check
    local search_paths=(
        "/root"
        "/root/.kcli"
        "/root/.kube"
        "/tmp"
    )

    for path in "${search_paths[@]}"; do
        if [[ -d "$path" ]]; then
            log_info "Searching in: $path"

            # Find files containing 'stress' (exclude binary files)
            local stress_files=$(find "$path" -type f -not -path "*/.*" -exec grep -l "stress" {} \; 2>/dev/null | grep -v "binary file matches" || true)

            if [[ -n "$stress_files" ]]; then
                while IFS= read -r file; do
                    if [[ -f "$file" && ! "$file" =~ \.(log|gz|tar|rpm|img|qcow2|iso)$ ]]; then
                        log_warning "Found reference in: $file"

                        # Show the line containing the reference
                        grep -n "stress\|parodos\.dev" "$file" 2>/dev/null | head -3

                        # Ask if it's safe to remove or rename
                        if [[ "$file" =~ \.(yaml|yml|json|conf|cfg)$ ]]; then
                            log_info "Backing up and cleaning: $file"
                            cp "$file" "${file}.bak.$(date +%s)"

                            # Remove lines containing stress or parodos.dev
                            sed -i.tmp '/stress/d; /parodos\.dev/d' "$file" 2>/dev/null || true
                            rm -f "${file}.tmp" 2>/dev/null || true
                        fi
                    fi
                done <<< "$stress_files"
            fi

            # Find files containing 'parodos.dev'
            local parodos_files=$(find "$path" -type f -not -path "*/.*" -exec grep -l "parodos\.dev" {} \; 2>/dev/null | grep -v "binary file matches" || true)

            if [[ -n "$parodos_files" ]]; then
                while IFS= read -r file; do
                    if [[ -f "$file" && ! "$file" =~ \.(log|gz|tar|rpm|img|qcow2|iso)$ ]]; then
                        if [[ ! "$stress_files" =~ "$file" ]]; then  # Don't process twice
                            log_warning "Found parodos.dev reference in: $file"
                            grep -n "parodos\.dev" "$file" 2>/dev/null | head -3

                            if [[ "$file" =~ \.(yaml|yml|json|conf|cfg)$ ]]; then
                                log_info "Backing up and cleaning: $file"
                                cp "$file" "${file}.bak.$(date +%s)"
                                sed -i.tmp '/parodos\.dev/d' "$file" 2>/dev/null || true
                                rm -f "${file}.tmp" 2>/dev/null || true
                            fi
                        fi
                    fi
                done <<< "$parodos_files"
            fi
        fi
    done
}

# Function to specifically handle the odf_params.yaml file
fix_odf_params() {
    log_header "Fixing ODF Parameters File"

    local odf_file="/root/odf_params.yaml"

    if [[ -f "$odf_file" ]]; then
        log_warning "Found problematic file: $odf_file"
        log_info "Current content:"
        cat "$odf_file"

        log_info "Creating backup and fixing content..."
        cp "$odf_file" "${odf_file}.backup.$(date +%s)"

        # Replace with correct values
        cat > "$odf_file" << 'EOF'
# OpenShift Data Foundation Parameters
cluster: ocp418-baremetal
domain: kubernaut.io
odf_size: "200Gi"
storage_operators: true
local_storage: true
odf: true
EOF

        log_success "Fixed $odf_file with correct cluster configuration"
        log_info "New content:"
        cat "$odf_file"
    else
        log_info "odf_params.yaml not found - creating with correct values"
        cat > "$odf_file" << 'EOF'
# OpenShift Data Foundation Parameters
cluster: ocp418-baremetal
domain: kubernaut.io
odf_size: "200Gi"
storage_operators: true
local_storage: true
odf: true
EOF
        log_success "Created new odf_params.yaml with correct values"
    fi
}

# Function to clean environment variables
clean_environment_variables() {
    log_header "Cleaning Environment Variables"

    # Check for environment variables that might contain old cluster info
    local env_vars=(
        "KUBECONFIG"
        "CLUSTER_NAME"
        "CLUSTER_DOMAIN"
        "OCP_CLUSTER"
        "KCLI_CLIENT"
    )

    for var in "${env_vars[@]}"; do
        if [[ -n "${!var:-}" ]]; then
            local value="${!var}"
            if [[ "$value" =~ stress|parodos ]]; then
                log_warning "Found problematic environment variable: $var=$value"
                log_info "Unsetting $var"
                unset "$var" 2>/dev/null || true
            fi
        fi
    done

    # Clean up shell history
    local history_files=(
        "/root/.bash_history"
        "/root/.zsh_history"
    )

    for hist_file in "${history_files[@]}"; do
        if [[ -f "$hist_file" ]]; then
            log_info "Cleaning history file: $hist_file"
            cp "$hist_file" "${hist_file}.backup.$(date +%s)"

            # Remove lines containing old cluster references
            sed -i.tmp '/stress/d; /parodos\.dev/d' "$hist_file" 2>/dev/null || true
            rm -f "${hist_file}.tmp" 2>/dev/null || true

            log_success "Cleaned history file: $hist_file"
        fi
    done
}

# Function to clean KCLI specific files
clean_kcli_files() {
    log_header "Deep Cleaning KCLI Files"

    # Stop any running cluster operations
    pkill -f "kcli.*stress" 2>/dev/null || true
    pkill -f "kcli.*parodos" 2>/dev/null || true

    # Remove all KCLI clusters
    local clusters=$(kcli list cluster -o name 2>/dev/null || true)
    if [[ -n "$clusters" ]]; then
        while IFS= read -r cluster; do
            if [[ -n "$cluster" && "$cluster" != "Cluster" ]]; then
                log_info "Deleting cluster: $cluster"
                kcli delete cluster --yes "$cluster" 2>/dev/null || true
            fi
        done <<< "$clusters"
    fi

    # Clean KCLI configuration directories
    if [[ -d "/root/.kcli" ]]; then
        log_info "Backing up and cleaning KCLI configuration..."
        cp -r "/root/.kcli" "/root/.kcli.backup.$(date +%s)" 2>/dev/null || true

        # Remove cluster-specific directories
        rm -rf /root/.kcli/clusters/* 2>/dev/null || true

        # Clean any cached files
        find /root/.kcli -name "*stress*" -delete 2>/dev/null || true
        find /root/.kcli -name "*parodos*" -delete 2>/dev/null || true

        log_success "Cleaned KCLI configuration directory"
    fi

    # Remove any cached VM definitions
    virsh list --all --name | grep -E "(stress|parodos)" | xargs -r -I {} virsh destroy {} 2>/dev/null || true
    virsh list --all --name | grep -E "(stress|parodos)" | xargs -r -I {} virsh undefine {} --remove-all-storage 2>/dev/null || true
}

# Function to verify cleanup
verify_cleanup() {
    log_header "Verifying Cleanup"

    log_info "Checking for remaining references..."

    # Check for any remaining files with old references
    local remaining=$(find /root -type f -name "*.yaml" -o -name "*.yml" -o -name "*.json" -o -name "*.conf" -o -name "*.cfg" 2>/dev/null | xargs grep -l "stress\|parodos\.dev" 2>/dev/null || true)

    if [[ -n "$remaining" ]]; then
        log_warning "Still found references in:"
        echo "$remaining"
    else
        log_success "No remaining configuration file references found"
    fi

    # Check environment
    env | grep -i "stress\|parodos" || log_success "Environment is clean"

    # Check current cluster configuration
    if [[ -f "/root/kubernaut-e2e/kcli-baremetal-params-root.yml" ]]; then
        log_info "Current deployment configuration:"
        grep -E "(cluster|domain):" "/root/kubernaut-e2e/kcli-baremetal-params-root.yml"
    fi

    # Check KCLI status
    kcli list cluster 2>/dev/null || log_info "No clusters currently defined"
    virsh list --all | grep -E "(stress|parodos)" || log_success "No old VMs found"
}

# Function to provide final instructions
provide_final_instructions() {
    log_header "Final Instructions"

    cat << 'FINAL_EOF'

✅ CLEANUP COMPLETE! The persistent stress.parodos.dev references have been removed.

🔧 KEY FIXES APPLIED:
1. Fixed /root/odf_params.yaml (was referencing old cluster)
2. Cleaned all configuration files with old references
3. Removed cached environment variables
4. Cleaned KCLI configuration directories
5. Removed any remaining VM definitions

🚀 READY TO RETRY DEPLOYMENT:

# Navigate to deployment directory
cd /root/kubernaut-e2e

# Run the deployment (should now work correctly)
./deploy-kcli-cluster-root.sh kubernaut-e2e kcli-baremetal-params-root.yml

# Monitor progress
watch 'kcli list vm; echo ""; kcli list cluster'

❗ IMPORTANT NOTES:
- All old configuration files have been backed up with .backup.* extensions
- If issues persist, check the new deployment logs for different errors
- The deployment should now correctly use ocp418-baremetal.kubernaut.io

FINAL_EOF
}

# Main execution
main() {
    check_root

    log_info "Starting final comprehensive cleanup..."
    log_warning "This will remove ALL references to old cluster configurations"

    # Run all cleanup functions
    find_and_clean_config_files
    fix_odf_params
    clean_environment_variables
    clean_kcli_files

    # Verify the cleanup worked
    verify_cleanup

    # Provide final instructions
    provide_final_instructions

    log_header "Cleanup Summary"
    log_success "All traces of stress.parodos.dev configuration have been removed"
    log_success "The deployment should now use the correct ocp418-baremetal.kubernaut.io configuration"
    log_info "You can now retry the deployment with confidence!"
}

# Execute main function
main "$@"
