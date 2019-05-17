# Release Process
KFServing's automated release processes run in Google Cloud Build under the project `kfserving`. For permissions to this project, please contact ellisbigelow@google.com.


## Latest Release
KFServing's release process relies on Github build triggers in Google Cloud Build. This build trigger was configured manually to execute the configurations in cloud-build-configs. For each build config, whenever a change is made to this repository, a Cloud Build is triggered which rebuilds all relevant images with a tag suffixed with `-head` and pushes them to `gcr.io/kfserving`. For example, `kfserving-controller.cloud-build.yaml` will result in an image called `gcr.io/kfserving/kfserving-controller-head`

## Versioned Releases
This process is currently TBD, but we will eventually provide releases of the form `gcr.io/kfserving/$IMAGE_NAME-$VERSION`

