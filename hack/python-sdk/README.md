
# Readme for Generating KServe SDK

The guide shows how to generate the openapi model and swagger.json file from KServe types using `openapi-gen` and generate Python SDK Client for the Python object models using `openapi-codegen`. Also show how to upload the Python SDK to Pypi.

## Generate openapi spec and swagger file.

From root folder, you can `make generate` or execute the below script directly to generate openapi spec and swagger file.

```
./hack/update-openapigen.sh
```
After executing, the `openapi_generated.go` and `swagger.json` are generated and stored under `pkg/apis/serving/v1beta1/`.

## Generate Python SDK

From root folder, execute the script `/hack/python-sdk/client-gen.sh` to install openapi-codegen and generate Python SDK.

```
./hack/python-sdk/client-gen.sh
```
After the script execution, the Python SDK is generated in the `python/kserve` directory. Some files such as [README](../../python/kserve/README.md) and documents need to be merged manually after the script execution.

## (Optional) Refresh Python SDK in the Pypi

Navigate to `python/kserve` directory from the root folder.

1. Install `poetry`:

   ```bash
   pip install poetry
   ```

2. Update the Python SDK version in the [pyproject.toml](../../python/kserve/pyproject.toml).

3. Create some distributions in the normal way:

    ```bash
    poetry build 
    ```
4. Configure Test PyPI [Credentials in Poetry](https://python-poetry.org/docs/repositories/#configuring-credentials):
   ```bash
   poetry config repositories.test-pypi https://test.pypi.org/legacy/
   ```
   ```bash
   poetry config http-basic.test-pypi <username> <password>
   ```
5. Upload with poetry to [Test PyPI](https://packaging.python.org/guides/using-testpypi/) and verify things look right:
    ```bash
    poetry publish -r test-pypi
    ```
6. Configure PyPI [Credentials in Poetry](https://python-poetry.org/docs/repositories/#configuring-credentials):
   ```bash
   poetry config http-basic.pypi <username> <password>
   ```
7. Upload to [PyPI](https://pypi.org/search/?q=kserve):
    ```bash
    poetry publish
    ```
