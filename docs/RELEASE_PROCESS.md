# Release Process
KFServing's automated release processes run in Google Cloud Build under the project `kfserving`. For additional permissions to this project, please contact ellisbigelow@google.com.

Builds are available at https://console.cloud.google.com/cloud-build/builds?project=kfserving.
To view builds, you must be a member of kubeflow-discuss@googlegroups.com.

## Latest Release
This release process relies on Google Cloud build Triggers. This build triggers are configured manually to execute builds for the following:

1. github.com/kubeflow/kfserving/Dockerfile -> `gcr.io/kfserving/kfserving-controller:latest`
2. github.com/kubeflow/kfserving/python/sklearn.Dockerfile -> `gcr.io/kfserving/sklearnserver:latest`
3. github.com/kubeflow/kfserving/python/xgboost.Dockerfile -> `gcr.io/kfserving/xgbserver:latest`

## Versioned Releases
This release process relies on Github release tags and Google Cloud Build Triggers. The built triggers are configured manually to execute builds for 
the following:

1. github.com/kubeflow/kfserving/Dockerfile -> `gcr.io/kfserving/kfserving-controller:$TAG`

Versioning for frameworks must be decoupled from KFServing Version. All framework images will be rebuilt regularly. Newer versions of KFServing may cease to include the latest changes of the KFServer for old versions of Frameworks.
