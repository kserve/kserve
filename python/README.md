# Scikit-learn Server

[Scikit-learn](https://scikit-learn.org/stable/) server is an implementation of KFServing for serving Scikit-learn models, and provides a Scikit-learn model implementation for prediction, pre and post processing. In addition, model lifecycle management functionalities like liveness handler, metrics handler etc. are supported.

To start the server locally for development needs, run the following command under this folder in your github repository. Also please ensure you have installed the [kfserving](../kfserving) before.

```
pip install -e .
```

The following output indicates a successful install.

```
Obtaining file://kfserving/python/sklearn
Requirement already satisfied: kfserver==0.1.0 in /Users/tommyli/Desktop/kfserving/python/kfserving (from sklearnserver==0.1.0) (0.1.0)
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

You can now point to your `joblib` model file and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage.


## Testing
To test the Scikit-learn Server, first we need to generate a simple scikit-learn model using Python
```python
from sklearn import svm
from sklearn import datasets
from joblib import dump
clf = svm.SVC(gamma='scale')
iris = datasets.load_iris()
X, y = iris.data, iris.target
clf.fit(X, y)
dump(clf, 'model.joblib')
```

Then, we can run the Scikit-learn Server using the generated model.
```shell
python -m sklearnserver --model_dir model.joblib --model_name svm
```

From here, we can load the dataset we have from Python and do some simple predictions
```python
from sklearn import datasets
import requests
iris = datasets.load_iris()
X, y = iris.data, iris.target
formData = {
    'instances': X[0:1].tolist()
}
res = requests.post('http://localhost:8080/models/svm:predict', json=formData)
print(res)
print(res.text)
```

## Build with Docker
To build this server using Docker, please run the following commands in this directory:
```shell
docker build -t sklearnserver ..
```

Then, you can run the sklearnserver using Docker with the following commands:
```shell
docker run -p 8080:8080 -v "$(pwd)"/model.joblib:/tmp/model.joblib sklearnserver -m sklearnserver --model_dir /tmp/model.joblib --model_name svm
```
