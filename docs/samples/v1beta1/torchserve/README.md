# Predict on a InferenceService using Torchserve

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Build image transformer

This example requires image transformer for converting bytes array image to json array. Refer torchserve image tranformer for building image.

[Image-transformer](../../transformer/torchserve_image_transformer/README.md)

## Creating model storage with model archive file

Refer [model archive file generation](./model-archiver/README.md)

## Create the InferenceService

Apply the CRD

```bash
kubectl apply -f transformer.yaml
```

Expected Output

```bash
$inferenceservice.serving.kubeflow.org/torchserve created
```

[Torchserve inference endpoints](https://github.com/pytorch/serve/blob/master/docs/inference_api.md)

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=torchserve
SERVICE_HOSTNAME=$(kubectl get inferenceservice torchserve -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/mnist:predict -d @./imgconv/input.json
```

Get Pods

```bash
kubectl get pods -n <namespace>

NAME                                                                  READY   STATUS    RESTARTS   AGE
pod/torchserve-predictor-default-8mw55-deployment-57f979c88-f2dkn     2/2     Running   0          4m25s
pod/torchserve-transformer-default-fssw5-deployment-74cbd5798f94rtd   2/2     Running   0          4m25s
```

Expected Output

```bash
*   Trying 52.89.19.61...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (52.89.19.61) port 80 (#0)
> PUT /predictions/mnist HTTP/1.1
> Host: torchserve-predictor-default.kfserving-test.example.com
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 167
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< cache-control: no-cache; no-store, must-revalidate, private
< content-length: 1
< date: Tue, 27 Oct 2020 08:26:19 GMT
< expires: Thu, 01 Jan 1970 00:00:00 UTC
< pragma: no-cache
< x-request-id: b10cfc9f-cd0f-4cda-9c6c-194c2cdaa517
< x-envoy-upstream-service-time: 6
< server: istio-envoy
<
* Connection #0 to host a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com left intact
{
    "predictions": [
        2
    ]
}
```

## For Autoscaling

Configurations for autoscaling pods [Auto scaling](docs/autoscaling.md)

## Canary Rollout

Configurations for canary [Canary Deployment](docs/canary.md)
