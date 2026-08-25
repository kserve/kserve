
# Deploying a BLOOM LLM model using the FasterTransformer backend
The BigScience BLOOM LM is an open source large language model (LLM) that can be used for a variety of natural language processing tasks including text generation, text classification, and question answering. In this example we will deploy a small version of the BLOOM model (560 million parameters) to KServe using Triton with the FasterTransformer (FT) backend as our inference server.

> **Note**  
> This example requires access to a KServe cluster with GPU support

In this example we will demonstrate:
- Building a custom Triton image that includes the FasterTransformer backend
- Defining a custom `ClusterServingRuntime`
- Exporting the [BLOOM 560m](https://huggingface.co/bigscience/bloom-560m) model for FasterTransformer
- Preparing the model for loading in Triton
- Setting up and building our KServe transformer
- Defining and deploying our inference service
- Running inference on our model

## Building our custom Triton image

Since the officially supported Triton images do not contain the FasterTransformer backend we will need to build our own custom image. The FasterTransformer backend repo [provides detailed instructions on building a custom image](https://github.com/triton-inference-server/fastertransformer_backend#prepare-docker-images). Build the image according to the instructions and push it to a container repository that is accessible by your KServe cluster.

## Setting up a `ClusterServingRuntime`

In order to allow our inference services to easily use our new image we will setup a custom cluster serving runtime. Apply the resource to your cluster as follows, making sure to replace the `image` field with the one you created in the previous step.


```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: ClusterServingRuntime
metadata:
  name: tritonserver-ft
spec:
  annotations:
    prometheus.kserve.io/path: /metrics
    prometheus.kserve.io/port: "8002"
  containers:
  - args:
    - tritonserver
    - --model-store=/mnt/models
    - --grpc-port=9000
    - --http-port=8080
    - --allow-grpc=true
    - --allow-http=true
    image: <my-custom-triton-image>
    name: kserve-container
    resources:
      limits:
        cpu: "1"
        memory: 2Gi
      requests:
        cpu: "1"
        memory: 2Gi
  protocolVersions:
  - v2
  - grpc-v2
  supportedModelFormats:
  - name: triton
    version: "2"
```

## Exporting the BLOOM 560m model

The FasterTransformer repo provides a script for converting the HuggingFace BLOOM model to FasterTransformer format. The following directions are based on those outlined in the [FasterTransformer backend repo](https://github.com/triton-inference-server/fastertransformer_backend/blob/main/docs/gpt_guide.md#run-bloom).

1. Create a working directory
```sh
mkdir ~/triton-bloom
cd ~/triton-bloom
```

2. Clone the HuggingFace BLOOM 560m repository

```sh
# Make sure you have git-lfs installed (https://git-lfs.com)
git lfs install
git clone https://huggingface.co/bigscience/bloom-560m
```

3. Fetch the conversion script

```sh
curl -LO https://raw.githubusercontent.com/NVIDIA/FasterTransformer/main/examples/pytorch/gpt/utils/huggingface_bloom_convert.py
```

4. Convert the HuggingFace model to FT format

```sh
pip3 install numpy transformers torch
python3 huggingface_bloom_convert.py -o bloom -i ./bloom-560m/ -tp 1
``` 

The `-tp` flag specifies the level of tensor parallelism we would like the exported model to have. Tensor parallelism corresponds to the number of GPU that will be used to load and execute the model.

Once this command completes execution you should have a new directory `bloom/1-cpu` that contains the model in FT format.

## Preparing the model for loading into Triton

Now that we have exported our model we need to prepare it for loading into Triton.

1. Store the model according to the Triton repository layout

Triton requires the filesystem to be arranged according to the [model repository specification](https://github.com/triton-inference-server/server/blob/main/docs/user_guide/model_repository.md#repository-layout).

```sh
# Create a folder for our model. Our model will have the name 'fastertransformer'
mkdir -p model-repo/fastertransformer
# Our model will be version 1
cp -r bloom/1-gpu model-repo/fastertransformer/1
# Add the tokenizer artifacts. These will be used by the KServe transformer
mkdir model-repo/fastertransformer/1/tokenizer
cp bloom-560m/tokenizer*.json model-repo/fastertransformer/1/tokenizer
```

2. Fetch the sample BLOOM model config

```sh
curl -L https://raw.githubusercontent.com/triton-inference-server/fastertransformer_backend/main/all_models/bloom/fastertransformer/config.pbtxt \
> model-repo/fastertransformer/config.pbtxt
```

3. Update the config to point to the correct checkpoint path

The FasterTransformer backend requires the `model_checkpoint_path` parameter to point to the absolute path of the exported model on disk. We need to set this to the location that the KServe storage initializer will download the model to.

```sh
parameters {
  key: "model_checkpoint_path"
  value: {
    string_value: "/mnt/models/fastertransformer/1"
  }
}
```

4. Upload your model to a storage location

For this example we will use [S3](https://kserve.github.io/website/0.10/modelserving/storage/s3/s3/) but you may also use any of the other storage providers supported by KServe.

```sh
aws s3 cp --recursive model-repo s3://example-bucket/bloom-ft-example
```

### Building our custom transformer

Our KServe transformer will provide a user friendly interface to our model. The string input provided by the caller will be tokenized before being passed into the model running on Triton. The tokenized result generated by the model will be converted to a string representation before being returned to the caller.

Build and push the container image:

```
docker build -t <my-transformer-image> -f transformer/Dockerfile transformer
docker push <my-transformer-image>
```


### Defining and deploying our inference service

Now we will put everything together by defining our inference service. Make sure to replace the transformer image and both storage URIs (predictor and transformer) with the correct values.

```sh
kubectl apply -f - << EOF
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: bloom-560m
spec:
  predictor:
    model:
      modelFormat:
        name: triton
        version: "2"
      name: ""
      resources:
        limits:
          cpu: "1"
          memory: 8Gi
          nvidia.com/gpu: "1"
        requests:
          cpu: "1"
          memory: 4Gi
          nvidia.com/gpu: "1"
      runtime: tritonserver-ft
      storageUri: s3://example-bucket/bloom-ft-example
  transformer:
    containers:
    - args:
      - --model_name
      - fastertransformer
      - --protocol
      - v2
      - --tokenizer_path
      - /mnt/models/
      command:
      - python
      - transformer.py
      env:
      - name: STORAGE_URI
        value: s3://example-bucket/bloom-ft-example/fastertransformer/1/tokenizer
      image: <my-transformer-image>
      name: kserve-container
      resources:
        limits:
          cpu: "1"
          memory: 2Gi
        requests:
          cpu: "1"
          memory: 2Gi
EOF
```

### Running inference on our model

Wait for the inference service to reach the ready status.

```sh
kubectl get isvc -w
```

Once it is ready we should see something like this:
```sh
NAME           URL                                        READY   LATESTREADYREVISION                    AGE
bloom-560m     http://bloom-560m-default.example.com      True    bloom-560m-predictor-default-00001     1m
```

Let's send a request!

```sh
curl -d '{"inputs":[{"input":"Kubernetes is the best platform to serve your models because","output_len":"18"}]}' \
  http://bloom-560m-default.example.com/v1/models/fastertransformer:predict
```

This should output something like the following:
```sh
[["Kubernetes is the best platform to serve your models because it is a 
cloud-based service that is built on top of the Kubernetes API."]]
```
