# Scikit-Learn Server

[Scikit-Learn](https://scikit-learn.org/stable/) server is an implementation for serving Scikit-learn models, and provides a Scikit-learn model implementation for prediction, pre and post processing.

To start the server locally for development needs, run the following command under this folder in your github repository.

```
pip install -e .
```

Once Scikit-learn server is up and running, you can check for successful installation by running the following command

```
python3 -m sklearnserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your `joblib` or `pkl` model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. If both joblib and pickle formats are presented, joblib model will get loaded. Please follow [this sample](https://github.com/kserve/kserve/tree/master/docs/samples/v1beta1/sklearn/v1) to test your server by generating your own model.

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

To run tests, please change the test file to point to your model.joblib file. Then run the following command:

```bash
make test
```

To run static type checks:

```bash
mypy --ignore-missing-imports sklearnserver
```
An empty result will indicate success.

## Building your own Scikit-Learn Server Docker Image

You can build and publish your own image for development needs. Please ensure that you modify the inferenceservice files for Scikit-Learn in the api directory to point to your own image.

To build your own image, navigate up one directory level to the `python` directory and run:

```bash
docker build -t docker_user_name/sklearnserver -f sklearn.Dockerfile .
```

Sometimes you may want to build the `SKLearnServer` image with a different version of `SKLearn`, you can modify the version `"scikit-learn == X.X.X"` in `setup.py` and build the image with
tag like `docker_user_name/sklearnserver:0.24`.

To push your image to your dockerhub repo,

```bash
docker push docker_user_name/sklearnserver:latest
```
