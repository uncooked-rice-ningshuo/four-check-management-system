FROM registry-1.docker.io/library/golang:1.25-alpine3.22 AS build
RUN CGO_ENABLED=0 GOBIN=/out go install github.com/minio/mc@RELEASE.2025-08-13T08-35-41Z
FROM registry-1.docker.io/library/alpine:3.22.1
RUN apk add --no-cache ca-certificates
COPY --from=build /out/mc /usr/local/bin/mc
ENTRYPOINT ["/usr/local/bin/mc"]
