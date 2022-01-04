# Build the inference-agent binary
FROM golang:1.17 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN go mod download

COPY pkg/    pkg/
COPY cmd/    cmd/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o agent ./cmd/agent

# Copy the inference-agent into a thin image
FROM gcr.io/distroless/static:latest
COPY third_party/ third_party/
WORKDIR /ko-app
COPY --from=builder /go/src/github.com/kserve/kserve/agent /ko-app/
ENTRYPOINT ["/ko-app/agent"]
