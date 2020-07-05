# A sample for gRPC Server in custom model

This sample can be used to try out gRPC, HTTP/2, and custom port configuration
in a custom inference  service.

## Build and Deploy the sample code

1. Use Docker to build a container image for this service and push to Docker Hub.

  Replace `{username}` with your Docker Hub username then run the commands:

  ```shell
  # Build the container on your local machine.
  docker build -t {username}/helloworld-grpc .

  # Push the container to docker registry.
  docker push {username}/helloworld-grpc
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
          name: grpc
          image: docker.io/{username}/helloworld-grpc:latest
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
kubectl get inferenceservice grpc-service
```

**Expect output**

```
NAME           URL                                                READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
grpc-service   http://grpc-service.default.1.2.3.4.xip.io         True    100                                4m30s
```
**How to setup the magic DNS**

 [Install Istio for Knative](https://knative.dev/docs/install/installing-istio/), at the `Configuring DNS` section, it describe how to config the DNS. 

You can also take a look at this [sample](https://github.com/kubeflow/kfserving/tree/master/docs/samples#deploy-kfserving-inferenceservice-with-a-custom-predictor) for more `custom model` detail.

Testing the gRPC service requires using a gRPC client built from the same
protobuf definition used by the server.

The Dockerfile builds the client binary. To run the client you will use the
same container image deployed for the server with an override to the
entrypoint command to use the client binary instead of the server binary.

Replace `{username}` with your Docker Hub user name and run the command:

```shell
docker run --rm {username}/helloworld-grpc \
  /client \
  -server_addr="grpc-service.default.1.2.3.4.xip.io:80" \
  -insecure
```

The arguments after the container tag `{username}/helloworld-grpc are used
instead of the entrypoint command defined in the Dockerfile `CMD` statement.

**Expect output**

```
2020/06/18 09:35:13 SyaHello got Hello world
2020/06/18 09:35:13 SendSomething got Hello KFServing
```



