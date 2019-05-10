## Creating your own model and testing the SKLearn server.

To test the Scikit-learn Server, first we need to generate a simple scikit-learn model using Python. Sample model is also added in this directory to reuse if needed.

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

Then, we can run the Scikit-learn Server using the generated model and test for prediction. Models can be on local filesystem, S3 compatible object storage or Google Cloud Storage.

```shell
python -m sklearnserver --model_dir model.joblib --model_name svm
```

We can also use the inbuilt sklearn support for sample datasets and do some simple predictions

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