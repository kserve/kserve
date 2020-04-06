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

# Releaser Guide
Sample PR: https://github.com/kubeflow/kfserving/pull/700

It's generally a good idea to search the repo for control-f for strings of the old version number and replace them with the new, keeping in mind conflicts with other library version numbers.

1. Update configmap to point to $VERSION for all images (e.g. https://github.com/kubeflow/kfserving/pull/700/files#diff-c8dbe0c491a054c35d4254b9dfc6a6dd).
2. Update python libraries to $VERSION.
3. Generate install manifest `./hack/generate-install.sh $VERSION`
4. Submit your PR and wait for it to merge.
5. Regenerate the SDK and refresh in Pypi: https://github.com/kubeflow/kfserving/blob/f471d0bd6e95e8556fa09fbb5bb8b22352592798/hack/python-sdk/README.md
6. Once everything has settled, tag and push the release with `git tag $VERSION` and `git push upstream --tags`.

