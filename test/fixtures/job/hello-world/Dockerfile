# Test Hello World Job Container for E2E Testing
# Authority: BR-WE-014 (Kubernetes Job Execution Backend)
# ADR-043: Contains /workflow-schema.yaml for catalog compliance
#
# This container image is used directly by the Kubernetes Job executor.
# Unlike Tekton bundles (which contain pipeline definitions), this image
# IS the execution unit -- its entrypoint is the remediation action.
#
# Base image: UBI9 minimal (no Docker Hub rate limits, consistent with Tekton steps)
#
# Build:
#   podman build -t quay.io/kubernaut-cicd/test-workflows/job-hello-world:v1.0.0 .
#
# Push:
#   podman push quay.io/kubernaut-cicd/test-workflows/job-hello-world:v1.0.0

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# ADR-043: Include workflow schema at root for catalog discovery
COPY workflow-schema.yaml /workflow-schema.yaml

# Job executor injects parameters as environment variables:
#   TARGET_RESOURCE - target resource being remediated
#   MESSAGE         - optional message to echo
#
# Default values for local testing
ENV TARGET_RESOURCE="test/deployment/hello" \
    MESSAGE="Hello from Kubernaut Job WorkflowExecution!"

ENTRYPOINT ["/bin/sh", "-c", "\
echo '============================================' && \
echo 'Kubernaut WorkflowExecution Job Test' && \
echo '============================================' && \
echo '' && \
echo \"Target Resource: ${TARGET_RESOURCE}\" && \
echo \"Message: ${MESSAGE}\" && \
echo '' && \
echo 'Workflow completed successfully!' && \
echo '============================================'"]
