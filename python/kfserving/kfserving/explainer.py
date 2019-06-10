from typing import List, Any


class KFExplainer(object):

    def __init__(self):
        self.ready = False

    def load(self):
        raise NotImplementedError

    # TODO return type TBD
    def explain(self, inputs: List) -> Any:
        raise NotImplementedError