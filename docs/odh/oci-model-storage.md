# Using OCI containers for model storage

Starting from ODH 2.18, the ability to use OCI containers as storage for models
is enabled in KServe by default. The benefits of using OCI containers for
model storage are described in the upstream KServe project
[documentation](https://kserve.github.io/website/latest/modelserving/storage/oci/),
which also explains how to deploy models from OCI images. 

This page offers a guide similar to the upstream project documentation, but
focusing on the OpenDataHub and OpenShift characteristics. To demonstrate
how to create an OCI container image, the publicly available [MobileNet v2-7
model](https://github.com/onnx/models/tree/main/validated/vision/classification/mobilenet)
is used. This model is in ONNX format.

The ODH projects provides configurations for the OpenVINO model server, which
supports models in ONNX format. Thus, this guide will use this model server
to demonstrate how deploy the MobileNet v2-7 model stored in an OCI image.

## Storing a model in an OCI image

Start by creating an empty directory for downloading the model and creating
the necessary support files to create the OCI image. You may use a temporary
directory by running the following command:
```shell
cd $(mktemp -d)
```

OpenVINO expects a specific directory tree for model versioning.
Starting from some base directory, its contents should be a collection of
numbered subdirectories using positive integer values. The numbers would
represent the versions of the model. When using OCI container images, this
structure may be irrelevant, as you can use the OCI container registry
features. However, since OpenVINO expects the versioned directory structure, a
single subdirectory with an artificial version `1` can be used. Using `models/` as the
base path, create the expected directory structure and download the sample
model into it:
```shell
mkdir -p models/1

DOWNLOAD_URL=https://github.com/onnx/models/raw/main/validated/vision/classification/mobilenet/model/mobilenetv2-7.onnx
curl -L $DOWNLOAD_URL -O --output-dir models/1/
```

> [!TIP]
> If you are planning to use a different model server, you should adapt this
> guide accordingly to your model server requirements. Typically, you would need
> to place your model files directly under `models/`.

Create a file named `Containerfile` with the following contents:
```Dockerfile
FROM registry.access.redhat.com/ubi8/ubi-micro:latest
COPY --chown=0:0 models /models
RUN chmod -R a=rX /models

# nobody user
USER 65534 
```

Notice that model files are copied into `/models` inside the container. KServe
expects this path to exist in the OCI image and also expects the model files to
be inside it.

Also, notice that `ubi8-micro` is used as a base container image. Empty images, like
`scratch` cannot be used, because KServe needs to configure the model image
with a command to keep it alive and ensure the model files remain available in
the pod. Thus, it is required to use a base image that provides a shell.

Finally, notice that ownership of the copied model files is changed to the `root`
group, and also read permissions are granted. This is important, because OpenShift
runs containers with a random user ID and with the `root` group ID. The adjustment
of the group and the privileges on the model files ensures that the model server
can access them.

Verify that the directory structure is good using the `tree` command:
```shell
tree

.
├── Containerfile
└── models
    └── 1
        └── mobilenetv2-7.onnx
```

> [!NOTE]
> Remember that the shown directory structure under `models/` is specific to OpenVINO.

Create the OCI container image with Podman, and upload it to a registry. For
example, using Quay as the registry:
```shell
podman build --format=oci -t quay.io/<user_name>/<repository_name>:<tag_name> .
podman push quay.io/<user_name>/<repository_name>:<tag_name>
```

> [!TIP]
> When uploading your container image, if your repository is private, ensure you
> are authenticated to the registry.

## Deploying a model stored in an OCI image in a public repository

Start by creating a namespace to deploy the model:
```shell
oc new-project oci-model-example
```

In the newly created namespace, you need to create a `ServingRuntime` resource
configuring OpenVINO model server. The ODH project provides templates with
configurations for some model servers, which you can list with the following
command:
```shell
oc get template -n opendatahub

NAME                                 DESCRIPTION                                                                        PARAMETERS    OBJECTS
caikit-standalone-serving-template   Caikit is an AI toolkit that enables users to manage models through a set of...    0 (all set)   1
caikit-tgis-serving-template         Caikit is an AI toolkit that enables users to manage models through a set of...    0 (all set)   1
kserve-ovms                          OpenVino Model Serving Definition                                                  0 (all set)   1
ovms                                 OpenVino Model Serving Definition                                                  0 (all set)   1
tgis-grpc-serving-template           Text Generation Inference Server (TGIS) is a high performance inference engin...   0 (all set)   1
vllm-runtime-template                vLLM is a high-throughput and memory-efficient inference and serving engine f...   0 (all set)   1
```

The template that is applicable for KServe and holds the OpenVINO configuration
is the one named as `kserve-ovms`. To create an instance of it, run the
following command:
```shell
oc process -n opendatahub -o yaml kserve-ovms | oc apply -f -
```

You can verify that the `ServingRuntime` has been created successfully with the
following command:
```shell
oc get servingruntimes

NAME          DISABLED   MODELTYPE     CONTAINERS         AGE
kserve-ovms              openvino_ir   kserve-container   1m
```

Notice that the ServingRuntime has been created with `kserve-ovms` name.

Now that the `ServingRuntime` is configured, a model stored in an OCI image can
be deployed by creating an `InferenceService` resource:
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: sample-isvc-using-oci
spec:
  predictor:
    model:
      runtime: kserve-ovms # This is the name of the ServingRuntime resource
      modelFormat:
        name: onnx
      storageUri: oci://quay.io/<user_name>/<repository_name>:<tag_name>
```

> [!IMPORTANT]
> The resulting `ServingRuntime` and `InferenceService` configurations won't set
> any resource limits.

Once the `InferenceService` resource is created, KServe will deploy the model
stored in the OCI image referred by the `storageUri` field. Check the status
of the deployment with the following command:
```shell
oc get inferenceservice

NAME                    URL                                                       READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                     AGE
sample-isvc-using-oci   https://sample-isvc-using-oci-oci-model-example.example   True           100                              sample-isvc-using-oci-predictor-00001   1m
```

> [!IMPORTANT]
> Remember that, by default, models are exposed outside the cluster and not
> protected with authorization. Read the [authorization guide](authorization.md#deploying-a-protected-inferenceservice)
> and the [private services guide (TODO)](#TODO) to learn how to privately deploy
> models and how to protect them with authorization.

## Deploying a model stored in an OCI image from a private repository

To deploy a model stored in a private OCI repository you need to configure an
image pull secret. For detailed documentation, please consult the OpenShift
[documentation](https://docs.openshift.com/container-platform/latest/openshift_images/managing_images/using-image-pull-secrets.html)
for image pull secrets.

When using namespaced pull secrets you can create a pull secret using the following
command template:

```shell
oc create secret docker-registry <pull-secret-name> \
  --docker-server=<registry-server> \
  --docker-username=<username> \
  --docker-password=<password>
```

Once the pull secret is created, you can follow the steps from the previous
section for deploying a model with one small variant: when creating the
`InferenceService`, specify your pull secret in the
`spec.predictor.imagePullSecrets` field:
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: sample-isvc-using-private-oci
spec:
  predictor:
    model:
      runtime: kserve-ovms
      modelFormat:
        name: onnx
      storageUri: oci://quay.io/<user_name>/<repository_name>:<tag_name>
    imagePullSecrets: # Specify image pull secrets to use for fetching container images (including OCI model images)
    - name: <pull-secret-name>
```
