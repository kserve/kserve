# Lightgbm Server

[LightGBM](https://lightgbm.readthedocs.io/en/latest/index.html ) server is an implementation for serving LightGBM models, and provides an LightGBM model implementation for prediction, pre and post processing. In addition, model lifecycle management functionalities like liveness handler, metrics handler etc. are supported. 

To start the server locally for development needs, run the following command under this folder in your github repository. Also please ensure you have installed the [kserve](../kserve) before.

```
pip install -e .
```

Once LightGBM server is up and running, you can check for successful installation by running the following command

```
python3 -m lgbserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. Please follow [this sample](https://github.com/kserve/kserve/tree/master/docs/samples/v1beta1/lightgbm) to test your server by generating your own model. 


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
mypy --ignore-missing-imports lgbserver
```
An empty result will indicate success.

## Building your own LightGBM Server Docker Image

You can build and publish your own image for development needs. Please ensure that you modify the inferenceservice files for LightGBM in the api directory to point to your own image.

To build your own image, navigate up one directory level to the `python` directory and run:

```bash
docker build -t docker_user_name/lgbserver -f lgb.Dockerfile .
```

Sometimes you may want to build the `LGBServer` image with a different version of `LightGBM`, you can modify the version `"lightgbm == X.X.X"` in `setup.py` and build the image with
tag like `docker_user_name/lgbserver:1.1.0`.

To push your image to your dockerhub repo, 

```bash
docker push docker_user_name/lgbserver:latest
```
