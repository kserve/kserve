# Build the inference-router binary
FROM golang:1.24 AS builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY cmd/    cmd/
COPY pkg/    pkg/

# Build
RUN CGO_ENABLED=0  go build -a -o router ./cmd/router

# Generate third-party licenses
COPY LICENSE LICENSE
RUN go install github.com/google/go-licenses@latest
# Forbidden Licenses: https://github.com/google/licenseclassifier/blob/e6a9bb99b5a6f71d5a34336b8245e305f5430f99/license_type.go#L341
RUN go-licenses check ./cmd/... ./pkg/... --disallowed_types="forbidden,unknown"
RUN go-licenses save --save_path third_party/library ./cmd/router

# Copy the inference-router into a thin image
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /go/src/github.com/kserve/kserve/third_party /third_party
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/router /ko-app/
ENTRYPOINT ["/ko-app/router"]
