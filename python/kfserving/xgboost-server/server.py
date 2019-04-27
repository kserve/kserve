from kfserving.kfserver import KFServer
import xgboost as xgb

class XGBoostServer(KFServer):
    def __init__(self):
        self.load("foo")
    
    def predict(self, request):
        try:
            dmatrix = xgb.DMatrix(request["instances"])
        except Exception as e:
            raise Exception("Could not initialize DMatrix from request: " + str(e))

        try: 
            predictions = self._booster.predict(dmatrix)
            pass
        except Exception as e:
            raise Exception("Could not initialize DMatrix from request: " + str(e))
        # predictions = model.predict(request.instances, signature_name=signature_name)
        return {"predictions": list(predictions)}

    def load(self, model_file):
        print("loading model")
        self._booster = xgb.Booster(model_file=model_file)

XGBoostServer().start()

