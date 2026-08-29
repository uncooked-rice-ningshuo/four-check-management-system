FROM golang:1.24.8-alpine3.22 AS build

ARG MINIO_VERSION=RELEASE.2025-10-15T17-29-55Z
RUN CGO_ENABLED=0 GOBIN=/out go install "github.com/minio/minio@${MINIO_VERSION}"

FROM alpine:3.22.1
RUN apk add --no-cache ca-certificates wget
COPY --from=build /out/minio /usr/local/bin/minio

ENTRYPOINT ["/usr/local/bin/minio"]
