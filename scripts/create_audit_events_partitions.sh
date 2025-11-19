#!/bin/bash
# Copyright 2025 Jordi Gil.
# SPDX-License-Identifier: Apache-2.0

# ========================================
# Audit Events Monthly Partition Creator
# ========================================
#
# Purpose: Create future monthly partitions for audit_events table
# Authority: ADR-034 Unified Audit Table Design
# BR-STORAGE-032: Unified audit trail
# Version: 5.7 (Phase 1 - Day 21)
#
# Usage:
#   # Create default 3 future partitions
#   ./create_audit_events_partitions.sh
#
#   # Create custom number of future partitions
#   ./create_audit_events_partitions.sh 6
#
#   # Use custom connection string
#   DB_CONN_STR="postgresql://user:pass@host:5432/dbname" ./create_audit_events_partitions.sh
#
# Cron Setup (monthly execution):
#   0 0 1 * * /path/to/create_audit_events_partitions.sh
#
# ========================================

set -euo pipefail

# ========================================
# CONFIGURATION
# ========================================

# Number of future months to create (default: 3)
FUTURE_MONTHS=${1:-3}

# Database connection string (override via environment variable)
DB_CONN_STR=${DB_CONN_STR:-"postgresql://db_user:db_password@localhost:5432/action_history"}

# Log file (optional)
LOG_FILE=${LOG_FILE:-"/var/log/kubernaut/audit_partitions.log"}
LOG_DIR=$(dirname "$LOG_FILE")

# ========================================
# LOGGING FUNCTIONS
# ========================================

log() {
    local level=$1
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    # Log to console
    echo "[$timestamp] [$level] $message"

    # Log to file if directory exists
    if [ -d "$LOG_DIR" ]; then
        echo "[$timestamp] [$level] $message" >> "$LOG_FILE"
    fi
}

log_info() {
    log "INFO" "$@"
}

log_error() {
    log "ERROR" "$@"
}

log_success() {
    log "SUCCESS" "$@"
}

# ========================================
# VALIDATION
# ========================================

validate_postgres() {
    log_info "Validating PostgreSQL connection..."

    if ! command -v psql &> /dev/null; then
        log_error "psql command not found. Please install PostgreSQL client."
        exit 1
    fi

    if ! psql "$DB_CONN_STR" -c "SELECT 1" &> /dev/null; then
        log_error "Cannot connect to PostgreSQL. Connection string: $DB_CONN_STR"
        exit 1
    fi

    log_success "PostgreSQL connection validated"
}

validate_table_exists() {
    log_info "Validating audit_events table exists..."

    local table_exists=$(psql "$DB_CONN_STR" -t -c \
        "SELECT EXISTS(SELECT 1 FROM pg_tables WHERE tablename = 'audit_events')")

    if [[ "$table_exists" != *"t"* ]]; then
        log_error "audit_events table does not exist. Run migration 013 first."
        exit 1
    fi

    log_success "audit_events table exists"
}

# ========================================
# PARTITION MANAGEMENT
# ========================================

get_existing_partitions() {
    log_info "Checking existing partitions..."

    psql "$DB_CONN_STR" -t -c \
        "SELECT c.relname
         FROM pg_inherits i
         JOIN pg_class c ON c.oid = i.inhrelid
         WHERE i.inhparent = 'audit_events'::regclass
         ORDER BY c.relname" | tr -d ' '
}

calculate_partition_dates() {
    local months_ahead=$1
    local partition_dates=()

    for ((i=0; i<months_ahead; i++)); do
        # Calculate first day of target month
        local partition_date=$(date -u -d "$(date +%Y-%m-01) + $i months" '+%Y-%m-01')
        partition_dates+=("$partition_date")
    done

    echo "${partition_dates[@]}"
}

create_partition() {
    local start_date=$1
    local partition_name="audit_events_$(date -d "$start_date" '+%Y_%m')"

    # Calculate end date (first day of next month)
    local end_date=$(date -u -d "$start_date + 1 month" '+%Y-%m-%d')

    log_info "Creating partition: $partition_name (range: $start_date to $end_date)"

    # Check if partition already exists
    local exists=$(psql "$DB_CONN_STR" -t -c \
        "SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = '$partition_name')")

    if [[ "$exists" == *"t"* ]]; then
        log_info "Partition $partition_name already exists, skipping"
        return 0
    fi

    # Create partition
    psql "$DB_CONN_STR" -c \
        "CREATE TABLE IF NOT EXISTS $partition_name PARTITION OF audit_events
         FOR VALUES FROM ('$start_date') TO ('$end_date')" || {
        log_error "Failed to create partition: $partition_name"
        return 1
    }

    log_success "Created partition: $partition_name"
}

# ========================================
# MAIN EXECUTION
# ========================================

main() {
    log_info "=========================================="
    log_info "Audit Events Partition Creator"
    log_info "Target: $FUTURE_MONTHS future month(s)"
    log_info "=========================================="

    # Validate environment
    validate_postgres
    validate_table_exists

    # Get existing partitions
    log_info "Existing partitions:"
    local existing_partitions=$(get_existing_partitions)
    echo "$existing_partitions" | while read -r partition; do
        if [ -n "$partition" ]; then
            log_info "  - $partition"
        fi
    done

    # Calculate partition dates
    log_info "Calculating partition dates for next $FUTURE_MONTHS month(s)..."
    local partition_dates=($(calculate_partition_dates $FUTURE_MONTHS))

    # Create partitions
    local created_count=0
    local skipped_count=0

    for start_date in "${partition_dates[@]}"; do
        if create_partition "$start_date"; then
            ((created_count++)) || true
        else
            ((skipped_count++)) || true
        fi
    done

    # Summary
    log_info "=========================================="
    log_success "Partition creation complete"
    log_info "Created: $created_count"
    log_info "Skipped (already exists): $skipped_count"
    log_info "=========================================="

    # Show current partition count
    local total_partitions=$(psql "$DB_CONN_STR" -t -c \
        "SELECT COUNT(*) FROM pg_inherits i
         WHERE i.inhparent = 'audit_events'::regclass")
    log_info "Total partitions: $(echo $total_partitions | tr -d ' ')"
}

# Run main function
main

exit 0

