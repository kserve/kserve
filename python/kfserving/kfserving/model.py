class KFModel(object):

    def __init__(self, name: str):
        self.name = name
        self.ready = False

    def predict(self, inputs):
        raise NotImplementedError

    def load(self):
        raise NotImplementedError
