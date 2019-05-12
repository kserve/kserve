import xgboost as xgb
from sklearn.datasets import load_digits
from xgbserver import XGBoostModel


def test_model():
    digits = load_digits(2)
    y = digits['target']
    X = digits['data']
    xgb_model = xgb.XGBClassifier(random_state=42).fit(X, y)
    server = XGBoostModel("xgbmodel", "", booster=xgb_model)
    request = [X[0].tolist()]
    response = server.predict(request)
    assert response == [0]
