
# Readme for Generating KFServing SDK

The guide shows how to generate the openapi model and swagger.json file from kersering types using `openapi-gen` and generate KFServing python sdk for the python object models using `swagger-codegen`.

### Install openapi-gen

```
cd ./vendor/k8s.io/code-generator/cmd/openapi-gen
go install -v
```

### Generate the openapi model from KFServing types

Update the following kative files to add `+k8s:openapi-gen=true` (This should be updated from knative).
```
vendor/github.com/knative/pkg/apis/condition_types.go
vendor/github.com/knative/pkg/apis/duck/v1beta1/status_types.go
vendor/github.com/knative/pkg/apis/volatile_time.go
```

Executing below to generate the openapi model:

```
openapi-gen --input-dirs github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1,github.com/knative/pkg/apis/duck/v1beta1,github.com/knative/pkg/apis,k8s.io/api/core/v1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/runtime,k8s.io/apimachinery/pkg/util/intstr,k8s.io/apimachinery/pkg/version  --output-package github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1/ --go-header-file hack/boilerplate.go.txt
```
After the command, you will see the `pkg/apis/serving/v1alpha1/openapi_generated.go` has been generated.

### Generate swagger file

```
go run hack/sdk-gen/main.go 0.1 > hack/sdk-gen/swagger.json
```
Note the version need to be supplied for the `main.go`, need to update the release version for future release. After the executing, the swagger generated to `hack/sdk-gen/swagger.json`.

### Install swagger-codegen

Following the [guide](https://github.com/swagger-api/swagger-codegen#getting-started) to install swagger-codegen, the following step assumps swagger-codegen has been installed under `hack` directory.

### Generate Python SDK from the swagger file

```
java -jar hack/swagger-codegen/modules/swagger-codegen-cli/target/swagger-codegen-cli.jar generate -i hack/sdk-gen/swagger.json -l python -o sdk -c hack/sdk-gen/swagger_config.json
```

After excuting, the kfserving python SDK is generated in the `sdk` directory.
