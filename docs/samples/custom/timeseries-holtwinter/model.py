import kfserving
from typing import List, Dict
import base64
import io
import numpy as np
import joblib
import pickle


class KFServingSampleModel(kfserving.KFModel):
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        #Loading the joblib files 
        self.holt =  joblib.load('model_holt.sav')
        self.ready = True

    def predict(self, request: Dict) -> Dict:
        inputs = request["instances"]
        weeks = int(inputs[0]["image"]["weeks"])
        print(weeks)
        #creating  dictionary to return
        dict1={}
        dict1["forecast_weeks"]=str(weeks)
        #Holt values
        n=np.array((self.holt).forecast(weeks))
        dict1["predicted_values_holt"]=str(n)
        return {"predictions": dict1}


if __name__ == "__main__":
    model = KFServingSampleModel("sales-application")
    model.load()
    kfserving.KFServer(workers=1).start([model])
