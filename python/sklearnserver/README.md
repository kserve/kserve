# Scikit-Learn Server

[Scikit-Learn](https://scikit-learn.org/stable/) server is an implementation of KFServing for serving Scikit-learn models, and provides a Scikit-learn model implementation for prediction, pre and post processing. 

To start the server locally for development needs, run the following command under this folder in your github repository. 

```
pip install -e .
```

The following output indicates a successful install.

```
Obtaining file://kfserving/python/sklearn
Requirement already satisfied: kfserver==0.1.0 in /Users/username/Desktop/kfserving/python/kfserving (from sklearnserver==0.1.0) (0.1.0)
Requirement already satisfied: scikit-learn==0.20.3 in /anaconda3/lib/python3.6/site-packages (from sklearnserver==0.1.0) (0.20.3)
Requirement already satisfied: argparse>=1.4.0 in /anaconda3/lib/python3.6/site-packages (from sklearnserver==0.1.0) (1.4.0)
Requirement already satisfied: numpy>=1.8.2 in /anaconda3/lib/python3.6/site-packages (from sklearnserver==0.1.0) (1.16.2)
Requirement already satisfied: tornado>=1.4.1 in /anaconda3/lib/python3.6/site-packages (from kfserver==0.1.0->sklearnserver==0.1.0) (5.0.2)
Requirement already satisfied: scipy>=0.13.3 in /anaconda3/lib/python3.6/site-packages (from scikit-learn==0.20.3->sklearnserver==0.1.0) (1.1.0)
Installing collected packages: sklearnserver
  Found existing installation: sklearnserver 0.1.0
    Uninstalling sklearnserver-0.1.0:
      Successfully uninstalled sklearnserver-0.1.0
  Running setup.py develop for sklearnserver
Successfully installed sklearnserver
```

Once Scikit-learn server is up and running, you can check for successful installation by running the following command

```
python3 -m sklearnserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir
```

You can now point to your `joblib` model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage. Please follow [this sample](https://github.com/kubeflow/kfserving/tree/master/docs/samples/sklearn) to test your server by generating your own model. 

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
Obtaining file:///Users/animeshsingh/DevAdv/kfserving/python/sklearnserver
Requirement already satisfied: kfserver==0.1.0 in /Users/animeshsingh/DevAdv/kfserving/python/kfserving (from sklearnserver==0.1.0) (0.1.0)
Requirement already satisfied: scikit-learn==0.20.3 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (0.20.3)
Requirement already satisfied: argparse>=1.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (1.4.0)
Requirement already satisfied: numpy>=1.8.2 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (1.16.3)
Requirement already satisfied: joblib>=0.13.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (0.13.2)
Requirement already satisfied: pytest in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (4.5.0)
Requirement already satisfied: pytest-tornasync in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages(from sklearnserver==0.1.0) (0.6.0.post1)
Requirement already satisfied: mypy in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from sklearnserver==0.1.0) (0.701)
Requirement already satisfied: tornado>=1.4.1 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from kfserver==0.1.0->sklearnserver==0.1.0) (6.0.2)
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
Requirement already satisfied: mypy-extensions<0.5.0,>=0.4.0 in /Library/Frameworks/Python.framework/Versions/3.7/lib/python3.7/site-packages (from mypy->sklearnserver==0.1.0) (0.4.1)
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
