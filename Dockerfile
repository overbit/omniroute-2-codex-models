# Build the same static binary for AMD64 or ARM64 without emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod main.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w -buildid=' -o /out/proxy .

# No shell, package manager, Node.js, libc, or writable application files.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/proxy /proxy
USER 65532:65532
ENV HOST=0.0.0.0 \
    PORT=8787 \
    OMNIROUTE_MODELS_URL=http://host.docker.internal:20128/v1/models?prefix=alias
EXPOSE 8787
ENTRYPOINT ["/proxy"]
