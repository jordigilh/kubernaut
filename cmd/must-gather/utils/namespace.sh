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
# Issue #2037: single source of truth for the RELEASE_NAMESPACE default.
#
# gather.sh always exports RELEASE_NAMESPACE (from --namespace= or its own
# default) before invoking collectors, so this default only matters when a
# collector is run standalone (direct debugging, or a unit test that doesn't
# go through gather.sh). Without this shared helper, that same
# ": ${RELEASE_NAMESPACE:=kubernaut-system}" fallback line was duplicated
# across logs.sh and datastorage.sh -- exactly the kind of copy-pasted
# constant that caused the original drift this issue fixes.
#
# This file is meant to be sourced, not executed.

resolve_release_namespace() {
    : "${RELEASE_NAMESPACE:=kubernaut-system}"
}

resolve_release_namespace
