FROM golang:1.26@sha256:079e59808d2d252516e27e3f3a9c003740dee7f75e55aa71528766d52bcfc16a AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /oas-runtime .

FROM ubuntu:26.04@sha256:b7f48194d4d8b763a478a621cdc81c27be222ba2206ca3ca6bc42b49685f3d9e
RUN groupadd -f -g 1000 sandbox && \
    (id -u 1000 >/dev/null 2>&1 && usermod -l sandbox -g sandbox -d /home/sandbox -m $(getent passwd 1000 | cut -d: -f1) || useradd -m -u 1000 -g sandbox sandbox) && \
    apt-get update && apt-get install -y iproute2 ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /oas-runtime /usr/local/bin/oas-runtime
USER sandbox
EXPOSE 8080
