FROM golang:1.12 as builder

# Retrieve the dependencies.
RUN go get google.golang.org/grpc

# Copy local code to the container image.
WORKDIR /go/src/helloworld
COPY . ./

# Build the command inside the container.
# To facilitate gRPC testing, this container includes a client command.
RUN CGO_ENABLED=0 go build -tags=grpchelloworld -o ./hello-server
RUN CGO_ENABLED=0 go build -tags=grpchelloworld -o ./hello-client ./client

# Use a Docker multi-stage build to create a lean production image.
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
# https://github.com/GoogleContainerTools/distroless#readme
FROM gcr.io/distroless/static

# Copy the binaries to the production image from the builder stage.
COPY --from=builder /go/src/helloworld/hello-server /server
COPY --from=builder /go/src/helloworld/hello-client /client

# Run the service on container startup.
CMD ["/server"]
