# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.19 as builder

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
RUN git config --global --add safe.directory ${PWD}
RUN make ${TARGET}

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.2

ARG TARGET

COPY --from=builder /opt/app-root/src/${TARGET} /usr/local/bin/manager
USER 65534:65534

ENTRYPOINT ["/usr/local/bin/manager"]
