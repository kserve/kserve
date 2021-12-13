# Canary Rollouts

## Deployment yaml

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "torchserve-custom"
spec:
  predictor:
    containers:
    - image: {username}/torchserve:latest
      name: torchserve-container
```

### Canary model

Change the path and deploy

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "torchserve-custom"
spec:
  predictor:
    canaryTrafficPercent: 20
    containers:
    - image: {username}/torchserve_v2:latest
      name: torchserve-container
```

## Create the InferenceService

Apply the CRD

```bash
kubectl apply -f torchserve-custom.yaml
```

Apply canary

```bash
kubectl apply -f canary.yaml
```

Expected Output

```bash
$inferenceservice.serving.kserve.io/torchserve-custom created
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=torchserve-custom
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/predictions/mnist -T 1.png
```

Expected Output

```bash
*   Trying 52.89.19.61...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (52.89.19.61) port 80 (#0)
> PUT /predictions/mnist HTTP/1.1
> Host: torchserve-custom.kserve-test.example.com
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
1
```

### Get Pods

```bash
Kubectl get pods -n kserve-test

NAME                                                             READY   STATUS        RESTARTS   AGE
torchserve-custom-predictor-default-cj2d8-deployment-69444c9c74-tsrwr   2/2     Running       0          113s
torchserve-custom-predictor-default-cj2d8-deployment-69444c9c74-vvpjl   2/2     Running       0          109s
```

## Get Revisions

Install [Knative CLI tool](https://knative.dev/docs/install/install-kn/)

```bash
kn revision list -n <namespace>

NAME                                                 SERVICE                                           TRAFFIC   TAGS     GENERATION   AGE   CONDITIONS   READY   REASON
torchserve-custom-predictor-defaul ...   torchserve-custom-predictor-default   20%       latest   2            34s   4 OK / 4     True    
torchserve-custom-predictor-defaul ...   torchserve-custom-predictor-default   80%       prev     1            56s   4 OK / 4     True  
```
