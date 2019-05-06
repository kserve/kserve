# KFServing

KFServing is a unit of model serving, and every model type (XGBosst, PyTorch, Tensorflow etc.) extends KFServing. At a highlevel it defines a model, a server and storage mechanism for models.

A model has essentially three functions:

* Pre processing
* Prediction 
* Post processing

A server on the other hand provides many funtionalities, inclding among others

* Registering a model and starting the server
* Prediction Handler
* Liveness Handler 
* Metrics Handler 

Models to load in KFServing can come from following three types of storage locations currently

* Google Cloud Storage with a prefix: "gs://"
* S3 Compatible Object Storage with a prefix "s3://"
* Local filesystem with a prefix "/"

To start the server locally on your machine for development needs, run the following command under this folder

```
pip3 install -e .
```

You would see an output similar to the one below, indicating that kfserving has been installed.

```
Obtaining file:///Users/animeshsingh/DevAdv/kfserving/python/kfserving
Requirement already satisfied: tornado>=1.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserver==0.1.0) (6.0.2)
Requirement already satisfied: argparse>=1.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserver==0.1.0) (1.4.0)
Installing collected packages: kfserver
  Running setup.py develop for kfserver
Successfully installed kfserver
```