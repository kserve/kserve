import xgboost as xgb
import os
from sklearn.datasets import load_iris
from xgbserver import XGBoostModel

model_dir = "."
BST_FILE = "model.bst"

def test_model():
    iris = load_iris()
    y = iris['target']
    X = iris['data']
    dtrain = xgb.DMatrix(X, label=y)
    param = {'max_depth': 6,
             'eta': 0.1,
             'silent': 1,
             'nthread': 4,
             'num_class': 10,
             'objective': 'multi:softmax'
             }
    xgb_model = xgb.train(params=param, dtrain=dtrain)
    model_file = os.path.join((model_dir), BST_FILE)
    xgb_model.save_model(model_file)
    server = XGBoostModel("xgbmodel", model_dir)
    server.load()
    request = [X[0].tolist()]
    response = server.predict(request)
    assert response == [0]