# Build the inference-agent binary
FROM golang:1.25 AS deps

WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ---- Build stage (parallel with license on BuildKit) ----
FROM deps AS builder

ARG CMD=agent
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/    pkg/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly go build -a -o agent ./cmd/${CMD}

# ---- License stage (parallel with build on BuildKit) ----
FROM deps AS license

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/google/go-licenses@v1.6.0

ARG CMD=agent
COPY cmd/${CMD}/ cmd/${CMD}/
COPY pkg/    pkg/
COPY LICENSE LICENSE
RUN --mount=type=cache,target=/go/pkg/mod \
    go-licenses save --save_path /third_party/library ./cmd/${CMD}

# Copy the inference-agent into a thin image
FROM gcr.io/distroless/static:nonroot
COPY --from=license /third_party /third_party
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/agent /ko-app/
ENTRYPOINT ["/ko-app/agent"]
