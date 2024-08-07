# Build the inference qpext binary
FROM golang:1.21 AS builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve/qpext
COPY qpext/go.mod  go.mod
COPY qpext/go.sum  go.sum

RUN go mod download

COPY qpext/cmd/qpext cmd/qpext
COPY qpext/logger.go logger.go

# Build
RUN CGO_ENABLED=0 go build -a -o qpext ./cmd/qpext

FROM gcr.io/distroless/static:nonroot
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/qpext /ko-app/
ENTRYPOINT ["/ko-app/qpext"]
