# Build the manager binary
FROM registry.access.redhat.com/ubi8/go-toolset:1.18.10-1 as builder

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Add the vendored dependencies
COPY vendor vendor

# Copy the go source
COPY api api
COPY api-hub api-hub
COPY cmd cmd
COPY controllers controllers
COPY docs.mk docs.mk
COPY internal internal

# Copy Makefile
COPY Makefile Makefile

# Copy the .git directory which is needed to store the build info
COPY .git .git

ARG TARGET

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make ${TARGET}

FROM registry.access.redhat.com/ubi8/ubi-micro:8.7

ARG TARGET

COPY --from=builder /opt/app-root/src/${TARGET} /usr/local/bin/manager
USER 65534:65534

ENTRYPOINT ["/usr/local/bin/manager"]
