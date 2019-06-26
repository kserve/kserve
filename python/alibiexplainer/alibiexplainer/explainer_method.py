from typing import List, Dict, Optional


class ExplainerMethodImpl(object):

    def validate(self, training_data_url: Optional[str]):
        pass

    def prepare(self, training_data_url: str):
        pass

    def explain(self, inputs: List) -> Dict:
        pass
