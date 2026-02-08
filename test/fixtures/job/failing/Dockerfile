# Test Intentionally Failing Job Container for E2E Testing
# Authority: BR-WE-014 (Kubernetes Job Execution Backend)
# ADR-043: Contains /workflow-schema.yaml for catalog compliance
#
# This container image exits with non-zero status to test failure handling
# in the WorkflowExecution controller's Kubernetes Job backend.
#
# Base image: UBI9 minimal (no Docker Hub rate limits, consistent with Tekton steps)
#
# Build:
#   podman build -t quay.io/kubernaut-cicd/test-workflows/job-failing:v1.0.0 .
#
# Push:
#   podman push quay.io/kubernaut-cicd/test-workflows/job-failing:v1.0.0

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# ADR-043: Include workflow schema at root for catalog discovery
COPY workflow-schema.yaml /workflow-schema.yaml

# Job executor injects parameters as environment variables:
#   TARGET_RESOURCE  - target resource being remediated
#   FAILURE_MESSAGE  - error message to include
#
# Default values for local testing
ENV TARGET_RESOURCE="test/deployment/failing" \
    FAILURE_MESSAGE="Intentional test failure"

ENTRYPOINT ["/bin/sh", "-c", "\
echo '============================================' && \
echo 'Kubernaut Failure Test Job' && \
echo '============================================' && \
echo '' && \
echo \"Target Resource: ${TARGET_RESOURCE}\" && \
echo \"Failure Message: ${FAILURE_MESSAGE}\" && \
echo '' && \
echo 'Exiting with non-zero status...' && \
exit 1"]
