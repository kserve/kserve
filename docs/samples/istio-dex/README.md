# KFServing on Kubeflow with Istio-Dex

This example shows how to create a KFServing InferenceService as well as sending a prediction request to the InferenceService in an Istio-Dex environment.

We will be using the [SKLearn example](../sklearn) to create our InferenceService.

## Setup
Deploy a Multi-user, auth-enabled Kubeflow from Kustomize manifests using [kftcl_istio_dex.yaml](https://raw.githubusercontent.com/kubeflow/manifests/v1.1-branch/kfdef/kfctl_istio_dex.yaml)

## Create the InferenceService

Apply the CRD to your namespace (this example uses the namespace `admin`)

```bash
kubectl apply -f sklearn.yaml -n admin
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/sklearn-iris created
```

## Run a prediction

### Authentication 

There are 2 methods to obtain the authservice_session token in order to send authenticated prediction requests to the `InferenceService`.

* From CLI

    Follow the steps:

    ```bash
    1) CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.clusterIP}')

    2) curl -v http://${CLUSTER_IP}
    Response:
    >> <a href="/dex/auth?client_id=kubeflow-oidc-authservice&amp;redirect_uri=%2Flogin%2Foidc&amp;response_type=code&amp;scope=profile+email+groups+openid&amp;state=STATE_VALUE">Found</a>.

    STATE=STATE_VALUE

    3) curl -v "http://${CLUSTER_IP}/dex/auth?client_id=kubeflow-oidc-authservice&redirect_uri=%2Flogin%2Foidc&response_type=code&scope=profile+email+groups+openid&amp;state=${STATE}"
    Response:
    >> <a href="/dex/auth/local?req=REQ_VALUE">Found</a>

    REQ=REQ_VALUE
    4) curl -v "http://${CLUSTER_IP}/dex/auth/local?req=${REQ}" -H 'Content-Type: application/x-www-form-urlencoded' --data 'login=admin%40kubeflow.org&password=12341234'

    5) curl -v "http://${CLUSTER_IP}/dex/approval?req=${REQ}"

    Response:
    >> <a href="/login/oidc?code=CODE_VALUE&amp;state=STATE_VALUE">See Other</a>.

    CODE=CODE_VALUE

    6) curl -v "http://${CLUSTER_IP}/login/oidc?code=${CODE}&amp;state=${STATE}"

    Response:
    >> set-cookie authservice_session=SESSION

    SESSION=SESSION
    ```

* From the browser
    
    1. Log in to Kubeflow Central Dashboard with your user account.
    2. View cookies used in the Kubeflow Central Dashboard site from your browser.
    3. Copy the token content from the cookie `authservice_session`

        ![authservice_session](https://user-images.githubusercontent.com/41395198/81792510-bbd28800-953a-11ea-8cab-f9bee161d5a7.png)

### Prediction

```bash
MODEL_NAME=sklearn-iris
INPUT_PATH=@./iris-input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.clusterIP}')
SERVICE_HOSTNAME=$(kubectl get -n admin inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" -H "Cookie: authservice_session=${SESSION}" http://${CLUSTER_IP}/v1/models/${MODEL_NAME}:predict -d ${INPUT_PATH}
```

Expected Output

```bash
*   Trying 10.152.183.241...
* TCP_NODELAY set
* Connected to 10.152.183.241 (10.152.183.241) port 80 (#0)
> POST /v1/models/sklearn-iris:predict HTTP/1.1
> Host: sklearn-iris.admin.example.com
> User-Agent: curl/7.58.0
> Accept: */*
> Cookie: authservice_session=MTU4OTI5NDAzMHxOd3dBTkVveldFUlRWa3hJUVVKV1NrZE1WVWhCVmxSS05GRTFSMGhaVmtWR1JrUlhSRXRRUmtnMVRrTkpUekpOTTBOSFNGcElXRkU9fLgsofp8amFkZv4N4gnFUGjCePgaZPAU20ylfr8J-63T
> Content-Length: 76
> Content-Type: application/x-www-form-urlencoded
> 
* upload completely sent off: 76 out of 76 bytes
< HTTP/1.1 200 OK
< content-length: 23
< content-type: text/html; charset=UTF-8
< date: Tue, 12 May 2020 14:38:50 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 7307
< 
* Connection #0 to host 10.152.183.241 left intact
{"predictions": [1, 1]}
```
