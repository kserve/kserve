# XGBoost Server

[XGBoost](https://xgboost.readthedocs.io/en/latest/index.html ) server is an implementation for serving XGBoost models, and provides an XGBoost model implementation for prediction, pre and post processing. In addition, model lifecycle management functionalities like liveness handler, metrics handler etc. are supported. 

To start the server locally for development needs, run the following command under this folder in your github repository. Also please ensure you have installed the [kserve](../kserve) before.

```
pip install -e .
```

Once XGBoost server is up and running, you can check for successful installation by running the following command

```
python3 -m xgbserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage.


## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

To run tests:

```bash
make test
```

To run static type checks:

```bash
mypy --ignore-missing-imports xgbserver
```
An empty result will indicate success.

## Building your own XGBoost Server Docker Image

You can build and publish your own image for development needs. Please ensure that you modify the inferenceservice files for XGBoost in the api directory to point to your own image.

To build your own image, navigate up one directory level to the `python` directory and run:

```bash
docker build -t docker_user_name/xgbserver -f xgb.Dockerfile .
```

Sometimes you may want to build the `XGBServer` image with a different version of `XGBoost`, you can modify the version `"xgboost == X.X.X"` in `setup.py` and build the image with
tag like `docker_user_name/xgbserver:1.1.0`.


To push your image to your dockerhub repo, 

```bash
docker push docker_user_name/xgbserver:latest
```
