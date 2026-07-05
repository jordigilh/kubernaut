# Job Failing - Execution Image (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017
#
# Execution-only image that intentionally exits with non-zero status to test
# failure handling in the WorkflowExecution controller's Job backend.
# The schema is delivered separately via a schema-only OCI image (FROM scratch).
# See: test/fixtures/workflows/Dockerfile (shared schema image builder)
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/job-failing:v1.0.0-exec .

FROM registry.access.redhat.com/ubi10/ubi-minimal:latest@sha256:b217fa65d8c21058887b18f005f587e47a17dd1281a5196ac88d01724a273dbd

# Job executor injects parameters as environment variables:
#   TARGET_RESOURCE  - target resource being remediated
#   FAILURE_MESSAGE  - error message to include
#
# Default values for local testing
ENV TARGET_RESOURCE="test/deployment/failing" \
    FAILURE_MESSAGE="Intentional test failure"

# BR-WE-018: spawned Job pods enforce runAsNonRoot; ubi-minimal has no USER
# directive by default and runs as root, which kubelet rejects outright
# (CreateContainerConfigError) once the pod-level SecurityContext applies.
USER 1001

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
