#!/usr/bin/env bash
# verify-helm-defaults-parity.sh (DD-PLATFORM-006 Decision Area 14, PR9)
#
# Proves that omitting a schema-defaulted field from charts/kubernaut/values.yaml
# is render-neutral: rendering the chart with the field absent (relying on
# kubernaut.mergedValues to inject the values.schema.json default at render
# time) must produce byte-identical output to rendering the same chart with
# every schema default explicitly materialized into values.yaml.
#
# Method: charts/kubernaut/templates/_generated_defaults.tpl already contains
# the full schema-defaults tree (every leaf with a declared "default", zero
# and non-zero alike) as the body of a `kubernaut.defaults` Helm named
# template -- the exact same tree kubernaut.mergedValues merges in at render
# time. This script extracts that YAML body verbatim (stripping only the
# `{{- define -}}` / `{{- end -}}` wrapper lines) and uses it as a `helm
# template -f` overlay, so the "explicit defaults" render exercises the
# actual generated artifact, not a hand-maintained copy that could drift.
#
# Usage: hack/verify-helm-defaults-parity.sh
# Exit code: 0 on parity, 1 on any rendered diff (printed to stderr).
set -euo pipefail

CHART_DIR="charts/kubernaut"
GENERATED_DEFAULTS="${CHART_DIR}/templates/_generated_defaults.tpl"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

# Required overrides for mandatory (no-default) fields the chart's fail()
# guards demand at render time -- same minimal set used by
# charts/kubernaut/tests/values_yaml_trim_test.yaml and this PR's manual
# golden-render checks.
REQUIRED_OVERRIDES=(
  --set global.llmProfiles.primary.provider=openai
  --set global.llmProfiles.primary.model=gpt-4
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials-primary
  --set aianalysis.policies.existingConfigMap=aianalysis-policies
  --set signalprocessing.policies.existingConfigMap=signalprocessing-policies
)

echo "==> Extracting full schema-defaults tree from ${GENERATED_DEFAULTS}..."
awk '/^\{\{- define "kubernaut.defaults" -\}\}$/{flag=1; next} /^\{\{- end -\}\}$/{flag=0} flag' \
  "${GENERATED_DEFAULTS}" > "${WORKDIR}/defaults-overlay-raw.yaml"

if [ ! -s "${WORKDIR}/defaults-overlay-raw.yaml" ]; then
  echo "ERROR: extracted defaults overlay is empty -- check ${GENERATED_DEFAULTS}'s define/end markers" >&2
  exit 1
fi

# workflowexecution.config.ansible is the one presence-gated opt-in block in
# the whole schema with a partial default set: tokenSecretRef.key/
# caCertSecretRef.key/organizationID have schema defaults, but the block's
# own required-when-present siblings (apiURL, tokenSecretRef.name,
# caCertSecretRef.name) do not, by design (BR-PLATFORM-005 -- AWX/AAP
# connectivity is entirely optional). Materializing the tree wholesale would
# make `ansible` a non-empty (truthy) map even when genuinely unconfigured --
# templates/workflowexecution/workflowexecution.yaml already knows this and
# gates on the raw `.Values.workflowexecution.config.ansible` presence check
# rather than kubernaut.mergedValues for exactly this reason. Strip it here
# (its whole indented subtree, by indentation-width, POSIX awk, no yq/python
# dependency) so the hydrated overlay matches what a real, valid values.yaml
# can express (either the whole block configured, or absent -- never
# partially present).
awk '
{
  line = $0
  n = 0
  while (substr(line, n + 1, 1) == " ") n++
  if (skip && n > skip_indent) next
  skip = 0
  if (line ~ /^ *ansible:$/) {
    skip = 1
    skip_indent = n
    next
  }
  print
}
' "${WORKDIR}/defaults-overlay-raw.yaml" > "${WORKDIR}/defaults-overlay.yaml"

echo "==> Rendering with values.yaml as-is (relies on kubernaut.mergedValues for omitted defaults)..."
helm template kubernaut "${CHART_DIR}" "${REQUIRED_OVERRIDES[@]}" \
  > "${WORKDIR}/render-trimmed.yaml" 2>"${WORKDIR}/render-trimmed.err" || {
    echo "ERROR: baseline render failed:" >&2
    cat "${WORKDIR}/render-trimmed.err" >&2
    exit 1
  }

# --skip-schema-validation: the generated defaults tree deliberately includes
# partial-default sub-objects for opt-in blocks (e.g.
# workflowexecution.config.ansible.{tokenSecretRef.key,caCertSecretRef.key}
# have schema defaults, but their required sibling `name` fields do not, by
# design -- the whole `ansible` block is presence-gated). Materializing that
# tree as a literal top-level values overlay makes those blocks look
# "present" to values.schema.json's requiredness rules, even though
# kubernaut.mergedValues' actual per-service, template-time merge never
# triggers that top-level schema check. Comparing rendered *templates* (this
# script's actual goal) does not need schema validation to pass twice.
echo "==> Rendering with every schema default explicitly materialized (-f defaults-overlay.yaml)..."
helm template kubernaut "${CHART_DIR}" -f "${WORKDIR}/defaults-overlay.yaml" "${REQUIRED_OVERRIDES[@]}" \
  --skip-schema-validation \
  > "${WORKDIR}/render-hydrated.yaml" 2>"${WORKDIR}/render-hydrated.err" || {
    echo "ERROR: hydrated-defaults render failed:" >&2
    cat "${WORKDIR}/render-hydrated.err" >&2
    exit 1
  }

if diff -u "${WORKDIR}/render-trimmed.yaml" "${WORKDIR}/render-hydrated.yaml" > "${WORKDIR}/diff.txt"; then
  echo "PASS: trimmed-values render is byte-identical to explicit-full-defaults render."
  exit 0
else
  echo "FAIL: rendering with values.yaml defaults omitted differs from rendering with every" >&2
  echo "schema default explicit -- a template is reading .Values.<path> directly instead of" >&2
  echo "going through kubernaut.mergedValues, or a default drifted out of sync. Diff:" >&2
  cat "${WORKDIR}/diff.txt" >&2
  exit 1
fi
