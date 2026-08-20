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

# Kubernaut Must-Gather - Namespace Resolution Helper
# Issue #2037: single source of truth for the RELEASE_NAMESPACE and
# OPERATOR_NAMESPACE defaults.
#
# gather.sh always exports RELEASE_NAMESPACE/OPERATOR_NAMESPACE (from their
# --namespace=/--operator-namespace= flags or its own defaults) before
# invoking collectors, so these defaults only matter when a collector is run
# standalone (direct debugging, or a unit test that doesn't go through
# gather.sh). Without this shared helper, the same
# ": ${RELEASE_NAMESPACE:=kubernaut-system}" fallback line was duplicated
# across logs.sh and datastorage.sh -- exactly the kind of copy-pasted
# constant that caused the original drift this issue fixes.
#
# This file is meant to be sourced, not executed.

resolve_release_namespace() {
    : "${RELEASE_NAMESPACE:=kubernaut-system}"
}

resolve_operator_namespace() {
    : "${OPERATOR_NAMESPACE:=kubernaut-operator-system}"
}

resolve_workflow_namespace() {
    : "${WORKFLOW_NAMESPACE:=kubernaut-workflows}"
}

# Issue #2196: KUBERNAUT_NAMESPACES is a bash array, but gather.sh invokes
# events.sh/metrics.sh/cluster-state.sh as separate `bash <script>.sh`
# subprocesses -- bash cannot export array variables across that boundary,
# only scalar strings (confirmed by direct repro; see #2196). Their
# `for namespace in "${KUBERNAUT_NAMESPACES[@]}"` loops silently iterated
# zero times in every real install as a result.
#
# Fix mirrors EXTRA_NAMESPACES_CSV (#2036/#2194): gather.sh exports a scalar,
# comma-joined KUBERNAUT_NAMESPACES_CSV instead of the array; this helper
# reconstructs the KUBERNAUT_NAMESPACES array locally in each collector's own
# process from that CSV. Falls back to RELEASE_NAMESPACE/WORKFLOW_NAMESPACE
# when the CSV is unset, matching gather.sh's own default composition --
# this only matters when a collector is run standalone (direct debugging, or
# a test that doesn't go through gather.sh).
resolve_kubernaut_namespaces() {
    # shellcheck disable=SC2034 # consumed by the caller after sourcing this file
    if [ -n "${KUBERNAUT_NAMESPACES_CSV:-}" ]; then
        IFS=',' read -ra KUBERNAUT_NAMESPACES <<< "${KUBERNAUT_NAMESPACES_CSV}"
    else
        resolve_release_namespace
        resolve_workflow_namespace
        KUBERNAUT_NAMESPACES=("${RELEASE_NAMESPACE}" "${WORKFLOW_NAMESPACE}")
    fi
}

resolve_release_namespace
resolve_operator_namespace
resolve_workflow_namespace
