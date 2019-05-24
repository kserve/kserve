# KFServing

KFServing is a unit of model serving. KFServing's python libraries implement a standardized KFServer library that is extended by model serving frameworks such as XGBoost and PyTorch. It encapsulates data plane API definitions and storage retrieval for models.

KFServing provides many functionalities, including among others:

* Registering a model and starting the server
* Prediction Handler
* Liveness Handler
* Metrics Handler

KFServing supports the following storage providers:

* Google Cloud Storage with a prefix: "gs://"
    * By default, it uses `GOOGLE_APPLICATION_CREDENTIALS` environment variable for user authentication.
    * If the GCS data source is public, `gsutil` will be used to download the artifacts.
* S3 Compatible Object Storage with a prefix "s3://"
    * By default, it uses `S3_ENDPOINT`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY` environment variables for user authentication.
* Local filesystem either without any prefix or with a prefix "file://". For example:
    * Absolute path: `/absolute/path` or `file:///absolute/path`
    * Relative path: `relative/path` or `file://relative/path`
    * For local filesystem, we recommended to use relative path without any prefix.

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

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
Obtaining file:///home/clive/go/src/github.com/kubeflow/kfserving/python/kfserving
Requirement already satisfied: tornado>=1.4.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0) (6.0.2)
Requirement already satisfied: argparse>=1.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0) (1.4.0)
Requirement already satisfied: numpy in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0) (1.16.3)
Requirement already satisfied: pytest in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0) (4.4.2)
Requirement already satisfied: pytest-tornasync in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0) (0.6.0.post1)
Requirement already satisfied: mypy in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0) (0.701)
Requirement already satisfied: setuptools in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->kfserver==0.1.0) (41.0.1)
Requirement already satisfied: py>=1.5.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->kfserver==0.1.0) (1.8.0)
Requirement already satisfied: attrs>=17.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->kfserver==0.1.0) (19.1.0)
Requirement already satisfied: six>=1.10.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->kfserver==0.1.0) (1.12.0)
Requirement already satisfied: atomicwrites>=1.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->kfserver==0.1.0) (1.3.0)
Requirement already satisfied: pluggy>=0.11 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->kfserver==0.1.0) (0.11.0)
Requirement already satisfied: more-itertools>=4.0.0; python_version > "2.7" in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->kfserver==0.1.0) (7.0.0)
Requirement already satisfied: typed-ast<1.4.0,>=1.3.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from mypy->kfserver==0.1.0) (1.3.5)
Requirement already satisfied: mypy-extensions<0.5.0,>=0.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from mypy->kfserver==0.1.0) (0.4.1)
Installing collected packages: kfserver
  Found existing installation: kfserver 0.1.0
    Uninstalling kfserver-0.1.0:
      Successfully uninstalled kfserver-0.1.0
  Running setup.py develop for kfserver
Successfully installed kfserver

```

To run tests:

```bash
make test
```

The following shows the type of output you should see:

```
pytest -W ignore
=================================================== test session starts ===================================================
platform linux -- Python 3.7.3, pytest-4.4.1, py-1.8.0, pluggy-0.9.0
rootdir: /home/clive/go/src/github.com/kubeflow/kfserving/python/kfserving
plugins: tornasync-0.6.0.post1
collected 7 items                                                                                                         

kfserving/test_server.py .......                                                                                    [100%]

================================================ 7 passed in 1.02 seconds =================================================
```

To run static type checks:

```bash
mypy --ignore-missing-imports xgbserver
```
An empty result will indicate success.
