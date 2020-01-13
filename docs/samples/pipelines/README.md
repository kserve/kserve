# Deploy to KFserving from [Kubeflow Pipelines](https://www.kubeflow.org/docs/pipelines/overview/pipelines-overview/)

These two example illustrated how to deploy two models, [one custom](https://github.com/wanghualei/kfserving/blob/4f683f7dbf08d56d81a6404155dc89fb1b1df31c/docs/samples/pipelines/sample-custom-model.py) and [one tensorflow](https://github.com/wanghualei/kfserving/blob/4f683f7dbf08d56d81a6404155dc89fb1b1df31c/docs/samples/pipelines/sample-tf-pipeline.py) from a kubeflow pipeline to kfserving. 

There is also [a notebook](https://github.com/wanghualei/kfserving/blob/4f683f7dbf08d56d81a6404155dc89fb1b1df31c/docs/samples/pipelines/kfs-pipeline.ipynb) to illustrate this. 

It should be noted that this dont allow you to deploy a complete pipeline to kubeflow serving but rather allow you to deploy to kfserving from a pipeline. This is done using the [kfserving component](https://github.com/kubeflow/pipelines/blob/master/components/kubeflow/kfserving/component.yaml) which is using the [kfservingdeployer](https://github.com/kubeflow/pipelines/blob/master/components/kubeflow/kfserving/src/kfservingdeployer.py)
