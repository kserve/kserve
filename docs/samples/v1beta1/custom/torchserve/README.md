# Predict on a InferenceService using a Custom Torchserve Image

In this example we use torchserve as custom server to serve an mnist model. The idea of using torchserve as custom server is to make the transition for new users from torchserve to kfserving easier.

## Setup

1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

### This example requires v1beta1/KFS 0.5

## Build and push the sample Docker Image

The custom torchserve image is wrapped with model inside the container and serves it with KServe.

In this example we build a torchserve image with marfile and config.properties into a container. To build and push with Docker Hub, run these commands replacing {username} with your Docker Hub username:

Refer steps for [building and publishing](./torchserve-image/README.md) docker image.

## Create the InferenceService

In the `torchserve-custom.yaml` file edit the container image and replace {username} with your Docker Hub username.

Apply the CRD

```bash
kubectl apply -f torchserve-custom.yaml
```

Expected Output

```bash
$inferenceservice.serving.kubeflow.org/torchserve-custom created
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

Download input image:

wget https://raw.githubusercontent.com/pytorch/serve/master/examples/image_classifier/mnist/test_data/0.png

```bash
MODEL_NAME=torchserve-custom
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -n <namespace> -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/predictions/mnist -T 0.png
```

Expected Output

```bash
*   Trying 52.89.19.61...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (52.89.19.61) port 80 (#0)
> PUT /predictions/mnist HTTP/1.1
> Host: torchserve-custom.kfserving-test.example.com
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 272
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< cache-control: no-cache; no-store, must-revalidate, private
< content-length: 1
< date: Fri, 23 Oct 2020 13:01:09 GMT
< expires: Thu, 01 Jan 1970 00:00:00 UTC
< pragma: no-cache
< x-request-id: 8881f2b9-462e-4e2d-972f-90b4eb083e53
< x-envoy-upstream-service-time: 5018
< server: istio-envoy
<
* Connection #0 to host a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com left intact
0
```

## For Autoscaling

Configurations for autoscaling pods [Auto scaling](docs/autoscaling.md)

## Canary Rollout

Configurations for canary [Canary Deployment](docs/canary.md)

## Log aggregation

Follow the link for torchserve log aggregation in kubernetes.
[Log aggregation with EFK Stack](https://www.digitalocean.com/community/tutorials/how-to-set-up-an-elasticsearch-fluentd-and-kibana-efk-logging-stack-on-kubernetes)
