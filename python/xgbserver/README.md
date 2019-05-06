# XG Boost Server

[XGBoost](https://xgboost.readthedocs.io/en/latest/index.html )server is an implementation of KFServing for serving XGBoost models, providing an XGBoost implementation for 

* Pre processing
* Prediction 
* Post processing

In addition, model lifecycle management fucntionalities like liveness handler, metrics handler etc. are supported. 

To start the server locally for development needs, run the following command under this folder, please ensure you have installed the kfserving before.

```
pip install -e .
```

You would see output similar to the one below, indicating that kfserving has been installed.

```
Obtaining file://kfserving/python/xgbserver
Requirement already satisfied: kfserver==0.1.0 in /kfserving/python/kfserving (from xgbserver==0.1.0) (0.1.0)
Requirement already satisfied: argparse>=1.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from xgbserver==0.1.0) (1.4.0)
Requirement already satisfied: tornado>=1.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserver==0.1.0->xgbserver==0.1.0) (6.0.2)

Installing collected packages: numpy, scipy, xgboost, scikit-learn, xgbserver
  Running setup.py install for xgboost ... done
  Running setup.py develop for xgbserver
Successfully installed numpy-1.16.3 scikit-learn-0.20.3 scipy-1.2.1 xgboost-0.82 xgbserver

```

Once XGBoost server is up and running, you can check for the installation by running the following command

```
python3 -m xgbserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage.
