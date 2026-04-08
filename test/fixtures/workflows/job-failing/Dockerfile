# Job Failing - Execution Image (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017
#
# Execution-only image that intentionally exits with non-zero status to test
# failure handling in the WorkflowExecution controller's Job backend.
# The schema is delivered separately via a schema-only OCI image (FROM scratch).
# See: test/fixtures/workflows/Dockerfile (shared schema image builder)
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/job-failing:v1.0.0-exec .

FROM registry.access.redhat.com/ubi10/ubi-minimal:latest

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
