# Build the KernelCacheNode agent.
FROM golang:1.26.8 AS deps

WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod go.mod
COPY go.sum go.sum
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

FROM deps AS builder

ARG CMD=kernelcachenode
ARG GOTAGS=""
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/ pkg/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly go build -tags "${GOTAGS}" -a -o kernelcachenode-agent ./cmd/${CMD}

FROM deps AS license

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/google/go-licenses@v1.6.0

ARG CMD=kernelcachenode

COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/ pkg/
COPY LICENSE LICENSE

RUN --mount=type=cache,target=/go/pkg/mod \
    go-licenses save \
    --save_path /third_party/library \
    ./cmd/${CMD}

FROM gcr.io/distroless/static:nonroot
COPY --from=license /third_party /third_party
COPY --from=builder /go/src/github.com/kserve/kserve/kernelcachenode-agent /manager
ENTRYPOINT ["/manager"]
