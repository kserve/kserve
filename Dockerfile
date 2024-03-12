# Build the manager binary
FROM golang:1.21 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY cmd/    cmd/
COPY pkg/    pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o manager ./cmd/manager

# Copy the controller-manager into a thin image
FROM gcr.io/distroless/static:nonroot
COPY third_party/ /third_party/
COPY --from=builder /go/src/github.com/kserve/kserve/manager /
ENTRYPOINT ["/manager"]
