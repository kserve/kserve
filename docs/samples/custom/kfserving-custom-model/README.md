# Predict on a InferenceService using a KFServing Model Server

The goal of custom image support is to allow users to bring their own wrapped model inside a container and serve it with KFServing. Please note that you will need to ensure that your container is also running a web server e.g. Flask to expose your model endpoints. This example located in the `model-server` directory extends `kfserving.KFModel` which uses the tornado web server.

You can choose to deploy the model server using the kubectl command line, or using the KFServing client SDK.

## Table of contents

[Deploy using the KFServing client SDK](#deploy-a-custom-image-inferenceservice-using-the-kfserving-client-sdk)

[Deploy using the command line](#deploy-a-custom-image-inferenceservice-using-the-command-line)

## Deploy a custom image InferenceService using the KFServing client SDK

Install Jupyter and the other depedencies needed to run the python notebook

```
pip install -r requirements.txt
```

Start Jupyter and open the notebook

```
jupyter notebook kfserving_sdk_custom_image.ipynb
```

Follow the instructions in the notebook to deploy the InferenseService with the KFServing client SDK

## Deploy a custom image InferenceService using the command line

### Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

### Build and push the sample Docker Image

In this example we use Docker to build the sample python server into a container. To build and push with Docker Hub, run these commands replacing {username} with your Docker Hub username:

```
# Build the container on your local machine
docker build -t {username}/kfserving-custom-model ./model-server

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
$ inferenceservice.serving.kubeflow.org/kfserving-custom-model created
```

### Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=kfserving-custom-model
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict -d $INPUT_PATH
```

Expected Output:

```
*   Trying 169.47.250.204...
* TCP_NODELAY set
* Connected to 169.47.250.204 (169.47.250.204) port 80 (#0)
> POST /v1/models/kfserving-custom-model:predict HTTP/1.1
> Host: kfserving-custom-model.default.example.com
> User-Agent: curl/7.64.1
> Accept: */*
> Content-Length: 105339
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 232
< content-type: text/html; charset=UTF-8
< date: Wed, 26 Feb 2020 15:19:15 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 213
<
* Connection #0 to host 169.47.250.204 left intact
{"predictions": {"Labrador retriever": 0.4158518612384796, "golden retriever": 0.1659165322780609, "Saluki, gazelle hound": 0.16286855936050415, "whippet": 0.028539149090647697, "Ibizan hound, Ibizan Podenco": 0.023924754932522774}}* Closing connection 0
```

### Delete the InferenceService

```
kubectl delete -f custom.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org "kfserving-custom-model" deleted
```
