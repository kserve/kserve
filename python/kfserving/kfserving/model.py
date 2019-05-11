class KFModel(object):

    def __init__(self, name: str):
        self.name = name
        self.ready = False

    def load(self):
        raise NotImplementedError

    def preprocess(self, inputs):
        raise NotImplementedError

    def predict(self, inputs):
        raise NotImplementedError

    def postprocess(self, inputs):
        raise NotImplementedError

    def explain(self, inputs):
        raise NotImplementedError

    def detectOutlier(self, inputs):
        raise NotImplementedError

