# Predict on a InferenceService using a Custom Image

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Build and push the sample Docker Image

The goal of custom image support is to allow users to bring their own wrapped model inside a container and serve it with KFServing. Please note that you will need to ensure that your container is also running a web server e.g. Flask to expose your model endpoints.

In this example we use Docker to build the sample python server into a container. To build and push with Docker Hub, run these commands replacing {username} with your Docker Hub username:

```
# Build the container on your local machine
docker build -t {username}/custom-sample .

# Push the container to docker registry
docker push {username}/custom-sample
```

## Create the InferenceService

In the `custom.yaml` file edit the container image and replace {username} with your Docker Hub username.

Apply the CRD

```
kubectl apply -f custom.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org/custom-sample created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=custom-sample
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict
```

Expected Output

```
*   Trying 184.172.247.174...
* TCP_NODELAY set
* Connected to 184.172.247.174 (184.172.247.174) port 31380 (#0)
> GET /v1/models/custom-sample:predict HTTP/1.1
> Host: custom-sample.default.example.com
> User-Agent: curl/7.64.1
> Accept: */*
>
< HTTP/1.1 200 OK
< content-length: 31
< content-type: text/html; charset=utf-8
< date: Thu, 13 Feb 2020 21:34:54 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 15
<
Hello Python KFServing Sample!
* Connection #0 to host 184.172.247.174 left intact
* Closing connection 0
```
