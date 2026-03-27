# db-migrate: UBI10-minimal image bundling goose + psql for database migrations.
#
# Eliminates runtime binary downloads (curl/wget from GitHub) so the migration
# Job works in disconnected/air-gapped environments (#351, C1).
#
# Build:
#   podman build --platform linux/amd64 \
#     --build-arg APP_VERSION=v3.24.1 \
#     -t quay.io/kubernaut-ai/db-migrate:v3.24.1-amd64 \
#     -f docker/db-migrate.Dockerfile .
#
# Multi-arch: TARGETARCH is set automatically by --platform.

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS production

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
ARG TARGETARCH

ENV GOOSE_VERSION=v3.24.1
ENV GOOSE_SHA256_x86_64=313ba1c77a367f431faf690fe817aad297722623b62395a8ac64281f54f6c415
ENV GOOSE_SHA256_aarch64=207681c8ed67511fa53da6790de986e9a70981af0eec6c4ba1d8ff05e552f043

RUN microdnf update -y && \
	microdnf install -y postgresql ca-certificates shadow-utils && \
	microdnf clean all

RUN set -e; \
	case "${TARGETARCH}" in \
		amd64)  GOOSE_ARCH="x86_64";  EXPECTED="${GOOSE_SHA256_x86_64}" ;; \
		arm64)  GOOSE_ARCH="arm64";   EXPECTED="${GOOSE_SHA256_aarch64}" ;; \
		*)      echo "ERROR: Unsupported arch: ${TARGETARCH}"; exit 1 ;; \
	esac; \
	curl -fsSL -o /usr/local/bin/goose \
		"https://github.com/pressly/goose/releases/download/${GOOSE_VERSION}/goose_linux_${GOOSE_ARCH}"; \
	ACTUAL=$(sha256sum /usr/local/bin/goose | awk '{print $1}'); \
	if [ "${ACTUAL}" != "${EXPECTED}" ]; then \
		echo "ERROR: SHA256 mismatch for goose_linux_${GOOSE_ARCH}"; \
		echo "  expected: ${EXPECTED}"; \
		echo "  actual:   ${ACTUAL}"; \
		exit 1; \
	fi; \
	chmod +x /usr/local/bin/goose

RUN useradd -r -u 1001 -g root db-migrate-user
USER 1001

LABEL name="kubernaut-db-migrate" \
	vendor="Kubernaut" \
	summary="Kubernaut Database Migration - goose + psql on UBI10-minimal" \
	description="Bundles goose SQL migration tool and PostgreSQL client for Helm post-install/post-upgrade database migrations in connected and disconnected environments." \
	maintainer="jgil@redhat.com" \
	component="db-migrate" \
	part-of="kubernaut" \
	io.k8s.description="Database migration image for Kubernaut Helm chart hooks" \
	io.k8s.display-name="Kubernaut DB Migrate" \
	io.openshift.tags="kubernaut,db-migrate,goose,postgresql,migration,helm-hook"

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-db-migrate" \
	org.opencontainers.image.description="Database migration image bundling goose and psql for Kubernaut Helm chart hooks in connected and disconnected environments." \
	org.opencontainers.image.vendor="Kubernaut"

ENTRYPOINT ["/usr/local/bin/goose"]
CMD ["--help"]
