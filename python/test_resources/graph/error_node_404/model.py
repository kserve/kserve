import kserve
from typing import Dict, Union
import argparse
import logging
from fastapi.responses import JSONResponse
from fastapi import status, HTTPException
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
        raise HTTPException(status_code=404, detail="Intentional 404 code")


DEFAULT_MODEL_NAME = "model"

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument('--model_name', default=DEFAULT_MODEL_NAME,
                    help='The name that the model is served under.')

args, _ = parser.parse_known_args()
if __name__ == "__main__":
    model = SampleTemplateNode(name=args.model_name)
    kserve.ModelServer(workers=1).start([model])