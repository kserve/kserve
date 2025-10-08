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

* [Fluid](https://github.com/fluid-cloudnative/fluid/blob/master/docs/en/userguide/get_started.md)
  * Deploy Fluid

    ```sh
    # Add Fluid repository to Helm repos and keep it up-to-date
    helm repo add fluid https://fluid-cloudnative.github.io/charts
    helm repo update
    # Deploy Fluid with Helm
    helm upgrade --install fluid --create-namespace --namespace=fluid-system fluid/fluid
    ```

## Prepare Demo Application

The demo application uses KServe to serve a LLM (Large Language Model). In this tutorial [Llama3.1-8B-Instruct](https://huggingface.co/meta-llama/Meta-Llama-3.1-8B-Instruct) model is used as an example.

### Prerequisite

Checkout KServe repo and go to the example:

```sh
git clone https://github.com/kserve/kserve.git
cd kserve/docs/samples/fluid
```

### Prepare Model Artifact

Download model from `HuggingFace`, `Llama3.1-8B-Instruct` model is using in this tutorial.

```sh
python -m venv .venv
source .venv/bin/activate
pip3 install --upgrade pip
pip3 install "huggingface_hub[hf_transfer]"

# create models folder
mkdir -p models

# to download Llama models, you need accept the Llama 3 community license agreement and set HF_TOKEN
export HF_TOKEN="xxxxxxxx"
HF_HUB_ENABLE_HF_TRANSFER=1 python3 download_model.py --model_name="meta-llama/Meta-Llama-3.1-8B-Instruct" --model_dir="./models"
# export output_dir=models/models--meta-llama--Meta-Llama-3.1-8B-Instruct/snapshots/main
```

Structure of the model artifact:
```sh
❯ tree ${output_dir}
models/models--meta-llama--Meta-Llama-3.1-8B-Instruct/snapshots/main
├── config.json
├── generation_config.json
├── model-00001-of-00004.safetensors
├── model-00002-of-00004.safetensors
├── model-00003-of-00004.safetensors
├── model-00004-of-00004.safetensors
├── model.safetensors.index.json
├── original
│   ├── params.json
│   └── tokenizer.model
├── special_tokens_map.json
├── tokenizer.json
└── tokenizer_config.json
```

Upload the model to S3 bucket:
```sh
# update the path accordingly
aws s3 cp --recursive ${output_dir} s3://${bucket}/models/meta-llama--Meta-Llama-3.1-8B-Instruct
```

## Demo

Let's start our demo.

### Create Demo Namespace

```sh
kubectl create ns kserve-fluid-demo
```

### Create S3 Credentials Secret And Service Account

```sh
# please update the s3 credentials to yours
$ kubectl create -f s3creds.yaml -n kserve-fluid-demo
```

### Create Dataset And Runtime

Create Dataset and Runtime (In this tutorial you will use `JindoFS` as the runtime, there are other runtimes like [JuiceFS](https://github.com/juicedata/juicefs) and [Alluxio](https://github.com/Alluxio/alluxio) etc).

```sh
# please update the `mountPoint` and `options`
$ kubectl create -f jindo.yaml -n kserve-fluid-demo

dataset.data.fluid.io/s3-data created
jindoruntime.data.fluid.io/s3-data created
```

Check the status:

```sh
$ kubectl get po -n kserve-fluid-demo
NAME                       READY   STATUS    RESTARTS   AGE
s3-data-jindofs-master-0   1/1     Running   0          5m18s
s3-data-jindofs-worker-0   1/1     Running   0          3m53s
s3-data-jindofs-worker-1   1/1     Running   0          3m52s
s3-data-jindofs-worker-2   1/1     Running   0          3m52s


$ kubectl get jindoruntime,dataset -n kserve-fluid-demo
NAME                                 MASTER PHASE   WORKER PHASE   FUSE PHASE   AGE
jindoruntime.data.fluid.io/s3-data   Ready          Ready          Ready        9m17s

NAME                            UFS TOTAL SIZE   CACHED     CACHE CAPACITY   CACHED PERCENTAGE   PHASE   AGE
dataset.data.fluid.io/s3-data   14.97GiB         12.11MiB   300.00GiB        0.1%                Bound   9m19s
```

A PVC is created to mount the dataset into the application:

```sh
$ kubectl get pvc -n kserve-fluid-demo
NAME      STATUS   VOLUME                      CAPACITY   ACCESS MODES   STORAGECLASS   VOLUMEATTRIBUTESCLASS   AGE
s3-data   Bound    kserve-fluid-demo-s3-data   100Pi      ROX            fluid          <unset>                 9m57s
```

### Preload The Data

Preload the data into workers to improve the performance:

```sh
# please update the `path` under `target`
$ kubectl create -f dataload.yaml -n kserve-fluid-demo
```

Check the dataload status:

```sh
$ kubectl get dataload -n kserve-fluid-demo
NAME          DATASET   PHASE      AGE    DURATION
s3-dataload   s3-data   Complete   6m1s   2m51s

$ kubectl get dataset -n kserve-fluid-demo
NAME      UFS TOTAL SIZE   CACHED     CACHE CAPACITY   CACHED PERCENTAGE   PHASE   AGE
s3-data   14.97GiB         44.92GiB   300.00GiB        100.0%              Bound   12m
```

Data are cached successfully.

### Deploy Cluster Serving Runtime

From my testing, the safetensors weights are loaded randomly which causing low performance when reading from remote cache, so I modify the huggingfaceserver servingruntime to load the safetensors weights sequentially to warm up the cache before running the application server.

```sh
# deploy the custom huggingfaceserver servingruntime
$ kubectl create -f kserve-huggingfaceserver.yaml
```

### Deploy The Inference Service

Create the InferenceService with Fluid:

```sh
# please update the `storageUri`.
$ kubectl create -f fluid-isvc.yaml -n kserve-fluid-demo

# run this command if you want to use the KServe Storage Initializer to download from cloud storage, update the `storageUri` as well. Please use the original huggerface server servingruntime.
$ kubectl create -f kserve-isvc.yaml -n kserve-fluid-demo
```

### Test The Demo Application

Follow this [guide](https://kserve.github.io/website/0.13/get_started/first_isvc/#4-determine-the-ingress-ip-and-ports) to get the ingress host and port:

```sh
export MODEL_NAME="llama-31-8b-instruct"
export SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -n kserve-fluid-demo -o jsonpath='{.status.url}' | cut -d "/" -f 3)
export INGRESS_HOST=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].port}')

$ curl "http://${INGRESS_HOST}:${INGRESS_PORT}/openai/v1/completions" \
-H "content-type: application/json" \
-H "Host: ${SERVICE_HOSTNAME}" \
-d '{"model": "'"$MODEL_NAME"'", "prompt": "Write a poem about colors", "stream":false, "max_tokens": 30}'
```

Output:

```sh
$ curl "http://${INGRESS_HOST}:${INGRESS_PORT}/openai/v1/completions" \
-H "content-type: application/json" \
-H "Host: ${SERVICE_HOSTNAME}" \
-d '{"model": "'"$MODEL_NAME"'", "prompt": "Write a poem about colors", "stream":false, "max_tokens": 30}'
{"id":"4e170959-8112-4a68-9415-4cb10239a2a5","choices":[{"finish_reason":"length","index":0,"logprobs":null,"text":". Many different things in life can be described by colors. Colors evoke emotions and memories. Use sensory details to bring your poem to life.\nSat in"}],"created":1724823045,"model":"llama-31-8b-instruct","system_fingerprint":null,"object":"text_completion","usage":{"completion_tokens":30,"prompt_tokens":6,"total_tokens":36}}
```

### Performance Benchmark

To conduct a performance benchmark, we will compare the scaling time of the inference service with Storage initializer and Fluid across different model artifacts, measuring the time it takes for the service to scale from `0 to 1`.

We will use the following [machine types](https://aws.amazon.com/ec2/instance-types/g5/) for our tests:

* Data: m5n.xlarge (EBS GP3)
* Workload: g5.8xlarge (NVIDIA A10G)

We will use the following Fluid runtimes for our tests:

* JindoFS

For the purposes of our benchmark, we will assume that nodes are readily available and that images are cached in the nodes, so we will not be factoring in node provisioning time or image pulling time.

Other assumptions:

* S3 bucket and Cluster are in the same region (eu-central-1)
* Data are preloaded in workers
* fuse.cleanPolicy: OnDemand


Command to measure the total time:

```sh
# Total time includes: pod initialization and running + download model (storage initializer) + load model + inference + network
$ curl --connect-timeout 3600 --max-time 3600 -o /dev/null -s -w 'Total: %{time_total}s\n' -H "Content-Type: application/json" -H "Host: ${SERVICE_HOSTNAME}" "http://${INGRESS_HOST}:${INGRESS_PORT}/openai/v1/completions" -d '{"model": "'"$MODEL_NAME"'", "prompt": "Write a poem about colors", "stream":false, "max_tokens": 30}'
# Total time: 53.037210s, status code: 200
```

| Model Name                                                                | Model Size (Snapshot) | Machine Type | KServe + Storage Initializer                         | KServe + Fluid(JindoFS)                    |
|---------------------------------------------------------------------------|-----------------------|--------------|------------------------------------------------------|--------------------------------------------|
| [ meta-llama/Meta-Llama-3.1-8B-Instruct ]( https://huggingface.co/meta-llama/Meta-Llama-3.1-8B-Instruct )   | 15GB               | g5.8xlarge   | total: 265.253s | total: 50.111s |

> NOTE: `total` is the response time of sending an inference request to a serverless inference service (scale from 0), which includes pod startup, model download, model load, model inference and network time.
> The above results are for reference only and may vary depending on the environment and the specific use case.

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
* [KServe with S3](https://kserve.github.io/website/0.13/modelserving/storage/s3/s3/)
* [Meta Llama](https://huggingface.co/meta-llama)