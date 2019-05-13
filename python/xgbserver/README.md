# XGBoost Server

[XGBoost](https://xgboost.readthedocs.io/en/latest/index.html ) server is an implementation of KFServing for serving XGBoost models, and provides an XGBoost model implementation for prediction, pre and post processing. In addition, model lifecycle management functionalities like liveness handler, metrics handler etc. are supported. 

To start the server locally for development needs, run the following command under this folder in your github repository. Also please ensure you have installed the [kfserving](../kfserving) before.

```
pip install -e .
```

The following output indicates a successful install.

```
Obtaining file://kfserving/python/xgbserver
Requirement already satisfied: kfserver==0.1.0 in /kfserving/python/kfserving (from xgbserver==0.1.0) (0.1.0)
Requirement already satisfied: argparse>=1.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from xgbserver==0.1.0) (1.4.0)
Requirement already satisfied: tornado>=1.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserver==0.1.0->xgbserver==0.1.0) (6.0.2)

Collecting numpy (from xgboost==0.82->xgbserver==0.1.0)
  Downloading https://files.pythonhosted.org/packages/43/6e/71a3af8680a159a141fab5b4d19988111a09c02ffbfdeb42175cca0fa341/numpy-1.16.3-cp37-cp37m-macosx_10_6_intel.macosx_10_9_intel.macosx_10_9_x86_64.macosx_10_10_intel.macosx_10_10_x86_64.whl (13.9MB)
     |████████████████████████████████| 13.9MB 1.5MB/s

Installing collected packages: numpy, scipy, xgboost, scikit-learn, xgbserver
  Running setup.py install for xgboost ... done
  Running setup.py develop for xgbserver
Successfully installed numpy-1.16.3 scikit-learn-0.20.3 scipy-1.2.1 xgboost-0.82 xgbserver

```

Once XGBoost server is up and running, you can check for successful installation by running the following command

```
python3 -m xgbserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage.


## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
Obtaining file:///home/clive/go/src/github.com/kubeflow/kfserving/python/xgbserver
Requirement already satisfied: kfserver==0.1.0 in /home/clive/go/src/github.com/kubeflow/kfserving/python/kfserving (from xgbserver==0.1.0) (0.1.0)
Requirement already satisfied: xgboost==0.82 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from xgbserver==0.1.0) (0.82)
Requirement already satisfied: scikit-learn==0.20.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from xgbserver==0.1.0) (0.20.3)
Requirement already satisfied: argparse>=1.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from xgbserver==0.1.0) (1.4.0)
Requirement already satisfied: pytest in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from xgbserver==0.1.0) (4.4.2)
Requirement already satisfied: pytest-tornasync in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from xgbserver==0.1.0) (0.6.0.post1)
Requirement already satisfied: mypy in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from xgbserver==0.1.0) (0.701)
Requirement already satisfied: tornado>=1.4.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->xgbserver==0.1.0) (6.0.2)
Requirement already satisfied: numpy in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->xgbserver==0.1.0) (1.16.3)
Requirement already satisfied: scipy in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from xgboost==0.82->xgbserver==0.1.0) (1.2.1)
Requirement already satisfied: py>=1.5.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->xgbserver==0.1.0) (1.8.0)
Requirement already satisfied: attrs>=17.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->xgbserver==0.1.0) (19.1.0)
Requirement already satisfied: more-itertools>=4.0.0; python_version > "2.7" in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->xgbserver==0.1.0) (7.0.0)
Requirement already satisfied: pluggy>=0.11 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->xgbserver==0.1.0) (0.11.0)
Requirement already satisfied: setuptools in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->xgbserver==0.1.0) (41.0.1)
Requirement already satisfied: atomicwrites>=1.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->xgbserver==0.1.0) (1.3.0)
Requirement already satisfied: six>=1.10.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->xgbserver==0.1.0) (1.12.0)
Requirement already satisfied: mypy-extensions<0.5.0,>=0.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from mypy->xgbserver==0.1.0) (0.4.1)
Requirement already satisfied: typed-ast<1.4.0,>=1.3.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from mypy->xgbserver==0.1.0) (1.3.5)
Installing collected packages: xgbserver
  Found existing installation: xgbserver 0.1.0
    Uninstalling xgbserver-0.1.0:
      Successfully uninstalled xgbserver-0.1.0
  Running setup.py develop for xgbserver
Successfully installed xgbserver
```


to run tests:

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

xgbserver/test_model.py ..                                                                                      [100%]

============================================== 2 passed in 0.44 seconds ===============================================

```

To run static type checks:

```bash
mypy --ignore-missing-imports xgbserver
```
An empty result will indicate success.
