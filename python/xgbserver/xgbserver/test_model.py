import xgboost as xgb
from sklearn.datasets import load_digits
from xgbserver import XGBoostModel
from kfserving.server import TFSERVING_HTTP_PROTOCOL, SELDON_HTTP_PROTOCOL


def test_model_tensorflow_request():
    digits = load_digits(2)
    y = digits['target']
    X = digits['data']
    xgb_model = xgb.XGBClassifier(random_state=42).fit(X, y)
    server = XGBoostModel("xgbmodel", "", TFSERVING_HTTP_PROTOCOL, booster=xgb_model)
    request = {"instances": [X[0].tolist()]}
    response = server.predict(request)
    assert response["predictions"] == [0]


def test_model_seldon_request():
    digits = load_digits(2)
    y = digits['target']
    X = digits['data']
    xgb_model = xgb.XGBClassifier(random_state=42).fit(X, y)
    server = XGBoostModel("xgbmodel", "", SELDON_HTTP_PROTOCOL, booster=xgb_model)
    request = {"data": {"ndarray": [X[0].tolist()]}}
    response = server.predict(request)
    assert response["data"]["ndarray"] == [0]
