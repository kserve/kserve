# KServe with Fluid on LLM (Large Language Model) Acceleration

In this example we will show how to use [Fluid](https://github.com/fluid-cloudnative/fluid) to improve autoscaling performance on LLM (Large Language Model) in KServe.

## DISCLAIMER

This Proof of Concept (PoC) is a collaborative effort between KServe and Fluid communities to integrate KServe with Fluid. The purpose of this PoC is to experiment with the use of Fluid to improve the autoscaling performance on LLMs.

It is important to note that this PoC has not been tested in a production environment and is intended for experimental purposes only. Therefore, if you decide to try it out, please carefully validate it in your use case before deploying it in a production environment.
## Context

With the increasing popularity of large language models (LLMs), many companies are now deploying them via KServe. However, due to their typical size ranging from a few GBs to several hundred GBs, it can take a considerable amount of time for the KServe Storage Initializer to pull them from remote storage. As a result, the start-up time for an inference service can take anywhere from a few minutes to over 30 minutes due to the lengthy model downloading and loading times. This makes the serverless autoscaling function of KServe less effective for LLM use cases.
To improve performance, it becomes crucial to cache the model artifacts rather than pulling them from remote storage during the autoscaling process. Since large language models are often served on GPU instances, which can be expensive, autoscaling becomes a helpful tool for reducing costs by allowing resources to be scaled up or down based on demand.

## Prerequisite

* [KServe](https://kserve.github.io/website/master/admin/serverless/serverless/)
  * Please follow this [guideline](https://kserve.github.io/website/master/developer/developer/#deploy-kserve-from-master-branch) to deploy KServe from `master` branch
  * Enable node selector in KServe InferenceService via KNative

    ```sh

    kubectl patch configmap/config-features \
      --namespace knative-serving \
      --type merge \
      --patch '{"data":{"kubernetes.podspec-nodeselector":"enabled", "kubernetes.podspec-tolerations":"enabled"}}'
    ```
  * Enable Direct VolumeMount for PVC In KServe

    ```sh
    # edit inferenceservice-config and update enableDirectPvcVolumeMount to true
    kubectl edit configmap inferenceservice-config  -n kserve

    #   storageInitializer: |-
    #     {
    #         "image" : "kserve/storage-initializer:latest",
    #         "memoryRequest": "100Mi",
    #         "memoryLimit": "1Gi",
    #         "cpuRequest": "100m",
    #         "cpuLimit": "1",
    #         "storageSpecSecretName": "storage-config",
    #         "enableDirectPvcVolumeMount": false        # change to true
    #     }
    ```

* [Fluid](https://github.com/fluid-cloudnative/fluid/blob/master/docs/en/userguide/get_started.md)
  * Deploy Fluid

    ```sh
    # Add Fluid repository to Helm repos and keep it up-to-date
    helm repo add fluid https://fluid-cloudnative.github.io/charts
    helm repo update
    # we use the devel version here
    helm upgrade --install \
      --set webhook.reinvocationPolicy=IfNeeded \
      fluid --create-namespace --namespace=fluid-system fluid/fluid --devel
    ```

## Prepare Demo Application

The demo application uses KServe to serve a LLM (Large Language Model). In this tutorial [BLOOM](https://huggingface.co/docs/transformers/model_doc/bloom) model is used as an example.

### Prerequisite

Checkout KServe repo and go to the example:

```sh
git clone https://github.com/kserve/kserve.git
cd kserve/docs/samples/fluid
```

### Build and Push Application Image

```sh
export DOCKER_REPO='docker.io/<username>'

# build and push application image
cd docker
docker build -f Dockerfile -t ${DOCKER_REPO}/kserve-fluid:bloom-gpu-v1 .
docker push ${DOCKER_REPO}/kserve-fluid:bloom-gpu-v1
cd ..
```

### Prepare Model Artifact

Download model from `HuggingFace`, `BLOOM` models are using in this tutorial.

```sh
python -m venv .venv
source .venv/bin/activate
pip install --upgrade pip
pip install 'transformers[torch]'

# create models folder
mkdir -p models

# please check https://huggingface.co/docs/transformers/model_doc/bloom for other BLOOM models and update the command accordingly
python3 download_model.py --model_name="bigscience/bloom-560m" --model_dir="models"
# export output_dir=models/models--bigscience--bloom-560m/snapshots/e985a63cdc139290c5f700ff1929f0b5942cced2
```

Structure of the model artifact:
```sh
❯ tree ${output_dir}
models/models--bigscience--bloom-560m/snapshots/e985a63cdc139290c5f700ff1929f0b5942cced2
├── LICENSE -> ../../blobs/ab1f554e777c9a2075dca946ae706c0ab257a9de
├── README.md -> ../../blobs/2d31ada6c9c81597141987c8d8eeeb1df27c42ef
├── config.json -> ../../blobs/a9f31df161b949147c63449ead0bd4e5fc70770d
├── flax_model.msgpack -> ../../blobs/fdd2b3658489e7d17525b5ccfc656e1e056c4b47360e2a3bd808bce15bf4a79c
├── model.safetensors -> ../../blobs/a8702498162c95d68d2724e7f333c83d7be08de81cfc091455c38730682116d3
├── pytorch_model.bin -> ../../blobs/8b42cc000f3764cd7983479c71e01cf7bda8e37352cef99cc44f36f5944377d8
├── special_tokens_map.json -> ../../blobs/25bc39604f72700b3b8e10bd69bb2f227157edd1
├── tokenizer.json -> ../../blobs/3fa39cd4b1500feb205bcce3b9703a4373414cafe4970e0657b413f7ddd2a9d3
└── tokenizer_config.json -> ../../blobs/e7016b49fcff7e162946ec012d3c7b4db0b66d87

0 directories, 9 files
```

Upload the model to S3 bucket:
```sh
# update the path accordingly
aws s3 cp --recursive ${output_dir} s3://${bucket}/models/bloom-560m
```

### Run The Application Locally

```sh
docker run -it --rm -p 8080:8080 \
  -v $(pwd)/models:/mnt/models \
  -e MODEL_URL=/mnt/${output_dir} \
  -e MODEL_NAME=bloom \
  ${DOCKER_REPO}/kserve-fluid:bloom-gpu-v1
```

### Test The Application

```sh
curl -i -X POST -H "Content-Type: application/json" "localhost:8080/v1/models/bloom:predict" -d '{"prompt": "It was a dark and stormy night", "result_length": 50}'
```

## Demo

Let's start our demo.

### Create Demo Namespace

```sh
kubectl create ns kserve-fluid-demo

# add label to inject fluid sidecar
kubectl label namespace kserve-fluid-demo fluid.io/enable-injection=true
```

### Create S3 Credentials Secret And Service Account

```sh
# please update the s3 credentials to yours
# NOTE: for Jindo runtime, https is not supported in current version
kubectl create -f s3creds.yaml -n kserve-fluid-demo
```

### Create Dataset And Runtime

Create Dataset and Runtime (In this tutorial you will use `JindoFS` as the runtime, there are other runtimes like [JuiceFS](https://github.com/juicedata/juicefs) and [Alluxio](https://github.com/Alluxio/alluxio) etc).

```sh
# please update the `mountPoint` and `options`
kubectl create -f jindo.yaml -n kserve-fluid-demo

dataset.data.fluid.io/s3-data created
jindoruntime.data.fluid.io/s3-data created
```

Check the status:

```sh
kubectl get po -n kserve-fluid-demo

NAME                       READY   STATUS    RESTARTS   AGE
s3-data-jindofs-master-0   1/1     Running   0          28s
s3-data-jindofs-worker-0   1/1     Running   0          5s
s3-data-jindofs-worker-1   1/1     Running   0          5s


kubectl get jindoruntime,dataset -n kserve-fluid-demo

NAME                                 MASTER PHASE   WORKER PHASE   FUSE PHASE   AGE
jindoruntime.data.fluid.io/s3-data   Ready          Ready          Ready        70s

NAME                            UFS TOTAL SIZE   CACHED   CACHE CAPACITY   CACHED PERCENTAGE   PHASE   AGE
dataset.data.fluid.io/s3-data   3.14GiB          0.00B    100.00GiB        0.0%                Bound   70s
```

A PVC is created to mount the dataset into the application:

```
kubectl get pvc -n kserve-fluid-demo

NAME      STATUS   VOLUME                      CAPACITY   ACCESS MODES   STORAGECLASS   AGE
s3-data   Bound    kserve-fluid-demo-s3-data   100Pi      ROX            fluid          65s
```

### Preload The Data

Preload the data into workers to improve the performance:

```sh
# please update the `path` under `target`
kubectl create -f dataload.yaml -n kserve-fluid-demo
```

Check the dataload status:

```sh
kubectl get dataload -n kserve-fluid-demo

NAME          DATASET   PHASE      AGE     DURATION
s3-dataload   s3-data   Complete   4m50s   2m22s

kubectl get dataset -n kserve-fluid-demo

NAME      UFS TOTAL SIZE   CACHED    CACHE CAPACITY   CACHED PERCENTAGE   PHASE   AGE
s3-data   3.14GiB          3.02GiB   100.00GiB        96.2%               Bound   13m
```

Data are cached successfully.

### Deploy The Demo Application

Create the InferenceService with Fluid:

```sh
# please update the `STORAGE_URI`
kubectl create -f fluid-isvc.yaml -n kserve-fluid-demo

# run this command if you want to use the KServe Storage Initializer
kubectl create -f kserve-isvc.yaml -n kserve-fluid-demo
```

### Test The Demo Application

Follow this [guide](https://kserve.github.io/website/0.10/get_started/first_isvc/#4-determine-the-ingress-ip-and-ports) to get the ingress host and port:

```sh
export MODEL_NAME=fluid-bloom
# export MODEL_NAME=kserve-bloom
export SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -n kserve-fluid-demo -o jsonpath='{.status.url}' | cut -d "/" -f 3)
export INGRESS_HOST=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].port}')

curl -v -H "Content-Type: application/json" -H "Host: ${SERVICE_HOSTNAME}" "http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/bloom:predict" -d '{"prompt": "It was a dark and stormy night", "result_length": 50}'
```

Output:

```
❯ curl -v -H "Content-Type: application/json" -H "Host: ${SERVICE_HOSTNAME}" "http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/bloom:predict" -d '{"prompt": "It was a dark and stormy night", "result_length": 50}'
*   Trying 52.29.43.253:80...
* Connected to a38c4xxxxxxxxxxxxxxx717177.eu-central-1.elb.amazonaws.com (52.29.43.253) port 80 (#0)
> POST /v1/models/bloom:predict HTTP/1.1
> Host: fluid-bloom.kserve-fluid-demo.a38c4xxxxxxxxxxxxxxx717177.eu-central-1.elb.amazonaws.com
> User-Agent: curl/7.86.0
> Accept: */*
> Content-Type: application/json
> Content-Length: 65
>
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< content-length: 190
< content-type: application/json
< date: Wed, 12 Apr 2023 15:46:16 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 52071
<
{
  "result": "It was a dark and stormy night, and the wind blew with a\nfearful howl. The moon was obscured by a thick cloud, and the stars\nshone dimly. The wind blew the snow from the"
}
* Connection #0 to host a38c4xxxxxxxxxxxxxxx717177.eu-central-1.elb.amazonaws.com left intact
```

### Performance Benchmark

To conduct a performance benchmark, we will compare the scaling time of the inference service with Storage initializer and Fluid across different model artifacts, measuring the time it takes for the service to scale from `0 to 1`.

We will use the following [machine types](https://aws.amazon.com/ec2/instance-types/m5/) for our tests:

* Data: m5.xlarge (gp3 volume)
* Workload: m5.2xlarge, m5.4xlarge

We will use the following Fluid runtimes for our tests:

* JindoFS

For the purposes of our benchmark, we will assume that nodes are readily available and that images are cached in the nodes, so we will not be factoring in node provisioning time or image pulling time.

Other assumptions:

* S3 bucket and Cluster are in the same region (eu-central-1)
* Data are preloaded in workers


Command to measure the total time:

```sh
# Total time includes: pod initialization and running + download model (storage initializer) + load model + inference + network
curl --connect-timeout 3600 --max-time 3600 -o /dev/null -s -w 'Total: %{time_total}s\n' -H "Content-Type: application/json" -H "Host: ${SERVICE_HOSTNAME}" "http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/bloom:predict" -d '{"prompt": "It was a dark and stormy night", "result_length": 50}'
# Total time: 53.037210s, status code: 200
```

| Model Name                                                                | Model Size (Snapshot) | Machine Type | KServe + Storage Initializer                         | KServe + Fluid(JindoFS)                    |
|---------------------------------------------------------------------------|-----------------------|--------------|------------------------------------------------------|--------------------------------------------|
| [ bigscience/bloom-560m ]( https://huggingface.co/bigscience/bloom-560m ) | 3.14GB                | m5.2xlarge   | total: 52.725s (download: 25.091s, load: 3.549s)     | total: 23.286s (load: 4.763s) (2 workers)  |
| [ bigscience/bloom-7b1 ]( https://huggingface.co/bigscience/bloom-7b1 )   | 26.35GB               | m5.4xlarge   | total: 365.479s (download: 219.844s, load: 102.299s) | total: 53.037s (load: 24.137s) (3 workers) |

> NOTE: `total` is the response time of sending an inference request to a serverless inference service (scale from 0), which includes pod startup, `model download`, `model load`, model inference and network time.
> TBD: add testing results for other runtimes

As can be observed, Fluid has demonstrated notable improvements in the autoscaling performance of KServe on LLM.

### Limitations

One potential limitation of this integration is the use of an additional Fuse-sidecar, which may lead to increased resource consumption. However, in `the use case of LLMs`, where the instance size is typically very large and the pod consumes the entire instance, the resource usage of the Fuse-sidecar is expected to be negligible. Nonetheless, it is important to carefully consider the resource impact of the Fuse-sidecar in your specific use case to ensure that it remains within acceptable limits.

### Clean Up

```sh
kubectl delete -f fluid-isvc.yaml -n kserve-fluid-demo
kubectl delete -f kserve-isvc.yaml -n kserve-fluid-demo

kubectl delete -f dataload.yaml -n kserve-fluid-demo
kubectl delete -f jindo.yaml -n kserve-fluid-demo

kubectl delete -f s3creds.yaml -n kserve-fluid-demo

# uninstall Fluid
helm delete fluid -n fluid-system
kubectl get crd | grep data.fluid.io | awk '{ print $1 }' | xargs kubectl delete crd
kubectl delete ns fluid-system
```

## Reference
* [KNative with Fluid](https://github.com/fluid-cloudnative/fluid/blob/master/docs/en/samples/knative.md)
* [JindoFS](https://www.alibabacloud.com/blog/introducing-jindofs-a-high-performance-data-lake-storage-solution_595600)
* [KServe with S3](https://kserve.github.io/website/0.8/modelserving/storage/s3/s3/)
* [BLOOM](https://huggingface.co/docs/transformers/model_doc/bloom)