# Build the inference qpext binary
FROM golang:1.18 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve/qpext
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY cmd/qpext cmd/qpext
COPY logger.go logger.go

# Build
RUN CGO_ENABLED=0 go build -a -o qpext ./cmd/qpext

FROM gcr.io/distroless/static:nonroot
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/qpext /ko-app/
ENTRYPOINT ["/ko-app/qpext"]
