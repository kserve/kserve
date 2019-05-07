TODO
=======
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
