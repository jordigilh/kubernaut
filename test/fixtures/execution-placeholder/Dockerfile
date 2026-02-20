# Minimal placeholder execution image for test workflow schemas.
# All Tekton workflow-schema.yaml files reference this image as their
# execution.bundle so that the DataStorage bundle-existence check
# (crane.Head) passes during registration.
#
# Build:
#   podman build --platform linux/amd64,linux/arm64 \
#     --manifest quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0 \
#     -f Dockerfile .
FROM busybox:1.36
CMD ["echo", "kubernaut placeholder execution bundle"]
