from typing import List, Dict


class KFExplainer(object):

    def __init__(self):
        self.ready = False

    def load(self):
        raise NotImplementedError

    def explain(self, inputs: List) -> Dict:
        raise NotImplementedError