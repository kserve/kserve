# Build the inference-router binary
FROM golang:1.25 AS builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

# Install license tool (cached independently of source changes)
RUN go install github.com/google/go-licenses@v1.6.0
COPY LICENSE LICENSE

ARG CMD=router
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/    pkg/

# Check and generate third-party licenses (fast, fail-fast on violations)
RUN go-licenses check ./cmd/${CMD} ./pkg/... --disallowed_types="forbidden,unknown" && \
    go-licenses save --save_path third_party/library ./cmd/${CMD}

# Build
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly go build -a -o router ./cmd/${CMD}

# Copy the inference-router into a thin image
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /go/src/github.com/kserve/kserve/third_party /third_party
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/router /ko-app/
ENTRYPOINT ["/ko-app/router"]
