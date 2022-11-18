# Build the manager binary
FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.18-openshift-4.11 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Add the vendored dependencies
COPY vendor vendor

# Copy the go source
COPY main.go main.go
COPY api api
COPY controllers controllers
COPY internal internal

# Copy Makefile
COPY Makefile Makefile

# Copy the .git directory which is needed to store the build info
COPY .git .git

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make manager

FROM registry.redhat.io/ubi8/ubi-micro:8.7

COPY --from=builder /workspace/manager /usr/local/bin
USER 65534:65534

ENTRYPOINT ["/usr/local/bin/manager"]
