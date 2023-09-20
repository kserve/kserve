import argparse
import logging
from typing import Dict, Union

import kserve
from kserve.model import InferRequest, ModelInferRequest

logger = logging.getLogger(__name__)


class SampleTemplateNode(kserve.Model):
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.load()

    def load(self):
        self.ready = True

    def predict(self, payload: Union[Dict, InferRequest, ModelInferRequest], headers) -> Dict:
        return {"message": "SUCCESS"}


DEFAULT_MODEL_NAME = "model"

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument('--model_name', default=DEFAULT_MODEL_NAME,
                    help='The name that the model is served under.')

args, _ = parser.parse_known_args()
if __name__ == "__main__":
    model = SampleTemplateNode(name=args.model_name)
    kserve.ModelServer(workers=1).start([model])
