# Build the manager binary
# Upstream already is on go 1.24, however there is no gotoolset for 1.24 yet.
# TODO move to ubi9/go-toolset:1.24 when available
FROM registry.access.redhat.com/ubi9/go-toolset:1.23 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY cmd/    cmd/
COPY pkg/    pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o localmodelnode-agent ./cmd/localmodelnode

# Copy the controller-manager into a thin image
FROM gcr.io/distroless/static:nonroot
COPY third_party/ /third_party/
COPY --from=builder /go/src/github.com/kserve/kserve/localmodelnode-agent /manager
ENTRYPOINT ["/manager"]
