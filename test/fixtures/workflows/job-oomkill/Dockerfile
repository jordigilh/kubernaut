# Job OOM-Kill Simulation - Execution Image (BR-WE-019, DD-WE-008)
# Authority: ADR-043, DD-WORKFLOW-017
#
# Execution-only image that deterministically exits with status 137 (the
# same exit code the kubelet reports for a real OOM-kill) to prove the Job
# controller's PodFailurePolicy Ignore rule for exit-137 pod failures on a
# real cluster (Phase 6, DD-WE-008 Section 8: unconditional exit 137 is
# sufficient -- no stateful retry-then-succeed logic needed, since the test
# only needs to observe job.Status.Failed accumulate while the Job as a
# whole never reaches a terminal Failed condition).
# The schema is delivered separately via a schema-only OCI image (FROM scratch).
# See: test/fixtures/workflows/Dockerfile (shared schema image builder)
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/job-oomkill:v1.0.0-exec .

FROM registry.access.redhat.com/ubi10/ubi-minimal:latest@sha256:b217fa65d8c21058887b18f005f587e47a17dd1281a5196ac88d01724a273dbd

# Job executor injects parameters as environment variables:
#   TARGET_RESOURCE  - target resource being remediated
#   FAILURE_MESSAGE  - error message to include
#
# Default values for local testing
ENV TARGET_RESOURCE="test/deployment/oomkill" \
    FAILURE_MESSAGE="Simulated OOM-kill for PodFailurePolicy E2E testing"

# BR-WE-018: spawned Job pods enforce runAsNonRoot; ubi-minimal has no USER
# directive by default and runs as root, which kubelet rejects outright
# (CreateContainerConfigError) once the pod-level SecurityContext applies.
USER 1001

ENTRYPOINT ["/bin/sh", "-c", "\
echo '============================================' && \
echo 'Kubernaut OOM-Kill Simulation Test Job' && \
echo '============================================' && \
echo '' && \
echo \"Target Resource: ${TARGET_RESOURCE}\" && \
echo \"Failure Message: ${FAILURE_MESSAGE}\" && \
echo '' && \
echo 'Exiting with status 137 (simulated OOM-kill)...' && \
exit 137"]
