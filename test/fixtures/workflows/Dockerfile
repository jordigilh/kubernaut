# Minimal OCI image containing only /workflow-schema.yaml
# Used by DataStorage OCI extractor (DD-WORKFLOW-017, ADR-043)
#
# Build: podman build -t <registry>/<name>:<tag> -f Dockerfile <workflow-dir>/
# The build context must contain workflow-schema.yaml
FROM scratch
COPY workflow-schema.yaml /workflow-schema.yaml
