# GPU Prerequisites
## Install Nvidia Device Drivers
Nvidia drivers must be installed for accelerators to be scheduled to any given node.

Google provides a DaemonSet that automatically installs the drivers for you.
```
kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/521205d4de2a4a4eaf7259b27bc2290c823d857c/nvidia-driver-installer/cos/daemonset-preloaded.yaml
```

## GPU Node Taints
### Azure
Azure does not use node taints for GPU enabled nodes.

### GKE
When you add a GPU node pool to an existing cluster that already runs a non-GPU node pool, GKE automatically taints the GPU nodes with the following node taint:
```
Key: nvidia.com/gpu
Effect: NoSchedule
```
GKE will automatically apply corresponding tolerations to Pods requesting GPUs by running the [ExtendedResourceToleration](https://kubernetes.io/docs/tasks/administer-cluster/extended-resource-node/) admission controller.

### On Prem
These details will be determined by your individual kubernetes installation.

## Specifying GPU type
Some kubernetes providers allow users to run multiple GPU types on the same cluster. These GPUs may be selected by providing an annotation on your InferenceService resource.

### GKE
Apply the GKE Accelerator annotation as follows:
```
metadata:
  annotations:
    "serving.kubeflow.org/gke-accelerator": "nvidia-tesla-k80"
```
The list of types is available at https://cloud.google.com/kubernetes-engine/docs/how-to/gpus#multiple_gpus.
