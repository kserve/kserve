# Predict on an InferenceService with Tensorflow model
## Setup
1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.


## Create the InferenceService with HTTP
 
Create an `InferenceService` yaml which specifies the framework `tensorflow` and `storageUri` that is pointed to a saved tensorflow model,
by default it exposes a HTTP/REST endpoint.
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "flower-sample"
spec:
  predictor:
    tensorflow:
      storageUri: "gs://kfserving-examples/models/tensorflow/flowers"
```

Apply the `tensorflow.yaml` to create the `InferenceService`.
```
kubectl apply -f tensorflow.yaml 
```

Expected Output
```
$ inferenceservice.serving.kserve.io/flower-sample created
```

Wait the `InferenceService` to be in ready state
```shell
kubectl get isvc flower-sample
NAME            URL                                        READY   PREV   LATEST   PREVROLLEDOUTREVISION        LATESTREADYREVISION                     AGE
flower-sample   http://flower-sample.default.example.com   True           100                                   flower-sample-predictor-default-n9zs6   7m15s
```
 
### Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`
```
MODEL_NAME=flower-sample
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

Expected Output
```
* Connected to localhost (::1) port 8080 (#0)
> POST /v1/models/tensorflow-sample:predict HTTP/1.1
> Host: tensorflow-sample.default.example.com
> User-Agent: curl/7.73.0
> Accept: */*
> Content-Length: 16201
> Content-Type: application/x-www-form-urlencoded
> 
* upload completely sent off: 16201 out of 16201 bytes
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< content-length: 222
< content-type: application/json
< date: Sun, 31 Jan 2021 01:01:50 GMT
< x-envoy-upstream-service-time: 280
< server: istio-envoy
< 
{
    "predictions": [
        {
            "scores": [0.999114931, 9.20987877e-05, 0.000136786213, 0.000337257545, 0.000300532585, 1.84813616e-05],
            "prediction": 0,
            "key": "   1"
        }
    ]
}
```

## Canary Rollout

Canary rollout is a great way to control the risk of rolling out a new model by first moving a small percent of the traffic to it and then gradually increase the percentage. 
To run a canary rollout, you can apply the `canary.yaml` with the `canaryTrafficPercent` field specified.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "flower-example"
spec:
  predictor:
    canaryTrafficPercent: 20
    tensorflow:
      storageUri: "gs://kfserving-examples/models/tensorflow/flowers-2"
```

```
kubectl apply -f canary.yaml 
```

To verify if the traffic split percentage is applied correctly, you can run the following command:

```
kubectl get isvc flower-sample
NAME            URL                                        READY   PREV   LATEST   PREVROLLEDOUTREVISION                   LATESTREADYREVISION                     AGE
flower-sample   http://flower-sample.default.example.com   True    80     20       flower-sample-predictor-default-n9zs6   flower-sample-predictor-default-2kwtr   7m15s
```

As you can see the traffic is split between the last rolled out revision and the current latest ready revision, KServe automatically tracks the last rolled out(stable) revision for you so you
do not need to maintain both default and canary on the `InferenceService` as in v1alpha2.


## Create the InferenceService with gRPC
Create `InferenceService` which exposes the gRPC port and by default it listens on port 9000.
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "flower-grpc"
spec:
  predictor:
    tensorflow:
      storageUri: "gs://kfserving-examples/models/tensorflow/flowers"
      ports:
        - containerPort: 9000
          name: h2c
          protocol: TCP
```

Apply `grpc.yaml` to create the gRPC InferenceService.
```
kubectl apply -f grpc.yaml 
```

Expected Output
```
$ inferenceservice.serving.kserve.io/flower-grpc created
```

### Run a prediction
We use a python gRPC client for the prediction, so you need to create a python virtual environment and
install the `tensorflow-serving-api`. 
```shell
# The prediction script is written in TensorFlow 1.x
pip install tensorflow-serving-api>=1.14.0,<2.0.0
```

Run prediction script
```shell
MODEL_NAME=flower-grpc
INPUT_PATH=./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
python grpc_client.py --host $INGRESS_HOST --port $INGRESS_PORT --model $MODEL_NAME --hostname $SERVICE_HOSTNAME --input_path $INPUT_PATH
```

Expected Output
```
outputs {
  key: "key"
  value {
    dtype: DT_STRING
    tensor_shape {
      dim {
        size: 1
      }
    }
    string_val: "   1"
  }
}
outputs {
  key: "prediction"
  value {
    dtype: DT_INT64
    tensor_shape {
      dim {
        size: 1
      }
    }
    int64_val: 0
  }
}
outputs {
  key: "scores"
  value {
    dtype: DT_FLOAT
    tensor_shape {
      dim {
        size: 1
      }
      dim {
        size: 6
      }
    }
    float_val: 0.9991149306297302
    float_val: 9.209887502947822e-05
    float_val: 0.00013678647519554943
    float_val: 0.0003372581850271672
    float_val: 0.0003005331673193723
    float_val: 1.848137799242977e-05
  }
}
model_spec {
  name: "flowers-sample"
  version {
    value: 1
  }
  signature_name: "serving_default"
}
```
