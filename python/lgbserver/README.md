# Lightgbm Server

[LightGBM](https://lightgbm.readthedocs.io/en/latest/index.html ) server is an implementation of KFServing for serving LightGBM models, and provides an LightGBM model implementation for prediction, pre and post processing. In addition, model lifecycle management functionalities like liveness handler, metrics handler etc. are supported. 

To start the server locally for development needs, run the following command under this folder in your github repository. Also please ensure you have installed the [kfserving](../kfserving) before.

```
pip install -e .
```

The following output indicates a successful install.

```
Obtaining file://kfserving/python/lgbserver
Requirement already satisfied: kfserving>=0.4.0 in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (0.4.1)
Requirement already satisfied: lightgbm==2.3.1 in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (2.3.1)
Requirement already satisfied: pandas==0.25.3 in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (0.25.3)
Requirement already satisfied: argparse>=1.4.0 in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (1.4.0)

Successfully installed argparse-1.4.0 pandas==0.25.3 kfserving-0.4.1 lightgbm-2.3.1
```
Once LightGBM server is up and running, you can check for successful installation by running the following command

```
python3 -m lgbserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. Please follow [this sample](https://github.com/kubeflow/kfserving/tree/master/docs/samples/lightgbm) to test your server by generating your own model. 


## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
Obtaining file:///home/morgan/workspace/ai/kfserving/python/lgbserver
Requirement already satisfied: kfserving>=0.4.0 in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (0.4.1)
Requirement already satisfied: lightgbm==2.3.1 in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (2.3.1)
Requirement already satisfied: pandas==0.25.3 in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (0.25.3)
Requirement already satisfied: argparse>=1.4.0 in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (1.4.0)
Requirement already satisfied: pytest in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (6.1.2)
Requirement already satisfied: pytest-asyncio in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (0.14.0)
Requirement already satisfied: pytest-tornasync in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (0.6.0.post2)
Requirement already satisfied: mypy in /home/morgan/anaconda3/envs/kfserving/lib/python3.7/site-packages (from lgbserver==0.1.0) (0.790)
Installing collected packages: lgbserver
  Attempting uninstall: lgbserver
    Found existing installation: lgbserver 0.1.0
    Uninstalling lgbserver-0.1.0:
      Successfully uninstalled lgbserver-0.1.0
  Running setup.py develop for lgbserver
Successfully installed lgbserver
```


To run tests:

```bash
make test
```

The following shows the type of output you should see:

```
pytest -W ignore
================================================= test session starts =================================================
platform linux -- Python 3.7.3, pytest-4.4.2, py-1.8.0, pluggy-0.11.0
rootdir: /home/clive/go/src/github.com/kubeflow/kfserving/python/xgbserver
plugins: tornasync-0.6.0.post1
collected 2 items                                                                                                     

lgbserver/test_model.py ..                                                                                      [100%]

============================================== 2 passed in 0.44 seconds ===============================================

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

You should see an output similar to this

```bash
Sending build context to Docker daemon  100.4kB
Step 1/7 : FROM python:3.7-slim
 ---> ca7f9e245002
Step 2/7 : RUN apt-get update && apt-get install libgomp1
 ---> Using cache
 ---> f042a4cda36d
Step 3/7 : COPY . .
 ---> 588a1060a077
Step 4/7 : RUN pip install --upgrade pip && pip install -e ./kfserving
 ---> Running in 6af80216c578
Requirement already up-to-date: pip in /usr/local/lib/python3.7/site-packages (19.1.1)
Obtaining file:///kfserving
Collecting tornado>=1.4.1 (from kfserving>=0.1.0)
  Downloading https://files.pythonhosted.org/packages/03/3f/5f89d99fca3c0100c8cede4f53f660b126d39e0d6a1e943e95cc3ed386fb/tornado-6.0.2.tar.gz (481kB)
Collecting argparse>=1.4.0 (from kfserving>=0.1.0)
  Downloading https://files.pythonhosted.org/packages/f2/94/3af39d34be01a24a6e65433d19e107099374224905f1e0cc6bbe1fd22a2f/argparse-1.4.0-py2.py3-none-any.whl
Collecting numpy (from kfserving>=0.1.0)
  Downloading https://files.pythonhosted.org/packages/bb/76/24e9f32c78e6f6fb26cf2596b428f393bf015b63459468119f282f70a7fd/numpy-1.16.3-cp37-cp37m-manylinux1_x86_64.whl (17.3MB)
Building wheels for collected packages: tornado
  Building wheel for tornado (setup.py): started
  Building wheel for tornado (setup.py): finished with status 'done'
  Stored in directory: /root/.cache/pip/wheels/61/7e/7a/5e02e60dc329aef32ecf70e0425319ee7e2198c3a7cf98b4a2
Successfully built tornado
Installing collected packages: tornado, argparse, numpy, kfserving
  Running setup.py develop for kfserving
Successfully installed argparse-1.4.0 kfserving numpy-1.16.3 tornado-6.0.2
Removing intermediate container 6af80216c578
 ---> 4896221b50d2
Step 5/7 : RUN pip install -e ./xgbserver
 ---> Running in 337dd37591f7
Obtaining file:///xgbserver
Requirement already satisfied: kfserving>=0.1.0 in /kfserving (from xgbserver==0.1.0) (0.1.0)
Collecting xgboost==0.82 (from xgbserver==0.1.0)
  Downloading https://files.pythonhosted.org/packages/6a/49/7e10686647f741bd9c8918b0decdb94135b542fe372ca1100739b8529503/xgboost-0.82-py2.py3-none-manylinux1_x86_64.whl (114.0MB)
Collecting scikit-learn==0.20.3 (from xgbserver==0.1.0)
  Downloading https://files.pythonhosted.org/packages/aa/cc/a84e1748a2a70d0f3e081f56cefc634f3b57013b16faa6926d3a6f0598df/scikit_learn-0.20.3-cp37-cp37m-manylinux1_x86_64.whl (5.4MB)
Requirement already satisfied: argparse>=1.4.0 in /usr/local/lib/python3.7/site-packages (from xgbserver==0.1.0) (1.4.0)
Requirement already satisfied: tornado>=1.4.1 in /usr/local/lib/python3.7/site-packages (from kfserving>=0.1.0->xgbserver==0.1.0) (6.0.2)
Requirement already satisfied: numpy in /usr/local/lib/python3.7/site-packages (from kfserving>=0.1.0->xgbserver==0.1.0) (1.16.3)
Collecting scipy (from xgboost==0.82->xgbserver==0.1.0)
  Downloading https://files.pythonhosted.org/packages/5d/bd/c0feba81fb60e231cf40fc8a322ed5873c90ef7711795508692b1481a4ae/scipy-1.3.0-cp37-cp37m-manylinux1_x86_64.whl (25.2MB)
Installing collected packages: scipy, xgboost, scikit-learn, xgbserver
  Running setup.py develop for xgbserver
Successfully installed scikit-learn-0.20.3 scipy-1.3.0 xgboost-0.82 xgbserver
Removing intermediate container 337dd37591f7
 ---> f6fe392b31af
Step 6/7 : COPY xgbserver/model.bst /tmp/models/model.bst
 ---> e36c0c9a8fdb
Step 7/7 : ENTRYPOINT ["python"]
 ---> Running in a0b648905528
Removing intermediate container a0b648905528
 ---> bc7972611f73
Successfully built bc7972611f73
Successfully tagged animeshsingh/xgbserver:latest
```

To push your image to your dockerhub repo, 

```bash
docker push docker_user_name/lgbserver:latest
```
