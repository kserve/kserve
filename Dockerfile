# Build the manager binary
FROM golang:1.17 as builder

# Copy in the go src
WORKDIR /go/src/github.com/kserve/kserve
COPY go.mod  go.mod
COPY go.sum  go.sum

RUN  go mod download

COPY cmd/    cmd/
COPY pkg/    pkg/
# Build
RUN if [ "$(uname -m)" = "ppc64le" ]; then \
        CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -a -o manager ./cmd/manager; \
    elif [ "$(uname -m)" = "aarch64" ]; then \
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -o manager ./cmd/manager; \
    else \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager ./cmd/manager; \
    fi

# Copy the controller-manager into a thin image
FROM gcr.io/distroless/static:latest
WORKDIR /
COPY third_party/ third_party/
COPY --from=builder /go/src/github.com/kserve/kserve/manager .
ENTRYPOINT ["/manager"]
