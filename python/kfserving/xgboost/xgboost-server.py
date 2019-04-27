import kfserving

class XGBoostServer(kfserving.KFServer):
    def __init__(self):
        print("loading model")
    
    def predict(self, tensor):
        return self.bar()

    def bar(self):
        return "ayyyyy bar"

XGBoostServer().start()