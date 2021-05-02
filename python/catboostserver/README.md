# Catboost server

[CatBoost](https://github.com/catboost/catboost) server is an implementation of KFServing for serving CatBoost models, and provides a CatBoost model implementation for prediction, pre and post processing.

To start the server locally for development needs, run the following command under this folder in your github repository.

```
pip install -e .
```

Once CatBoost server is up and running, you can check for successful installation by running the following command

```
python3 -m catboostserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   [--max_buffer_size MAX_BUFFER_SIZE] [--workers WORKERS]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
                   [--nthread NTHREAD]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. Please follow [this sample](https://github.com/kubeflow/kfserving/tree/master/docs/samples/sklearn) to test your server by generating your own model.

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
Obtaining file:///Users/animeshsingh/DevAdv/kfserving/python/sklearnserver
Requirement already satisfied: kfserving>=0.1.0 in /Users/animeshsingh/DevAdv/kfserving/python/kfserving (from sklearnserver==0.1.0) (0.1.0)
Requirement already satisfied: scikit-learn==0.20.3 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (0.20.3)
Requirement already satisfied: argparse>=1.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (1.4.0)
Requirement already satisfied: numpy>=1.8.2 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (1.16.3)
Requirement already satisfied: joblib>=0.13.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (0.13.2)
Requirement already satisfied: pytest in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (4.5.0)
Requirement already satisfied: pytest-tornasync in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages(from sklearnserver==0.1.0) (0.6.0.post1)
Requirement already satisfied: mypy in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (0.701)
Requirement already satisfied: tornado>=1.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserving>=0.1.0->sklearnserver==0.1.0) (6.0.2)
Requirement already satisfied: scipy>=0.13.3 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from scikit-learn==0.20.3->sklearnserver==0.1.0) (1.2.1)
Requirement already satisfied: attrs>=17.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->sklearnserver==0.1.0) (19.1.0)
Requirement already satisfied: setuptools in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (frompytest->sklearnserver==0.1.0) (40.8.0)
Requirement already satisfied: wcwidth in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->sklearnserver==0.1.0) (0.1.7)
Requirement already satisfied: atomicwrites>=1.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->sklearnserver==0.1.0) (1.3.0)
Requirement already satisfied: pluggy!=0.10,<1.0,>=0.9 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->sklearnserver==0.1.0) (0.11.0)
Requirement already satisfied: more-itertools>=4.0.0; python_version > "2.7" in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->sklearnserver==0.1.0) (7.0.0)
Requirement already satisfied: six>=1.10.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->sklearnserver==0.1.0) (1.12.0)
Requirement already satisfied: py>=1.5.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from pytest->sklearnserver==0.1.0) (1.8.0)
Requirement already satisfied: typed-ast<1.4.0,>=1.3.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from mypy->sklearnserver==0.1.0) (1.3.5)
Requirement already satisfied: mypy-extensions<0.5.0,>=0.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from mypy->sklearnserver==0.1.0) (0.4.1)
Installing collected packages: sklearnserver
  Found existing installation: sklearnserver 0.1.0
    Uninstalling sklearnserver-0.1.0:
      Successfully uninstalled sklearnserver-0.1.0
  Running setup.py develop for sklearnserver
Successfully installed sklearnserver
```

To run tests, please change the test file to point to your model.joblib file. Then run the following command:

```bash
make test
```

The following shows the type of output you should see:

```
pytest -W ignore
====================================================== test session starts ======================================================
platform darwin -- Python 3.7.3, pytest-4.5.0, py-1.8.0, pluggy-0.11.0
rootdir: /Users/animeshsingh/DevAdv/kfserving/python/sklearnserver
plugins: tornasync-0.6.0.post1
collected 1 item

sklearnserver/test_model.py .                                                                                             [100%]

=================================================== 1 passed in 0.43 seconds ====================================================
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

You should see an output similar to this

```bash
Sending build context to Docker daemon  110.6kB
Step 1/6 : FROM python:3.7-slim
 ---> ca7f9e245002
Step 2/6 : COPY . .
 ---> 874da9073958
Step 3/6 : RUN pip install --upgrade pip && pip install -e ./kfserving
 ---> Running in 132fedc2d28c
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
Removing intermediate container 132fedc2d28c
 ---> 151c55ffa783
Step 4/6 : RUN pip install -e ./sklearnserver
 ---> Running in 18d911ec940f
Obtaining file:///sklearnserver
Requirement already satisfied: kfserving>=0.1.0 in /kfserving (from sklearnserver==0.1.0) (0.1.0)
Collecting scikit-learn==0.20.3 (from sklearnserver==0.1.0)
  Downloading https://files.pythonhosted.org/packages/aa/cc/a84e1748a2a70d0f3e081f56cefc634f3b57013b16faa6926d3a6f0598df/scikit_learn-0.20.3-cp37-cp37m-manylinux1_x86_64.whl (5.4MB)
Requirement already satisfied: argparse>=1.4.0 in /usr/local/lib/python3.7/site-packages (from sklearnserver==0.1.0) (1.4.0)
Requirement already satisfied: numpy>=1.8.2 in /usr/local/lib/python3.7/site-packages (from sklearnserver==0.1.0) (1.16.3)
Collecting joblib>=0.13.0 (from sklearnserver==0.1.0)
  Downloading https://files.pythonhosted.org/packages/cd/c1/50a758e8247561e58cb87305b1e90b171b8c767b15b12a1734001f41d356/joblib-0.13.2-py2.py3-none-any.whl (278kB)
Requirement already satisfied: tornado>=1.4.1 in /usr/local/lib/python3.7/site-packages (from kfserving>=0.1.0->sklearnserver==0.1.0) (6.0.2)
Collecting scipy>=0.13.3 (from scikit-learn==0.20.3->sklearnserver==0.1.0)
  Downloading https://files.pythonhosted.org/packages/5d/bd/c0feba81fb60e231cf40fc8a322ed5873c90ef7711795508692b1481a4ae/scipy-1.3.0-cp37-cp37m-manylinux1_x86_64.whl (25.2MB)
Installing collected packages: scipy, scikit-learn, joblib, sklearnserver
  Running setup.py develop for sklearnserver
Successfully installed joblib-0.13.2 scikit-learn-0.20.3 scipy-1.3.0 sklearnserver
Removing intermediate container 18d911ec940f
 ---> 69eb68c41c67
Step 5/6 : COPY sklearnserver/model.joblib /tmp/models/model.joblib
 ---> 8c25d6b7b2b0
```

To push your image to your dockerhub repo,

```bash
docker push docker_user_name/sklearnserver:latest
```
