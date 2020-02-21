# Predict on a InferenceService using the KFServing sdk

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.

## Build and push the sample Docker Image

The goal of custom image support is to allow users to bring their own wrapped model inside a container and serve it with KFServing. Please note that you will need to ensure that your container is also running a web server e.g. Flask to expose your model endpoints.

In this example we use Docker to build the sample python server into a container. To build and push with Docker Hub, run these commands replacing {username} with your Docker Hub username:

```
# Build the container on your local machine
docker build -t {username}/kfservingsdksample .

# Push the container to docker registry
docker push {username}/kfservingsdksample
```

## Create the InferenceService

In the `custom.yaml` file edit the container image and replace {username} with your Docker Hub username.

Apply the CRD

```
kubectl apply -f custom.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org/kfservingsdksample created
```

## Run a prediction

```
MODEL_NAME=kfservingsdksample
INPUT_PATH=@./input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${CLUSTER_IP}/v1/models/${MODEL_NAME}:predict -d $INPUT_PATH
```

Expected Output:
```
*   Trying 184.172.247.174...
* TCP_NODELAY set
* Connected to 184.172.247.174 (184.172.247.174) port 31380 (#0)
> POST /v1/models/kfservingsdksample:predict HTTP/1.1
> Host: kfservingsdksample.default.example.com
> User-Agent: curl/7.64.1
> Accept: */*
> Content-Length: 105318
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 224
< content-type: text/html; charset=UTF-8
< date: Fri, 21 Feb 2020 14:36:57 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 251
<
* Connection #0 to host 184.172.247.174 left intact
{"predictions": {"Labrador retriever": 41.58518600463867, "golden retriever": 16.59165382385254, "Saluki, gazelle hound": 16.286855697631836, "whippet": 2.853914976119995, "Ibizan hound, Ibizan Podenco": 2.3924756050109863}}* Closing connection 0
```