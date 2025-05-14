# Build the inference qpext binary
FROM golang:1.24 AS builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve/qpext
COPY qpext/go.mod  go.mod
COPY qpext/go.sum  go.sum

RUN go mod download

COPY qpext/cmd/qpext cmd/qpext
COPY qpext/logger.go logger.go

# Build
RUN CGO_ENABLED=0 go build -a -o qpext ./cmd/qpext

# Generate third-party licenses
COPY LICENSE LICENSE
RUN go install github.com/google/go-licenses@latest
# Forbidden Licenses: https://github.com/google/licenseclassifier/blob/e6a9bb99b5a6f71d5a34336b8245e305f5430f99/license_type.go#L341
RUN go-licenses check ./...  --disallowed_types="forbidden,unknown"
RUN go-licenses save --save_path third_party/library ./cmd/qpext

FROM gcr.io/distroless/static:nonroot
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/qpext/third_party /third_party
COPY --from=builder /go/src/github.com/kserve/kserve/qpext /ko-app/
ENTRYPOINT ["/ko-app/qpext"]
