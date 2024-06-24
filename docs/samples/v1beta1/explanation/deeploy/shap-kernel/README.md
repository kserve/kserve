## Creating your own model and explainer to test on Sklearn and Deeploy Shap Kenel server.

First we need to generate a simple Sklearn model and Shap kenel explainer using Python.

```shell
python train.py
```

For this example the artifacts are available in Google storage.

## Predict and explain on a InferenceService Scikit-learn and Deeploy Shap Kenel server

## Setup
1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create the InferenceService

Apply the CRD
```
kubectl apply -f sample.yaml
```

Expected Output
```
$ inferenceservice.serving.kserve.io/deeploy-sample created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=deeploy-sample
INPUT_PATH=@./sample-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice deeploy-sample -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:explain -d $INPUT_PATH
```

Expected Output

```
{
    "predictions": [
        true
    ],
    "explanations": [
        {
            "shap_values": [
                0.01666666666666666,
                0,
                0.10000000000000013,
                -0.15000000000000008,
                0.03333333333333335,
                -0.4166666666666668,
                0,
                0,
                0.01666666666666672,
                0
            ],
            "expected_value": 0.4
        }
    ]
}
```
