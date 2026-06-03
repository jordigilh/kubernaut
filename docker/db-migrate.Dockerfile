# db-migrate: UBI10-minimal image bundling goose + psql for database migrations.
#
# Goose is built with `go install` at the version pinned in go.mod
# (github.com/pressly/goose/v3) so the CLI matches application dependencies
# without maintaining per-release binary SHA256 checksums.
#
# Build:
#   podman build --platform linux/amd64 \
#     --build-arg APP_VERSION=v3.27.1 \
#     -t quay.io/kubernaut-ai/db-migrate:v3.27.1-amd64 \
#     -f docker/db-migrate.Dockerfile .
#
# Multi-arch: TARGETARCH is set from --platform for the goose compile step.

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

FROM --platform=$BUILDPLATFORM golang:1.25.11-bookworm AS goose-builder
ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH}
RUN GOMODCACHE=$(mktemp -d) && \
    cd "$GOMODCACHE" && \
    go mod init tmp && \
    go get github.com/pressly/goose/v3/cmd/goose@v3.27.1 && \
    go build -o /go/bin/goose github.com/pressly/goose/v3/cmd/goose

FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS production

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

COPY --from=goose-builder /go/bin/goose /usr/local/bin/goose

RUN microdnf update -y && \
	microdnf install -y postgresql ca-certificates shadow-utils && \
	microdnf clean all

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
