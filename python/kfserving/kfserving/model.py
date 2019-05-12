from typing import List, Any


class KFModel(object):

    def __init__(self, name: str):
        self.name = name
        self.ready = False

    def load(self):
        raise NotImplementedError

    def preprocess(self, inputs: List) -> List:
        raise NotImplementedError

    def predict(self, inputs: List) -> List:
        raise NotImplementedError

    def postprocess(self, inputs: List) -> List:
        raise NotImplementedError

    # TODO return type TBD
    def explain(self, inputs: List) -> Any:
        raise NotImplementedError

    # TODO return type TBD
    def detectOutlier(self, inputs: List):
        raise NotImplementedError
