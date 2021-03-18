# Deploy to KFserving from [Kubeflow Pipelines](https://www.kubeflow.org/docs/pipelines/overview/pipelines-overview/)

These examples illustrate how to use the Kubeflow Pipelines component for KFServing.

* Deploy a [custom model](./sample-custom-model.py).
* Deploy a [Tensorflow model](./sample-tf-pipeline.py). There is also [a notebook](./sample-tf-pipeline.py) which illustrates this.
* Deploy a sample [MNIST model end to end using Kubeflow Pipelines with Tekton](https://github.com/kubeflow/kfp-tekton/tree/master/samples/e2e-mnist). The [notebook](https://github.com/kubeflow/kfp-tekton/blob/master/samples/e2e-mnist/mnist.ipynb) demonstrates how to compile and execute an End to End Machine Learning workflow that uses Katib, TFJob, KFServing, and Tekton pipeline. This pipeline contains 5 steps, it finds the best hyperparameter using Katib, creates PVC for storing models, processes the hyperparameter results, distributedly trains the model on TFJob with the best hyperparameter using more iterations, and finally serves the model using KFServing. You can visit this medium blog for more details on this pipeline.

![kfserving-mnist-pipeline](images/kfserving-mnist-pipeline.png)

Additional usage instructions can be found in the component [README](https://github.com/kubeflow/pipelines/blob/master/components/kubeflow/kfserving/README.md).
To dive into the source behind the KFServing Kubeflow Pipelines Component, please look into the YAML for the [KFServing Component](https://github.com/kubeflow/pipelines/blob/master/components/kubeflow/kfserving/component.yaml) and the [source code](https://github.com/kubeflow/pipelines/blob/master/components/kubeflow/kfserving/src/kfservingdeployer.py).
