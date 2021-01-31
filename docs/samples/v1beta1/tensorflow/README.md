
# Predict on a InferenceService using Tensorflow
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.


## Create the InferenceService
Apply the CRD
```
kubectl apply -f tensorflow.yaml 
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/flower-sample created
```

### Run a prediction
The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`
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

To test a canary rollout, you can apply the canary.yaml 

Apply the CRD
```
kubectl apply -f canary.yaml 
```

To verify if your traffic split percentage is applied correctly, you can use the following command:

```
kubectl get isvc flower-sample
NAME            URL                                        READY   PREV   LATEST   PREVROLLEDOUTREVISION                   LATESTREADYREVISION                     AGE
flower-sample   http://flower-sample.default.example.com   True    90     10       flower-sample-predictor-default-n9zs6   flower-sample-predictor-default-2kwtr   7m15s
```

## Create the InferenceService with gRPC
Apply the CRD
```
kubectl apply -f grpc.yaml 
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/flower-grpc created
```

### Run a prediction
We will be using a python gRPC client for our prediction, so we need to create a python virtual environment and
install the tensorflow-serving-api. 
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
