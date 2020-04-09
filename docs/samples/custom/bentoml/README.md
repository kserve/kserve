# Predict on an InferenceService using BentoML

## Prerequisites

1. `~/.kube/config` should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Cluster's Istio Ingress gateway must be network accessible.
3. python 3.6 or above
4. Jupyter notebook

    ```shell
    pip install jupyterlab
    ```

[BentoML](https://bentoml.org) is an open-source framework for high-performance ML model serving.

This example will build a classifier model using iris dataset with BentoML, build and
push docker image to Docker Hub, and then deploy it to a cluster with KFServing installed.

Start Jupyter and open the notebook

  ```shell
  jupyter notebook bentoml_custom.ipynb
  ```

Follow the instructions in the notebook to deploy the InferenceService with BentoML.
