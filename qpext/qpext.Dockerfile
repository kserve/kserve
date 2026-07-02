# Build the inference qpext binary
FROM golang:1.25 AS deps

WORKDIR /go/src/github.com/kserve/kserve/qpext
COPY qpext/go.mod  go.mod
COPY qpext/go.sum  go.sum
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ---- Build stage (parallel with license on BuildKit) ----
FROM deps AS builder

COPY qpext/cmd/qpext cmd/qpext
COPY qpext/logger.go logger.go
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o qpext ./cmd/qpext

# ---- License stage (parallel with build on BuildKit) ----
FROM deps AS license

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/google/go-licenses@v1.6.0

COPY qpext/cmd/qpext cmd/qpext
COPY qpext/logger.go logger.go
COPY LICENSE LICENSE
RUN --mount=type=cache,target=/go/pkg/mod \
    go-licenses save --save_path /third_party/library ./cmd/qpext

# ---- Runtime ----
FROM gcr.io/distroless/static:nonroot
WORKDIR /ko-app
COPY --from=license /third_party /third_party
COPY --from=builder /go/src/github.com/kserve/kserve/qpext/qpext /ko-app/
ENTRYPOINT ["/ko-app/qpext"]
