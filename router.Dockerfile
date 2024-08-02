# Build the inference-router binary
FROM registry.access.redhat.com/ubi8/go-toolset:1.21 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY pkg/    pkg/
COPY cmd/    cmd/

# Build
USER root
RUN CGO_ENABLED=0  go build -a -o router ./cmd/router

# Copy the inference-router into a thin image
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
RUN microdnf install -y --disablerepo=* --enablerepo=ubi-8-baseos-rpms shadow-utils && \
    microdnf clean all && \ 
    useradd kserve -m -u 1000
RUN microdnf remove -y shadow-utils
COPY third_party/ third_party/
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/router /ko-app/
USER 1000:1000

ENTRYPOINT ["/ko-app/router"]