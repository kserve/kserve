# Build the manager binary
FROM golang:1.17 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY tools/  tools/
COPY pkg/    pkg/
COPY go.mod  go.mod
COPY go.sum  go.sum

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o tf2openapi ./tools/tf2openapi/cmd

# Copy tf2openapi into a thin image
FROM gcr.io/distroless/static:latest
WORKDIR /
COPY third_party/ third_party/
COPY --from=builder /go/src/github.com/kserve/kserve/tf2openapi .
ENTRYPOINT ["/tf2openapi"]
