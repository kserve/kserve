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

You can now point to your `joblib` model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage. Please follow [this sample](../../../docs/samples/sklearn/) to test your server by generating your own model. 
