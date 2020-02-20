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

```sh
MODEL_NAME=kfservingsdksample
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl --request POST \
  --url http://${CLUSTER_IP}/v1/models/${MODEL_NAME}:predict \
  --header "host: ${SERVICE_HOSTNAME}" \
  --data '{
    "instances": [
      "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEAYABgAAD/2 . . . # base64 encoded data URI"
    ]
}'
```
