
# Readme for Generating KFServing SDK

The guide shows how to generate the openapi model and swagger.json file from kersering types using `openapi-gen` and generate KFServing python sdk for the python object models using `swagger-codegen`.

## Generate openapi spec and swagger file.

From KFServing root folder, you can `make generate` or execute the below script directly to generate openapi spec and swagger file.

```
./hack/update-openapigen.sh
```
After executing, the `openapi_generated.go` and `swagger.json` are generated and stored under `pkg/apis/serving/v1alpha1/`.

## Generate KFServing Python SDK

From KFSering root folder, execute the script `hack/sdk-gen/sdk-gen.sh` to install swagger-codegen and generate KFServing Python SDK, or you can install customized swagger-codegen and generate SDK manually following the [guide](https://github.com/swagger-api/swagger-codegen#getting-started) of swagger-codegen.

```
./hack/sdk-gen/sdk-gen.sh
```
After the script execution, the kfserving Python SDK is generated in the `sdk` directory.
