# Build the inference-router binary
FROM golang:1.21 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY cmd/    cmd/
COPY pkg/    pkg/

# Build
RUN CGO_ENABLED=0  go build -a -o router ./cmd/router

# Copy the inference-router into a thin image
FROM gcr.io/distroless/static:nonroot
COPY third_party/ third_party/
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/router /ko-app/
ENTRYPOINT ["/ko-app/router"]
