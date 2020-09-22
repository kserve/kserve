# Deploying a Time-Series model (Holt Winter's) using a KFServing Model Server

The goal of custom image support is to allow users to bring their own wrapped model inside a container and serve it with KFServing. Please note that you will need to ensure that your container is also running a web server e.g. Flask to expose your model endpoints. This example located in the `model-server` directory extends `kfserving.KFModel` which uses the tornado web server.

 You can use model_creation.py to create a joblib file

## Deploy a custom image InferenceService using the command line

### Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

### Build and push the sample Docker Image

In this example we use Docker to build the sample python server into a container. To build and push with Docker Hub, run these commands replacing {username} with your Docker Hub username:

```
# Build the container on your local machine
docker build -t {username}/kfserving-custom-model .

# Push the container to docker registry
docker push {username}/kfserving-custom-model
```

### Create the InferenceService

In the `custom.yaml` file edit the container image and replace {username} with your Docker Hub username.

Apply the CRD

```
kubectl apply -f custom.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org/sales-application created
```

### Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=sales-application
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict -d '{"instances": [{ "image": {"weeks":"4"}}]}'
```

Expected Output:

```

* About to connect() to 169.0.0.1 port 31380 (#0)
*   Trying 169.0.0.1...
* Connected to 169.0.0.1 (169.0.0.1) port 31380 (#0)
> POST /v1/models/sales-application:predict HTTP/1.1
> User-Agent: curl/7.29.0
> Accept: */*
> Host: sales-application.kfserving.169.0.0.1.xip.io
> Content-Length: 42
> Content-Type: application/x-www-form-urlencoded
> 
* upload completely sent off: 42 out of 42 bytes
< HTTP/1.1 200 OK
< content-length: 145
< content-type: application/json; charset=UTF-8
< date: Fri, 18 Sep 2020 12:46:27 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 60
< 
* Connection #0 to host 169.0.0.1 left intact
{"predictions": {"forecast_weeks": "4", "predicted_values": "[8096.28550854 8342.86514061 8420.74367698 8754.6565151 ]"}}
```

### Delete the InferenceService

```
kubectl delete -f custom.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org "sales-application" deleted
```



