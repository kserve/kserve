# PyTorch Server

[PyTorch](https://PyTorch.org) server is an implementation for serving PyTorch models, and provides a PyTorch model implementation for prediction, pre and post processing.

To start the server locally for development needs, run the following command under this folder in your github repository.

```
pip install -e .
```

Once PyTorch server is up and running, you can check for successful installation by running the following command

```
python3 -m pytorchserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   [--protocol {tensorflow.http,seldon.http}] --model_dir
                   MODEL_DIR [--model_name MODEL_NAME]
                   [--model_class_name MODEL_CLASS_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your `pytorch` model directory and use the server to load the model and test for prediction. Model and associated model class file can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. Please follow [this sample](https://github.com/kserve/kserve/tree/master/docs/samples/v1beta1/torchserve) to test your server by generating your own model. 

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

To run tests, please change the test file to point to your model.pt file. Then run the following command:

```bash
make test
```

To run static type checks:

```bash
mypy --ignore-missing-imports pytorchserver
```

An empty result will indicate success.

## Building your own PyTorch server Docker Image

You can build and publish your own image for development needs. Please ensure that you modify the inferenceservice files for PyTorch in the api directory to point to your own image.

To build your own image, navigate up one directory level to the `python` directory and run:

```bash
docker build -t docker_user_name/pytorchserver -f pytorch.Dockerfile .
```

You should see an output with an ending similar to this

```bash
Installing collected packages: torch, pytorchserver
  Found existing installation: torch 1.0.0
    Uninstalling torch-1.0.0:
      Successfully uninstalled torch-1.0.0
  Running setup.py develop for pytorchserver
Successfully installed pytorchserver torch-1.1.0
Removing intermediate container 9f6cb904ec57
 ---> 1272c4674955
Step 11/11 : ENTRYPOINT ["python", "-m", "pytorchserver"]
 ---> Running in 6bbbdda829ec
Removing intermediate container 6bbbdda829ec
 ---> c5ac6833fdfe
Successfully built c5ac6833fdfe
Successfully tagged animeshsingh/pytorchserver:latest
```

To push your image to your dockerhub repo,

```bash
docker push docker_user_name/pytorchserver:latest
```
