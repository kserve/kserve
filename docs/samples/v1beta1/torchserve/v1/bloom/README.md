# TorchServe example with Huggingface BLOOM model
In this example we will show how to serve [Large Huggingface models with TorchServe](https://github.com/pytorch/serve/tree/master/examples/Huggingface_Largemodels)
on KServe.

## Model archive file creation

Clone [pytorch/serve](https://github.com/pytorch/serve) repository,
navigate to `examples/Huggingface_Largemodels` and follow the steps for creating the MAR file including serialized model and other dependent files.

The above Torchserve example works on shard version on Huggingface models.

The For sharding Huggingface models use the below script, and continue with [this step](https://github.com/pytorch/serve/tree/master/examples/Huggingface_Largemodels#step-2-compress-downloaded-model)

```python
from transformers import AutoModelForCausalLM, AutoTokenizer
model_name="bigscience/bloomz-7b1"
model = AutoModelForCausalLM.from_pretrained(model_name)
tokenizer = AutoTokenizer.from_pretrained(model_name)
model.save_pretrained("model/"+model_name, max_shard_size="5GB")
tokenizer.save_pretrained("model/"+model_name)
```

TorchServe supports both eager model and torchscript and here we save as the pretrained model. 
 
```bash
torch-model-archiver --model-name BLOOMSeqClassification --version 1.0 \
--serialized-file Transformer_model/pytorch_model.bin \
--handler ./Transformer_handler_generalized.py \
--extra-files "Transformer_model/config.json,./setup_config.json,./Seq_classification_artifacts/index_to_name.json"
```

## Create NVMe Persistent Volume

Use SSH to connect to the worker nodes and prepare the NVMe drives for Kubernetes, as follows.

Run the `lsblk`  command on each worker node to lists information about all available. 

```bash
NAME          MAJ:MIN RM   SIZE RO TYPE MOUNTPOINT
nvme1n1       259:0    0 116.4G  0 disk 
nvme0n1       259:1    0    80G  0 disk 
├─nvme0n1p1   259:2    0    80G  0 part /
└─nvme0n1p128 259:3    0     1M  0 part 
```

Output of the command line that shows the result of running the lsblk command.


```bash
$ sudo mkfs.xfs /dev/nvme1n1
```

```bash
$ sudo mkdir -p /mnt/data/vol1
$ sudo chmod -R 777 /mnt     
$ sudo mount /dev/nvme1n1 /mnt/data/vol1
```

Permanently mount the device:

```bash
$ sudo blkid /dev/nvme1n1
```

To get it to mount every time, add the following line to the /etc/fstab file:

```bash
UUID=nvme_UUID      /mnt/data/vol1   xfs    defaults,nofail    0    2
```

Clone the local provisioner repository:

```bash
$ git clone https://github.com/kubernetes-sigs/sig-storage-local-static-provisioner.git
```

Create a StorageClass 

storageclass.yaml
```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: fast-disks
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
```

```bash
$ kubectl apply -f storageclass.yaml
```

Create Local Persistent Volumes for Kubernetes

- Change `hostDir` to the mount path

```bash
cd sig-storage-local-static-provisioner
helm template ./helm/provisioner > ./provisioner/deployment/kubernetes/provisioner_generated.yaml

kubectl apply -f ./deployment/kubernetes/provisioner_generated.yaml
```

Output of the command line that shows the result of running the kubectl get pods command.

```bash
NAME                                         READY   STATUS    RESTARTS   AGE
kserve-controller-manager-5c5c4d8c89-lrzbd   2/2     Running   0          4d2h
local-nvme-pv-provisioner-vwxgt              1/1     Running   0          16m
```

```bash
$ kubectl get pv

NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS      CLAIM                     STORAGECLASS   REASON   AGE
local-pv-2a85b8ac                          116Gi      RWO            Delete           Bound       kserve-test/model-cache   fast-disks              4d3h
```
## Create PVC and Mount the Model.

```bash
kubectl apply -f pvc-pod.yaml
```

Refer: [Bloom Model Example](https://github.com/pytorch/serve/tree/master/examples/Huggingface_Largemodels)

Move the config.properties and MAR file to PVC in the below structure.

```bash
|_model-store
  |_bloom-560m.mar
|_config
  |_config.properties
```

```bash
kubectl exec -it pv-pod -- mkdir /pv/config
kubectl exec -it pv-pod -- mkdir /pv/model-store

kubectl cp config.properties pv-pod:/pv/config
kubectl cp bloom-560m.mar -it pv-pod:/pv/config
```

## Create the InferenceService

Apply the CRD

```bash
kubectl apply -f bloom-560m.yaml
```

Expected Output

```bash
$inferenceservice.serving.kserve.io/torchserve-bloom-560m created
```

## Run a prediction

The first step is to [determine the ingress IP and ports](https://kserve.github.io/website/0.10/get_started/first_isvc/#4-determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=BLOOMSeqClassification
ISVC_NAME=torchserve-bloom-560m
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${ISVC_NAME} -n <namespace> -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/${MODEL_NAME}:predict -d ./sample_text.txt
```

Expected Output

```bash
*   Trying 44.239.20.204...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (44.239.20.204) port 80 (#0)
> PUT /v1/models/BLOOMSeqClassification:predict HTTP/1.1
> Host: torchserve-bloom-560m.kserve-test.example.com
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
* Connection #0 to host torchserve-bloom-560m.kserve-test.example.com left intact
Accepted
```

**__Note__** For larger models use `A100 80g` GPU instances. 