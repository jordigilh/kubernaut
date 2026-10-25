#!/bin/bash
# Copyright 2025 Jordi Gil
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Kubernaut Must-Gather - DataStorage API Collector
# BR-PLATFORM-001.6a: Collect workflow catalog and audit trail

set -euo pipefail

COLLECTION_DIR="${1:-}"
OUTPUT_DIR="${COLLECTION_DIR}/datastorage"

# shellcheck source=../utils/namespace.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../utils/namespace.sh"

# DataStorage API configuration
# Issue #2037: build the URL from RELEASE_NAMESPACE instead of a hardcoded
# "kubernaut-system" literal, so collection still reaches DataStorage when the
# chart is installed into a non-default Helm release namespace. The Service
# name itself is "data-storage-service" (operator-managed manifests/
# datastorage/datastorage.yaml), NOT "datastorage" -- confirmed against a
# real live-cluster install during this issue's own e2e-tests CI validation,
# where the old "datastorage" name silently resolved to nothing and every
# collection silently produced an empty error.json instead of real data.
DATASTORAGE_URL="${DATASTORAGE_URL:-https://data-storage-service.${RELEASE_NAMESPACE}.svc:8080}"
WORKFLOW_LIMIT="${WORKFLOW_LIMIT:-50}"
AUDIT_LIMIT="${AUDIT_LIMIT:-1000}"
AUDIT_TIMEFRAME="${AUDIT_TIMEFRAME:-${SINCE_DURATION:-24h}}"
TIMEOUT="${DATASTORAGE_TIMEOUT:-30}"
CA_FILE="${DATASTORAGE_CA_FILE:-${TLS_CA_FILE:-}}"
TOKEN_FILE="${DATASTORAGE_TOKEN_FILE:-/var/run/secrets/kubernetes.io/serviceaccount/token}"

if [[ -z "${COLLECTION_DIR}" ]]; then
    echo "Usage: $0 <collection-directory>"
    exit 1
fi

echo "Collecting DataStorage API data..."
mkdir -p "${OUTPUT_DIR}"

# Temporary credential/response files are never placed inside the archive tree.
umask 077
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/datastorage-collector.XXXXXX")
cleanup() {
    rm -f "${TEMP_DIR}"/*
    rmdir "${TEMP_DIR}"
}
trap cleanup EXIT
touch "${TEMP_DIR}/body" "${TEMP_DIR}/stderr"
SAFE_TOKEN_FILE=/dev/null
if [[ -r "${TOKEN_FILE}" ]]; then
    SAFE_TOKEN_FILE="${TOKEN_FILE}"
fi

# Function to handle API errors
handle_api_error() {
    local endpoint="$1"
    local exit_code="$2"
    local status="$3"
    local error_file="${4:-${OUTPUT_DIR}/error.json}"
    jq -n --arg endpoint "${endpoint}" --arg status "${status}" --argjson code "${exit_code}" \
        --arg timestamp "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
        --rawfile body "${TEMP_DIR}/body" --rawfile stderr "${TEMP_DIR}/stderr" \
        --rawfile secret "${SAFE_TOKEN_FILE}" '
        ($secret | rtrimstr("\n")) as $token |
        def safe: if $token == "" then . else split($token) | join("[REDACTED]") end;
        {error:"Failed to collect data from DataStorage API", endpoint:$endpoint,
         exit_code:$code, http_status:$status, message:($body | safe | .[:4096]),
         stderr:($stderr | safe | .[:4096]), timestamp:$timestamp}' > "${error_file}"
    echo "  Warning: DataStorage collection failed (curl exit ${exit_code}, HTTP ${status}); see ${error_file##*/}"
}

config_error() {
    printf '%s' "$1" > "${TEMP_DIR}/body"
    handle_api_error "configuration" 1 "000"
    exit 1
}

[[ "${AUDIT_TIMEFRAME}" =~ ^[1-9][0-9]*(s|m|h)$ ]] || config_error "invalid audit duration: expected a positive s, m or h duration"
[[ "${AUDIT_LIMIT}" =~ ^[0-9]+$ ]] && (( AUDIT_LIMIT >= 1 && AUDIT_LIMIT <= 1000 )) || config_error "invalid audit page limit"

curl_args=(--silent --show-error --max-time "${TIMEOUT}" --max-filesize 10485760)
if [[ -n "${CA_FILE}" ]]; then
    [[ -r "${CA_FILE}" ]] || config_error "configured DataStorage CA file is not readable"
    curl_args+=(--cacert "${CA_FILE}")
fi
if [[ -n "${DATASTORAGE_CONNECT_TO:-}" ]]; then
    # Keep URL authority/SNI/SAN verification while routing to the owned NodePort.
    curl_args+=(--connect-to "${DATASTORAGE_CONNECT_TO}" --noproxy '*')
fi
if [[ -r "${TOKEN_FILE}" ]]; then
    [[ "${DATASTORAGE_URL}" == https://* ]] || config_error "credential-bearing DataStorage requests require https"
    token=$(cat "${TOKEN_FILE}")
    [[ -n "${token}" && "${token}" != *$'\r'* && "${token}" != *$'\n'* ]] || config_error "invalid credential file"
    printf 'Authorization: Bearer %s\n' "${token}" > "${TEMP_DIR}/headers"
    unset token
    # @file means no bearer token appears in argv or diagnostic command output.
    curl_args+=(--header "@${TEMP_DIR}/headers")
elif [[ -n "${DATASTORAGE_TOKEN_FILE:-}" ]]; then
    config_error "configured DataStorage credential file is not readable"
fi

# Function to make API request with error handling
api_request() {
    local endpoint="$1"
    local output_file="$2"
    local description="$3"
    local error_file="${4:-${OUTPUT_DIR}/error.json}"

    echo "  Collecting ${description}..."

    local status exit_code=0
    status=$(curl "${curl_args[@]}" --output "${TEMP_DIR}/body" --write-out '%{http_code}' \
        "${DATASTORAGE_URL}${endpoint}" 2> "${TEMP_DIR}/stderr") || exit_code=$?
    if [[ "${exit_code}" == 0 && "${status}" == 2[0-9][0-9] ]]; then
        if ! jq --rawfile secret "${SAFE_TOKEN_FILE}" '
            ($secret | rtrimstr("\n")) as $token |
            walk(if type == "string" and $token != "" then split($token) | join("[REDACTED]") else . end)' \
            "${TEMP_DIR}/body" > "${output_file}"; then
            handle_api_error "${endpoint}" 1 "${status}" "${error_file}"
            return 1
        fi
        echo "    ✓ Collected ${description}"
        return 0
    else
        handle_api_error "${endpoint}" "${exit_code}" "${status:-000}" "${error_file}"
        return 1
    fi
}

# Collect workflows (limit 50, most recent)
if api_request "/api/v1/workflows?limit=${WORKFLOW_LIMIT}" \
               "${OUTPUT_DIR}/workflows.json" \
               "workflow catalog (limit ${WORKFLOW_LIMIT})" "${OUTPUT_DIR}/workflows.error.json"; then
    # Count workflows collected
    workflow_count=$(jq -r '.workflows | length' "${OUTPUT_DIR}/workflows.json" 2>/dev/null || echo "0")
    echo "    Workflows collected: ${workflow_count}"
fi

# Relative since is a supported DS parameter; never reinterpret '24h ago' or
# silently fall back to now. Ceil the frozen whole-second upper bound so a
# persisted fractional-second event is not excluded by serializer precision.
# jq's UTC formatting works with both BSD and GNU date's epoch output.
UNTIL=$(jq -nr --argjson now "$(date -u +%s)" '($now + 1) | strftime("%Y-%m-%dT%H:%M:%SZ")')
offset=0
complete=false
for page in {0..9}; do
    page_file="${TEMP_DIR}/page-${page}.json"
    if ! api_request "/api/v1/audit/events?limit=${AUDIT_LIMIT}&offset=${offset}&since=${AUDIT_TIMEFRAME}&until=${UNTIL}" \
        "${page_file}" "audit events page ${page}"; then
        exit 1
    fi
    if ! jq -e '.data | type == "array"' "${page_file}" > /dev/null; then
        config_error "invalid audit response: data must be an array"
    fi
    count=$(jq '.data | length' "${page_file}")
    total=$(jq '.pagination.total // 0' "${page_file}")
    offset=$((offset + count))
    if (( count < AUDIT_LIMIT || (total > 0 && offset >= total) )); then
        complete=true
        break
    fi
done
jq -s '{data:(map(.data) | add), pagination:{total:(last.pagination.total // 0)},
    collection:{pages:length}}' "${TEMP_DIR}"/page-*.json > "${OUTPUT_DIR}/audit-events.json"
if [[ "${complete}" != true ]]; then
    config_error "audit pagination exceeded the bounded ten-page collection budget; partial rows retained"
fi
echo "    Audit events collected: ${offset}"

echo "✓ DataStorage API collection complete"
