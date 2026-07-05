# Minimal placeholder execution image for test workflow schemas.
# All Tekton workflow-schema.yaml files reference this image as their
# execution.bundle so that the DataStorage bundle-existence check
# (crane.Head) passes during registration.
#
# Build:
#   podman build --platform linux/amd64,linux/arm64 \
#     --manifest quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0 \
#     -f Dockerfile .
FROM busybox:1.36@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
# BR-WE-018: spawned Job pods enforce runAsNonRoot; busybox's default image
# has no USER directive and runs as root, which kubelet rejects outright
# (CreateContainerConfigError) once that pod-level SecurityContext applies.
USER 1000
CMD ["echo", "kubernaut placeholder execution bundle"]
