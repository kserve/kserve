# TorchServe example with Huggingface bert model
In this example we will show how to serve [Huggingface Transformers with TorchServe](https://github.com/pytorch/serve/tree/master/examples/Huggingface_Transformers)
on KServe.

## Model archive file creation

Clone [pytorch/serve](https://github.com/pytorch/serve) repository,
navigate to `examples/Huggingface_Transformers` and follow the steps for creating the MAR file including serialized model and other dependent files.
TorchServe supports both eager model and torchscript and here we save as the pretrained model. 
 
```bash
torch-model-archiver --model-name BERTSeqClassification --version 1.0 \
--serialized-file Transformer_model/pytorch_model.bin \
--handler ./Transformer_handler_generalized.py \
--extra-files "Transformer_model/config.json,./setup_config.json,./Seq_classification_artifacts/index_to_name.json"
```

## Create the InferenceService

Apply the CRD

```bash
kubectl apply -f bert.yaml
```

Expected Output

```bash
$inferenceservice.serving.kubeflow.org/torchserve-bert created
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=torchserve-bert
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -n <namespace> -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/BERTSeqClassification:predict -d ./sample_text.txt
```

Expected Output

```bash
*   Trying 44.239.20.204...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (44.239.20.204) port 80 (#0)
> PUT /v1/models/BERTSeqClassification:predict HTTP/1.1
> Host: torchserve-bert.kfserving-test.example.com
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 79
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< cache-control: no-cache; no-store, must-revalidate, private
< content-length: 8
< date: Wed, 04 Nov 2020 10:54:49 GMT
< expires: Thu, 01 Jan 1970 00:00:00 UTC
< pragma: no-cache
< x-request-id: 4b54d3ac-185f-444c-b344-b8a785fdeb50
< x-envoy-upstream-service-time: 2085
< server: istio-envoy
<
* Connection #0 to host torchserve-bert.kfserving-test.example.com left intact
Accepted
```

## Captum Explanations
In order to understand the word importances and attributions when we make an explanation Request, we use Captum Insights for the HuggingFace Transformers pre-trained model.
```bash
MODEL_NAME=torchserve-bert
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -n <namespace> -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/BERTSeqClassification:explain -d ./sample_text.txt
```
Expected output
```bash
*   Trying ::1:8080...
* Connected to localhost (::1) port 8080 (#0)
> POST /v1/models/BERTSeqClassification:explain HTTP/1.1
> Host: torchserve-bert.default.example.com
> User-Agent: curl/7.73.0
> Accept: */*
> Content-Length: 84
> Content-Type: application/x-www-form-urlencoded
>Handling connection for 8080
 
* upload completely sent off: 84 out of 84 bytes
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< content-length: 292
< content-type: application/json; charset=UTF-8
< date: Sun, 27 Dec 2020 05:53:52 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 5769
< 
* Connection #0 to host localhost left intact
{"explanations": [{"importances": [0.0, -0.6324463574494716, -0.033115653530477414, 0.2681695752722339, -0.29124745608778546, 0.5422589681903883, -0.3848768219546909, 0.0], 
"words": ["[CLS]", "bloomberg", "has", "reported", "on", "the", "economy", "[SEP]"], "delta": -0.0007350619859377225}]}
```


