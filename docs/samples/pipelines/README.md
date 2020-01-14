# Deploy to KFserving from [Kubeflow Pipelines](https://www.kubeflow.org/docs/pipelines/overview/pipelines-overview/)

These two examples illustrate how to use Kubeflow Pipeline component for KFServing. The first one deploys a [custom  model](https://github.com/kubeflow/kfserving/blob/master/docs/samples/pipelines/sample-custom-model.py) and the second one deploys a [tensorflow model](https://github.com/kubeflow/kfserving/blob/master/docs/samples/pipelines/sample-tf-pipeline.py) from a Kubeflow Pipeline to KFServing. 

There is also [a notebook](https://github.com/kubeflow/kfserving/blob/master/docs/samples/pipelines/sample-tf-pipeline.py) which illustrates this. 

To dive into the source behind the KFServing component, please look into the yaml for [KFServing Component](https://github.com/kubeflow/pipelines/blob/master/components/kubeflow/kfserving/component.yaml) and the [source code](https://github.com/kubeflow/pipelines/blob/master/components/kubeflow/kfserving/src/kfservingdeployer.py)
