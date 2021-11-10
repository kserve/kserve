
# Readme for Generating KServe SDK

The guide shows how to generate the openapi model and swagger.json file from KFServing types using `openapi-gen` and generate KFServing Python SDK Client for the Python object models using `swagger-codegen`. Also show how to upload the KFServing SDK to Pypi.

## Generate openapi spec and swagger file.

From root folder, you can `make generate` or execute the below script directly to generate openapi spec and swagger file.

```
./hack/update-openapigen.sh
```
After executing, the `openapi_generated.go` and `swagger.json` are generated and stored under `pkg/apis/serving/v1beta1/`.

## Generate Python SDK

From root folder, execute the script `/hack/python-sdk/client-gen.sh` to install swagger-codegen and generate KFServing Python SDK, or you can install customized swagger-codegen and generate SDK manually following the [guide](https://github.com/swagger-api/swagger-codegen#getting-started) of swagger-codegen.

```
./hack/python-sdk/client-gen.sh
```
After the script execution, the Python SDK is generated in the `python/kserve` directory. Some files such as [README](../../python/kserve/README.md) and documents need to be merged manually after the script execution.

## (Optional) Refresh Python SDK in the Pypi

Navigate to `python/kserve` directory from the root folder.

1. Install `twine`:

   ```bash
   pip install twine
   ```

2. Update the Python SDK version in the [setup.py](../../python/kserve/setup.py).

3. Create some distributions in the normal way:

    ```bash
    python setup.py sdist bdist_wheel
    ```

4. Upload with twine to [Test PyPI](https://packaging.python.org/guides/using-testpypi/) and verify things look right. `Twine` will automatically prompt for your username and password:
    ```bash
    twine upload --repository-url https://test.pypi.org/legacy/ dist/*
    username: ...
    password:
    ...
    ```

5. Upload to [PyPI](https://pypi.org/search/?q=kserve):
    ```bash
    twine upload dist/*
    ```
