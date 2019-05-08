# KFServing

KFServing is a unit of model serving. KFServing's python libraries implement a standardized KFServer library that is extended by model serving frameworks such as XGBoost and PyTorch. It encapsulates data plane API definitions and storage retrieval for models.

KFServing provides many funtionalities, including among others:

* Registering a model and starting the server
* Prediction Handler
* Liveness Handler 
* Metrics Handler 

KFServing supports the following storage providers:

* Google Cloud Storage with a prefix: "gs://"
* S3 Compatible Object Storage with a prefix "s3://"
* Local filesystem with a prefix "/"

To start the server locally on your machine for development needs, run the following command under this folder

```
pip3 install -e .
```

The following output indicates a successful install.

```
Obtaining file:///Users/animeshsingh/DevAdv/kfserving/python/kfserving
Requirement already satisfied: tornado>=1.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserver==0.1.0) (6.0.2)
Requirement already satisfied: argparse>=1.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserver==0.1.0) (1.4.0)
Installing collected packages: kfserver
  Running setup.py develop for kfserver
Successfully installed kfserver
```

## Development

To install development requirements

```
pip install -r dev_requirements.txt
```

To run tests:

```
make test
```

The following shows the type of output you should see:

```bash
pytest -W ignore
=================================================== test session starts ===================================================
platform linux -- Python 3.7.3, pytest-4.4.1, py-1.8.0, pluggy-0.9.0
rootdir: /home/clive/go/src/github.com/kubeflow/kfserving/python/kfserving
plugins: tornasync-0.6.0.post1
collected 7 items                                                                                                         

kfserving/test_server.py .......                                                                                    [100%]

================================================ 7 passed in 1.02 seconds =================================================
```