#!/usr/bin/env bash
# Post-processes crd-ref-docs markdown output to strip internal references
# that should not appear in user-facing documentation.
set -euo pipefail

FILE="${1:?Usage: clean-output.sh <file>}"

sed -E -i.bak \
  -e 's/={5,}<br \/>//g' \
  -e 's/<br \/>={5,}//g' \
  -e 's/={5,}//g' \
  -e 's/BR-[A-Z_]+-[0-9]+(\.[0-9]+)*:?//g' \
  -e 's/ADR-[0-9]+(\.[0-9]+)*:?//g' \
  -e 's/DD-[A-Z_]+-[0-9]+(\s+v[0-9.]+)?:?//g' \
  -e 's/Gap #[0-9]+:?//g' \
  -e 's/HAPI-[0-9]+(\.[0-9]+)*:?//g' \
  -e 's/\([^)]*_CLARIFICATION\.md\)//g' \
  -e 's/\([^)]*_STATUS\.md\)//g' \
  -e 's/\([^)]*_PLAN\.md\)//g' \
  -e 's/PHASE [0-9]+ ADDITION//g' \
  -e 's/\(Dec [0-9]+\)//g' \
  -e 's/<br \/>[A-Z][A-Z /]{2,}[A-Z]( *\([^)]*\))?<br \/>/<br \/>/g' \
  -e 's/\|[[:space:]]*[A-Z][A-Z /]{2,}[A-Z]( *\([^)]*\))?<br \/>/| /g' \
  -e 's/  +/ /g' \
  -e 's/\( +\)/()/g' \
  -e 's/\(\)//g' \
  -e 's/ +\|/|/g' \
  -e '/^[A-Z][A-Z ]{5,}$/d' \
  "$FILE"

# Convert broken internal anchors for shared types (pkg/shared/types/) to plain text.
# These types are resolved by crd-ref-docs but not rendered because they're outside api/.
sed -E -i.bak \
  -e 's/\[KubernetesContext\]\(#kubernetescontext\)/_KubernetesContext_/g' \
  -e 's/\[BusinessClassification\]\(#businessclassification\)/_BusinessClassification_/g' \
  -e 's/\[OwnerChainEntry\]\(#ownerchainentry\)/_OwnerChainEntry_/g' \
  -e 's/\[NamespaceContext\]\(#namespacecontext\)/_NamespaceContext_/g' \
  -e 's/\[WorkloadDetails\]\(#workloaddetails\)/_WorkloadDetails_/g' \
  "$FILE"
rm -f "${FILE}.bak"

rm -f "${FILE}.bak"
