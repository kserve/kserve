from sklearn import svm
from sklearn import datasets
from sklearnserver import SKLearnModel
import joblib
import os

model_dir = "../../docs/samples/sklearn"
JOBLIB_FILE = "model.joblib"

def test_model():
     iris = datasets.load_iris()
     X, y = iris.data, iris.target
     sklearn_model = svm.SVC(gamma='scale')
     sklearn_model.fit(X, y)
     model_file = os.path.join((model_dir),JOBLIB_FILE)
     joblib.dump(value=sklearn_model, filename=model_file)
     server = SKLearnModel("sklearnmodel", model_dir)
     server.load()
     request = X[0:1].tolist()
     response = server.predict(request)
     assert response == [0]
