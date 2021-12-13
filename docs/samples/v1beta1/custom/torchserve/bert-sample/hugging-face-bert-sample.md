# Torchserve custom server with Hugging Face bert sample

## Model archive file creation

Clone [pytorch/serve](https://github.com/pytorch/serve) repository. Install torchserve and torch model archiver with the steps provided. Navigate to examples/Huggingface_Transformers and follow the steps for creating mar file.

[Torchserve Huggingface Transformers Bert example](https://github.com/pytorch/serve/tree/master/examples/Huggingface_Transformers)

## Download and install transformer models

```bash
pip install transformers
```

Modify setup_config.json

```json
{
 "model_name":"bert-base-uncased",
 "mode":"sequence_classification",
 "do_lower_case":"True",
 "num_labels":"2",
 "save_mode":"pretrained",
 "max_length":"150"
}
```

Download models

```bash
python Download_Transformer_models.py
```

Create a requirements.txt and add `transformers` to it.

Run the below command with all files in place to generate mar file.

```bash
torch-model-archiver --model-name BERTSeqClassification --version 1.0 --serialized-file Transformer_model/pytorch_model.bin --handler ./Transformer_handler_generalized.py --extra-files "Transformer_model/config.json,./setup_config.json,./Seq_classification_artifacts/index_to_name.json" -r requirements.txt
```

## Build and push the sample Docker Image

The custom torchserve image is wrapped with model inside the container and serves it with KServe.

In this example we use Docker to build the torchserve image with marfile and config.properties into a container.

Add the `install_py_dep_per_model=true` config in config.properties to install transformers


```json
inference_address=http://0.0.0.0:8080
management_address=http://0.0.0.0:8081
number_of_netty_threads=4
job_queue_size=100
install_py_dep_per_model=true
model_store=/mnt/models
model_snapshot={"name":"startup.cfg","modelCount":1,"models":{"BERTSeqClassification":{"1.0":{"defaultVersion":true,"marName":"BERTSeqClassification.mar","minWorkers":1,"maxWorkers":5,"batchSize":1,"maxBatchDelay":5000,"responseTimeout":120}}}}
```

To build and push with Docker Hub, run these commands replacing {username} with your Docker Hub username:

[Dockerfile for torchserve image building](https://github.com/pytorch/serve/blob/master/docker/Dockerfile)


```bash
# Build the container on your local machine

# For CPU
DOCKER_BUILDKIT=1 docker build --file Dockerfile -t torchserve-bert:latest .

# For GPU
DOCKER_BUILDKIT=1 docker build --file Dockerfile --build-arg BASE_IMAGE=nvidia/cuda:10.1-cudnn7-runtime-ubuntu18.04 -t torchserve-bert-gpu:latest .

# Push the container to docker registry
docker tag torchserve-bert:latest {username}/torchserve-bert:latest
docker push {username}/torchserve-bert:latest
```

## Create the InferenceService

In the `bert.yaml` file edit the container image and replace {username} with your Docker Hub username.

Apply the CRD

```bash
kubectl apply -f bert.yaml
```

Expected Output

```bash
$inferenceservice.serving.kubeflow.org/torchserve-bert created
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=torchserve-bert
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -n <namespace> -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/predictions/BERTSeqClassification -T serve/examples/Huggingface_Transformers/sample_text.txt
```

Expected Output

```bash
*   Trying 44.239.20.204...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (44.239.20.204) port 80 (#0)
> PUT /predictions/BERTSeqClassification HTTP/1.1
> Host: torchserve-bert.kserve-test.example.com
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
* Connection #0 to host torchserve-bert.kserve-test.example.com left intact
Accepted
```
