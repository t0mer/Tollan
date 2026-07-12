# syntax=docker/dockerfile:1

# Stage 1: web UI — built once on the native build platform (static output).
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Go binary — runs on the native build platform, cross-compiles to target.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
WORKDIR /app
ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0
RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace the placeholder embed dir with the freshly built UI.
COPY --from=frontend /app/web/dist ./web/dist
ARG VERSION=docker
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build \
    -trimpath -ldflags="-s -w -X github.com/t0mer/tollan/internal/version.Version=${VERSION}" \
    -o /out/tollan ./cmd/tollan
# Pre-create the data dir owned by the runtime uid so anonymous/named volumes
# mounted over /data inherit writable ownership (scratch has no shell to chown).
RUN mkdir -p /data

# Stage 3: runtime — scratch, self-contained.
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder --chown=65534:65534 /data /data
COPY --from=builder /out/tollan /tollan

ENV TOLLAN_DATA_DIR=/data
VOLUME ["/data"]
USER 65534:65534

# UI/API, syslog (tcp+udp, tls), GELF (tcp+udp), Beats, NetFlow, IPFIX.
EXPOSE 8080
EXPOSE 1514/udp 1514/tcp 6514/tcp
EXPOSE 12201/udp 12201/tcp
EXPOSE 5044/tcp
EXPOSE 2055/udp
EXPOSE 4739/udp

ENTRYPOINT ["/tollan"]
CMD ["run"]
