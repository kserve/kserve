# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.22.9 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY cmd/    cmd/
COPY pkg/    pkg/

# Build
USER root
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=mod go build -a -o manager ./cmd/manager

# Use distroless as minimal base image to package the manager binary
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
RUN microdnf install -y --disablerepo=* --enablerepo=ubi-9-baseos-rpms shadow-utils && \
    microdnf clean all && \
    useradd kserve -m -u 1000
RUN microdnf remove -y shadow-utils
COPY third_party/ /third_party/
COPY --from=builder /go/src/github.com/kserve/kserve/manager /
USER 1000:1000

ENTRYPOINT ["/manager"]
