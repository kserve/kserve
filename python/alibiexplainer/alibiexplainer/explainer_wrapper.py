from typing import List, Dict, Optional


class ExplainerWrapper(object):

    def validate(self, training_data_url: Optional[str]):
        pass

    def explain(self, inputs: List) -> Dict:
        pass
