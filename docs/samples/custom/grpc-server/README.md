# A sample for gRPC Server in custom model

This sample can be used to try out gRPC, HTTP/2, and custom port configuration
in a custom inference service.

## Build and Deploy the sample code

1. Use Docker to build a container image for this service and push to Docker Hub.

  Replace `{username}` with your Docker Hub username then run the commands:

  ```shell
  # Build the container on your local machine.
  docker build --tag "{username}/grpc-ping-go" .

  # Push the container to docker registry.
  docker push "{username}/grpc-ping-go"
  ```

3. Update the `grpc-service.yaml` file in the project to reference the published image from step 1.

   Replace `{username}` in `grpc-service.yaml` with your Docker Hub user name:
   
   
  ```yaml
 apiVersion: serving.kubeflow.org/v1alpha2
kind: InferenceService
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: grpc-service
spec:
  default:
    predictor:
      custom:
        container:
          image: docker.io/{username}/grpc-ping-go:latest
          ports:
          - name: h2c
            containerPort: 8080
  ```

4. Use `kubectl` to deploy the service.

  ```shell
  kubectl apply -f grpc-service.yaml
  ```

## Testing the service

**Get the inference service address:**

```
kubectl get ksvc grpc-service-predictor-default
```

**Expect output**

```
NAME                             URL                                                                  READY   REASON
grpc-service-predictor-default   http://grpc-service-predictor-default.default.1.2.3.4.xip.io   True    
```



Testing the gRPC service requires using a gRPC client built from the same
protobuf definition used by the server.

The Dockerfile builds the client binary. To run the client you will use the
same container image deployed for the server with an override to the
entrypoint command to use the client binary instead of the server binary.

Replace `{username}` with your Docker Hub user name and run the command:

```shell
docker run --rm {username}/grpc-ping-go \
  /client \
  -server_addr="grpc-service-predictor-default.default.1.2.3.4.xip.io:80" \
  -insecure
```

The arguments after the container tag `{username}/grpc-ping-go` are used
instead of the entrypoint command defined in the Dockerfile `CMD` statement.

**Expect output**

```
2020/06/12 08:06:13 Ping got hello - pong
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.743929451 +0000 UTC m=+240.900023848
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.743960468 +0000 UTC m=+240.900054864
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.743973358 +0000 UTC m=+240.900067753
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.743985219 +0000 UTC m=+240.900079615
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744000796 +0000 UTC m=+240.900095194
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744012112 +0000 UTC m=+240.900106509
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744025087 +0000 UTC m=+240.900119485
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744037323 +0000 UTC m=+240.900131719
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744060392 +0000 UTC m=+240.900154788
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744071567 +0000 UTC m=+240.900165963
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744082464 +0000 UTC m=+240.900176861
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.74410204 +0000 UTC m=+240.900196435
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744120041 +0000 UTC m=+240.900214438
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744139729 +0000 UTC m=+240.900234125
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744150312 +0000 UTC m=+240.900244709
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744162866 +0000 UTC m=+240.900257263
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744177277 +0000 UTC m=+240.900271675
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744188217 +0000 UTC m=+240.900282615
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744200542 +0000 UTC m=+240.900294939
2020/06/12 08:06:13 Got pong 2020-06-12 08:06:13.744212023 +0000 UTC m=+240.900306419
```



