# PyTorch Server

[PyTorch](https://PyTorch.org) server is an implementation of KFServing for serving PyTorch models, and provides a PyTorch model implementation for prediction, pre and post processing.

To start the server locally for development needs, run the following command under this folder in your github repository.

```
pip install -e .
```

The following output indicates a successful install.

```
Obtaining file:///Users/animeshsingh/go/src/github.com/kubeflow/kfserving/python/pytorchserver
Requirement already satisfied: kfserving>=0.1.0 in /Users/animeshsingh/DevAdv/kfserving/python/kfserving (from pytorchserver==0.1.0) (0.1.0)
Requirement already satisfied: torch>=1.0.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (1.1.0)
Requirement already satisfied: argparse>=1.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (1.4.0)
Requirement already satisfied: numpy>=1.8.2 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (1.16.3)
Collecting torchvision>=0.2.2 (from pytorchserver==0.1.0)
  Downloading https://files.pythonhosted.org/packages/af/7c/247d46a1f76dee688636d4d5394e440bb32c4e251ea8afe4442c91296830/torchvision-0.3.0-cp37-cp37m-macosx_10_7_x86_64.whl (231kB)
Requirement already satisfied: tornado>=1.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserving>=0.1.0->pytorchserver==0.1.0) (6.0.2)
Requirement already satisfied: six in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from torchvision>=0.2.2->pytorchserver==0.1.0) (1.12.0)
Requirement already satisfied: pillow>=4.1.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from torchvision>=0.2.2->pytorchserver==0.1.0) (6.0.0)
Installing collected packages: torchvision, pytorchserver
  Found existing installation: pytorchserver 0.1.0
    Uninstalling pytorchserver-0.1.0:
      Successfully uninstalled pytorchserver-0.1.0
  Running setup.py develop for pytorchserver
Successfully installed pytorchserver torchvision-0.3.0
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

You can now point to your `pytorch` model directory and use the server to load the model and test for prediction. Model and associaed model class file can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. Please follow [this sample](https://github.com/kubeflow/kfserving/tree/master/docs/samples/pytorch) to test your server by generating your own model. 

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
Obtaining file:///Users/animeshsingh/go/src/github.com/kubeflow/kfserving/python/pytorchserver
Requirement already satisfied: kfserving>=0.1.0 in /Users/animeshsingh/DevAdv/kfserving/python/kfserving (from pytorchserver==0.1.0) (0.1.0)
Requirement already satisfied: torch>=1.0.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (1.1.0)
Requirement already satisfied: argparse>=1.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (1.4.0)
Requirement already satisfied: numpy>=1.8.2 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (1.16.3)
Requirement already satisfied: torchvision>=0.2.2 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (0.3.0)
Requirement already satisfied: pytest in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (4.5.0)
Requirement already satisfied: pytest-tornasync in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (0.6.0.post1)
Requirement already satisfied: mypy in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytorchserver==0.1.0) (0.701)
Requirement already satisfied: tornado>=1.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserving>=0.1.0->pytorchserver==0.1.0) (6.0.2)
Requirement already satisfied: pillow>=4.1.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from torchvision>=0.2.2->pytorchserver==0.1.0) (6.0.0)
Requirement already satisfied: six in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from torchvision>=0.2.2->pytorchserver==0.1.0) (1.12.0)
Requirement already satisfied: py>=1.5.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->pytorchserver==0.1.0) (1.8.0)
Requirement already satisfied: pluggy!=0.10,<1.0,>=0.9 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->pytorchserver==0.1.0) (0.11.0)
Requirement already satisfied: more-itertools>=4.0.0; python_version > "2.7" in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->pytorchserver==0.1.0) (7.0.0)
Requirement already satisfied: wcwidth in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->pytorchserver==0.1.0) (0.1.7)
Requirement already satisfied: atomicwrites>=1.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->pytorchserver==0.1.0) (1.3.0)
Requirement already satisfied: attrs>=17.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->pytorchserver==0.1.0) (19.1.0)
Requirement already satisfied: setuptools in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->pytorchserver==0.1.0) (40.8.0)
Requirement already satisfied: mypy-extensions<0.5.0,>=0.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from mypy->pytorchserver==0.1.0) (0.4.1)
Requirement already satisfied: typed-ast<1.4.0,>=1.3.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from mypy->pytorchserver==0.1.0) (1.3.5)
Installing collected packages: pytorchserver
  Found existing installation: pytorchserver 0.1.0
    Uninstalling pytorchserver-0.1.0:
      Successfully uninstalled pytorchserver-0.1.0
  Running setup.py develop for pytorchserver
Successfully installed pytorchserver
```

To run tests, please change the test file to point to your model.pt file. Then run the following command:

```bash
make test
```

The following shows the type of output you should see:

```
pytest -W ignore
=========================================================== test session starts ============================================================
platform darwin -- Python 3.7.3, pytest-4.5.0, py-1.8.0, pluggy-0.11.0
rootdir: /Users/animeshsingh/go/src/github.com/kubeflow/kfserving/python/pytorchserver
plugins: tornasync-0.6.0.post1
collected 1 item                                                                                                                           

pytorchserver/test_model.py .                        
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
