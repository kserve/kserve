# PyTorch Server

[PyTorch](https://PyTorch.org) server is an implementation of KFServing for serving PyTorch models, and provides a PyTorch model implementation for prediction, pre and post processing. 

To start the server locally for development needs, run the following command under this folder in your github repository. 

```
pip install -e .
```

The following output indicates a successful install.

```

```

Once PyTorch server is up and running, you can check for successful installation by running the following command

```
python3 -m pytorchserver
python3 -m pytorchserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   [--protocol {tensorflow.http,seldon.http}] --model_dir
                   MODEL_DIR [--model_name MODEL_NAME]
                   [--model_class_name MODEL_CLASS_NAME]
                   [--model_class_file MODEL_CLASS_FILE]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your `pytorch` model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage. Please follow [this sample](https://github.com/kubeflow/kfserving/tree/master/docs/samples/pytorch) to test your server by generating your own model. 

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```

```

To run tests, please change the test file to point to your model.joblib file. Then run the following command:

```bash
make test
```

The following shows the type of output you should see:

```

```

To run static type checks:

```bash
mypy --ignore-missing-imports pytorchserver
```
An empty result will indicate success.

## Building your own PyTorch erver Docker Image

You can build and publish your own image for development needs. Please ensure that you modify the kfservice files for PyTorch in the api directory to point to your own image.

To build your own image, run

```bash
docker build -t animeshsingh/pytorchserver -f pytorch.Dockerfile .
```

You should see an output similar to this

```bash
p
```

To push your image to your dockerhub repo, 

```bash
docker push docker_user_name/pytorchserver:latest
```